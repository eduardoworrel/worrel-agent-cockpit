package codex

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

func (a *Adapter) sessionsRoot() string {
	if a.SessionsRoot != "" {
		return a.SessionsRoot
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "sessions")
}

// isRolloutFile reconhece os arquivos de sessão (rollout-*.jsonl). Outros .jsonl
// que apareçam sob sessions/ são ignorados (não são sessões reais).
func isRolloutFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasPrefix(base, "rollout-") && strings.HasSuffix(base, ".jsonl")
}

func (a *Adapter) DiscoverSessions(since time.Time) ([]adapter.ExternalSession, error) {
	root := a.sessionsRoot()
	var paths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || !isRolloutFile(path) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if !since.IsZero() && info.ModTime().Before(since) {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]adapter.ExternalSession, 0, len(paths))
	for _, path := range paths {
		if es, ok := a.scanSessionMeta(path); ok {
			out = append(out, es)
		}
	}
	return out, nil
}

// scanSessionMeta lê o rollout uma vez para extrair id, cwd, título e tempos.
// O título é derivado da primeira mensagem do usuário (primeira linha não vazia).
func (a *Adapter) scanSessionMeta(path string) (adapter.ExternalSession, bool) {
	f, err := os.Open(path)
	if err != nil {
		return adapter.ExternalSession{}, false
	}
	defer f.Close()

	es := adapter.ExternalSession{Adapter: "codex", Path: path}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var ln rawLine
		if json.Unmarshal(sc.Bytes(), &ln) != nil {
			continue
		}
		if t, err := time.Parse(time.RFC3339, ln.Timestamp); err == nil {
			if es.StartedAt.IsZero() {
				es.StartedAt = t
			}
			es.UpdatedAt = t
		}
		switch ln.Type {
		case "session_meta":
			var mp metaPayload
			if json.Unmarshal(ln.Payload, &mp) == nil {
				if es.ExternalRef == "" && mp.ID != "" {
					es.ExternalRef = mp.ID
				}
				if es.Dir == "" && mp.Cwd != "" {
					es.Dir = mp.Cwd
				}
			}
		case "response_item":
			if es.Title != "" {
				continue
			}
			var ip itemPayload
			if json.Unmarshal(ln.Payload, &ip) != nil || ip.Type != "message" || ip.Role != "user" {
				continue
			}
			text := extractContent(ip.Content)
			// Pula mensagens de usuário sintéticas (contexto de ambiente etc.).
			if text == "" || strings.HasPrefix(text, "<") {
				continue
			}
			es.Title = firstLine(text)
		}
	}

	if es.ExternalRef == "" {
		es.ExternalRef = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	if es.Title == "" {
		es.Title = es.ExternalRef
	}
	return es, true
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	const max = 120
	if len(s) > max {
		s = s[:max]
	}
	return strings.TrimSpace(s)
}

func (a *Adapter) ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	f, err := os.Open(ref.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []adapter.TranscriptEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var ln rawLine
		if json.Unmarshal(sc.Bytes(), &ln) != nil || ln.Type != "response_item" {
			continue
		}
		var ip itemPayload
		if json.Unmarshal(ln.Payload, &ip) != nil || ip.Type != "message" {
			continue
		}
		switch ip.Role {
		case "user", "assistant":
		default:
			// developer/system são prompts internos do Codex — fora do transcript.
			continue
		}
		text := extractContent(ip.Content)
		if text == "" {
			continue
		}
		// Mensagens de usuário sintéticas (env/skills) começam com tag XML.
		if ip.Role == "user" && strings.HasPrefix(text, "<") {
			continue
		}
		te := adapter.TranscriptEvent{Role: ip.Role, Kind: "text", Content: text}
		if t, err := time.Parse(time.RFC3339, ln.Timestamp); err == nil {
			te.CreatedAt = t.UnixMilli()
		}
		out = append(out, te)
	}
	return out, sc.Err()
}

// ContextUsage lê o rollout (ref.Path) e devolve o último last_token_usage.total_tokens
// como "usado" (contexto do turno corrente; total_token_usage seria o acumulado
// da sessão, que cresce sem teto) e o último model_context_window como limite.
// Ambos são gravados a cada turno, então o último reflete o estado mais recente.
func (a *Adapter) ContextUsage(ref adapter.SessionRef) (used, limit int, ok bool) {
	if ref.Path == "" {
		return 0, 0, false
	}
	f, err := os.Open(ref.Path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()

	var lastTotal int
	var window int
	found := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var ln rawLine
		if json.Unmarshal(sc.Bytes(), &ln) != nil {
			continue
		}
		// token_count e task_started aparecem tanto como type de topo quanto
		// dentro de event_msg (payload.type). Parseamos o payload e olhamos
		// ambos os rótulos.
		var ip itemPayload
		if json.Unmarshal(ln.Payload, &ip) != nil {
			continue
		}
		if ln.Type == "token_count" || ip.Type == "token_count" {
			// last_token_usage é o uso do ÚLTIMO turno = contexto ocupado AGORA
			// (input vivo + output). total_token_usage é cumulativo da sessão
			// inteira (cresce sem teto, inútil p/ janela). Por isso usamos last.
			if ip.Info != nil && ip.Info.LastTokenUsage != nil {
				lastTotal = int(ip.Info.LastTokenUsage.TotalTokens)
				found = true
			}
		}
		if (ln.Type == "task_started" || ip.Type == "task_started") && ip.ModelContextWindow > 0 {
			window = ip.ModelContextWindow
		}
	}
	if !found {
		return 0, 0, false
	}
	if window <= 0 {
		window = contextWindowTokens
	}
	return lastTotal, window, true
}
