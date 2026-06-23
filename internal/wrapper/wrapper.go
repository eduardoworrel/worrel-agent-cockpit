// Package wrapper gere sessões interativas em PTY: spawna o CmdSpec de um
// adaptador, distribui o output para assinantes WebSocket e acumula o bruto,
// repassa stdin/resize/kill, e ao sair encerra a sessão no store + bus.
package wrapper

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// maxRawBytes limita o output bruto acumulado por sessão: mantemos apenas a
// CAUDA (últimos 2MB). Subscribe (replay) e RawOutput servem essa cauda.
const maxRawBytes = 2 << 20 // 2MB

// subChanBuf é o tamanho do buffer por assinante. Se o buffer encher
// (assinante lento), o chunk é DESCARTADO para esse assinante — o loop de
// leitura do PTY nunca bloqueia.
const subChanBuf = 256

// killGraceTimeout é quanto Kill espera o grupo de processos sair após
// SIGTERM antes de escalar para SIGKILL.
const killGraceTimeout = 3 * time.Second

type session struct {
	id      string
	ptmx    *os.File
	cmd     *exec.Cmd
	cleanup func() error

	mu      sync.Mutex
	raw     []byte
	subs    map[int]chan []byte
	nextID  int
	closed  bool
	capture *ptyCapture // observação própria do stream → transcript_events
}

// Manager gere sessões PTY ativas.
type Manager struct {
	store *store.Store
	bus   *bus.Bus

	mu           sync.Mutex
	sessions     map[string]*session
	highNotified map[string]bool // sessões que já receberam session.context_high
	awaiting     map[string]bool // último estado "aguardando input" por sessão
	titled       map[string]bool // sessões cujo título já foi derivado (para de pollar)
}

// New cria um Manager com store e bus fornecidos.
func New(st *store.Store, b *bus.Bus) *Manager {
	return &Manager{store: st, bus: b, sessions: map[string]*session{}, highNotified: map[string]bool{}, awaiting: map[string]bool{}, titled: map[string]bool{}}
}

// Spawn inicia o CmdSpec num PTY e começa o loop de leitura.
//
// pty.Start define SysProcAttr.Setsid=true (necessário para o ctty), o que
// torna o filho líder de sessão com seu PRÓPRIO grupo de processos
// (pgid == pid). Kill aproveita isso para sinalizar o grupo inteiro;
// Setpgid não é usado pois conflitaria com Setsid.
func (m *Manager) Spawn(sessionID string, spec adapter.CmdSpec) error {
	m.mu.Lock()
	if _, ok := m.sessions[sessionID]; ok {
		m.mu.Unlock()
		return fmt.Errorf("sessão %s já está rodando", sessionID)
	}
	m.mu.Unlock()

	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = append(os.Environ(), spec.Env...)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		if spec.Cleanup != nil {
			_ = spec.Cleanup()
		}
		return fmt.Errorf("pty.Start: %w", err)
	}

	s := &session{
		id: sessionID, ptmx: ptmx, cmd: cmd, cleanup: spec.Cleanup,
		subs: map[int]chan []byte{},
	}
	// Captura própria do PTY: cada turno fechado vira um transcript_event
	// (Kind="pty"). É a fonte do handoff de sessões PTY — sem reler o CLI.
	s.capture = newPTYCapture(func(role, content string) {
		_ = m.store.AppendTranscriptEventRich(sessionID, role, "pty", content, "", 0, 0)
	})
	m.mu.Lock()
	m.sessions[sessionID] = s
	m.mu.Unlock()

	go m.readLoop(s)
	return nil
}

func (m *Manager) readLoop(s *session) {
	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			if s.capture != nil {
				s.capture.onOutput(chunk) // observação própria → transcript_events
			}
			s.mu.Lock()
			s.raw = append(s.raw, chunk...)
			// ring/tail buffer: mantém só os últimos maxRawBytes.
			// memmove in-place (sem realocar) — o backing array
			// estabiliza em ~maxRawBytes.
			if over := len(s.raw) - maxRawBytes; over > 0 {
				s.raw = append(s.raw[:0], s.raw[over:]...)
			}
			// fan-out não-bloqueante: assinante com buffer cheio perde
			// o chunk; o read loop do PTY nunca espera por ninguém.
			for _, ch := range s.subs {
				select {
				case ch <- chunk:
				default:
				}
			}
			s.mu.Unlock()
		}
		if err != nil {
			break
		}
	}
	m.onExit(s)
}

