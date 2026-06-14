import { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { listActiveSessions, killSession, listProjects } from '../api';
import type { Session, Project } from '../api';
import { useEvents } from '../useEvents';
import type { WsEvent } from '../useEvents';

export default function SessionTabs() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [projects, setProjects] = useState<Record<string, Project>>({});

  async function refresh() {
    try {
      const [active, projs] = await Promise.all([
        listActiveSessions(),
        listProjects(),
      ]);
      setSessions(active);
      const map: Record<string, Project> = {};
      projs.forEach((p) => { map[p.id] = p; });
      setProjects(map);
    } catch { /* silent */ }
  }

  useEffect(() => { void refresh(); }, []);

  const handleEvent = useCallback((ev: WsEvent) => {
    const relevant = ['session.started', 'session.ended', 'session.classified', 'session.context'];
    if (relevant.includes(ev.type)) void refresh();
  }, []);

  useEvents(handleEvent);

  if (sessions.length === 0) return null;

  function contextPct(s: Session): string {
    if (s.context_used != null && s.context_limit != null && s.context_limit > 0) {
      return `${Math.round((s.context_used / s.context_limit) * 100)}%`;
    }
    return '';
  }

  async function handleKill(e: React.MouseEvent, id: string) {
    e.stopPropagation();
    try {
      await killSession(id);
      await refresh();
    } catch { /* silent */ }
  }

  return (
    <div className="session-strip">
      <span className="session-strip-label">{t('sessions.active')}:</span>
      {sessions.map((s) => {
        const proj = s.project_id ? projects[s.project_id] : null;
        const title = proj ? proj.name : t('sessions.unclassified');
        const pct = contextPct(s);
        return (
          <div
            key={s.id}
            className="tab session-tab"
            onClick={() => navigate(`/sessions/${s.id}`)}
          >
            <span className="session-tab-title">{title}</span>
            {pct && <span className="session-tab-pct">{pct}</span>}
            <button
              className="session-tab-close"
              title={t('terminal.kill')}
              aria-label={t('terminal.kill')}
              onClick={(e) => void handleKill(e, s.id)}
            >
              ×
            </button>
          </div>
        );
      })}
    </div>
  );
}
