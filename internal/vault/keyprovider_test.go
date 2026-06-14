package vault

import (
	"bytes"
	"testing"
)

func TestScryptProviderDeterministico(t *testing.T) {
	salt := []byte("salt-fixo-16byte")
	p1 := NewScryptProvider("senha-mestra", salt)
	p2 := NewScryptProvider("senha-mestra", salt)
	k1, err := p1.MasterKey()
	if err != nil {
		t.Fatal(err)
	}
	k2, _ := p2.MasterKey()
	if len(k1) != 32 {
		t.Fatalf("chave = %d bytes, want 32", len(k1))
	}
	if !bytes.Equal(k1, k2) {
		t.Fatal("scrypt não é determinístico com mesma senha+salt")
	}
	p3 := NewScryptProvider("outra-senha", salt)
	k3, _ := p3.MasterKey()
	if bytes.Equal(k1, k3) {
		t.Fatal("senhas diferentes geraram a mesma chave")
	}
}

func TestNewSaltTamanho(t *testing.T) {
	s, err := NewSalt()
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 16 {
		t.Fatalf("salt = %d bytes, want 16", len(s))
	}
}
