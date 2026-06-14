package codex

import (
	"encoding/json"
	"strings"
)

// rawLine espelha uma linha do rollout JSONL do Codex (campos relevantes).
//
// Formato confirmado (rollout-*.jsonl, cli_version 0.116):
//   - toda linha tem {"timestamp": RFC3339, "type": ..., "payload": {...}}.
//   - type "session_meta": payload tem id, timestamp, cwd, originator, cli_version.
//   - type "response_item" + payload.type "message": payload tem role
//     ("user"/"assistant"/"developer"/"system") e content[] com blocos
//     {"type":"input_text"|"output_text","text":...}.
//   - type "token_count": payload.info.{total_token_usage,last_token_usage} com
//     input_tokens/output_tokens/total_tokens.
//   - type "task_started": payload.model_context_window (janela do modelo).
type rawLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

// metaPayload é o payload de session_meta.
type metaPayload struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
}

// itemPayload cobre response_item (message), token_count e task_started.
type itemPayload struct {
	Type    string         `json:"type"`
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`

	// token_count
	Info *struct {
		TotalTokenUsage *tokenUsage `json:"total_token_usage"`
		LastTokenUsage  *tokenUsage `json:"last_token_usage"`
	} `json:"info"`

	// task_started
	ModelContextWindow int `json:"model_context_window"`
}

type tokenUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// extractContent concatena o texto dos blocos input_text/output_text.
func extractContent(blocks []contentBlock) string {
	var parts []string
	for _, b := range blocks {
		switch b.Type {
		case "input_text", "output_text", "text":
			if b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