func (m *Manager) onExit(s *session) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	if s.capture != nil {
		s.capture.flush() // fecha turnos pendentes antes de encerrar
	}
	// cauda do output do PTY (stdout+stderr do CLI) ANTES de fechar: é a única
	// pista do porquê do encerramento — sem isto, o fim da sessão fica "sem
	// explicação" (o exit code sozinho raramente diz o motivo).
	tail := lastBytes(s.raw, exitTailBytes)
	// fecha os canais de todos os assinantes → goroutines de escrita saem.
	for id, ch := range s.subs {
		delete(s.subs, id)
		close(ch)
	}
	s.mu.Unlock()

	_ = s.ptmx.Close()
	waitErr := s.cmd.Wait()
	if s.cleanup != nil {
		_ = s.cleanup()
	}

	m.mu.Lock()
	delete(m.sessions, s.id)
	delete(m.highNotified, s.id)
	delete(m.awaiting, s.id)
	delete(m.titled, s.id)
	m.mu.Unlock()

	reason := exitReason(waitErr, tail)
	_ = m.store.EndSessionWithReason(s.id, reason)
	m.bus.Publish(bus.Event{Type: "session.ended",
		Payload: map[string]any{"id": s.id, "reason": reason}})
}

// exitTailBytes é quanto da cauda do PTY guardamos como pista do encerramento.
const exitTailBytes = 2 << 10 // 2KB

// lastBytes devolve uma cópia dos últimos n bytes de b (ou b inteiro se menor).
func lastBytes(b []byte, n int) []byte {
	if len(b) <= n {
		return append([]byte(nil), b...)
	}
	return append([]byte(nil), b[len(b)-n:]...)
}

// exitReason monta um motivo legível do encerramento a partir do erro de Wait
// (exit code / sinal) e da cauda do PTY. Saída normal (exit 0) sem cauda útil
// devolve "" — só registramos detalhe quando há algo a explicar.
func exitReason(waitErr error, tail []byte) string {
	t := strings.TrimSpace(stripANSI(string(tail)))
	var head string
	switch {
	case waitErr == nil:
		if t == "" {
			return ""
		}
		head = "CLI encerrou (exit 0)"
	default:
		if ee, ok := waitErr.(*exec.ExitError); ok {
			if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
				head = fmt.Sprintf("CLI morto por sinal %s", ws.Signal())
			} else {
				head = fmt.Sprintf("CLI saiu com código %d", ee.ExitCode())
			}
		} else {
			head = "CLI terminou: " + waitErr.Error()
		}
	}
	if t == "" {
		return head
	}
	return head + " — " + t
}

// stripANSI remove sequências de escape ANSI (cores/cursor) da cauda do PTY,
// para que o motivo gravado seja texto legível e não lixo de terminal.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b { // ESC: pula até a letra final de uma sequência CSI/OSC
			i++
			for i < len(s) && !((s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z')) {
				i++
			}
			if i < len(s) {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func (m *Manager) get(sessionID string) (*session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("sessão %s não está rodando", sessionID)
	}
	return s, nil
}

// Subscribe registra um callback de output; devolve unsubscribe.
//
// Cada assinante ganha um canal bufferizado (subChanBuf chunks) e uma
// goroutine de escrita dedicada que chama fn. O read loop do PTY faz envio
// não-bloqueante: se o buffer do assinante encher, o chunk é descartado para
// ele. O replay do bruto acumulado (cauda de até maxRawBytes) é entregue como
// primeiro chunk. O unsubscribe fecha o canal e espera a goroutine drenar.
func (m *Manager) Subscribe(sessionID string, fn func([]byte)) func() {
	s, err := m.get(sessionID)
	if err != nil {
		return func() {}
	}
	ch := make(chan []byte, subChanBuf)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return func() {}
	}
	id := s.nextID
	s.nextID++
	s.subs[id] = ch
	if len(s.raw) > 0 {
		// replay: canal recém-criado e vazio, nunca bloqueia
		ch <- append([]byte(nil), s.raw...)
	}
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for p := range ch {
			fn(p)
		}
	}()

	return func() {
		s.mu.Lock()
		if c, ok := s.subs[id]; ok {
			delete(s.subs, id)
			close(c)
		}
		s.mu.Unlock()
		<-done // espera a goroutine de escrita sair (sem leak)
	}
}

