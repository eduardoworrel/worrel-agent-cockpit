package httpapi

import (
	"context"
	"sync"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

const requestSummaryTimeout = 25 * time.Second

// requestSummaryCache guarda o resumo do pedido por sessão e a mensagem que o
// originou — regenera quando o usuário manda um pedido novo (P2: última msg).
// Uma geração em voo por sessão. Mesmo formato do interpretCache.
type requestSummaryCache struct {
	mu       sync.Mutex
	result   map[string]string
	forMsg   map[string]string
	inflight map[string]bool
}

func newRequestSummaryCache() *requestSummaryCache {
	return &requestSummaryCache{result: map[string]string{}, forMsg: map[string]string{}, inflight: map[string]bool{}}
}

func (c *requestSummaryCache) get(id, msg string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.forMsg[id] == msg {
		r, ok := c.result[id]
		return r, ok
	}
	return "", false
}

func (c *requestSummaryCache) claim(id, msg string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inflight[id] || c.forMsg[id] == msg {
		return false
	}
	c.inflight[id] = true
	return true
}

func (c *requestSummaryCache) store(id, msg, r string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.result[id] = r
	c.forMsg[id] = msg
	delete(c.inflight, id)
}

func (c *requestSummaryCache) release(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.inflight, id)
}

// attachRequestSummary condensa (via LLM, assíncrono e cacheado por mensagem) o
// último pedido do usuário e o expõe em snap.RequestSummary para o bloco "Seu
// pedido". Nunca propaga erro: em falha o front cai no user_message cru.
func (s *Server) attachRequestSummary(snap *agui.Snapshot) {
	if s.deps.Summarizer == nil || snap.UserMessage == "" {
		return
	}
	if s.deps.Store != nil && !s.deps.Store.EngineEnabled("request_summary", snap.SessionID, true) {
		return
	}
	id := snap.SessionID
	if r, ok := s.requestSummary.get(id, snap.UserMessage); ok {
		snap.RequestSummary = r
		return
	}
	if !s.requestSummary.claim(id, snap.UserMessage) {
		return
	}
	msg := snap.UserMessage
	prompt := agui.RequestSummaryPrompt(msg)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), requestSummaryTimeout)
		defer cancel()
		llm, opts := s.summarizerFor("request_summary", id)
		out, err := llm.RunHeadless(ctx, prompt, opts)
		if err != nil {
			s.requestSummary.release(id)
			return
		}
		if s.deps.Store != nil {
			_ = s.deps.Store.LogEngineRun(&store.EngineLogEntry{
				EngineID: "request_summary", SessionID: id, Trigger: "realtime",
				Input: prompt, Output: out,
			})
		}
		summary := agui.ParseRequestSummary(out)
		if summary == "" {
			s.requestSummary.release(id)
			return
		}
		s.requestSummary.store(id, msg, summary)
		s.deps.Bus.Publish(bus.Event{Type: "interaction.changed", Payload: map[string]any{"session_id": id}})
	}()
}
