import { useCallback, useEffect, useRef, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { getInteraction, sendPrompt, respondInteraction, killSession } from '../api';
import type { InteractionSnapshot, HistoryLine } from '../api';
import { useEvents } from '../useEvents';
import { useDraft } from '../useDraft';
import { providerLabel } from '../session';

// SessionStream é a "interface de terminal" de uma sessão dirigida pelo MOTOR
// (stream-json): não há PTY/xterm — mostramos o HISTÓRICO da conversa (você, IA,
// ferramentas, decisões), a permissão pendente e um campo para mandar prompts.
export default function SessionStream() {
  const { id } = useParams<{ id: string }>();
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [snap, setSnap] = useState<InteractionSnapshot | null>(null);
  // Rascunho persistido por sessão: navegar entre terminais não perde o texto.
  const [text, setText, clearDraft] = useDraft(id);
  const [busy, setBusy] = useState(false);
  // Confirmação de encerramento: espelha o modal do x no sidebar (AppNav).
  const [confirmKill, setConfirmKill] = useState(false);
  const bodyRef = useRef<HTMLDivElement>(null);

  const load = useCallback(() => {
    if (!id) return;
    getInteraction(id).then(setSnap).catch(() => { /* ignore */ });
  }, [id]);

  useEffect(() => { load(); }, [load]);
  useEvents(useCallback((ev) => {
    const p = ev.payload as { session_id?: string; id?: string };
    if ((p?.session_id === id || p?.id === id) &&
      ['interaction.changed', 'session.awaiting', 'session.busy', 'session.ended'].includes(ev.type)) {
      load();
    }
  }, [id, load]));

  // rola para o fim quando o histórico cresce.
  useEffect(() => {
    bodyRef.current?.scrollTo({ top: bodyRef.current.scrollHeight });
  }, [snap?.history?.length, snap?.interrupt]);

  async function act(fn: () => Promise<unknown>) {
    if (busy) return;
    setBusy(true);
    try { await fn(); load(); } catch { /* noop */ } finally { setBusy(false); }
  }

  function submit() {
    if (!id || !text.trim()) return;
    const t = text.trim();
    clearDraft();
    act(() => sendPrompt(id, t));
  }

  const interrupt = snap?.interrupt;
  const isPermission = !!interrupt?.request_id;
  const state = snap?.state ?? 'working';
  const history = snap?.history ?? [];

  const reply = (value: string) => { if (id) act(() => sendPrompt(id, value)); };

  return (
    <div className="sstream">
      <header className="sstream-head">
        <button className="btn btn-secondary btn-sm" onClick={() => navigate('/')}>← {t('home.nav.home')}</button>
        <span className="ixp-state" data-state={state}>{t(`home.ix.state.${state}`)}</span>
        <span className="sstream-engine">{providerLabel('engine')}</span>
        <button className="btn btn-danger btn-sm" style={{ marginLeft: 'auto' }}
          disabled={busy} onClick={() => setConfirmKill(true)}>
          {t('terminal.kill')}
        </button>
      </header>

      <div className="sstream-body" ref={bodyRef}>
        {history.length === 0 && <div className="sstream-empty">{t('home.ix.working')}</div>}
        {history.map((h, i) => <ChatLine key={i} line={h} />)}
        {state === 'working' && history.length > 0 && (
          <div className="chat-thinking">{t('home.ix.working')}<span className="dots"><i /><i /><i /></span></div>
        )}
      </div>

      {isPermission ? (
        <div className="sstream-foot sstream-permission">
          <div className="sstream-perm-q">{interrupt!.prompt}</div>
          <div className="ixp-actions">
            <button className="btn btn-primary btn-sm" disabled={busy}
              onClick={() => id && act(() => respondInteraction(id, interrupt!.request_id, 'allow'))}>{t('ask.allow')}</button>
            <button className="btn btn-danger btn-sm" disabled={busy}
              onClick={() => id && act(() => respondInteraction(id, interrupt!.request_id, 'deny'))}>{t('ask.deny')}</button>
          </div>
        </div>
      ) : interrupt?.kind === 'choice' && interrupt.options?.length ? (
        <div className="sstream-foot sstream-permission">
          {interrupt.prompt && <div className="sstream-perm-q">{interrupt.prompt}</div>}
          <div className="ixp-actions ixp-options">
            {interrupt.options.map((opt) => (
              <button key={opt} className="btn btn-secondary btn-sm" disabled={busy} onClick={() => reply(opt)}>{opt}</button>
            ))}
          </div>
          <div className="sstream-foot" style={{ padding: 0, border: 'none', background: 'none' }}>
            <span className="nsw-prompt-glyph" aria-hidden="true">›</span>
            <textarea className="sstream-input" value={text} onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submit(); } }}
              placeholder={t('home.ix.promptPlaceholder')} rows={2} />
            <button className="btn btn-primary btn-sm" disabled={busy || !text.trim()} onClick={submit}>{t('home.ix.send')}</button>
          </div>
        </div>
      ) : (
        <div className="sstream-foot">
          <span className="nsw-prompt-glyph" aria-hidden="true">›</span>
          <textarea
            className="sstream-input"
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submit(); } }}
            placeholder={t('home.ix.promptPlaceholder')}
            rows={2}
            autoFocus
          />
          <button className="btn btn-primary btn-sm" disabled={busy || !text.trim()} onClick={submit}>{t('home.ix.send')}</button>
        </div>
      )}

      {confirmKill && (
        <div className="modal-overlay" onClick={() => !busy && setConfirmKill(false)}>
          <div className="modal" role="dialog" aria-modal="true" aria-labelledby="sstream-kill-title"
            onClick={(e) => e.stopPropagation()}>
            <h3 id="sstream-kill-title" style={{ marginTop: 0 }}>{t('sessions.endConfirmTitle', 'Encerrar sessão em andamento?')}</h3>
            <p>{t('sessions.endConfirmMsg', 'O processo do agente será finalizado. A sessão fica no histórico e pode ser recomeçada depois.')}</p>
            <div style={{ display: 'flex', gap: '1rem', marginTop: '1.5rem' }}>
              <button className="btn btn-secondary" style={{ flex: 1 }} disabled={busy} onClick={() => setConfirmKill(false)}>
                {t('common.cancel')}
              </button>
              <button className="btn btn-primary" style={{ flex: 1 }} disabled={busy}
                onClick={() => id && act(() => killSession(id).then(() => { setConfirmKill(false); navigate('/'); }))}>
                {t('sessions.end', 'Encerrar')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// ChatLine renderiza uma linha do histórico como uma bolha de chat:
//   you → bolha do usuário (direita); ai → markdown renderizado (esquerda);
//   tool → linha discreta (mono); system → nota central.
function ChatLine({ line }: { line: HistoryLine }) {
  if (line.role === 'tool') {
    return <div className="chat-tool"><code>{line.text}</code></div>;
  }
  if (line.role === 'system') {
    return <div className="chat-system">{line.text}</div>;
  }
  if (line.role === 'you') {
    return <div className="chat-row chat-you"><div className="chat-bubble">{line.text}</div></div>;
  }
  // ai → markdown
  return (
    <div className="chat-row chat-ai">
      <div className="chat-bubble chat-md">
        <ReactMarkdown remarkPlugins={[remarkGfm]}>{line.text}</ReactMarkdown>
      </div>
    </div>
  );
}
