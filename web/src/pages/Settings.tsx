import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getSettings, putSettings, listProjects, listSessions } from '../api';
import EngineCard, { type EngineItem } from '../components/EngineCard';
import HomeEngineConfig from '../components/HomeEngineConfig';
import OnboardingWizard from '../components/OnboardingWizard';
import { getInteractionStyle, setInteractionStyle } from '../interactionStyle';
import type { InteractionStyle } from '../interactionStyle';

type EngineLogEntry = { id: number; engine_id: string; trigger: string; suggestions: number; detail: string; input?: string; output?: string; created_at: number };

export default function Settings() {
  const { t } = useTranslation();
  const [retentionDays, setRetentionDays] = useState('30');
  const [loading, setLoading] = useState(true);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState(false);
  const [busy, setBusy] = useState(false);
  const [resetting, setResetting] = useState(false);

  // Config dos motores de destilação (mesma fonte do wizard de onboarding).
  const [engines, setEngines] = useState<EngineItem[]>([]);
  const [showWizard, setShowWizard] = useState(false);
  const [tab, setTab] = useState('geral');
  const [activity, setActivity] = useState<EngineLogEntry[]>([]);
  const loadActivity = () =>
    fetch('/api/engines/activity').then(r => r.json()).then(setActivity).catch(() => setActivity([]));
  const [ixStyle, setIxStyle] = useState<InteractionStyle>(getInteractionStyle());
  const chooseStyle = (s: InteractionStyle) => { setInteractionStyle(s); setIxStyle(s); };
  const loadEngines = () =>
    fetch('/api/engines').then(r => r.json()).then(setEngines).catch(() => setEngines([]));
  const setEngineConfig = (id: string, key: string, value: string) =>
    fetch(`/api/engines/${id}/config`, {
      method: 'PUT', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key, value }),
    }).then(loadEngines);
  // Rodar sob demanda: dispara o motor e mostra o resultado na aba Atividade.
  const runEngine = (id: string) =>
    fetch(`/api/engines/${id}/run`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ project_id: '', session_id: '' }),
    }).then(() => { setTab('atividade'); loadActivity(); }).catch(() => {});

  useEffect(() => { loadEngines(); }, []);
  useEffect(() => { if (tab === 'atividade') loadActivity(); }, [tab]);

  // Primeiro uso: abre o wizard automaticamente (sem projetos/sessões e ainda não visto).
  useEffect(() => {
    if (localStorage.getItem('worrel.onboarding.seen') === '1') return;
    Promise.all([listProjects().catch(() => []), listSessions().catch(() => [])])
      .then(([projs, sess]) => {
        const empty = projs.length === 0 && sess.filter(s => s.mode === 'wrapper').length === 0;
        if (empty) setShowWizard(true);
      });
  }, []);

  const closeWizard = () => {
    localStorage.setItem('worrel.onboarding.seen', '1');
    setShowWizard(false);
    loadEngines();
  };

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

  if (showWizard) return <OnboardingWizard onClose={closeWizard} />;
  if (loading) return <div className="main"><p>{t('common.loading')}</p></div>;

  const engine = engines.find(e => e.spec.id === tab);

  return (
    <div className="main">
      <div className="page-head">
        <div><h1>{t('nav.settings')}</h1></div>
        <button className="btn btn-secondary" onClick={() => setShowWizard(true)}>{t('settings.configureEngines', 'Abrir assistente')}</button>
      </div>

      <div className="tabs">
        <button className={`tab${tab === 'geral' ? ' active' : ''}`} onClick={() => setTab('geral')}>{t('settings.tabGeral', 'Geral')}</button>
        {engines.map(it => (
          <button key={it.spec.id} className={`tab${tab === it.spec.id ? ' active' : ''}`} onClick={() => setTab(it.spec.id)}>
            {it.spec.name}
          </button>
        ))}
        <button className={`tab${tab === 'atividade' ? ' active' : ''}`} onClick={() => setTab('atividade')}>{t('settings.tabActivity', 'Atividade')}</button>
      </div>

      {tab === 'geral' && (
        <>
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

          <div className="card" style={{ maxWidth: '480px', marginTop: '1.5rem' }}>
            <h2 style={{ marginTop: 0 }}>{t('settings.layout', 'Layout')}</h2>
            <p style={{ marginTop: 0, color: 'var(--muted)' }}>{t('onboardingStyle.subtitle')}</p>
            <div className="onb-style-options">
              <button className={`onb-style-opt${ixStyle === 'modal' ? ' on' : ''}`} onClick={() => chooseStyle('modal')}>
                <span className="onb-style-art onb-style-modal"><i /></span>
                <b>{t('onboardingStyle.modal')}</b>
                <span>{t('onboardingStyle.modalDesc')}</span>
              </button>
              <button className={`onb-style-opt${ixStyle === 'drawer' ? ' on' : ''}`} onClick={() => chooseStyle('drawer')}>
                <span className="onb-style-art onb-style-drawer"><i /></span>
                <b>{t('onboardingStyle.drawer')}</b>
                <span>{t('onboardingStyle.drawerDesc')}</span>
              </button>
            </div>
          </div>

          <div style={{ maxWidth: '760px', marginTop: '1rem' }}>
            <h2 style={{ marginBottom: '0.2rem' }}>{t('settings.aiCostTitle', 'Inteligência & custo')}</h2>
            <p style={{ color: 'var(--muted)', marginTop: 0 }}>
              {t('settings.aiCostHint', 'Recursos de IA da Home. Escolha harness e modelo. Toda execução é auditada.')}
            </p>
            <HomeEngineConfig id="summary" defaultOn={false}
              title={t('settings.aiSummary', 'Resumo de progresso')}
              description={t('settings.aiSummaryDesc', 'Narração ao vivo das miniaturas. Desligado mostra a cauda crua (sem custo).')} />
            <HomeEngineConfig id="interpret" defaultOn={true}
              title={t('settings.aiInterpret', 'Interpretação para resposta')}
              description={t('settings.aiInterpretDesc', 'Transforma a fala do agente em opções acionáveis.')} />
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
        </>
      )}

      {engine && (
        <EngineCard item={engine} setConfig={setEngineConfig} onRun={runEngine} />
      )}

      {tab === 'atividade' && (
        <div className="card" style={{ maxWidth: '760px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <h2 style={{ margin: 0 }}>{t('settings.activityTitle', 'Atividade dos motores')}</h2>
            <button className="btn btn-secondary" onClick={loadActivity}>{t('common.refresh', 'Atualizar')}</button>
          </div>
          <p style={{ marginTop: '0.4rem', color: 'var(--muted)' }}>
            {t('settings.activityHint', 'Cada execução de motor, as sugestões que gerou e o prompt/resposta da IA quando houver (explicabilidade).')}
          </p>
          {activity.length === 0 && <p style={{ color: 'var(--muted)' }}>{t('settings.activityEmpty', 'Nenhuma execução ainda.')}</p>}
          {activity.map(a => (
            <div key={a.id} style={{ borderTop: '1px solid var(--border, #333)', padding: '0.5rem 0' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: '1rem' }}>
                <strong>{engines.find(e => e.spec.id === a.engine_id)?.spec.name ?? a.engine_id}</strong>
                <span style={{ color: 'var(--muted)', fontSize: '0.8rem' }}>{new Date(a.created_at).toLocaleString()}</span>
              </div>
              <div style={{ fontSize: '0.85rem', color: 'var(--muted)' }}>
                gatilho: {a.trigger || '—'} · {a.suggestions} sugestão(ões)
              </div>
              {a.detail && <div style={{ fontSize: '0.85rem', marginTop: '0.2rem' }}>{a.detail}</div>}
              {(a.input || a.output) && (
                <details style={{ marginTop: '0.3rem', fontSize: '0.8rem' }}>
                  <summary style={{ cursor: 'pointer', color: 'var(--muted)' }}>
                    {t('settings.activityIO', 'Ver prompt e resposta da IA')}
                  </summary>
                  {a.input && (
                    <pre style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', background: 'var(--bg-elev, #1a1a1a)', padding: '0.5rem', borderRadius: 4, marginTop: '0.3rem' }}>
                      <strong>input:</strong>{'\n'}{a.input}
                    </pre>
                  )}
                  {a.output && (
                    <pre style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', background: 'var(--bg-elev, #1a1a1a)', padding: '0.5rem', borderRadius: 4, marginTop: '0.3rem' }}>
                      <strong>output:</strong>{'\n'}{a.output}
                    </pre>
                  )}
                </details>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
