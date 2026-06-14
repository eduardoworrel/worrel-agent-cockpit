package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func setup(t *testing.T) (*Service, *store.Store, *bus.Bus) {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	b := bus.New()
	return New(s, b), s, b
}

// connect cria client+server ligados por InMemoryTransports.
// attribution: token de sessão ("" = externo).
func connect(t *testing.T, svc *Service, token string) *mcp.ClientSession {
	t.Helper()
	srv := svc.ServerFor(token)
	ct, st := mcp.NewInMemoryTransports()
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.Run(runCtx, st) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func callText(t *testing.T, cs *mcp.ClientSession, tool string, args any) string {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func TestListAndGetProjects(t *testing.T) {
	svc, s, _ := setup(t)
	p, _ := s.CreateProject("Meu App", "descrição x")
	cs := connect(t, svc, "")

	out := callText(t, cs, "list_projects", map[string]any{})
	if !strings.Contains(out, "Meu App") || !strings.Contains(out, p.ID) {
		t.Fatalf("list_projects: %s", out)
	}
	out = callText(t, cs, "get_project", map[string]any{"project_id": p.ID})
	if !strings.Contains(out, "descrição x") {
		t.Fatalf("get_project: %s", out)
	}
}

func TestMemoryAndSkills(t *testing.T) {
	svc, s, _ := setup(t)
	p, _ := s.CreateProject("App", "")
	s.SaveMemory(p.ID, "# Convenções\n- use tabs", "init")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# Objetivo\nfazer deploy\n## Passos\n1. build")

	cs := connect(t, svc, "")
	if out := callText(t, cs, "get_memory", map[string]any{"project_id": p.ID}); !strings.Contains(out, "use tabs") {
		t.Fatalf("get_memory: %s", out)
	}
	if out := callText(t, cs, "list_skills", map[string]any{"project_id": p.ID}); !strings.Contains(out, sk.ID) {
		t.Fatalf("list_skills: %s", out)
	}
	if out := callText(t, cs, "load_skill", map[string]any{"skill_id": sk.ID}); !strings.Contains(out, "fazer deploy") {
		t.Fatalf("load_skill: %s", out)
	}
}

func TestToolsListExposesAll(t *testing.T) {
	svc, _, _ := setup(t)
	cs := connect(t, svc, "")
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tl := range res.Tools {
		names[tl.Name] = true
	}
	for _, want := range []string{"list_projects", "get_project", "get_memory", "list_skills",
		"get_skill", "load_skill",
		"report_task_completed", "report_correction", "propose_skill",
		"propose_skill_update", "append_memory_suggestion",
		"get_session_summary"} {
		if !names[want] {
			b, _ := json.Marshal(names)
			t.Fatalf("tool %s ausente em %s", want, b)
		}
	}
}
