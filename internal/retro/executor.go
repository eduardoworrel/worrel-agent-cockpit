package retro

import (
	"context"
	"encoding/json"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type clock interface{ Now() time.Time }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Executor é o núcleo arquitetural: máquina de estados persistida, orçada,
// pausável/retomável/cancelável, com cursor por sessão (critérios 3, 4, 7, 8, 11).
type Executor struct {
	store     *store.Store
	engine    *distill.Engine
	bus       *bus.Bus
	clk       clock
	batchSize int
	calls     []time.Time // timestamps das invocações para a janela deslizante de 1h
	// headless mapeia ID de adapter → cliente headless, p/ override de provider por run.
	headless map[string]distill.Headless
}

func NewExecutor(s *store.Store, eng *distill.Engine, b *bus.Bus, headless map[string]distill.Headless) *Executor {
	return &Executor{store: s, engine: eng, bus: b, clk: realClock{}, batchSize: 5, headless: headless}
}

func (e *Executor) withClock(c clock) *Executor    { e.clk = c; return e }
func (e *Executor) withBatchSize(n int) *Executor  { e.batchSize = n; return e }

func (e *Executor) publish(typ, runID string, extra map[string]any) {
	if e.bus == nil {
		return
	}
	p := map[string]any{"run_id": runID}
	for k, v := range extra {
		p[k] = v
	}
	e.bus.Publish(bus.Event{Type: typ, Payload: p})
}

// countInWindow conta invocações na última hora a partir de `now`.
func (e *Executor) countInWindow(now time.Time) int {
	cutoff := now.Add(-time.Hour)
	n := 0
	for _, t := range e.calls {
		if t.After(cutoff) {
			n++
		}
	}
	return n
}

// Run avança a run enquanto houver orçamento e pendentes. Devolve o status final.
func (e *Executor) Run(ctx context.Context, runID string) (string, error) {
	run, err := e.store.GetRetroRun(runID)
	if err != nil {
		return "", err
	}
	if run.Status != "running" {
		return run.Status, nil
	}
	suppress := run.Depth == "leve"

	// Resolve provider/modelo escolhidos para ESTA run (persistidos no scope JSON).
	// Adapter vazio → usa o cliente fixado no boot (override nil).
	var sc Scope
	_ = json.Unmarshal([]byte(run.Scope), &sc)
	var override distill.Headless
	if sc.Adapter != "" && e.headless != nil {
		override = e.headless[sc.Adapter]
	}

	byProject, err := e.store.PendingRunSessionsByProject(runID)
	if err != nil {
		return "", err
	}

	// Total de sessões pendentes nesta passagem da run + contador acumulado de
	// concluídas, para alimentar a barra de progresso por estágio na UI.
	total := 0
	for _, sessions := range byProject {
		total += len(sessions)
	}
	done := 0
	// Progress inicial: a UI conhece o total imediatamente (done=0).
	e.publish("retro.run.progress", runID, map[string]any{"done": done, "total": total})

	for projectID, sessions := range byProject {
		for start := 0; start < len(sessions); start += e.batchSize {
			end := start + e.batchSize
			if end > len(sessions) {
				end = len(sessions)
			}
			lote := sessions[start:end]

			// cancelamento
			cur, err := e.store.GetRetroRun(runID)
			if err != nil {
				return "", err
			}
			if cur.Status == "canceled" {
				return "canceled", nil
			}
			// teto total
			if cur.BudgetTotal > 0 && cur.LLMCalls >= cur.BudgetTotal {
				_ = e.store.SetRetroRunStatus(runID, "paused")
				e.publish("retro.run.paused", runID, map[string]any{"reason": "budget_total"})
				return "paused", nil
			}
			// rate limit por hora
			now := e.clk.Now()
			if cur.BudgetPerHour > 0 && e.countInWindow(now) >= int(cur.BudgetPerHour) {
				_ = e.store.SetRetroRunStatus(runID, "paused")
				e.publish("retro.run.paused", runID, map[string]any{"reason": "budget_per_hour"})
				return "paused", nil
			}

			if _, err := e.engine.AnalyzeBatchDepth(ctx, projectID, lote, distill.AnalyzeOpts{SuppressSkills: suppress, Origin: "retroativa", Headless: override, Model: sc.Model}); err != nil {
				return "", err
			}
			e.calls = append(e.calls, now)
			_ = e.store.IncrRunLLMCalls(runID, 1)
			for _, sid := range lote {
				_ = e.store.MarkRunSessionDone(runID, sid)
			}
			done += len(lote)
			e.publish("retro.run.progress", runID, map[string]any{"project_id": projectID, "batch": len(lote), "done": done, "total": total})
		}
	}

	_ = e.store.SetRetroRunStatus(runID, "done")
	e.publish("retro.run.finished", runID, nil)
	return "done", nil
}

func (e *Executor) Pause(runID string) error  { return e.store.SetRetroRunStatus(runID, "paused") }
func (e *Executor) Cancel(runID string) error { return e.store.SetRetroRunStatus(runID, "canceled") }
