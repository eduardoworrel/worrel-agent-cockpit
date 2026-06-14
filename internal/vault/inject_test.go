package vault

import "testing"

// secretSourceStub injeta segredos de teste sem SQLite.
type secretSourceStub struct {
	enabled bool
	items   []InjectableSecret
}

func (s secretSourceStub) InjectionEnabled(projectID string) bool { return s.enabled }
func (s secretSourceStub) InjectableSecrets(projectID string) ([]InjectableSecret, error) {
	return s.items, nil
}

func TestInjectableEnvDisabledByDefault(t *testing.T) {
	v := testVault(t)
	ct, _ := v.Encrypt([]byte("segredo"))
	src := secretSourceStub{enabled: false, items: []InjectableSecret{{Name: "K", Ciphertext: ct}}}
	env, err := v.InjectableEnv("p1", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(env) != 0 {
		t.Fatalf("injeção desabilitada deveria devolver vazio, veio %+v", env)
	}
}

func TestInjectableEnvDecifra(t *testing.T) {
	v := testVault(t)
	ct, _ := v.Encrypt([]byte("valor-secreto"))
	src := secretSourceStub{enabled: true, items: []InjectableSecret{{Name: "API_KEY", Ciphertext: ct}}}
	env, err := v.InjectableEnv("p1", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(env) != 1 || env[0] != "API_KEY=valor-secreto" {
		t.Fatalf("env = %+v", env)
	}
}
