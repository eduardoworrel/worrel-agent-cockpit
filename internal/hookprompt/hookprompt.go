// Package hookprompt implementa o subcomando "worrel hook prompt": lê o JSON do
// hook pré-execução no stdin, pede a decisão ao worrel (que bloqueia até o balão),
// e imprime a decisão de permissão no formato esperado pelo CLI alvo.
//
// Formatos suportados (--format):
//   - "claude" / "codex": idênticos —
//     {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow|deny|ask"}}.
//     Codex (PreToolUse) reusa o mesmo schema do Claude Code, incluindo o enum
//     allow/deny/ask, então compartilham a renderização.
package hookprompt

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// input é o subconjunto do JSON do hook que usamos. Claude Code e Codex
// entregam o nome e os args da tool em tool_name/tool_input.
type input struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

// render produz o JSON de saída no formato do CLI alvo. decision ∈
// {"allow","deny",""}; "" significa deferir ao fluxo nativo de permissão.
func render(format, decision string) any {
	// "claude", "codex" e qualquer outro: schema do Claude Code (mesmo enum
	// allow/deny/ask). O parâmetro format permanece para futuros formatos.
	_ = format
	pd := decision
	if pd == "" {
		pd = "ask" // defer: deixa o CLI seguir o fluxo normal de permissão
	}
	return map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":      "PreToolUse",
			"permissionDecision": pd,
		},
	}
}

// Run executa o fluxo do hook. baseURL é "http://127.0.0.1:<port>"; format
// seleciona o schema de saída. Em qualquer falha (stdin ilegível, worrel
// inacessível) cai no fallback seguro de DEFERIR, deixando o CLI seguir o
// fluxo normal de permissão em vez de negar silenciosamente.
func Run(stdin io.Reader, stdout io.Writer, baseURL, sessionID, format string) error {
	enc := json.NewEncoder(stdout)

	var in input
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		return enc.Encode(render(format, ""))
	}
	body, _ := json.Marshal(map[string]any{"tool": in.ToolName, "input": in.ToolInput})

	client := &http.Client{Timeout: 365 * 24 * time.Hour}
	resp, err := client.Post(baseURL+"/api/sessions/"+sessionID+"/permission-request",
		"application/json", bytes.NewReader(body))
	if err != nil {
		return enc.Encode(render(format, ""))
	}
	defer resp.Body.Close()

	var dec struct {
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dec); err != nil {
		return enc.Encode(render(format, ""))
	}
	// Só "allow"/"deny" explícitos são honrados; qualquer outra coisa (corpo
	// inesperado, decisão vazia) defere ao fluxo nativo.
	d := ""
	if dec.Decision == "allow" || dec.Decision == "deny" {
		d = dec.Decision
	}
	return enc.Encode(render(format, d))
}
