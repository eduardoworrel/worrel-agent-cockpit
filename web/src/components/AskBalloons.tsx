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

// Ícone de prompt de terminal (›_) usado no segmento secundário do split button.
function TerminalIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M5 8l4 4-4 4" />
      <path d="M13 16h6" />
    </svg>
  );
}

type Variant = 'primary' | 'danger' | 'secondary';

// SplitButton: um único botão com duas ações fundidas. O corpo principal executa
// `onMain` (responde); o segmento secundário (ícone de terminal) executa `onAux`
// (responde e abre o terminal da sessão). Visualmente é uma peça só.
function SplitButton({
  label,
  variant,
  disabled,
  auxTitle,
  onMain,
  onAux,
}: {
  label: string;
  variant: Variant;
  disabled?: boolean;
  auxTitle: string;
  onMain: () => void;
  onAux: () => void;
}) {
  return (
    <span className={`split-btn is-${variant}`}>
      <button
        type="button"
        className={`btn btn-${variant} btn-sm split-btn-main`}
        disabled={disabled}
        onClick={onMain}
      >
        {label}
      </button>
      <button
        type="button"
        className="split-btn-aux"
        disabled={disabled}
        title={auxTitle}
        aria-label={auxTitle}
        onClick={onAux}
      >
        <TerminalIcon />
      </button>
    </span>
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

  // Responde e abre o terminal da sessão (ação do segmento secundário).
  async function answerAndOpen(value: string) {
    await answer(value);
    navigate(`/sessions/${ask.session_id}`);
  }

  const hasOptions = ask.options && ask.options.length > 0;
  const auxTitle = t('ask.answerAndOpen');

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
          <SplitButton
            label={t('ask.allow')}
            variant="primary"
            disabled={busy}
            auxTitle={auxTitle}
            onMain={() => answer('allow')}
            onAux={() => answerAndOpen('allow')}
          />
          <SplitButton
            label={t('ask.deny')}
            variant="danger"
            disabled={busy}
            auxTitle={auxTitle}
            onMain={() => answer('deny')}
            onAux={() => answerAndOpen('deny')}
          />
        </div>
      ) : hasOptions ? (
        <div className="ask-balloon-actions">
          {ask.options!.map((opt) => (
            <SplitButton
              key={opt}
              label={opt}
              variant="secondary"
              disabled={busy}
              auxTitle={auxTitle}
              onMain={() => answer(opt)}
              onAux={() => answerAndOpen(opt)}
            />
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
          <span className="split-btn is-primary">
            <button className="btn btn-primary btn-sm split-btn-main" type="submit" disabled={busy || !text.trim()}>
              {t('ask.send')}
            </button>
            <button
              type="button"
              className="split-btn-aux"
              disabled={busy || !text.trim()}
              title={auxTitle}
              aria-label={auxTitle}
              onClick={() => answerAndOpen(text.trim())}
            >
              <TerminalIcon />
            </button>
          </span>
        </form>
      )}
    </div>
  );
}
