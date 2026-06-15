import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { NavLink } from 'react-router-dom';
import { FanMark } from '../components/Fan';
import NewSessionDropdown from '../components/NewSessionDropdown';
import type { Project, Session } from '../api';

const MYSPACE = '__myspace__';

interface Props {
  projects: Project[];
  wrapperSessions: Session[];
  onStarted: (s: Session) => void;
  onAnalyzeHistory: () => void;
}

// ProjectSidebar lista o grupo virtual MySpace (sessões órfãs, sem project_id)
// fixado no topo, seguido dos projetos reais. Cada grupo tem o ＋ que abre o
// NewSessionDropdown multistep. MySpace inicia sessão livre (projectId null).
export default function ProjectSidebar({ projects, wrapperSessions, onStarted, onAnalyzeHistory }: Props) {
  const { t } = useTranslation();
  const [openFor, setOpenFor] = useState<string | null>(null); // null = nenhum dropdown aberto
  const byProject = (pid: string) => wrapperSessions.filter((s) => s.project_id === pid);
  const orphans = wrapperSessions.filter((s) => !s.project_id);

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
            onClick={() => setOpenFor(openFor === id ? null : id)}
          >＋</button>
          {openFor === id && (
            <NewSessionDropdown
              projectId={dropdownProjectId}
              projectName={name}
              onClose={() => setOpenFor(null)}
              onStarted={handleStarted}
            />
          )}
        </div>
        <div className="sidebar-sessions">
          {sessions.map((s) => (
            <NavLink key={s.id} to={`/sessions/${s.id}`} className="sidebar-session">
              {s.title || s.id.slice(0, 8)}
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
        {row(MYSPACE, 'MySpace', orphans, null)}
        {projects.map((p) => row(p.id, p.name, byProject(p.id), p.id, `/projects/${p.id}`))}
      </nav>

      <div className="sidebar-foot">
        <button className="btn btn-secondary" style={{ width: '100%' }} onClick={onAnalyzeHistory}>
          {t('onboarding.analyzeHistory')}
        </button>
      </div>
    </aside>
  );
}
