import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Session, Suggestion, InteractionSnapshot } from '../api';
import { listSuggestions, getInteraction, getEngineEnabled, setEngineEnabled } from '../api';
import { useEvents } from '../useEvents';
import TerminalCard from '../components/TerminalCard';

interface Props {
  // Sessões vivas (com processo ativo) — derivadas de useAppState no App.
  liveSessions: Session[];
  awaitingIds: Set<string>;
  onNewSession: () => void;
  // reloadKey muda quando uma sugestão é criada → recontamos por sessão.
  reloadKey: number;
}

// Home é a tela de gestão: todas as sessões de terminal vivas como cards num
// grid, cada uma resumida em linhas simples e interagível pelo canal AG-UI.
export default function Home({ liveSessions, awaitingIds, onNewSession, reloadKey }: Props) {
  const { t } = useTranslation();
  const [pendingBySession, setPendingBySession] = useState<Record<string, number>>({});
  const [snapshots, setSnapshots] = useState<Record<string, InteractionSnapshot>>({});
  // Estado do toggle de resumo por IA por sessão (Plano 3: custo on/off).
  const [summaryOn, setSummaryOn] = useState<Record<string, boolean>>({});

  const ids = liveSessions.map((s) => s.id).join(',');

  const loadCounts = useCallback(() => {
    listSuggestions(undefined, 'pending')
      .then((all: Suggestion[]) => {
        const counts: Record<string, number> = {};
        for (const s of all) {
          if (s.session_id) counts[s.session_id] = (counts[s.session_id] ?? 0) + 1;
        }
        setPendingBySession(counts);
      })
      .catch(() => setPendingBySession({}));
  }, []);

  // loadSnapshots busca o snapshot AG-UI de cada sessão viva (estado, contexto,
  // interrupt). Re-disparado em transições relevantes (ver useEvents abaixo).
  const loadSnapshots = useCallback(() => {
    const list = ids ? ids.split(',') : [];
    Promise.all(list.map((id) => getInteraction(id).then((s) => [id, s] as const).catch(() => null)))
      .then((pairs) => {
        const next: Record<string, InteractionSnapshot> = {};
        for (const p of pairs) if (p) next[p[0]] = p[1];
        setSnapshots(next);
      });
  }, [ids]);

  // loadSummaryFlags resolve o toggle de resumo (default OFF) de cada sessão viva.
  const loadSummaryFlags = useCallback(() => {
    const list = ids ? ids.split(',') : [];
    Promise.all(list.map((id) =>
      getEngineEnabled('summary', id, false).then((on) => [id, on] as const).catch(() => [id, false] as const),
    )).then((pairs) => {
      const next: Record<string, boolean> = {};
      for (const [id, on] of pairs) next[id] = on;
      setSummaryOn(next);
    });
  }, [ids]);

  const toggleSummary = useCallback((id: string, on: boolean) => {
    setSummaryOn((prev) => ({ ...prev, [id]: on })); // otimista
    setEngineEnabled('summary', on, id).then(loadSnapshots).catch(() => loadSummaryFlags());
  }, [loadSnapshots, loadSummaryFlags]);

  useEffect(() => { loadCounts(); }, [loadCounts, reloadKey]);
  useEffect(() => { loadSnapshots(); }, [loadSnapshots]);
  useEffect(() => { loadSummaryFlags(); }, [loadSummaryFlags]);

  // Re-busca o snapshot quando o canal de interação de alguma sessão muda:
  // pergunta ab/fechou, ou o turno virou (ocioso/trabalhando/encerrado).
  useEvents(useCallback((ev) => {
    if (['ask.requested', 'ask.resolved', 'session.awaiting', 'session.busy', 'session.ended', 'session.titled', 'interaction.changed'].includes(ev.type)) {
      loadSnapshots();
    }
  }, [loadSnapshots]));

  return (
    <div className="home">
      <header className="home-head">
        <button className="home-new-session" onClick={onNewSession}>
          {t('home.newSession')}
        </button>
      </header>

      {liveSessions.length === 0 ? (
        <div className="home-empty">{t('home.empty')}</div>
      ) : (
        <div className="home-grid">
          {liveSessions.map((s) => (
            <TerminalCard
              key={s.id}
              session={s}
              snapshot={snapshots[s.id]}
              awaiting={awaitingIds.has(s.id)}
              suggestions={pendingBySession[s.id] ?? 0}
              onActed={loadSnapshots}
              summaryEnabled={summaryOn[s.id] ?? false}
              onToggleSummary={(on) => toggleSummary(s.id, on)}
            />
          ))}
        </div>
      )}
    </div>
  );
}
