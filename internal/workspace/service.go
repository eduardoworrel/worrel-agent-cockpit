package workspace

// SyncProject garante o workspace de um escopo e sincroniza seus symlinks com
// a lista de pastas reais associadas. Devolve o caminho do workspace.
func (m *Manager) SyncProject(slug string, realDirs []string) (string, error) {
	ws, err := m.EnsureWorkspace(slug)
	if err != nil {
		return "", err
	}
	if err := m.SyncSymlinks(ws, realDirs); err != nil {
		return "", err
	}
	return ws, nil
}
