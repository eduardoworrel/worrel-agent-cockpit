package wrapper

import (
	"strings"
	"sync"
	"time"
)

// ptyCapture observa o stream de um PTY (entrada do usuário + saída do agente)
// e o persiste em transcript_events como dados PRÓPRIOS do worrel — em vez de
// reler o arquivo do CLI (a "invasão" que o P2 elimina).
//
// Fontes:
//   - entrada do usuário: Manager.Write (o que a UI manda ao stdin) → role "you".
//   - saída do agente: o readLoop do PTY → role "ai".
//
// Heurística de turno (a saída do PTY ecoa o que o usuário digita, então não dá
// para separar papéis só pela saída): acumulamos a entrada do usuário e só
// fechamos um turno "you" quando o usuário submete (CR/LF), com os controles/ANSI
// removidos. A saída do agente vira um turno "ai" fechado na próxima submissão
// do usuário (troca de direção) ou no flush final ao encerrar a sessão.
//
// A persistência é APENDE-APENAS: cada turno fechado é gravado uma única vez via
// AppendTranscriptEventRich (Kind="pty"), espelhando a disciplina de ingestTranscript
// para que reentrada/flush nunca dupliquem.
type ptyCapture struct {
	mu sync.Mutex

	userBuf strings.Builder // entrada do usuário ainda não submetida
	aiBuf   strings.Builder // saída do agente desde o último turno "you"

	persist func(role, content string) // grava um turno fechado no store
}

func newPTYCapture(persist func(role, content string)) *ptyCapture {
	return &ptyCapture{persist: persist}
}

// onOutput recebe um chunk da saída do agente (stdout/stderr do CLI via PTY).
func (c *ptyCapture) onOutput(chunk []byte) {
	if len(chunk) == 0 {
		return
	}
	clean := stripTerminal(string(chunk))
	if clean == "" {
		return
	}
	c.mu.Lock()
	c.aiBuf.WriteString(clean)
	c.mu.Unlock()
}

// onInput recebe os bytes que a UI enviou ao stdin do PTY. Cada submissão
// (CR/LF) fecha um turno do usuário; antes disso, fecha o turno "ai" acumulado
// (a fala do agente terminou quando o usuário responde).
func (c *ptyCapture) onInput(p []byte) {
	if len(p) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := 0; i < len(p); {
		b := p[i]
		switch {
		case b == '\r' || b == '\n':
			c.flushUserLocked()
			i++
		case b == 0x1b: // ESC: pula a sequência inteira (setas, etc.) — não é texto
			i++
			if i < len(p) && p[i] == '[' {
				i++
				for i < len(p) && !((p[i] >= 'A' && p[i] <= 'Z') || (p[i] >= 'a' && p[i] <= 'z')) {
					i++
				}
				if i < len(p) {
					i++
				}
			} else if i < len(p) {
				i++
			}
		case b == 0x7f || b == 0x08: // backspace/del: remove o último byte acumulado
			s := c.userBuf.String()
			if len(s) > 0 {
				c.userBuf.Reset()
				c.userBuf.WriteString(s[:len(s)-1])
			}
			i++
		default:
			// só imprimíveis / utf-8 entram no buffer da fala do usuário;
			// outros controles são ignorados — o objetivo é a frase digitada.
			if b >= 0x20 {
				c.userBuf.WriteByte(b)
			}
			i++
		}
	}
}

// flushUserLocked fecha o turno do usuário pendente (se houver conteúdo útil):
// primeiro fecha a fala do agente acumulada (turno "ai"), depois grava o "you".
// chamador deve segurar c.mu.
func (c *ptyCapture) flushUserLocked() {
	user := strings.TrimSpace(c.userBuf.String())
	c.userBuf.Reset()
	if user == "" {
		return
	}
	c.flushAILocked()
	c.persist("you", user)
}

// flushAILocked grava a fala do agente acumulada como turno "ai" (se útil).
// chamador deve segurar c.mu.
func (c *ptyCapture) flushAILocked() {
	ai := strings.TrimSpace(c.aiBuf.String())
	c.aiBuf.Reset()
	if ai == "" {
		return
	}
	c.persist("ai", ai)
}

// flush fecha quaisquer turnos pendentes (usado no encerramento da sessão).
func (c *ptyCapture) flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flushUserLocked() // fecha "you" pendente (que também fecha "ai" antes)
	c.flushAILocked()   // fecha "ai" pendente quando não houve último "you"
}

// quietFlushInterval é o intervalo do flush periódico da fala do agente: se o
// agente produziu saída e ficou quieto, fechamos o turno "ai" para que o handoff
// veja o conteúdo mesmo sem o usuário ter respondido ainda.
const quietFlushInterval = 5 * time.Second

// stripTerminal remove sequências de escape ANSI/controle do texto do PTY,
// preservando \n e o texto legível. É mais completa que stripANSI (que serve a
// caudas curtas de exit): trata CSI, OSC, e descarta caracteres de controle
// soltos exceto a quebra de linha.
func stripTerminal(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == 0x1b: // ESC
			i++
			if i >= len(s) {
				break
			}
			switch s[i] {
			case '[': // CSI: ESC [ … letra-final
				i++
				for i < len(s) && !((s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z')) {
					i++
				}
				if i < len(s) {
					i++
				}
			case ']': // OSC: ESC ] … BEL ou ESC \
				i++
				for i < len(s) {
					if s[i] == 0x07 { // BEL
						i++
						break
					}
					if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' { // ST
						i += 2
						break
					}
					i++
				}
			default:
				i++ // escape de um caractere (ex.: ESC ( B)
			}
		case c == '\r':
			i++ // CR isolado: descartado (a saída do PTY usa \n para linha)
		case c == '\n':
			b.WriteByte('\n')
			i++
		case c == '\t':
			b.WriteByte(' ')
			i++
		case c < 0x20: // outros controles (BEL, backspace de render, etc.)
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}
