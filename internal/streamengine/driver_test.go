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
