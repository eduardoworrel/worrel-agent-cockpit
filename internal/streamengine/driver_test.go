// internal/streamengine/driver_test.go
package streamengine

import (
	"context"
	"testing"
)

func TestDefaultDriversHasClaude(t *testing.T) {
	d := DefaultDrivers()
	if _, ok := d["claude-code"]; !ok {
		t.Fatalf("DefaultDrivers() faltou claude-code: %v", keys(d))
	}
}

func TestClaudeDriverSatisfiesDriver(t *testing.T) {
	var _ Driver = claudeDriver{}
}

// compile-time guard that the concrete Claude session satisfies LiveSession.
func TestSessionSatisfiesLiveSession(t *testing.T) {
	var _ LiveSession = (*Session)(nil)
}

func keys(m map[string]Driver) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

var _ = context.Background

func TestManagerStartUnknownProvider(t *testing.T) {
	m := NewManager(nil, nil)
	err := m.Start(context.Background(), "antigravity", "s1", t.TempDir(), Opts{})
	if err != ErrUnknownProvider {
		t.Fatalf("quer ErrUnknownProvider p/ provider sem driver, veio: %v", err)
	}
	if m.Has("s1") {
		t.Fatal("não deveria registrar sessão para provider desconhecido")
	}
}
