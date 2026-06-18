import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getSettings, putSettings } from '../api';

export default function Settings() {
  const { t } = useTranslation();
  const [retentionDays, setRetentionDays] = useState('30');
  const [loading, setLoading] = useState(true);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState(false);
  const [busy, setBusy] = useState(false);
  const [resetting, setResetting] = useState(false);

  async function handleReset() {
    const ok = window.confirm(
      'Reiniciar do zero?\n\nIsto APAGA projetos, memórias, skills, pipelines, ' +
      'sugestões, sessões, segredos, histórico de chat e configurações. ' +
      'O esquema do banco e a chave-mestra do sistema (Keychain) são preservados.\n\n' +
      'Esta ação é irreversível.',
    );
    if (!ok) return;
    setResetting(true);
    setError(false);
    try {
      const res = await fetch('/api/reset', { method: 'POST' });
      if (!res.ok) throw new Error(await res.text());
      window.location.href = '/';
    } catch {
      setError(true);
      setResetting(false);
    }
  }

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const s = await getSettings();
        if (cancelled) return;
        if (s.retention_days) setRetentionDays(s.retention_days);
      } catch {
        if (!cancelled) setError(true);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    load();
    return () => { cancelled = true; };
  }, []);

  async function handleSave() {
    if (busy) return;
    setBusy(true);
    setError(false);
    try {
      await putSettings({ retention_days: retentionDays });
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch {
      setError(true);
    } finally {
      setBusy(false);
    }
  }

  if (loading) return <div className="main"><p>{t('common.loading')}</p></div>;

  return (
    <div className="main">
      <div className="page-head"><div><h1>{t('nav.settings')}</h1></div></div>
      <div className="card" style={{ maxWidth: '480px' }}>
        <label htmlFor="set-retention" style={{ display: 'block', marginBottom: '0.25rem' }}>{t('settings.retentionDays')}</label>
        <input
          id="set-retention"
          type="number"
          min={1}
          value={retentionDays}
          onChange={(e) => setRetentionDays(e.target.value)}
          style={{ marginBottom: '1rem' }}
        />

        {error && <p className="error-banner">{t('common.actionFailed')}</p>}
        <button className="btn btn-primary" disabled={busy} onClick={handleSave}>{t('settings.save')}</button>
        {saved && <span style={{ marginLeft: '1rem', color: 'var(--green)', fontWeight: 600 }}>{t('settings.saved')}</span>}
      </div>

      <div className="card" style={{ maxWidth: '480px', marginTop: '1.5rem', borderColor: 'var(--red)' }}>
        <h2 style={{ marginTop: 0, color: 'var(--red)' }}>Zona de perigo</h2>
        <p style={{ marginTop: 0, color: 'var(--muted)' }}>
          Reinicia a configuração do zero: apaga projetos, memórias, skills, pipelines,
          sugestões, sessões, segredos, histórico de chat e configurações. Preserva o
          esquema do banco e a chave-mestra do sistema. Ação irreversível.
        </p>
        <button className="btn btn-danger" disabled={resetting} onClick={handleReset}>
          {resetting ? 'Reiniciando…' : 'Reiniciar configuração do zero'}
        </button>
      </div>
    </div>
  );
}
