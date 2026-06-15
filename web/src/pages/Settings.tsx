import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getSettings, putSettings, getPrompts, savePrompt } from '../api';
import type { PromptDef } from '../api';

const PROMPT_LABEL: Record<string, string> = {
  memory: 'Memória — o que destilar como memória do projeto',
  skill: 'Skills — o que vira procedimento reutilizável',
  scope: 'Escopo — como agrupar sessões em projetos',
};

// PromptsCard expõe os prompts usados nas análises retroativas: editáveis,
// com reset ao default embarcado. Override vazio volta ao default.
function PromptsCard() {
  const [prompts, setPrompts] = useState<PromptDef[]>([]);
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState<string | null>(null);
  const [savedName, setSavedName] = useState<string | null>(null);

  function load() {
    getPrompts()
      .then((ps) => {
        setPrompts(ps);
        setDrafts(Object.fromEntries(ps.map((p) => [p.name, p.value])));
      })
      .catch(() => setPrompts([]));
  }
  useEffect(() => { load(); }, []);

  async function save(name: string, value: string) {
    setBusy(name);
    try {
      await savePrompt(name, value);
      setSavedName(name);
      setTimeout(() => setSavedName(null), 2000);
      load();
    } finally {
      setBusy(null);
    }
  }

  if (prompts.length === 0) return null;

  return (
    <div className="card" style={{ maxWidth: '760px', marginTop: '1.5rem' }}>
      <h2 style={{ marginTop: 0 }}>Prompts das análises</h2>
      <p style={{ marginTop: 0, color: 'var(--muted)' }}>
        Texto que orienta cada etapa da análise retroativa. Edite e salve para
        sobrepor; deixe igual ao default (ou use “Restaurar default”) para voltar.
      </p>
      {prompts.map((p) => (
        <div key={p.name} style={{ marginBottom: '1.25rem' }}>
          <label htmlFor={`prompt-${p.name}`} style={{ display: 'block', marginBottom: '0.25rem', fontWeight: 600 }}>
            {PROMPT_LABEL[p.name] ?? p.name}
            {p.overridden && <span style={{ marginLeft: 8, color: 'var(--orange-ink)', fontSize: '0.8rem' }}>editado</span>}
          </label>
          <textarea
            id={`prompt-${p.name}`}
            value={drafts[p.name] ?? ''}
            onChange={(e) => setDrafts((d) => ({ ...d, [p.name]: e.target.value }))}
            rows={Math.min(20, (drafts[p.name] ?? '').split('\n').length + 1)}
            style={{ width: '100%', fontFamily: 'var(--mono)', fontSize: '0.8rem' }}
          />
          <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.4rem', alignItems: 'center' }}>
            <button className="btn btn-primary" disabled={busy === p.name} onClick={() => save(p.name, drafts[p.name] ?? '')}>
              Salvar
            </button>
            <button
              className="btn btn-secondary"
              disabled={busy === p.name || !p.overridden}
              onClick={() => save(p.name, '')}
            >
              Restaurar default
            </button>
            {savedName === p.name && <span style={{ color: 'var(--green)', fontWeight: 600 }}>salvo</span>}
          </div>
        </div>
      ))}
    </div>
  );
}

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
          <option value="pidev">pi</option>
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

      <PromptsCard />

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
