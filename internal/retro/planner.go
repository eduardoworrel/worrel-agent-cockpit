package retro

import (
	"encoding/json"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// Scope descreve o recorte escolhido pelo usuário (estágio 1).
type Scope struct {
	CLIs       []string `json:"clis"`
	Dirs       []string `json:"dirs"`
	WindowDays int      `json:"window_days"`
	// Adapter sobrescreve o provider headless para ESTA run (vazio = setting global).
	Adapter string `json:"adapter"`
	// Model sobrescreve o modelo do CLI para ESTA run (vazio = default do CLI).
	Model string `json:"model"`
}

// Planner materializa o escopo: importa as sessões (idempotente por external_ref
// via distill.Importer), abre a run e popula o cursor retro_run_sessions.
type Planner struct {
	store    *store.Store
	importer *distill.Importer
	obs      map[string]Observer
}

func NewPlanner(s *store.Store, imp *distill.Importer, obs []Observer) *Planner {
	m := map[string]Observer{}
	for _, o := range obs {
		m[o.ID()] = o
	}
	return &Planner{store: s, importer: imp, obs: m}
}

func (p *Planner) since(scope Scope) time.Time {
	if scope.WindowDays <= 0 {
		return time.Time{}
	}
	return time.Now().AddDate(0, 0, -scope.WindowDays)
}

// Plan importa as sessões no escopo, cria a run (scoped) e popula o cursor.
// Reaplicar com o mesmo escopo é idempotente: importação pula external_ref
// conhecido e AddRunSession é INSERT OR IGNORE.
func (p *Planner) Plan(scope Scope) (*store.RetroRun, error) {
	clis := scope.CLIs
	if len(clis) == 0 {
		for id := range p.obs {
			clis = append(clis, id)
		}
	}
	since := p.since(scope)

	// 1) importação idempotente (traz transcripts; pula external_ref já conhecido)
	for _, cli := range clis {
		o := p.obs[cli]
		if o == nil {
			continue
		}
		if _, err := p.importer.Import(o); err != nil {
			return nil, err
		}
	}

	// 2) abre a run
	scopeJSON, _ := json.Marshal(scope)
	run, err := p.store.CreateRetroRun(&store.RetroRun{Status: "scoped", Scope: string(scopeJSON)})
	if err != nil {
		return nil, err
	}

	// 3) seleciona as sessões em escopo (janela + dirs) e popula o cursor
	dirSet := map[string]bool{}
	for _, d := range scope.Dirs {
		dirSet[d] = true
	}
	for _, cli := range clis {
		o := p.obs[cli]
		if o == nil {
			continue
		}
		ext, err := o.DiscoverSessions(since)
		if err != nil {
			return nil, err
		}
		for _, es := range ext {
			if len(dirSet) > 0 && !dirSet[es.Dir] {
				continue
			}
			sid, err := p.store.SessionIDByExternalRef(es.ExternalRef)
			if err != nil {
				return nil, err
			}
			if sid == "" {
				continue
			}
			if err := p.store.AddRunSession(run.ID, sid, ""); err != nil {
				return nil, err
			}
		}
	}
	return run, nil
}
