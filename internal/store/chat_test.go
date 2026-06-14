package store

import (
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestChatThreadCRUD(t *testing.T) {
	s := openTestStore(t)

	th, err := s.CreateChatThread(`{"project_id":"p1"}`, "claudecode", "sonnet", "Sobre X")
	if err != nil {
		t.Fatalf("CreateChatThread: %v", err)
	}
	if th.ID == "" || th.Scope != `{"project_id":"p1"}` || th.Provider != "claudecode" {
		t.Fatalf("thread mal preenchido: %+v", th)
	}

	got, err := s.GetChatThread(th.ID)
	if err != nil {
		t.Fatalf("GetChatThread: %v", err)
	}
	if got.Title != "Sobre X" || got.Model != "sonnet" {
		t.Fatalf("GetChatThread mismatch: %+v", got)
	}

	// scope vazio vira "{}"
	th2, err := s.CreateChatThread("", "", "", "")
	if err != nil {
		t.Fatalf("CreateChatThread vazio: %v", err)
	}
	if th2.Scope != "{}" {
		t.Fatalf("scope default esperado {}, veio %q", th2.Scope)
	}

	list, err := s.ListChatThreads()
	if err != nil {
		t.Fatalf("ListChatThreads: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("esperava 2 threads, veio %d", len(list))
	}
}

func TestChatMessageCRUD(t *testing.T) {
	s := openTestStore(t)
	th, err := s.CreateChatThread("", "", "", "")
	if err != nil {
		t.Fatalf("CreateChatThread: %v", err)
	}

	m1, err := s.AppendChatMessage(th.ID, "user", "oi", "")
	if err != nil {
		t.Fatalf("AppendChatMessage: %v", err)
	}
	if m1.Seq != 1 || m1.Sources != "[]" {
		t.Fatalf("primeira msg: seq=%d sources=%q", m1.Seq, m1.Sources)
	}

	m2, err := s.AppendChatMessage(th.ID, "assistant", "olá", `[{"session_id":"s1"}]`)
	if err != nil {
		t.Fatalf("AppendChatMessage 2: %v", err)
	}
	if m2.Seq != 2 {
		t.Fatalf("seq esperado 2, veio %d", m2.Seq)
	}

	msgs, err := s.ListChatMessages(th.ID)
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("mensagens inesperadas: %+v", msgs)
	}
	if msgs[1].Sources != `[{"session_id":"s1"}]` {
		t.Fatalf("sources não persistido: %q", msgs[1].Sources)
	}

	// updated_at do thread avançou após mensagens.
	got, _ := s.GetChatThread(th.ID)
	if got.UpdatedAt < th.UpdatedAt {
		t.Fatalf("updated_at não avançou")
	}
}
