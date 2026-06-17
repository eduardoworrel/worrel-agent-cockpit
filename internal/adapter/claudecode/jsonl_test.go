package claudecode

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractToolUseRich(t *testing.T) {
	raw := json.RawMessage(`[{"type":"tool_use","name":"Bash","input":{"command":"ls -la"}}]`)
	text, kind := extractText(raw)
	if kind != "tool_use" {
		t.Fatalf("kind=%q", kind)
	}
	if !strings.Contains(text, "Bash") || !strings.Contains(text, "ls -la") {
		t.Fatalf("text=%q", text)
	}
	pl := blockPayload(raw)
	if !strings.Contains(pl, `"name":"Bash"`) || !strings.Contains(pl, "ls -la") {
		t.Fatalf("payload=%q", pl)
	}
}

func TestExtractToolResultRich(t *testing.T) {
	raw := json.RawMessage(`[{"type":"tool_result","content":"total 8\ndrwxr","is_error":false}]`)
	text, kind := extractText(raw)
	if kind != "tool_result" || !strings.Contains(text, "total 8") {
		t.Fatalf("text=%q kind=%q", text, kind)
	}
	if pl := blockPayload(raw); !strings.Contains(pl, "total 8") {
		t.Fatalf("payload=%q", pl)
	}
}

func TestBlockPayloadEmptyForNonTool(t *testing.T) {
	if pl := blockPayload(json.RawMessage(`[{"type":"text","text":"oi"}]`)); pl != "" {
		t.Fatalf("texto deveria dar payload vazio, got %q", pl)
	}
	if pl := blockPayload(json.RawMessage(`"string direta"`)); pl != "" {
		t.Fatalf("string deveria dar payload vazio, got %q", pl)
	}
}

func TestExtractTextStillWorksForText(t *testing.T) {
	text, kind := extractText(json.RawMessage(`[{"type":"text","text":"olá mundo"}]`))
	if kind != "text" || text != "olá mundo" {
		t.Fatalf("text=%q kind=%q", text, kind)
	}
}
