package vault

// InjectableSecret é um segredo injetável com seu valor cifrado.
type InjectableSecret struct {
	Name       string
	Ciphertext []byte
}

// SecretSource é o subconjunto do store usado pela injeção (testável sem SQLite).
type SecretSource interface {
	InjectionEnabled(projectID string) bool
	InjectableSecrets(projectID string) ([]InjectableSecret, error)
}

// InjectableEnv devolve "NOME=valor" dos segredos injetáveis do projeto, decifrados.
// Vazio se a injeção estiver desabilitada (padrão; spec §8.3). PONTO DE INTEGRAÇÃO
// da fase 3 (wrapper PTY chama isto ao montar o ambiente do subprocesso).
func (v *Vault) InjectableEnv(projectID string, src SecretSource) ([]string, error) {
	if !src.InjectionEnabled(projectID) {
		return nil, nil
	}
	items, err := src.InjectableSecrets(projectID)
	if err != nil {
		return nil, err
	}
	env := make([]string, 0, len(items))
	for _, it := range items {
		plain, err := v.Decrypt(it.Ciphertext)
		if err != nil {
			return nil, err
		}
		env = append(env, it.Name+"="+string(plain))
	}
	return env, nil
}
