import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { respondInteraction, sendPrompt } from '../api';
import type { InteractionSnapshot } from '../api';
import { useDraft } from '../useDraft';

interface Props {
  snapshot: InteractionSnapshot;
  // Chamado após responder/enviar para a Home re-buscar o snapshot.
  onActed: () => void;
  onClose: () => void;
  // Abre a conversa completa (chat) para decidir lá vendo todo o contexto.
  onOpenChat?: () => void;
}

// cleanDetail transforma o input cru de uma ferramenta (JSON) num resumo legível
// — sem despejar JSON gigante no modal.
function cleanDetail(detail?: string): string {
  if (!detail) return '';
  try {
    const obj = JSON.parse(detail);
    if (obj && typeof obj === 'object') {
      return Object.entries(obj)
        .map(([k, v]) => {
          let s = typeof v === 'string' ? v : JSON.stringify(v);
          if (s.length > 140) s = s.slice(0, 139) + '…';
          return `${k}: ${s}`;
        })
        .join('\n');
    }
  } catch { /* não é JSON */ }
  return detail.length > 200 ? detail.slice(0, 199) + '…' : detail;
}

// InteractionPanel é a janela de resposta ao agente. Mostra um CONTEXTO conciso
// (eventos narrados — não o output cru) e o responder adequado:
//   - permissão (interrupt com request_id) → allow/deny pelo control protocol;
//   - escolha interpretada (kind=choice, sem request_id) → opções → viram prompt;
//   - texto (kind=text) ou ocioso → campo livre → vira prompt.
export default function InteractionPanel({ snapshot, onActed, onClose, onOpenChat }: Props) {
  const { t } = useTranslation();
  const { interrupt, state } = snapshot;
  const id = snapshot.session_id;
  // Rascunho persistido por sessão (compartilhado com o chat).
  const [text, setText, clearDraft] = useDraft(id);
  const [busy, setBusy] = useState(false);
  const isPermission = !!interrupt?.request_id;

  async function act(fn: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    try { await fn(); clearDraft(); onActed(); onClose(); } catch { /* já resolvido/encerrado */ }
    finally { setBusy(false); }
  }

  const permit = (allow: boolean) =>
    act(() => respondInteraction(id, interrupt!.request_id, allow ? 'allow' : 'deny'));
  const reply = (value: string) => act(() => sendPrompt(id, value));

  // Contexto = eventos narrados (concisos), nunca o output cru / JSONs.
  const context = snapshot.progress ?? [];
  // A IA só "espera de você" quando há um interrupt pendente ou o harness marca
  // a sessão como aguardando. Fora disso ela não espera nada — mostramos só "ok".
  const awaitsYou = !!interrupt || state === 'awaiting';
  // O que a IA espera de você: a pergunta bloqueante pendente OU a última fala da
  // IA (que costuma terminar numa pergunta).
  const expects = interrupt?.prompt ?? snapshot.message;

  return (
    <div className="ixp" role="dialog" aria-label={t('home.ix.title')}>
      <div className="ixp-head">
        <span className="ixp-state" data-state={state}>{t(`home.ix.state.${state}`)}</span>
        {onOpenChat && (
          <button className="ixp-open-chat" onClick={onOpenChat}>{t('home.ix.openChat')} →</button>
        )}
        <button className="ixp-close" onClick={onClose} aria-label={t('common.cancel')}>✕</button>
      </div>

      {/* Meu último pedido: o que o usuário pediu por último (o harness consegue digerir). */}
      {snapshot.user_message && (
        <section className="ixp-section">
          <h4 className="ixp-section-title">{t('home.ix.myRequest')}</h4>
          <p className="ixp-section-body">{snapshot.user_message}</p>
        </section>
      )}

      {/* Últimas ações: o que a IA já fez. */}
      {context.length > 0 && (
        <section className="ixp-section">
          <h4 className="ixp-section-title">{t('home.ix.lastActions')}</h4>
          <ul className="ixp-ctx-lines">
            {context.map((l, i) => <li key={i} className="ixp-ctx">{l}</li>)}
          </ul>
        </section>
      )}

      {/* A IA espera de você → mostra a pergunta final em markdown.
          Não espera nada → só um "ok" discreto, sem o título pesado. */}
      {awaitsYou && expects ? (
        <section className="ixp-section ixp-section-expects">
          <h4 className="ixp-section-title">{t('home.ix.expects')}</h4>
          <div className="ixp-section-body chat-md">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{expects}</ReactMarkdown>
          </div>
        </section>
      ) : (
        <section className="ixp-section ixp-section-ok">
          <span className="ixp-ok">✓ {t('home.ix.nothingExpected')}</span>
        </section>
      )}

      {/* Responder. */}
      {isPermission ? (
        <>
          {interrupt!.detail && <pre className="ixp-detail">{cleanDetail(interrupt!.detail)}</pre>}
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
        // texto livre (ocioso): manda um prompt novo.
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
