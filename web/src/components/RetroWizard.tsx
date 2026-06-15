import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useEvents } from '../useEvents';
import type { WsEvent } from '../useEvents';
import {
  inventory, createRun, cluster, listClusters, approveCluster, discardCluster,
  mergeClusters, startRun, pauseRun, resumeRun, cancelRun, getRun, listAdapterModels,
  type InventoryReport, type RetroRun, type RetroCluster, type Scope,
  type RetroRunProgress,
} from '../retroApi';
import RetroBatchReview from './RetroBatchReview';
import RetroRangePicker, { type RangeValue } from './RetroRangePicker';

const PROVIDERS = ['', 'claude-code', 'opencode', 'gemini', 'codex', 'pidev'];

type StageKey = 'inventory' | 'scope' | 'map' | 'execution' | 'review';
const STAGE_ORDER: StageKey[] = ['inventory', 'scope', 'map', 'execution', 'review'];

function stageFromStatus(run: RetroRun | null): StageKey {
  if (!run) return 'scope';
  switch (run.status) {
    case 'scoped':
    case 'clustering':
    case 'clustered':
      return 'map';
    case 'running':
    case 'paused':
      return 'execution';
    case 'done':
      return 'review';
    default:
      return 'scope';
  }
}

function Stepper({ active }: { active: StageKey }) {
  const { t } = useTranslation();
  const activeIdx = STAGE_ORDER.indexOf(active);
  return (
    <ol className="retro-stepper" aria-label={t('retro.title')}>
      {STAGE_ORDER.map((s, i) => {
        const state = i < activeIdx ? 'done' : i === activeIdx ? 'current' : 'todo';
        return (
          <li key={s} className={`retro-step ${state}`} aria-current={state === 'current' ? 'step' : undefined}>
            <span className="retro-step-dot" aria-hidden="true">
              {state === 'done' ? '✓' : i + 1}
            </span>
            <span className="retro-step-label">{t(`retro.wizard.steps.${s}`)}</span>
          </li>
        );
      })}
    </ol>
  );
}

