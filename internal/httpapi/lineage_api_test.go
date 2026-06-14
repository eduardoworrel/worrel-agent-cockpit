package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestGetGenerations(t *testing.T) {
	ts, s := newTestServer(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")

	var gens []store.SkillGeneration
	code := getJSON(t, ts, "/api/skills/"+sk.ID+"/generations", &gens)
	if code != 200 {
		t.Fatalf("status = %d", code)
	}
	if len(gens) != 1 {
		t.Fatalf("gens = %d", len(gens))
	}
}

func TestRevertGeneration(t *testing.T) {
	ts, s := newTestServer(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")
	_, _ = s.AddGeneration(sk.ID, store.GenerationInput{
		EvolutionType: "correction", Snapshot: "# v2", Authorship: "human",
	})

	code := postJSON(t, ts, "/api/skills/"+sk.ID+"/revert",
		map[string]int{"generation": 1}, nil)
	if code != 200 {
		t.Fatalf("revert status = %d", code)
	}
	got, _ := s.GetSkill(sk.ID)
	if got.Content != "# v1" || got.ActiveGeneration != 1 {
		t.Fatalf("revert falhou: %+v", got)
	}
}

func TestSkillStats(t *testing.T) {
	ts, s := newTestServer(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")
	uid, _ := s.RecordSkillUsageStart(sk.ID, nil, 1)
	_ = s.CloseSkillUsage(uid, "success", 0, false, 100)

	var stats store.SkillStats
	code := getJSON(t, ts, "/api/skills/"+sk.ID+"/stats", &stats)
	if code != 200 {
		t.Fatalf("status = %d", code)
	}
	if stats.TotalUses != 1 {
		t.Fatalf("total_uses = %d", stats.TotalUses)
	}
}

func TestSetSkillPolicyHTTP(t *testing.T) {
	ts, s := newTestServer(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")

	code := putJSONLineage(t, ts, "/api/skills/"+sk.ID+"/policy",
		map[string]string{"policy": "auto_correction"}, nil)
	if code != 200 {
		t.Fatalf("status = %d", code)
	}
	got, _ := s.GetSkill(sk.ID)
	if got.EvolutionPolicy != "auto_correction" {
		t.Fatalf("policy = %q", got.EvolutionPolicy)
	}
}

func putJSONLineage(t *testing.T, ts *httptest.Server, path string, body any, out any) int {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", ts.URL+path, bytes.NewReader(b))
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
