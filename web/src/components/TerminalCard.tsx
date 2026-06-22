import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { Session, InteractionSnapshot } from '../api';
import { ProviderBadge, sessionName } from '../session';
import InteractionPanel from './InteractionPanel';
import ResponderShell from './ResponderShell';

interface Props {
  session: Session;
  // Snapshot AG-UI da sessão (estado/contexto/interrupt). Pode faltar enquanto carrega.
  snapshot?: InteractionSnapshot;
  // Fallback de "aguardando" enquanto o snapshot não chegou (vem do App).
  awaiting: boolean;
  // Nº de sugestões pendentes geradas por esta sessão.
  suggestions: number;
  // Chamado após responder/enviar no painel → Home re-busca os snapshots.
  onActed: () => void;
}

// timelineLines escolhe o que mostrar na timeline do card, em ordem de preferência:
// 1) o resumo narrado por IA (snapshot.progress); 2) último pedido + última fala
// da IA; 3) summary persistido; 4) placeholder.
function timelineLines(s: Session, snapshot: InteractionSnapshot | undefined, fallback: string): string[] {
  if (snapshot?.progress && snapshot.progress.length > 0) return snapshot.progress.slice(0, 3);
  // Sessão do motor: o card mostra SÓ os eventos narrados; enquanto não chegam,
  // fica o placeholder — nunca as mensagens cruas trocadas.
  if (s.adapter === 'engine') return [fallback];
  const lines: string[] = [];
  if (snapshot?.user_message) lines.push(snapshot.user_message);
  if (snapshot?.message) lines.push(snapshot.message);
  if (lines.length > 0) return lines.slice(0, 3);
  // sem snapshot ainda: cai no summary persistido, senão placeholder.
  const fromSummary = (s.summary ?? '')
    .split('\n')
    .map((l) => l.replace(/^#+\s*/, '').replace(/^[-*]\s*/, '').replace(/[*_`]/g, '').trim())
    .filter((l) => l.length >= 4);
  return fromSummary.length > 0 ? fromSummary.slice(0, 3) : [fallback];
}

// TerminalCard é o card da Home: uma sessão "ao vivo" resumida em linhas
// simples, com o ⚠️ que abre o canal de interação (responder/mandar prompt).
export default function TerminalCard({ session, snapshot, awaiting, suggestions, onActed }: Props) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const goTerminal = () => navigate(`/sessions/${session.id}`);
  const lines = timelineLines(session, snapshot, t('home.noProgress'));

  // Precisa de atenção: interrupt pendente OU ociosa aguardando o usuário.
  const needsAttention = snapshot
    ? (!!snapshot.interrupt || snapshot.state === 'awaiting')
    : awaiting;

  return (
    <div className={`tcard${needsAttention ? ' needs-attention' : ''}`}>
      <div className="tcard-titlebar" onClick={goTerminal} role="button" tabIndex={0}
        onKeyDown={(e) => { if (e.key === 'Enter') goTerminal(); }}>
        <span className="tcard-dots" aria-hidden="true"><i /><i /><i /></span>
        <span className="tcard-mode">{t('home.autoMode')}</span>
        <ProviderBadge adapter={session.adapter} />
      </div>

      <div className="tcard-body">
        <div className="tcard-heading">{sessionName(session)}</div>
        <ol className="tcard-timeline">
          {lines.map((line, i) => (
            <li key={i}>{line}</li>
          ))}
        </ol>
      </div>

      <div className="tcard-foot">
        <button
          className={`tcard-alert${needsAttention ? ' on' : ''}`}
          title={needsAttention ? t('home.ix.open') : ''}
          disabled={!snapshot}
          onClick={() => setOpen(true)}
        >
          ⚠️
        </button>
        <button className="tcard-send" aria-label={t('home.open')} onClick={goTerminal}>
          ➤
        </button>
        {suggestions > 0 && (
          <span className="tcard-suggestions">
            <b>{suggestions}</b> {t('home.suggestions', { count: suggestions })}
          </span>
        )}
      </div>

      {open && snapshot && (
        <ResponderShell onClose={() => setOpen(false)}>
          <InteractionPanel snapshot={snapshot} onActed={onActed} onClose={() => setOpen(false)}
            onOpenChat={() => { setOpen(false); navigate(`/sessions/${session.id}`); }} />
        </ResponderShell>
      )}
    </div>
  );
}
