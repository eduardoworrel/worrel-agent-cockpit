import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Session, Suggestion, InteractionSnapshot } from '../api';
import { listSuggestions, getEngineEnabled, setEngineEnabled } from '../api';
import TerminalCard from '../components/TerminalCard';

interface Props {
  // Sessões vivas (com processo ativo) — derivadas de useAppState no App.
  liveSessions: Session[];
  awaitingIds: Set<string>;
  // Snapshots AG-UI das sessões vivas — fonte ÚNICA gerida pelo App (useSnapshots),
  // compartilhada com o farol de estado da sidebar.
  snapshots: Record<string, InteractionSnapshot>;
  // Re-busca os snapshots após uma ação no card (responder/enviar/toggle resumo).
  reloadSnapshots: () => void;
  onNewSession: () => void;
  // reloadKey muda quando uma sugestão é criada → recontamos por sessão.
  reloadKey: number;
  // Abre o modal global de interação de uma sessão (clique no ⚠️ do card).
  onOpenSession: (sessionId: string) => void;
}

// Home é a tela de gestão: todas as sessões de terminal vivas como cards num
// grid, cada uma resumida em linhas simples e interagível pelo canal AG-UI.
export default function Home({ liveSessions, awaitingIds, snapshots, reloadSnapshots, onNewSession, reloadKey, onOpenSession }: Props) {
  const { t } = useTranslation();
  const [pendingBySession, setPendingBySession] = useState<Record<string, number>>({});
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
    setEngineEnabled('summary', on, id).then(reloadSnapshots).catch(() => loadSummaryFlags());
  }, [reloadSnapshots, loadSummaryFlags]);

  useEffect(() => { loadCounts(); }, [loadCounts, reloadKey]);
  useEffect(() => { loadSummaryFlags(); }, [loadSummaryFlags]);

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
              onActed={reloadSnapshots}
              onOpen={() => onOpenSession(s.id)}
              summaryEnabled={summaryOn[s.id] ?? false}
              onToggleSummary={(on) => toggleSummary(s.id, on)}
            />
          ))}
        </div>
      )}
    </div>
  );
}
