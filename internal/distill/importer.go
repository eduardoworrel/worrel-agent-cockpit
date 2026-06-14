package distill

import (
	"encoding/json"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/metasession"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

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
		// Defesa idempotente: a descoberta já filtra meta-sessões, mas mantemos
		// a checagem aqui caso o observador não tenha como inspecionar o 1º prompt.
		if rtErr == nil && metasession.IsWorrelMeta(firstUserText(evs)) {
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
