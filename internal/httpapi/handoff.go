package httpapi

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
)

// handoffSummaryTimeout limita a geração de resumo no caminho de retomada para
// que a CLI headless não trave o endpoint indefinidamente. Se estourar, o
// handoff segue sem resumo (a sessão ainda abre).
const handoffSummaryTimeout = 45 * time.Second

// SummaryGeneratorIface é satisfeita por *handoff.Generator.
type SummaryGeneratorIface interface {
	GenerateSummary(ctx context.Context, sessionID string) (string, error)
}

// Spawner cria uma nova sessão wrapper num projeto, com um primer e um link
// de continuação para a sessão anterior.
type Spawner interface {
	Spawn(projectID, primer, continues string) (newSessionID string, err error)
}

func (s *Server) routesHandoff() {
	s.mux.HandleFunc("POST /api/sessions/{id}/handoff", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		old, err := s.deps.Store.GetSession(id)
		if err != nil {
			writeErr(w, 404, "sessão não encontrada")
			return
		}
		if s.deps.Handoff == nil || s.deps.Spawner == nil {
			writeErr(w, 503, "handoff indisponível (wrapper/adaptador não configurado)")
			return
		}

		// 1) resumo estruturado. Reusa o que já estiver persistido (ex.: gerado na
		// recuperação de boot das órfãs) para retomar sem custo de LLM. Caso
		// contrário gera com timeout: se a CLI headless travar ou falhar, o
		// handoff NÃO aborta — segue sem resumo para que a sessão sempre abra.
		summary := old.Summary
		if strings.TrimSpace(summary) == "" {
			ctx, cancel := context.WithTimeout(r.Context(), handoffSummaryTimeout)
			gen, gerr := s.deps.Handoff.GenerateSummary(ctx, old.ID)
			cancel()
			if gerr != nil {
				log.Printf("handoff %s: resumo falhou, seguindo sem ele: %v", old.ID, gerr)
			} else {
				summary = gen
			}
		}

		// 2) primer = memória + resumo + skills em uso
		primer := s.buildHandoffPrimer(old.ProjectID, summary)

		// 3) nova sessão continuando a antiga
		newID, err := s.deps.Spawner.Spawn(old.ProjectID, primer, old.ID)
		if err != nil {
			writeErr(w, 500, "falha ao iniciar nova sessão: "+err.Error())
			return
		}

		// 4) arquivar a antiga
		if err := s.deps.Store.ArchiveSession(old.ID); err != nil {
			writeErr(w, 500, err.Error())
			return
		}

		s.deps.Bus.Publish(bus.Event{Type: "session.handoff", Payload: map[string]any{
			"old_id": old.ID, "new_id": newID}})
		writeJSON(w, 200, map[string]string{
			"old_id": old.ID, "new_id": newID, "summary": summary})
	})
}

func (s *Server) buildHandoffPrimer(projectID, summary string) string {
	var b strings.Builder
	if mem, err := s.deps.Store.GetMemory(projectID); err == nil && mem.Content != "" {
		b.WriteString("# Memória do projeto\n\n" + mem.Content + "\n\n")
	}
	if strings.TrimSpace(summary) != "" {
		b.WriteString("# Resumo de handoff da sessão anterior\n\n" + summary + "\n\n")
	}
	if skills, err := s.deps.Store.ListSkills(projectID); err == nil && len(skills) > 0 {
		b.WriteString("# Skills disponíveis\n\n")
		for _, sk := range skills {
			b.WriteString("- " + sk.Name + "\n")
		}
	}
	return b.String()
}
