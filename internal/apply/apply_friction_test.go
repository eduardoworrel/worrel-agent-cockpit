package apply

import (
	"path/filepath"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func newApF(t *testing.T) (*Applier, *store.Store) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	return New(st, mirror.New(t.TempDir()), bus.New()), st
}

func TestAcceptAgentCorrection(t *testing.T) {
	a, st := newApF(t)
	p, _ := st.CreateProject("App", "")
	ag, _ := st.CreateAgent(p.ID, "Revisor", "persona v1", "")
	pl := `{"target_agent_id":"` + ag.ID + `","persona":"persona v2","change_summary":"tom","evidence":"sess2"}`
	sg, _ := st.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "agent.correction", Title: "refino", Payload: pl})
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetAgent(ag.ID)
	if got.Persona != "persona v2" || got.ActiveGeneration != 2 {
		t.Fatalf("persona=%q gen=%d", got.Persona, got.ActiveGeneration)
	}
}

func TestAcceptSkillHealth(t *testing.T) {
	a, st := newApF(t)
	p, _ := st.CreateProject("App", "")
	sk, _ := st.CreateSkill(p.ID, "Deploy", "## passos")
	// eleva a política para auto_total para que rebaixar p/ manual seja observável
	if err := st.SetSkillPolicy(sk.ID, "auto_total"); err != nil {
		t.Fatal(err)
	}
	pl := `{"skill_id":"` + sk.ID + `","action":"suspend","evidence":"3 falhas"}`
	sg, _ := st.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "skill.health", Title: "saúde", Payload: pl})
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetSkill(sk.ID)
	if got.EvolutionPolicy != "manual" {
		t.Fatalf("policy=%q (esperava manual após health)", got.EvolutionPolicy)
	}
}
