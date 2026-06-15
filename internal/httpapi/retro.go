package httpapi

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/retro"
)

func (s *Server) routesRetro() {
	if s.deps.Retro == nil {
		return
	}
	svc := s.deps.Retro

	// Lista os provedores disponíveis SEM varrer o histórico (a UI exige
	// aprovação explícita por provedor antes de qualquer scan).
	s.mux.HandleFunc("GET /api/retro/providers", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, svc.Providers())
	})

	// Estágio 0: inventário local (sem LLM). clis restringe aos provedores
	// aprovados; vazio varre todos (compat).
	s.mux.HandleFunc("POST /api/retro/inventory", func(w http.ResponseWriter, r *http.Request) {
		in, _ := decode[struct {
			WindowDays int      `json:"window_days"`
			Clis       []string `json:"clis"`
		}](r)
		since := time.Time{}
		if in.WindowDays > 0 {
			since = time.Now().AddDate(0, 0, -in.WindowDays)
		}
		rep, err := svc.Inventory(since, in.Clis)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, rep)
	})

	// Estágio 0 (assíncrono): dispara o inventário em goroutine e reporta
	// progresso REAL no bus (contagem de arquivos de histórico processados).
	// Eventos: retro.inventory.progress {cli, done, total, overall_done, overall_total}
	//          retro.inventory.done {report}
	s.mux.HandleFunc("POST /api/retro/inventory/start", func(w http.ResponseWriter, r *http.Request) {
		in, _ := decode[struct {
			WindowDays int      `json:"window_days"`
			Clis       []string `json:"clis"`
		}](r)
		since := time.Time{}
		if in.WindowDays > 0 {
			since = time.Now().AddDate(0, 0, -in.WindowDays)
		}
		clis := in.Clis
		b := s.deps.Bus
		go func() {
			var mu sync.Mutex
			type pg struct{ done, total int }
			per := map[string]*pg{}
			emit := func(cli string, done, total int) {
				if b == nil {
					return
				}
				mu.Lock()
				p := per[cli]
				if p == nil {
					p = &pg{}
					per[cli] = p
				}
				p.done, p.total = done, total
				var od, ot int
				for _, v := range per {
					od += v.done
					ot += v.total
				}
				mu.Unlock()
				b.Publish(bus.Event{Type: "retro.inventory.progress", Payload: map[string]any{
					"cli": cli, "done": done, "total": total,
					"overall_done": od, "overall_total": ot,
				}})
			}
			rep, err := svc.InventoryProgress(since, clis, emit)
			if b == nil {
				return
			}
			if err != nil {
				b.Publish(bus.Event{Type: "retro.inventory.done", Payload: map[string]any{"error": err.Error()}})
				return
			}
			b.Publish(bus.Event{Type: "retro.inventory.done", Payload: rep})
		}()
		writeJSON(w, 202, map[string]string{"status": "scanning"})
	})

	// Estágio 1: abre/reusa run para o escopo.
	s.mux.HandleFunc("POST /api/retro/runs", func(w http.ResponseWriter, r *http.Request) {
		in, err := decode[struct {
			Scope         retro.Scope `json:"scope"`
			Depth         string      `json:"depth"`
			BudgetPerHour int64       `json:"budget_per_hour"`
			BudgetTotal   int64       `json:"budget_total"`
		}](r)
		if err != nil {
			writeErr(w, 400, "corpo inválido")
			return
		}
		run, err := svc.Plan(in.Scope)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		depth := in.Depth
		if depth == "" {
			depth = "completa"
		}
		_ = s.deps.Store.SetRetroRunScope(run.ID, run.Scope, depth, in.BudgetPerHour, in.BudgetTotal)
		run, _ = svc.GetRun(run.ID)
		writeJSON(w, 201, run)
	})

	s.mux.HandleFunc("GET /api/retro/runs", func(w http.ResponseWriter, r *http.Request) {
		runs, err := svc.ListRuns()
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, runs)
	})

	s.mux.HandleFunc("GET /api/retro/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		run, err := svc.GetRun(r.PathValue("id"))
		if err != nil {
			notFoundOr500(w, err, "run não encontrada")
			return
		}
		writeJSON(w, 200, run)
	})

	// Estágio 2: clusterização (1ª LLM).
	s.mux.HandleFunc("POST /api/retro/runs/{id}/cluster", func(w http.ResponseWriter, r *http.Request) {
		if err := svc.Cluster(context.Background(), r.PathValue("id")); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		clusters, _ := svc.ListClusters(r.PathValue("id"))
		writeJSON(w, 200, clusters)
	})

	s.mux.HandleFunc("GET /api/retro/runs/{id}/clusters", func(w http.ResponseWriter, r *http.Request) {
		clusters, err := svc.ListClusters(r.PathValue("id"))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, clusters)
	})

	s.mux.HandleFunc("POST /api/retro/clusters/{id}/approve", func(w http.ResponseWriter, r *http.Request) {
		in, _ := decode[struct {
			Rename string `json:"rename"`
		}](r)
		pid, err := svc.ApproveCluster(r.PathValue("id"), in.Rename)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"project_id": pid})
	})

	s.mux.HandleFunc("POST /api/retro/clusters/{id}/discard", func(w http.ResponseWriter, r *http.Request) {
		if err := svc.DiscardCluster(r.PathValue("id")); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"status": "discarded"})
	})

	s.mux.HandleFunc("POST /api/retro/runs/{id}/merge", func(w http.ResponseWriter, r *http.Request) {
		in, err := decode[struct {
			ClusterIDs        []string `json:"cluster_ids"`
			Name              string   `json:"name"`
			ExistingProjectID string   `json:"existing_project_id"`
		}](r)
		if err != nil || len(in.ClusterIDs) == 0 {
			writeErr(w, 400, "cluster_ids obrigatório")
			return
		}
		pid, err := svc.MergeClusters(r.PathValue("id"), in.ClusterIDs, in.Name, in.ExistingProjectID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"project_id": pid})
	})

	// Execução: dispara o executor em goroutine (não bloqueia o request).
	s.mux.HandleFunc("POST /api/retro/runs/{id}/start", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		go func() { _, _ = svc.Start(context.Background(), id) }()
		writeJSON(w, 202, map[string]string{"status": "running"})
	})

	s.mux.HandleFunc("POST /api/retro/runs/{id}/resume", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		go func() { _, _ = svc.Resume(context.Background(), id) }()
		writeJSON(w, 202, map[string]string{"status": "running"})
	})

	s.mux.HandleFunc("POST /api/retro/runs/{id}/pause", func(w http.ResponseWriter, r *http.Request) {
		if err := svc.Pause(r.PathValue("id")); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"status": "paused"})
	})

	s.mux.HandleFunc("POST /api/retro/runs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		if err := svc.Cancel(r.PathValue("id")); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"status": "canceled"})
	})

	// Revisão em lote.
	s.mux.HandleFunc("GET /api/retro/runs/{id}/batch", func(w http.ResponseWriter, r *http.Request) {
		view, err := svc.BatchView(r.PathValue("id"))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, view)
	})

	s.mux.HandleFunc("POST /api/retro/runs/{id}/bulk", func(w http.ResponseWriter, r *http.Request) {
		in, err := decode[struct {
			ProjectID string `json:"project_id"`
			Type      string `json:"type"`
			Action    string `json:"action"`
		}](r)
		if err != nil || in.ProjectID == "" || in.Type == "" || in.Action == "" {
			writeErr(w, 400, "project_id, type e action obrigatórios")
			return
		}
		n, err := svc.BulkResolve(r.PathValue("id"), in.ProjectID, in.Type, in.Action)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]int{"resolved": n})
	})
}
