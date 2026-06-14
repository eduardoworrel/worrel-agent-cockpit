package adapter

import "sort"

// Registry mantém um mapa de adaptadores registrados.
type Registry struct {
	byID map[string]Adapter
}

// NewRegistry cria um Registry vazio.
func NewRegistry() *Registry { return &Registry{byID: map[string]Adapter{}} }

// Register adiciona um adaptador ao registry.
func (r *Registry) Register(a Adapter) { r.byID[a.ID()] = a }

// Get devolve o adaptador pelo ID, ou false se não encontrado.
func (r *Registry) Get(id string) (Adapter, bool) {
	a, ok := r.byID[id]
	return a, ok
}

// DetectedAdapter é a visão serializável para a API /api/adapters.
type DetectedAdapter struct {
	ID        string    `json:"id"`
	Installed Installed `json:"installed"`
	Caps      Caps      `json:"caps"`
}

// Detected roda Detect() em todos os adaptadores, ordenado por ID.
func (r *Registry) Detected() []DetectedAdapter {
	out := []DetectedAdapter{}
	for _, a := range r.byID {
		inst, err := a.Detect()
		if err != nil {
			inst = Installed{}
		}
		out = append(out, DetectedAdapter{ID: a.ID(), Installed: inst, Caps: a.Capabilities()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
