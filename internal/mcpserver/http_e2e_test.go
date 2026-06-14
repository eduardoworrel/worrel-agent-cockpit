package mcpserver_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/httpapi"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mcpserver"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// buildServer spins up a full httptest.Server wired with store, bus, mirror,
// apply and mcpserver — exactly as main.go does in production.
func buildServer(t *testing.T) (ts *httptest.Server, st *store.Store, b *bus.Bus) {
	t.Helper()

	var err error
	st, err = store.Open(t.TempDir() + "/e2e.db")
	if err != nil {
		t.Fatal("store.Open:", err)
	}
	t.Cleanup(func() { st.Close() })

	b = bus.New()
	mir := mirror.New(t.TempDir())
	applier := apply.New(st, mir, b)
	svc := mcpserver.New(st, b)

	handler := httpapi.New(httpapi.Deps{
		Store:   st,
		Mirror:  mir,
		Bus:     b,
		Applier: applier,
		MCP:     svc.HTTPHandler(),
	}).Handler()

	ts = httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts, st, b
}

// connectHTTP connects a real MCP client to the streamable HTTP endpoint.
func connectHTTP(t *testing.T, endpoint string) *mcp.ClientSession {
	t.Helper()
	transport := &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		DisableStandaloneSSE: true, // avoid persistent GET stream in tests
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "e2e-test", Version: "0"}, nil)
	cs, err := client.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatalf("mcp client connect to %s: %v", endpoint, err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// callTextHTTP calls a tool and returns concatenated text content.
func callTextHTTP(t *testing.T, cs *mcp.ClientSession, tool string, args any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: %v", tool, err)
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String(), res.IsError
}

// ptr is a convenience helper for *string literals.
func ptrStr(s string) *string { return &s }

// TestHTTPE2E_WithToken verifies end-to-end MCP over HTTP streamable transport
// with an authenticated session token.
func TestHTTPE2E_WithToken(t *testing.T) {
	ts, st, _ := buildServer(t)

	// Seed: project, skill, session with MCP token.
	proj, err := st.CreateProject("E2E Project", "projeto para teste e2e")
	if err != nil {
		t.Fatal("CreateProject:", err)
	}
	_, err = st.CreateSkill(proj.ID, "E2E Skill", "# Objetivo\nfazer o deploy\n## Passos\n1. build\n2. push")
	if err != nil {
		t.Fatal("CreateSkill:", err)
	}
	sess, err := st.CreateSession(&store.Session{
		ProjectID: proj.ID,
		Adapter:   "claude-code",
		Mode:      "wrapper",
		MCPToken:  ptrStr("tok-e2e"),
	})
	if err != nil {
		t.Fatal("CreateSession:", err)
	}

	endpoint := ts.URL + "/mcp?s=tok-e2e"
	cs := connectHTTP(t, endpoint)

	// ListTools must include list_projects and report_task_completed.
	toolsRes, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal("ListTools:", err)
	}
	names := make(map[string]bool, len(toolsRes.Tools))
	for _, tl := range toolsRes.Tools {
		names[tl.Name] = true
	}
	for _, want := range []string{"list_projects", "report_task_completed"} {
		if !names[want] {
			t.Errorf("tool %q missing from ListTools; got: %v", want, toolsRes.Tools)
		}
	}

	// CallTool list_projects must return the seeded project name.
	out, isErr := callTextHTTP(t, cs, "list_projects", map[string]any{})
	if isErr {
		t.Fatalf("list_projects returned error: %s", out)
	}
	if !strings.Contains(out, "E2E Project") {
		t.Fatalf("list_projects: expected %q in output, got: %s", "E2E Project", out)
	}

	// CallTool report_task_completed must create a pending suggestion with the
	// correct SessionID.
	out, isErr = callTextHTTP(t, cs, "report_task_completed", map[string]any{
		"summary":  "tarefa e2e concluída",
		"evidence": "arquivo e2e_test.go criado",
	})
	if isErr {
		t.Fatalf("report_task_completed returned error: %s", out)
	}
	if !strings.Contains(strings.ToLower(out), "sugestão") {
		t.Fatalf("report_task_completed: expected confirmation, got: %s", out)
	}

	// Verify suggestion was created in the store with the session ID.
	pend, err := st.ListSuggestions(proj.ID, "pending")
	if err != nil {
		t.Fatal("ListSuggestions:", err)
	}
	if len(pend) != 1 {
		t.Fatalf("expected 1 pending suggestion, got %d", len(pend))
	}
	sg := pend[0]
	if sg.Type != "add_memory" {
		t.Errorf("expected suggestion type add_memory, got %q", sg.Type)
	}
	if sg.SessionID == nil || *sg.SessionID != sess.ID {
		t.Errorf("expected SessionID=%q, got %v", sess.ID, sg.SessionID)
	}
}

// TestHTTPE2E_WithoutToken verifies external (tokenless) MCP client behavior.
func TestHTTPE2E_WithoutToken(t *testing.T) {
	ts, st, _ := buildServer(t)

	// Seed a project for explicit project_id usage.
	proj, err := st.CreateProject("External Project", "projeto externo")
	if err != nil {
		t.Fatal("CreateProject:", err)
	}

	endpoint := ts.URL + "/mcp"
	cs := connectHTTP(t, endpoint)

	// report_correction without project_id must return an IsError result
	// mentioning project_id.
	out, isErr := callTextHTTP(t, cs, "report_correction", map[string]any{
		"what_failed": "npm test",
		"what_worked": "npm test -- --runInBand",
	})
	if !isErr {
		t.Fatalf("expected IsError=true when project_id missing, got output: %s", out)
	}
	if !strings.Contains(out, "project_id") {
		t.Fatalf("expected error mentioning project_id, got: %s", out)
	}

	// report_correction WITH explicit project_id must succeed and create a
	// suggestion with nil SessionID (external agent, no token).
	out, isErr = callTextHTTP(t, cs, "report_correction", map[string]any{
		"project_id":  proj.ID,
		"what_failed": "npm test",
		"what_worked": "npm test -- --runInBand",
	})
	if isErr {
		t.Fatalf("report_correction with project_id returned error: %s", out)
	}

	pend, err := st.ListSuggestions(proj.ID, "pending")
	if err != nil {
		t.Fatal("ListSuggestions:", err)
	}
	if len(pend) != 1 {
		t.Fatalf("expected 1 pending suggestion, got %d", len(pend))
	}
	sg := pend[0]
	if sg.SessionID != nil {
		t.Errorf("expected nil SessionID for external agent, got %v", *sg.SessionID)
	}
	if sg.Type != "add_correction" {
		t.Errorf("expected suggestion type add_correction, got %q", sg.Type)
	}
}
