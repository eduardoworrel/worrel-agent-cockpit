import { useCallback, useEffect, useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  listActiveSessions,
  listProjects,
  killSession,
  classifySession,
  promoteSession,
} from '../api';
import type { Session, Project } from '../api';
import { useEvents } from '../useEvents';
import type { WsEvent } from '../useEvents';

interface ClassifyPopoverProps {
  session: Session;
  projects: Project[];
  onDone: () => void;
}

function ClassifyPopover({ session, projects, onDone }: ClassifyPopoverProps) {
  const { t } = useTranslation();
  const [mode, setMode] = useState<'existing' | 'new'>('existing');
  const [projectId, setProjectId] = useState(projects[0]?.id ?? '');
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function onClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) onDone();
    }
    document.addEventListener('mousedown', onClick);
    return () => document.removeEventListener('mousedown', onClick);
  }, [onDone]);

  async function handleClassify() {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      if (mode === 'existing') {
        await classifySession(session.id, projectId);
      } else {
        await promoteSession(session.id, name, description);
      }
      onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.actionFailed'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div
      ref={ref}
      style={{
        position: 'absolute',
        zIndex: 100,
        background: 'var(--surface)',
        border: '1px solid var(--line)',
        borderRadius: '6px',
        padding: '1rem',
        minWidth: '260px',
        boxShadow: '0 4px 16px rgba(0,0,0,0.4)',
      }}
    >
      {error && <p className="error-banner">{error}</p>}
      <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '0.75rem' }}>
        <button
          className={`btn${mode === 'existing' ? ' btn-primary' : ' btn-secondary'}`}
          style={{ flex: 1, fontSize: '0.8rem' }}
          onClick={() => setMode('existing')}
        >
          {t('sessions.classifyExisting')}
        </button>
        <button
          className={`btn${mode === 'new' ? ' btn-primary' : ' btn-secondary'}`}
          style={{ flex: 1, fontSize: '0.8rem' }}
          onClick={() => setMode('new')}
        >
          {t('sessions.createScope')}
        </button>
      </div>

      {mode === 'existing' ? (
        projects.length === 0 ? (
          <p style={{ color: 'var(--muted)', fontSize: '0.85rem' }}>No projects.</p>
        ) : (
          <select
            value={projectId}
            onChange={(e) => setProjectId(e.target.value)}
            style={{ width: '100%', marginBottom: '0.5rem' }}
          >
            {projects.map((p) => (
              <option key={p.id} value={p.id}>{p.name}</option>
            ))}
          </select>
        )
      ) : (
        <>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t('modal.name')}
            style={{ width: '100%', marginBottom: '0.5rem' }}
          />
          <input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t('modal.description')}
            style={{ width: '100%', marginBottom: '0.5rem' }}
          />
        </>
      )}

      <div style={{ display: 'flex', gap: '0.5rem' }}>
        <button
          className="btn btn-primary"
          disabled={busy || (mode === 'existing' ? !projectId : !name)}
          onClick={handleClassify}
          style={{ flex: 1 }}
        >
          {busy ? t('common.loading') : t('sessions.classify')}
        </button>
        <button className="btn btn-secondary" onClick={onDone}>
          {t('common.cancel')}
        </button>
      </div>
    </div>
  );
}

export default function Sessions() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectMap, setProjectMap] = useState<Record<string, Project>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [classifyOpen, setClassifyOpen] = useState<string | null>(null);

  async function refresh() {
    try {
      const [active, projs] = await Promise.all([
        listActiveSessions(),
        listProjects(),
      ]);
      setSessions(active);
      setProjects(projs);
      const map: Record<string, Project> = {};
      projs.forEach((p) => { map[p.id] = p; });
      setProjectMap(map);
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void refresh(); }, []);

  const handleEvent = useCallback((ev: WsEvent) => {
    const relevant = ['session.started', 'session.ended', 'session.classified', 'session.context'];
    if (relevant.includes(ev.type)) void refresh();
  }, []);

  useEvents(handleEvent);

  async function handleKill(id: string) {
    try {
      await killSession(id);
      await refresh();
    } catch { /* silent */ }
  }

  function contextPct(s: Session): string {
    if (s.context_used != null && s.context_limit != null && s.context_limit > 0) {
      return `${Math.round((s.context_used / s.context_limit) * 100)}%`;
    }
    return '–';
  }

  if (loading) return <div className="main"><p>{t('common.loading')}</p></div>;

  return (
    <div className="main">
      <h1>{t('sessions.hub')}</h1>
      {error && <p className="error-banner">{t('common.actionFailed')}</p>}

      {sessions.length === 0 ? (
        <p style={{ color: 'var(--muted)' }}>{t('sessions.noSessions')}</p>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
          {sessions.map((s) => {
            const proj = s.project_id ? projectMap[s.project_id] : null;
            return (
              <div key={s.id} className="card" style={{ position: 'relative' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: '1rem', flexWrap: 'wrap' }}>
                  <div>
                    {proj ? (
                      <strong>
                        <Link to={`/projects/${proj.id}`} style={{ color: 'var(--orange-ink)' }}>{proj.name}</Link>
                      </strong>
                    ) : (
                      <span style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                        <span className="pill">{t('sessions.unclassified')}</span>
                        <button
                          className="btn btn-secondary"
                          style={{ fontSize: '0.8rem' }}
                          onClick={() => setClassifyOpen(classifyOpen === s.id ? null : s.id)}
                        >
                          {t('sessions.classify')}
                        </button>
                      </span>
                    )}
                    {classifyOpen === s.id && (
                      <div style={{ marginTop: '0.5rem' }}>
                        <ClassifyPopover
                          session={s}
                          projects={projects}
                          onDone={() => { setClassifyOpen(null); void refresh(); }}
                        />
                      </div>
                    )}
                    <div style={{ marginTop: '0.35rem', fontSize: '0.85rem', color: 'var(--muted)' }}>
                      {t('sessions.adapter')}: {s.adapter} &nbsp;·&nbsp; {t('sessions.contextBar', { used: s.context_used ?? '–', limit: s.context_limit ?? '–' })} ({contextPct(s)})
                    </div>
                  </div>

                  <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
                    <button
                      className="btn btn-primary"
                      style={{ fontSize: '0.85rem' }}
                      onClick={() => navigate(`/sessions/${s.id}`)}
                    >
                      {t('sessions.openTerminal')}
                    </button>
                    {proj && (
                      <Link
                        to={`/projects/${proj.id}`}
                        className="btn btn-secondary"
                        style={{ fontSize: '0.85rem' }}
                      >
                        {t('nav.dashboard')}
                      </Link>
                    )}
                    <button
                      className="btn btn-danger"
                      style={{ fontSize: '0.85rem' }}
                      onClick={() => void handleKill(s.id)}
                    >
                      {t('terminal.kill')}
                    </button>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
