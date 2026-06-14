package vault

import "testing"

func TestVaultExpoeBroker(t *testing.T) {
	v := testVault(t)
	if v.Broker() == nil {
		t.Fatal("Vault.Broker() nil")
	}
	id, ch := v.Broker().Open()
	go v.Broker().Resolve(id, true)
	if ok := <-ch; !ok {
		t.Fatal("esperava aprovação via fachada")
	}
}
