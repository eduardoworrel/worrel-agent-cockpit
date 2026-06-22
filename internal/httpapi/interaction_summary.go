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

// HeadlessLLM é a dependência mínima para resumir uma sessão (satisfeita por um
// adapter.Adapter com capacidade headless). nil = resumo por IA indisponível.
type HeadlessLLM interface {
	RunHeadless(ctx context.Context, prompt string, opts adapter.HeadlessOpts) (string, error)
}

// Limiares do resumo de progresso: só vale a pena resumir com algum conteúdo, e
// só regeneramos quando o transcript cresceu o bastante — limita o custo do LLM
// a ~uma chamada por avanço real da sessão, não por poll.
const (
	progressMinEvents = 2
	progressRegenEvery = 3
	progressTimeout    = 30 * time.Second
)

// progressCache guarda as linhas de progresso por sessão e o tamanho do
// transcript em que foram geradas, além de marcar gerações em voo (uma por vez).
type progressCache struct {
	mu       sync.Mutex
	lines    map[string][]string
	atLen    map[string]int
	inflight map[string]bool
}

func newProgressCache() *progressCache {
	return &progressCache{lines: map[string][]string{}, atLen: map[string]int{}, inflight: map[string]bool{}}
}

func (c *progressCache) get(id string) ([]string, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lines[id], c.atLen[id]
}

// claim decide se vale gerar agora e, em caso afirmativo, marca em-voo. Retorna
// false quando já há geração em voo ou o transcript não avançou o bastante.
func (c *progressCache) claim(id string, curLen int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inflight[id] || curLen < progressMinEvents {
		return false
	}
	had, seen := c.atLen[id]
	if seen && curLen-had < progressRegenEvery {
		return false
	}
	c.inflight[id] = true
	return true
}

func (c *progressCache) store(id string, lines []string, atLen int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines[id] = lines
	c.atLen[id] = atLen
	delete(c.inflight, id)
}

func (c *progressCache) release(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.inflight, id)
}

// attachEngineSummary gera (via LLM, assíncrono e cacheado) o TÍTULO vivo + os
// EVENTOS NARRADOS de uma sessão do MOTOR a partir do histórico. Os eventos
// narrados ("o agente fez X", "está fazendo Y") substituem as mensagens cruas
// no card (é o que a Home deve mostrar). O título vira o nome da sessão.
func (s *Server) attachEngineSummary(snap *agui.Snapshot) {
	if s.deps.Summarizer == nil {
		return
	}
	if s.deps.Store != nil && !s.deps.Store.EngineEnabled("summary", snap.SessionID, false) {
		return
	}
	id := snap.SessionID
	// usa os eventos narrados em cache no card (em vez das mensagens cruas).
	if lines, _ := s.titles.get(id); len(lines) > 0 {
		snap.Progress = lines
	}
	if len(snap.History) < 2 || !s.titles.claim(id, len(snap.History)) {
		return
	}
	atLen := len(snap.History)
	prompt := agui.ProgressPrompt(historyToEvents(snap.History))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), progressTimeout)
		defer cancel()
		out, err := s.deps.Summarizer.RunHeadless(ctx, prompt, adapter.HeadlessOpts{})
		if err != nil {
			s.titles.release(id)
			return
		}
		// auditoria inegociável: grava o prompt enviado e a resposta crua da IA.
		if s.deps.Store != nil {
			_ = s.deps.Store.LogEngineRun(&store.EngineLogEntry{
				EngineID: "summary", SessionID: id, Trigger: "realtime",
				Input: prompt, Output: out,
			})
		}
		title, lines := agui.ParseProgress(out)
		s.titles.store(id, lines, atLen)
		if title != "" {
			_ = s.deps.Store.SetSessionTitle(id, title)
			s.deps.Bus.Publish(bus.Event{Type: "session.titled", Payload: map[string]any{"id": id}})
		}
		s.deps.Bus.Publish(bus.Event{Type: "interaction.changed", Payload: map[string]any{"session_id": id}})
	}()
}

// historyToEvents converte o histórico AG-UI no formato de transcript que o
// ProgressPrompt consome.
func historyToEvents(h []agui.HistoryLine) []*store.TranscriptEvent {
	out := make([]*store.TranscriptEvent, 0, len(h))
	for _, l := range h {
		out = append(out, &store.TranscriptEvent{Role: l.Role, Kind: "text", Content: l.Text})
	}
	return out
}

// attachProgress anexa as linhas de progresso em cache ao snapshot e, se o
// transcript avançou, dispara uma regeneração assíncrona (não bloqueia o GET).
// Ao concluir, publica interaction.changed para a Home rebuscar o snapshot.
func (s *Server) attachProgress(snap *agui.Snapshot, events []*store.TranscriptEvent) {
	lines, _ := s.progress.get(snap.SessionID)
	snap.Progress = lines

	if s.deps.Summarizer == nil || snap.State == agui.StateEnded {
		return
	}
	// toggle de custo: resumo desligado (global ou por-sessão) → não chama IA.
	if s.deps.Store != nil && !s.deps.Store.EngineEnabled("summary", snap.SessionID, false) {
		return
	}
	if !s.progress.claim(snap.SessionID, len(events)) {
		return
	}
	id := snap.SessionID
	prompt := agui.ProgressPrompt(events)
	atLen := len(events)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), progressTimeout)
		defer cancel()
		out, err := s.deps.Summarizer.RunHeadless(ctx, prompt, adapter.HeadlessOpts{})
		if err != nil {
			s.progress.release(id)
			return
		}
		// auditoria inegociável: grava o prompt enviado e a resposta crua da IA.
		if s.deps.Store != nil {
			_ = s.deps.Store.LogEngineRun(&store.EngineLogEntry{
				EngineID: "summary", SessionID: id, Trigger: "realtime",
				Input: prompt, Output: out,
			})
		}
		title, parsed := agui.ParseProgress(out)
		if len(parsed) == 0 && title == "" {
			s.progress.release(id)
			return
		}
		s.progress.store(id, parsed, atLen)
		// título "vivo": sobrescreve o nome da sessão e avisa a UI (sidebar/card).
		if title != "" {
			_ = s.deps.Store.SetSessionTitle(id, title)
			s.deps.Bus.Publish(bus.Event{Type: "session.titled", Payload: map[string]any{"id": id}})
		}
		s.deps.Bus.Publish(bus.Event{Type: "interaction.changed", Payload: map[string]any{"session_id": id}})
	}()
}
