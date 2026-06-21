// Package skill implementa o Motor de Skill/Agente: detecta workflows dirigidos
// pelo usuário, acumula recorrência entre sessões e os matura (SP4).
package skill

import (
	"regexp"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// WorkflowWindow é um trecho onde o usuário dirigiu uma sequência de passos.
type WorkflowWindow struct {
	Signal string
	Events []*store.TranscriptEvent
}

// stepMarkers são pistas de que o usuário ENUMEROU passos (não só um objetivo).
var stepMarkers = regexp.MustCompile(`(?i)\b(primeiro|depois|ent[ãa]o|em seguida|por fim|passo\s*\d|step\s*\d|1\.|2\.|3\.)\b`)

func isToolUse(e *store.TranscriptEvent) bool {
	return e.Kind == "tool_use"
}

// DetectWorkflows acha janelas de workflow dirigido pelo usuário: uma mensagem
// de usuário enumerando passos (marcadores sequenciais) seguida de ≥2 tool_use.
// Conservador: delegação simples ("resolve X") sem enumeração NÃO gera janela.
// A assinatura semântica NÃO é calculada aqui (vem do LLM, Task 5).
func DetectWorkflows(events []*store.TranscriptEvent) []WorkflowWindow {
	var out []WorkflowWindow
	for i := 0; i < len(events); i++ {
		e := events[i]
		if e.Role != "user" || e.Kind != "text" {
			continue
		}
		if !stepMarkers.MatchString(e.Content) {
			continue
		}
		// coleta os tool_use que seguem (até a próxima mensagem de usuário)
		win := []*store.TranscriptEvent{e}
		tools := 0
		for j := i + 1; j < len(events); j++ {
			if events[j].Role == "user" && events[j].Kind == "text" {
				break
			}
			win = append(win, events[j])
			if isToolUse(events[j]) {
				tools++
			}
		}
		if tools >= 2 {
			out = append(out, WorkflowWindow{Signal: "user_steps", Events: win})
		}
	}
	return out
}

// joinContents é util p/ o distiller montar o texto da janela.
func joinContents(w WorkflowWindow) string {
	var parts []string
	for _, e := range w.Events {
		parts = append(parts, e.Role+"/"+e.Kind+": "+e.Content)
	}
	return strings.Join(parts, "\n")
}
