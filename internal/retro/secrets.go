package retro

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// Finding descreve uma credencial detectada. O valor cru (raw) é NÃO exportado e
// nunca serializado/persistido (critério 9) — só máscara e hash saem do pacote.
type Finding struct {
	Name   string
	Masked string
	Hash   string
	raw    string
}

// patterns: famílias comuns de credenciais. O grupo 1 (quando existe) isola o valor.
var patterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"github_token", regexp.MustCompile(`gh[opsu]_[A-Za-z0-9]{16,}`)},
	{"openai_key", regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`)},
	{"aws_access_key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	// Chaves estilo prefixo_ambiente_valor (Stripe, Guara, etc.): sk_live_, pk_test_,
	// gk_live_… O underscore quebra o padrão base64 abaixo, então precisa ser próprio.
	{"api_key", regexp.MustCompile(`\b[A-Za-z]{2,8}_(?:live|test)_[A-Za-z0-9]{16,}\b`)},
	{"password", regexp.MustCompile(`(?i)password\s*[=:]\s*(\S+)`)},
	{"bearer_token", regexp.MustCompile(`(?i)bearer\s+([A-Za-z0-9._-]{12,})`)},
	{"long_secret", regexp.MustCompile(`\b[A-Za-z0-9+/]{40,}={0,2}\b`)},
}

func hashOf(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

// mask preserva prefixo e sufixo curtos e mascara o miolo.
func mask(v string) string {
	if len(v) <= 8 {
		return "****"
	}
	prefix := 4
	if len(v) < 12 {
		prefix = 2
	}
	return v[:prefix] + strings.Repeat("*", 4) + v[len(v)-4:]
}

// Detect aplica os padrões e devolve credenciais únicas (por valor cru).
func Detect(text string) []Finding {
	seen := map[string]bool{}
	var out []Finding
	for _, p := range patterns {
		for _, m := range p.re.FindAllStringSubmatch(text, -1) {
			val := m[0]
			if len(m) > 1 && m[1] != "" {
				val = m[1]
			}
			val = strings.TrimSpace(val)
			if val == "" || seen[val] {
				continue
			}
			seen[val] = true
			out = append(out, Finding{Name: p.name, Masked: mask(val), Hash: hashOf(val), raw: val})
		}
	}
	return out
}

// SecretScan emite sugestões secret.detected (origem retroativa) com mascaramento
// obrigatório e respeita a supressão por hash (critério 9).
type SecretScan struct {
	store *store.Store
}

func NewSecretScan(s *store.Store) *SecretScan { return &SecretScan{store: s} }

// secretAlreadyPending evita re-emitir uma sugestão de segredo cujo hash já está
// pendente (idempotência ao re-rodar/retomar a run — critério 8). O hash mora no
// payload JSON da sugestão secret.detected.
func (sc *SecretScan) secretAlreadyPending(hash string) bool {
	var n int
	_ = sc.store.DB().QueryRow(`SELECT COUNT(*) FROM suggestions
		WHERE type='secret.detected' AND status='pending' AND payload LIKE ?`,
		"%\"hash\":\""+hash+"\"%").Scan(&n)
	return n > 0
}

// Scan varre os textos das sessões do projeto e cria sugestões mascaradas. Devolve
// o número de sugestões criadas. Valor cru NUNCA entra em evidência/payload/UI.
func (sc *SecretScan) Scan(projectID string, sessionTexts []string) (int, error) {
	emitted := map[string]bool{}
	created := 0
	for _, txt := range sessionTexts {
		for _, f := range Detect(txt) {
			if emitted[f.Hash] || sc.store.IsSecretSuppressed(f.Hash) || sc.secretAlreadyPending(f.Hash) {
				continue
			}
			emitted[f.Hash] = true
			payload, _ := json.Marshal(map[string]any{
				"mode": "value", "name": f.Name, "hash": f.Hash, "masked": f.Masked,
			})
			_, err := sc.store.CreateSuggestion(&store.Suggestion{
				ProjectID: projectID,
				Type:      "secret.detected",
				Title:     "Segredo detectado: " + f.Name,
				Evidence:  f.Masked,
				Origin:    "retroativa",
				Payload:   string(payload),
			})
			if err != nil {
				return created, err
			}
			created++
		}
	}
	return created, nil
}
