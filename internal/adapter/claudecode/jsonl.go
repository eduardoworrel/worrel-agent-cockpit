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
	// tool_use
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
	// tool_result (content é string OU lista de blocos {type:text,text})
	Content   json.RawMessage `json:"content"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
}

// extractText devolve o texto legível de um message.content e o kind do evento.
// String direta → text. Lista de blocos → concatena text/thinking; tool_use vira
// "<name> <input>"; tool_result vira o texto do output. O dado ESTRUTURADO da
// ferramenta sai em blockPayload (não aqui).
func extractText(raw json.RawMessage) (text, kind string) {
	if len(raw) == 0 {
		return "", "text"
	}
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
		case "tool_use":
			kind = "tool_use"
			if b.Name != "" {
				parts = append(parts, strings.TrimSpace(b.Name+" "+string(b.Input)))
			} else {
				parts = append(parts, "[tool_use]")
			}
		case "tool_result":
			kind = "tool_result"
			parts = append(parts, toolResultText(b.Content))
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n")), kind
}

// toolResultText extrai o texto de um tool_result.content (string direta, lista
// de blocos {type:text,text}, ou JSON cru como fallback).
func toolResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "[tool_result]"
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return string(raw)
}

// blockPayload devolve um JSON com os dados estruturados de ferramenta de um
// message.content (tool_use: {type,name,input}; tool_result: {type,output,is_error}),
// ou "" quando não há bloco de ferramenta. Usado para enriquecer transcript_events.
func blockPayload(raw json.RawMessage) string {
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	type toolPayload struct {
		Type    string          `json:"type"`
		Name    string          `json:"name,omitempty"`
		Input   json.RawMessage `json:"input,omitempty"`
		Output  string          `json:"output,omitempty"`
		IsError bool            `json:"is_error,omitempty"`
	}
	var tools []toolPayload
	for _, b := range blocks {
		switch b.Type {
		case "tool_use":
			tools = append(tools, toolPayload{Type: "tool_use", Name: b.Name, Input: b.Input})
		case "tool_result":
			tools = append(tools, toolPayload{Type: "tool_result", Output: toolResultText(b.Content), IsError: b.IsError})
		}
	}
	if len(tools) == 0 {
		return ""
	}
	out, err := json.Marshal(tools)
	if err != nil {
		return ""
	}
	return string(out)
}
