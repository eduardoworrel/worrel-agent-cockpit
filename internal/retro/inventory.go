package retro

import (
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type CLIInventory struct {
	Sessions     int   `json:"sessions"`
	AlreadyKnown int   `json:"already_known"`
	OldestMs     int64 `json:"oldest_ms"`
	NewestMs     int64 `json:"newest_ms"`
}

type FolderGroup struct {
	Dir      string `json:"dir"`
	Sessions int    `json:"sessions"`
	CLI      string `json:"cli"`
}

type InventoryReport struct {
	PerCLI               map[string]*CLIInventory `json:"per_cli"`
	Folders              []FolderGroup            `json:"folders"`
	EstimatedInvocations int                      `json:"estimated_invocations"` // escopo padrão = sessões NÃO conhecidas
}

// Inventory NÃO possui Headless: por construção não há como chamar LLM (critério 1).
type Inventory struct {
	store *store.Store
	obs   []Observer
}

func NewInventory(s *store.Store, obs []Observer) *Inventory { return &Inventory{store: s, obs: obs} }

// progressObserver é o observador capaz de reportar progresso granular do scan.
type progressObserver interface {
	DiscoverSessionsProgress(since time.Time, onProgress func(done, total int)) ([]adapter.ExternalSession, error)
}

// Scan varre os históricos a partir de `since` (zero = tudo). 100% local.
func (in *Inventory) Scan(since time.Time) (*InventoryReport, error) {
	return in.ScanProgress(since, nil)
}

// ScanProgress é como Scan mas reporta progresso real por CLI. `emit` (se não-nil)
// é chamado durante o scan com (cli, done, total) contando ARQUIVOS de histórico
// processados naquele observador. Cai no caminho síncrono quando o observador não
// implementa progressObserver.
func (in *Inventory) ScanProgress(since time.Time, emit func(cli string, done, total int)) (*InventoryReport, error) {
	rep := &InventoryReport{PerCLI: map[string]*CLIInventory{}}
	folderIdx := map[string]*FolderGroup{}
	for _, o := range in.obs {
		var ext []adapter.ExternalSession
		var err error
		if po, ok := o.(progressObserver); ok && emit != nil {
			cli := o.ID()
			ext, err = po.DiscoverSessionsProgress(since, func(done, total int) {
				emit(cli, done, total)
			})
		} else {
			ext, err = o.DiscoverSessions(since)
		}
		if err != nil {
			return nil, err
		}
		ci := &CLIInventory{}
		for _, es := range ext {
			ci.Sessions++
			ms := es.UpdatedAt.UnixMilli()
			if ci.OldestMs == 0 || ms < ci.OldestMs {
				ci.OldestMs = ms
			}
			if ms > ci.NewestMs {
				ci.NewestMs = ms
			}
			if in.known(es.ExternalRef) {
				ci.AlreadyKnown++
			}
			// Estimativa = sessões que o run AINDA vai processar = não analisadas
			// (analyzed_at NULL), importadas ou não. Antes contava só "não
			// importadas", então zerava assim que o import rodava (bug "sempre 0").
			if !in.analyzed(es.ExternalRef) {
				rep.EstimatedInvocations++
			}
			if es.Dir != "" {
				key := o.ID() + "\x00" + es.Dir
				g := folderIdx[key]
				if g == nil {
					g = &FolderGroup{Dir: es.Dir, CLI: o.ID()}
					folderIdx[key] = g
				}
				g.Sessions++
			}
		}
		rep.PerCLI[o.ID()] = ci
	}
	for _, g := range folderIdx {
		rep.Folders = append(rep.Folders, *g)
	}
	return rep, nil
}

func (in *Inventory) known(externalRef string) bool {
	var n int
	_ = in.store.DB().QueryRow(`SELECT count(*) FROM sessions WHERE external_ref=?`, externalRef).Scan(&n)
	return n > 0
}

// analyzed reporta se a sessão já foi DESTILADA (analyzed_at preenchido). Base
// da estimativa: o run só processa sessões ainda não analisadas.
func (in *Inventory) analyzed(externalRef string) bool {
	var n int
	_ = in.store.DB().QueryRow(
		`SELECT count(*) FROM sessions WHERE external_ref=? AND analyzed_at IS NOT NULL`,
		externalRef).Scan(&n)
	return n > 0
}
