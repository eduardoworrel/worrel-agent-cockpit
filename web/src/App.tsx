import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { BrowserRouter, Routes, Route, useNavigate, useMatch } from 'react-router-dom';
import { useEvents } from './useEvents';
import type { WsEvent } from './useEvents';
import { distillSession, getInteraction, listDeferred } from './api';
import type { DistillResult } from './api';
import GlobalInteractionModal from './components/GlobalInteractionModal';
import Project from './pages/Project';
import Settings from './pages/Settings';
import SessionRoute from './pages/SessionRoute';
import SecretApprovalModal from './components/SecretApprovalModal';
import NewSessionWizard from './components/NewSessionWizard';
import EmptyState from './shell/EmptyState';
import AppNav from './shell/AppNav';
import Home from './pages/Home';
import Dashboard from './pages/Dashboard';
import SuggestionsDrawer from './shell/SuggestionsDrawer';
import { useAppState } from './shell/useAppState';
import { useSnapshots } from './useSnapshots';
import { sessionStatus } from './sessionStatus';
import type { SessionStatus } from './sessionStatus';
import type { Session } from './api';
import './styles.css';

interface ApprovalRequest { requestId: string; secretName: string; }

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
  const { initialLoading, projects, wrapperSessions, liveIds, isEmpty, reload } = useAppState();
  const [approval, setApproval] = useState<ApprovalRequest | null>(null);
  const [showWizard, setShowWizard] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);
  const [extract, setExtract] = useState<ExtractState | null>(null);
  // awaitingIds: sessões cujo CLI terminou o turno e aguarda o usuário.
  const [awaitingIds, setAwaitingIds] = useState<Set<string>>(new Set());
  // Modal global de interação (uma sessão por vez). openSessionId = sessão visível;
  // queue = sessões que entraram em "awaiting" e aguardam a vez; deferredSet =
  // sessões adiadas (não auto-abrem). Edge-triggered: só enfileira no evento.
  const [openSessionId, setOpenSessionId] = useState<string | null>(null);
  const [modalQueue, setModalQueue] = useState<string[]>([]);
  const deferredRef = useRef<Set<string>>(new Set());
  // Sessões já conhecidas como "aguardando", p/ disparar a auto-abertura só na
  // TRANSIÇÃO para awaiting (edge-trigger) e não a cada interaction.changed.
  const awaitingKnownRef = useRef<Set<string>>(new Set());
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

  // enqueueAutoOpen coloca a sessão na fila do modal se ela ainda não está
  // adiada (sessões adiadas só reabrem pela bolinha). Uma por vez.
  const enqueueAutoOpen = useCallback((sid: string) => {
    if (deferredRef.current.has(sid)) return;
    setModalQueue((q) => (q.includes(sid) ? q : [...q, sid]));
  }, []);

  const handleEvent = useCallback((ev: WsEvent) => {
    if (ev.type === 'suggestion.created') setReloadKey((n) => n + 1);
    // Sessões de motor (stream-json) não emitem session.awaiting — só
    // interaction.changed. Buscamos o snapshot e, na TRANSIÇÃO para awaiting
    // (ou interrupt), enfileiramos o modal. Edge-trigger via awaitingKnownRef.
    if (ev.type === 'interaction.changed') {
      const p = ev.payload as { session_id?: string; id?: string };
      const sid = p.session_id ?? p.id;
      if (sid) {
        getInteraction(sid).then((snap) => {
          const awaits = !!snap.interrupt || snap.state === 'awaiting';
          if (awaits) {
            if (!awaitingKnownRef.current.has(sid)) {
              awaitingKnownRef.current.add(sid);
              enqueueAutoOpen(sid);
            }
          } else {
            awaitingKnownRef.current.delete(sid);
          }
        }).catch(() => { /* ignore */ });
      }
    }
    // Pergunta bloqueante via broker (hook PreToolUse de sessões PTY ou a tool
    // MCP ask_user): o backend publica SÓ ask.requested — não há session.awaiting
    // nem interaction.changed. Era o gatilho do antigo AskBalloons; aqui auto-abre
    // o modal global. O snapshot da sessão expõe o ask pendente como interrupt.
    if (ev.type === 'ask.requested') {
      const p = ev.payload as { session_id?: string; id?: string };
      const sid = p.session_id ?? p.id;
      if (sid) enqueueAutoOpen(sid);
    }
    // CLI terminou o turno (aguardando resposta/confirmação) ou voltou a
    // trabalhar: liga/desliga o alerta de atenção da sessão na sidebar.
    if (ev.type === 'session.awaiting' || ev.type === 'session.busy' || ev.type === 'session.ended') {
      const p = ev.payload as { session_id?: string; id?: string };
      const sid = p.session_id ?? p.id;
      if (sid) {
        setAwaitingIds((prev) => {
          const has = prev.has(sid);
          if (ev.type === 'session.awaiting') {
            if (has) return prev; // já marcado: não recria o Set
            const next = new Set(prev);
            next.add(sid);
            return next;
          }
          if (!has) return prev; // já ausente: não recria o Set
          const next = new Set(prev);
          next.delete(sid);
          return next;
        });
        // Modal global (sessões PTY/wrapper): entrou em "awaiting" → auto-abre.
        if (ev.type === 'session.awaiting') {
          if (!awaitingKnownRef.current.has(sid)) {
            awaitingKnownRef.current.add(sid);
            enqueueAutoOpen(sid);
          }
        } else {
          awaitingKnownRef.current.delete(sid); // busy/ended: saiu do awaiting
        }
      }
    }
    // Adiada/respondida: tira da fila e fecha o modal se for a sessão visível.
    if (ev.type === 'session.deferred' || ev.type === 'session.undeferred' || ev.type === 'session.ended') {
      const p = ev.payload as { session_id?: string; id?: string };
      const sid = p.session_id ?? p.id;
      if (sid) {
        if (ev.type === 'session.deferred') deferredRef.current.add(sid);
        else deferredRef.current.delete(sid);
        setModalQueue((q) => q.filter((x) => x !== sid));
        setOpenSessionId((cur) => (cur === sid && ev.type !== 'session.undeferred' ? null : cur));
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
  }, [reload, enqueueAutoOpen]);
  // reconcileAutoOpen reconcilia o estado REAL ao montar/reconectar: eventos
  // (interaction.changed/session.awaiting) são transientes, então ao carregar a
  // página numa rota qualquer — ou após o WS reconectar — uma sessão que JÁ está
  // aguardando não dispararia evento novo. Aqui varremos as sessões vivas e
  // abrimos o modal das que aguardam e não estão adiadas. Edge via awaitingKnownRef.
  const reconcileAutoOpen = useCallback(async () => {
    try {
      const def = await listDeferred();
      deferredRef.current = new Set(def.map((d) => d.session_id));
    } catch { /* ignore */ }
    const live = wrapperSessions.filter((s) => liveIds.has(s.id));
    for (const s of live) {
      try {
        const snap = await getInteraction(s.id);
        if ((!!snap.interrupt || snap.state === 'awaiting') && !awaitingKnownRef.current.has(s.id)) {
          awaitingKnownRef.current.add(s.id);
          enqueueAutoOpen(s.id);
        }
      } catch { /* ignore */ }
    }
  }, [wrapperSessions, liveIds, enqueueAutoOpen]);

  useEvents(handleEvent, reconcileAutoOpen);
  useEffect(() => { reconcileAutoOpen(); }, [reconcileAutoOpen]);

  // Auto-abre o próximo da fila quando não há modal visível.
  useEffect(() => {
    if (openSessionId || modalQueue.length === 0) return;
    setOpenSessionId(modalQueue[0]);
    setModalQueue((q) => q.slice(1));
  }, [openSessionId, modalQueue]);

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

  // openTerminal=true abre o terminal; senão fica na Home, onde a sessão vira a
  // miniatura AG-UI no card. Default true (abrir) para chamadas legadas.
  function handleSessionCreated(sess: Session, openTerminal = true) {
    setShowWizard(false);
    reload();
    navigate(openTerminal ? `/sessions/${sess.id}` : '/');
  }

  const liveSessions = wrapperSessions.filter((s) => liveIds.has(s.id));

  // Snapshots AG-UI das sessões vivas: fonte ÚNICA (App) lida pela Home e pela
  // sidebar, para que o farol de estado seja idêntico nos dois lugares.
  const [snapshots, reloadSnapshots] = useSnapshots(liveSessions);

  // statusById deriva o farol de cada sessão pela MESMA função do card (ver
  // sessionStatus): encerradas → 'ended'; vivas → working/awaiting/clássica.
  const statusById = useMemo(() => {
    const m: Record<string, SessionStatus> = {};
    for (const s of wrapperSessions) {
      m[s.id] = liveIds.has(s.id)
        ? sessionStatus({ snapshot: snapshots[s.id], awaiting: awaitingIds.has(s.id), classic: s.adapter !== 'engine' })
        : 'ended';
    }
    return m;
  }, [wrapperSessions, liveIds, snapshots, awaitingIds]);

  if (initialLoading) return <div className="app-layout" />;

  if (isEmpty) {
    return (
      <>
        <Routes>
          <Route
            path="*"
            element={
              <EmptyState
                onNewSession={() => setShowWizard(true)}
              />
            }
          />
        </Routes>
        {showWizard && (
          <NewSessionWizard onCreated={handleSessionCreated} onClose={() => setShowWizard(false)} />
        )}
        {approval && (
          <SecretApprovalModal requestId={approval.requestId} secretName={approval.secretName} onDone={() => setApproval(null)} />
        )}
        {openSessionId && (
          <GlobalInteractionModal sessionId={openSessionId} onClose={() => setOpenSessionId(null)} />
        )}
        {extractToast}
      </>
    );
  }

  return (
    <div className="app-layout">
      <AppNav projects={projects} sessions={wrapperSessions} liveIds={liveIds} statusById={statusById} onChanged={reload} />

      <div style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
        <main style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
          <Routes>
            <Route path="/" element={
              <Home
                liveSessions={liveSessions}
                awaitingIds={awaitingIds}
                snapshots={snapshots}
                reloadSnapshots={reloadSnapshots}
                onNewSession={() => setShowWizard(true)}
                reloadKey={reloadKey}
                onOpenSession={setOpenSessionId}
              />
            } />
            <Route path="/projects" element={<Dashboard onPendingCount={() => { /* badge gerido alhures */ }} />} />
            <Route path="/projects/:id" element={<Project />} />
            <Route path="/sessions/:id" element={<SessionRoute sessions={wrapperSessions} />} />
            <Route path="/settings" element={<Settings />} />
          </Routes>
        </main>
      </div>

      <SuggestionsDrawer projects={projects} reloadKey={reloadKey} onOpen={setOpenSessionId} />

      {showWizard && (
        <NewSessionWizard onCreated={handleSessionCreated} onClose={() => setShowWizard(false)} />
      )}
      {approval && (
        <SecretApprovalModal requestId={approval.requestId} secretName={approval.secretName} onDone={() => setApproval(null)} />
      )}
      {openSessionId && (
        <GlobalInteractionModal sessionId={openSessionId} onClose={() => setOpenSessionId(null)} />
      )}
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
