// Package memory implementa o Motor de Memória: detecta sinais de atrito no
// transcript e destila golden truths anti-erro (SP3).
package memory

import (
	"encoding/json"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// FrictionWindow é um trecho contíguo de eventos que carrega um sinal de atrito.
type FrictionWindow struct {
	Signal string                   // ex.: "error_then_success"
	Events []*store.TranscriptEvent // eventos relevantes da janela
}

// toolBlock espelha um item do payload JSON de um evento (formato SP2).
type toolBlock struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Output  string `json:"output"`
	IsError bool   `json:"is_error"`
}

// eventErrored reporta se o evento é um tool_result com is_error=true.
func eventErrored(e *store.TranscriptEvent) bool {
	for _, b := range parseBlocks(e.Payload) {
		if b.Type == "tool_result" && b.IsError {
			return true
		}
	}
	return false
}

// eventToolUse devolve (true, "") se o evento é um tool_use; o segundo valor é o
// nome da ferramenta (ex.: "Bash"), quando disponível.
func eventToolUse(e *store.TranscriptEvent) (bool, string) {
	for _, b := range parseBlocks(e.Payload) {
		if b.Type == "tool_use" {
			return true, b.Name
		}
	}
	return false, ""
}

func parseBlocks(payload string) []toolBlock {
	if payload == "" {
		return nil
	}
	var bs []toolBlock
	if json.Unmarshal([]byte(payload), &bs) != nil {
		return nil
	}
	return bs
}

// DetectFriction varre os eventos (em ordem de seq) e devolve as janelas de
// atrito. Heurística conservadora do SP3: o padrão tentativa-falha-resolução —
// um tool_result com is_error=true que é seguido, mais adiante na sessão, por um
// tool_use (nova tentativa). Um erro isolado sem tentativa seguinte NÃO gera
// janela (provável transitório). A janela abrange do tool_use que falhou até a
// nova tentativa.
func DetectFriction(events []*store.TranscriptEvent) []FrictionWindow {
	var out []FrictionWindow
	for i := 0; i < len(events); i++ {
		if !eventErrored(events[i]) {
			continue
		}
		// procura a próxima tentativa (tool_use) após o erro
		for j := i + 1; j < len(events); j++ {
			if ok, _ := eventToolUse(events[j]); ok {
				start := i
				if i > 0 {
					start = i - 1 // inclui o tool_use que falhou, se houver
				}
				win := events[start : j+1]
				out = append(out, FrictionWindow{Signal: "error_then_success", Events: win})
				i = j // não re-emite janelas sobrepostas a partir do mesmo erro
				break
			}
		}
	}
	return out
}
