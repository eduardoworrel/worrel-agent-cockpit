package httpapi

import (
	"context"
	"sync"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

const interpretTimeout = 25 * time.Second

// interpretCache guarda a interpretação por sessão e a mensagem que a originou
// (regenera quando o agente fala algo novo). Uma geração em voo por sessão.
type interpretCache struct {
	mu       sync.Mutex
	result   map[string]agui.Interpretation
	forMsg   map[string]string
	inflight map[string]bool
}

func newInterpretCache() *interpretCache {
	return &interpretCache{result: map[string]agui.Interpretation{}, forMsg: map[string]string{}, inflight: map[string]bool{}}
}

func (c *interpretCache) get(id, msg string) (agui.Interpretation, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.forMsg[id] == msg {
		r, ok := c.result[id]
		return r, ok
	}
	return agui.Interpretation{}, false
}

func (c *interpretCache) claim(id, msg string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inflight[id] || c.forMsg[id] == msg {
		return false
	}
	c.inflight[id] = true
	return true
}

func (c *interpretCache) store(id, msg string, r agui.Interpretation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.result[id] = r
	c.forMsg[id] = msg
	delete(c.inflight, id)
}

func (c *interpretCache) release(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.inflight, id)
}

// attachInterpretation: quando a sessão do motor encerra o turno FALANDO (auto-
// mode, sem permissão pendente), interpreta a fala via LLM e expõe como um
// Interrupt (kind choice/text, sem request_id) para a Home renderizar opções/
// campo de resposta. Assíncrono e cacheado por mensagem; publica
// interaction.changed ao concluir.
func (s *Server) attachInterpretation(snap *agui.Snapshot) {
	if s.deps.Summarizer == nil || snap.Interrupt != nil ||
		snap.State != agui.StateAwaiting || snap.Message == "" {
		return
	}
	// toggle de custo: interpretação é só-global (default ON).
	if s.deps.Store != nil && !s.deps.Store.EngineEnabled("interpret", "", true) {
		return
	}
	id := snap.SessionID
	if r, ok := s.interpret.get(id, snap.Message); ok {
		snap.Interrupt = interpretationToInterrupt(r, snap.Message)
		return
	}
	if !s.interpret.claim(id, snap.Message) {
		return
	}
	msg := snap.Message
	prompt := agui.InterpretPrompt(msg, snap.History)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), interpretTimeout)
		defer cancel()
		out, err := s.deps.Summarizer.RunHeadless(ctx, prompt, adapter.HeadlessOpts{})
		if err != nil {
			s.interpret.release(id)
			return
		}
		// auditoria inegociável: grava o prompt enviado e a resposta crua da IA.
		if s.deps.Store != nil {
			_ = s.deps.Store.LogEngineRun(&store.EngineLogEntry{
				EngineID: "interpret", SessionID: id, Trigger: "agent_self",
				Input: prompt, Output: out,
			})
		}
		s.interpret.store(id, msg, agui.ParseInterpretation(out))
		s.deps.Bus.Publish(bus.Event{Type: "interaction.changed", Payload: map[string]any{"session_id": id}})
	}()
}

// interpretationToInterrupt vira um Interrupt SÓ quando há opções discretas reais
// (kind=choice). Para fala comum (saudação, pergunta aberta, statement) devolve
// nil — a sessão fica em chat livre, sem transformar tudo numa "decisão".
func interpretationToInterrupt(r agui.Interpretation, fallback string) *agui.Interrupt {
	if r.Kind != agui.KindChoice || len(r.Options) == 0 {
		return nil
	}
	prompt := r.Prompt
	if prompt == "" {
		prompt = fallback
	}
	return &agui.Interrupt{Kind: agui.KindChoice, Prompt: prompt, Options: r.Options}
}
