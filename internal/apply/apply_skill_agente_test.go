package apply

import (
	"path/filepath"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func newAp(t *testing.T) (*Applier, *store.Store) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	return New(st, mirror.New(t.TempDir()), bus.New()), st
}

const candPayload = `{"title":"Deploy","signature":"sig","skill_draft":{"name":"Deploy","content":"## passos","structured":"{\"steps\":[\"build\"]}"},"agente_draft":{"name":"Deployer","persona":"Você cuida de deploys."}}`

func TestAcceptAsSkill(t *testing.T) {
	a, st := newAp(t)
	p, _ := st.CreateProject("App", "")
	sg, _ := st.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "skill_or_agente_candidate", Title: "Deploy", Payload: candPayload})
	if err := a.AcceptAs(sg.ID, "skill"); err != nil {
		t.Fatal(err)
	}
	sks, _ := st.ListSkills(p.ID)
	if len(sks) != 1 || sks[0].Name != "Deploy" {
		t.Fatalf("skills=%+v", sks)
	}
	got, _ := st.GetSkill(sks[0].ID)
	if got.Structured == "" {
		t.Fatalf("structured não setado")
	}
}

func TestAcceptAsAgente(t *testing.T) {
	a, st := newAp(t)
	p, _ := st.CreateProject("App", "")
	sg, _ := st.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "skill_or_agente_candidate", Title: "Deploy", Payload: candPayload})
	if err := a.AcceptAs(sg.ID, "agente"); err != nil {
		t.Fatal(err)
	}
	ags, _ := st.ListAgents(p.ID)
	if len(ags) != 1 || ags[0].Persona == "" {
		t.Fatalf("agents=%+v", ags)
	}
}
