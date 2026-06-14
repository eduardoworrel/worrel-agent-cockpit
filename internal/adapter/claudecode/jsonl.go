package claudecode

import (
	"encoding/json"
	"strings"
)

// rawEvent espelha uma linha do JSONL do Claude Code (campos relevantes).
type rawEvent struct {
	Type      string      `json:"type"`
	SessionID string      `json:"sessionId"`
	Cwd       string      `json:"cwd"`
	Timestamp string      `json:"timestamp"`
	Title     string      `json:"title"` // só em ai-title
	Message   *rawMessage `json:"message"`
}

type rawMessage struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Usage   *rawUsage       `json:"usage"`
	Content json.RawMessage `json:"content"` // string OU lista de blocos
}

type rawUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

type contentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
}

// extractText devolve o texto normalizado de um message.content
// (string direta, ou concatenação de blocos text/thinking; tool_use/tool_result
// viram JSON compacto rotulado).
func extractText(raw json.RawMessage) (text, kind string) {
	if len(raw) == 0 {
		return "", "text"
	}
	// tenta string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, "text"
	}
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return string(raw), "text"
	}
	var parts []string
	kind = "text"
	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, b.Text)
		case "thinking":
			parts = append(parts, b.Thinking)
		case "tool_use", "tool_result":
			kind = b.Type
			parts = append(parts, "["+b.Type+"]")
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n")), kind
}
