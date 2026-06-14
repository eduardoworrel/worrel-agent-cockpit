package pidev

import (
	"context"
	"os/exec"
	"strings"
)

// ListModels usa o comando REAL `pi --list-models`, que imprime os modelos
// disponíveis por id (provider/model, ex.: "openai/gpt-4o", "anthropic/...").
// A saída reflete os providers configurados/autenticados do usuário. Degrada
// graciosamente: se o CLI não estiver instalado, devolve ([], erro).
func (a *Adapter) ListModels(ctx context.Context) ([]string, error) {
	if _, err := exec.LookPath(binaryName); err != nil {
		return []string{}, err
	}
	out, err := exec.CommandContext(ctx, binaryName, "--list-models").Output()
	if err != nil {
		return []string{}, err
	}
	return parsePiModels(out), nil
}

// parsePiModels extrai um id de modelo por linha da saída de `pi --list-models`.
// Pega o primeiro campo de cada linha (id), ignorando texto secundário/nome após
// espaços ou tabs; ignora vazias e deduplica preservando a ordem.
func parsePiModels(out []byte) []string {
	models := []string{}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		id := strings.Fields(line)[0]
		if seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, id)
	}
	return models
}
