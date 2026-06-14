package claudecode

import (
	"context"
	"os/exec"
)

// ListModels: o CLI `claude` (Claude Code) NÃO expõe um comando para listar os
// modelos da subscription ativa — `--model` aceita um id/alias, mas não há
// subcomando de listagem. Por isso devolvemos uma lista CURADA dos ids atuais da
// família Claude (Opus/Sonnet/Haiku 4.x). Esses ids são reais e correspondem ao
// que `claude --model <id>` aceita; não refletem 1:1 o que a subscription do
// usuário libera (limitação do CLI). Degrada: se o `claude` não estiver no PATH,
// devolve ([], erro) para o consumidor sinalizar indisponibilidade.
func (a *Adapter) ListModels(ctx context.Context) ([]string, error) {
	if _, err := exec.LookPath("claude"); err != nil {
		return []string{}, err
	}
	return curatedClaudeModels(), nil
}

// curatedClaudeModels: ids curados (não inventados) da família Claude atual.
// Mantenha em sincronia com os modelos ativos do Claude.
func curatedClaudeModels() []string {
	return []string{
		"claude-opus-4-8",
		"claude-opus-4-7",
		"claude-opus-4-6",
		"claude-sonnet-4-6",
		"claude-haiku-4-5",
	}
}
