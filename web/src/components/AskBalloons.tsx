import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { respondAsk } from '../api';
import type { AskRequest } from '../api';

interface Props {
  asks: AskRequest[];
  onResolved: (requestId: string) => void;
}

// AskBalloons: pilha de balões no canto inferior direito. Cada balão é um pedido
// de permissão (Allow/Deny) ou uma pergunta do modelo (opções clicáveis ou texto
// livre). Responder chama o backend e remove o card localmente.
export default function AskBalloons({ asks, onResolved }: Props) {
  if (asks.length === 0) return null;
  return (
    <div className="ask-balloons">
      {asks.map((a) => (
        <Balloon key={a.request_id} ask={a} onResolved={onResolved} />
      ))}
    </div>
  );
}

function Balloon({ ask, onResolved }: { ask: AskRequest; onResolved: (id: string) => void }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);

  async function answer(value: string) {
    if (busy) return;
    setBusy(true);
    try {
      await respondAsk(ask.request_id, value);
    } catch { /* pode já ter sido resolvido/cancelado */ }
    onResolved(ask.request_id);
  }

  const hasOptions = ask.options && ask.options.length > 0;

  return (
    <div className="ask-balloon" role="alertdialog" aria-label={ask.title}>
      <button
        className="ask-balloon-session"
        onClick={() => navigate(`/sessions/${ask.session_id}`)}
        title={t('ask.openSession')}
      >
        {ask.session_label}
      </button>
      <div className="ask-balloon-title">{ask.title}</div>
      {ask.detail && <pre className="ask-balloon-detail">{ask.detail}</pre>}

      {ask.kind === 'permission' ? (
        <div className="ask-balloon-actions">
          <button className="btn btn-primary btn-sm" disabled={busy} onClick={() => answer('allow')}>
            {t('ask.allow')}
          </button>
          <button className="btn btn-danger btn-sm" disabled={busy} onClick={() => answer('deny')}>
            {t('ask.deny')}
          </button>
        </div>
      ) : hasOptions ? (
        <div className="ask-balloon-actions">
          {ask.options!.map((opt) => (
            <button key={opt} className="btn btn-secondary btn-sm" disabled={busy} onClick={() => answer(opt)}>
              {opt}
            </button>
          ))}
        </div>
      ) : (
        <form
          className="ask-balloon-actions"
          onSubmit={(e) => { e.preventDefault(); if (text.trim()) answer(text.trim()); }}
        >
          <input
            className="ask-balloon-input"
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder={t('ask.placeholder')}
            autoFocus
          />
          <button className="btn btn-primary btn-sm" type="submit" disabled={busy || !text.trim()}>
            {t('ask.send')}
          </button>
        </form>
      )}
    </div>
  );
}
