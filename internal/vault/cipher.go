// Package vault cifra segredos localmente (AES-256-GCM), resolve a chave-mestra
// (Keychain do macOS ou senha-mestra via scrypt) e arbitra a política de
// aprovação de acessos (spec §8).
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// KeyProvider devolve a chave-mestra de 32 bytes. Implementações:
// KeychainProvider (macOS, produção) e ScryptProvider (fallback). Testes usam
// um provider de chave fixa — NUNCA o Keychain real.
type KeyProvider interface {
	MasterKey() ([]byte, error)
}

type Vault struct {
	gcm    cipher.AEAD
	broker *Broker
}

// New monta o AEAD a partir da chave do provider. Exige chave de 32 bytes (AES-256).
func New(kp KeyProvider) (*Vault, error) {
	key, err := kp.MasterKey()
	if err != nil {
		return nil, fmt.Errorf("chave-mestra: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("chave-mestra deve ter 32 bytes, tem %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Vault{gcm: gcm, broker: NewBroker()}, nil
}

// Encrypt cifra plain e devolve nonce || ciphertext (nonce prefixado).
func (v *Vault) Encrypt(plain []byte) ([]byte, error) {
	nonce := make([]byte, v.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return v.gcm.Seal(nonce, nonce, plain, nil), nil
}

// Decrypt separa o nonce prefixado e decifra; falha de autenticação (GCM) é erro.
func (v *Vault) Decrypt(blob []byte) ([]byte, error) {
	ns := v.gcm.NonceSize()
	if len(blob) < ns {
		return nil, fmt.Errorf("ciphertext curto demais")
	}
	nonce, ct := blob[:ns], blob[ns:]
	return v.gcm.Open(nil, nonce, ct, nil)
}
