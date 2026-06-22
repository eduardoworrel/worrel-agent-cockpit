package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter/claudecode"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/engine"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/engine/friction"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/engine/memory"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/engine/skill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter/codex"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter/gemini"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter/opencode"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter/pidev"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/ask"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/handoff"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/httpapi"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mcpserver"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/retention"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/scheduler"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/streamengine"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/vault"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/workspace"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/wrapper"
)

// version é estampada no build via -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "hook" {
		runHookCmd(os.Args[2:])
		return
	}
	addr := flag.String("addr", "127.0.0.1:7717", "endereço de escuta")
	dataDir := flag.String("data", defaultDataDir(), "diretório de dados (~/.worrel)")
	portFlag := flag.Int("port", 0, "porta (atalho; sobrepõe a porta de --addr)")
	noOpen := flag.Bool("no-open", false, "não abrir o navegador automaticamente")
	showVersion := flag.Bool("version", false, "imprime a versão e sai")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	// WORREL_CLAUDE_PROJECTS overrides the default ~/.claude/projects root (fase 4 E2E)
	claudeProjectsRoot := os.Getenv("WORREL_CLAUDE_PROJECTS")

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(filepath.Join(*dataDir, "worrel.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()
	st.SetDataDir(*dataDir)

	wsManager := workspace.New(*dataDir)

	jan := retention.New(st)
	// context.Background() por ora — a costura via ctx permite cancelamento limpo no futuro.
	go runJanitor(context.Background(), jan)

	masterPass := os.Getenv("WORREL_MASTER_PASSWORD")
	vlt, err := vault.Open(st, masterPass)
	if err != nil {
		log.Fatalf("cofre: %v", err)
	}

	mir := mirror.New(*dataDir)
	b := bus.New()
	mcp := mcpserver.New(st, b)
	askBroker := ask.New()
	mcp.WithAskBroker(askBroker)

	cc := &claudecode.Adapter{ProjectsRoot: claudeProjectsRoot}
	oc := &opencode.Adapter{}
	gem := gemini.New()
	cx := codex.New()
	pd := pidev.New()
	reg := adapter.NewRegistry()
	reg.Register(cc)
	reg.Register(oc)
	reg.Register(gem)
	reg.Register(cx)
	reg.Register(pd)
	wm := wrapper.New(st, b)

	// Reconciliação de boot: nenhum PTY de wrapper sobrevive a um restart do
	// processo, então toda sessão wrapper ainda active no banco é órfã. Encerra-as
	// para que não reapareçam na faixa de abas como sessões mortas a re-encerrar.
	if n, err := st.EndOrphanedWrapperSessions(); err != nil {
		log.Printf("reconciliação de sessões órfãs falhou: %v", err)
	} else if n > 0 {
		log.Printf("reconciliação de boot: %d sessão(ões) wrapper órfã(s) encerrada(s)", n)
	}

	// endereço de escuta: --port (se setado) sobrepõe a porta de --addr.
	listenAddr := *addr
	if *portFlag != 0 {
		if host, _, err := net.SplitHostPort(*addr); err == nil {
			listenAddr = net.JoinHostPort(host, strconv.Itoa(*portFlag))
		}
	}
	ln, err := listenWithFallback(listenAddr, 20)
	if err != nil {
		log.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	mcp.SetVault(vlt)

	// Handoff: usa o primeiro adaptador headless disponível (preferência: claude-code).
	var handoffGen *handoff.Generator
	var spawner *wrapperSpawner
	var headlessAdapter adapter.Adapter // mesmo adapter headless serve o resumo da Home
	for _, adID := range []string{"claude-code", "opencode"} {
		if ad, ok := reg.Get(adID); ok && ad.Capabilities().Headless {
			headlessAdapter = ad
			sum := handoff.NewAdapterSummarizer(ad)
			// O mesmo adaptador lê o transcript ao vivo do .jsonl da sessão in-app
			// (que não é ingerida em transcript_events) — sem isso o resumo de
			// handoff de uma sessão viva sairia vazio.
			handoffGen = handoff.New(st, sum).WithLiveReader(ad)
			mcp.WithSummaryGenerator(handoffGen)
			spawner = &wrapperSpawner{store: st, wrapper: wm, workspace: wsManager, adapter: ad, reg: reg, port: port}
			break
		}
	}

	// Motor stream-json: dirige sessões 100% pelo protocolo do CLI (sem PTY/ask/
	// hook). onChange publica interaction.changed; ao encerrar, fecha a sessão no
	// store para sair da faixa de vivas da Home.
	var engineMgr *streamengine.Manager
	engineMgr = streamengine.NewManager(func(id string) {
		b.Publish(bus.Event{Type: "interaction.changed", Payload: map[string]any{"session_id": id}})
		if snap, ok := engineMgr.Snapshot(id); ok && snap.State == agui.StateEnded {
			_ = st.EndSession(id)
			b.Publish(bus.Event{Type: "session.ended", Payload: map[string]any{"id": id}})
		}
	}, func(id, role, text string) {
		// Persiste cada linha do chat (kind="history") para o transcript
		// sobreviver ao restart: na volta, agui.Build reconstrói o histórico
		// a partir desses eventos quando a sessão não está mais viva na memória.
		_ = st.AppendTranscriptEvent(id, role, "history", text, 0, 0)
	})

	// Applier manual com bus para eventos de linhagem.
	applier := apply.New(st, mir, b)

	engines := engine.NewRegistry()
	engines.Register(memory.New(cc).WithRegistry(reg))
	engines.Register(skill.New(cc).WithRegistry(reg))
	engines.Register(friction.New(cc).WithRegistry(reg))

	// Scheduler: dispara motores HABILITADOS sobre sessões encerradas (uma vez
	// cada). Nada roda por default — só motores com __enabled=true na config.
	go scheduler.New(engines, st).Start(context.Background(), 2*time.Minute)

	srv := httpapi.New(httpapi.Deps{
		Store:     st,
		Mirror:    mir,
		Bus:       b,
		Applier:   applier,
		MCP:       mcp.HTTPHandler(),
		Wrapper:   wm,
		Workspace: wsManager,
		Adapters:  reg,
		Port:      port,
		Vault:     vlt,
		Handoff:   handoffGen,
		Spawner:   spawner,
		Ask:       askBroker,
		Engines:   engines,
		Summarizer: headlessAdapter,
		Engine:    engineMgr,
	})

	url := fmt.Sprintf("http://%s", ln.Addr().String())
	log.Printf("worrel ouvindo em %s (dados em %s)", url, *dataDir)
	if !*noOpen {
		go openBrowser(url)
	}
	log.Fatal(http.Serve(ln, srv.Handler()))
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".worrel"
	}
	return filepath.Join(home, ".worrel")
}

// runJanitor varre transcripts expirados no start e a cada 6h (spec §11);
// encerra quando ctx é cancelado.
func runJanitor(ctx context.Context, j *retention.Janitor) {
	sweep := func() {
		n, err := j.Sweep()
		if err != nil {
			log.Printf("retention: erro na varredura: %v", err)
			return
		}
		if n > 0 {
			log.Printf("retention: %d transcript(s) podado(s)", n)
		}
	}
	sweep()
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sweep()
		}
	}
}

