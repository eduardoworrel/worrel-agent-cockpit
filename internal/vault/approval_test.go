package vault

import (
	"testing"
	"time"
)

func TestApprovalGrant(t *testing.T) {
	br := NewBroker()
	id, ch := br.Open()
	go func() {
		time.Sleep(10 * time.Millisecond)
		if !br.Resolve(id, true) {
			t.Errorf("Resolve devolveu false para id pendente")
		}
	}()
	select {
	case ok := <-ch:
		if !ok {
			t.Fatal("esperava aprovação")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestApprovalDeny(t *testing.T) {
	br := NewBroker()
	id, ch := br.Open()
	go br.Resolve(id, false)
	if ok := <-ch; ok {
		t.Fatal("esperava negação")
	}
}

func TestApprovalWaitTimeout(t *testing.T) {
	br := NewBroker()
	id, ch := br.Open()
	got, err := br.Wait(ch, 20*time.Millisecond)
	if err == nil {
		t.Fatal("esperava erro de timeout")
	}
	if got {
		t.Fatal("timeout não deveria conceder")
	}
	// Após timeout, o id não está mais pendente.
	if br.Resolve(id, true) {
		t.Fatal("Resolve após timeout deveria devolver false")
	}
}

func TestResolveIDInexistente(t *testing.T) {
	br := NewBroker()
	if br.Resolve("nao-existe", true) {
		t.Fatal("Resolve de id inexistente deveria devolver false")
	}
}

// TestWaitNaoPerdeAprovacaoNaCorridaComTimeout cobre a corrida em que Resolve
// responde entre o disparo do timer e a limpeza do timeout: a aprovação já
// entregue no canal bufferizado NUNCA pode ser descartada como "expirada".
func TestWaitNaoPerdeAprovacaoNaCorridaComTimeout(t *testing.T) {
	for i := 0; i < 200; i++ {
		br := NewBroker()
		id, ch := br.Open()
		if !br.Resolve(id, true) {
			t.Fatal("Resolve devolveu false para id pendente")
		}
		// timeout 0: o timer dispara imediatamente, competindo com o canal
		// já preenchido — o select pode cair em qualquer um dos dois braços.
		ok, err := br.Wait(ch, 0)
		if err != nil || !ok {
			t.Fatalf("iteração %d: aprovação concedida foi perdida (ok=%v err=%v)", i, ok, err)
		}
	}
}
