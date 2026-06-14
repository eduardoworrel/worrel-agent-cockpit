package distill

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// Assinaturas dos prompts headless do PRÓPRIO worrel (destilador e clusterizador).
// Sessões cujo primeiro prompt de usuário começa por uma destas são chamadas
// internas do app e NÃO devem ser re-importadas como trabalho do usuário —
// caso contrário o worrel "come o próprio rabo" (gera skills sobre o próprio prompt).
// Mantidas em sincronia com os literais de internal/distill/prompt.go e
// internal/retro/cluster.go (que NÃO devem ser editados aqui).
const (
	metaSigDistiller = "Você é um destilador de conhecimento."
	metaSigClusterer = "Você organiza um histórico de sessões de codificação em projetos."
)

// metaHeadWindow limita a busca da assinatura ao INÍCIO do primeiro evento:
// curto o bastante para não pegar conversas reais que só citam o texto mais
// adiante, largo o bastante para o preâmbulo <local-command-caveat> (~230 chars)
// que o Claude Code injeta antes do prompt headless.
const metaHeadWindow = 1000

// isWorrelMetaSession reporta se o primeiro texto de usuário da sessão é uma
// invocação headless do próprio worrel. Robusto a dois embrulhos observados:
// OpenCode persiste o prompt entre aspas (`"Você é um destilador...`) e o Claude
// Code prefixa um bloco <local-command-caveat>…</local-command-caveat>. Por isso
// procuramos a assinatura numa janela curta do início (não prefixo exato, nem o
// diálogo todo — que descartaria conversas reais que apenas citam o texto).
func isWorrelMetaSession(firstUserText string) bool {
	head := firstUserText
	if len(head) > metaHeadWindow {
		head = head[:metaHeadWindow]
	}
	return strings.Contains(head, metaSigDistiller) || strings.Contains(head, metaSigClusterer)
}

// firstUserText devolve o conteúdo do primeiro evento de papel "user".
func firstUserText(evs []adapter.TranscriptEvent) string {
	for _, e := range evs {
		if e.Role == "user" {
			return e.Content
		}
	}
	return ""
}

// Observer é a fração observadora da interface de adaptador.
type Observer interface {
	DiscoverSessions(since time.Time) ([]adapter.ExternalSession, error)
	ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error)
}

type Importer struct {
	store *store.Store
	bus   *bus.Bus
}

func NewImporter(s *store.Store, b *bus.Bus) *Importer { return &Importer{store: s, bus: b} }

// Import descobre sessões externas, pula as já conhecidas (inclui wrappers),
// importa transcript, associa/sugere projeto. Retorna nº de sessões importadas.
func (imp *Importer) Import(obs Observer) (int, error) {
	ext, err := obs.DiscoverSessions(time.Time{})
	if err != nil {
		return 0, err
	}
	count := 0
	for _, es := range ext {
		if imp.known(es.ExternalRef) {
			continue
		}
		// Lê o transcript antes de materializar a sessão: barato e permite
		// detectar/descartar chamadas headless do próprio worrel pelo 1º prompt.
		evs, rtErr := obs.ReadTranscript(adapter.SessionRef{
			Adapter: es.Adapter, ExternalRef: es.ExternalRef, Path: es.Path,
		})
		if rtErr == nil && isWorrelMetaSession(firstUserText(evs)) {
			// meta-sessão do worrel: não cria store.Session.
			continue
		}
		projectID := imp.resolveProject(es.Dir)
		ref := es.ExternalRef
		sess, err := imp.store.CreateSession(&store.Session{
			ProjectID: projectID, Adapter: es.Adapter, ExternalRef: &ref,
			Mode: "observed", Title: es.Title, Status: "observed", SourceDir: es.Dir,
		})
		if err != nil {
			return count, err
		}
		if rtErr == nil {
			for _, e := range evs {
				_ = imp.store.AppendTranscriptEvent(sess.ID, e.Role, e.Kind, e.Content, e.TokensIn, e.TokensOut)
			}
		}
		// encerra a sessão observada p/ entrar na fila de varredura
		_ = imp.store.EndSession(sess.ID)
		if imp.bus != nil {
			imp.bus.Publish(bus.Event{Type: "session.imported", Payload: map[string]any{"id": sess.ID, "adapter": es.Adapter}})
		}
		count++
	}
	return count, nil
}

func (imp *Importer) known(externalRef string) bool {
	var n int
	_ = imp.store.DB().QueryRow(`SELECT count(*) FROM sessions WHERE external_ref=?`, externalRef).Scan(&n)
	return n > 0
}

func (imp *Importer) resolveProject(dir string) string {
	if dir == "" {
		return ""
	}
	p, err := imp.store.ProjectByDir(dir)
	if err != nil {
		return ""
	}
	return p.ID
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
