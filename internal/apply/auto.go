package apply

import (
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// MaybeAutoApply aplica automaticamente a sugestão se a política da skill
// permitir (spec §6.1, critérios 6/8). Regras:
//   - learned/criação inicial (sem skill_id) NUNCA é automática;
//   - skill.correction exige policy auto_correction OU auto_total;
//   - skill.variant exige policy auto_total;
//   - respeita o cap diário por skill (excedente → permanece pending);
//   - após aplicar, verifica salvaguarda de saúde (MaybeSuspend).
//
// Retorna (true, nil) se aplicou, (false, nil) se não aplicou (sem erro).
func (a *Applier) MaybeAutoApply(suggestionID string, dailyCap int) (bool, error) {
	sg, err := a.store.GetSuggestion(suggestionID)
	if err != nil {
		return false, err
	}
	if sg.Status != "pending" && sg.Status != "deferred" {
		return false, nil
	}
	if sg.SkillID == nil {
		return false, nil // learned/variant sem mãe → manual (§6.1)
	}
	sk, err := a.store.GetSkill(*sg.SkillID)
	if err != nil {
		return false, err
	}
	policy := sk.EvolutionPolicy
	allow := (sg.Type == "skill.correction" && (policy == "auto_correction" || policy == "auto_total")) ||
		(sg.Type == "skill.variant" && policy == "auto_total")
	if !allow {
		return false, nil
	}

	// Cap diário por skill: excedente permanece pending na fila manual (§6.2, critério 8).
	startOfDay := startOfDayMilli()
	count, err := a.store.CountAutoAppliedToday(*sg.SkillID, startOfDay)
	if err != nil {
		return false, err
	}
	if count >= dailyCap {
		return false, nil
	}

	// Linha de base de saúde antes da aplicação, p/ detectar piora (§6.2).
	var baseline float64
	if st, _ := a.store.SkillStats(*sg.SkillID); st != nil {
		baseline = st.SuccessRate
	}

	if err := a.AutoApply(suggestionID); err != nil {
		return false, err
	}
	a.publishBus("skill.auto_applied", map[string]any{
		"skill_id": *sg.SkillID, "suggestion_id": sg.ID,
	})

	// Salvaguarda pós-fato: suspende se a saúde piorou (§6.2, critério 7).
	_, _ = a.MaybeSuspend(*sg.SkillID, baseline)
	return true, nil
}

// MaybeSuspend rebaixa a política da skill para "manual" e alerta o usuário
// (evento skill.auto_suspended) se a saúde piorou após uma aplicação
// automática (spec §6.2, critério 7): taxa de sucesso caindo na janela
// seguinte, OU falhas consecutivas acumuladas. Retorna true se suspendeu.
func (a *Applier) MaybeSuspend(skillID string, baselineRate float64) (bool, error) {
	st, err := a.store.SkillStats(skillID)
	if err != nil {
		return false, err
	}
	degraded := false
	if st != nil && st.TotalUses >= 3 && st.SuccessRate < baselineRate-0.1 {
		degraded = true
	}
	if st != nil && st.ConsecFail >= 2 {
		degraded = true
	}
	if !degraded {
		return false, nil
	}
	if err := a.store.SetSkillPolicy(skillID, "manual"); err != nil {
		return false, err
	}
	a.publishBus("skill.auto_suspended", map[string]any{
		"skill_id": skillID, "reason": "degradação de saúde após aplicação automática",
	})
	return true, nil
}

func startOfDayMilli() int64 {
	now := time.Now()
	sod := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return sod.UnixMilli()
}

// expose store interface for MaybeSuspend
func (a *Applier) ConsecutiveFailures(skillID string) (int, error) {
	return a.store.ConsecutiveFailures(skillID)
}

// expose store for settings check
func (a *Applier) GetSetting(key, def string) string {
	return a.store.GetSetting(key, def)
}

// expose for tests
func (a *Applier) Store() *store.Store { return a.store }
