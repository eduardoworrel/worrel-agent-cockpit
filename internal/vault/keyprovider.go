package vault

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const (
	keychainAccount = "worrel"
	keychainService = "worrel-master-key"
)

// ScryptProvider deriva a chave-mestra de uma senha + salt (fallback sem Keychain).
type ScryptProvider struct {
	password string
	salt     []byte
}

func NewScryptProvider(password string, salt []byte) *ScryptProvider {
	return &ScryptProvider{password: password, salt: salt}
}

// MasterKey deriva 32 bytes via scrypt (N=32768, r=8, p=1) — parâmetros fixos.
func (p *ScryptProvider) MasterKey() ([]byte, error) {
	return scrypt.Key([]byte(p.password), p.salt, 32768, 8, 1, 32)
}

// NewSalt gera um salt aleatório de 16 bytes (persistido em settings).
func NewSalt() ([]byte, error) {
	s := make([]byte, 16)
	_, err := rand.Read(s)
	return s, err
}

// KeychainProvider lê/escreve a chave-mestra no Keychain do macOS via `security`.
// Só é exercido em produção e na Task de E2E — nunca nos testes unitários.
type KeychainProvider struct{}

func (KeychainProvider) MasterKey() ([]byte, error) {
	out, err := exec.Command("security", "find-generic-password",
		"-a", keychainAccount, "-s", keychainService, "-w").Output()
	if err == nil {
		key, decErr := hex.DecodeString(strings.TrimSpace(string(out)))
		if decErr == nil && len(key) == 32 {
			return key, nil
		}
	}
	// Primeiro uso: gera chave nova e grava no Keychain.
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := exec.Command("security", "add-generic-password",
		"-a", keychainAccount, "-s", keychainService,
		"-w", hex.EncodeToString(key)).Run(); err != nil {
		return nil, fmt.Errorf("security add-generic-password: %w", err)
	}
	return key, nil
}
