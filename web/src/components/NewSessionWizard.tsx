import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  listProjects, listAdapters, listSkills, listAgents,
  createEngineSession, sendPrompt,
} from '../api';
import type { Project, DetectedAdapter, Skill, Agent, Session, PermissionMode } from '../api';

interface Props {
  // openTerminal=true abre o terminal da sessão; false fica na Home (o terminal
  // vira a miniatura AG-UI no card).
  onCreated: (sess: Session, openTerminal: boolean) => void;
  onClose: () => void;
}

// Glifo de prompt de terminal (›_) para o segmento "abrir terminal".
function TerminalGlyph() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2"
      strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" width="15" height="15">
      <path d="M5 8l4 4-4 4" />
      <path d="M13 16h6" />
    </svg>
  );
}

type Mode = 'on-demand' | 'from-begin';

// NewSessionWizard inicia uma sessão em dois passos, dentro de uma "janela de
// terminal": a window-chrome carrega o ambiente (modo + provider), e o corpo
// carrega a intenção (passo 1 = onde; passo 2 = skills + tarefa). Um rail
// vertical de dois nós mostra o progresso.
//
// O prompt ("o que vamos fazer?") é capturado aqui; a injeção no primer da
// sessão é um passo de backend posterior (hoje só inicia a sessão com a skill).
export default function NewSessionWizard({ onCreated, onClose }: Props) {
  const { t } = useTranslation();
  const [step, setStep] = useState<1 | 2>(1);

  const [projects, setProjects] = useState<Project[]>([]);
  const [adapters, setAdapters] = useState<DetectedAdapter[]>([]);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);

  // projectId: undefined = nada escolhido; null = "sem projeto" (sessão livre).
  const [projectId, setProjectId] = useState<string | null | undefined>(undefined);
  const [mode, setMode] = useState<Mode>('on-demand');
  const [adapterId, setAdapterId] = useState('');
  const [permMode, setPermMode] = useState<PermissionMode>('auto');
  // seed: a semente da sessão — nada, uma skill ou um agent.
  const [seed, setSeed] = useState<{ kind: 'skill' | 'agent'; id: string } | null>(null);
  const [prompt, setPrompt] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    listProjects().then(setProjects).catch(() => setProjects([]));
    listAdapters().then((a) => {
      const present = a.filter((x) => x.installed.present);
      setAdapters(present);
      if (present[0]) setAdapterId(present[0].id);
    }).catch(() => { /* ignore */ });
  }, []);

  // Ao escolher um projeto real, carrega suas skills E agents (sementes) p/ o passo 2.
  useEffect(() => {
    if (projectId) {
      listSkills(projectId).then(setSkills).catch(() => setSkills([]));
      listAgents(projectId).then(setAgents).catch(() => setAgents([]));
    } else {
      setSkills([]);
      setAgents([]);
    }
  }, [projectId]);

  useEffect(() => {
    function onKey(e: KeyboardEvent) { if (e.key === 'Escape') onClose(); }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose]);

  function pickProject(id: string | null) {
    setProjectId(id);
    setSeed(null);
    setStep(2);
  }

  async function start(openTerminal: boolean) {
    if (busy || !adapterId) return;
    setBusy(true);
    setError(null);
    try {
      // Toda sessão é dirigida pelo motor stream-json (auto-mode). openTerminal
      // só decide a navegação: ficar na Home (miniatura) ou abrir a conversa.
      const sess = await createEngineSession(projectId ?? undefined, permMode);
      if (prompt.trim()) await sendPrompt(sess.id, prompt.trim());
      onCreated(sess, openTerminal);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.actionFailed'));
      setBusy(false);
    }
  }

  const projectLabel = projectId === null
    ? t('home.wizard.noProject')
    : projects.find((p) => p.id === projectId)?.name ?? '';

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="nsw" role="dialog" aria-modal="true" aria-label={t('home.wizard.title')}
        onClick={(e) => e.stopPropagation()}>

        {/* Window-chrome: ambiente da sessão (modo + provider). */}
        <div className="nsw-chrome">
          <span className="nsw-dots" aria-hidden="true"><i /><i /><i /></span>
          <label className="nsw-mode-select" title={t('home.wizard.permModeHint')}>
            <select value={permMode} onChange={(e) => setPermMode(e.target.value as PermissionMode)}
              aria-label={t('home.wizard.permMode')}>
              <option value="auto">{t('home.wizard.permAuto')}</option>
              <option value="default">{t('home.wizard.permAsk')}</option>
              <option value="bypassPermissions">{t('home.wizard.permYolo')}</option>
            </select>
          </label>
          <label className="nsw-provider">
            <select value={adapterId} onChange={(e) => setAdapterId(e.target.value)}
              aria-label={t('home.wizard.provider')}>
              {adapters.length === 0 && <option value="">—</option>}
              {adapters.map((a) => <option key={a.id} value={a.id}>{a.id}</option>)}
            </select>
          </label>
        </div>

        {error && <p className="error-banner nsw-error">{error}</p>}

        <div className="nsw-stage">
          {/* Rail de passos: dois nós com conector. */}
          <ol className="nsw-rail" aria-hidden="true">
            <li className={`nsw-node${step === 1 ? ' active' : ' done'}`}>
              <span className="nsw-dot" />
              <span className="nsw-rail-label">{t('home.wizard.railWhere')}</span>
            </li>
            <li className={`nsw-node${step === 2 ? ' active' : ''}`}>
              <span className="nsw-dot" />
              <span className="nsw-rail-label">{t('home.wizard.railWhat')}</span>
            </li>
          </ol>

          {step === 1 ? (
            <div className="nsw-panel" key="s1">
              <h3 className="nsw-q">{t('home.wizard.where')}</h3>
              <ul className="nsw-projects">
                <li>
                  <button className={`nsw-project${projectId === null ? ' on' : ''}`}
                    onClick={() => pickProject(null)}>
                    <span className="nsw-caret">▸</span>{t('home.wizard.noProject')}
                  </button>
                </li>
                {projects.map((p) => (
                  <li key={p.id}>
                    <button className={`nsw-project${projectId === p.id ? ' on' : ''}`}
                      onClick={() => pickProject(p.id)}>
                      <span className="nsw-caret">▸</span>{p.name}
                    </button>
                  </li>
                ))}
              </ul>

              <div className="nsw-field">
                <span className="nsw-field-label">{t('home.wizard.mode')}</span>
                <div className="nsw-modes">
                  {(['on-demand', 'from-begin'] as Mode[]).map((m) => (
                    <button key={m} className={`nsw-mode${mode === m ? ' on' : ''}`}
                      onClick={() => setMode(m)} aria-pressed={mode === m}>
                      <span className="nsw-mode-name">
                        {t(m === 'on-demand' ? 'home.wizard.onDemand' : 'home.wizard.fromBegin')}
                      </span>
                      <span className="nsw-mode-desc">
                        {t(m === 'on-demand' ? 'home.wizard.onDemandDesc' : 'home.wizard.fromBeginDesc')}
                      </span>
                    </button>
                  ))}
                </div>
              </div>
            </div>
          ) : (
            <div className="nsw-panel" key="s2">
              <button className="nsw-crumb" onClick={() => setStep(1)}>
                ← <b>{projectLabel}</b> · {t(mode === 'on-demand' ? 'home.wizard.onDemand' : 'home.wizard.fromBegin')}
              </button>

              <h3 className="nsw-q">{t('home.wizard.skillsAgents')}</h3>
              <div className="nsw-skills">
                <button className={`nsw-skill${seed === null ? ' on' : ''}`} onClick={() => setSeed(null)}>
                  {t('home.wizard.skillNone')}
                </button>
                {skills.map((s) => (
                  <button key={s.id}
                    className={`nsw-skill${seed?.kind === 'skill' && seed.id === s.id ? ' on' : ''}`}
                    onClick={() => setSeed({ kind: 'skill', id: s.id })}>
                    {s.name}
                  </button>
                ))}
                {agents.map((a) => (
                  <button key={a.id}
                    className={`nsw-skill is-agent${seed?.kind === 'agent' && seed.id === a.id ? ' on' : ''}`}
                    onClick={() => setSeed({ kind: 'agent', id: a.id })}
                    title={a.persona}>
                    <span className="nsw-agent-mark" aria-hidden="true">◆</span>{a.name}
                  </button>
                ))}
              </div>

              <label className="nsw-promptline">
                <span className="nsw-prompt-glyph" aria-hidden="true">›</span>
                <textarea
                  className="nsw-prompt"
                  placeholder={t('home.wizard.promptPlaceholder')}
                  value={prompt}
                  onChange={(e) => setPrompt(e.target.value)}
                  rows={2}
                  autoFocus
                />
              </label>

              <div className="nsw-actions">
                <button className="nsw-back" onClick={() => setStep(1)}>{t('common.back')}</button>
                {/* Split: corpo fica na Home (miniatura AG-UI); aux abre o terminal. */}
                <span className="nsw-start-split">
                  <button className="nsw-start" disabled={busy || !adapterId}
                    onClick={() => start(false)}>
                    {busy ? t('common.loading') : <>{t('home.wizard.start')} <span aria-hidden="true">›</span></>}
                  </button>
                  <button className="nsw-start-aux" disabled={busy || !adapterId}
                    title={t('home.wizard.startTerminal')} aria-label={t('home.wizard.startTerminal')}
                    onClick={() => start(true)}>
                    <TerminalGlyph />
                  </button>
                </span>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
