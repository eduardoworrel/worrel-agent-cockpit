import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { Session, InteractionSnapshot } from '../api';
import { sessionName } from '../session';
import { sessionStatus } from '../sessionStatus';

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
  // Abre o modal global de interação desta sessão (clique no ⚠️).
  onOpen: () => void;
  // Toggle de custo do resumo de IA desta miniatura (Plano 3).
  summaryEnabled: boolean;
  onToggleSummary: (enabled: boolean) => void;
}

// kind do evento da timeline → cor da bolinha:
// 'you' (verde) = o usuário respondeu; 'ai' (âmbar) = a IA pergunta/espera você;
// 'idle' (cinza) = a IA respondeu sem esperar nada do usuário;
// 'neutral' (azul) = narração de progresso, summary ou placeholder.
type EventKind = 'you' | 'ai' | 'idle' | 'neutral';
interface TimelineEvent { kind: EventKind; text: string; }

// aiKind decide a cor da fala da IA: âmbar quando ela espera você (interrupt ou
// estado aguardando); cinza quando ela só respondeu e não há nada pendente.
function aiKind(awaitsYou: boolean): EventKind {
  return awaitsYou ? 'ai' : 'idle';
}

// frozenTail mostra as últimas linhas do histórico cru (o "final" da sessão),
// usado quando o resumo de IA está desligado para aquela miniatura.
function frozenTail(snapshot: InteractionSnapshot | undefined, awaitsYou: boolean, fallback: string): TimelineEvent[] {
  const h = snapshot?.history ?? [];
  if (h.length === 0) return [{ kind: 'neutral', text: fallback }];
  const tail = h
    .filter((l) => l.text.trim().length > 0)
    .slice(-3)
    .map((l): TimelineEvent => ({
      kind: l.role === 'you' ? 'you' : l.role === 'ai' ? aiKind(awaitsYou) : 'neutral',
      text: l.text,
    }));
  return tail.length > 0 ? tail : [{ kind: 'neutral', text: fallback }];
}

// timelineLines escolhe o que mostrar na timeline do card, em ordem de preferência:
// 1) o resumo narrado por IA (snapshot.progress); 2) último pedido + última fala
// da IA; 3) summary persistido; 4) placeholder.
function timelineLines(s: Session, snapshot: InteractionSnapshot | undefined, awaitsYou: boolean, fallback: string): TimelineEvent[] {
  if (snapshot?.progress && snapshot.progress.length > 0)
    return snapshot.progress.slice(0, 3).map((text): TimelineEvent => ({ kind: 'neutral', text }));
  // Sessão do motor: o card mostra SÓ os eventos narrados; enquanto não chegam,
  // fica o placeholder — nunca as mensagens cruas trocadas.
  if (s.adapter === 'engine') return [{ kind: 'neutral', text: fallback }];
  const lines: TimelineEvent[] = [];
  if (snapshot?.user_message) lines.push({ kind: 'you', text: snapshot.user_message });
  if (snapshot?.message) lines.push({ kind: aiKind(awaitsYou), text: snapshot.message });
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
export default function TerminalCard({ session, snapshot, awaiting, suggestions, onOpen, summaryEnabled, onToggleSummary }: Props) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const goTerminal = () => navigate(`/sessions/${session.id}`);

  // Sessão clássica: terminal puro (PTY/CLI real), sem o motor de eventos. Não
  // tem snapshot AG-UI (resumo por IA, interrupts, timeline narrada), então o
  // card se reduz ao essencial: título, dica e o botão de abrir o terminal.
  const classic = session.adapter !== 'engine';

  // A IA espera você quando há interrupt pendente OU o estado é "awaiting".
  const awaitsYou = snapshot
    ? (!!snapshot.interrupt || snapshot.state === 'awaiting')
    : awaiting;

  // Resumo ligado: timeline narrada por IA. Desligado: cauda crua do histórico
  // (sem rolagem, só o final), custo zero.
  const lines = summaryEnabled
    ? timelineLines(session, snapshot, awaitsYou, t('home.noProgress'))
    : frozenTail(snapshot, awaitsYou, t('home.summaryOff', 'Resumo por IA desligado'));

  // "Visto": ao clicar no ⚠️ a pessoa já viu o pedido atual, então o alerta volta
  // ao cinza. Persistimos a assinatura do pedido visto (request_id da interrupt ou
  // a fala da IA); se um NOVO pedido chegar, a assinatura muda e o ⚠️ reacende.
  const attentionSig = snapshot?.interrupt?.request_id ?? snapshot?.message ?? (awaitsYou ? 'awaiting' : '');
  const seenKey = `cockpit.ackd.${session.id}`;
  const [seenSig, setSeenSig] = useState<string>(() => localStorage.getItem(seenKey) ?? '');
  const markSeen = () => {
    if (attentionSig) { localStorage.setItem(seenKey, attentionSig); setSeenSig(attentionSig); }
  };

  // Precisa de atenção: a IA espera você E você ainda não viu este pedido.
  const needsAttention = awaitsYou && attentionSig !== seenSig;

  // Farol de estado: derivação ÚNICA compartilhada com a bolinha da sidebar
  // (ver sessionStatus) — em andamento / esperando você / parada / clássica.
  const status = sessionStatus({ snapshot, awaiting, classic });
  const stateLabel = classic ? t('home.classicBadge') : t(`home.ix.state.${status}`);

  return (
    <div className={`tcard${classic ? ' tcard-classic' : ''}${!classic && needsAttention ? ' needs-attention' : ''}`}>
      <div className="tcard-titlebar" onClick={goTerminal} role="button" tabIndex={0}
        onKeyDown={(e) => { if (e.key === 'Enter') goTerminal(); }}>
        {/* Farol de estado: verde pulsante = em andamento; âmbar = esperando você; cinza = parada.
            Clássico não tem motor → farol neutro. */}
        <span className="tcard-status" data-state={status}
          title={stateLabel} role="img" aria-label={stateLabel} />
        <span className="tcard-dots" aria-hidden="true"><i /><i /><i /></span>
        {/* Título da sessão: reflete o que está acontecendo e é atualizado com frequência. */}
        <span className="tcard-mode" title={sessionName(session)}>{sessionName(session)}</span>
        {classic ? (
          // Clássico: sem switch de resumo por IA (recurso do motor). Badge no lugar.
          <span className="tcard-classic-badge">{t('home.classicBadge')}</span>
        ) : (
          // Switch de resumo por IA (custa créditos) — ocupa o lugar antes usado pela badge do adapter.
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
        )}
      </div>

      <div className="tcard-body">
        {classic ? (
          <p className="tcard-classic-hint">{t('home.classicHint')}</p>
        ) : (
          <ol className="tcard-timeline">
            {lines.map((line, i) => (
              <li key={i} data-kind={line.kind}>{line.text}</li>
            ))}
          </ol>
        )}
      </div>

      <div className="tcard-foot">
        {/* Clássico não tem canal AG-UI → sem ⚠️ de interação. */}
        {!classic && (
          <button
            className={`tcard-alert${needsAttention ? ' on' : ''}`}
            title={needsAttention ? t('home.ix.open') : ''}
            disabled={!snapshot}
            onClick={() => { markSeen(); onOpen(); }}
          >
            ⚠️
          </button>
        )}
        <button className="tcard-send" aria-label={t('home.open')} onClick={goTerminal}>
          ➤
        </button>
        {suggestions > 0 && (
          <span className="tcard-suggestions">
            <b>{suggestions}</b> {t('home.suggestions', { count: suggestions })}
          </span>
        )}
      </div>
    </div>
  );
}
