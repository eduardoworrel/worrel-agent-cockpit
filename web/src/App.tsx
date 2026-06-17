import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { BrowserRouter, Routes, Route, useNavigate, useMatch } from 'react-router-dom';
import { useEvents } from './useEvents';
import type { WsEvent } from './useEvents';
import { distillSession, getPendingAsks } from './api';
import type { DistillResult, AskRequest } from './api';
import AskBalloons from './components/AskBalloons';
import Project from './pages/Project';
import Settings from './pages/Settings';
import Terminal from './pages/Terminal';
import Retro from './pages/Retro';
import SecretApprovalModal from './components/SecretApprovalModal';
import NewSessionModal from './components/NewSessionModal';
import EmptyState from './shell/EmptyState';
import ProjectSidebar from './shell/ProjectSidebar';
import SuggestionsDrawer from './shell/SuggestionsDrawer';
import { useAppState } from './shell/useAppState';
import type { Session } from './api';
import './styles.css';

interface ApprovalRequest { requestId: string; secretName: string; }

// useActiveProjectId deriva o projeto ativo da rota (/projects/:id ou /sessions/:id).
function useActiveProjectId(wrapperSessions: Session[]): string | null {
  const projMatch = useMatch('/projects/:id');
  const sessMatch = useMatch('/sessions/:id');
  if (projMatch?.params.id) return projMatch.params.id;
  if (sessMatch?.params.id) {
    const s = wrapperSessions.find((x) => x.id === sessMatch.params.id);
    return s?.project_id ?? null;
  }
  return null;
}

// ExtractState acompanha o toast de extração de extrato disparado ao encerrar
// uma sessão. id = sessão a extrair; o resto é o ciclo de vida da chamada.
interface ExtractState {
  id: string;
  status: 'idle' | 'busy' | 'done' | 'error';
  result?: DistillResult;
  reason?: string; // motivo do encerramento (exit code + cauda do stderr do CLI)
}

