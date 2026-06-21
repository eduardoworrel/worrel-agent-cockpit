import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { listSuggestions, acceptSuggestion, rejectSuggestion } from '../api';
import type { Suggestion, Project } from '../api';
import SuggestionBody from '../components/SuggestionBody';

interface Props {
  // Projeto ativo (terminal/projeto aberto). Vazio = sem escopo de projeto.
  activeProjectId: string | null;
  // Projetos, para nomear os atalhos de "outros projetos".
  projects: Project[];
  // Sinal para recarregar (ex.: evento suggestion.created).
  reloadKey: number;
}

// SuggestionsDrawer fica sempre visível (recolhível). Mostra as sugestões do
// projeto ativo; as dos demais projetos viram atalhos navegáveis (não há
// sugestões "globais" ainda). Aceitar/descartar inline.
export default function SuggestionsDrawer({ activeProjectId, projects, reloadKey }: Props) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [collapsed, setCollapsed] = useState(false);
  const [all, setAll] = useState<Suggestion[]>([]);
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    listSuggestions(undefined, 'pending').then(setAll).catch(() => setAll([]));
  }, []);
  useEffect(() => { load(); }, [load, reloadKey]);

  const nameOf = (pid: string) => projects.find((p) => p.id === pid)?.name ?? pid.slice(0, 8);
  const scoped = activeProjectId ? all.filter((s) => s.project_id === activeProjectId) : [];
  // Demais sugestões agrupadas por projeto → atalhos navegáveis.
  const otherGroups = Object.entries(
    all
      .filter((s) => s.project_id && s.project_id !== activeProjectId)
      .reduce<Record<string, number>>((acc, s) => {
        acc[s.project_id] = (acc[s.project_id] ?? 0) + 1;
        return acc;
      }, {}),
  );

  async function act(id: string, fn: (id: string) => Promise<unknown>) {
    if (busy) return;
    setBusy(true);
    try {
      await fn(id);
      setAll((prev) => prev.filter((s) => s.id !== id));
    } finally {
      setBusy(false);
    }
  }

  function getMemoryEntryRelatedId(sg: Suggestion): string {
    if (sg.type !== 'add_memory_entry') return '';
    try {
      const p = JSON.parse(sg.payload);
      return typeof p?.related_entry_id === 'string' ? p.related_entry_id : '';
    } catch {
      return '';
    }
  }

  if (collapsed) {
    return (
      <aside className="drawer drawer-collapsed">
        <button className="drawer-toggle" aria-label={t('drawer.expand')} onClick={() => setCollapsed(false)}>
          ‹ {all.length > 0 && <span className="badge">{all.length}</span>}
        </button>
      </aside>
    );
  }

  function renderItem(sg: Suggestion) {
    const relatedId = getMemoryEntryRelatedId(sg);
    return (
      <div key={sg.id} className="card drawer-card">
        <strong>{sg.title}</strong>
        <SuggestionBody sg={sg} />
        <div className="drawer-card-actions">
          {sg.type === 'add_memory_entry' && relatedId ? (
            <>
              <button className="btn btn-primary btn-sm" disabled={busy} onClick={() => act(sg.id, (id) => acceptSuggestion(id))}>
                {t('suggestions.coexist', 'Coexistir')}
              </button>
              <button className="btn btn-secondary btn-sm" disabled={busy} onClick={() => act(sg.id, (id) => acceptSuggestion(id, undefined, relatedId))}>
                {t('suggestions.replaceEntry', 'Substituir entrada')}
              </button>
            </>
          ) : (
            <button className="btn btn-primary btn-sm" disabled={busy} onClick={() => act(sg.id, acceptSuggestion)}>
              {t('suggestions.accept')}
            </button>
          )}
          <button className="btn btn-secondary btn-sm" disabled={busy} onClick={() => act(sg.id, rejectSuggestion)}>
            {t('suggestions.reject')}
          </button>
        </div>
      </div>
    );
  }

  return (
    <aside className="drawer">
      <div className="drawer-head">
        <span>{t('nav.suggestions')}</span>
        {all.length > 0 && <span className="badge">{all.length}</span>}
        <button className="drawer-toggle" aria-label={t('drawer.collapse')} onClick={() => setCollapsed(true)}>›</button>
      </div>

      <div className="drawer-body">
        {scoped.length > 0
          ? scoped.map(renderItem)
          : otherGroups.length === 0 && <p className="muted drawer-empty">{t('drawer.none')}</p>}

        {otherGroups.length > 0 && (
          <>
            <div className="drawer-section-label">{t('drawer.otherProjects')}</div>
            {otherGroups.map(([pid, n]) => (
              <button key={pid} className="drawer-shortcut" onClick={() => navigate(`/projects/${pid}`)}>
                <span>{nameOf(pid)}</span>
                <span className="badge">{n}</span>
              </button>
            ))}
          </>
        )}
      </div>
    </aside>
  );
}
