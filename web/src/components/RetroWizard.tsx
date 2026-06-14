import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useEvents } from '../useEvents';
import type { WsEvent } from '../useEvents';
import {
  inventory, createRun, cluster, listClusters, approveCluster, discardCluster,
  mergeClusters, startRun, pauseRun, resumeRun, cancelRun, getRun,
  type InventoryReport, type RetroRun, type RetroCluster, type Scope,
  type RetroRunProgress,
} from '../retroApi';
import RetroBatchReview from './RetroBatchReview';

const WINDOW_PRESETS = [30, 60, 90, 0]; // 0 = tudo

export default function RetroWizard() {
  const { t } = useTranslation();
  const [report, setReport] = useState<InventoryReport | null>(null);
  const [windowDays, setWindowDays] = useState(60);
  const [depth, setDepth] = useState<'completa' | 'leve'>('completa');
  const [provider, setProvider] = useState<'' | 'claude-code' | 'opencode'>('');
  const [model, setModel] = useState('');
  const [budgetPerHour, setBudgetPerHour] = useState(0);
  const [run, setRun] = useState<RetroRun | null>(null);
  const [clusters, setClusters] = useState<RetroCluster[]>([]);
  const [selected, setSelected] = useState<string[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [clustering, setClustering] = useState(false);
  const [progress, setProgress] = useState<RetroRunProgress | null>(null);

  const refreshInventory = useCallback(async () => {
    try {
      setReport(await inventory(windowDays));
    } catch (e) {
      setErr(String(e));
    }
  }, [windowDays]);

  useEffect(() => {
    refreshInventory();
  }, [refreshInventory]);

  // progresso ao vivo via WS
  const onEvent = useCallback((ev: WsEvent) => {
    if (!run) return;
    if (typeof ev.type !== 'string' || !ev.type.startsWith('retro.')) return;
    if (ev.type === 'retro.run.progress') {
      const p = ev.payload as RetroRunProgress;
      if (p && p.run_id === run.id) setProgress(p);
    }
    getRun(run.id).then(setRun).catch(() => {});
  }, [run]);
  useEvents(onEvent);

  async function openRun() {
    const scope: Scope = {
      clis: Object.keys(report?.per_cli ?? {}),
      dirs: [],
      window_days: windowDays,
      adapter: provider,
      model: model.trim(),
    };
    try {
      const r = await createRun(scope, depth, budgetPerHour, 0);
      setRun(r);
    } catch (e) {
      setErr(String(e));
    }
  }

  async function proposeClusters() {
    if (!run) return;
    setClustering(true);
    try {
      await cluster(run.id);
      setClusters(await listClusters(run.id));
      setRun(await getRun(run.id));
    } catch (e) {
      setErr(String(e));
    } finally {
      setClustering(false);
    }
  }

  async function approve(id: string) {
    await approveCluster(id);
    if (run) setClusters(await listClusters(run.id));
  }
  async function discard(id: string) {
    await discardCluster(id);
    if (run) setClusters(await listClusters(run.id));
  }
  async function doMerge() {
    if (!run || selected.length < 2) return;
    await mergeClusters(run.id, selected);
    setSelected([]);
    setClusters(await listClusters(run.id));
  }

  async function execute() {
    if (!run) return;
    const invocations = report?.estimated_invocations ?? '?';
    const budget = budgetPerHour === 0 ? t('retro.wizard.unlimited') : String(budgetPerHour);
    const depthLabel = depth === 'completa' ? t('retro.wizard.depthFull') : t('retro.wizard.depthLight');
    if (!window.confirm(t('retro.wizard.confirmRun', { invocations, budget, depth: depthLabel }))) return;
    await startRun(run.id);
    setRun(await getRun(run.id));
  }

  const status = run?.status ?? 'inventoried';

  return (
    <div className="retro-wizard">
      {err && <div className="error-banner">{err}</div>}

      {/* Estágio 0/1: inventário + escopo */}
      {!run && (
        <section>
          <h3>{t('retro.wizard.scopeTitle')}</h3>
          {report && (
            <ul className="retro-inventory">
              {Object.entries(report.per_cli).map(([cli, ci]) => (
                <li key={cli}>{cli}: {ci.sessions} {t('retro.wizard.sessions')} ({ci.already_known} {t('retro.wizard.known')})</li>
              ))}
              <li><strong>{t('retro.wizard.estimate')}: {report.estimated_invocations}</strong></li>
            </ul>
          )}
          <label>{t('retro.wizard.window')}: </label>
          {WINDOW_PRESETS.map((w) => (
            <button key={w} className={`btn ${windowDays === w ? 'btn-primary' : 'btn-secondary'}`} style={{ marginRight: '0.25rem' }} onClick={() => setWindowDays(w)}>
              {w === 0 ? t('retro.wizard.all') : `${w}d`}
            </button>
          ))}
          <div>
            <label>{t('retro.wizard.depth')}: </label>
            <select value={depth} onChange={(e) => setDepth(e.target.value as 'completa' | 'leve')}>
              <option value="completa">{t('retro.wizard.depthFull')}</option>
              <option value="leve">{t('retro.wizard.depthLight')}</option>
            </select>
          </div>
          <div>
            <label>{t('retro.wizard.provider')}: </label>
            <select value={provider} onChange={(e) => setProvider(e.target.value as '' | 'claude-code' | 'opencode')}>
              <option value="">{t('retro.wizard.providerDefault')}</option>
              <option value="claude-code">claude-code</option>
              <option value="opencode">opencode</option>
              <option value="gemini">gemini</option>
              <option value="codex">codex</option>
            </select>
          </div>
          <div>
            <label>{t('retro.wizard.model')}: </label>
            <input
              type="text"
              value={model}
              placeholder={provider === 'opencode' ? 'anthropic/claude-sonnet-4-6' : 'claude-sonnet-4-6'}
              onChange={(e) => setModel(e.target.value)}
            />
          </div>
          <div>
            <label>{t('retro.wizard.budgetPerHour')}: </label>
            <input type="number" min={0} value={budgetPerHour} onChange={(e) => setBudgetPerHour(Number(e.target.value))} />
          </div>
          <button className="btn btn-primary" onClick={openRun}>{t('retro.wizard.openRun')}</button>
        </section>
      )}

      {/* Estágio 2: mapa de projetos */}
      {run && (status === 'scoped' || status === 'clustering' || status === 'clustered') && (
        <section>
          <h3>{t('retro.wizard.mapTitle')}</h3>
          {clusters.length === 0 && (
            <button className="btn btn-secondary" onClick={proposeClusters} disabled={clustering || status === 'clustering'}>
              {clustering || status === 'clustering' ? t('retro.wizard.clustering') : t('retro.wizard.propose')}
            </button>
          )}
          {(clustering || status === 'clustering') && (
            <p className="muted pulse">{t('retro.wizard.clusteringHint')}</p>
          )}
          <ul className="retro-clusters">
            {clusters.map((c) => (
              <li key={c.id} className={c.decision !== 'pending' ? 'resolved' : ''}>
                <input type="checkbox" checked={selected.includes(c.id)} onChange={(e) =>
                  setSelected((s) => e.target.checked ? [...s, c.id] : s.filter((x) => x !== c.id))} />
                <strong>{c.name}</strong>
                {c.existing_project_id && <span className="badge">{t('retro.wizard.associated')}</span>}
                <span className="muted"> {c.decision}</span>
                <button className="btn btn-primary" onClick={() => approve(c.id)}>{t('retro.wizard.approve')}</button>
                <button className="btn btn-danger" onClick={() => discard(c.id)}>{t('retro.wizard.discard')}</button>
              </li>
            ))}
          </ul>
          {selected.length >= 2 && <button className="btn btn-secondary" onClick={doMerge}>{t('retro.wizard.merge')}</button>}
          {clusters.length > 0 && <button className="btn btn-primary" onClick={execute}>{t('retro.wizard.execute')}</button>}
        </section>
      )}

      {/* Estágio 3: execução */}
      {run && (status === 'running' || status === 'paused') && (
        <section>
          <h3>{t('retro.wizard.runningTitle')}</h3>
          <p>{t('retro.wizard.status')}: {status}</p>
          {(() => {
            const total = progress?.total ?? 0;
            const doneN = progress?.done ?? 0;
            const pct = total > 0 ? Math.round((doneN / total) * 100) : 0;
            return (
              <div className="inv-progress">
                <div className="inv-progress-label">
                  <span>{t('retro.wizard.progressLabel')}</span>
                  {total > 0 && <span className="mono">{doneN}/{total} · {pct}%</span>}
                </div>
                <div
                  className="inv-progress-track"
                  role="progressbar"
                  aria-valuenow={doneN}
                  aria-valuemin={0}
                  aria-valuemax={total || 100}
                  aria-label={t('retro.wizard.progressLabel')}
                >
                  <div className="inv-progress-fill" style={{ width: `${pct}%` }} />
                </div>
              </div>
            );
          })()}
          <p>
            {t('retro.wizard.sessionsDone')}: {progress?.done ?? 0}/{progress?.total ?? '?'}
          </p>
          <p>
            {t('retro.wizard.budgetUsage')}: {run.llm_calls}
            {run.budget_total > 0 && <> / {run.budget_total}</>} {t('retro.wizard.llmCalls')}
            {run.budget_per_hour > 0 && <> · {t('retro.wizard.perHourCap', { n: run.budget_per_hour })}</>}
          </p>
          {status === 'running' && <button className="btn btn-secondary" onClick={() => pauseRun(run.id).then(() => getRun(run.id)).then(setRun)}>{t('retro.wizard.pause')}</button>}
          {status === 'paused' && <button className="btn btn-secondary" onClick={() => resumeRun(run.id).then(() => getRun(run.id)).then(setRun)}>{t('retro.wizard.resume')}</button>}
          <button className="btn btn-danger" onClick={() => cancelRun(run.id).then(() => getRun(run.id)).then(setRun)}>{t('retro.wizard.cancel')}</button>
        </section>
      )}

      {/* Estágio 4: revisão em lote */}
      {run && status === 'done' && <RetroBatchReview runId={run.id} />}
    </div>
  );
}
