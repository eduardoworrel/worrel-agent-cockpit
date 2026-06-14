package store

import (
	"database/sql"
	"testing"
)

func TestSuggestionLifecycle(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sg, err := s.CreateSuggestion(&Suggestion{
		ProjectID: p.ID, Type: "create_skill", Title: "Deploy",
		Payload: `{"name":"Deploy","content":"# x"}`, Evidence: "trecho",
	})
	if err != nil {
		t.Fatal(err)
	}
	pend, _ := s.ListSuggestions("", "pending")
	if len(pend) != 1 {
		t.Fatalf("pendentes = %d", len(pend))
	}
	if err := s.ResolveSuggestion(sg.ID, "accepted"); err != nil {
		t.Fatal(err)
	}
	pend, _ = s.ListSuggestions("", "pending")
	if len(pend) != 0 {
		t.Fatalf("pendentes pós-aceite = %d", len(pend))
	}
	got, err := s.GetSuggestion(sg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "accepted" || got.ResolvedAt == nil {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveSuggestionBogusID(t *testing.T) {
	s := newTestStore(t)
	err := s.ResolveSuggestion("nonexistent", "accepted")
	if err != sql.ErrNoRows {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
}

func TestResolveSuggestionInvalidStatus(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sg, _ := s.CreateSuggestion(&Suggestion{
		ProjectID: p.ID, Type: "create_skill", Title: "Deploy",
		Payload: `{"name":"Deploy","content":"# x"}`,
	})
	err := s.ResolveSuggestion(sg.ID, "typo")
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
	// Verify status wasn't changed
	got, err := s.GetSuggestion(sg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "pending" {
		t.Fatalf("status = %q, want pending", got.Status)
	}
}

func TestUpdateSuggestionPayloadRoundTrip(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sg, _ := s.CreateSuggestion(&Suggestion{
		ProjectID: p.ID, Type: "create_skill", Title: "Deploy",
		Payload: `{"name":"Deploy","content":"# x"}`,
	})
	err := s.UpdateSuggestionPayload(sg.ID, "New Title", `{"name":"UpdatedDeploy","content":"# y"}`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSuggestion(sg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "New Title" || got.Payload != `{"name":"UpdatedDeploy","content":"# y"}` {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveSuggestionRejectPath(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sg, _ := s.CreateSuggestion(&Suggestion{
		ProjectID: p.ID, Type: "create_skill", Title: "Deploy",
		Payload: `{"name":"Deploy","content":"# x"}`,
	})
	if err := s.ResolveSuggestion(sg.ID, "rejected"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSuggestion(sg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "rejected" || got.ResolvedAt == nil {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveSuggestionDeferPath(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sg, _ := s.CreateSuggestion(&Suggestion{
		ProjectID: p.ID, Type: "create_skill", Title: "Deploy",
		Payload: `{"name":"Deploy","content":"# x"}`,
	})
	if err := s.ResolveSuggestion(sg.ID, "deferred"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSuggestion(sg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "deferred" || got.ResolvedAt == nil {
		t.Fatalf("got %+v", got)
	}
}
