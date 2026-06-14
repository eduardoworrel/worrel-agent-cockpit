import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { listProjects, type Project } from '../api';
import {
  createThread,
  listThreads,
  getThread,
  sendMessage,
  listAdapterModels,
  type ChatThread,
  type ChatMessage,
  type ChatSource,
  type CreatedSuggestion,
} from '../chatApi';

const WINDOW_PRESETS = [0, 30, 60, 90]; // 0 = tudo
const PROVIDERS = ['', 'claude-code', 'opencode', 'gemini', 'codex', 'pidev'];

function providerLabel(p: string, t: (k: string) => string): string {
  if (p === '') return t('chat.scope.providerDefault');
  if (p === 'pidev') return 'pi';
  return p;
}

// Bloco "Fontes" recolhível por mensagem do assistant.
function SourcesBlock({ sources }: { sources: ChatSource[] }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  if (!sources || sources.length === 0) return null;
  return (
    <div className="chat-sources">
      <button
        type="button"
        className="chat-sources-toggle"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
      >
        <span aria-hidden="true" className={`chat-caret ${open ? 'open' : ''}`}>▸</span>
        {t('chat.sources.title', { n: sources.length })}
      </button>
      {open && (
        <ul className="chat-sources-list">
          {sources.map((s, i) => (
            <li key={s.session_id || i} className="chat-source">
              <span className="chat-source-title">{s.title || s.session_id}</span>
              {s.adapter && <span className="pill faint chat-source-adapter">{s.adapter}</span>}
              {s.snippet && <p className="chat-source-snippet muted">{s.snippet}</p>}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function SuggestionChips({ items }: { items: CreatedSuggestion[] }) {
  const { t } = useTranslation();
  if (!items || items.length === 0) return null;
  return (
    <div className="chat-suggestions" role="list">
      {items.map((s) => (
        <Link
          key={s.id}
          to="/suggestions"
          role="listitem"
          className="chat-suggestion-chip"
          title={s.title}
        >
          <span aria-hidden="true">✓</span>
          <span className="chat-suggestion-kind">
            {s.type === 'pipeline'
              ? t('chat.suggestion.pipelineCreated')
              : t('chat.suggestion.created')}
          </span>
          <span className="chat-suggestion-title">{s.title}</span>
        </Link>
      ))}
    </div>
  );
}

function Bubble({ msg }: { msg: ChatMessage }) {
  const isUser = msg.role === 'user';
  return (
    <div className={`chat-row ${isUser ? 'user' : 'assistant'}`}>
      <div className={`chat-bubble ${isUser ? 'user' : 'assistant'}`}>
        <div className="chat-bubble-text">{msg.content}</div>
        {!isUser && msg.sources && <SourcesBlock sources={msg.sources} />}
        {!isUser && msg.created_suggestions && <SuggestionChips items={msg.created_suggestions} />}
      </div>
    </div>
  );
}

export default function Chat() {
  const { t } = useTranslation();

  // Escopo / config
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectId, setProjectId] = useState('');
  const [windowDays, setWindowDays] = useState(0);
  const [provider, setProvider] = useState('');
  const [model, setModel] = useState('');
  const [models, setModels] = useState<string[]>([]);
  const [modelsLoading, setModelsLoading] = useState(false);
  const [modelFreeText, setModelFreeText] = useState(false);

  // Threads
  const [threads, setThreads] = useState<ChatThread[]>([]);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [loadingThread, setLoadingThread] = useState(false);

  // Composer
  const [text, setText] = useState('');
  const [sending, setSending] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const scrollRef = useRef<HTMLDivElement | null>(null);
  const composerRef = useRef<HTMLTextAreaElement | null>(null);

  // Carrega projetos + threads na montagem.
  useEffect(() => {
    listProjects().then(setProjects).catch(() => setProjects([]));
    listThreads().then(setThreads).catch(() => setThreads([]));
  }, []);

  // Busca modelos do provider; degrada para texto livre se vazio/404.
  useEffect(() => {
    let cancelled = false;
    setModel('');
    setModelFreeText(false);
    if (!provider) {
      setModels([]);
      return;
    }
    setModelsLoading(true);
    listAdapterModels(provider)
      .then((ms) => {
        if (cancelled) return;
        setModels(ms);
        setModelFreeText(ms.length === 0);
      })
      .finally(() => !cancelled && setModelsLoading(false));
    return () => {
      cancelled = true;
    };
  }, [provider]);

  // Auto-scroll para o fim quando mensagens mudam.
  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages, sending]);

  const openThread = useCallback(async (id: string) => {
    setActiveId(id);
    setErr(null);
    setLoadingThread(true);
    try {
      const detail = await getThread(id);
      setMessages(detail.messages ?? []);
      if (detail.thread?.provider) setProvider(detail.thread.provider);
      if (detail.thread?.model) setModel(detail.thread.model);
    } catch (e) {
      setErr(String(e));
      setMessages([]);
    } finally {
      setLoadingThread(false);
    }
  }, []);

  async function startNewThread() {
    setErr(null);
    try {
      const scope =
        projectId || windowDays
          ? { project_id: projectId || undefined, window_days: windowDays || undefined }
          : undefined;
      const th = await createThread({ scope, provider: provider || undefined, model: model.trim() || undefined });
      setThreads((prev) => [th, ...prev.filter((p) => p.id !== th.id)]);
      setActiveId(th.id);
      setMessages([]);
      setTimeout(() => composerRef.current?.focus(), 0);
    } catch (e) {
      setErr(String(e));
    }
  }

  async function ensureThread(): Promise<string | null> {
    if (activeId) return activeId;
    try {
      const scope =
        projectId || windowDays
          ? { project_id: projectId || undefined, window_days: windowDays || undefined }
          : undefined;
      const th = await createThread({ scope, provider: provider || undefined, model: model.trim() || undefined });
      setThreads((prev) => [th, ...prev.filter((p) => p.id !== th.id)]);
      setActiveId(th.id);
      return th.id;
    } catch (e) {
      setErr(String(e));
      return null;
    }
  }

  async function doSend() {
    const trimmed = text.trim();
    if (!trimmed || sending) return;
    setErr(null);
    const tid = await ensureThread();
    if (!tid) return;

    const userMsg: ChatMessage = {
      seq: messages.length ? messages[messages.length - 1].seq + 1 : 0,
      role: 'user',
      content: trimmed,
    };
    setMessages((m) => [...m, userMsg]);
    setText('');
    setSending(true);
    try {
      const res = await sendMessage(tid, { text: trimmed, provider: provider || undefined, model: model.trim() || undefined });
      const assistant: ChatMessage = {
        ...res.assistant,
        role: 'assistant',
        sources: res.assistant?.sources ?? res.sources ?? [],
        created_suggestions: res.assistant?.created_suggestions ?? res.created_suggestions ?? [],
      };
      setMessages((m) => [...m, assistant]);
      // Atualiza a lista de threads (contagem/título podem ter mudado).
      listThreads().then(setThreads).catch(() => {});
    } catch (e) {
      setErr(String(e));
      // devolve o texto para o usuário poder reenviar
      setText(trimmed);
      setMessages((m) => m.filter((x) => x !== userMsg));
    } finally {
      setSending(false);
      setTimeout(() => composerRef.current?.focus(), 0);
    }
  }

  function onComposerKey(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      doSend();
    }
  }

  const projectName = useMemo(() => {
    if (!projectId) return '';
    return projects.find((p) => p.id === projectId)?.name ?? projectId;
  }, [projectId, projects]);

  const showModelDropdown = models.length > 0 && !modelFreeText;

  return (
    <div className="chat-page">
      <header className="chat-page-head">
        <div>
          <h2>{t('chat.title')}</h2>
          <p className="muted">{t('chat.subtitle')}</p>
        </div>
        <button className="btn btn-accent" onClick={startNewThread}>
          + {t('chat.newThread')}
        </button>
      </header>

      {err && <div className="error-banner" role="alert">{err}</div>}

      <div className="chat-layout">
        {/* Sidebar: escopo + threads */}
        <aside className="chat-sidebar" aria-label={t('chat.threads.title')}>
          <section className="chat-scope" aria-label={t('chat.scope.title')}>
            <div className="chat-scope-head muted">{t('chat.scope.title')}</div>

            <div className="retro-field">
              <label htmlFor="chat-project">{t('chat.scope.project')}</label>
              <select id="chat-project" value={projectId} onChange={(e) => setProjectId(e.target.value)}>
                <option value="">{t('chat.scope.allProjects')}</option>
                {projects.map((p) => (
                  <option key={p.id} value={p.id}>{p.name}</option>
                ))}
              </select>
            </div>

            <div className="retro-field">
              <span className="retro-field-label">{t('chat.scope.window')}</span>
              <div className="retro-segmented" role="group" aria-label={t('chat.scope.window')}>
                {WINDOW_PRESETS.map((w) => (
                  <button
                    key={w}
                    type="button"
                    className={`retro-seg ${windowDays === w ? 'active' : ''}`}
                    aria-pressed={windowDays === w}
                    onClick={() => setWindowDays(w)}
                  >
                    {w === 0 ? t('chat.scope.allTime') : `${w}d`}
                  </button>
                ))}
              </div>
            </div>

            <div className="retro-field">
              <label htmlFor="chat-provider">{t('chat.scope.provider')}</label>
              <select id="chat-provider" value={provider} onChange={(e) => setProvider(e.target.value)}>
                {PROVIDERS.map((p) => (
                  <option key={p || 'default'} value={p}>{providerLabel(p, t)}</option>
                ))}
              </select>
            </div>

            <div className="retro-field">
              <label htmlFor="chat-model">
                {t('chat.scope.model')} <span className="faint">({t('chat.scope.modelOptional')})</span>
              </label>
              {modelsLoading ? (
                <div className="retro-inline-loading muted pulse">{t('chat.scope.modelLoading')}</div>
              ) : showModelDropdown ? (
                <select
                  id="chat-model"
                  value={model}
                  onChange={(e) => {
                    if (e.target.value === '__custom__') {
                      setModelFreeText(true);
                      setModel('');
                    } else {
                      setModel(e.target.value);
                    }
                  }}
                >
                  <option value="">{t('chat.scope.modelSelectPlaceholder')}</option>
                  {models.map((m) => (
                    <option key={m} value={m}>{m}</option>
                  ))}
                  <option value="__custom__">{t('chat.scope.modelCustom')}</option>
                </select>
              ) : (
                <>
                  <input
                    id="chat-model"
                    type="text"
                    value={model}
                    placeholder={provider === 'opencode' ? 'anthropic/claude-sonnet-4-6' : 'claude-sonnet-4-6'}
                    onChange={(e) => setModel(e.target.value)}
                  />
                  {provider && <span className="retro-field-hint">{t('chat.scope.modelFreeHint')}</span>}
                </>
              )}
            </div>
          </section>

          <section className="chat-thread-list" aria-label={t('chat.threads.title')}>
            <div className="chat-scope-head muted">{t('chat.threads.title')}</div>
            {threads.length === 0 ? (
              <p className="faint chat-threads-empty">{t('chat.threads.empty')}</p>
            ) : (
              <ul>
                {threads.map((th) => (
                  <li key={th.id}>
                    <button
                      type="button"
                      className={`chat-thread-item ${activeId === th.id ? 'active' : ''}`}
                      aria-current={activeId === th.id ? 'true' : undefined}
                      onClick={() => openThread(th.id)}
                    >
                      <span className="chat-thread-title">
                        {th.title || t('chat.threads.untitled')}
                      </span>
                      {typeof th.message_count === 'number' && (
                        <span className="faint chat-thread-count">{th.message_count}</span>
                      )}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </aside>

        {/* Conversa */}
        <section className="chat-main" aria-label={t('chat.title')}>
          <div className="chat-scroll" ref={scrollRef}>
            {loadingThread ? (
              <div className="chat-empty muted pulse">{t('chat.loading')}</div>
            ) : messages.length === 0 ? (
              <div className="chat-empty">
                <div className="chat-empty-mark" aria-hidden="true">✦</div>
                <h3>{t('chat.empty.title')}</h3>
                <p className="muted">{t('chat.empty.body')}</p>
                {(projectId || windowDays) && (
                  <p className="faint chat-empty-scope">
                    {t('chat.empty.scopeHint', {
                      project: projectName || t('chat.scope.allProjects'),
                      window: windowDays === 0 ? t('chat.scope.allTime') : `${windowDays}d`,
                    })}
                  </p>
                )}
              </div>
            ) : (
              <div className="chat-messages">
                {messages.map((m) => (
                  <Bubble key={`${m.role}-${m.seq}`} msg={m} />
                ))}
                {sending && (
                  <div className="chat-row assistant">
                    <div className="chat-bubble assistant chat-typing" aria-live="polite">
                      <span className="chat-dot" />
                      <span className="chat-dot" />
                      <span className="chat-dot" />
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>

          <form
            className="chat-composer"
            onSubmit={(e) => {
              e.preventDefault();
              doSend();
            }}
          >
            <textarea
              ref={composerRef}
              className="chat-composer-input"
              value={text}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={onComposerKey}
              placeholder={t('chat.composer.placeholder')}
              aria-label={t('chat.composer.placeholder')}
              rows={1}
              disabled={sending}
            />
            <button
              type="submit"
              className="btn btn-accent chat-send"
              disabled={sending || !text.trim()}
              aria-label={t('chat.composer.send')}
            >
              {sending ? t('chat.composer.sending') : t('chat.composer.send')}
            </button>
          </form>
          <div className="chat-composer-hint faint">{t('chat.composer.hint')}</div>
        </section>
      </div>
    </div>
  );
}
