import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  listSecrets,
  createSecret,
  deleteSecret,
  updateSecretPolicy,
  listSecretAudit,
  setInjection,
} from '../api';
import type { Secret, SecretAudit } from '../api';

interface Props {
  projectId: string;
}

export default function SecretsTab({ projectId }: Props) {
  const { t } = useTranslation();
  const [secrets, setSecrets] = useState<Secret[]>([]);
  const [audit, setAudit] = useState<SecretAudit[]>([]);
  const [injectionEnabled, setInjectionEnabled] = useState(false);
  const [showAdd, setShowAdd] = useState(false);
  const [showAudit, setShowAudit] = useState(false);
  const [busy, setBusy] = useState(false);

  // form state
  const [name, setName] = useState('');
  const [mode, setMode] = useState<'value' | 'recipe'>('value');
  const [value, setValue] = useState('');
  const [recipe, setRecipe] = useState('');
  const [policy, setPolicy] = useState<'always' | 'per_session' | 'per_access'>('per_access');
  const [injectable, setInjectable] = useState(false);

  useEffect(() => {
    loadSecrets();
  }, [projectId]);

  async function loadSecrets() {
    try {
      const list = await listSecrets(projectId);
      setSecrets(list ?? []);
    } catch { /* ignore */ }
  }

  async function loadAudit() {
    try {
      const rows = await listSecretAudit(projectId);
      setAudit(rows ?? []);
    } catch { /* ignore */ }
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    if (!name) return;
    if (injectable && !confirm(t('secrets.confirmInjectable'))) return;
    setBusy(true);
    try {
      await createSecret(projectId, { name, mode, value: mode === 'value' ? value : undefined, recipe: mode === 'recipe' ? recipe : undefined, policy, injectable });
      setName(''); setValue(''); setRecipe(''); setMode('value'); setPolicy('per_access'); setInjectable(false);
      setShowAdd(false);
      await loadSecrets();
    } catch { /* ignore */ } finally { setBusy(false); }
  }

  async function handleDelete(sec: Secret) {
    if (!window.confirm(t('secrets.confirmDelete', { name: sec.name }))) return;
    await deleteSecret(sec.id);
    await loadSecrets();
  }

  async function handlePolicyChange(sec: Secret, newPolicy: string) {
    await updateSecretPolicy(sec.id, newPolicy, sec.injectable);
    await loadSecrets();
  }

  async function handleInjectionToggle() {
    const next = !injectionEnabled;
    if (next && !confirm(t('secrets.confirmInjectable'))) return;
    await setInjection(projectId, next);
    setInjectionEnabled(next);
  }

  const riskStyle: React.CSSProperties = {
    background: '#fef2f2',
    border: '1px solid #ef4444',
    color: '#b91c1c',
    padding: '0.6rem 0.8rem',
    borderRadius: '6px',
    fontWeight: 600,
    fontSize: '0.9rem',
    marginTop: '0.5rem',
  };

  async function handleShowAudit() {
    await loadAudit();
    setShowAudit(true);
  }

  return (
    <div className="secrets-tab">
      <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '1rem', alignItems: 'center' }}>
        <h3 style={{ margin: 0 }}>{t('secrets.title')}</h3>
        <button className="btn btn-primary" onClick={() => setShowAdd(!showAdd)}>{t('secrets.add')}</button>
        <button className="btn btn-secondary" onClick={handleInjectionToggle} style={{ marginLeft: 'auto' }}>
          {t('secrets.injection')}: {injectionEnabled ? t('secrets.injectionEnabled') : t('secrets.injectionDisabled')}
        </button>
        <button className="btn btn-secondary" onClick={handleShowAudit}>{t('secrets.audit')}</button>
      </div>

      {injectionEnabled && (
        <div className="risk-warning risk-strong" role="alert" style={riskStyle}>{t('secrets.riskInjectable')}</div>
      )}

      {showAdd && (
        <form onSubmit={handleCreate} style={{ background: 'var(--surface)', border: '1px solid var(--line)', padding: '1rem', borderRadius: '6px', marginBottom: '1rem' }}>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '0.5rem' }}>
            <input placeholder={t('secrets.name')} value={name} onChange={e => setName(e.target.value)} required />
            <select value={mode} onChange={e => setMode(e.target.value as 'value' | 'recipe')}>
              <option value="value">{t('secrets.modeValue')}</option>
              <option value="recipe">{t('secrets.modeRecipe')}</option>
            </select>
            <select value={policy} onChange={e => setPolicy(e.target.value as typeof policy)}>
              <option value="always">{t('secrets.policyAlways')}</option>
              <option value="per_session">{t('secrets.policySession')}</option>
              <option value="per_access">{t('secrets.policyAccess')}</option>
            </select>
            <label style={{ display: 'flex', alignItems: 'center', gap: '0.25rem' }}>
              <input type="checkbox" checked={injectable} onChange={e => setInjectable(e.target.checked)} />
              {t('secrets.injectable')}
            </label>
          </div>
          {mode === 'value' && (
            <>
              <div className="risk-warning" role="alert" style={riskStyle}>{t('secrets.riskValue')}</div>
              <input type="password" placeholder={t('secrets.value')} value={value} onChange={e => setValue(e.target.value)} style={{ marginTop: '0.5rem', width: '100%' }} />
            </>
          )}
          {mode === 'recipe' && (
            <textarea placeholder={t('secrets.recipe')} value={recipe} onChange={e => setRecipe(e.target.value)} style={{ marginTop: '0.5rem', width: '100%', minHeight: '60px' }} />
          )}
          {injectable && (
            <div className="risk-warning risk-strong" role="alert" style={riskStyle}>{t('secrets.riskInjectable')}</div>
          )}
          <div style={{ marginTop: '0.5rem', display: 'flex', gap: '0.5rem' }}>
            <button className="btn btn-primary" type="submit" disabled={busy}>{t('common.create')}</button>
            <button className="btn btn-secondary" type="button" onClick={() => setShowAdd(false)}>{t('common.cancel')}</button>
          </div>
        </form>
      )}

      {secrets.length === 0 ? (
        <p>{t('secrets.noSecrets')}</p>
      ) : (
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr>
              <th style={{ textAlign: 'left' }}>{t('secrets.name')}</th>
              <th>{t('secrets.mode')}</th>
              <th>{t('secrets.policy')}</th>
              <th>{t('secrets.injectable')}</th>
              <th>{t('secrets.hasValue')}</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {secrets.map(sec => (
              <tr key={sec.id} style={{ borderTop: '1px solid var(--border,#ddd)' }}>
                <td style={{ padding: '0.4rem 0' }}>{sec.name}</td>
                <td style={{ textAlign: 'center' }}>{sec.mode}</td>
                <td>
                  <select value={sec.policy} onChange={e => handlePolicyChange(sec, e.target.value)}>
                    <option value="always">{t('secrets.policyAlways')}</option>
                    <option value="per_session">{t('secrets.policySession')}</option>
                    <option value="per_access">{t('secrets.policyAccess')}</option>
                  </select>
                </td>
                <td style={{ textAlign: 'center' }}>{sec.injectable ? '✓' : '—'}</td>
                <td style={{ textAlign: 'center' }}>{sec.has_value ? '✓' : sec.mode === 'recipe' ? '—' : '✗'}</td>
                <td>
                  <button className="btn btn-danger" onClick={() => handleDelete(sec)}>{t('secrets.delete')}</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {showAudit && (
        <div style={{ marginTop: '1.5rem' }}>
          <h4>{t('secrets.audit')}</h4>
          {audit.length === 0 ? <p>{t('secrets.noAudit')}</p> : (
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.85rem' }}>
              <thead>
                <tr>
                  <th style={{ textAlign: 'left' }}>Secret</th>
                  <th>Action</th>
                  <th>Session</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                {audit.map(row => (
                  <tr key={row.id} style={{ borderTop: '1px solid var(--border,#ddd)' }}>
                    <td style={{ padding: '0.3rem 0' }}>{row.secret_name}</td>
                    <td>{row.action}</td>
                    <td>{row.session_id ?? '—'}</td>
                    <td>{new Date(row.created_at).toLocaleString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  );
}
