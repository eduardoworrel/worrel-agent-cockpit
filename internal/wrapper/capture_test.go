package wrapper

import (
	"reflect"
	"testing"
)

type turn struct {
	role, content string
}

// collect cria uma captura que acumula os turnos fechados em memória.
func collect(turns *[]turn) *ptyCapture {
	return newPTYCapture(func(role, content string) {
		*turns = append(*turns, turn{role, content})
	})
}

func TestCaptureStripsANSI(t *testing.T) {
	var turns []turn
	c := collect(&turns)
	// saída do agente com cores ANSI, OSC (título) e CR de render
	c.onOutput([]byte("\x1b]0;titulo\x07\x1b[32mOlá\x1b[0m mundo\r\n"))
	c.flush()
	if len(turns) != 1 || turns[0].role != "ai" {
		t.Fatalf("turnos = %+v", turns)
	}
	if turns[0].content != "Olá mundo" {
		t.Fatalf("ANSI não removido: %q", turns[0].content)
	}
}

func TestCaptureRoleSegmentation(t *testing.T) {
	var turns []turn
	c := collect(&turns)
	c.onOutput([]byte("Bem-vindo ao CLI\n"))   // agente fala
	c.onInput([]byte("corrigir o bug\r"))      // usuário submete
	c.onOutput([]byte("Corrigindo agora...\n")) // agente responde
	c.onInput([]byte("obrigado\n"))            // usuário submete de novo
	c.flush()

	want := []turn{
		{"ai", "Bem-vindo ao CLI"},
		{"you", "corrigir o bug"},
		{"ai", "Corrigindo agora..."},
		{"you", "obrigado"},
	}
	if !reflect.DeepEqual(turns, want) {
		t.Fatalf("segmentação\n got=%+v\nquer=%+v", turns, want)
	}
}

func TestCaptureIgnoresControlKeys(t *testing.T) {
	var turns []turn
	c := collect(&turns)
	// teclado bruto: setas (ESC[C), backspace (0x7f) corrigindo um erro
	c.onInput([]byte("teste\x7f\x7fxto\x1b[D")) // "teste"→2×backspace→"tes"+"xto"; ESC[D ignorado
	c.onInput([]byte("\r"))
	c.flush()
	if len(turns) != 1 || turns[0].role != "you" {
		t.Fatalf("turnos = %+v", turns)
	}
	// backspaces removem 'te' → 'tes', + 'xto' = 'tesxto'; a seta ESC[D some
	if turns[0].content != "tesxto" {
		t.Fatalf("controles não filtrados: %q", turns[0].content)
	}
}

func TestCaptureAppendOnlyNoEmptyTurns(t *testing.T) {
	var turns []turn
	c := collect(&turns)
	// CRs vazios e output só-ANSI não geram turnos
	c.onInput([]byte("\r\r"))
	c.onOutput([]byte("\x1b[2J\x1b[H")) // limpar tela: vira ""
	c.flush()
	if len(turns) != 0 {
		t.Fatalf("turnos vazios não deveriam ser gravados: %+v", turns)
	}

	// flush duplo não duplica nada
	c.onOutput([]byte("resposta\n"))
	c.flush()
	c.flush()
	if len(turns) != 1 {
		t.Fatalf("flush duplo duplicou: %+v", turns)
	}
}
