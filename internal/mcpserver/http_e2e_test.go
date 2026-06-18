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

	// ListTools must include list_projects and get_session_summary (report tools removed).
	toolsRes, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal("ListTools:", err)
	}
	names := make(map[string]bool, len(toolsRes.Tools))
	for _, tl := range toolsRes.Tools {
		names[tl.Name] = true
	}
	for _, want := range []string{"list_projects", "get_session_summary"} {
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
	_ = sess
}

// TestHTTPE2E_WithoutToken verifies external (tokenless) MCP client behavior:
// read-only tools (list_projects) work without a token.
func TestHTTPE2E_WithoutToken(t *testing.T) {
	ts, st, _ := buildServer(t)

	// Seed a project.
	proj, err := st.CreateProject("External Project", "projeto externo")
	if err != nil {
		t.Fatal("CreateProject:", err)
	}

	endpoint := ts.URL + "/mcp"
	cs := connectHTTP(t, endpoint)

	// list_projects must return seeded project even without a token.
	out, isErr := callTextHTTP(t, cs, "list_projects", map[string]any{})
	if isErr {
		t.Fatalf("list_projects returned error: %s", out)
	}
	if !strings.Contains(out, "External Project") {
		t.Fatalf("list_projects: expected project name in output, got: %s", out)
	}
	_ = proj
}
