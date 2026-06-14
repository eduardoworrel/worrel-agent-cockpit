package gemini

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

// logEntry espelha um item de logs.json do Gemini CLI (LogEntry). Só mensagens
// do usuário são registradas aqui (type == "user").
type logEntry struct {
	SessionID string `json:"sessionId"`
	MessageID int    `json:"messageId"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Message   string `json:"message"`
}

// genaiContent é o formato @google/genai usado em chats/*.json e
// checkpoints/checkpoint-*.json: {role:"user"|"model", parts:[{text}]}.
type genaiContent struct {
	Role  string `json:"role"`
	Parts []struct {
		Text string `json:"text"`
	} `json:"parts"`
}

// checkpointFile é o envelope salvo por `/chat save` e checkpoints:
// {"history":[genaiContent...], "authType":"..."}. chats/*.json podem ser o
// envelope OU um array nu de genaiContent — tratamos os dois.
type checkpointFile struct {
	History []genaiContent `json:"history"`
}

func (a *Adapter) tmpRoot() string {
	if a.TmpRoot != "" {
		return a.TmpRoot
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gemini", "tmp")
}

// DiscoverSessions varre ~/.gemini/tmp/<id>/ por projeto. Cada dir vira UMA
// sessão (o Gemini CLI agrega histórico por projeto em logs.json). cwd vem do
// marcador .project_root; tempos vêm do logs.json (ou mtime do dir).
func (a *Adapter) DiscoverSessions(since time.Time) ([]adapter.ExternalSession, error) {
	root := a.tmpRoot()
	dirs, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []adapter.ExternalSession
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join(root, d.Name())
		info, err := d.Info()
		if err != nil {
			continue
		}
		if !since.IsZero() && info.ModTime().Before(since) {
			continue
		}
		es, ok := a.scanProjectDir(dir, d.Name())
		if ok {
			out = append(out, es)
		}
	}
	return out, nil
}

// scanProjectDir monta a ExternalSession de um diretório de projeto do Gemini.
func (a *Adapter) scanProjectDir(dir, id string) (adapter.ExternalSession, bool) {
	es := adapter.ExternalSession{Adapter: "gemini", ExternalRef: id, Path: dir}

	if b, err := os.ReadFile(filepath.Join(dir, ".project_root")); err == nil {
		es.Dir = strings.TrimSpace(string(b))
	}

	if entries := readLogs(filepath.Join(dir, "logs.json")); len(entries) > 0 {
		if t, err := time.Parse(time.RFC3339, entries[0].Timestamp); err == nil {
			es.StartedAt = t
		}
		if t, err := time.Parse(time.RFC3339, entries[len(entries)-1].Timestamp); err == nil {
			es.UpdatedAt = t
		}
		// Primeira mensagem do usuário como título (truncada).
		es.Title = truncate(entries[0].Message, 80)
	}
	if es.UpdatedAt.IsZero() {
		if info, err := os.Stat(dir); err == nil {
			es.UpdatedAt = info.ModTime()
		}
	}
	if es.Title == "" {
		if es.Dir != "" {
			es.Title = filepath.Base(es.Dir)
		} else {
			es.Title = id
		}
	}
	return es, true
}

func readLogs(path string) []logEntry {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var entries []logEntry
	if json.Unmarshal(b, &entries) != nil {
		return nil
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].MessageID < entries[j].MessageID })
	return entries
}

// ReadTranscript prefere o histórico rico (chats/*.json + checkpoints/
// checkpoint-*.json, formato @google/genai com respostas do modelo). Se não
// houver nenhum, degrada para logs.json (só prompts do usuário). ref.Path é o
// diretório do projeto (ExternalSession.Path).
func (a *Adapter) ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	dir := ref.Path
	if dir == "" {
		return nil, adapter.ErrNotSupported
	}

	// 1) histórico rico: o arquivo de chat/checkpoint mais recente.
	if path := newestHistoryFile(dir); path != "" {
		if hist, ok := readGenaiHistory(path); ok {
			out := make([]adapter.TranscriptEvent, 0, len(hist))
			for _, c := range hist {
				text := genaiText(c)
				if strings.TrimSpace(text) == "" {
					continue
				}
				out = append(out, adapter.TranscriptEvent{
					Role:    normRole(c.Role),
					Kind:    "text",
					Content: text,
				})
			}
			if len(out) > 0 {
				return out, nil
			}
		}
	}

	// 2) degradação: logs.json (somente prompts do usuário).
	entries := readLogs(filepath.Join(dir, "logs.json"))
	if len(entries) == 0 {
		return nil, nil
	}
	out := make([]adapter.TranscriptEvent, 0, len(entries))
	for _, e := range entries {
		te := adapter.TranscriptEvent{Role: "user", Kind: "text", Content: e.Message}
		if t, err := time.Parse(time.RFC3339, e.Timestamp); err == nil {
			te.CreatedAt = t.UnixMilli()
		}
		out = append(out, te)
	}
	return out, nil
}

// newestHistoryFile devolve o caminho do arquivo de histórico mais recente
// (chats/*.json ou checkpoints/checkpoint-*.json), ou "".
func newestHistoryFile(dir string) string {
	candidates := []string{}
	for _, sub := range []string{"chats", "checkpoints"} {
		matches, _ := filepath.Glob(filepath.Join(dir, sub, "*.json"))
		candidates = append(candidates, matches...)
	}
	best, bestT := "", time.Time{}
	for _, p := range candidates {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestT) {
			best, bestT = p, info.ModTime()
		}
	}
	return best
}

// readGenaiHistory lê um arquivo de histórico, aceitando tanto o envelope
// {"history":[...]} quanto um array nu de genaiContent.
func readGenaiHistory(path string) ([]genaiContent, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var env checkpointFile
	if json.Unmarshal(b, &env) == nil && len(env.History) > 0 {
		return env.History, true
	}
	var arr []genaiContent
	if json.Unmarshal(b, &arr) == nil && len(arr) > 0 {
		return arr, true
	}
	return nil, false
}

func genaiText(c genaiContent) string {
	var parts []string
	for _, p := range c.Parts {
		if p.Text != "" {
			parts = append(parts, p.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func normRole(role string) string {
	if role == "model" {
		return "assistant"
	}
	return role
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ContextUsage: o Gemini CLI não persiste totais de tokens de forma estável nos
// arquivos de histórico (chats/checkpoints guardam Content[], sem usage; logs.json
// só tem prompts). Degradação graciosa (spec §4): ok=false. O handoff manual
// continua disponível; medição via `--output-format json` (stats.models.tokens)
// só existe em runs headless, não na sessão interativa.
func (a *Adapter) ContextUsage(ref adapter.SessionRef) (used, limit int, ok bool) {
	return 0, 0, false
}
