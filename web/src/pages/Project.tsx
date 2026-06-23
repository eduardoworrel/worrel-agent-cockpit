import { useEffect, useState } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  getProject,
  archiveProject,
  getMemory,
  saveMemory,
  listMemoryVersions,
  revertMemory,
  listMemoryEntries,
  deleteMemoryEntry,
  listAgents,
  listSkills,
  createSkill,
  updateSkill,
  listSuggestions,
  acceptSuggestion,
  rejectSuggestion,
  deferSuggestion,
  listAdapters,
  getSkillStats,
  setSkillPolicy,
  setProjectSkillsPolicy,
  exportSkill,
  importSkill,
} from '../api';
import type { Project as ProjectType, MemoryVersion, MemoryEntry, Agent, Skill, Suggestion, DetectedAdapter, SkillStats } from '../api';
import SecretsTab from '../components/SecretsTab';
import SkillHealth from '../components/SkillHealth';
import Lineage from '../components/Lineage';
import NewSessionModal from '../components/NewSessionModal';
import SuggestionBody from '../components/SuggestionBody';

const SKILL_POLICIES = ['manual', 'auto_correction', 'auto_total'];

// sugTypeLabel mapeia o tipo de sugestão (ex.: "skill.correction") para um rótulo
// traduzido, espelhando a lógica usada em Suggestions.tsx.
function sugTypeLabel(t: (key: string) => string, type: string): string {
  const key = type.replace(/\./g, '_');
  const label = t(`suggestions.suggestionType.${key}`);
  return label.includes('suggestions.suggestionType') ? type : label;
}

type Tab = 'memory' | 'skills' | 'sessions' | 'suggestions' | 'secrets';

