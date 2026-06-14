// Package retention implementa o janitor de transcripts (spec §11):
// transcripts brutos além da janela de retenção (padrão 30 dias) são
// apagados; metadados de sessão, memória, skills, sugestões+evidências e
// auditoria permanecem permanentes.
package retention

import (
	"strconv"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

const DefaultRetentionDays = 30

type Janitor struct {
	store *store.Store
}

func New(s *store.Store) *Janitor { return &Janitor{store: s} }

// RetentionDays lê o setting; default 30 se ausente/inválido/<=0.
func (j *Janitor) RetentionDays() int {
	v := j.store.GetSetting("retention_days", "")
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return n
	}
	return DefaultRetentionDays
}

// Sweep poda todos os transcripts expirados. Devolve quantos foram podados.
func (j *Janitor) Sweep() (int, error) {
	days := j.RetentionDays()
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
	ids, err := j.store.ExpiredSessionIDs(cutoff)
	if err != nil {
		return 0, err
	}
	pruned := 0
	for _, id := range ids {
		if err := j.store.PruneSessionTranscript(id); err != nil {
			return pruned, err
		}
		pruned++
	}
	return pruned, nil
}
