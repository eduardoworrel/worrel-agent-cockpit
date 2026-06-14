// Package retro implementa o fluxo de análise retroativa do acervo (spec v3):
// inventário local, clusterização de projetos e destilação orçada/pausável.
// REUSA distill.Engine.AnalyzeBatch, os observadores de adaptador e o vault —
// não reimplementa nenhum deles.
package retro

import (
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

// Observer é a fração observadora + ID que o inventário/clusterização precisam.
type Observer interface {
	ID() string
	DiscoverSessions(since time.Time) ([]adapter.ExternalSession, error)
	ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error)
}
