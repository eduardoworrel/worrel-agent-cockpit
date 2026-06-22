import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { Session, InteractionSnapshot } from '../api';
import { sessionName } from '../session';
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
  // Toggle de custo do resumo de IA desta miniatura (Plano 3).
  summaryEnabled: boolean;
  onToggleSummary: (enabled: boolean) => void;
}

// kind do evento da timeline → cor da bolinha:
// 'you' (verde) = o usuário respondeu; 'ai' (âmbar) = a IA pergunta/fala;
// 'neutral' (azul) = narração de progresso, summary ou placeholder.
type EventKind = 'you' | 'ai' | 'neutral';
interface TimelineEvent { kind: EventKind; text: string; }

// frozenTail mostra as últimas linhas do histórico cru (o "final" da sessão),
// usado quando o resumo de IA está desligado para aquela miniatura.
function frozenTail(snapshot: InteractionSnapshot | undefined, fallback: string): TimelineEvent[] {
  const h = snapshot?.history ?? [];
  if (h.length === 0) return [{ kind: 'neutral', text: fallback }];
  const tail = h
    .filter((l) => l.text.trim().length > 0)
    .slice(-3)
    .map((l): TimelineEvent => ({
      kind: l.role === 'you' ? 'you' : l.role === 'ai' ? 'ai' : 'neutral',
      text: l.text,
    }));
  return tail.length > 0 ? tail : [{ kind: 'neutral', text: fallback }];
}

// timelineLines escolhe o que mostrar na timeline do card, em ordem de preferência:
// 1) o resumo narrado por IA (snapshot.progress); 2) último pedido + última fala
// da IA; 3) summary persistido; 4) placeholder.
function timelineLines(s: Session, snapshot: InteractionSnapshot | undefined, fallback: string): TimelineEvent[] {
  if (snapshot?.progress && snapshot.progress.length > 0)
    return snapshot.progress.slice(0, 3).map((text): TimelineEvent => ({ kind: 'neutral', text }));
  // Sessão do motor: o card mostra SÓ os eventos narrados; enquanto não chegam,
  // fica o placeholder — nunca as mensagens cruas trocadas.
  if (s.adapter === 'engine') return [{ kind: 'neutral', text: fallback }];
  const lines: TimelineEvent[] = [];
  if (snapshot?.user_message) lines.push({ kind: 'you', text: snapshot.user_message });
  if (snapshot?.message) lines.push({ kind: 'ai', text: snapshot.message });
  if (lines.length > 0) return lines.slice(0, 3);
  // sem snapshot ainda: cai no summary persistido, senão placeholder.
  const fromSummary = (s.summary ?? '')
    .split('\n')
    .map((l) => l.replace(/^#+\s*/, '').replace(/^[-*]\s*/, '').replace(/[*_`]/g, '').trim())
    .filter((l) => l.length >= 4);
  return fromSummary.length > 0
    ? fromSummary.slice(0, 3).map((text): TimelineEvent => ({ kind: 'neutral', text }))
    : [{ kind: 'neutral', text: fallback }];
}

// TerminalCard é o card da Home: uma sessão "ao vivo" resumida em linhas
// simples, com o ⚠️ que abre o canal de interação (responder/mandar prompt).
export default function TerminalCard({ session, snapshot, awaiting, suggestions, onActed, summaryEnabled, onToggleSummary }: Props) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const goTerminal = () => navigate(`/sessions/${session.id}`);
  // Resumo ligado: timeline narrada por IA. Desligado: cauda crua do histórico
  // (sem rolagem, só o final), custo zero.
  const lines = summaryEnabled
    ? timelineLines(session, snapshot, t('home.noProgress'))
    : frozenTail(snapshot, t('home.summaryOff', 'Resumo por IA desligado'));

  // Precisa de atenção: interrupt pendente OU ociosa aguardando o usuário.
  const needsAttention = snapshot
    ? (!!snapshot.interrupt || snapshot.state === 'awaiting')
    : awaiting;

  // Estado ao vivo para o farol da barra: em andamento / esperando você / parada.
  const liveState: 'working' | 'awaiting' | 'ended' =
    snapshot?.state ?? (awaiting ? 'awaiting' : 'working');
  const stateLabel = t(`home.ix.state.${liveState}`);

  return (
    <div className={`tcard${needsAttention ? ' needs-attention' : ''}`}>
      <div className="tcard-titlebar" onClick={goTerminal} role="button" tabIndex={0}
        onKeyDown={(e) => { if (e.key === 'Enter') goTerminal(); }}>
        {/* Farol de estado: verde pulsante = em andamento; âmbar = esperando você; cinza = parada. */}
        <span className="tcard-status" data-state={liveState} title={stateLabel} role="img" aria-label={stateLabel} />
        <span className="tcard-dots" aria-hidden="true"><i /><i /><i /></span>
        {/* Título da sessão: reflete o que está acontecendo e é atualizado com frequência. */}
        <span className="tcard-mode" title={sessionName(session)}>{sessionName(session)}</span>
        {/* Switch de resumo por IA (custa créditos) — ocupa o lugar antes usado pela badge do adapter. */}
        <label className="tcard-ai-switch" title={t('home.summaryToggle', 'Resumo por IA (custa créditos)')}
          onClick={(e) => e.stopPropagation()}>
          <input
            type="checkbox"
            checked={summaryEnabled}
            onChange={(e) => onToggleSummary(e.target.checked)}
          />
          <span className="tcard-ai-switch-track" aria-hidden="true">
            <span className="tcard-ai-switch-thumb" />
          </span>
          <span className="tcard-ai-switch-label">IA</span>
        </label>
      </div>

      <div className="tcard-body">
        <ol className="tcard-timeline">
          {lines.map((line, i) => (
            <li key={i} data-kind={line.kind}>{line.text}</li>
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
