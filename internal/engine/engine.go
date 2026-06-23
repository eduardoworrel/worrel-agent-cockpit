// Package engine é o framework de motores de destilação: um registry de
// componentes declarativos, cada um com toggle, gatilho, prompts e config
// editáveis. Nada roda sozinho; a execução é sempre disparada explicitamente.
package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type Trigger string

const (
	TriggerOnDemand         Trigger = "on_demand"
	TriggerRealtime         Trigger = "realtime"
	TriggerPeriodic         Trigger = "periodic"
	TriggerProjectOpenClose Trigger = "project_open_close"
	TriggerAgentSelf        Trigger = "agent_self"
)

// ConfigField descreve um campo editável (config ou prompt) de um motor.
type ConfigField struct {
	Key     string         `json:"key"`
	Label   string         `json:"label"`
	Type    string         `json:"type"` // "text" | "number" | "textarea" | "select"
	Default string         `json:"default"`
	Options []ConfigOption `json:"options,omitempty"` // opções p/ type "select" (renderizadas como cards)
}

// ConfigOption é uma escolha de um campo "select": o valor gravado, um rótulo
// curto e uma descrição do que aquela opção faz (mostrada no card de seleção).
type ConfigOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// DetectionModeOptions é o enum de modo de detecção compartilhado pelos motores.
var DetectionModeOptions = []ConfigOption{
	{Value: "hybrid", Label: "Híbrido", Description: "Heurística rápida + LLM nos casos ambíguos. Equilíbrio entre custo e precisão (recomendado)."},
	{Value: "llm_full", Label: "LLM completo", Description: "Usa o LLM para analisar todo o transcript. Mais preciso, porém mais caro e lento."},
	{Value: "heuristic_only", Label: "Só heurística", Description: "Apenas regras determinísticas, sem LLM. Grátis e rápido, mas mais grosseiro (mais falso-positivo)."},
}

// HarnessOptions: adapters selecionáveis como executor do LLM (modos com LLM).
// Apenas os headless-capazes destilam de fato; os demais caem no default.
var HarnessOptions = []ConfigOption{
	{Value: "", Label: "Padrão", Description: "Usa o harness padrão do worrel."},
	{Value: "claude-code", Label: "Claude Code", Description: "Executa a destilação via Claude Code (headless)."},
	{Value: "opencode", Label: "opencode", Description: "Executa via opencode (headless)."},
	{Value: "antigravity", Label: "Antigravity", Description: "Executa via Antigravity CLI (agy)."},
	{Value: "codex", Label: "Codex", Description: "Executa via Codex CLI (se suportar headless)."},
}

// LLMFields são os campos de harness + modelo, comuns aos motores que usam LLM
// (relevantes só fora do modo heuristic_only).
func LLMFields() []ConfigField {
	return []ConfigField{
		{Key: "harness", Label: "Harness (executor do LLM)", Type: "select", Default: "", Options: HarnessOptions},
		{Key: "model", Label: "Modelo (vazio = default do harness)", Type: "text", Default: ""},
	}
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
	// Snapshot das sugestões antes, p/ registrar no log quais nasceram nesta
	// execução (explicabilidade).
	beforeIDs := map[string]bool{}
	if before, e := st.ListSuggestions(projectID, ""); e == nil {
		for _, s := range before {
			beforeIDs[s.ID] = true
		}
	}
	runErr := e.Run(ctx, RunContext{ProjectID: projectID, SessionID: sessionID, Config: cfg, Store: st})
	var created []string
	if after, e := st.ListSuggestions(projectID, ""); e == nil {
		for _, s := range after {
			if !beforeIDs[s.ID] {
				created = append(created, s.Title)
			}
		}
	}
	detail := strings.Join(created, "; ")
	if runErr != nil {
		detail = "erro: " + runErr.Error()
	}
	_ = st.LogEngineRun(&store.EngineLogEntry{
		EngineID: engineID, ProjectID: projectID, SessionID: sessionID,
		Trigger: cfg["__trigger"], Suggestions: len(created), Detail: detail,
	})
	return runErr
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
