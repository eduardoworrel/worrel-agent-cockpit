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
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter/codex"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter/gemini"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter/opencode"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter/pidev"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/ask"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/chat"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/handoff"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/httpapi"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mcpserver"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/retention"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/retro"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
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

	// Fase 4: motor de varredura, importador e watcher.
	// Adapters headless por ID (todos os que suportam RunHeadless): reusado para
	// (a) escolher o adapter das análises pelo setting e (b) override por run na
	// análise retroativa. pidev fica de fora enquanto RunHeadless não é suportado.
	retroHeadless := map[string]distill.Headless{cc.ID(): cc, oc.ID(): oc, gem.ID(): gem, cx.ID(): cx, pd.ID(): pd}
	// Adaptador headless das análises respeita o setting (spec §10): default
	// claude-code; outro provider quando configurado (evita gastar a quota do Claude).
	var headless distill.Headless = cc
	if h, ok := retroHeadless[st.GetSetting("headless_adapter", "claude-code")]; ok {
		headless = h
	}
	eng := distill.New(st, headless, b)
	// O cockpit lida exclusivamente com sessões iniciadas DENTRO do app (wrapper).
	// O histórico externo do usuário (todas as sessões de CLI no disco) só é tocado
	// pelo fluxo de análise retroativa (retro), explícito e opt-in por provedor —
	// NÃO há mais import automático no boot nem watcher de filesystem vasculhando
	// ~/.claude/projects. (A varredura de escopo já havia saído do boot pelo mesmo
	// motivo: evitar ações não solicitadas sobre dados que não são do app.)

	mcp.SetVault(vlt)

	// Handoff: usa o primeiro adaptador headless disponível (preferência: claude-code).
	var handoffGen *handoff.Generator
	var spawner *wrapperSpawner
	for _, adID := range []string{"claude-code", "opencode"} {
		if ad, ok := reg.Get(adID); ok && ad.Capabilities().Headless {
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

	// Applier com bus para eventos de linhagem/auto-modo; liga o auto-aplicador
	// ao engine para o modo automático opt-in (spec §6) durante o sweep.
	applier := apply.New(st, mir, b)
	eng.SetAutoApplier(applier)

	// Fase 8: serviço de análise retroativa. Observadores = adaptadores instalados
	// (claude-code, opencode), que satisfazem retro.Observer; reusa engine + applier.
	// retroHeadless (definido acima) mapeia provider→headless p/ override por run
	// (seletor na tela de análise retroativa). Observadores = adapters que leem
	// histórico (claude-code, opencode, gemini, codex).
	retroSvc := retro.New(st, eng, applier, b, []retro.Observer{cc, oc, gem, cx, pd}, retroHeadless)

	// Chat de destilação: reusa o mapa de headless por provider e o adapter do
	// boot como fallback; gera sugestões (origin=chat) sobre as sessões.
	chatSvc := chat.NewService(st, retroHeadless, headless, b)

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
		Distiller: eng,
		Vault:     vlt,
		Handoff:   handoffGen,
		Spawner:   spawner,
		Retro:     retroSvc,
		Chat:      chatSvc,
		Ask:       askBroker,
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
	opts, err := wrapper.BuildSpawnOpts(ws.store, ws.workspace, sess.ID, ws.port, "")
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