function AppInner() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { loading, projects, wrapperSessions, liveIds, isEmpty, reload } = useAppState();
  const [approval, setApproval] = useState<ApprovalRequest | null>(null);
  const [newSessionProject, setNewSessionProject] = useState<string | null | undefined>(undefined);
  const [reloadKey, setReloadKey] = useState(0);
  const [extract, setExtract] = useState<ExtractState | null>(null);
  const [asks, setAsks] = useState<AskRequest[]>([]);
  // awaitingIds: sessões cujo CLI terminou o turno e aguarda o usuário.
  const [awaitingIds, setAwaitingIds] = useState<Set<string>>(new Set());
  const activeProjectId = useActiveProjectId(wrapperSessions);
  const activeSessionMatch = useMatch('/sessions/:id');
  const activeSessionId = activeSessionMatch?.params.id ?? null;

  // Abrir a sessão já a "lê": limpa o alerta de atenção localmente (o backend
  // confirma no próximo poll quando o usuário responde).
  useEffect(() => {
    if (!activeSessionId) return;
    setAwaitingIds((prev) => {
      if (!prev.has(activeSessionId)) return prev;
      const next = new Set(prev);
      next.delete(activeSessionId);
      return next;
    });
  }, [activeSessionId]);

  const loadAsks = useCallback(() => {
    getPendingAsks().then(setAsks).catch(() => { /* ignore */ });
  }, []);

  const handleEvent = useCallback((ev: WsEvent) => {
    if (ev.type === 'suggestion.created') setReloadKey((n) => n + 1);
    // CLI terminou o turno (aguardando resposta/confirmação) ou voltou a
    // trabalhar: liga/desliga o alerta de atenção da sessão na sidebar.
    if (ev.type === 'session.awaiting' || ev.type === 'session.busy' || ev.type === 'session.ended') {
      const p = ev.payload as { session_id?: string; id?: string };
      const sid = p.session_id ?? p.id;
      if (sid) {
        setAwaitingIds((prev) => {
          const next = new Set(prev);
          if (ev.type === 'session.awaiting') next.add(sid);
          else next.delete(sid);
          return next;
        });
      }
    }
    // Sessão encerrada (kill manual ou processo saiu): recarrega para que
    // liveIds seja recomputado e a sessão saia da sidebar automaticamente, e
    // oferece extrair os aprendizados da sessão (skills/memórias) antes que o
    // transcript seja podado.
    if (ev.type === 'session.ended') {
      reload();
      const p = ev.payload as { id?: string; reason?: string };
      if (p.id) setExtract({ id: p.id, status: 'idle', reason: p.reason });
    }
    // Título da sessão derivado do 1º recado: recarrega para refletir na sidebar.
    if (ev.type === 'session.titled') reload();
    if (ev.type === 'secret.approval_requested') {
      const p = ev.payload as Record<string, string>;
      setApproval({ requestId: p.request_id, secretName: p.name });
    }
    if (ev.type === 'ask.requested') {
      const p = ev.payload as AskRequest;
      setAsks((prev) => (prev.some((a) => a.request_id === p.request_id) ? prev : [...prev, p]));
    }
    if (ev.type === 'ask.resolved') {
      const p = ev.payload as { request_id: string };
      setAsks((prev) => prev.filter((a) => a.request_id !== p.request_id));
    }
    if (ev.type === 'session.ended') {
      const p = ev.payload as { id?: string };
      if (p.id) setAsks((prev) => prev.filter((a) => a.session_id !== p.id));
    }
  }, [reload]);
  useEvents(handleEvent, loadAsks);
  useEffect(() => { loadAsks(); }, [loadAsks]);

  async function handleExtract() {
    if (!extract) return;
    setExtract((e) => (e ? { ...e, status: 'busy' } : e));
    try {
      const result = await distillSession(extract.id);
      setExtract((e) => (e ? { ...e, status: 'done', result } : e));
      setReloadKey((n) => n + 1); // atualiza o drawer de sugestões
    } catch {
      setExtract((e) => (e ? { ...e, status: 'error' } : e));
    }
  }

  const extractToast = extract && (
    <div className="extract-toast">
      {extract.status === 'done' ? (
        <>
          <div className="extract-toast-msg">
            {extract.result && extract.result.created > 0
              ? t('sessionExtract.created', { count: extract.result.created })
              : t('sessionExtract.none')}
          </div>
          <button className="btn btn-secondary" onClick={() => setExtract(null)}>
            {t('sessionExtract.close')}
          </button>
        </>
      ) : (
        <>
          <div className="extract-toast-msg">{t('sessionExtract.prompt')}</div>
          {extract.reason && (
            <div className="extract-toast-reason">{extract.reason}</div>
          )}
          {extract.status === 'error' && (
            <div className="extract-toast-err">{t('sessionExtract.error')}</div>
          )}
          <div className="extract-toast-actions">
            <button
              className="btn btn-accent"
              disabled={extract.status === 'busy'}
              onClick={handleExtract}
            >
              {extract.status === 'busy' ? t('sessionExtract.extracting') : t('sessionExtract.extract')}
            </button>
            <button className="btn btn-secondary" onClick={() => setExtract(null)}>
              {t('sessionExtract.dismiss')}
            </button>
          </div>
        </>
      )}
    </div>
  );

  function handleSessionCreated(sess: Session) {
    setNewSessionProject(undefined);
    reload();
    navigate(`/sessions/${sess.id}`);
  }

  if (loading) return <div className="app-layout" />;

  if (isEmpty) {
    return (
      <>
        <Routes>
          <Route
            path="/retro"
            element={
              <main className="app-layout" style={{ flexDirection: 'column', overflow: 'auto' }}>
                <Retro />
              </main>
            }
          />
          <Route
            path="*"
            element={
              <EmptyState
                onNewSession={() => setNewSessionProject(null)}
                onAnalyzeHistory={() => navigate('/retro')}
              />
            }
          />
        </Routes>
        {newSessionProject !== undefined && (
          <NewSessionModal onCreated={handleSessionCreated} onClose={() => setNewSessionProject(undefined)} />
        )}
        {approval && (
          <SecretApprovalModal requestId={approval.requestId} secretName={approval.secretName} onDone={() => setApproval(null)} />
        )}
        <AskBalloons asks={asks} onResolved={(id) => setAsks((prev) => prev.filter((a) => a.request_id !== id))} />
        {extractToast}
      </>
    );
  }

  return (
    <div className="app-layout">
      <ProjectSidebar
        projects={projects}
        wrapperSessions={wrapperSessions}
        liveIds={liveIds}
        awaitingIds={awaitingIds}
        onStarted={handleSessionCreated}
        onAnalyzeHistory={() => navigate('/retro')}
      />

      <div style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
        <main style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
          <Routes>
            <Route path="/" element={<Retro />} />
            <Route path="/projects/:id" element={<Project />} />
            <Route path="/sessions/:id" element={<Terminal />} />
            <Route path="/retro" element={<Retro />} />
            <Route path="/settings" element={<Settings />} />
          </Routes>
        </main>
      </div>

      <SuggestionsDrawer activeProjectId={activeProjectId} projects={projects} reloadKey={reloadKey} />

      {approval && (
        <SecretApprovalModal requestId={approval.requestId} secretName={approval.secretName} onDone={() => setApproval(null)} />
      )}
      <AskBalloons asks={asks} onResolved={(id) => setAsks((prev) => prev.filter((a) => a.request_id !== id))} />
      {extractToast}
    </div>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <AppInner />
    </BrowserRouter>
  );
}
