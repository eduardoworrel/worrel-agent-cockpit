import { useCallback, useEffect, useState } from 'react';
import type { InteractionSnapshot, Session } from './api';
import { getInteraction } from './api';
import { useEvents } from './useEvents';

// Transições em que o snapshot AG-UI pode ter mudado (pergunta abriu/fechou,
// turno virou ocioso/trabalhando/encerrado, título derivado).
const REFRESH_EVENTS = [
  'ask.requested', 'ask.resolved', 'session.awaiting', 'session.busy',
  'session.ended', 'session.titled', 'interaction.changed',
];

// useSnapshots mantém o snapshot AG-UI (estado/timeline/interrupt) de cada
// sessão viva, re-buscando nas transições relevantes. É a fonte ÚNICA lida pela
// Home (conteúdo dos cards) e, via sessionStatus, pelo farol de estado tanto do
// card quanto da bolinha da sidebar — garantindo as duas bolinhas sincronizadas.
export function useSnapshots(liveSessions: Session[]): [Record<string, InteractionSnapshot>, () => void] {
  const [snapshots, setSnapshots] = useState<Record<string, InteractionSnapshot>>({});
  const ids = liveSessions.map((s) => s.id).join(',');

  const reload = useCallback(() => {
    const list = ids ? ids.split(',') : [];
    Promise.all(list.map((id) => getInteraction(id).then((s) => [id, s] as const).catch(() => null)))
      .then((pairs) => {
        const next: Record<string, InteractionSnapshot> = {};
        for (const p of pairs) if (p) next[p[0]] = p[1];
        setSnapshots(next);
      });
  }, [ids]);

  useEffect(() => { reload(); }, [reload]);

  useEvents(useCallback((ev) => {
    if (REFRESH_EVENTS.includes(ev.type)) reload();
  }, [reload]));

  return [snapshots, reload];
}