// Write envia bytes para o stdin do PTY.
func (m *Manager) Write(sessionID string, p []byte) error {
	s, err := m.get(sessionID)
	if err != nil {
		return err
	}
	if s.capture != nil {
		s.capture.onInput(p) // entrada do usuário → turno "you" no transcript
	}
	_, err = s.ptmx.Write(p)
	return err
}

// Resize redimensiona o PTY.
func (m *Manager) Resize(sessionID string, cols, rows uint16) error {
	s, err := m.get(sessionID)
	if err != nil {
		return err
	}
	return pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows})
}

// Kill termina o GRUPO de processos da sessão: SIGTERM no grupo (-pgid),
// espera até killGraceTimeout pela saída, depois SIGKILL no grupo. Como o
// spawn usa Setsid (via pty.Start), o filho é líder do próprio grupo
// (pgid == pid) — filhos do CLI morrem junto. Devolve nil se o grupo já saiu.
func (m *Manager) Kill(sessionID string) error {
	s, err := m.get(sessionID)
	if err != nil {
		return err
	}
	proc := s.cmd.Process
	if proc == nil {
		return nil
	}
	pgid := proc.Pid // Setsid → líder de sessão → pgid == pid
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
		// ESRCH: grupo já saiu — sucesso. Outro erro: melhor esforço no processo.
		if err != syscall.ESRCH {
			_ = proc.Kill()
		}
		return nil
	}
	deadline := time.Now().Add(killGraceTimeout)
	for time.Now().Before(deadline) {
		if syscall.Kill(-pgid, 0) == syscall.ESRCH {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	return nil
}

// RawOutput devolve uma cópia da cauda do output acumulado (≤ maxRawBytes).
func (m *Manager) RawOutput(sessionID string) []byte {
	s, err := m.get(sessionID)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.raw...)
}

// IsRunning informa se a sessão está ativa no manager.
func (m *Manager) IsRunning(sessionID string) bool {
	_, err := m.get(sessionID)
	return err == nil
}

// handoffThreshold lê o setting handoff_threshold_pct (default 80).
func (m *Manager) handoffThreshold() int {
	v := m.store.GetSetting("handoff_threshold_pct", "")
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return n
	}
	return 80
}

// trackContext verifica uso de contexto do adaptador e publica eventos no bus.
// Publica session.context_high no máximo uma vez por sessão (guarda em highNotified).
func (m *Manager) trackContext(sessID string, ref adapter.SessionRef, ad adapter.Adapter) {
	used, limit, ok := ad.ContextUsage(ref)
	if !ok || limit <= 0 {
		return
	}
	_ = m.store.UpdateSessionContext(sessID, int64(used), int64(limit))
	m.bus.Publish(bus.Event{Type: "session.context",
		Payload: map[string]any{"session_id": sessID, "used": used, "limit": limit}})

	threshold := m.handoffThreshold()
	m.mu.Lock()
	alreadyNotified := m.highNotified[sessID]
	m.mu.Unlock()
	if used*100/limit >= threshold && !alreadyNotified {
		m.mu.Lock()
		m.highNotified[sessID] = true
		m.mu.Unlock()
		m.bus.Publish(bus.Event{Type: "session.context_high",
			Payload: map[string]any{"session_id": sessID}})
	}
}

// maxTitleLen limita o título derivado do primeiro recado do usuário.
const maxTitleLen = 64

// deriveTitle extrai um rótulo conciso do primeiro recado do usuário: primeira
// linha não-vazia, sem marcação markdown, truncada. Devolve "" se nada útil.
// Aceita os papéis de usuário usados no store ("you", a captura do PTY) e o
// legado ("user").
func deriveTitle(events []*store.TranscriptEvent) string {
	for _, e := range events {
		if e.Role != "you" && e.Role != "user" {
			continue
		}
		for _, raw := range strings.Split(e.Content, "\n") {
			line := strings.TrimSpace(raw)
			line = strings.TrimLeft(line, "#-*> ")
			line = strings.Map(func(r rune) rune {
				if r == '`' || r == '*' || r == '_' {
					return -1
				}
				return r
			}, line)
			line = strings.TrimSpace(line)
			if len([]rune(line)) >= 4 {
				rs := []rune(line)
				if len(rs) > maxTitleLen {
					return strings.TrimSpace(string(rs[:maxTitleLen-1])) + "…"
				}
				return line
			}
		}
	}
	return ""
}

