package vault

import (
	"bytes"
	"testing"
)

// fixedKey é o KeyProvider usado em todos os testes: NUNCA toca no Keychain real.
type fixedKey struct{ k []byte }

func (f fixedKey) MasterKey() ([]byte, error) { return f.k, nil }

func testVault(t *testing.T) *Vault {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	v, err := New(fixedKey{k: key})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	v := testVault(t)
	plain := []byte("sk-super-secreto-123")
	ct, err := v.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ct, plain) {
		t.Fatal("ciphertext contém o texto claro")
	}
	got, err := v.Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("decrypt = %q, want %q", got, plain)
	}
}

func TestEncryptNonceVaria(t *testing.T) {
	v := testVault(t)
	a, _ := v.Encrypt([]byte("x"))
	b, _ := v.Encrypt([]byte("x"))
	if bytes.Equal(a, b) {
		t.Fatal("nonce não é aleatório: dois ciphertexts idênticos")
	}
}

func TestDecryptTamperFails(t *testing.T) {
	v := testVault(t)
	ct, _ := v.Encrypt([]byte("segredo"))
	ct[len(ct)-1] ^= 0xff // corrompe o último byte (tag GCM)
	if _, err := v.Decrypt(ct); err == nil {
		t.Fatal("decrypt aceitou ciphertext adulterado (falha de autenticação GCM esperada)")
	}
}

func TestNewRejeitaChaveErrada(t *testing.T) {
	if _, err := New(fixedKey{k: make([]byte, 16)}); err == nil {
		t.Fatal("New aceitou chave de 16 bytes (esperava 32)")
	}
}
