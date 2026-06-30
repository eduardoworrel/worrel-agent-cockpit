package httpapi

import (
	"context"
	"sync"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

const askHTMLTimeout = 30 * time.Second

// askHTMLCache guarda o HTML rico + widget por sessão e o conteúdo de "expects"
// que o originou. Chavear pelo conteúdo dá o edge-trigger desejado: um novo
// episódio de awaiting tem expects diferente → regenera; polls do MESMO awaiting
// reaproveitam (sem chamar o LLM a cada poll e sem piscar a tela).
type askHTMLCache struct {
	mu       sync.Mutex
	result   map[string]agui.AskHTML
	forKey   map[string]string
	inflight map[string]bool
}

func newAskHTMLCache() *askHTMLCache {
	return &askHTMLCache{result: map[string]agui.AskHTML{}, forKey: map[string]string{}, inflight: map[string]bool{}}
}

func (c *askHTMLCache) get(id, key string) (agui.AskHTML, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.forKey[id] == key {
		r, ok := c.result[id]
		return r, ok
	}
	return agui.AskHTML{}, false
}

func (c *askHTMLCache) claim(id, key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inflight[id] || c.forKey[id] == key {
		return false
	}
	c.inflight[id] = true
	return true
}

func (c *askHTMLCache) store(id, key string, r agui.AskHTML) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.result[id] = r
	c.forKey[id] = key
	delete(c.inflight, id)
}

func (c *askHTMLCache) release(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.inflight, id)
}

// expectsOf é o que a IA espera do usuário: a pergunta bloqueante pendente ou a
// última fala da IA. Espelha a lógica do InteractionPanel (interrupt ?? message).
func expectsOf(snap *agui.Snapshot) string {
	if snap.Interrupt != nil && snap.Interrupt.Prompt != "" {
		return snap.Interrupt.Prompt
	}
	return snap.Message
}

// attachAskHTML gera (via LLM, assíncrono e cacheado por conteúdo) a apresentação
// rica em HTML + o widget de resposta dinâmico, expostos em snap.AskHTML /
// snap.ResponseWidget. Só roda quando a sessão espera o usuário. Nunca propaga
// erro: em falha o front cai no render markdown atual.
func (s *Server) attachAskHTML(snap *agui.Snapshot) {
	if s.deps.Summarizer == nil || !snap.NeedsAttention() {
		return
	}
	// Permissões (can_use_tool) NÃO ganham HTML rico: respondem-se por allow/deny
	// pelo control protocol — choices clicáveis mandariam um prompt de texto e
	// deixariam a tool pendente (tela travada). Além disso a permissão surge no
	// MEIO do turno, a cada ferramenta — gerar aqui é o "sem parar". O HTML rico
	// fica para o fim de turno e perguntas ask_user (choice/text).
	if snap.Interrupt != nil && snap.Interrupt.Kind == agui.KindPermission {
		return
	}
	expects := expectsOf(snap)
	if expects == "" {
		return
	}
	if s.deps.Store != nil && !s.deps.Store.EngineEnabled("ask_html", snap.SessionID, true) {
		return
	}
	id := snap.SessionID
	if r, ok := s.askHTML.get(id, expects); ok {
		snap.AskHTML = r.HTML
		snap.ResponseWidget = r.Widget
		return
	}
	// Sem HTML para este conteúdo ainda: ou começamos a gerar agora, ou já está em
	// voo. Em ambos os casos o front mostra "preparando…" em vez do markdown cru —
	// evita o flash do modelo antigo antes do HTML rico chegar.
	snap.AskHTMLPending = true
	if !s.askHTML.claim(id, expects) {
		return
	}
	prompt := agui.AskHTMLPrompt(expects, snap.History)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), askHTMLTimeout)
		defer cancel()
		llm, opts := s.summarizerFor("ask_html", id)
		out, err := llm.RunHeadless(ctx, prompt, opts)
		if err == nil && s.deps.Store != nil {
			_ = s.deps.Store.LogEngineRun(&store.EngineLogEntry{
				EngineID: "ask_html", SessionID: id, Trigger: "realtime",
				Input: prompt, Output: out,
			})
		}
		res := agui.AskHTML{}
		if err == nil {
			res = agui.ParseAskHTML(out)
		}
		// Sucesso OU falha: grava o resultado (HTML vazio em falha) para ESTE
		// conteúdo. Vazio em cache faz o front cair no markdown e NÃO ficar
		// retentando (loading eterno); regenera só quando o expects mudar.
		s.askHTML.store(id, expects, res)
		s.deps.Bus.Publish(bus.Event{Type: "interaction.changed", Payload: map[string]any{"session_id": id}})
	}()
}
