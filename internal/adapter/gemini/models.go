package gemini

import (
	"context"
	"os/exec"
)

// ListModels: o CLI `gemini` (Gemini CLI) seleciona modelo via flag `--model`/`-m`
// e via comando interativo `/model`, mas NÃO oferece um comando headless para
// listar os modelos disponíveis para a subscription/login. Por isso devolvemos uma
// lista CURADA dos ids atuais da família Gemini 2.5/3 (ids reais aceitos por
// `gemini --model <id>`; não refletem 1:1 o acesso exato da conta). Degrada: se o
// binário `gemini` não estiver no PATH, devolve ([], erro).
func (a *Adapter) ListModels(ctx context.Context) ([]string, error) {
	if _, err := exec.LookPath("gemini"); err != nil {
		return []string{}, err
	}
	return curatedGeminiModels(), nil
}

// curatedGeminiModels: ids curados (não inventados) da família Gemini atual.
func curatedGeminiModels() []string {
	return []string{
		"gemini-2.5-pro",
		"gemini-2.5-flash",
	}
}
