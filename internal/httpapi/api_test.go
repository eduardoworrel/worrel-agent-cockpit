package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	m := mirror.New(t.TempDir())
	srv := New(Deps{Store: s, Mirror: m, Bus: bus.New(), Applier: apply.New(s, m, bus.New())})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, s
}

func postJSON(t *testing.T, ts *httptest.Server, path string, body any, out any) int {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := ts.Client().Post(ts.URL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func getJSON(t *testing.T, ts *httptest.Server, path string, out any) int {
	t.Helper()
	resp, err := ts.Client().Get(ts.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func putJSON(t *testing.T, ts *httptest.Server, path string, body any, out any) int {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

// Case 1 & 2: POST /api/projects and GET /api/projects
func TestCreateAndListProjects(t *testing.T) {
	ts, _ := newTestServer(t)

	// Case 1: POST creates project with slug
	var proj map[string]any
	code := postJSON(t, ts, "/api/projects", map[string]any{"name": "App", "description": "d"}, &proj)
	if code != 201 {
		t.Fatalf("POST /api/projects = %d, want 201", code)
	}
	slug, _ := proj["slug"].(string)
	if slug == "" {
		t.Fatalf("slug is empty, got %+v", proj)
	}

	// Case 2: GET /api/projects returns list with 1 item
	var list []any
	code = getJSON(t, ts, "/api/projects", &list)
	if code != 200 {
		t.Fatalf("GET /api/projects = %d, want 200", code)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}
}

// Case 3: memory endpoints
func TestMemoryEndpoints(t *testing.T) {
	ts, _ := newTestServer(t)

	var proj map[string]any
	postJSON(t, ts, "/api/projects", map[string]any{"name": "App", "description": "d"}, &proj)
	id, _ := proj["id"].(string)

	// PUT memory
	code := putJSON(t, ts, "/api/projects/"+id+"/memory", map[string]any{"content": "# m", "note": "manual"}, nil)
	if code != 200 {
		t.Fatalf("PUT memory = %d, want 200", code)
	}

	// GET memory returns content
	var mem map[string]any
	getJSON(t, ts, "/api/projects/"+id+"/memory", &mem)
	if mem["content"] != "# m" {
		t.Fatalf("memory content = %q, want '# m'", mem["content"])
	}

	// GET memory/versions returns 1 version
	var versions []any
	getJSON(t, ts, "/api/projects/"+id+"/memory/versions", &versions)
	if len(versions) != 1 {
		t.Fatalf("memory versions len = %d, want 1", len(versions))
	}
}

// Case 4: skills
func TestSkillEndpoints(t *testing.T) {
	ts, _ := newTestServer(t)

	var proj map[string]any
	postJSON(t, ts, "/api/projects", map[string]any{"name": "App", "description": "d"}, &proj)
	id, _ := proj["id"].(string)

	// POST skill
	code := postJSON(t, ts, "/api/projects/"+id+"/skills", map[string]any{"name": "Deploy", "content": "# s"}, nil)
	if code != 201 {
		t.Fatalf("POST skill = %d, want 201", code)
	}

	// GET /api/skills returns 1
	var skills []any
	getJSON(t, ts, "/api/skills", &skills)
	if len(skills) != 1 {
		t.Fatalf("skills len = %d, want 1", len(skills))
	}
}

// Case 5: suggestions lifecycle
func TestSuggestionsLifecycle(t *testing.T) {
	ts, _ := newTestServer(t)

	var proj map[string]any
	postJSON(t, ts, "/api/projects", map[string]any{"name": "App", "description": "d"}, &proj)
	projID, _ := proj["id"].(string)

	// Create suggestion for accept
	var sg1 map[string]any
	code := postJSON(t, ts, "/api/suggestions", map[string]any{
		"project_id": projID,
		"type":       "create_skill",
		"title":      "Deploy",
		"payload":    `{"name":"Deploy","content":"# steps"}`,
	}, &sg1)
	if code != 201 {
		t.Fatalf("POST suggestion = %d, want 201", code)
	}
	sg1ID, _ := sg1["id"].(string)

	// Create suggestion for reject
	var sg2 map[string]any
	postJSON(t, ts, "/api/suggestions", map[string]any{
		"project_id": projID,
		"type":       "create_skill",
		"title":      "Lint",
		"payload":    `{"name":"Lint","content":"# lint"}`,
	}, &sg2)
	sg2ID, _ := sg2["id"].(string)

	// Create suggestion for defer
	var sg3 map[string]any
	postJSON(t, ts, "/api/suggestions", map[string]any{
		"project_id": projID,
		"type":       "create_skill",
		"title":      "Test",
		"payload":    `{"name":"Test","content":"# test"}`,
	}, &sg3)
	sg3ID, _ := sg3["id"].(string)

	// GET /api/suggestions?status=pending → 3
	var pending []any
	getJSON(t, ts, "/api/suggestions?status=pending", &pending)
	if len(pending) != 3 {
		t.Fatalf("pending len = %d, want 3", len(pending))
	}

	// Accept sg1 → skill created
	code = postJSON(t, ts, "/api/suggestions/"+sg1ID+"/accept", nil, nil)
	if code != 200 {
		t.Fatalf("accept = %d, want 200", code)
	}
	var skills []any
	getJSON(t, ts, "/api/skills", &skills)
	if len(skills) != 1 {
		t.Fatalf("skills after accept = %d, want 1", len(skills))
	}

	// Reject sg2
	var rejRes map[string]any
	code = postJSON(t, ts, "/api/suggestions/"+sg2ID+"/reject", nil, &rejRes)
	if code != 200 {
		t.Fatalf("reject = %d, want 200", code)
	}
	if rejRes["status"] != "rejected" {
		t.Fatalf("reject status = %q, want 'rejected'", rejRes["status"])
	}

	// Defer sg3
	var defRes map[string]any
	code = postJSON(t, ts, "/api/suggestions/"+sg3ID+"/defer", nil, &defRes)
	if code != 200 {
		t.Fatalf("defer = %d, want 200", code)
	}
	if defRes["status"] != "deferred" {
		t.Fatalf("defer status = %q, want 'deferred'", defRes["status"])
	}
}

// Case 6: GET /api/sessions
func TestListSessions(t *testing.T) {
	ts, _ := newTestServer(t)

	var list []any
	code := getJSON(t, ts, "/api/sessions", &list)
	if code != 200 {
		t.Fatalf("GET /api/sessions = %d, want 200", code)
	}
	if len(list) != 0 {
		t.Fatalf("sessions len = %d, want 0", len(list))
	}
}

// Case 7: settings
func TestSettings(t *testing.T) {
	ts, _ := newTestServer(t)

	var settings map[string]string
	code := getJSON(t, ts, "/api/settings", &settings)
	if code != 200 {
		t.Fatalf("GET /api/settings = %d, want 200", code)
	}
	if settings["retention_days"] == "" {
		t.Fatal("retention_days should have a default value")
	}

	// PUT settings
	code = putJSON(t, ts, "/api/settings", map[string]string{"retention_days": "15"}, nil)
	if code != 200 {
		t.Fatalf("PUT /api/settings = %d, want 200", code)
	}

	// Verify persisted
	var updated map[string]string
	getJSON(t, ts, "/api/settings", &updated)
	if updated["retention_days"] != "15" {
		t.Fatalf("retention_days = %q, want '15'", updated["retention_days"])
	}
}

// WS test: connect, publish event via the API, receive it as JSON
func TestWebSocketEvents(t *testing.T) {
	ts, _ := newTestServer(t)

	var proj map[string]any
	postJSON(t, ts, "/api/projects", map[string]any{"name": "WSApp", "description": ""}, &proj)
	projID, _ := proj["id"].(string)

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/events"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal("ws dial:", err)
	}
	defer conn.Close()

	// Publish events via the suggestions endpoint until the WS delivers one.
	// A background publisher avoids racing the handler's bus subscription.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		body := []byte(`{"project_id":"` + projID + `","type":"create_skill","title":"WS Test","payload":"{\"name\":\"WS\",\"content\":\"# ws\"}"}`)
		for i := 0; i < 40; i++ {
			resp, err := http.Post(ts.URL+"/api/suggestions", "application/json", bytes.NewReader(body))
			if err == nil {
				resp.Body.Close()
			}
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
	}()

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var ev map[string]any
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatal("read ws event:", err)
	}
	if ev["type"] != "suggestion.created" {
		t.Fatalf("ws event type = %q, want 'suggestion.created'", ev["type"])
	}
}

// Case: /mcp is mounted when MCP handler is provided
func TestMCPMounted(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	m := mirror.New(t.TempDir())
	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	srv := New(Deps{Store: s, Mirror: m, Bus: bus.New(), Applier: apply.New(s, m, bus.New()), MCP: stub})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	resp, err := ts.Client().Post(ts.URL+"/mcp", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("POST /mcp = 404, want non-404 (MCP not mounted)")
	}
}

// Error mapping: invalid input and bogus ids
func TestErrorResponses(t *testing.T) {
	ts, _ := newTestServer(t)

	var proj map[string]any
	postJSON(t, ts, "/api/projects", map[string]any{"name": "App", "description": ""}, &proj)
	id, _ := proj["id"].(string)

	// PUT project with empty name → 400
	if code := putJSON(t, ts, "/api/projects/"+id, map[string]any{"name": "", "description": "d"}, nil); code != 400 {
		t.Fatalf("PUT project empty name = %d, want 400", code)
	}

	// GET project with bogus id → 404
	if code := getJSON(t, ts, "/api/projects/bogus-id", nil); code != 404 {
		t.Fatalf("GET bogus project = %d, want 404", code)
	}

	// PUT skill with bogus id → 404
	if code := putJSON(t, ts, "/api/skills/bogus-id", map[string]any{"name": "X", "content": "# x"}, nil); code != 404 {
		t.Fatalf("PUT bogus skill = %d, want 404", code)
	}

	// POST reject bogus suggestion id → 404
	if code := postJSON(t, ts, "/api/suggestions/bogus-id/reject", nil, nil); code != 404 {
		t.Fatalf("reject bogus suggestion = %d, want 404", code)
	}

	// POST accept bogus suggestion id → 404
	if code := postJSON(t, ts, "/api/suggestions/bogus-id/accept", nil, nil); code != 404 {
		t.Fatalf("accept bogus suggestion = %d, want 404", code)
	}

	// POST accept an already-accepted suggestion twice → second returns 409
	var projMap map[string]any
	postJSON(t, ts, "/api/projects", map[string]any{"name": "App2", "description": ""}, &projMap)
	projID2, _ := projMap["id"].(string)
	var sgMap map[string]any
	postJSON(t, ts, "/api/suggestions", map[string]any{
		"project_id": projID2,
		"type":       "create_skill",
		"title":      "Deploy",
		"payload":    `{"name":"Deploy","content":"# steps"}`,
	}, &sgMap)
	sgID, _ := sgMap["id"].(string)
	// First accept → 200
	if code := postJSON(t, ts, "/api/suggestions/"+sgID+"/accept", nil, nil); code != 200 {
		t.Fatalf("first accept = %d, want 200", code)
	}
	// Second accept → 409
	if code := postJSON(t, ts, "/api/suggestions/"+sgID+"/accept", nil, nil); code != 409 {
		t.Fatalf("second accept = %d, want 409", code)
	}
}
