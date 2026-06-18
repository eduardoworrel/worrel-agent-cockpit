// Package engine é o framework de motores de destilação: um registry de
// componentes declarativos, cada um com toggle, gatilho, prompts e config
// editáveis. Nada roda sozinho; a execução é sempre disparada explicitamente.
package engine

import (
	"context"
	"fmt"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type Trigger string

const (
	TriggerOnDemand         Trigger = "on_demand"
	TriggerRealtime         Trigger = "realtime"
	TriggerPeriodic         Trigger = "periodic"
	TriggerProjectOpenClose Trigger = "project_open_close"
)

// ConfigField descreve um campo editável (config ou prompt) de um motor.
type ConfigField struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Type    string `json:"type"` // "text" | "number" | "textarea"
	Default string `json:"default"`
}

// Spec é a declaração de um motor. A UI (config/onboarding) é derivada disto.
type Spec struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Triggers    []Trigger     `json:"triggers"`
	Prompts     []ConfigField `json:"prompts"`
	Config      []ConfigField `json:"config"`
	OutputType  string        `json:"output_type"`
	DefaultOn   bool          `json:"default_on"`
}

// RunContext é o estado resolvido entregue a um motor na execução.
type RunContext struct {
	ProjectID string
	SessionID string
	Config    map[string]string // override ⊕ global ⊕ default (inclui __enabled, __trigger)
	Store     *store.Store
}

// Engine é um motor de destilação registrável.
type Engine interface {
	Spec() Spec
	Run(ctx context.Context, rc RunContext) error
}

// Registry guarda os motores registrados, em ordem de registro.
type Registry struct {
	engines map[string]Engine
	order   []string
}

func NewRegistry() *Registry {
	return &Registry{engines: map[string]Engine{}}
}

func (r *Registry) Register(e Engine) {
	id := e.Spec().ID
	if _, exists := r.engines[id]; !exists {
		r.order = append(r.order, id)
	}
	r.engines[id] = e
}

func (r *Registry) List() []Spec {
	out := make([]Spec, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.engines[id].Spec())
	}
	return out
}

func (r *Registry) Get(id string) (Engine, bool) {
	e, ok := r.engines[id]
	return e, ok
}

// Run resolve a config (override ⊕ global ⊕ default) e executa o motor sob
// demanda. Não checa __enabled aqui: disparo sob demanda é uma ação explícita
// do usuário e roda mesmo com o toggle desligado. Schedulers automáticos (que
// respeitam __enabled/__trigger) chegam num sub-projeto futuro.
func (r *Registry) Run(ctx context.Context, st *store.Store, engineID, projectID, sessionID string) error {
	e, ok := r.Get(engineID)
	if !ok {
		return fmt.Errorf("motor desconhecido: %s", engineID)
	}
	cfg, err := st.ResolveEngineConfig(engineID, projectID, r.Defaults(engineID))
	if err != nil {
		return err
	}
	return e.Run(ctx, RunContext{ProjectID: projectID, SessionID: sessionID, Config: cfg, Store: st})
}

// Defaults devolve o mapa de config-default de um motor, incluindo as chaves
// reservadas __enabled (de DefaultOn) e __trigger (primeiro gatilho suportado).
func (r *Registry) Defaults(id string) map[string]string {
	e, ok := r.engines[id]
	if !ok {
		return nil
	}
	sp := e.Spec()
	def := map[string]string{}
	for _, f := range sp.Prompts {
		def[f.Key] = f.Default
	}
	for _, f := range sp.Config {
		def[f.Key] = f.Default
	}
	def["__enabled"] = "false"
	if sp.DefaultOn {
		def["__enabled"] = "true"
	}
	def["__trigger"] = string(TriggerOnDemand)
	if len(sp.Triggers) > 0 {
		def["__trigger"] = string(sp.Triggers[0])
	}
	return def
}
