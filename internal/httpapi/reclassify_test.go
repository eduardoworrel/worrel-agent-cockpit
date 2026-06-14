package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestReclassifySuggestion(t *testing.T) {
	ts, s := newTestServer(t)
	p, _ := s.CreateProject("App", "")
	sg, _ := s.CreateSuggestion(&store.Suggestion{
		ProjectID: p.ID, Type: "update_skill", Title: "Fix",
		Payload: `{"name":"Deploy","content":"# v2"}`,
	})

	body, _ := json.Marshal(map[string]string{"type": "skill.correction"})
	req, _ := http.NewRequest("PUT", ts.URL+"/api/suggestions/"+sg.ID+"/type",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out store.Suggestion
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Type != "skill.correction" {
		t.Fatalf("type = %q", out.Type)
	}
}

func TestReclassifyAlreadyResolvedReturns404(t *testing.T) {
	ts, s := newTestServer(t)
	p, _ := s.CreateProject("App", "")
	sg, _ := s.CreateSuggestion(&store.Suggestion{
		ProjectID: p.ID, Type: "update_skill", Title: "Fix",
		Payload: `{"name":"Deploy","content":"# v2"}`,
	})
	_ = s.ResolveSuggestion(sg.ID, "rejected")

	body, _ := json.Marshal(map[string]string{"type": "skill.correction"})
	req, _ := http.NewRequest("PUT", ts.URL+"/api/suggestions/"+sg.ID+"/type",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := ts.Client().Do(req)
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
