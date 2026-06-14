package vault

import (
	"encoding/hex"
	"log"
)

// SettingsAccessor é o subconjunto de *store.Store usado para persistir o salt
// (interface estreita para testabilidade sem SQLite).
type SettingsAccessor interface {
	GetSetting(key, def string) string
	SetSetting(key, value string) error
}

// OpenWithScrypt monta o Vault pelo fallback de senha-mestra, reusando/gerando o
// salt persistido em settings (chave "vault_salt", hex).
func OpenWithScrypt(st SettingsAccessor, password string) (*Vault, error) {
	saltHex := st.GetSetting("vault_salt", "")
	var salt []byte
	if saltHex != "" {
		s, err := hex.DecodeString(saltHex)
		if err == nil {
			salt = s
		}
	}
	if len(salt) == 0 {
		s, err := NewSalt()
		if err != nil {
			return nil, err
		}
		salt = s
		if err := st.SetSetting("vault_salt", hex.EncodeToString(salt)); err != nil {
			return nil, err
		}
	}
	return New(NewScryptProvider(password, salt))
}

// Open é o ponto de montagem em produção: tenta o Keychain do macOS e, em caso de
// falha, cai para scrypt com a senha-mestra fornecida. Usado por cmd/worrel.
func Open(st SettingsAccessor, masterPassword string) (*Vault, error) {
	if v, err := New(KeychainProvider{}); err == nil {
		return v, nil
	} else {
		log.Printf("vault: Keychain indisponível (%v); usando senha-mestra (scrypt)", err)
	}
	return OpenWithScrypt(st, masterPassword)
}
