package vault

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Broker arbitra aprovações de acesso a segredos: pedidos pendentes por request_id
// com canal de resposta. Estado em memória (não persiste) — spec §8.1.
type Broker struct {
	mu      sync.Mutex
	pending map[string]chan bool
}

func NewBroker() *Broker { return &Broker{pending: map[string]chan bool{}} }

// Open registra um pedido e devolve (request_id, canal de resposta bufferizado).
func (b *Broker) Open() (string, chan bool) {
	id := uuid.NewString()
	ch := make(chan bool, 1)
	b.mu.Lock()
	b.pending[id] = ch
	b.mu.Unlock()
	return id, ch
}

// Resolve responde um pedido pendente; devolve false se o id não existir mais
// (já respondido ou expirado).
func (b *Broker) Resolve(id string, approve bool) bool {
	b.mu.Lock()
	ch, ok := b.pending[id]
	if ok {
		delete(b.pending, id)
	}
	b.mu.Unlock()
	if !ok {
		return false
	}
	ch <- approve
	return true
}

// Wait espera a resposta no canal ou expira após timeout, removendo o pedido.
func (b *Broker) Wait(ch chan bool, timeout time.Duration) (bool, error) {
	select {
	case ok := <-ch:
		return ok, nil
	case <-time.After(timeout):
		// remove qualquer pendência que ainda aponte para este canal
		b.mu.Lock()
		for id, c := range b.pending {
			if c == ch {
				delete(b.pending, id)
			}
		}
		b.mu.Unlock()
		// Corrida timeout × Resolve: se a resposta chegou entre o disparo do
		// timer e a limpeza acima, ela já está no canal bufferizado — honrá-la
		// em vez de descartar uma aprovação concedida como "expirada".
		select {
		case ok := <-ch:
			return ok, nil
		default:
		}
		return false, fmt.Errorf("aprovação expirou após %s", timeout)
	}
}

// Broker devolve o broker de aprovações deste vault.
func (v *Vault) Broker() *Broker { return v.broker }
