import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { listSuggestions, listDeferred, acceptSuggestion, rejectSuggestion } from '../api';
import type { Suggestion, Project, DeferredSession } from '../api';
import { useEvents } from '../useEvents';
import SuggestionBody from '../components/SuggestionBody';

interface Props {
  // Projetos, para rotular cada sugestão/bolinha com o nome do projeto.
  projects: Project[];
  // Sinal para recarregar (ex.: evento suggestion.created).
  reloadKey: number;
  // Reabre o modal de interação de uma sessão adiada (clique na bolinha).
  onOpen: (sessionId: string) => void;
}

// SuggestionsDrawer é o sidebar direito, estreito por padrão. Em repouso mostra
// as BOLINHAS das sessões adiadas (5 mais recentes, readonly — clicar reabre o
// modal). O ícone no topo expande a LISTA de todas as sugestões pendentes (sem
// filtro por projeto; cada item rotulado com o nome do projeto).
export default function SuggestionsDrawer({ projects, reloadKey, onOpen }: Props) {
  const { t } = useTranslation();
  const [showList, setShowList] = useState(false);
  const [all, setAll] = useState<Suggestion[]>([]);
  const [deferred, setDeferred] = useState<DeferredSession[]>([]);
  const [busy, setBusy] = useState(false);

  const loadSuggestions = useCallback(() => {
    listSuggestions(undefined, 'pending').then(setAll).catch(() => setAll([]));
  }, []);
  const loadDeferred = useCallback(() => {
    listDeferred().then(setDeferred).catch(() => setDeferred([]));
  }, []);

  useEffect(() => { loadSuggestions(); }, [loadSuggestions, reloadKey]);
  useEffect(() => { loadDeferred(); }, [loadDeferred]);

  // A fila de adiadas muda fora daqui (adiar no modal / responder limpa a marca).
  useEvents(useCallback((ev) => {
    if (ev.type === 'session.deferred' || ev.type === 'session.undeferred' || ev.type === 'session.ended') {
      loadDeferred();
    }
  }, [loadDeferred]));

  const nameOf = (pid: string) => projects.find((p) => p.id === pid)?.name ?? pid.slice(0, 8);

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

  function renderItem(sg: Suggestion) {
    const relatedId = getMemoryEntryRelatedId(sg);
    return (
      <div key={sg.id} className="card drawer-card">
        {sg.project_id && <div className="drawer-card-project">{nameOf(sg.project_id)}</div>}
        <strong>{sg.title}</strong>
        <SuggestionBody sg={sg} />
        <div className="drawer-card-actions">
          {sg.type === 'skill_or_agente_candidate' ? (
            <>
              <button className="btn btn-primary btn-sm" disabled={busy} onClick={() => act(sg.id, (id) => acceptSuggestion(id, undefined, undefined, 'skill'))}>
                {t('suggestions.createAsSkill', 'Criar como Skill')}
              </button>
              <button className="btn btn-secondary btn-sm" disabled={busy} onClick={() => act(sg.id, (id) => acceptSuggestion(id, undefined, undefined, 'agente'))}>
                {t('suggestions.createAsAgente', 'Criar como Agente')}
              </button>
            </>
          ) : sg.type === 'add_memory_entry' && relatedId ? (
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

  // Iniciais da sessão p/ a bolinha (rótulo da sessão adiada).
  const initials = (label: string) => (label.trim().slice(0, 2) || '·').toUpperCase();

  return (
    <aside className={`drawer${showList ? ' drawer-expanded' : ''}`}>
      <div className="drawer-head">
        <button
          className={`drawer-toggle${showList ? ' on' : ''}`}
          aria-label={showList ? t('drawer.collapse') : t('drawer.expand')}
          title={showList ? t('drawer.collapse') : t('drawer.expand')}
          onClick={() => setShowList((v) => !v)}
        >
          ✦
          {all.length > 0 && <span className="badge">{all.length}</span>}
        </button>
        {showList && <span className="drawer-title">{t('nav.suggestions')}</span>}
      </div>

      {showList ? (
        <div className="drawer-body">
          {all.length > 0
            ? all.map(renderItem)
            : <p className="muted drawer-empty">{t('drawer.none')}</p>}
        </div>
      ) : (
        <div className="drawer-deferred" aria-label={t('drawer.deferred', 'Adiadas')}>
          {deferred.slice(0, 5).map((d) => (
            <button
              key={d.session_id}
              className="deferred-dot"
              title={d.label || d.session_id}
              onClick={() => onOpen(d.session_id)}
            >
              {initials(d.label || d.session_id)}
            </button>
          ))}
        </div>
      )}
    </aside>
  );
}
