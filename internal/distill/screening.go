package distill

import "strings"

// screenSignal é um sinal local (sem LLM) que indica se vale chamar o LLM.
// Fase 1 do pipeline (spec §5): heurísticas puramente locais decidem se um
// lote de sessões merece gastar tokens na confirmação por LLM (Fase 2).
type screenSignal struct {
	HasTranscript bool
	HasErrors     bool
	Substantial   bool
}

// Pass indica se o lote passou no screening de Fase 1 e portanto justifica
// uma chamada de LLM. Critério de aceitação 5: candidato que não passa aqui
// NUNCA aciona RunHeadless.
func (s screenSignal) Pass() bool {
	if !s.HasTranscript {
		return false
	}
	return s.HasErrors || s.Substantial
}

// screenSessions verifica localmente (sem LLM) se há sinais que justifiquem
// uma chamada de LLM: erros/falhas no transcript, ou volume substancial de
// conteúdo (sinal de tarefa reutilizável). Lotes vazios ou triviais reprovam.
func screenSessions(batch []sessionTranscript) screenSignal {
	if len(batch) == 0 {
		return screenSignal{}
	}
	sig := screenSignal{HasTranscript: true}
	var totalLen int
	var totalEvents int
	for _, s := range batch {
		for _, ev := range s.Events {
			totalEvents++
			totalLen += len(ev.Content)
			low := strings.ToLower(ev.Content)
			if ev.Role == "tool" || ev.Kind == "error" ||
				strings.Contains(low, "error") || strings.Contains(low, "erro") ||
				strings.Contains(low, "failed") || strings.Contains(low, "falhou") ||
				strings.Contains(low, "exception") || strings.Contains(low, "traceback") {
				sig.HasErrors = true
			}
		}
	}
	// "Substancial": conteúdo longo OU várias trocas — indica tarefa não-trivial.
	sig.Substantial = totalLen > 400 || totalEvents > 8
	return sig
}
