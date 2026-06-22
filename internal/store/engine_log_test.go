package store

import "testing"

func TestEngineLogHasInputOutputColumns(t *testing.T) {
	st, err := Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	for _, col := range []string{"input", "output"} {
		var n int
		if err := st.DB().QueryRow(
			`SELECT count(*) FROM pragma_table_info('engine_log') WHERE name=?`, col,
		).Scan(&n); err != nil {
			t.Fatalf("pragma: %v", err)
		}
		if n != 1 {
			t.Fatalf("coluna %q ausente em engine_log", col)
		}
	}
}

func TestLogEngineRunPersistsInputOutput(t *testing.T) {
	st, err := Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	if err := st.LogEngineRun(&EngineLogEntry{
		EngineID: "summary", SessionID: "s1",
		Trigger: "realtime", Suggestions: 0, Detail: "",
		Input: "PROMPT-X", Output: "RESPOSTA-Y",
	}); err != nil {
		t.Fatalf("log: %v", err)
	}
	got, err := st.ListEngineLog(10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Input != "PROMPT-X" || got[0].Output != "RESPOSTA-Y" {
		t.Fatalf("input/output não persistiram: %+v", got)
	}
}
