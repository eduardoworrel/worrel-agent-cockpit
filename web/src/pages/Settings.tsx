import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getSettings, putSettings } from '../api';

export default function Settings() {
  const { t } = useTranslation();
  const [retentionDays, setRetentionDays] = useState('30');
  const [headlessAdapter, setHeadlessAdapter] = useState('claude-code');
  const [healthMinRate, setHealthMinRate] = useState('0.5');
  const [healthConsecFailures, setHealthConsecFailures] = useState('2');
  const [autoDailyCap, setAutoDailyCap] = useState('3');
  const [loading, setLoading] = useState(true);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const s = await getSettings();
        if (cancelled) return;
        if (s.retention_days) setRetentionDays(s.retention_days);
        if (s.headless_adapter) setHeadlessAdapter(s.headless_adapter);
        if (s.health_min_success_rate) setHealthMinRate(s.health_min_success_rate);
        if (s.health_consec_failures) setHealthConsecFailures(s.health_consec_failures);
        if (s.auto_daily_cap) setAutoDailyCap(s.auto_daily_cap);
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
      await putSettings({
        retention_days: retentionDays,
        headless_adapter: headlessAdapter,
        health_min_success_rate: healthMinRate,
        health_consec_failures: healthConsecFailures,
        auto_daily_cap: autoDailyCap,
      });
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

        <label htmlFor="set-adapter" style={{ display: 'block', marginBottom: '0.25rem' }}>{t('settings.headlessAdapter')}</label>
        <select
          id="set-adapter"
          value={headlessAdapter}
          onChange={(e) => setHeadlessAdapter(e.target.value)}
          style={{ marginBottom: '1rem' }}
        >
          <option value="claude-code">claude-code</option>
          <option value="opencode">opencode</option>
          <option value="gemini">gemini</option>
          <option value="codex">codex</option>
        </select>

        <label htmlFor="set-health-rate" style={{ display: 'block', marginBottom: '0.25rem' }}>{t('settings.healthMinRate')}</label>
        <input id="set-health-rate" type="number" min={0} max={1} step={0.05}
          value={healthMinRate} onChange={(e) => setHealthMinRate(e.target.value)} style={{ marginBottom: '1rem' }} />

        <label htmlFor="set-health-consec" style={{ display: 'block', marginBottom: '0.25rem' }}>{t('settings.healthConsecFailures')}</label>
        <input id="set-health-consec" type="number" min={1}
          value={healthConsecFailures} onChange={(e) => setHealthConsecFailures(e.target.value)} style={{ marginBottom: '1rem' }} />

        <label htmlFor="set-auto-cap" style={{ display: 'block', marginBottom: '0.25rem' }}>{t('settings.autoDailyCap')}</label>
        <input id="set-auto-cap" type="number" min={0}
          value={autoDailyCap} onChange={(e) => setAutoDailyCap(e.target.value)} style={{ marginBottom: '1rem' }} />

        {error && <p className="error-banner">{t('common.actionFailed')}</p>}
        <button className="btn btn-primary" disabled={busy} onClick={handleSave}>{t('settings.save')}</button>
        {saved && <span style={{ marginLeft: '1rem', color: 'var(--green)', fontWeight: 600 }}>{t('settings.saved')}</span>}
      </div>
    </div>
  );
}