// trackTitle deriva e persiste o título da sessão a partir do primeiro recado
// do usuário no transcript — lendo NOSSOS transcript_events (a captura do PTY),
// não o arquivo do CLI. Roda a cada poll até conseguir gravar (guarda em
// titled), então publica session.titled para a UI recarregar a lista.
func (m *Manager) trackTitle(sessID string) {
	m.mu.Lock()
	done := m.titled[sessID]
	m.mu.Unlock()
	if done {
		return
	}
	events, err := m.store.ListTranscriptEvents(sessID)
	if err != nil || len(events) == 0 {
		return
	}
	title := deriveTitle(events)
	if title == "" {
		return
	}
	wrote, err := m.store.SetSessionTitleIfEmpty(sessID, title)
	if err != nil {
		return
	}
	m.mu.Lock()
	m.titled[sessID] = true // título resolvido (gravado agora ou já existente): para de pollar
	m.mu.Unlock()
	if wrote {
		m.bus.Publish(bus.Event{Type: "session.titled",
			Payload: map[string]any{"session_id": sessID, "title": title}})
	}
}

// trackAwaiting consulta se o CLI terminou um turno e aguarda o usuário (resposta
// ou confirmação), publicando session.awaiting (entra no estado) ou session.busy
// (sai do estado) APENAS nas transições — evita repetir eventos a cada poll. Só
// age se o adaptador implementa InputWaiter; sinal indisponível mantém o estado.
func (m *Manager) trackAwaiting(sessID string, ref adapter.SessionRef, ad adapter.Adapter) {
	waiter, ok := ad.(adapter.InputWaiter)
	if !ok {
		return
	}
	awaiting, ok := waiter.AwaitingInput(ref)
	if !ok {
		return
	}
	m.mu.Lock()
	prev, seen := m.awaiting[sessID]
	m.awaiting[sessID] = awaiting
	m.mu.Unlock()
	if seen && prev == awaiting {
		return // sem mudança de estado
	}
	typ := "session.busy"
	if awaiting {
		typ = "session.awaiting"
	}
	m.bus.Publish(bus.Event{Type: typ, Payload: map[string]any{"session_id": sessID}})
}

// contextPollInterval é o período do tracker de contexto por sessão.
const contextPollInterval = 10 * time.Second

// SpawnWithAdapter inicia o CmdSpec num PTY e inicia um goroutine de tracking
// de contexto: uma medição imediata e depois ContextUsage a cada
// contextPollInterval enquanto a sessão estiver rodando.
func (m *Manager) SpawnWithAdapter(sessionID string, spec adapter.CmdSpec, ad adapter.Adapter, ref adapter.SessionRef) error {
	if err := m.Spawn(sessionID, spec); err != nil {
		return err
	}
	go func() {
		m.trackContext(sessionID, ref, ad) // medição imediata no spawn
		m.trackTitle(sessionID)
		m.trackAwaiting(sessionID, ref, ad)
		// flush periódico da captura própria do PTY: fecha turnos "ai" que
		// ficaram quietos, para o handoff ver o conteúdo sem esperar o usuário.
		quiet := time.NewTicker(quietFlushInterval)
		defer quiet.Stop()
		ctxTick := time.NewTicker(contextPollInterval)
		defer ctxTick.Stop()
		for {
			select {
			case <-quiet.C:
				if !m.IsRunning(sessionID) {
					return
				}
				m.flushCapture(sessionID)
				m.trackTitle(sessionID)
			case <-ctxTick.C:
				if !m.IsRunning(sessionID) {
					return
				}
				m.trackContext(sessionID, ref, ad)
				m.trackAwaiting(sessionID, ref, ad)
			}
		}
	}()
	return nil
}

// flushCapture fecha o turno "ai" pendente da captura do PTY (flush por quietude).
func (m *Manager) flushCapture(sessionID string) {
	s, err := m.get(sessionID)
	if err != nil || s.capture == nil {
		return
	}
	s.capture.mu.Lock()
	s.capture.flushAILocked()
	s.capture.mu.Unlock()
}