export default function RetroWizard() {
  const { t } = useTranslation();
  const [report, setReport] = useState<InventoryReport | null>(null);
  const [loadingInventory, setLoadingInventory] = useState(true);
  const [range, setRange] = useState<RangeValue>({ since: 0, until: 0 });
  const [depth, setDepth] = useState<'completa' | 'leve'>('completa');
  const [provider, setProvider] = useState('');
  const [model, setModel] = useState('');
  const [models, setModels] = useState<string[]>([]);
  const [modelsLoading, setModelsLoading] = useState(false);
  const [modelFreeText, setModelFreeText] = useState(false);
  const [budgetPerHour, setBudgetPerHour] = useState(0);
  const [excludedClis, setExcludedClis] = useState<Record<string, boolean>>({}); // CLIs desmarcados (fora do histórico)
  const [run, setRun] = useState<RetroRun | null>(null);
  const [clusters, setClusters] = useState<RetroCluster[]>([]);
  const [selected, setSelected] = useState<string[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [clustering, setClustering] = useState(false);
  const [progress, setProgress] = useState<RetroRunProgress | null>(null);

  const refreshInventory = useCallback(async () => {
    setLoadingInventory(true);
    try {
      setReport(await inventory(0)); // varre tudo; o recorte é feito no cliente via range
    } catch (e) {
      setErr(String(e));
    } finally {
      setLoadingInventory(false);
    }
  }, []);

  useEffect(() => {
    refreshInventory();
  }, [refreshInventory]);

  // Busca modelos do provider; degrada para texto livre se vazio/404.
  useEffect(() => {
    let cancelled = false;
    setModel('');
    setModelFreeText(false);
    if (!provider) {
      setModels([]);
      return;
    }
    setModelsLoading(true);
    listAdapterModels(provider)
      .then((ms) => {
        if (cancelled) return;
        setModels(ms);
        setModelFreeText(ms.length === 0);
      })
      .finally(() => !cancelled && setModelsLoading(false));
    return () => { cancelled = true; };
  }, [provider]);

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
      clis: Object.keys(report?.per_cli ?? {}).filter((c) => !excludedClis[c]),
      dirs: [],
      window_days: 0,
      since: range.since,
      until: range.until,
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
  const stage = stageFromStatus(run);

  const cliEntries = Object.entries(report?.per_cli ?? {});

  return (
    <div className="retro-wizard">
      <Stepper active={stage} />

      {err && <div className="error-banner" role="alert">{err}</div>}

      {/* Estágio 0/1: inventário + escopo */}
      {!run && (
        <section className="retro-card">
          <header className="retro-card-head">
            <h3>{t('retro.wizard.scopeTitle')}</h3>
            <p className="muted">{t('retro.wizard.scopeSubtitle')}</p>
          </header>

          {/* Período: recorte data-driven derivado do inventário */}
          <div className="retro-field">
            <span className="retro-field-label">{t('retro.wizard.window')}</span>
            <RetroRangePicker
              report={report}
              excludedClis={excludedClis}
              value={range}
              onChange={setRange}
            />
          </div>

          {/* Inventário + seleção de quais CLIs entram no histórico */}
          <div className="retro-field">
            <span className="retro-field-label">{t('retro.wizard.inventoryTitle')}</span>
            {loadingInventory ? (
              <div className="retro-inventory-panel muted">
                <span className="pulse">{t('retro.wizard.inventoryLoading')}</span>
              </div>
            ) : cliEntries.length === 0 ? (
              <div className="retro-inventory-panel muted">{t('retro.wizard.inventoryEmpty')}</div>
            ) : (
              <div className="retro-inventory-panel">
                <ul className="retro-inventory">
                  {cliEntries.map(([cli, ci]) => (
                    <li key={cli}>
                      <label className="retro-inv-toggle">
                        <input
                          type="checkbox"
                          checked={!excludedClis[cli]}
                          onChange={(e) =>
                            setExcludedClis((m) => ({ ...m, [cli]: !e.target.checked }))
                          }
                        />
                        <span className="retro-inv-cli mono">{cli === 'pidev' ? 'pi' : cli}</span>
                      </label>
                      <span className="retro-inv-meta">
                        {ci.sessions} {t('retro.wizard.sessions')}
                        <span className="faint"> · {ci.already_known} {t('retro.wizard.known')}</span>
                      </span>
                    </li>
                  ))}
                </ul>
                <span className="retro-field-hint">{t('retro.wizard.includeHint')}</span>
                {report && (
                  <div className="retro-estimate">
                    <span className="muted">{t('retro.wizard.estimate')}</span>
                    <strong className="mono">{report.estimated_invocations}</strong>
                  </div>
                )}
              </div>
            )}
          </div>

          {/* Profundidade */}
          <div className="retro-field">
            <span className="retro-field-label">{t('retro.wizard.depth')}</span>
            <div className="retro-segmented" role="group" aria-label={t('retro.wizard.depth')}>
              <button
                type="button"
                className={`retro-seg ${depth === 'completa' ? 'active' : ''}`}
                aria-pressed={depth === 'completa'}
                onClick={() => setDepth('completa')}
              >
                {t('retro.wizard.depthFull')}
              </button>
              <button
                type="button"
                className={`retro-seg ${depth === 'leve' ? 'active' : ''}`}
                aria-pressed={depth === 'leve'}
                onClick={() => setDepth('leve')}
              >
                {t('retro.wizard.depthLight')}
              </button>
            </div>
            <span className="retro-field-hint">{t('retro.wizard.depthHint')}</span>
          </div>

          {/* Provider + modelo */}
          <div className="retro-grid-2">
            <div className="retro-field">
              <label htmlFor="retro-provider">{t('retro.wizard.provider')}</label>
              <select className="retro-select" id="retro-provider" value={provider} onChange={(e) => setProvider(e.target.value)}>
                {PROVIDERS.map((p) => (
                  <option key={p || 'default'} value={p}>
                    {p === '' ? t('retro.wizard.providerDefault') : p === 'pidev' ? 'pi' : p}
                  </option>
                ))}
              </select>
            </div>
            <div className="retro-field">
              <label htmlFor="retro-model">
                {t('retro.wizard.model')} <span className="faint">({t('retro.wizard.modelOptional')})</span>
              </label>
              {modelsLoading ? (
                <div className="retro-inline-loading muted pulse">{t('retro.wizard.modelLoading')}</div>
              ) : models.length > 0 && !modelFreeText ? (
                <select
                  className="retro-select"
                  id="retro-model"
                  value={model}
                  onChange={(e) => {
                    if (e.target.value === '__custom__') {
                      setModelFreeText(true);
                      setModel('');
                    } else {
                      setModel(e.target.value);
                    }
                  }}
                >
                  <option value="">{t('retro.wizard.modelSelectPlaceholder')}</option>
                  {models.map((m) => (
                    <option key={m} value={m}>{m}</option>
                  ))}
                  <option value="__custom__">{t('retro.wizard.modelCustom')}</option>
                </select>
              ) : (
                <>
                  <input
                    id="retro-model"
                    type="text"
                    value={model}
                    placeholder={provider === 'opencode' ? 'anthropic/claude-sonnet-4-6' : 'claude-sonnet-4-6'}
                    onChange={(e) => setModel(e.target.value)}
                  />
                  <span className="retro-field-hint">{t('retro.wizard.modelFreeHint')}</span>
                </>
              )}
            </div>
          </div>

          {/* Avançado: limite de chamadas/hora (oculto por padrão; raramente necessário) */}
          <details className="retro-advanced">
            <summary>{t('retro.wizard.advanced')}</summary>
            <div className="retro-field">
              <label htmlFor="retro-budget">{t('retro.wizard.budgetPerHour')}</label>
              <input
                id="retro-budget"
                type="number"
                min={0}
                value={budgetPerHour}
                onChange={(e) => setBudgetPerHour(Number(e.target.value))}
              />
              <span className="retro-field-hint">{t('retro.wizard.budgetPerHourHint')}</span>
            </div>
          </details>

          <div className="retro-card-foot">
            <button className="btn btn-accent" onClick={openRun} disabled={cliEntries.length === 0}>
              {t('retro.wizard.openRun')} →
            </button>
          </div>
        </section>
      )}

      {/* Estágio 2: mapa de projetos */}
      {run && (status === 'scoped' || status === 'clustering' || status === 'clustered') && (
        <section className="retro-card">
          <header className="retro-card-head">
            <h3>{t('retro.wizard.mapTitle')}</h3>
            <p className="muted">{t('retro.wizard.mapSubtitle')}</p>
          </header>

          {clusters.length === 0 && !(clustering || status === 'clustering') && (
            <div className="retro-empty-inline">
              <p className="muted">{t('retro.wizard.mapEmpty')}</p>
              <button className="btn btn-accent" onClick={proposeClusters}>
                {t('retro.wizard.propose')}
              </button>
            </div>
          )}

          {(clustering || status === 'clustering') && (
            <div className="retro-empty-inline">
              <p className="muted pulse">{t('retro.wizard.clusteringHint')}</p>
            </div>
          )}

          {clusters.length > 0 && (
            <>
              <ul className="retro-clusters">
                {clusters.map((c) => {
                  const sids = (c.session_ids || '').split(',').filter(Boolean).length;
                  const resolved = c.decision !== 'pending';
                  const decisionKey =
                    c.decision === 'approved' ? 'decisionApproved'
                    : c.decision === 'discarded' ? 'decisionDiscarded'
                    : 'decisionPending';
                  return (
                    <li key={c.id} className={`retro-cluster ${resolved ? 'resolved' : ''}`}>
                      <label className="retro-cluster-check">
                        <input
                          type="checkbox"
                          checked={selected.includes(c.id)}
                          disabled={resolved}
                          aria-label={c.name}
                          onChange={(e) =>
                            setSelected((s) => (e.target.checked ? [...s, c.id] : s.filter((x) => x !== c.id)))
                          }
                        />
                      </label>
                      <div className="retro-cluster-body">
                        <div className="retro-cluster-head">
                          <strong className="retro-cluster-name">{c.name}</strong>
                          {c.existing_project_id && <span className="pill green">{t('retro.wizard.associated')}</span>}
                          <span className={`pill retro-decision-${c.decision}`}>{t(`retro.wizard.${decisionKey}`)}</span>
                        </div>
                        {c.description && <p className="retro-cluster-desc muted">{c.description}</p>}
                        <span className="retro-cluster-meta faint">{t('retro.wizard.clusterSessions', { n: sids })}</span>
                      </div>
                      {!resolved && (
                        <div className="retro-cluster-actions">
                          <button className="btn btn-secondary btn-sm" onClick={() => approve(c.id)}>{t('retro.wizard.approve')}</button>
                          <button className="btn btn-danger btn-sm" onClick={() => discard(c.id)}>{t('retro.wizard.discard')}</button>
                        </div>
                      )}
                    </li>
                  );
                })}
              </ul>

              <div className="retro-card-foot retro-card-foot-split">
                <button className="btn btn-secondary" onClick={doMerge} disabled={selected.length < 2}>
                  {selected.length < 2 ? t('retro.wizard.selectToMerge') : t('retro.wizard.merge', { n: selected.length })}
                </button>
                <button className="btn btn-accent" onClick={execute}>{t('retro.wizard.execute')} →</button>
              </div>
            </>
          )}
        </section>
      )}

      {/* Estágio 3: execução */}
      {run && (status === 'running' || status === 'paused') && (
        <section className="retro-card">
          <header className="retro-card-head">
            <h3>{t('retro.wizard.runningTitle')}</h3>
            <p className="muted">{t('retro.wizard.runningSubtitle')}</p>
          </header>

          <div className="retro-run-status">
            <span className={`pill ${status === 'running' ? 'sky' : 'amber'}`}>
              {status === 'running' ? t('retro.wizard.statusRunning') : t('retro.wizard.statusPaused')}
            </span>
            {progress?.project_id && (
              <span className="retro-run-meta">
                {t('retro.wizard.currentProject')}: <strong className="mono">{progress.project_id}</strong>
              </span>
            )}
            {typeof progress?.batch === 'number' && (
              <span className="retro-run-meta faint">{t('retro.wizard.batchLabel')} {progress.batch}</span>
            )}
          </div>

          {(() => {
            const total = progress?.total ?? 0;
            const doneN = progress?.done ?? 0;
            const pct = total > 0 ? Math.round((doneN / total) * 100) : 0;
            return (
              <div className="inv-progress retro-progress">
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

          <dl className="retro-run-stats">
            <div>
              <dt>{t('retro.wizard.sessionsDone')}</dt>
              <dd className="mono">{progress?.done ?? 0}/{progress?.total ?? '?'}</dd>
            </div>
            <div>
              <dt>{t('retro.wizard.budgetUsage')}</dt>
              <dd className="mono">
                {run.llm_calls}{run.budget_total > 0 && <> / {run.budget_total}</>} {t('retro.wizard.llmCalls')}
                {run.budget_per_hour > 0 && <span className="faint"> · {t('retro.wizard.perHourCap', { n: run.budget_per_hour })}</span>}
              </dd>
            </div>
          </dl>

          <div className="retro-card-foot retro-card-foot-split">
            <div className="retro-run-controls">
              {status === 'running' && (
                <button className="btn btn-secondary" onClick={() => pauseRun(run.id).then(() => getRun(run.id)).then(setRun)}>
                  {t('retro.wizard.pause')}
                </button>
              )}
              {status === 'paused' && (
                <button className="btn btn-accent" onClick={() => resumeRun(run.id).then(() => getRun(run.id)).then(setRun)}>
                  {t('retro.wizard.resume')}
                </button>
              )}
            </div>
            <button className="btn btn-danger" onClick={() => cancelRun(run.id).then(() => getRun(run.id)).then(setRun)}>
              {t('retro.wizard.cancel')}
            </button>
          </div>
        </section>
      )}

      {/* Estágio 4: revisão em lote */}
      {run && status === 'done' && <RetroBatchReview runId={run.id} />}
    </div>
  );
}
