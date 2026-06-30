import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { respondInteraction, sendPrompt, deferSession, idleSession, killSession } from '../api';
import type { InteractionSnapshot } from '../api';
import { useDraft } from '../useDraft';
import ResponseWidget, { widgetSupported } from './ResponseWidget';
import AskHtmlFrame from './AskHtmlFrame';

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
    // Em erro (ex.: 409 de interrupt já resolvido/órfão) ainda refazemos onActed
    // para sincronizar o snapshot — senão os botões velhos ficam presos e nenhum
    // clique surte efeito. Só fechamos o modal no caminho de sucesso.
    try { await fn(); clearDraft(); onActed(); onClose(); } catch { onActed(); }
    finally { setBusy(false); }
  }

  // Adiar: fecha o modal sem responder e marca a sessão como adiada no backend
  // (vira bolinha no sidebar). Não limpa o rascunho — a resposta fica pendente.
  async function doDefer() {
    if (busy) return;
    setBusy(true);
    try { await deferSession(id); onClose(); } catch { /* já resolvido/encerrado */ }
    finally { setBusy(false); }
  }

  // Ocioso: dispensa a pergunta e marca a sessão como ociosa no backend — vira
  // bolinha CINZA no sidebar (sem pendência), distinta da laranja do Adiar.
  // Reabre o modal só ao clicar a bolinha.
  async function doIdle() {
    if (busy) return;
    setBusy(true);
    try { await idleSession(id); onClose(); } catch { /* já resolvido/encerrado */ }
    finally { setBusy(false); }
  }

  // Encerrar: finaliza o processo do agente (kill). Pede confirmação porque é
  // irreversível — a sessão acaba.
  async function doClose() {
    if (busy) return;
    if (!window.confirm(t('home.ix.closeConfirm'))) return;
    setBusy(true);
    try { await killSession(id); onActed(); onClose(); } catch { /* já encerrado */ }
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
        {awaitsYou && (
          <>
            <button className="ixp-defer" disabled={busy} onClick={doDefer}
              title={t('home.ix.deferHint')}>{t('home.ix.defer', 'Adiar')}</button>
            <button className="ixp-idle" disabled={busy} onClick={doIdle}
              title={t('home.ix.idleHint')}>{t('home.ix.idle', 'Ocioso')}</button>
            <button className="ixp-close-session" disabled={busy} onClick={doClose}
              title={t('home.ix.closeHint')}>{t('home.ix.close', 'Encerrar')}</button>
          </>
        )}
        <button className="ixp-close" onClick={onClose} aria-label={t('common.cancel')}
          title={t('home.ix.dismissHint')}>✕</button>
      </div>

      {/* Seu pedido: o pedido condensado por IA (request_summary); enquanto ela
          não chega, cai no user_message cru (que antes sumia no fluxo). */}
      {(snapshot.request_summary || snapshot.user_message) && (
        <section className="ixp-section">
          <h4 className="ixp-section-title">{t('home.ix.myRequest')}</h4>
          <p className="ixp-section-body">{snapshot.request_summary || snapshot.user_message}</p>
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
          {/* HTML rico (ask_html) num iframe ISOLADO (sandbox sem same-origin);
              se a IA ainda não gerou o HTML, cai no markdown atual. O HTML nunca
              bloqueia o input abaixo. */}
          {!isPermission && snapshot.ask_html ? (
            <AskHtmlFrame html={snapshot.ask_html} onChoice={reply} />
          ) : !isPermission && snapshot.ask_html_pending ? (
            // Gerando o HTML rico: mostra um loading em vez de "piscar" o markdown
            // cru antes da versão condensada chegar.
            <div className="ixp-ask-loading">
              <span className="ixp-spinner" aria-hidden="true" />
              {t('home.ix.preparing', 'Preparando a visualização…')}
            </div>
          ) : (
            <div className="ixp-section-body chat-md">
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{expects}</ReactMarkdown>
            </div>
          )}
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
      ) : awaitsYou && snapshot.ask_html ? (
        // HTML rico cuida da apresentação E das choices clicáveis (sem botões
        // duplicados). Só o slider (range) não cabe como clicável no HTML → cai no
        // widget. Abaixo, um campo livre de escape para responder fora das opções.
        <>
          {snapshot.response_widget?.type === 'range' && widgetSupported(snapshot.response_widget) && (
            <ResponseWidget widget={snapshot.response_widget} busy={busy} onSubmit={reply} />
          )}
          <form className="ixp-form" onSubmit={(e) => { e.preventDefault(); if (text.trim()) reply(text.trim()); }}>
            <input className="ixp-input" value={text} onChange={(e) => setText(e.target.value)}
              placeholder={t('home.ix.promptPlaceholder')} />
            <button className="btn btn-primary btn-sm" type="submit" disabled={busy || !text.trim()}>{t('home.ix.send')}</button>
          </form>
        </>
      ) : awaitsYou && widgetSupported(snapshot.response_widget) ? (
        // Fallback SEM ask_html: o widget dinâmico (experimental) renderiza o
        // controle. Mantém um form de texto de escape.
        <>
          <ResponseWidget widget={snapshot.response_widget!} busy={busy} onSubmit={reply} />
          <form className="ixp-form" onSubmit={(e) => { e.preventDefault(); if (text.trim()) reply(text.trim()); }}>
            <input className="ixp-input" value={text} onChange={(e) => setText(e.target.value)}
              placeholder={t('home.ix.promptPlaceholder')} />
            <button className="btn btn-primary btn-sm" type="submit" disabled={busy || !text.trim()}>{t('home.ix.send')}</button>
          </form>
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
