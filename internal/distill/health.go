package distill

import (
	"fmt"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// HealthSignal representa um alerta de saúde para uma skill.
type HealthSignal struct {
	SkillID          string `json:"skill_id"`
	SkillName        string `json:"skill_name"`
	ConsecFailures   int    `json:"consec_failures"`
	NeedsAutoCorrect bool   `json:"needs_auto_correct"`
}

// HealthChecker verifica saúde das skills com base em falhas consecutivas.
type HealthChecker struct {
	store     *store.Store
	threshold int
}

// NewHealthChecker cria um HealthChecker com threshold de falhas consecutivas.
func NewHealthChecker(s *store.Store, threshold int) *HealthChecker {
	return &HealthChecker{store: s, threshold: threshold}
}

// Scan percorre todas as skills e retorna sinais de saúde para as que ultrapassaram o threshold.
func (h *HealthChecker) Scan() ([]*HealthSignal, error) {
	skills, err := h.store.ListSkills("")
	if err != nil {
		return nil, err
	}
	var signals []*HealthSignal
	for _, sk := range skills {
		n, err := h.store.ConsecutiveFailures(sk.ID)
		if err != nil {
			return nil, err
		}
		if n < h.threshold {
			continue
		}
		needsAuto := sk.EvolutionPolicy != "manual" && !h.hasPendingProactive(sk.ID)
		signals = append(signals, &HealthSignal{
			SkillID:          sk.ID,
			SkillName:        sk.Name,
			ConsecFailures:   n,
			NeedsAutoCorrect: needsAuto,
		})
	}
	return signals, nil
}

// CreateProactiveCorrections varre as skills e, para cada uma degradada
// (≥ threshold falhas consecutivas), cria UMA sugestão skill.correction
// proativa com diagnóstico em evidence — sem ação do usuário (spec §4.3,
// critério 4). Idempotente: não duplica se já houver correção pendente da
// mesma skill. Publica skill.health.degraded no bus. Se a skill tem política
// auto e um auto-aplicador foi fornecido, tenta auto-aplicar a correção.
// Retorna quantas correções proativas foram criadas.
func (h *HealthChecker) CreateProactiveCorrections(b *bus.Bus, auto AutoApplier, dailyCap int) (int, error) {
	skills, err := h.store.ListSkills("")
	if err != nil {
		return 0, err
	}
	created := 0
	for _, sk := range skills {
		n, err := h.store.ConsecutiveFailures(sk.ID)
		if err != nil {
			return created, err
		}
		if n < h.threshold {
			continue
		}
		if h.hasPendingProactive(sk.ID) {
			continue
		}
		stats, _ := h.store.SkillStats(sk.ID)
		rate := 0.0
		uses := 0
		if stats != nil {
			rate = stats.SuccessRate
			uses = stats.TotalUses
		}
		diag := fmt.Sprintf("Saúde degradada: %d falha(s) consecutiva(s), taxa de sucesso %.0f%% em %d uso(s).",
			n, rate*100, uses)
		sid := sk.ID
		payload := fmt.Sprintf(`{"name":%q,"content":%q,"change_summary":"correção proativa por degradação de saúde"}`,
			sk.Name, sk.Content)
		sg, err := h.store.CreateSuggestion(&store.Suggestion{
			ProjectID: sk.ProjectID,
			SkillID:   &sid,
			Type:      "skill.correction",
			Title:     "Correção proativa: " + sk.Name,
			Payload:   payload,
			Evidence:  diag,
		})
		if err != nil {
			return created, err
		}
		created++
		if b != nil {
			b.Publish(bus.Event{Type: "skill.health.degraded",
				Payload: map[string]any{"skill_id": sk.ID, "diagnosis": diag}})
		}
		if auto != nil {
			_, _ = auto.MaybeAutoApply(sg.ID, dailyCap)
		}
	}
	return created, nil
}

func (h *HealthChecker) hasPendingProactive(skillID string) bool {
	sgs, err := h.store.ListSuggestions("", "pending")
	if err != nil {
		return false
	}
	for _, sg := range sgs {
		if sg.SkillID != nil && *sg.SkillID == skillID && sg.Type == "skill.correction" {
			return true
		}
	}
	return false
}
