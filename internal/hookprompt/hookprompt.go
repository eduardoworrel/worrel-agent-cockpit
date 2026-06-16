// Package hookprompt implementa o subcomando "worrel hook prompt": lê o JSON do
// PreToolUse no stdin, pede a decisão ao worrel (que bloqueia até o balão), e
// imprime a decisão de permissão no formato esperado pelo Claude Code.
package hookprompt

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// input é o subconjunto do JSON do hook PreToolUse que usamos.
type input struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

type hookSpecificOutput struct {
	HookEventName      string `json:"hookEventName"`
	PermissionDecision string `json:"permissionDecision"`
}

type output struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
}

func decision(pd string) output {
	return output{HookSpecificOutput: hookSpecificOutput{HookEventName: "PreToolUse", PermissionDecision: pd}}
}

func writeOut(w io.Writer, o output) error {
	return json.NewEncoder(w).Encode(o)
}

// Run executa o fluxo do hook. baseURL é "http://127.0.0.1:<port>".
// Em qualquer falha (stdin ilegível, worrel inacessível) cai no fallback seguro
// "ask", deixando o Claude Code seguir o fluxo normal de permissão.
func Run(stdin io.Reader, stdout io.Writer, baseURL, sessionID string) error {
	var in input
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		return writeOut(stdout, decision("ask"))
	}
	body, _ := json.Marshal(map[string]any{"tool": in.ToolName, "input": in.ToolInput})

	client := &http.Client{Timeout: 365 * 24 * time.Hour}
	resp, err := client.Post(baseURL+"/api/sessions/"+sessionID+"/permission-request",
		"application/json", bytes.NewReader(body))
	if err != nil {
		return writeOut(stdout, decision("ask"))
	}
	defer resp.Body.Close()

	var dec struct {
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dec); err != nil {
		return writeOut(stdout, decision("ask"))
	}
	// Só "allow"/"deny" explícitos são honrados; qualquer outra coisa (corpo
	// inesperado, decisão vazia) cai no fallback seguro "ask" — deixa o Claude
	// Code seguir o fluxo normal de permissão em vez de negar silenciosamente.
	pd := "ask"
	if dec.Decision == "allow" || dec.Decision == "deny" {
		pd = dec.Decision
	}
	return writeOut(stdout, decision(pd))
}
