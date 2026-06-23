package antigravity

import (
	"context"
	"os/exec"
	"strings"
)

// ListModels usa o comando real `agy models`, que imprime um modelo por linha
// (ex.: "Gemini 3.1 Pro (Low)"). Reflete a subscription/login ativo do usuário.
// Degrada: se o binário `agy` não estiver no PATH, devolve ([], erro).
func (a *Adapter) ListModels(ctx context.Context) ([]string, error) {
	if _, err := exec.LookPath("agy"); err != nil {
		return []string{}, err
	}
	out, err := exec.CommandContext(ctx, "agy", "models").Output()
	if err != nil {
		return []string{}, err
	}
	return parseModels(out), nil
}

// parseModels quebra a saída de `agy models` em linhas, faz trim e descarta vazias.
func parseModels(out []byte) []string {
	var models []string
	for _, line := range strings.Split(string(out), "\n") {
		if m := strings.TrimSpace(line); m != "" {
			models = append(models, m)
		}
	}
	return models
}
