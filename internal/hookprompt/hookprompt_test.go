package hookprompt

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunReturnsAllowDecision(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/sessions/sess-1/permission-request") {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["tool"] != "Bash" {
			t.Errorf("tool = %v", body["tool"])
		}
		w.Write([]byte(`{"decision":"allow"}`))
	}))
	defer srv.Close()

	in := strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"ls"}}`)
	var out strings.Builder
	if err := Run(in, &out, srv.URL, "sess-1", "claude"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"permissionDecision":"allow"`) {
		t.Fatalf("out = %s", out.String())
	}
}

// Codex compartilha o schema do Claude: permissionDecision allow/deny/ask.
func TestRunCodexUsesPermissionDecision(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"decision":"deny"}`))
	}))
	defer srv.Close()

	in := strings.NewReader(`{"tool_name":"shell","tool_input":{}}`)
	var out strings.Builder
	if err := Run(in, &out, srv.URL, "sess-1", "codex"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"permissionDecision":"deny"`) {
		t.Fatalf("out = %s", out.String())
	}
}

func TestRunFallsBackToAskWhenServerDown(t *testing.T) {
	in := strings.NewReader(`{"tool_name":"Bash","tool_input":{}}`)
	var out strings.Builder
	if err := Run(in, &out, "http://127.0.0.1:1", "sess-1", "claude"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"permissionDecision":"ask"`) {
		t.Fatalf("out = %s", out.String())
	}
}

func TestRunBadStdinFallsBackToAsk(t *testing.T) {
	in := strings.NewReader(`not json`)
	var out strings.Builder
	if err := Run(in, &out, "http://127.0.0.1:1", "sess-1", "claude"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"permissionDecision":"ask"`) {
		t.Fatalf("out = %s", out.String())
	}
}
