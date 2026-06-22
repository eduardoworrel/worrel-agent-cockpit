import { useState } from 'react';
import { NavLink } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { FanMark } from '../components/Fan';
import { ProviderBadge } from '../session';
import type { Project, Session } from '../api';

interface Props {
  projects: Project[];
  // Todas as sessões wrapper (vivas + encerradas) e o conjunto das vivas.
  sessions: Session[];
  liveIds: Set<string>;
  awaitingIds: Set<string>;
}

// AppNav é a navegação principal. O item "terminals" não navega: expande, no
// próprio sidebar, a lista de terminais ATIVOS (agrupados por projeto, incluindo
// "sem projeto") e um HISTÓRICO das sessões encerradas.
export default function AppNav({ projects, sessions, liveIds, awaitingIds }: Props) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(true);

  const live = sessions.filter((s) => liveIds.has(s.id));
  const ended = sessions.filter((s) => !liveIds.has(s.id));
  const byProject = (pid: string) => live.filter((s) => s.project_id === pid);
  const orphans = live.filter((s) => !s.project_id);

  function item(s: Session, isLive: boolean) {
    const name = s.title?.trim() || t('terminals.untitled', 'Sessão');
    const time = s.started_at
      ? new Date(s.started_at).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
      : '';
    return (
      <NavLink key={s.id} to={`/sessions/${s.id}`}
        className={`appnav-term${awaitingIds.has(s.id) ? ' needs-attention' : ''}${isLive ? ' live' : ''}`}>
        <span className="appnav-term-top">
          <ProviderBadge adapter={s.adapter} />
          <span className="appnav-term-time">{time}</span>
        </span>
        <span className="appnav-term-name">
          {isLive && <span className="appnav-live-dot" aria-hidden="true" />}
          <span className="appnav-term-label">{name}</span>
        </span>
      </NavLink>
    );
  }

  function group(name: string, list: Session[]) {
    if (list.length === 0) return null;
    return (
      <div className="appnav-term-group" key={name}>
        <div className="appnav-term-proj">{name}</div>
        {list.map((s) => item(s, true))}
      </div>
    );
  }

  return (
    <aside className="appnav">
      <div className="appnav-brand">
        <FanMark size={22} />
        Worrel
      </div>
      <nav className="appnav-links">
        <NavLink to="/" end className="appnav-link">{t('home.nav.home')}</NavLink>
        <span className="appnav-link disabled">{t('home.nav.metrics')}</span>
        <span className="appnav-link disabled">{t('home.nav.joystick')}</span>
        <span className="appnav-link disabled">{t('home.nav.lab')}</span>

        <button className={`appnav-link strong appnav-toggle${open ? ' open' : ''}`}
          onClick={() => setOpen((v) => !v)} aria-expanded={open}>
          {t('home.nav.terminals')}
          {live.length > 0 && <span className="appnav-term-count">{live.length}</span>}
        </button>

        {open && (
          <div className="appnav-terms">
            {live.length === 0 && <div className="appnav-term-empty">{t('terminals.empty')}</div>}
            {group(t('home.wizard.noProject'), orphans)}
            {projects.map((p) => group(p.name, byProject(p.id)))}

            {ended.length > 0 && (
              <div className="appnav-term-group">
                <div className="appnav-term-proj appnav-term-history">{t('terminals.history')}</div>
                {ended.slice(0, 8).map((s) => item(s, false))}
              </div>
            )}
          </div>
        )}
      </nav>
      <div className="appnav-foot">
        <NavLink to="/settings" className="appnav-link">{t('nav.settings')}</NavLink>
      </div>
    </aside>
  );
}
