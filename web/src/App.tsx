import { useState, useCallback } from 'react';
import { BrowserRouter, Routes, Route, NavLink, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useEvents } from './useEvents';
import type { WsEvent } from './useEvents';
import Dashboard from './pages/Dashboard';
import Project from './pages/Project';
import Suggestions from './pages/Suggestions';
import Settings from './pages/Settings';
import Terminal from './pages/Terminal';
import Retro from './pages/Retro';
import Chat from './pages/Chat';
import Pipelines from './pages/Pipelines';
import Sessions from './pages/Sessions';
import SecretApprovalModal from './components/SecretApprovalModal';
import { FanMark } from './components/Fan';
import NewSessionModal from './components/NewSessionModal';
import SessionTabs from './components/SessionTabs';
import type { Session } from './api';
import './styles.css';

interface ApprovalRequest { requestId: string; secretName: string; }

function AppInner() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [pendingCount, setPendingCount] = useState(0);
  const [toast, setToast] = useState<string | null>(null);
  const [approval, setApproval] = useState<ApprovalRequest | null>(null);
  const [showNewSession, setShowNewSession] = useState(false);

  const handleEvent = useCallback((ev: WsEvent) => {
    if (ev.type === 'suggestion.created') {
      setToast(t('toast.newSuggestion'));
      setPendingCount((n) => n + 1);
      setTimeout(() => setToast(null), 4000);
    }
    if (ev.type === 'secret.approval_requested') {
      const p = ev.payload as Record<string, string>;
      setApproval({ requestId: p.request_id, secretName: p.name });
      setToast(t('toast.approvalRequest'));
      setTimeout(() => setToast(null), 4000);
    }
  }, [t]);

  useEvents(handleEvent);

  function handleSessionCreated(sess: Session) {
    setShowNewSession(false);
    navigate(`/sessions/${sess.id}`);
  }

  return (
    <div className="app-layout">
      <aside className="sidebar">
        <div className="sidebar-title">
          <FanMark size={22} />
          Worrel
        </div>
        <div className="sidebar-section">Cockpit</div>
        <nav>
          <NavLink to="/" end>{t('nav.dashboard')}</NavLink>
          <NavLink to="/suggestions">
            {t('nav.suggestions')}
            {pendingCount > 0 && <span className="badge">{pendingCount}</span>}
          </NavLink>
          <NavLink to="/sessions">{t('sessions.hub')}</NavLink>
          <NavLink to="/retro">{t('nav.retro')}</NavLink>
          <NavLink to="/chat">{t('nav.chat')}</NavLink>
          <NavLink to="/pipelines">{t('nav.pipelines')}</NavLink>
          <NavLink to="/settings">{t('nav.settings')}</NavLink>
        </nav>
        <div style={{ padding: '0.75rem 1rem', marginTop: 'auto' }}>
          <button
            className="btn btn-primary"
            style={{ width: '100%' }}
            onClick={() => setShowNewSession(true)}
          >
            {t('nav.newSession')}
          </button>
        </div>
      </aside>

      <div style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
        <SessionTabs />
        <main style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
          <Routes>
            <Route path="/" element={<Dashboard onPendingCount={setPendingCount} />} />
            <Route path="/projects/:id" element={<Project />} />
            <Route path="/sessions" element={<Sessions />} />
            <Route path="/sessions/:id" element={<Terminal />} />
            <Route path="/suggestions" element={<Suggestions />} />
            <Route path="/retro" element={<Retro />} />
            <Route path="/chat" element={<Chat />} />
            <Route path="/pipelines" element={<Pipelines />} />
            <Route path="/settings" element={<Settings />} />
          </Routes>
        </main>
      </div>

      <div className="toast-container" aria-live="polite">
        {toast && <div className="toast">{toast}</div>}
      </div>

      {approval && (
        <SecretApprovalModal
          requestId={approval.requestId}
          secretName={approval.secretName}
          onDone={() => setApproval(null)}
        />
      )}

      {showNewSession && (
        <NewSessionModal
          onCreated={handleSessionCreated}
          onClose={() => setShowNewSession(false)}
        />
      )}
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
