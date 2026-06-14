package store

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// Secret são os metadados serializáveis (SEM o valor cru). HasValue indica se
// há ciphertext armazenado; o valor só é obtido via SecretCiphertext (interno).
type Secret struct {
	ID         string `json:"id"`
	ProjectID  string `json:"project_id"` // "" = global
	Name       string `json:"name"`
	Mode       string `json:"mode"`   // value | recipe
	Recipe     string `json:"recipe"` // só no modo recipe
	Policy     string `json:"policy"` // always | per_session | per_access
	Injectable bool   `json:"injectable"`
	HasValue   bool   `json:"has_value"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
}

type SecretAudit struct {
	ID         int64   `json:"id"`
	SecretID   *string `json:"secret_id"`
	SecretName string  `json:"secret_name"`
	SessionID  *string `json:"session_id"`
	ProjectID  *string `json:"project_id"`
	Action     string  `json:"action"`
	Detail     string  `json:"detail"`
	CreatedAt  int64   `json:"created_at"`
}

// CreateSecret grava o segredo. ciphertext é o valor já cifrado (nil no modo recipe).
func (s *Store) CreateSecret(sec *Secret, ciphertext []byte) (*Secret, error) {
	if sec.Policy == "" {
		sec.Policy = "per_access"
	}
	// Unicidade explícita para escopo global (NULLs são distintos no índice UNIQUE).
	if sec.ProjectID == "" {
		var n int
		if err := s.db.QueryRow(`SELECT count(*) FROM secrets WHERE project_id IS NULL AND name=?`,
			sec.Name).Scan(&n); err != nil {
			return nil, err
		}
		if n > 0 {
			return nil, fmt.Errorf("segredo global %q já existe", sec.Name)
		}
	}
	sec.ID = uuid.NewString()
	sec.CreatedAt = now()
	sec.UpdatedAt = now()
	var ct any
	if len(ciphertext) > 0 {
		ct = ciphertext
		sec.HasValue = true
	}
	var recipe any
	if sec.Recipe != "" {
		recipe = sec.Recipe
	}
	_, err := s.db.Exec(`INSERT INTO secrets
		(id, project_id, name, mode, ciphertext, recipe, policy, injectable, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		sec.ID, nullable(sec.ProjectID), sec.Name, sec.Mode, ct, recipe, sec.Policy,
		boolInt(sec.Injectable), sec.CreatedAt, sec.UpdatedAt)
	return sec, err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

const secretCols = `SELECT id, COALESCE(project_id,''), name, mode, COALESCE(recipe,''),
	policy, injectable, ciphertext IS NOT NULL, created_at, updated_at FROM secrets`

func scanSecret(r rowScanner) (*Secret, error) {
	x := &Secret{}
	var inj int
	if err := r.Scan(&x.ID, &x.ProjectID, &x.Name, &x.Mode, &x.Recipe, &x.Policy,
		&inj, &x.HasValue, &x.CreatedAt, &x.UpdatedAt); err != nil {
		return nil, err
	}
	x.Injectable = inj == 1
	return x, nil
}

func (s *Store) GetSecret(id string) (*Secret, error) {
	return scanSecret(s.db.QueryRow(secretCols+` WHERE id=?`, id))
}

// ListSecrets: projectID "" lista os globais; caso contrário, os do projeto.
func (s *Store) ListSecrets(projectID string) ([]*Secret, error) {
	q := secretCols
	args := []any{}
	if projectID == "" {
		q += ` WHERE project_id IS NULL`
	} else {
		q += ` WHERE project_id=?`
		args = append(args, projectID)
	}
	q += ` ORDER BY name`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Secret{}
	for rows.Next() {
		x, err := scanSecret(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// ResolveSecret busca pelo nome no escopo do projeto e, se não achar, no global.
func (s *Store) ResolveSecret(projectID, name string) (*Secret, error) {
	if projectID != "" {
		if sec, err := scanSecret(s.db.QueryRow(secretCols+
			` WHERE project_id=? AND name=?`, projectID, name)); err == nil {
			return sec, nil
		} else if err != sql.ErrNoRows {
			return nil, err
		}
	}
	return scanSecret(s.db.QueryRow(secretCols+` WHERE project_id IS NULL AND name=?`, name))
}

// SecretCiphertext devolve o valor cifrado (uso interno do vault/MCP; nunca via REST).
func (s *Store) SecretCiphertext(id string) ([]byte, error) {
	var ct []byte
	err := s.db.QueryRow(`SELECT ciphertext FROM secrets WHERE id=?`, id).Scan(&ct)
	return ct, err
}

func (s *Store) UpdateSecretValue(id string, ciphertext []byte) error {
	_, err := s.db.Exec(`UPDATE secrets SET ciphertext=?, updated_at=? WHERE id=?`,
		ciphertext, now(), id)
	return err
}

func (s *Store) UpdateSecretRecipe(id, recipe string) error {
	_, err := s.db.Exec(`UPDATE secrets SET recipe=?, updated_at=? WHERE id=?`, recipe, now(), id)
	return err
}

func (s *Store) UpdateSecretPolicy(id, policy string, injectable bool) error {
	_, err := s.db.Exec(`UPDATE secrets SET policy=?, injectable=?, updated_at=? WHERE id=?`,
		policy, boolInt(injectable), now(), id)
	return err
}

func (s *Store) DeleteSecret(id string) error {
	_, err := s.db.Exec(`DELETE FROM secrets WHERE id=?`, id)
	return err
}

// --- Auditoria (permanente) ---

func (s *Store) AuditSecret(secretID, secretName string, sessionID, projectID *string, action, detail string) error {
	_, err := s.db.Exec(`INSERT INTO secret_audit
		(secret_id, secret_name, session_id, project_id, action, detail, created_at)
		VALUES (?,?,?,?,?,?,?)`,
		nullable(secretID), secretName, sessionID, projectID, action, detail, now())
	return err
}

// HasSessionGrant: existe um "granted" para (session, secret)? Base da política per_session.
func (s *Store) HasSessionGrant(sessionID, secretID string) bool {
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM secret_audit
		WHERE session_id=? AND secret_id=? AND action='granted'`, sessionID, secretID).Scan(&n)
	return n > 0
}

// ListSecretAudit: auditoria do projeto (mais recentes primeiro).
func (s *Store) ListSecretAudit(projectID string) ([]*SecretAudit, error) {
	rows, err := s.db.Query(`SELECT id, secret_id, secret_name, session_id, project_id,
		action, detail, created_at FROM secret_audit WHERE project_id=? ORDER BY id DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*SecretAudit{}
	for rows.Next() {
		a := &SecretAudit{}
		if err := rows.Scan(&a.ID, &a.SecretID, &a.SecretName, &a.SessionID, &a.ProjectID,
			&a.Action, &a.Detail, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AuditSecretByName audita usando o nome quando o id não está em mãos (ex.: timeout).
func (s *Store) AuditSecretByName(name, projectID string, sessionID, projPtr *string, action, detail string) error {
	sec, err := s.ResolveSecret(projectID, name)
	var secID string
	if err == nil {
		secID = sec.ID
	}
	return s.AuditSecret(secID, name, sessionID, projPtr, action, detail)
}

// InjectionEnabled retorna true se a injeção de segredos está habilitada para o projeto.
func (s *Store) InjectionEnabled(projectID string) bool {
	return s.GetSetting("secrets_injection_enabled:"+projectID, "false") == "true"
}

// InjectableSecrets lista os segredos injetáveis do projeto com seus ciphertexts.
func (s *Store) InjectableSecrets(projectID string) ([]InjectableItem, error) {
	rows, err := s.db.Query(`SELECT name, ciphertext FROM secrets
		WHERE project_id=? AND injectable=1 AND mode='value' AND ciphertext IS NOT NULL
		ORDER BY name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InjectableItem{}
	for rows.Next() {
		var item InjectableItem
		if err := rows.Scan(&item.Name, &item.Ciphertext); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// InjectableItem é um par nome/ciphertext para injeção de env.
type InjectableItem struct {
	Name       string
	Ciphertext []byte
}
