package retro

import (
	"context"
	"encoding/json"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// Service é a fachada única do fluxo de análise retroativa (a API HTTP só fala
// com ela). Costura inventário→plano→clusterização→aprovação→execução→consolidação.
type Service struct {
	store        *store.Store
	bus          *bus.Bus
	inv          *Inventory
	planner      *Planner
	clusterer    *Clusterer
	approver     *Approver
	executor     *Executor
	consolidator *Consolidator
	secretScan   *SecretScan
	synthesizer  *Synthesizer
}

// New costura o serviço. headless mapeia ID de adapter → cliente headless,
// permitindo override de provider por run (escolha na tela de análise retroativa);
// vazio/nil mantém o adapter fixado no boot do engine.
func New(s *store.Store, eng *distill.Engine, applier *apply.Applier, b *bus.Bus, obs []Observer, headless map[string]distill.Headless) *Service {
	imp := distill.NewImporter(s, b)
	return &Service{
		store:        s,
		bus:          b,
		inv:          NewInventory(s, obs),
		planner:      NewPlanner(s, imp, obs),
		clusterer:    NewClusterer(s, eng.HeadlessCLI(), b),
		approver:     NewApprover(s),
		executor:     NewExecutor(s, eng, b, headless),
		consolidator: NewConsolidator(s).WithApplier(applier),
		secretScan:   NewSecretScan(s),
		synthesizer:  NewSynthesizer(s, eng.HeadlessCLI(), b),
	}
}

func (svc *Service) Inventory(since time.Time) (*InventoryReport, error) {
	return svc.inv.Scan(since)
}

// InventoryProgress roda o scan reportando progresso real por CLI via emit.
func (svc *Service) InventoryProgress(since time.Time, emit func(cli string, done, total int)) (*InventoryReport, error) {
	return svc.inv.ScanProgress(since, emit)
}

// Plan abre (ou reusa) uma run para o escopo. A reutilização evita runs duplicadas
// e é parte da idempotência ponta-a-ponta (critério 8).
func (svc *Service) Plan(scope Scope) (*store.RetroRun, error) {
	if existing := svc.findOpenRun(scope); existing != nil {
		return existing, nil
	}
	return svc.planner.Plan(scope)
}

func (svc *Service) findOpenRun(scope Scope) *store.RetroRun {
	want, _ := json.Marshal(scope)
	runs, err := svc.store.ListRetroRuns()
	if err != nil {
		return nil
	}
	for _, r := range runs {
		if r.Status == "done" || r.Status == "canceled" {
			continue
		}
		if r.Scope == string(want) {
			return r
		}
	}
	return nil
}

func (svc *Service) Cluster(ctx context.Context, runID string) error {
	if err := svc.store.SetRetroRunStatus(runID, "clustering"); err != nil {
		return err
	}
	return svc.clusterer.Propose(ctx, runID)
}

func (svc *Service) ListClusters(runID string) ([]*store.RetroCluster, error) {
	return svc.store.ListRetroClusters(runID)
}

func (svc *Service) ApproveCluster(clusterID, rename string) (string, error) {
	return svc.approver.ApproveCluster(clusterID, rename)
}

func (svc *Service) MergeClusters(runID string, clusterIDs []string, name, existingProjectID string) (string, error) {
	return svc.approver.MergeClusters(runID, clusterIDs, name, existingProjectID)
}

func (svc *Service) DiscardCluster(clusterID string) error {
	return svc.approver.DiscardCluster(clusterID)
}

// Start coloca a run em running, executa a destilação orçada, varre segredos
// (modo completa) e consolida a revisão em lote.
func (svc *Service) Start(ctx context.Context, runID string) (string, error) {
	if err := svc.store.SetRetroRunStatus(runID, "running"); err != nil {
		return "", err
	}
	svc.bus.Publish(bus.Event{Type: "retro.run.started", Payload: map[string]any{"run_id": runID}})
	return svc.resume(ctx, runID)
}

// Resume retoma uma run pausada (não reprocessa — critério 4).
func (svc *Service) Resume(ctx context.Context, runID string) (string, error) {
	if err := svc.store.SetRetroRunStatus(runID, "running"); err != nil {
		return "", err
	}
	return svc.resume(ctx, runID)
}

func (svc *Service) resume(ctx context.Context, runID string) (string, error) {
	st, err := svc.executor.Run(ctx, runID)
	if err != nil {
		return "", err
	}
	if st == "done" {
		run, _ := svc.store.GetRetroRun(runID)
		if run != nil && run.Depth == "completa" {
			svc.scanSecrets(runID)
			// Síntese por projeto: costura skills/memórias fragmentadas que são
			// etapas de um mesmo workflow recorrente numa skill unificada. Só no
			// modo completa (a leve não gera skills). Roda ANTES da consolidação
			// para que a consolidação ainda possa fundir redundâncias resultantes.
			_ = svc.synthesizer.Synthesize(ctx, runID)
		}
		_ = svc.consolidator.Consolidate(runID)
	}
	return st, nil
}

func (svc *Service) scanSecrets(runID string) {
	byProject, err := svc.store.PendingRunSessionsByProject(runID)
	if err != nil {
		return
	}
	// também inclui projetos cujas sessões já foram processadas
	projects, _ := svc.consolidator.runProjects(runID)
	seen := map[string]bool{}
	for pid := range byProject {
		seen[pid] = true
	}
	for _, pid := range projects {
		seen[pid] = true
	}
	for pid := range seen {
		texts := svc.projectTexts(runID, pid)
		_, _ = svc.secretScan.Scan(pid, texts)
	}
}

// projectTexts coleta o conteúdo dos transcripts das sessões da run associadas ao projeto.
func (svc *Service) projectTexts(runID, projectID string) []string {
	rows, err := svc.store.DB().Query(`SELECT session_id FROM retro_run_sessions WHERE run_id=? AND project_id=?`, runID, projectID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var sids []string
	for rows.Next() {
		var sid string
		if err := rows.Scan(&sid); err == nil {
			sids = append(sids, sid)
		}
	}
	var texts []string
	for _, sid := range sids {
		evs, err := svc.store.ListTranscriptEvents(sid)
		if err != nil {
			continue
		}
		for _, e := range evs {
			texts = append(texts, e.Content)
		}
	}
	return texts
}

func (svc *Service) Pause(runID string) error  { return svc.executor.Pause(runID) }
func (svc *Service) Cancel(runID string) error { return svc.executor.Cancel(runID) }

func (svc *Service) ListRuns() ([]*store.RetroRun, error)    { return svc.store.ListRetroRuns() }
func (svc *Service) GetRun(id string) (*store.RetroRun, error) { return svc.store.GetRetroRun(id) }

func (svc *Service) BatchView(runID string) ([]*ProjectGroup, error) {
	return svc.consolidator.BatchView(runID)
}

func (svc *Service) BulkResolve(runID, projectID, typ, action string) (int, error) {
	return svc.consolidator.BulkResolve(runID, projectID, typ, action)
}
