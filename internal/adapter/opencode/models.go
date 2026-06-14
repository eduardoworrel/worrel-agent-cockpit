package opencode

import (
	"context"
	"os/exec"
	"strings"
)

// ListModels usa o comando REAL `opencode models`, que imprime um modelo por
// linha no formato "provider/model" considerando apenas os providers em que o
// usuário está autenticado (reflete a subscription/login ativo). Degrada
// graciosamente: se o CLI não estiver instalado, devolve ([], erro).
func (a *Adapter) ListModels(ctx context.Context) ([]string, error) {
	if _, err := exec.LookPath("opencode"); err != nil {
		return []string{}, err
	}
	out, err := exec.CommandContext(ctx, "opencode", "models").Output()
	if err != nil {
		return []string{}, err
	}
	return parseOpencodeModels(out), nil
}

// parseOpencodeModels extrai os ids de modelo da saída de `opencode models`
// (um "provider/model" por linha). Ignora linhas vazias e deduplica preservando
// a ordem de aparição.
func parseOpencodeModels(out []byte) []string {
	models := []string{}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		models = append(models, line)
	}
	return models
}
