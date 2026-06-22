// Package scheduler dispara os motores de destilação automaticamente sobre
// sessões encerradas, respeitando a config de cada motor. Preserva a invariante
// "nada roda sozinho por default": um motor só executa se __enabled=true na sua
// config (global ou override por projeto). Cada (motor, sessão) roda no máximo
// uma vez (marca d'água em engine_runs), evitando re-emissão de sugestões.
package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/engine"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type Scheduler struct {
	reg *engine.Registry
	st  *store.Store
}

func New(reg *engine.Registry, st *store.Store) *Scheduler { return &Scheduler{reg: reg, st: st} }

// Start roda um tick imediato e depois a cada interval, até ctx cancelar.
func (s *Scheduler) Start(ctx context.Context, interval time.Duration) {
	s.Tick(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Tick(ctx)
		}
	}
}

// Tick processa, para cada motor habilitado, as sessões encerradas que ele ainda
// não processou (uma vez cada).
func (s *Scheduler) Tick(ctx context.Context) {
	for _, spec := range s.reg.List() {
		sessions, err := s.st.UnrunEndedSessions(spec.ID)
		if err != nil {
			log.Printf("scheduler: %v", err)
			continue
		}
		for _, sess := range sessions {
			cfg, err := s.st.ResolveEngineConfig(spec.ID, sess.ProjectID, s.reg.Defaults(spec.ID))
			if err != nil {
				continue
			}
			if cfg["__enabled"] != "true" {
				continue // desabilitado p/ este projeto: não roda nem marca (roda se habilitar depois)
			}
			if err := s.reg.Run(ctx, s.st, spec.ID, sess.ProjectID, sess.ID); err != nil {
				log.Printf("scheduler: motor %s sessão %s: %v", spec.ID, sess.ID, err)
				continue // erro: não marca, retenta no próximo tick
			}
			_ = s.st.MarkEngineRun(spec.ID, sess.ID)
		}
	}
}
