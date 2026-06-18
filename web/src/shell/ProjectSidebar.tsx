import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { NavLink } from 'react-router-dom';
import { FanMark } from '../components/Fan';
import NewSessionDropdown from '../components/NewSessionDropdown';
import type { Project, Session } from '../api';
import { sessionName, ProviderBadge } from '../session';

const MYSPACE = '__myspace__';

interface Props {
  projects: Project[];
  wrapperSessions: Session[];
  liveIds: Set<string>;
  // awaitingIds: sessões cujo CLI terminou o turno e aguarda o usuário (resposta
  // ou confirmação) → marca a sessão com um alerta visual na sidebar.
  awaitingIds: Set<string>;
  onStarted: (s: Session) => void;
}

// ProjectSidebar lista o grupo virtual MySpace (sessões órfãs, sem project_id)
// fixado no topo, seguido dos projetos reais. Cada grupo tem o ＋ que abre o
// NewSessionDropdown multistep. MySpace inicia sessão livre (projectId null).
// Só sessões VIVAS (com processo ativo) aparecem; encerradas saem da sidebar.
export default function ProjectSidebar({ projects, wrapperSessions, liveIds, awaitingIds, onStarted }: Props) {
  const { t } = useTranslation();
  const [openFor, setOpenFor] = useState<string | null>(null); // null = nenhum dropdown aberto
  const [anchor, setAnchor] = useState<{ top: number; left: number } | null>(null);
  const live = wrapperSessions.filter((s) => liveIds.has(s.id));
  const byProject = (pid: string) => live.filter((s) => s.project_id === pid);
  const orphans = live.filter((s) => !s.project_id);

  function handleStarted(s: Session) {
    setOpenFor(null);
    onStarted(s);
  }

  function row(id: string, name: string, sessions: Session[], dropdownProjectId: string | null, to?: string) {
    return (
      <div key={id} className="sidebar-project">
        <div className="sidebar-project-head">
          {to ? (
            <NavLink to={to} className="sidebar-project-name">{name}</NavLink>
          ) : (
            <span className="sidebar-project-name">{name}</span>
          )}
          <button
            className="sidebar-new-btn"
            aria-label={t('sidebar.newSessionIn', { name })}
            onClick={(e) => {
              if (openFor === id) { setOpenFor(null); return; }
              const r = e.currentTarget.getBoundingClientRect();
              setAnchor({ top: r.bottom + 4, left: r.left });
              setOpenFor(id);
            }}
          >＋</button>
          {openFor === id && anchor && (
            <NewSessionDropdown
              projectId={dropdownProjectId}
              anchor={anchor}
              onClose={() => setOpenFor(null)}
              onStarted={handleStarted}
            />
          )}
        </div>
        <div className="sidebar-sessions">
          {sessions.map((s) => (
            <NavLink
              key={s.id}
              to={`/sessions/${s.id}`}
              className={`sidebar-session${awaitingIds.has(s.id) ? ' needs-attention' : ''}`}
              title={awaitingIds.has(s.id) ? t('sidebar.awaiting') : undefined}
            >
              <span className="sidebar-session-body">
                <span className="sidebar-session-name">{sessionName(s)}</span>
                <ProviderBadge adapter={s.adapter} />
              </span>
            </NavLink>
          ))}
        </div>
      </div>
    );
  }

  return (
    <aside className="sidebar">
      <div className="sidebar-title">
        <FanMark size={22} />
        Worrel
      </div>
      <div className="sidebar-section">{t('sidebar.projects')}</div>

      <nav className="sidebar-projects">
        {row(MYSPACE, 'no project', orphans, null)}
        {projects.map((p) => row(p.id, p.name, byProject(p.id), p.id, `/projects/${p.id}`))}
      </nav>

      <div className="sidebar-foot">
        <NavLink to="/settings" className="sidebar-settings-link">⚙ {t('nav.settings')}</NavLink>
      </div>
    </aside>
  );
}
