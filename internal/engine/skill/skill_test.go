package skill_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	eng "github.com/eduardoworrel/worrel-agent-cockpit/internal/engine"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/engine/skill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type fakeLLM struct{ out string }

func (f fakeLLM) RunHeadless(_ context.Context, _ string, _ adapter.HeadlessOpts) (string, error) {
	return f.out, nil
}

func seedSteps(t *testing.T, st *store.Store, sid string) {
	_ = st.AppendTranscriptEvent(sid, "user", "text", "primeiro roda lint, depois build, então deploy", 0, 0)
	_ = st.AppendTranscriptEventRich(sid, "assistant", "tool_use", "Bash lint", `[{"type":"tool_use","name":"Bash"}]`, 0, 0)
	_ = st.AppendTranscriptEventRich(sid, "assistant", "tool_use", "Bash build", `[{"type":"tool_use","name":"Bash"}]`, 0, 0)
	_ = st.AppendTranscriptEventRich(sid, "assistant", "tool_use", "Bash deploy", `[{"type":"tool_use","name":"Bash"}]`, 0, 0)
}

const draftOut = `[{"signature":"sig-deploy","title":"Deploy","skill_draft":{"name":"Deploy","content":"## passos","structured":"{}"},"agente_draft":{"name":"Deployer","persona":"p"}}]`

func TestSkillMaturesOnSecondSession(t *testing.T) {
	st, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	p, _ := st.CreateProject("App", "")
	m := skill.New(fakeLLM{out: draftOut})
	r := eng.NewRegistry()
	r.Register(m)

	s1, _ := st.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	seedSteps(t, st, s1.ID)
	if err := r.Run(context.Background(), st, "skill", p.ID, s1.ID); err != nil {
		t.Fatal(err)
	}
	// 1ª sessão: candidato acumulando, sem sugestão
	if sg, _ := st.ListSuggestions("", ""); len(sg) != 0 {
		t.Fatalf("1ª sessão não deveria criar sugestão, got %d", len(sg))
	}

	s2, _ := st.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	seedSteps(t, st, s2.ID)
	if err := r.Run(context.Background(), st, "skill", p.ID, s2.ID); err != nil {
		t.Fatal(err)
	}
	sg, _ := st.ListSuggestions("", "")
	if len(sg) != 1 || sg[0].Type != "skill_or_agente_candidate" || sg[0].Origin != "engine:skill" {
		t.Fatalf("2ª sessão deveria maturar: %d %+v", len(sg), sg)
	}
}
