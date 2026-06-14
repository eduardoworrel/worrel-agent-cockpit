// Package metasession detecta chamadas headless do PRÓPRIO worrel (destilador e
// clusterizador) a partir do primeiro texto de usuário de uma sessão.
//
// Vive num pacote neutro e minúsculo (sem dependências internas) para que TANTO
// os adaptadores (internal/adapter/...) na DESCOBERTA quanto o importador
// (internal/distill) usem EXATAMENTE a mesma lógica sem criar ciclo de import:
// distill_test importa claudecode, então claudecode não pode importar distill.
package metasession

import "strings"

// Assinaturas dos prompts headless do PRÓPRIO worrel (destilador e clusterizador).
// Sessões cujo primeiro prompt de usuário começa por uma destas são chamadas
// internas do app e NÃO devem ser observadas/re-importadas como trabalho do
// usuário — caso contrário o worrel "come o próprio rabo".
// Mantidas em sincronia com os literais de internal/distill/prompt.go e
// internal/retro/cluster.go.
const (
	sigDistiller = "Você é um destilador de conhecimento."
	sigClusterer = "Você organiza um histórico de sessões de codificação em projetos."
)

// headWindow limita a busca da assinatura ao INÍCIO do primeiro evento: curto o
// bastante para não pegar conversas reais que só citam o texto mais adiante,
// largo o bastante para o preâmbulo <local-command-caveat> (~230 chars) que o
// Claude Code injeta antes do prompt headless.
const headWindow = 1000

// IsWorrelMeta reporta se o primeiro texto de usuário da sessão é uma invocação
// headless do próprio worrel. Robusto a dois embrulhos observados: OpenCode
// persiste o prompt entre aspas (`"Você é um destilador...`) e o Claude Code
// prefixa um bloco <local-command-caveat>…</local-command-caveat>. Por isso
// procuramos a assinatura numa janela curta do início (não prefixo exato, nem o
// diálogo todo — que descartaria conversas reais que apenas citam o texto).
func IsWorrelMeta(firstUserText string) bool {
	head := firstUserText
	if len(head) > headWindow {
		head = head[:headWindow]
	}
	return strings.Contains(head, sigDistiller) || strings.Contains(head, sigClusterer)
}
