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
