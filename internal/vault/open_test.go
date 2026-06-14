package vault

import "testing"

// settingsStub simula store.GetSetting/SetSetting para o salt, sem SQLite.
type settingsStub struct{ m map[string]string }

func (s *settingsStub) GetSetting(k, def string) string {
	if v, ok := s.m[k]; ok {
		return v
	}
	return def
}
func (s *settingsStub) SetSetting(k, v string) error { s.m[k] = v; return nil }

func TestOpenWithScryptFallbackPersisteSalt(t *testing.T) {
	st := &settingsStub{m: map[string]string{}}
	v1, err := OpenWithScrypt(st, "senha-mestra")
	if err != nil {
		t.Fatal(err)
	}
	salt1 := st.GetSetting("vault_salt", "")
	if salt1 == "" {
		t.Fatal("salt não persistido em settings")
	}
	// Reabrir com o mesmo salt+senha deve decifrar o que o primeiro cifrou.
	ct, _ := v1.Encrypt([]byte("abc"))
	v2, err := OpenWithScrypt(st, "senha-mestra")
	if err != nil {
		t.Fatal(err)
	}
	got, err := v2.Decrypt(ct)
	if err != nil || string(got) != "abc" {
		t.Fatalf("reabertura não decifrou: %q %v", got, err)
	}
}
