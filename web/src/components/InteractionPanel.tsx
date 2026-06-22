import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { respondInteraction, sendPrompt } from '../api';
import type { InteractionSnapshot } from '../api';

interface Props {
  snapshot: InteractionSnapshot;
  // Chamado após responder/enviar para a Home re-buscar o snapshot.
  onActed: () => void;
  onClose: () => void;
}

// InteractionPanel é a janela de resposta ao agente. Mostra a última fala da IA
// + contexto recente e o responder adequado:
//   - permissão (interrupt com request_id) → allow/deny pelo control protocol;
//   - escolha interpretada (kind=choice, sem request_id) → opções → viram prompt;
//   - texto (kind=text) ou ocioso → campo livre → vira prompt.
export default function InteractionPanel({ snapshot, onActed, onClose }: Props) {
  const { t } = useTranslation();
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  const { interrupt, state } = snapshot;
  const id = snapshot.session_id;
  const isPermission = !!interrupt?.request_id;

  async function act(fn: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    try { await fn(); setText(''); onActed(); onClose(); } catch { /* já resolvido/encerrado */ }
    finally { setBusy(false); }
  }

  // Permissão: responde pelo control protocol (allow/deny).
  const permit = (allow: boolean) =>
    act(() => respondInteraction(id, interrupt!.request_id, allow ? 'allow' : 'deny'));
  // Escolha/texto/prompt livre: vira um novo turno (prompt).
  const reply = (value: string) => act(() => sendPrompt(id, value));

  const contextLines = (snapshot.history ?? []).slice(-6, -1); // contexto antes da última fala

  return (
    <div className="ixp" role="dialog" aria-label={t('home.ix.title')}>
      <div className="ixp-head">
        <span className="ixp-state" data-state={state}>{t(`home.ix.state.${state}`)}</span>
        <button className="ixp-close" onClick={onClose} aria-label={t('common.cancel')}>✕</button>
      </div>

      {/* Contexto: o que veio antes + o último pedido. */}
      <div className="ixp-context">
        {snapshot.user_message && (
          <p className="ixp-you"><b>{t('home.ix.youAsked')}</b> {snapshot.user_message}</p>
        )}
        {contextLines.length > 0 && (
          <ul className="ixp-ctx-lines">
            {contextLines.map((h, i) => (
              <li key={i} className={`ixp-ctx role-${h.role}`}>{h.text}</li>
            ))}
          </ul>
        )}
      </div>

      {/* A última coisa dita / a pergunta. */}
      {(interrupt?.prompt || snapshot.message) && !isPermission && (
        <div className="ixp-said">{interrupt?.prompt || snapshot.message}</div>
      )}

      {/* Responder. */}
      {isPermission ? (
        <>
          <div className="ixp-prompt-q">{interrupt!.prompt}</div>
          {interrupt!.detail && <pre className="ixp-detail">{interrupt!.detail}</pre>}
          <div className="ixp-actions">
            <button className="btn btn-primary btn-sm" disabled={busy} onClick={() => permit(true)}>{t('ask.allow')}</button>
            <button className="btn btn-danger btn-sm" disabled={busy} onClick={() => permit(false)}>{t('ask.deny')}</button>
          </div>
        </>
      ) : interrupt?.kind === 'choice' && interrupt.options?.length ? (
        <>
          <div className="ixp-actions ixp-options">
            {interrupt.options.map((opt) => (
              <button key={opt} className="btn btn-secondary btn-sm" disabled={busy} onClick={() => reply(opt)}>{opt}</button>
            ))}
          </div>
          <form className="ixp-form" onSubmit={(e) => { e.preventDefault(); if (text.trim()) reply(text.trim()); }}>
            <input className="ixp-input" value={text} onChange={(e) => setText(e.target.value)}
              placeholder={t('home.ix.promptPlaceholder')} />
            <button className="btn btn-primary btn-sm" type="submit" disabled={busy || !text.trim()}>{t('home.ix.send')}</button>
          </form>
        </>
      ) : state === 'working' ? (
        <div className="ixp-thinking">{t('home.ix.working')}<span className="dots"><i /><i /><i /></span></div>
      ) : state === 'ended' ? (
        <p className="ixp-muted">{t('home.ix.ended')}</p>
      ) : (
        // texto livre (kind=text, ou ocioso sem interrupt): manda um prompt novo.
        <form className="ixp-form" onSubmit={(e) => { e.preventDefault(); if (text.trim()) reply(text.trim()); }}>
          <textarea className="ixp-textarea" value={text} onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); if (text.trim()) reply(text.trim()); } }}
            placeholder={t('home.ix.promptPlaceholder')} rows={2} autoFocus />
          <button className="btn btn-primary btn-sm" type="submit" disabled={busy || !text.trim()}>{t('home.ix.send')}</button>
        </form>
      )}
    </div>
  );
}
