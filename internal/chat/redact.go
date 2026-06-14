package chat

import (
	"regexp"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/retro"
)

// redactPatterns espelha as famílias de credenciais de retro.Detect para
// permitir a SUBSTITUIÇÃO in-place do valor cru pela máscara no texto. retro.Detect
// só expõe a máscara (o valor cru é privado), então re-aplicamos os padrões aqui
// para localizar a posição exata e trocar pelo mascarado, garantindo que NENHUM
// segredo cru chegue ao prompt do LLM.
var redactPatterns = []*regexp.Regexp{
	regexp.MustCompile(`gh[opsu]_[A-Za-z0-9]{16,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`\b[A-Za-z]{2,8}_(?:live|test)_[A-Za-z0-9]{16,}\b`),
	regexp.MustCompile(`(?i)(password\s*[=:]\s*)(\S+)`),
	regexp.MustCompile(`(?i)(bearer\s+)([A-Za-z0-9._-]{12,})`),
	regexp.MustCompile(`\b[A-Za-z0-9+/]{40,}={0,2}\b`),
}

// mask reproduz a máscara de retro (prefixo+sufixo curtos, miolo escondido).
func mask(v string) string {
	if len(v) <= 8 {
		return "****"
	}
	prefix := 4
	if len(v) < 12 {
		prefix = 2
	}
	return v[:prefix] + "****" + v[len(v)-4:]
}

// redactSecrets substitui no texto todos os valores que retro.Detect classifica
// como segredo pela sua máscara. Retorna o texto saneado e quantos achados houve.
func redactSecrets(text string) (string, int) {
	findings := retro.Detect(text)
	n := len(findings)
	out := text
	for _, re := range redactPatterns {
		if re.NumSubexp() >= 2 {
			// padrões com prefixo capturado (password=, bearer ): mascara só o valor.
			out = re.ReplaceAllStringFunc(out, func(m string) string {
				sub := re.FindStringSubmatch(m)
				return sub[1] + mask(sub[2])
			})
			continue
		}
		out = re.ReplaceAllStringFunc(out, mask)
	}
	return out, n
}
