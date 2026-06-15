package claudecode

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/metasession"
)

// ProjectsRoot é configurável para testes. Se vazio, usa ~/.claude/projects.
// Campo adicionado ao Adapter da fase 3 via este arquivo (fase 4).
// Nota: a struct Adapter é declarada em claudecode.go; aqui apenas adicionamos
// o campo via um arquivo separado. Como Go não permite declarar a struct em dois
// arquivos, usamos uma abordagem diferente: guardamos o campo em observer.go
// e a struct Adapter em claudecode.go precisa ter o campo.
// Ver nota abaixo — o campo ProjectsRoot será adicionado à struct existente.

func (a *Adapter) projectsRoot() string {
	if a.ProjectsRoot != "" {
		return a.ProjectsRoot
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

// ProjectsRootOrDefault devolve o root configurado ou o default.
func (a *Adapter) ProjectsRootOrDefault() string { return a.projectsRoot() }

func (a *Adapter) DiscoverSessions(since time.Time) ([]adapter.ExternalSession, error) {
	return a.DiscoverSessionsProgress(since, nil)
}

// DiscoverSessionsProgress varre o histórico do Claude Code reportando progresso
// real. Primeiro coleta TODOS os caminhos elegíveis (WalkDir só appendando paths,
// rápido) para conhecer o total; depois parseia cada arquivo, chamando onProgress
// a cada ~50 arquivos e ao final. onProgress pode ser nil.
func (a *Adapter) DiscoverSessionsProgress(since time.Time, onProgress func(done, total int)) ([]adapter.ExternalSession, error) {
	root := a.projectsRoot()
	var paths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		// Pula transcripts de subagentes (sidechains): são runs internos do
		// agente, não sessões de trabalho do usuário — não devem virar sessão.
		if strings.Contains(path, string(os.PathSeparator)+"subagents"+string(os.PathSeparator)) {
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
	total := len(paths)
	if onProgress != nil {
		onProgress(0, total)
	}
	out := make([]adapter.ExternalSession, 0, total)
	for i, path := range paths {
		es, ok := a.scanSessionMeta(path)
		if ok {
			out = append(out, es)
		}
		if onProgress != nil && (i+1)%50 == 0 {
			onProgress(i+1, total)
		}
	}
	if onProgress != nil {
		onProgress(total, total)
	}
	return out, nil
}

// scanSessionMeta lê o jsonl uma vez para extrair sessionId, cwd, título e tempos.
func (a *Adapter) scanSessionMeta(path string) (adapter.ExternalSession, bool) {
	f, err := os.Open(path)
	if err != nil {
		return adapter.ExternalSession{}, false
	}
	defer f.Close()
	es := adapter.ExternalSession{Adapter: "claude-code"}
	firstUserText := ""
	haveFirstUser := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var ev rawEvent
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if es.ExternalRef == "" && ev.SessionID != "" {
			es.ExternalRef = ev.SessionID
		}
		if es.Dir == "" && ev.Cwd != "" {
			es.Dir = ev.Cwd
		}
		if ev.Type == "ai-title" && ev.Title != "" {
			es.Title = ev.Title
		}
		// Captura o PRIMEIRO texto de usuário para detectar meta-sessões do
		// próprio worrel (chamadas headless do destilador/clusterizador).
		if !haveFirstUser && ev.Type == "user" && ev.Message != nil {
			if text, _ := extractText(ev.Message.Content); strings.TrimSpace(text) != "" {
				firstUserText = text
				haveFirstUser = true
			}
		}
		if t, err := time.Parse(time.RFC3339, ev.Timestamp); err == nil {
			if es.StartedAt.IsZero() {
				es.StartedAt = t
			}
			es.UpdatedAt = t
		}
	}
	// Descarta meta-sessões na própria descoberta: inventário e importador veem
	// exatamente o mesmo conjunto de sessões "reais".
	if metasession.IsWorrelMeta(firstUserText) {
		return adapter.ExternalSession{}, false
	}
	if es.ExternalRef == "" {
		es.ExternalRef = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	if es.Title == "" {
		es.Title = es.ExternalRef
	}
	es.Path = path
	return es, true
}

func (a *Adapter) ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	// Resolve o caminho por external ref quando o Path não vem dado (sessões
	// in-app: o .jsonl tem o nome do session id == ExternalRef). Espelha a
	// resolução de ContextUsage.
	path := ref.Path
	if path == "" && ref.ExternalRef != "" {
		if matches, _ := filepath.Glob(filepath.Join(a.projectsRoot(), "*", ref.ExternalRef+".jsonl")); len(matches) > 0 {
			path = matches[0]
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []adapter.TranscriptEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var ev rawEvent
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if ev.Message == nil {
			continue
		}
		switch ev.Type {
		case "user", "assistant", "system":
		default:
			continue
		}
		text, kind := extractText(ev.Message.Content)
		if strings.TrimSpace(text) == "" {
			continue
		}
		te := adapter.TranscriptEvent{Role: ev.Message.Role, Kind: kind, Content: text}
		if te.Role == "" {
			te.Role = ev.Type
		}
		if ev.Message.Usage != nil {
			te.TokensIn = ev.Message.Usage.InputTokens
			te.TokensOut = ev.Message.Usage.OutputTokens
		}
		if t, err := time.Parse(time.RFC3339, ev.Timestamp); err == nil {
			te.CreatedAt = t.UnixMilli()
		}
		out = append(out, te)
	}
	return out, sc.Err()
}
