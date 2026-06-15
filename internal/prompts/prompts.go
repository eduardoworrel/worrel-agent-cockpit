// Package prompts embarca os prompts (markdown) usados nas análises retroativas
// e os expõe como defaults editáveis. O texto efetivo de cada prompt é resolvido
// no store (override em settings sob a chave "prompt.<name>", senão o default).
package prompts

import "embed"

//go:embed memory.md skill.md scope.md
var files embed.FS

// Names lista os prompts editáveis das análises, na ordem de exibição.
var Names = []string{"memory", "skill", "scope"}

// Default devolve o texto embarcado (default) de um prompt. "" se desconhecido.
func Default(name string) string {
	b, err := files.ReadFile(name + ".md")
	if err != nil {
		return ""
	}
	return string(b)
}
