import { useEffect, useRef, useState } from 'react';
import { listSkills, listAdapters, createSession, createFreeSession } from '../api';
import type { Skill, Session, DetectedAdapter } from '../api';

interface Props {
  projectId: string | null; // null = "no project" (sessão livre, sem projeto)
  anchor: { top: number; left: number }; // posição fixa (rect do ＋), foge do overflow
  onClose: () => void;
  onStarted: (s: Session) => void;
}

// NewSessionDropdown: menu limpo e direto. Fase 1 = sementes (memory + skills,
// só nomes); clicar avança para a fase 2 = providers instalados; clicar inicia.
// Sem títulos, sem rótulos de passo, sem botões — só linhas. Fecha no clique
// fora / Esc (e zera, pois desmonta).
export default function NewSessionDropdown({ projectId, anchor, onClose, onStarted }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  // "no project" (projectId null) não tem memória nem skills → vai direto a provider.
  const [phase, setPhase] = useState<'seed' | 'provider'>(projectId ? 'seed' : 'provider');
  const [skills, setSkills] = useState<Skill[]>([]);
  const [adapters, setAdapters] = useState<DetectedAdapter[]>([]);
  const [skillId, setSkillId] = useState('');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (projectId) listSkills(projectId).then(setSkills).catch(() => setSkills([]));
    listAdapters().then((a) => setAdapters(a.filter((x) => x.installed.present))).catch(() => setAdapters([]));
  }, [projectId]);

  useEffect(() => {
    function onDoc(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose();
    }
    document.addEventListener('mousedown', onDoc);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDoc);
      document.removeEventListener('keydown', onKey);
    };
  }, [onClose]);

  function pickSeed(id: string) {
    setSkillId(id);
    setPhase('provider');
  }

  async function start(adapter: string) {
    if (busy) return;
    setBusy(true);
    try {
      const sess = projectId
        ? await createSession(projectId, adapter, skillId ? { skillId } : undefined)
        : await createFreeSession(adapter, [], skillId || undefined);
      onStarted(sess);
    } catch {
      setBusy(false);
    }
  }

  return (
    <div className="ns-menu" ref={ref} style={{ top: anchor.top, left: anchor.left }} role="menu">
      {phase === 'seed' ? (
        <>
          <button className="ns-item" role="menuitem" onClick={() => pickSeed('')}>new</button>
          {projectId && skills.map((s, i) => (
            <button key={s.id} className="ns-item" role="menuitem" onClick={() => pickSeed(s.id)}>
              <span>{s.name}</span>
              {i === 0 && s.last_used_at > 0 && <span className="ns-dot" aria-hidden="true">⟲</span>}
            </button>
          ))}
        </>
      ) : (
        adapters.map((a) => (
          <button key={a.id} className="ns-item" role="menuitem" disabled={busy} onClick={() => start(a.id)}>
            {a.id}
          </button>
        ))
      )}
    </div>
  );
}
