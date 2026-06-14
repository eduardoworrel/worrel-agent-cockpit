import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  listSuggestions,
  listProjects,
  acceptSuggestion,
  rejectSuggestion,
  deferSuggestion,
  reclassifySuggestion,
  revertGeneration,
} from '../api';
import type { Suggestion, Project } from '../api';
import { FanHero } from '../components/Fan';
import SuggestionBody from '../components/SuggestionBody';

const SKILL_TYPES = ['skill.learned', 'skill.correction', 'skill.variant'];
// Tipos com edição estruturada por campo (qualquer outro cai no fallback JSON cru).
const FIELD_EDIT_TYPES = ['skill.learned', 'skill.variant', 'add_memory', 'create_project'];

// sugTypeLabel maps a suggestion type (e.g. "skill.learned") to a translated label.
// The dot-separated types are mapped to underscore keys for i18n lookup.
function sugTypeLabel(t: (key: string) => string, type: string): string {
  const key = type.replace(/\./g, '_');
  const label = t(`suggestions.suggestionType.${key}`);
  // If the key was not found, i18next returns the key itself — fall back to raw type.
  return label.includes('suggestions.suggestionType') ? type : label;
}

export default function Suggestions() {
  const { t } = useTranslation();
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [busy, setBusy] = useState(false);
  const [editingSug, setEditingSug] = useState<string | null>(null);
  const [editingType, setEditingType] = useState<string>('');
  const [editSugTitle, setEditSugTitle] = useState('');
  const [editSugPayload, setEditSugPayload] = useState(''); // fallback (JSON cru)
  const [editParsed, setEditParsed] = useState<Record<string, unknown>>({});
  const [editName, setEditName] = useState('');
  const [editContent, setEditContent] = useState('');
  const [editDescription, setEditDescription] = useState('');
  const [tab, setTab] = useState<'pending' | 'auto_applied'>('pending');

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      try {
        const [sugs, projs] = await Promise.all([
          listSuggestions(undefined, tab),
          listProjects(),
        ]);
        if (cancelled) return;
        setSuggestions(sugs);
        setProjects(projs);
      } catch {
        if (!cancelled) setError(true);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    load();
    return () => { cancelled = true; };
  }, [tab]);

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

  function handleAccept(sugId: string) {
    return run(async () => {
      await acceptSuggestion(sugId);
      setSuggestions((prev) => prev.filter((s) => s.id !== sugId));
    });
  }

  function beginEdit(sg: Suggestion) {
    setEditingSug(sg.id);
    setEditingType(sg.type);
    setEditSugTitle(sg.title);
    setEditSugPayload(sg.payload);
    let parsed: Record<string, unknown> = {};
    try {
      const p = JSON.parse(sg.payload || '{}');
      if (p && typeof p === 'object') parsed = p as Record<string, unknown>;
    } catch {
      parsed = {};
    }
    setEditParsed(parsed);
    setEditName(typeof parsed.name === 'string' ? parsed.name : '');
    setEditContent(
      typeof parsed.content === 'string' && parsed.content
        ? parsed.content
        : typeof parsed.description === 'string'
          ? parsed.description
          : '',
    );
    setEditDescription(typeof parsed.description === 'string' ? parsed.description : '');
  }

  function handleEditAccept(sugId: string) {
    return run(async () => {
      let payload = editSugPayload;
      if (FIELD_EDIT_TYPES.includes(editingType)) {
        const merged = { ...editParsed };
        if (editingType === 'skill.learned' || editingType === 'skill.variant') {
          merged.name = editName;
          merged.content = editContent;
        } else if (editingType === 'add_memory') {
          merged.content = editContent;
        } else if (editingType === 'create_project') {
          merged.description = editDescription;
        }
        payload = JSON.stringify(merged);
      }
      await acceptSuggestion(sugId, { title: editSugTitle, payload });
      setEditingSug(null);
      setSuggestions((prev) => prev.filter((s) => s.id !== sugId));
    });
  }

  function handleReject(sugId: string) {
    const sg = suggestions.find((s) => s.id === sugId);
    if (!window.confirm(t('suggestions.confirmReject', { title: sg?.title ?? sugId }))) return Promise.resolve();
    return run(async () => {
      await rejectSuggestion(sugId);
      setSuggestions((prev) => prev.filter((s) => s.id !== sugId));
    });
  }

  function handleDefer(sugId: string) {
    return run(async () => {
      await deferSuggestion(sugId);
      setSuggestions((prev) => prev.filter((s) => s.id !== sugId));
    });
  }

  // Critério 1: reclassificar o tipo da sugestão antes de aprovar.
  function handleReclassify(sugId: string, newType: string) {
    return run(async () => {
      await reclassifySuggestion(sugId, newType);
      setSuggestions((prev) => prev.map((s) => (s.id === sugId ? { ...s, type: newType } : s)));
    });
  }

  // Critério 6: reverter uma aplicação automática em um clique (reativa geração anterior).
  function handleRevertAuto(sg: Suggestion) {
    if (!sg.skill_id) return Promise.resolve();
    return run(async () => {
      // A geração anterior é a ativa - 1; em geral 1 quando há 2 gerações.
      await revertGeneration(sg.skill_id as string, 1);
      setSuggestions((prev) => prev.filter((s) => s.id !== sg.id));
    });
  }

  if (loading) return <div className="main"><p>{t('common.loading')}</p></div>;

  // Group by project_id then by type
  const projectMap = Object.fromEntries(projects.map((p) => [p.id, p]));
  const grouped: Record<string, Record<string, Suggestion[]>> = {};
  suggestions.forEach((sg) => {
    if (!grouped[sg.project_id]) grouped[sg.project_id] = {};
    if (!grouped[sg.project_id][sg.type]) grouped[sg.project_id][sg.type] = [];
    grouped[sg.project_id][sg.type].push(sg);
  });

  return (
    <div className="main">
      <div className="page-head">
        <div>
          <h1>{t('nav.suggestions')}</h1>
          <p className="sub">{t('suggestions.subtitle')}</p>
        </div>
      </div>
      <div className="tabs">
        <button
          className={tab === 'pending' ? 'tab active' : 'tab'}
          onClick={() => setTab('pending')}
        >{t('suggestions.pendingTab')}</button>
        <button
          className={tab === 'auto_applied' ? 'tab active' : 'tab'}
          onClick={() => setTab('auto_applied')}
        >{t('suggestions.autoAppliedTab')}</button>
      </div>
      {error && <p className="error-banner">{t('common.actionFailed')}</p>}
      {suggestions.length === 0 ? (
        <div className="empty">
          <FanHero width={120} height={62} />
          <h2>{t('suggestions.noSuggestions')}</h2>
        </div>
      ) : (
        Object.entries(grouped).map(([projectId, byType]) => (
          <div key={projectId} style={{ marginBottom: 32 }}>
            <h2 style={{ fontSize: '1.05rem', marginBottom: 12 }}>
              {projectId ? (projectMap[projectId]?.name ?? projectId) : t('suggestions.noProjectGroup')}
            </h2>
            {Object.entries(byType).map(([type, items]) => (
              <div key={type} style={{ marginBottom: 16 }}>
                <div style={{ marginBottom: 8 }}>
                  <span className="pill" data-type={type}>{sugTypeLabel(t, type)}</span>
                </div>
                {items.map((sg) => (
                  <div key={sg.id} className="card" style={{ marginBottom: 12 }}>
                    <strong style={{ color: 'var(--ink)' }}>{sg.title}</strong>
                    {sg.evidence && (
                      <p className="muted" style={{ fontSize: '0.85rem', margin: '6px 0' }}>
                        {sg.evidence}
                      </p>
                    )}
                    <SuggestionBody sg={sg} />

                    {editingSug === sg.id ? (
                      <div style={{ marginTop: '0.5rem' }}>
                        <label htmlFor={`gsug-title-${sg.id}`}>{t('suggestions.editTitle')}</label>
                        <input id={`gsug-title-${sg.id}`} value={editSugTitle} onChange={(e) => setEditSugTitle(e.target.value)} />

                        {(editingType === 'skill.learned' || editingType === 'skill.variant') && (
                          <>
                            <label htmlFor={`gsug-name-${sg.id}`} style={{ marginTop: '0.5rem' }}>{t('suggestions.fieldName')}</label>
                            <input id={`gsug-name-${sg.id}`} value={editName} onChange={(e) => setEditName(e.target.value)} />
                            <label htmlFor={`gsug-content-${sg.id}`} style={{ marginTop: '0.5rem' }}>{t('suggestions.fieldContent')}</label>
                            <textarea id={`gsug-content-${sg.id}`} value={editContent} onChange={(e) => setEditContent(e.target.value)} rows={8} />
                          </>
                        )}

                        {editingType === 'add_memory' && (
                          <>
                            <label htmlFor={`gsug-content-${sg.id}`} style={{ marginTop: '0.5rem' }}>{t('suggestions.fieldContent')}</label>
                            <textarea id={`gsug-content-${sg.id}`} value={editContent} onChange={(e) => setEditContent(e.target.value)} rows={6} />
                          </>
                        )}

                        {editingType === 'create_project' && (
                          <>
                            <label htmlFor={`gsug-desc-${sg.id}`} style={{ marginTop: '0.5rem' }}>{t('suggestions.fieldDescription')}</label>
                            <textarea id={`gsug-desc-${sg.id}`} value={editDescription} onChange={(e) => setEditDescription(e.target.value)} rows={4} />
                          </>
                        )}

                        {!FIELD_EDIT_TYPES.includes(editingType) && (
                          <>
                            <label htmlFor={`gsug-payload-${sg.id}`} style={{ marginTop: '0.5rem' }}>{t('suggestions.editPayload')}</label>
                            <textarea
                              id={`gsug-payload-${sg.id}`}
                              value={editSugPayload}
                              onChange={(e) => setEditSugPayload(e.target.value)}
                              rows={4}
                            />
                          </>
                        )}
                        <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.5rem' }}>
                          <button className="btn btn-primary" disabled={busy} onClick={() => handleEditAccept(sg.id)}>{t('suggestions.accept')}</button>
                          <button className="btn btn-secondary" onClick={() => setEditingSug(null)}>{t('common.cancel')}</button>
                        </div>
                      </div>
                    ) : tab === 'auto_applied' ? (
                      <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.75rem', flexWrap: 'wrap', alignItems: 'center' }}>
                        <span className="pill">{t('suggestions.autoAppliedBadge')}</span>
                        {sg.skill_id && (
                          <button className="btn btn-secondary" disabled={busy} onClick={() => handleRevertAuto(sg)}>{t('suggestions.revert')}</button>
                        )}
                      </div>
                    ) : (
                      <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.75rem', flexWrap: 'wrap', alignItems: 'center' }}>
                        {SKILL_TYPES.includes(sg.type) && (
                          <label style={{ display: 'flex', alignItems: 'center', gap: '0.25rem', fontSize: '0.85rem' }}>
                            {t('suggestions.reclassify')}
                            <select
                              value={sg.type}
                              disabled={busy}
                              onChange={(e) => handleReclassify(sg.id, e.target.value)}
                            >
                              {SKILL_TYPES.map((tp) => (
                                <option key={tp} value={tp}>{sugTypeLabel(t, tp)}</option>
                              ))}
                            </select>
                          </label>
                        )}
                        <button className="btn btn-primary" disabled={busy} onClick={() => handleAccept(sg.id)}>{t('suggestions.accept')}</button>
                        {sg.type !== 'secret.detected' && (
                          <button className="btn btn-secondary" disabled={busy} onClick={() => beginEdit(sg)}>{t('suggestions.editAccept')}</button>
                        )}
                        <button className="btn btn-danger" disabled={busy} onClick={() => handleReject(sg.id)}>{t('suggestions.reject')}</button>
                        <button className="btn btn-secondary" disabled={busy} onClick={() => handleDefer(sg.id)}>{t('suggestions.defer')}</button>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            ))}
          </div>
        ))
      )}
    </div>
  );
}
