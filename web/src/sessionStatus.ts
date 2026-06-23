import type { InteractionSnapshot } from './api';

// SessionStatus é o "farol" de estado de uma sessão, compartilhado entre o card
// da Home e a bolinha da sidebar. 'classic' = terminal puro (sem motor de
// eventos) → farol neutro, sem pulso.
export type SessionStatus = 'working' | 'awaiting' | 'ended' | 'classic';

// sessionStatus é a ÚNICA fonte da cor da bolinha: tanto a miniatura da Home
// quanto a lista de sessões ativas da sidebar derivam o estado por aqui, então
// qualquer mudança de estado reflete nos dois lugares por construção.
//
// Precedência: clássico vence tudo (neutro); senão usa o snapshot AG-UI; e
// enquanto o snapshot não chegou, cai no fallback de "aguardando".
export function sessionStatus(opts: {
  // Snapshot AG-UI da sessão (pode faltar enquanto carrega).
  snapshot?: InteractionSnapshot;
  // Fallback de "aguardando" antes de o snapshot chegar (vem do App).
  awaiting: boolean;
  // Sessão clássica (sem motor) → farol neutro.
  classic: boolean;
}): SessionStatus {
  if (opts.classic) return 'classic';
  return opts.snapshot?.state ?? (opts.awaiting ? 'awaiting' : 'working');
}
