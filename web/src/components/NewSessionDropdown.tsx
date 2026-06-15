import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { listSkills, listAdapters, createSession, createFreeSession } from '../api';
import type { Skill, Session, DetectedAdapter } from '../api';

interface Props {
  projectId: string | null; // null = MySpace (sessão livre, sem projeto)
  projectName: string;
  onClose: () => void;
  onStarted: (s: Session) => void;
}

// NewSessionDropdown: popover multistep ancorado no ＋ do projeto.
// Passo 1 escolhe a semente (memory only OU uma skill, que também carrega a
// memória); passo 2 escolhe o provider instalado e inicia. Fecha (e zera, pois
// desmonta) ao clicar fora, no ✕ ou Esc.
export default function NewSessionDropdown({ projectId, projectName, onClose, onStarted }: Props) {
  const { t } = useTranslation();
  const ref = useRef<HTMLDivElement>(null);
  const [step, setStep] = useState<1 | 2>(1);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [adapters, setAdapters] = useState<DetectedAdapter[]>([]);
  const [skillId, setSkillId] = useState(''); // '' = memory only
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

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

  async function start(adapter: string) {
    if (busy) return;
    setBusy(true);
    setErr(null);
    try {
      const sess = projectId
        ? await createSession(projectId, adapter, skillId || undefined)
        : await createFreeSession(adapter, [], skillId || undefined);
      onStarted(sess);
    } catch (e) {
      setErr(String(e));
      setBusy(false);
    }
  }

  return (
    <div className="ns-dropdown" ref={ref} role="dialog" aria-label={t('newSession.title', { name: projectName })}>
      <div className="ns-head">
        <span>{t('newSession.title', { name: projectName })}</span>
        <button className="ns-x" aria-label={t('common.cancel')} onClick={onClose}>✕</button>
      </div>

      {err && <div className="ns-err">{err}</div>}

      {step === 1 ? (
        <div className="ns-step">
          <div className="ns-step-label">{t('newSession.step1')}</div>
          <button className={`ns-opt ${skillId === '' ? 'active' : ''}`} onClick={() => setSkillId('')}>
            ◉ {t('newSession.memoryOnly')}
          </button>

          {projectId && skills.length > 0 && (
            <>
              <div className="ns-sub">{t('newSession.skills')}</div>
              <div className="ns-skills">
                {skills.map((s, i) => (
                  <button
                    key={s.id}
                    className={`ns-opt ${skillId === s.id ? 'active' : ''}`}
                    onClick={() => setSkillId(s.id)}
                  >
                    <span>{s.name}</span>
                    {i === 0 && s.last_used_at > 0 && <span className="ns-last">⟲ {t('newSession.lastUsed')}</span>}
                  </button>
                ))}
              </div>
            </>
          )}

          <div className="ns-foot">
            <button className="btn btn-primary" onClick={() => setStep(2)}>{t('newSession.next')} →</button>
          </div>
        </div>
      ) : (
        <div className="ns-step">
          <div className="ns-step-head">
            <button className="ns-back" onClick={() => setStep(1)}>‹ {t('newSession.back')}</button>
            <div className="ns-step-label">{t('newSession.step2')}</div>
          </div>
          {adapters.length === 0 ? (
            <p className="muted">{t('newSession.noProviders')}</p>
          ) : (
            <div className="ns-providers">
              {adapters.map((a) => (
                <button key={a.id} className="ns-opt" disabled={busy} onClick={() => start(a.id)}>
                  <span>{a.id}</span>
                  <span className="ns-ver">{a.installed.version}</span>
                </button>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
