import { useState } from 'react';
import { NavLink } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { FanMark } from '../components/Fan';
import { ProviderBadge } from '../session';
import { killSession, archiveSession } from '../api';
import { useFlip } from './useFlip';
import type { Project, Session } from '../api';
import type { SessionStatus } from '../sessionStatus';

interface Props {
  projects: Project[];
  // Todas as sessões wrapper (vivas + encerradas) e o conjunto das vivas.
  sessions: Session[];
  liveIds: Set<string>;
  // statusById: farol de estado por sessão, derivado pela MESMA função do card
  // da Home (ver sessionStatus) — mantém a bolinha da sidebar em sincronia com a
  // da miniatura.
  statusById: Record<string, SessionStatus>;
  // onChanged recarrega o estado da app após uma ação (encerrar/arquivar) para
  // que a sidebar reflita o novo estado da sessão.
  onChanged: () => void;
}

// AppNav é a navegação principal. O item "terminals" não navega: expande, no
// próprio sidebar, a lista de terminais ATIVOS (agrupados por projeto, incluindo
// "sem projeto") e um HISTÓRICO das sessões encerradas. Cada item ganha as ações
// por estado: vivas → Encerrar; encerradas → Arquivar.
export default function AppNav({ projects, sessions, liveIds, statusById, onChanged }: Props) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(true);
  const [busy, setBusy] = useState(false);
  // Estado de colapso do histórico por projeto (chave = group.key, não índice:
  // o índice muda quando os grupos reordenam, a chave acompanha o projeto).
  const [histOpen, setHistOpen] = useState<Record<string, boolean>>({});
  // Alvos dos modais de confirmação (a ação destrutiva só acontece após confirmar).
  const [killTarget, setKillTarget] = useState<Session | null>(null);
  const [archiveTarget, setArchiveTarget] = useState<Session | null>(null);

  const live = sessions.filter((s) => liveIds.has(s.id));
  const ended = sessions.filter((s) => !liveIds.has(s.id));
  const byProject = (pid: string) => live.filter((s) => s.project_id === pid);
  const orphans = live.filter((s) => !s.project_id);

  // Recência de uma sessão encerrada = fim, ou início como fallback.
  const recency = (s: Session) => s.ended_at ?? s.started_at ?? 0;
  const projName = (pid: string) =>
    projects.find((p) => p.id === pid)?.name || t('home.wizard.noProject');

  // Histórico agrupado por projeto. Sessões e grupos ordenam pela ATIVIDADE MAIS
  // RECENTE (não por nome/criação): o projeto cuja última sessão é mais nova sobe
  // ao topo — como uma lista de conversas. Reordena reativamente a cada render.
  const ORPHAN = '__orphan__';
  const historyGroups = (() => {
    const map = new Map<string, Session[]>();
    for (const s of ended) {
      const key = s.project_id || ORPHAN;
      const arr = map.get(key);
      if (arr) arr.push(s); else map.set(key, [s]);
    }
    return [...map.entries()]
      .map(([key, list]) => {
        const sessions = [...list].sort((a, b) => recency(b) - recency(a));
        return {
          key,
          name: key === ORPHAN ? t('home.wizard.noProject') : projName(key),
          sessions,
          last: recency(sessions[0]),
        };
      })
      .sort((a, b) => b.last - a.last);
  })();

  // Anima a reordenação dos grupos quando a recência muda a ordem.
  const flipRef = useFlip(historyGroups.map((g) => g.key));

  // Tempo relativo curto para o header do grupo ("agora", "12min", "3h", "5d").
  function timeAgo(ts: number): string {
    if (!ts) return '';
    const min = Math.round((Date.now() - ts) / 60000);
    if (min < 1) return t('time.now', 'agora') as string;
    if (min < 60) return `${min}min`;
    const h = Math.round(min / 60);
    if (h < 24) return `${h}h`;
    return `${Math.round(h / 24)}d`;
  }

  // run serializa as ações (evita duplo clique) e recarrega o estado ao final.
  async function run(fn: () => Promise<unknown>) {
    if (busy) return;
    setBusy(true);
    try { await fn(); onChanged(); } catch { /* preserva o último estado bom */ } finally { setBusy(false); }
  }

  function handleKill(sessionId: string) {
    return run(async () => {
      await killSession(sessionId);
      setKillTarget(null);
    });
  }

  function handleArchive(sessionId: string) {
    return run(async () => {
      await archiveSession(sessionId);
      setArchiveTarget(null);
    });
  }

  function item(s: Session, isLive: boolean) {
    const name = s.title?.trim() || t('terminals.untitled', 'Sessão');
    const time = s.started_at
      ? new Date(s.started_at).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
      : '';
    // Farol de estado vindo do App (mesma derivação do card da Home): a bolinha
    // segue o estado real (verde = trabalhando, âmbar = esperando você).
    const status = statusById[s.id];
    return (
      <div key={s.id} className="appnav-term-wrap">
        <NavLink to={`/sessions/${s.id}`}
          className={`appnav-term${status === 'awaiting' ? ' needs-attention' : ''}${isLive ? ' live' : ''}`}>
          <span className="appnav-term-top">
            <ProviderBadge adapter={s.adapter} />
            <span className="appnav-term-time">{time}</span>
          </span>
          <span className="appnav-term-name">
            {isLive && <span className="appnav-live-dot" data-state={status} aria-hidden="true" />}
            <span className="appnav-term-label">{name}</span>
          </span>
        </NavLink>
        <span className="appnav-term-actions">
          {isLive ? (
            <button
              className="appnav-term-act"
              disabled={busy}
              title={t('sessions.endHint', 'Encerra o processo do agente desta sessão') as string}
              aria-label={t('sessions.end', 'Encerrar') as string}
              onClick={() => setKillTarget(s)}
            >⨯</button>
          ) : (
              <button
                className="appnav-term-act"
                disabled={busy}
                title={t('sessions.archive') as string}
                aria-label={t('sessions.archive') as string}
                onClick={() => setArchiveTarget(s)}
              >🗄</button>
          )}
        </span>
      </div>
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
        <NavLink to="/projects" className="appnav-link">{t('home.nav.projects')}</NavLink>
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

            {historyGroups.length > 0 && (
              <>
                <div className="appnav-term-proj appnav-term-history">{t('terminals.history')}</div>
                {historyGroups.map((g, idx) => {
                  // Default: o grupo mais recente já vem aberto; os demais colapsados.
                  const isOpen = histOpen[g.key] ?? idx === 0;
                  return (
                    <div className="appnav-term-group" key={g.key} ref={flipRef(g.key)}>
                      <button
                        className={`appnav-hist-head${isOpen ? ' open' : ''}`}
                        onClick={() => setHistOpen((m) => ({ ...m, [g.key]: !isOpen }))}
                        aria-expanded={isOpen}
                      >
                        <span className="appnav-hist-name">{g.name}</span>
                        <span className="appnav-hist-meta">{g.sessions.length} · {timeAgo(g.last)}</span>
                      </button>
                      {isOpen && g.sessions.slice(0, 8).map((s) => item(s, false))}
                    </div>
                  );
                })}
              </>
            )}
          </div>
        )}
      </nav>
      <div className="appnav-foot">
        <NavLink to="/settings" className="appnav-link">{t('nav.settings')}</NavLink>
      </div>

      {killTarget && (
        <div className="modal-overlay" onClick={() => !busy && setKillTarget(null)}>
          <div className="modal" role="dialog" aria-modal="true" aria-labelledby="appnav-kill-title"
            onClick={(e) => e.stopPropagation()}>
            <h3 id="appnav-kill-title" style={{ marginTop: 0 }}>{t('sessions.endConfirmTitle', 'Encerrar sessão em andamento?')}</h3>
            <p>{t('sessions.endConfirmMsg', 'O processo do agente será finalizado. A sessão fica no histórico e pode ser recomeçada depois.')}</p>
            <div style={{ display: 'flex', gap: '1rem', marginTop: '1.5rem' }}>
              <button className="btn btn-secondary" style={{ flex: 1 }} disabled={busy} onClick={() => setKillTarget(null)}>
                {t('common.cancel')}
              </button>
              <button className="btn btn-primary" style={{ flex: 1 }} disabled={busy} onClick={() => handleKill(killTarget.id)}>
                {t('sessions.end', 'Encerrar')}
              </button>
            </div>
          </div>
        </div>
      )}

      {archiveTarget && (
        <div className="modal-overlay" onClick={() => !busy && setArchiveTarget(null)}>
          <div className="modal" role="dialog" aria-modal="true" aria-labelledby="appnav-archive-title"
            onClick={(e) => e.stopPropagation()}>
            <h3 id="appnav-archive-title" style={{ marginTop: 0 }}>{t('sessions.archiveConfirmTitle')}</h3>
            <p>{t('sessions.archiveConfirmMsg')}</p>
            <div style={{ display: 'flex', gap: '1rem', marginTop: '1.5rem' }}>
              <button className="btn btn-secondary" style={{ flex: 1 }} disabled={busy} onClick={() => setArchiveTarget(null)}>
                {t('common.cancel')}
              </button>
              <button className="btn btn-primary" style={{ flex: 1 }} disabled={busy} onClick={() => handleArchive(archiveTarget.id)}>
                {t('sessions.archive')}
              </button>
            </div>
          </div>
        </div>
      )}
    </aside>
  );
}
