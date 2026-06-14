import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { listProjects } from '../api';
import type { Project, Skill } from '../api';
import {
  listPipelineSkills,
  listPipelines,
  createPipeline,
  updatePipeline,
} from '../pipelinesApi';
import type { Pipeline, PipelineStep } from '../pipelinesApi';

function emptyStep(): PipelineStep {
  return { skill_id: '', note: '', inputs: '', credentials: '' };
}

function buildMarkdown(name: string, steps: PipelineStep[], skillName: (id: string) => string): string {
  const lines: string[] = [];
  lines.push(`# ${name || '...'}`);
  lines.push('');
  steps.forEach((s, i) => {
    lines.push(`## ${i + 1}. ${skillName(s.skill_id) || '—'}`);
    if (s.note.trim()) lines.push(s.note.trim());
    if (s.inputs.trim()) lines.push(`**Inputs:** ${s.inputs.trim()}`);
    if (s.credentials.trim()) lines.push(`**Credentials:** ${s.credentials.trim()}`);
    lines.push('');
  });
  return lines.join('\n');
}

export default function Pipelines() {
  const { t } = useTranslation();
  const tr = (key: string, dft: string, opts?: Record<string, unknown>) =>
    t(key, { defaultValue: dft, ...opts });

  const [projects, setProjects] = useState<Project[]>([]);
  const [projectId, setProjectId] = useState('');
  const [skills, setSkills] = useState<Skill[]>([]);
  const [pipelines, setPipelines] = useState<Pipeline[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Builder state
  const [building, setBuilding] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [name, setName] = useState('');
  const [steps, setSteps] = useState<PipelineStep[]>([]);
  const [busy, setBusy] = useState(false);
  const [showPreview, setShowPreview] = useState(true);

  useEffect(() => {
    listProjects()
      .then((ps) => {
        setProjects(ps);
        if (ps.length) setProjectId(ps[0].id);
      })
      .catch(() => setError(tr('common.actionFailed', 'Action failed')))
      .finally(() => setLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!projectId) return;
    closeBuilder();
    setError(null);
    Promise.all([listPipelineSkills(projectId), listPipelines(projectId)])
      .then(([sk, pl]) => {
        setSkills(sk);
        setPipelines(pl);
      })
      .catch(() => setError(tr('common.actionFailed', 'Action failed')));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId]);

  const skillName = useMemo(() => {
    const map: Record<string, string> = {};
    skills.forEach((s) => { map[s.id] = s.name; });
    return (id: string) => map[id] ?? '';
  }, [skills]);

  function refresh() {
    if (!projectId) return;
    listPipelines(projectId).then(setPipelines).catch(() => {});
  }

  function closeBuilder() {
    setBuilding(false);
    setEditingId(null);
    setName('');
    setSteps([]);
  }

  function startNew() {
    setEditingId(null);
    setName('');
    setSteps([emptyStep(), emptyStep()]);
    setBuilding(true);
    setError(null);
  }

  function startEdit(p: Pipeline) {
    setEditingId(p.id);
    setName(p.name);
    setSteps(p.steps.length ? p.steps.map((s) => ({ ...s })) : [emptyStep(), emptyStep()]);
    setBuilding(true);
    setError(null);
  }

  function updateStep(i: number, patch: Partial<PipelineStep>) {
    setSteps((prev) => prev.map((s, idx) => (idx === i ? { ...s, ...patch } : s)));
  }

  function addStep() {
    setSteps((prev) => [...prev, emptyStep()]);
  }

  function removeStep(i: number) {
    setSteps((prev) => prev.filter((_, idx) => idx !== i));
  }

  function move(i: number, dir: -1 | 1) {
    setSteps((prev) => {
      const j = i + dir;
      if (j < 0 || j >= prev.length) return prev;
      const next = [...prev];
      [next[i], next[j]] = [next[j], next[i]];
      return next;
    });
  }

  const validation = useMemo(() => {
    if (!name.trim()) return tr('pl.errNoName', 'Name the pipeline.');
    if (steps.length < 2) return tr('pl.errMinSteps', 'A pipeline needs at least 2 steps.');
    if (steps.some((s) => !s.skill_id)) return tr('pl.errStepSkill', 'Every step needs a skill.');
    return null;
  }, [name, steps]); // eslint-disable-line react-hooks/exhaustive-deps

  async function save() {
    if (validation || busy || !projectId) return;
    setBusy(true);
    setError(null);
    try {
      const payload = { name: name.trim(), steps };
      if (editingId) {
        await updatePipeline(editingId, payload);
      } else {
        await createPipeline(projectId, payload);
      }
      closeBuilder();
      refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : tr('common.actionFailed', 'Action failed'));
    } finally {
      setBusy(false);
    }
  }

  if (loading) return <div className="main"><p>{tr('common.loading', 'Loading...')}</p></div>;

  return (
    <div className="main">
      <div className="page-head">
        <div style={{ minWidth: 0 }}>
          <h1>{tr('pl.title', 'Pipelines')}</h1>
          <p className="sub">{tr('pl.subtitle', 'Compose ordered skills into a reusable pipeline.')}</p>
        </div>
        {projectId && !building && (
          <div className="actions">
            <button className="btn btn-accent" onClick={startNew}>
              {tr('pl.new', 'New pipeline')}
            </button>
          </div>
        )}
      </div>

      {error && <p className="error-banner">{error}</p>}

      {projects.length === 0 ? (
        <div className="empty">
          <h2>{tr('common.noProjects', 'No projects yet')}</h2>
          <p className="muted">{tr('pl.noProjectHint', 'Create a project to build pipelines.')}</p>
        </div>
      ) : (
        <div className="pl-field" style={{ maxWidth: 360, marginBottom: 24 }}>
          <label htmlFor="pl-project">{tr('pl.project', 'Project')}</label>
          <select
            id="pl-project"
            value={projectId}
            onChange={(e) => setProjectId(e.target.value)}
          >
            {projects.map((p) => (
              <option key={p.id} value={p.id}>{p.name}</option>
            ))}
          </select>
        </div>
      )}

      {projectId && building && (
        <div className="pl-builder">
          <div className="retro-card">
            <div className="retro-card-head">
              <h3>{editingId ? tr('pl.editing', 'Edit pipeline') : tr('pl.creating', 'New pipeline')}</h3>
              <p className="muted">{tr('pl.builderHint', 'Order the steps; each step runs a skill in sequence.')}</p>
            </div>

            <div className="retro-field">
              <label htmlFor="pl-name" className="retro-field-label">{tr('pl.name', 'Pipeline name')}</label>
              <input
                id="pl-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={tr('pl.namePh', 'e.g. Onboard new client')}
              />
            </div>

            <ol className="pl-steps">
              {steps.map((s, i) => (
                <li key={i} className="pl-step">
                  <div className="pl-step-rail">
                    <span className="pl-step-num">{i + 1}</span>
                    <div className="pl-step-moves">
                      <button
                        type="button"
                        className="btn btn-secondary btn-sm pl-move"
                        onClick={() => move(i, -1)}
                        disabled={i === 0}
                        aria-label={tr('pl.moveUp', 'Move up')}
                        title={tr('pl.moveUp', 'Move up')}
                      >↑</button>
                      <button
                        type="button"
                        className="btn btn-secondary btn-sm pl-move"
                        onClick={() => move(i, 1)}
                        disabled={i === steps.length - 1}
                        aria-label={tr('pl.moveDown', 'Move down')}
                        title={tr('pl.moveDown', 'Move down')}
                      >↓</button>
                    </div>
                  </div>

                  <div className="pl-step-body">
                    <div className="pl-step-head">
                      <div className="pl-field" style={{ flex: 1, minWidth: 0 }}>
                        <label className="retro-field-label">{tr('pl.step', 'Skill (step)')}</label>
                        <select
                          value={s.skill_id}
                          onChange={(e) => updateStep(i, { skill_id: e.target.value })}
                        >
                          <option value="">{tr('pl.chooseSkill', 'Choose a skill…')}</option>
                          {skills.map((sk) => (
                            <option key={sk.id} value={sk.id}>{sk.name}</option>
                          ))}
                        </select>
                      </div>
                      <button
                        type="button"
                        className="btn btn-danger btn-sm"
                        onClick={() => removeStep(i)}
                        disabled={steps.length <= 1}
                      >
                        {tr('pl.remove', 'Remove')}
                      </button>
                    </div>

                    <div className="pl-field">
                      <label className="retro-field-label">{tr('pl.note', 'Note')}</label>
                      <textarea
                        value={s.note}
                        onChange={(e) => updateStep(i, { note: e.target.value })}
                        placeholder={tr('pl.notePh', 'What this step should do…')}
                        rows={2}
                      />
                    </div>

                    <div className="retro-grid-2">
                      <div className="pl-field">
                        <label className="retro-field-label">{tr('pl.inputs', 'Inputs')}</label>
                        <input
                          value={s.inputs}
                          onChange={(e) => updateStep(i, { inputs: e.target.value })}
                          placeholder={tr('pl.inputsPh', 'Data passed in')}
                        />
                      </div>
                      <div className="pl-field">
                        <label className="retro-field-label">{tr('pl.credentials', 'Credentials')}</label>
                        <input
                          value={s.credentials}
                          onChange={(e) => updateStep(i, { credentials: e.target.value })}
                          placeholder={tr('pl.credentialsPh', 'Secret / key name')}
                        />
                      </div>
                    </div>
                  </div>
                </li>
              ))}
            </ol>

            <div>
              <button type="button" className="btn btn-secondary" onClick={addStep}>
                {tr('pl.addStep', '+ Add step')}
              </button>
            </div>

            {validation && <p className="muted" style={{ fontSize: '0.82rem' }}>{validation}</p>}

            <div className="retro-card-foot retro-card-foot-split">
              <button
                type="button"
                className="btn btn-secondary btn-sm"
                onClick={() => setShowPreview((v) => !v)}
              >
                {showPreview ? tr('pl.hidePreview', 'Hide preview') : tr('pl.showPreview', 'Show preview')}
              </button>
              <div style={{ display: 'flex', gap: 10 }}>
                <button type="button" className="btn btn-secondary" onClick={closeBuilder} disabled={busy}>
                  {tr('common.cancel', 'Cancel')}
                </button>
                <button
                  type="button"
                  className="btn btn-primary"
                  onClick={save}
                  disabled={!!validation || busy}
                >
                  {busy ? tr('common.loading', 'Loading...') : tr('pl.save', 'Save pipeline')}
                </button>
              </div>
            </div>
          </div>

          {showPreview && (
            <div className="pl-preview">
              <div className="retro-field-label" style={{ marginBottom: 8 }}>
                {tr('pl.preview', 'Markdown preview')}
              </div>
              <pre className="pl-preview-code mono">{buildMarkdown(name, steps, skillName)}</pre>
            </div>
          )}
        </div>
      )}

      {projectId && !building && (
        pipelines.length === 0 ? (
          <div className="empty">
            <h2>{tr('pl.empty', 'No pipelines yet')}</h2>
            <p className="muted">{tr('pl.emptyHint', 'Build your first composed skill.')}</p>
            <button className="btn btn-accent" style={{ marginTop: 16 }} onClick={startNew}>
              {tr('pl.new', 'New pipeline')}
            </button>
          </div>
        ) : (
          <div className="grid">
            {pipelines.map((p) => (
              <div key={p.id} className="card clickable" onClick={() => startEdit(p)}>
                <strong style={{ color: 'var(--ink)', fontSize: '1rem' }}>{p.name}</strong>
                <div className="muted" style={{ fontSize: '0.82rem', marginTop: 6 }}>
                  {tr('pl.stepCount', '{{n}} steps', { n: p.steps.length })}
                </div>
                <ol className="pl-card-steps">
                  {p.steps.slice(0, 5).map((s, i) => (
                    <li key={i}>{skillName(s.skill_id) || s.skill_id || '—'}</li>
                  ))}
                  {p.steps.length > 5 && <li className="faint">…</li>}
                </ol>
              </div>
            ))}
          </div>
        )
      )}
    </div>
  );
}