export default function Project() {
  const { id } = useParams<{ id: string }>();
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [project, setProject] = useState<ProjectType | null>(null);
  const [tab, setTab] = useState<Tab>('memory');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [busy, setBusy] = useState(false);

  // Memory
  const [memContent, setMemContent] = useState('');
  const [memNote, setMemNote] = useState('');
  const [versions, setVersions] = useState<MemoryVersion[]>([]);
  const [memoryEntries, setMemoryEntries] = useState<MemoryEntry[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);

  // Skills
  const [skills, setSkills] = useState<Skill[]>([]);
  const [newSkillName, setNewSkillName] = useState('');
  const [newSkillContent, setNewSkillContent] = useState('');
  const [editingSkill, setEditingSkill] = useState<string | null>(null);
  const [editSkillName, setEditSkillName] = useState('');
  const [editSkillContent, setEditSkillContent] = useState('');
  const [skillStats, setSkillStats] = useState<Record<string, SkillStats>>({});
  const [lineageOpen, setLineageOpen] = useState<string | null>(null);
  const [bulkPolicy, setBulkPolicy] = useState('manual');

  // Sessions
  const [adapters, setAdapters] = useState<DetectedAdapter[]>([]);
  const [showNewSession, setShowNewSession] = useState(false);
  // Quando preenchido, o NewSessionModal injeta este conteúdo (skill/pipeline) no primer.
  const [sessionSkill, setSessionSkill] = useState<{ content: string; label: string } | null>(null);

  // Suggestions
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [editingSug, setEditingSug] = useState<string | null>(null);
  const [editSugTitle, setEditSugTitle] = useState('');
  const [editSugPayload, setEditSugPayload] = useState('');

  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    async function loadAll(projectId: string) {
      setLoading(true);
      try {
        const [proj, mem, vers, entries, ags, sk, sugs, adps] = await Promise.all([
          getProject(projectId),
          getMemory(projectId).catch(() => null),
          listMemoryVersions(projectId).catch(() => [] as MemoryVersion[]),
          listMemoryEntries(projectId).catch(() => [] as MemoryEntry[]),
          listAgents(projectId).catch(() => [] as Agent[]),
          listSkills(projectId),
          listSuggestions(projectId, 'pending'),
          listAdapters().catch(() => [] as DetectedAdapter[]),
        ]);
        if (cancelled) return;
        setProject(proj);
        if (mem) { setMemContent(mem.content); }
        setVersions(vers);
        setMemoryEntries(entries);
        setAgents(ags);
        setSkills(sk);
        void loadSkillStats(sk);
        setSuggestions(sugs);
        const present = adps.filter((a) => a.installed.present);
        setAdapters(present);
      } catch {
        if (!cancelled) setError(true);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    loadAll(id);
    return () => { cancelled = true; };
  }, [id]);

  async function run(action: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setError(false);
    try {
      await action();
    } catch {
      setError(true);
    } finally {
      setBusy(false);
    }
  }

  function handleArchive() {
    if (!id) return;
    if (!window.confirm(t('project.archiveConfirm'))) return;
    return run(async () => {
      await archiveProject(id);
      navigate('/');
    });
  }

  function handleSaveMemory() {
    if (!id) return;
    return run(async () => {
      const m = await saveMemory(id, memContent, memNote);
      setMemContent(m.content);
      setMemNote('');
      setVersions(await listMemoryVersions(id));
    });
  }

  function handleRevert(versionId: number) {
    if (!id) return;
    return run(async () => {
      const m = await revertMemory(id, versionId);
      setMemContent(m.content);
      setVersions(await listMemoryVersions(id));
    });
  }

  function handleDeleteMemoryEntry(entryId: string) {
    if (!id) return;
    return run(async () => {
      await deleteMemoryEntry(id, entryId);
      setMemoryEntries(await listMemoryEntries(id));
    });
  }

  function handleCreateSkill() {
    if (!id || !newSkillName) return;
    return run(async () => {
      await createSkill(id, newSkillName, newSkillContent);
      setNewSkillName('');
      setNewSkillContent('');
      setSkills(await listSkills(id));
    });
  }

  function handleUpdateSkill(skillId: string) {
    return run(async () => {
      await updateSkill(skillId, editSkillName, editSkillContent);
      setEditingSkill(null);
      if (id) setSkills(await listSkills(id));
    });
  }

  async function loadSkillStats(list: Skill[]) {
    const entries = await Promise.all(
      list.map(async (sk) => {
        try {
          return [sk.id, await getSkillStats(sk.id)] as const;
        } catch {
          return null;
        }
      })
    );
    const map: Record<string, SkillStats> = {};
    for (const e of entries) if (e) map[e[0]] = e[1];
    setSkillStats(map);
  }

  // Critério 6/7: alterar a política de evolução de uma skill (opt-in auto).
  function handleSetPolicy(skillId: string, policy: string) {
    return run(async () => {
      await setSkillPolicy(skillId, policy);
      setSkills((prev) => prev.map((s) => (s.id === skillId ? { ...s, evolution_policy: policy } : s)));
    });
  }

  function handleBulkPolicy() {
    if (!id) return;
    return run(async () => {
      await setProjectSkillsPolicy(id, bulkPolicy);
      setSkills(await listSkills(id));
    });
  }

  // Critério 9: exportar SKILL.md (download) e importar de texto colado.
  async function handleExport(sk: Skill) {
    const md = await exportSkill(sk.id);
    const blob = new Blob([md], { type: 'text/markdown' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'SKILL.md';
    a.click();
    URL.revokeObjectURL(url);
  }

  function handleImport() {
    if (!id) return;
    const content = window.prompt(t('skills.importSkill') as string);
    if (!content) return;
    return run(async () => {
      await importSkill(id, content);
      setSkills(await listSkills(id));
    });
  }

  // Badge "Correção proativa sugerida": há sugestão skill.correction pendente da skill.
  function hasProactive(skillId: string): boolean {
    return suggestions.some((s) => s.skill_id === skillId && s.type === 'skill.correction' && s.status === 'pending');
  }

  // Após qualquer ação em sugestão, re-busca skills/memória/sugestões do projeto
  // para evitar estado obsoleto (ex.: aceitar uma correção gera nova geração de skill).
  async function refetchAfterSuggestion() {
    if (!id) return;
    const [sk, mem, vers, sugs] = await Promise.all([
      listSkills(id),
      getMemory(id).catch(() => null),
      listMemoryVersions(id).catch(() => [] as MemoryVersion[]),
      listSuggestions(id, 'pending'),
    ]);
    setSkills(sk);
    void loadSkillStats(sk);
    if (mem) setMemContent(mem.content);
    setVersions(vers);
    setSuggestions(sugs);
  }

  function handleAccept(sugId: string) {
    return run(async () => {
      await acceptSuggestion(sugId);
      await refetchAfterSuggestion();
    });
  }

  function handleEditAccept(sugId: string) {
    return run(async () => {
      await acceptSuggestion(sugId, { title: editSugTitle, payload: editSugPayload });
      setEditingSug(null);
      await refetchAfterSuggestion();
    });
  }

  function handleReject(sugId: string) {
    const sg = suggestions.find((s) => s.id === sugId);
    if (!window.confirm(t('suggestions.confirmReject', { title: sg?.title ?? sugId }))) return Promise.resolve();
    return run(async () => {
      await rejectSuggestion(sugId);
      await refetchAfterSuggestion();
    });
  }

  function handleDefer(sugId: string) {
    return run(async () => {
      await deferSuggestion(sugId);
      await refetchAfterSuggestion();
    });
  }

  // Abre o modal de sessão já com o conteúdo da skill para injeção no primer.
  function openSkillSession(content: string, label: string) {
    setSessionSkill({ content, label });
    setShowNewSession(true);
  }

  if (loading) return <div className="main"><p>{t('common.loading')}</p></div>;
  if (!project) return <div className="main"><p>{t('common.error')}</p></div>;

  return (
    <div className="main">
      <div className="page-head" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: '1rem', flexWrap: 'wrap' }}>
        <div>
          <Link to="/" className="muted" style={{ fontSize: '0.8rem', fontWeight: 600 }}>← {t('nav.dashboard')}</Link>
          <h1 style={{ marginTop: 6 }}>{project.name}</h1>
          {project.description && <p className="sub">{project.description}</p>}
        </div>
        <button className="btn-secondary" onClick={handleArchive} disabled={busy}>
          {t('project.archive')}
        </button>
      </div>

      {error && <p className="error-banner">{t('common.actionFailed')}</p>}

      {showNewSession && id && (
        <NewSessionModal
          projectId={id}
          skill={sessionSkill?.content}
          skillLabel={sessionSkill?.label}
          onCreated={(sess) => { setShowNewSession(false); setSessionSkill(null); navigate(`/sessions/${sess.id}`); }}
          onClose={() => { setShowNewSession(false); setSessionSkill(null); }}
        />
      )}

      <div className="tabs">
        {(['memory', 'skills', 'suggestions', 'secrets'] as Tab[]).map((tabName) => (
          <button
            key={tabName}
            className={`tab${tab === tabName ? ' active' : ''}`}
            onClick={() => setTab(tabName)}
          >
            {t(`project.tabs.${tabName}`)}
            {tabName === 'suggestions' && suggestions.length > 0 && (
              <span className="badge" style={{ marginLeft: '0.4rem' }}>{suggestions.length}</span>
            )}
          </button>
        ))}
      </div>

      {tab === 'memory' && (
        <div>
          {memoryEntries.length > 0 && (
            <div className="card" style={{ marginBottom: '1.5rem' }}>
              <h3 style={{ margin: '0 0 0.75rem' }}>{t('memory.entries')}</h3>
              {(['convencao', 'arquitetura', 'gotcha', 'never_do', 'decisao'] as const).map((cat) => {
                const items = memoryEntries.filter((e) => e.category === cat);
                if (items.length === 0) return null;
                const labels: Record<string, string> = { convencao: 'Convenções', arquitetura: 'Arquitetura', gotcha: 'Gotchas', never_do: 'Nunca faça', decisao: 'Decisões' };
                return (
                  <div key={cat} style={{ marginBottom: '0.75rem' }}>
                    <strong style={{ fontSize: '0.85rem', color: 'var(--muted)', textTransform: 'uppercase', letterSpacing: '0.04em' }}>{labels[cat]}</strong>
                    {items.map((e) => (
                      <div key={e.id} style={{ display: 'flex', alignItems: 'flex-start', gap: '0.5rem', marginTop: '0.35rem' }}>
                        <span style={{ flex: 1, fontSize: '0.9rem' }}>— {e.content}</span>
                        <button
                          className="btn btn-danger"
                          style={{ fontSize: '0.75rem', padding: '2px 8px', flexShrink: 0 }}
                          disabled={busy}
                          onClick={() => handleDeleteMemoryEntry(e.id)}
                        >✕</button>
                      </div>
                    ))}
                  </div>
                );
              })}
            </div>
          )}
          {agents.length > 0 && (
            <div className="card" style={{ marginBottom: '1.5rem' }}>
              <h3 style={{ margin: '0 0 0.75rem' }}>{t('agents.title', 'Agentes')}</h3>
              {agents.map((a) => (
                <div key={a.id} style={{ marginBottom: '0.75rem' }}>
                  <strong style={{ fontSize: '0.9rem' }}>{a.name}</strong>
                  <div style={{ fontSize: '0.85rem', color: 'var(--muted)', whiteSpace: 'pre-wrap', marginTop: '0.25rem' }}>{a.persona}</div>
                </div>
              ))}
            </div>
          )}
          <label htmlFor="mem-content">{t('memory.content')}</label>
          <textarea
            id="mem-content"
            value={memContent}
            onChange={(e) => setMemContent(e.target.value)}
            placeholder={t('memory.contentPlaceholder')}
            rows={12}
          />
          <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.5rem', alignItems: 'flex-end' }}>
            <div style={{ flex: 1 }}>
              <label htmlFor="mem-note">{t('memory.note')}</label>
              <input
                id="mem-note"
                value={memNote}
                onChange={(e) => setMemNote(e.target.value)}
                placeholder={t('memory.notePlaceholder')}
              />
            </div>
            <button className="btn btn-primary" disabled={busy} onClick={handleSaveMemory}>
              {t('memory.save')}
            </button>
          </div>

          <h3 style={{ marginTop: '1.5rem' }}>{t('memory.versions')}</h3>
          {versions.length === 0 ? (
            <p style={{ color: 'var(--muted)' }}>{t('memory.noVersions')}</p>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>#</th>
                  <th>{t('memory.note')}</th>
                  <th>{t('memory.date')}</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {versions.map((v) => (
                  <tr key={v.id}>
                    <td>{v.id}</td>
                    <td>{v.note}</td>
                    <td>{new Date(v.created_at).toLocaleString()}</td>
                    <td>
                      <button className="btn btn-secondary" disabled={busy} onClick={() => handleRevert(v.id)}>
                        {t('memory.revert')}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}

      {tab === 'skills' && (
        <div>
          <div className="card" style={{ marginBottom: '1rem' }}>
            <h3 style={{ margin: '0 0 0.75rem' }}>{t('skills.create')}</h3>
            <input
              value={newSkillName}
              onChange={(e) => setNewSkillName(e.target.value)}
              placeholder={t('skills.name')}
            />
            <textarea
              value={newSkillContent}
              onChange={(e) => setNewSkillContent(e.target.value)}
              placeholder={t('skills.content')}
              rows={4}
              style={{ marginTop: '0.5rem' }}
            />
            <button className="btn btn-primary" style={{ marginTop: '0.5rem' }} disabled={busy} onClick={handleCreateSkill}>
              {t('common.create')}
            </button>
            <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center', marginTop: '0.75rem', flexWrap: 'wrap' }}>
              <span style={{ fontSize: '0.85rem' }}>{t('skills.bulkPolicy')}:</span>
              <select value={bulkPolicy} onChange={(e) => setBulkPolicy(e.target.value)}>
                {SKILL_POLICIES.map((p) => (
                  <option key={p} value={p}>{t(`skills.policy${p === 'manual' ? 'Manual' : p === 'auto_correction' ? 'AutoCorrection' : 'AutoTotal'}`)}</option>
                ))}
              </select>
              <button className="btn btn-secondary" disabled={busy} onClick={handleBulkPolicy}>{t('skills.save')}</button>
              <button className="btn btn-secondary" disabled={busy} onClick={handleImport}>{t('skills.importSkill')}</button>
            </div>
          </div>

          {skills.length === 0 ? (
            <p style={{ color: 'var(--muted)' }}>{t('skills.noSkills')}</p>
          ) : (
            skills.map((sk) => (
              <div key={sk.id} className="card" style={{ marginBottom: '0.75rem' }}>
                {editingSkill === sk.id ? (
                  <>
                    <input value={editSkillName} onChange={(e) => setEditSkillName(e.target.value)} />
                    <textarea
                      value={editSkillContent}
                      onChange={(e) => setEditSkillContent(e.target.value)}
                      rows={6}
                      style={{ marginTop: '0.5rem' }}
                    />
                    <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.5rem' }}>
                      <button className="btn btn-primary" disabled={busy} onClick={() => handleUpdateSkill(sk.id)}>{t('skills.save')}</button>
                      <button className="btn btn-secondary" onClick={() => setEditingSkill(null)}>{t('skills.cancel')}</button>
                    </div>
                  </>
                ) : (
                  <>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '0.5rem' }}>
                      <span style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', flexWrap: 'wrap' }}>
                        <strong>{sk.name}</strong>
                        {sk.evolution_policy !== 'manual' && (
                          <span className="pill" title={t('skills.autoMode') as string}>
                            {sk.evolution_policy === 'auto_total' ? t('skills.policyAutoTotal') : t('skills.policyAutoCorrection')}
                          </span>
                        )}
                        {hasProactive(sk.id) && (
                          <span className="pill pink">
                            {t('skills.proactiveBadge')}
                          </span>
                        )}
                      </span>
                      <div style={{ display: 'flex', gap: '0.5rem' }}>
                        {adapters.length > 0 && (
                          <button className="btn btn-primary" style={{ fontSize: '0.8rem' }} disabled={busy}
                            onClick={() => openSkillSession(sk.content, sk.name)}>
                            ▶ {t('sessions.startFromSkill')}
                          </button>
                        )}
                        <button className="btn btn-secondary" onClick={() => {
                          setEditingSkill(sk.id);
                          setEditSkillName(sk.name);
                          setEditSkillContent(sk.content);
                        }}>{t('skills.edit')}</button>
                      </div>
                    </div>
                    <div style={{ display: 'flex', gap: '0.75rem', alignItems: 'center', marginTop: '0.4rem', flexWrap: 'wrap' }}>
                      <SkillHealth stats={skillStats[sk.id]} />
                      <label style={{ display: 'flex', alignItems: 'center', gap: '0.25rem', fontSize: '0.8rem' }}>
                        {t('skills.policy')}:
                        <select value={sk.evolution_policy} disabled={busy} onChange={(e) => handleSetPolicy(sk.id, e.target.value)}>
                          {SKILL_POLICIES.map((p) => (
                            <option key={p} value={p}>{t(`skills.policy${p === 'manual' ? 'Manual' : p === 'auto_correction' ? 'AutoCorrection' : 'AutoTotal'}`)}</option>
                          ))}
                        </select>
                      </label>
                      <button className="btn btn-secondary" style={{ fontSize: '0.78rem' }}
                        onClick={() => setLineageOpen(lineageOpen === sk.id ? null : sk.id)}>
                        {t('skills.lineage')}
                      </button>
                      <button className="btn btn-secondary" style={{ fontSize: '0.78rem' }} onClick={() => handleExport(sk)}>
                        {t('skills.exportSkill')}
                      </button>
                    </div>
                    {sk.evolution_policy !== 'manual' && (
                      <p style={{ fontSize: '0.75rem', color: 'var(--muted)', marginTop: '0.4rem' }}>{t('skills.autoExplain')}</p>
                    )}
                    <pre className="mono" style={{ marginTop: '0.5rem', fontSize: '0.78rem', color: 'var(--ink-soft)', background: 'var(--surface-warm)', border: '1px solid var(--line)', borderRadius: 'var(--r-sm)', padding: '10px 12px', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                      {sk.content.slice(0, 200)}{sk.content.length > 200 ? '…' : ''}
                    </pre>
                    {lineageOpen === sk.id && (
                      <Lineage skillId={sk.id} activeGeneration={sk.active_generation} onReverted={() => { if (id) listSkills(id).then(setSkills); }} />
                    )}
                  </>
                )}
              </div>
            ))
          )}
        </div>
      )}

      {tab === 'suggestions' && (
        <div>
          {suggestions.length === 0 ? (
            <p style={{ color: 'var(--muted)' }}>{t('suggestions.noSuggestions')}</p>
          ) : (
            suggestions.map((sg) => (
              <div key={sg.id} className="card" style={{ marginBottom: '0.75rem' }}>
                <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '0.5rem', flexWrap: 'wrap', alignItems: 'center' }}>
                  <span className="pill" data-type={sg.type}>{sugTypeLabel(t, sg.type)}</span>
                  <strong style={{ color: 'var(--ink)' }}>{sg.title}</strong>
                </div>
                {sg.evidence && <p style={{ color: 'var(--muted)', fontSize: '0.85rem', margin: '0 0 0.5rem' }}>{sg.evidence}</p>}
                <SuggestionBody sg={sg} skills={skills} />

                {editingSug === sg.id ? (
                  <div style={{ marginTop: '0.5rem' }}>
                    <label htmlFor={`sug-title-${sg.id}`}>{t('suggestions.editTitle')}</label>
                    <input id={`sug-title-${sg.id}`} value={editSugTitle} onChange={(e) => setEditSugTitle(e.target.value)} />
                    <label htmlFor={`sug-payload-${sg.id}`} style={{ marginTop: '0.5rem' }}>{t('suggestions.editPayload')}</label>
                    <textarea
                      id={`sug-payload-${sg.id}`}
                      value={editSugPayload}
                      onChange={(e) => setEditSugPayload(e.target.value)}
                      rows={4}
                    />
                    <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.5rem' }}>
                      <button className="btn btn-primary" disabled={busy} onClick={() => handleEditAccept(sg.id)}>{t('suggestions.accept')}</button>
                      <button className="btn btn-secondary" onClick={() => setEditingSug(null)}>{t('common.cancel')}</button>
                    </div>
                  </div>
                ) : (
                  <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.5rem', flexWrap: 'wrap' }}>
                    <button className="btn btn-primary" disabled={busy} onClick={() => handleAccept(sg.id)}>{t('suggestions.accept')}</button>
                    <button className="btn btn-secondary" disabled={busy} onClick={() => {
                      setEditingSug(sg.id);
                      setEditSugTitle(sg.title);
                      setEditSugPayload(sg.payload);
                    }}>{t('suggestions.editAccept')}</button>
                    <button className="btn btn-danger" disabled={busy} onClick={() => handleReject(sg.id)}>{t('suggestions.reject')}</button>
                    <button className="btn btn-secondary" disabled={busy} onClick={() => handleDefer(sg.id)}>{t('suggestions.defer')}</button>
                  </div>
                )}
              </div>
            ))
          )}
        </div>
      )}

      {tab === 'secrets' && id && (
        <SecretsTab projectId={id} />
      )}
    </div>
  );
}