// wrapperSpawner implementa httpapi.Spawner usando wrapper.Manager.
type wrapperSpawner struct {
	store     *store.Store
	wrapper   *wrapper.Manager
	workspace *workspace.Manager
	adapter   adapter.Adapter
	reg       *adapter.Registry
	port      int
}

// Spawn cria uma nova sessão wrapper no projeto com o primer e o link continues.
func (ws *wrapperSpawner) Spawn(projectID, primer, continues string) (string, error) {
	sess, err := ws.store.CreateSession(&store.Session{
		ProjectID: projectID,
		Adapter:   ws.adapter.ID(),
		Mode:      "wrapper",
		Continues: &continues,
	})
	if err != nil {
		return "", err
	}
	// Monta SpawnOpts a partir do store (memória + MCP token), depois sobrescreve o primer.
	opts, err := wrapper.BuildSpawnOpts(ws.store, ws.workspace, sess.ID, ws.port, "", "")
	if err != nil {
		return "", err
	}
	// Substituir o primer pelo handoff primer (memória + resumo + skills).
	if primer != "" {
		opts.Primer = primer
	}
	spec, err := ws.adapter.BuildInteractive(opts)
	if err != nil {
		return "", err
	}
	if err := ws.wrapper.Spawn(sess.ID, spec); err != nil {
		_ = ws.store.EndSession(sess.ID)
		return "", err
	}
	return sess.ID, nil
}
