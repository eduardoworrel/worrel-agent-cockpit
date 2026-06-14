package codex

import (
	"context"
	"encoding/json"
	"os/exec"
)

// ListModels usa o comando REAL `codex debug models --bundled`, que imprime o
// catálogo de modelos que o Codex enxerga como JSON. Preferimos `--bundled` para
// pular o refresh remoto e devolver rápido o catálogo embutido (que reflete os
// modelos disponíveis para o login/subscription do usuário). Degrada
// graciosamente: se o CLI não estiver instalado OU o comando falhar/mudar de
// formato, cai numa lista curada de ids atuais da família GPT-5/Codex.
func (a *Adapter) ListModels(ctx context.Context) ([]string, error) {
	if _, err := exec.LookPath("codex"); err != nil {
		return []string{}, err
	}
	out, err := exec.CommandContext(ctx, "codex", "debug", "models", "--bundled").Output()
	if err != nil {
		// CLI presente mas comando indisponível/instável: usa curado.
		return curatedCodexModels(), nil
	}
	if models := parseCodexModels(out); len(models) > 0 {
		return models, nil
	}
	return curatedCodexModels(), nil
}

// parseCodexModels extrai ids do catálogo JSON de `codex debug models`. O shape
// exato pode variar entre versões; tentamos os formatos conhecidos (array de
// objetos com "id"/"name", ou {"models":[...]}). Retorna [] se nada reconhecido.
func parseCodexModels(out []byte) []string {
	type entry struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	pick := func(es []entry) []string {
		ids := []string{}
		seen := map[string]bool{}
		for _, e := range es {
			id := e.ID
			if id == "" {
				id = e.Name
			}
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
		return ids
	}

	// Formato 1: array no topo.
	var arr []entry
	if err := json.Unmarshal(out, &arr); err == nil && len(arr) > 0 {
		return pick(arr)
	}
	// Formato 2: {"models":[...]}.
	var wrap struct {
		Models []entry `json:"models"`
	}
	if err := json.Unmarshal(out, &wrap); err == nil && len(wrap.Models) > 0 {
		return pick(wrap.Models)
	}
	return []string{}
}

// curatedCodexModels é a lista CURADA de fallback (família GPT-5/Codex) usada só
// quando o CLI não expõe/falha o catálogo. Não reflete a subscription exata.
func curatedCodexModels() []string {
	return []string{
		"gpt-5.5",
		"gpt-5.4",
		"gpt-5.4-mini",
		"gpt-5.2-codex",
		"gpt-5.1-codex-max",
		"gpt-5.1-codex-mini",
	}
}
