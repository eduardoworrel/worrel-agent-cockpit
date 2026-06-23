import { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { getInteraction } from '../api';
import type { InteractionSnapshot } from '../api';
import { useEvents } from '../useEvents';
import ResponderShell from './ResponderShell';
import InteractionPanel from './InteractionPanel';

interface Props {
  // Sessão cuja interação está aberta (auto, clique no card ou bolinha de adiada).
  sessionId: string;
  onClose: () => void;
}

// GlobalInteractionModal é a janela de resposta ao agente promovida a nível
// global (App). Busca o snapshot AG-UI da sessão aberta e se reatualiza quando a
// interação dela muda. Fechar (X/overlay) não responde nada — só some.
export default function GlobalInteractionModal({ sessionId, onClose }: Props) {
  const navigate = useNavigate();
  const [snap, setSnap] = useState<InteractionSnapshot | null>(null);

  const load = useCallback(() => {
    getInteraction(sessionId).then(setSnap).catch(() => setSnap(null));
  }, [sessionId]);
  useEffect(() => { setSnap(null); load(); }, [load]);

  useEvents(useCallback((ev) => {
    const p = ev.payload as { session_id?: string; id?: string };
    const sid = p?.session_id ?? p?.id;
    if (sid === sessionId &&
      ['ask.requested', 'ask.resolved', 'session.awaiting', 'session.busy', 'session.ended', 'interaction.changed'].includes(ev.type)) {
      load();
    }
  }, [sessionId, load]));

  if (!snap) return null;
  return (
    <ResponderShell onClose={onClose}>
      <InteractionPanel
        snapshot={snap}
        onActed={load}
        onClose={onClose}
        onOpenChat={() => { onClose(); navigate(`/sessions/${sessionId}`); }}
      />
    </ResponderShell>
  );
}
