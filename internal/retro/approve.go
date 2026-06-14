package retro

import (
	"encoding/json"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// Approver materializa as decisões do mapa de projetos (critérios 5 e 6):
// aprovar cria/associa o projeto e propaga project_id às sessões no cursor;
// fundir une clusters num único projeto; descartar deixa as sessões sem projeto.
type Approver struct {
	store *store.Store
}

func NewApprover(s *store.Store) *Approver { return &Approver{store: s} }

func sessionIDsOf(c *store.RetroCluster) []string {
	var ids []string
	_ = json.Unmarshal([]byte(c.SessionIDs), &ids)
	return ids
}

func dirsOf(c *store.RetroCluster) []string {
	var d []string
	_ = json.Unmarshal([]byte(c.Dirs), &d)
	return d
}

// resolveProject devolve o projeto a usar: o existente associado, senão cria um novo.
func (a *Approver) resolveProject(existingProjectID, name, fallbackName string, dirs []string) (string, error) {
	if existingProjectID != "" {
		return existingProjectID, nil
	}
	// Idempotência (critério 8): se alguma pasta já pertence a um projeto, reusa-o
	// em vez de criar um novo.
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if p, err := a.store.ProjectByDir(d); err == nil && p != nil {
			return p.ID, nil
		}
	}
	n := name
	if n == "" {
		n = fallbackName
	}
	p, err := a.store.CreateProject(n, "")
	if err != nil {
		return "", err
	}
	for _, d := range dirs {
		if d != "" {
			_ = a.store.AddProjectDir(p.ID, d)
		}
	}
	return p.ID, nil
}

func (a *Approver) propagate(runID, projectID string, sessionIDs []string) error {
	for _, sid := range sessionIDs {
		if err := a.store.SetRunSessionProject(runID, sid, projectID); err != nil {
			return err
		}
	}
	return nil
}

// ApproveCluster cria/associa o projeto e propaga aos session ids do cluster.
func (a *Approver) ApproveCluster(clusterID, rename string) (string, error) {
	c, err := a.store.GetRetroCluster(clusterID)
	if err != nil {
		return "", err
	}
	existing := ""
	if c.ExistingProjectID != nil {
		existing = *c.ExistingProjectID
	}
	pid, err := a.resolveProject(existing, rename, c.Name, dirsOf(c))
	if err != nil {
		return "", err
	}
	if err := a.store.SetClusterDecision(c.ID, "approved", pid); err != nil {
		return "", err
	}
	if err := a.propagate(c.RunID, pid, sessionIDsOf(c)); err != nil {
		return "", err
	}
	return pid, nil
}

// MergeClusters funde vários clusters num único projeto com todas as sessões.
func (a *Approver) MergeClusters(runID string, clusterIDs []string, name, existingProjectID string) (string, error) {
	var all []string
	var dirs []string
	clusters := make([]*store.RetroCluster, 0, len(clusterIDs))
	for _, id := range clusterIDs {
		c, err := a.store.GetRetroCluster(id)
		if err != nil {
			return "", err
		}
		clusters = append(clusters, c)
		all = append(all, sessionIDsOf(c)...)
		dirs = append(dirs, dirsOf(c)...)
	}
	fallback := name
	if fallback == "" && len(clusters) > 0 {
		fallback = clusters[0].Name
	}
	pid, err := a.resolveProject(existingProjectID, name, fallback, dirs)
	if err != nil {
		return "", err
	}
	for _, c := range clusters {
		if err := a.store.SetClusterDecision(c.ID, "approved", pid); err != nil {
			return "", err
		}
	}
	if err := a.propagate(runID, pid, all); err != nil {
		return "", err
	}
	return pid, nil
}

// DiscardCluster marca o cluster como descartado; suas sessões ficam sem project_id
// e portanto não entram na destilação.
func (a *Approver) DiscardCluster(clusterID string) error {
	return a.store.SetClusterDecision(clusterID, "discarded", "")
}
