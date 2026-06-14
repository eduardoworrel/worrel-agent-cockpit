import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  listProjects,
  createProject,
  listSuggestions,
  listSessions,
} from '../api';
import type { Project, Session } from '../api';
import { listRuns, startInventory } from '../retroApi';
import type { InventoryReport } from '../retroApi';
import { FanHero } from '../components/Fan';
import FolderPicker from '../components/FolderPicker';
import { useEvents } from '../useEvents';

interface Props {
  onPendingCount: (n: number) => void;
}

export default function Dashboard({ onPendingCount }: Props) {
  const { t } = useTranslation();
  const [projects, setProjects] = useState<Project[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [pendingMap, setPendingMap] = useState<Record<string, number>>({});
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [form, setForm] = useState<{ name: string; description: string; dirs: string[] }>({ name: '', description: '', dirs: [] });
  const nameInputRef = useRef<HTMLInputElement>(null);
  const [error, setError] = useState(false);
  const [busy, setBusy] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);
  const [firstRetroUse, setFirstRetroUse] = useState(false);
  const [inv, setInv] = useState<InventoryReport | null>(null);
  const [invLoading, setInvLoading] = useState(true);
  const [invProgress, setInvProgress] = useState<{ cli: string; done: number; total: number } | null>(null);

  useEffect(() => {
    listRuns().then((rs) => setFirstRetroUse(rs.length === 0)).catch(() => {});
  }, []);

  // Inventário local (count-only, sem LLM): dispara o scan ASSÍNCRONO e acompanha
  // o progresso REAL (arquivos de histórico processados) via WebSocket. Serve de
  // atalho para a análise retroativa e torna transparente o que já foi observado.
  useEffect(() => {
    startInventory(0).catch(() => setInvLoading(false));
  }, []);

  useEvents((ev) => {
    if (ev.type === 'retro.inventory.progress') {
      const p = ev.payload as { cli: string; overall_done: number; overall_total: number };
      setInvProgress({ cli: p.cli, done: p.overall_done, total: p.overall_total });
    } else if (ev.type === 'retro.inventory.done') {
      const p = ev.payload as InventoryReport & { error?: string };
      if (!p.error && p.per_cli) setInv(p);
      setInvLoading(false);
      setInvProgress(null);
    }
  });

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      try {
        const [ps, ss, sugs] = await Promise.all([
          listProjects(),
          listSessions(),
          listSuggestions(undefined, 'pending'),
        ]);
        if (cancelled) return;
        setProjects(ps);
        setSessions(ss.filter((s) => s.status === 'active'));
        const map: Record<string, number> = {};
        sugs.forEach((sg) => {
          map[sg.project_id] = (map[sg.project_id] ?? 0) + 1;
        });
        setPendingMap(map);
        onPendingCount(sugs.length);
      } catch {
        if (!cancelled) setError(true);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    load();
    return () => { cancelled = true; };
  }, [reloadKey, onPendingCount]);

  useEffect(() => {
    if (!showModal) return;
    nameInputRef.current?.focus();
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setShowModal(false);
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [showModal]);

  async function handleCreate() {
    if (!form.name || busy) return;
    setBusy(true);
    setError(false);
    try {
      await createProject(form.name, form.description, form.dirs);
      setShowModal(false);
      setForm({ name: '', description: '', dirs: [] });
      setReloadKey((k) => k + 1);
    } catch {
      setError(true);
    } finally {
      setBusy(false);
    }
  }

  if (loading) return <div className="main"><p>{t('common.loading')}</p></div>;

  return (
    <div className="main">
      <div className="page-head">
        <div>
          <h1>{t('dashboard.title')}</h1>
          <p className="sub">{t('dashboard.subtitle')}</p>
        </div>
        <div className="actions">
          <Link
            to="/retro"
            className={firstRetroUse ? 'btn btn-accent' : 'btn btn-secondary'}
            title={firstRetroUse ? t('retro.firstUse') : undefined}
          >
            {t('retro.cta')}
          </Link>
          <button className="btn btn-primary" onClick={() => setShowModal(true)}>
            {t('dashboard.newProject')}
          </button>
        </div>
      </div>

      {error && <p className="error-banner">{t('common.actionFailed')}</p>}

      {projects.length === 0 ? (
        <div className="empty">
          <FanHero width={132} height={68} />
          <h2>{t('dashboard.noProjects')}</h2>
          <p>{t('dashboard.noProjectsHint')}</p>
          <button className="btn btn-accent" style={{ marginTop: 18 }} onClick={() => setShowModal(true)}>
            {t('dashboard.newProject')}
          </button>

          {(() => {
            if (invLoading) {
              const total = invProgress?.total ?? 0;
              const done = invProgress?.done ?? 0;
              const pct = total > 0 ? Math.min(100, Math.round((done / total) * 100)) : 0;
              const label = invProgress
                ? t('dashboard.readingCli', { cli: invProgress.cli })
                : t('dashboard.reading');
              return (
                <div className="inv-progress">
                  <div className="inv-progress-label">
                    <span>{label}</span>
                    {total > 0 && <span className="mono">{done}/{total} · {pct}%</span>}
                  </div>
                  <div
                    className="inv-progress-track"
                    role="progressbar"
                    aria-valuenow={done}
                    aria-valuemin={0}
                    aria-valuemax={total || 100}
                    aria-label={label}
                  >
                    <div className="inv-progress-fill" style={{ width: `${pct}%` }} />
                  </div>
                </div>
              );
            }
            const perCli = inv?.per_cli ?? {};
            const total = Object.values(perCli).reduce((a, c) => a + (c?.sessions ?? 0), 0);
            if (total === 0) return null;
            return (
              <div className="card" style={{ marginTop: 28, maxWidth: 520, textAlign: 'left' }}>
                <h3 style={{ margin: '0 0 4px' }}>{t('dashboard.observedTitle')}</h3>
                <p className="muted" style={{ margin: '0 0 12px', fontSize: '0.88rem' }}>
                  {t('dashboard.observedHint')}
                </p>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 14 }}>
                  {Object.entries(perCli).map(([cli, ci]) => (
                    <span key={cli} className="pill">
                      {cli}: {ci.sessions} {t('dashboard.observedSessions')}
                    </span>
                  ))}
                </div>
                <Link className="btn btn-accent" to="/retro">{t('dashboard.analyzeRetro')}</Link>
              </div>
            );
          })()}
        </div>
      ) : (
        <div className="grid">
          {projects.map((p) => (
            <Link key={p.id} to={`/projects/${p.id}`} className="card clickable">
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 8 }}>
                <h3 style={{ margin: 0, color: 'var(--ink)' }}>{p.name}</h3>
                {(pendingMap[p.id] ?? 0) > 0 && (
                  <span className="badge">{pendingMap[p.id]}</span>
                )}
              </div>
              <p className="muted" style={{ margin: '8px 0 0', fontSize: '0.875rem' }}>
                {p.description || '—'}
              </p>
            </Link>
          ))}
        </div>
      )}

      {sessions.length > 0 && (
        <>
          <h2 style={{ marginTop: 36 }}>{t('dashboard.activeSessions')}</h2>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {sessions.map((s) => (
              <Link key={s.id} to={`/sessions/${s.id}`} className="card clickable" style={{ padding: '12px 16px' }}>
                <strong style={{ color: 'var(--ink)' }}>{s.title || s.id}</strong>
                <span className="mono muted" style={{ marginLeft: 12, fontSize: '0.8rem' }}>
                  {s.adapter} / {s.mode}
                </span>
              </Link>
            ))}
          </div>
        </>
      )}

      {showModal && (
        <div className="modal-overlay" onClick={() => setShowModal(false)}>
          <div
            className="modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="new-project-title"
            onClick={(e) => e.stopPropagation()}
          >
            <h2 id="new-project-title">{t('modal.newProject')}</h2>
            <label htmlFor="np-name">{t('modal.name')}</label>
            <input
              ref={nameInputRef}
              id="np-name"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
            <label htmlFor="np-description" style={{ marginTop: '0.75rem' }}>{t('modal.description')}</label>
            <input
              id="np-description"
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
            />
            <label style={{ marginTop: '0.75rem', display: 'block' }}>{t('modal.dirs')}</label>
            <FolderPicker value={form.dirs} onChange={(dirs) => setForm({ ...form, dirs })} />
            {error && <p className="error-banner">{t('common.actionFailed')}</p>}
            <div style={{ display: 'flex', gap: '0.5rem', marginTop: '1rem' }}>
              <button className="btn btn-primary" disabled={busy} onClick={handleCreate}>{t('modal.create')}</button>
              <button className="btn btn-secondary" onClick={() => setShowModal(false)}>{t('modal.cancel')}</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
