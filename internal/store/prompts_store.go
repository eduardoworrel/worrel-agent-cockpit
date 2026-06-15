package store

import (
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/prompts"
)

// Prompt devolve o texto efetivo de um prompt de análise: o override salvo em
// settings (chave "prompt.<name>") se houver e não-vazio, senão o default
// embarcado em internal/prompts. Override vazio/em branco volta ao default.
func (s *Store) Prompt(name string) string {
	def := prompts.Default(name)
	v := s.GetSetting("prompt."+name, def)
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
