package wrapper

import "github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"

// ingestTranscript apenda em transcript_events apenas os eventos NOVOS lidos do
// .jsonl da sessão. ReadTranscript devolve a lista COMPLETA a cada chamada
// (filtrando eventos vazios de forma determinística), então os já persistidos
// são os N primeiros: apendamos events[N:]. Devolve quantos foram gravados.
func (m *Manager) ingestTranscript(sessionID string, events []adapter.TranscriptEvent) (int, error) {
	existing, err := m.store.ListTranscriptEvents(sessionID)
	if err != nil {
		return 0, err
	}
	n := len(existing)
	if n >= len(events) {
		return 0, nil
	}
	written := 0
	for _, e := range events[n:] {
		if err := m.store.AppendTranscriptEventRich(sessionID, e.Role, e.Kind, e.Content, e.Payload, e.TokensIn, e.TokensOut); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// trackTranscript lê o .jsonl ao vivo da sessão wrapper e ingere os eventos
// novos em transcript_events (fonte da verdade para os motores de análise).
// Falhas de leitura (sessão recém-criada, arquivo ainda inexistente) são
// silenciosas — o próximo poll tenta de novo.
func (m *Manager) trackTranscript(sessID string, ref adapter.SessionRef, ad adapter.Adapter) {
	events, err := ad.ReadTranscript(ref)
	if err != nil {
		return
	}
	_, _ = m.ingestTranscript(sessID, events)
}
