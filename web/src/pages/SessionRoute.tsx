import { useParams } from 'react-router-dom';
import type { Session } from '../api';
import Terminal from './Terminal';
import SessionStream from './SessionStream';

// SessionRoute decide a "visão de sessão" pelo tipo: sessões do MOTOR
// (adapter "engine") não têm PTY → mostram a conversa estruturada
// (SessionStream); as demais abrem o terminal xterm (Terminal).
export default function SessionRoute({ sessions }: { sessions: Session[] }) {
  const { id } = useParams<{ id: string }>();
  const sess = sessions.find((s) => s.id === id);
  // Sessões legadas (adapter de CLI real, ex.: claude-code/opencode com PTV)
  // abrem o xterm. O motor (adapter "engine") e o caso desconhecido — incluindo
  // uma sessão recém-criada ainda não listada — abrem a conversa do motor.
  const legacyAdapters = ['claude-code', 'opencode', 'antigravity', 'codex', 'pidev'];
  if (sess && legacyAdapters.includes(sess.adapter)) return <Terminal />;
  return <SessionStream />;
}
