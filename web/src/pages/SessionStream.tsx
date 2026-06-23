import { useCallback, useEffect, useRef, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { getInteraction, sendPrompt, respondInteraction, killSession } from '../api';
import type { InteractionSnapshot, HistoryLine } from '../api';
import { useEvents } from '../useEvents';
import { useDraft } from '../useDraft';
import { providerLabel } from '../session';

// SessionStream é a "interface de terminal" de uma sessão dirigida pelo MOTOR
// (stream-json): não há PTY/xterm — mostramos o HISTÓRICO da conversa (você, IA,
// ferramentas, decisões), a permissão pendente e um campo para mandar prompts.
export default function SessionStream() {
  const { id } = useParams<{ id: string }>();
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [snap, setSnap] = useState<InteractionSnapshot | null>(null);
  // Rascunho persistido por sessão: navegar entre terminais não perde o texto.
  const [text, setText, clearDraft] = useDraft(id);
  const [busy, setBusy] = useState(false);
  // Confirmação de encerramento: espelha o modal do x no sidebar (AppNav).
  const [confirmKill, setConfirmKill] = useState(false);
  const bodyRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  const load = useCallback(() => {
    if (!id) return;
    getInteraction(id).then(setSnap).catch(() => { /* ignore */ });
  }, [id]);

  useEffect(() => { load(); }, [load]);
  useEvents(useCallback((ev) => {
    const p = ev.payload as { session_id?: string; id?: string };
    if ((p?.session_id === id || p?.id === id) &&
      ['interaction.changed', 'session.awaiting', 'session.busy', 'session.ended'].includes(ev.type)) {
      load();
    }
  }, [id, load]));

  // Foca o campo de prompt ao selecionar uma sessão (muda o id) — o autoFocus
  // do textarea só dispara na montagem inicial, então ao navegar entre sessões
  // (mesmo componente, id diferente) ele não refocaria sozinho.
  useEffect(() => {
    inputRef.current?.focus();
  }, [id]);

  // rola para o fim quando o histórico cresce.
  useEffect(() => {
    bodyRef.current?.scrollTo({ top: bodyRef.current.scrollHeight });
  }, [snap?.history?.length, snap?.interrupt]);

  async function act(fn: () => Promise<unknown>) {
    if (busy) return;
    setBusy(true);
    try { await fn(); load(); } catch { /* noop */ } finally { setBusy(false); }
  }

  function submit() {
    if (!id || !text.trim()) return;
    const t = text.trim();
    clearDraft();
    act(() => sendPrompt(id, t));
  }

  const interrupt = snap?.interrupt;
  const isPermission = !!interrupt?.request_id;
  const state = snap?.state ?? 'working';
  const history = snap?.history ?? [];

  const reply = (value: string) => { if (id) act(() => sendPrompt(id, value)); };

  return (
    <div className="sstream">
      <header className="sstream-head">
        <button className="btn btn-secondary btn-sm" onClick={() => navigate('/')}>← {t('home.nav.home')}</button>
        <span className="ixp-state" data-state={state}>{t(`home.ix.state.${state}`)}</span>
        <span className="sstream-engine">{providerLabel('engine')}</span>
        <button className="btn btn-danger btn-sm" style={{ marginLeft: 'auto' }}
          disabled={busy} onClick={() => setConfirmKill(true)}>
          {t('terminal.kill')}
        </button>
      </header>

      <div className="sstream-body" ref={bodyRef}>
        {history.length === 0 && <div className="sstream-empty">{t('home.ix.working')}</div>}
        {groupHistory(history).map((g, i) =>
          g.kind === 'console'
            ? <ConsoleBlock key={i} rows={g.rows} />
            : <ChatLine key={i} line={g.line} />,
        )}
        {state === 'working' && history.length > 0 && (
          <div className="chat-thinking">{t('home.ix.working')}<span className="dots"><i /><i /><i /></span></div>
        )}
      </div>

      {isPermission ? (
        <div className="sstream-foot sstream-permission">
          <div className="sstream-perm-q">{interrupt!.prompt}</div>
          <div className="ixp-actions">
            <button className="btn btn-primary btn-sm" disabled={busy}
              onClick={() => id && act(() => respondInteraction(id, interrupt!.request_id, 'allow'))}>{t('ask.allow')}</button>
            <button className="btn btn-danger btn-sm" disabled={busy}
              onClick={() => id && act(() => respondInteraction(id, interrupt!.request_id, 'deny'))}>{t('ask.deny')}</button>
          </div>
        </div>
      ) : interrupt?.kind === 'choice' && interrupt.options?.length ? (
        <div className="sstream-foot sstream-permission">
          {interrupt.prompt && <div className="sstream-perm-q">{interrupt.prompt}</div>}
          <div className="ixp-actions ixp-options">
            {interrupt.options.map((opt) => (
              <button key={opt} className="btn btn-secondary btn-sm" disabled={busy} onClick={() => reply(opt)}>{opt}</button>
            ))}
          </div>
          <div className="sstream-foot" style={{ padding: 0, border: 'none', background: 'none' }}>
            <span className="nsw-prompt-glyph" aria-hidden="true">›</span>
            <textarea ref={inputRef} className="sstream-input" value={text} onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submit(); } }}
              placeholder={t('home.ix.promptPlaceholder')} rows={2} />
            <button className="btn btn-primary btn-sm" disabled={busy || !text.trim()} onClick={submit}>{t('home.ix.send')}</button>
          </div>
        </div>
      ) : (
        <div className="sstream-foot">
          <span className="nsw-prompt-glyph" aria-hidden="true">›</span>
          <textarea
            ref={inputRef}
            className="sstream-input"
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submit(); } }}
            placeholder={t('home.ix.promptPlaceholder')}
            rows={2}
            autoFocus
          />
          <button className="btn btn-primary btn-sm" disabled={busy || !text.trim()} onClick={submit}>{t('home.ix.send')}</button>
        </div>
      )}

      {confirmKill && (
        <div className="modal-overlay" onClick={() => !busy && setConfirmKill(false)}>
          <div className="modal" role="dialog" aria-modal="true" aria-labelledby="sstream-kill-title"
            onClick={(e) => e.stopPropagation()}>
            <h3 id="sstream-kill-title" style={{ marginTop: 0 }}>{t('sessions.endConfirmTitle', 'Encerrar sessão em andamento?')}</h3>
            <p>{t('sessions.endConfirmMsg', 'O processo do agente será finalizado. A sessão fica no histórico e pode ser recomeçada depois.')}</p>
            <div style={{ display: 'flex', gap: '1rem', marginTop: '1.5rem' }}>
              <button className="btn btn-secondary" style={{ flex: 1 }} disabled={busy} onClick={() => setConfirmKill(false)}>
                {t('common.cancel')}
              </button>
              <button className="btn btn-primary" style={{ flex: 1 }} disabled={busy}
                onClick={() => id && act(() => killSession(id).then(() => { setConfirmKill(false); navigate('/'); }))}>
                {t('sessions.end', 'Encerrar')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────
// Console: a voz da MÁQUINA.
//
// O backend emite duas formas de linha "tool":
//   • chamada   →  "<nome> <json-dos-argumentos>"   (ex.: Bash {"command":"go test"})
//   • resultado →  "→ <stdout truncado>"
// Linhas tool consecutivas pertencem ao MESMO transcript de shell, então as
// agrupamos num único bloco-console escuro embutido na conversa em papel.
// ─────────────────────────────────────────────────────────────────────────

type ConsoleRow =
  | { kind: 'cmd'; tool: string; args: string }
  | { kind: 'out'; text: string };

type Group =
  | { kind: 'console'; rows: ConsoleRow[] }
  | { kind: 'line'; line: HistoryLine };

// groupHistory funde linhas tool adjacentes num grupo "console" e deixa
// você/IA/sistema como linhas avulsas.
function groupHistory(history: HistoryLine[]): Group[] {
  const out: Group[] = [];
  for (const line of history) {
    if (line.role === 'tool') {
      const last = out[out.length - 1];
      const rows = last?.kind === 'console' ? last.rows : (out.push({ kind: 'console', rows: [] }), (out[out.length - 1] as { rows: ConsoleRow[] }).rows);
      rows.push(parseToolLine(line.text));
    } else {
      out.push({ kind: 'line', line });
    }
  }
  return out;
}

function parseToolLine(text: string): ConsoleRow {
  if (text.startsWith('→')) return { kind: 'out', text: text.replace(/^→\s*/, '') };
  const sp = text.indexOf(' ');
  const tool = sp === -1 ? text : text.slice(0, sp);
  const rest = sp === -1 ? '' : text.slice(sp + 1).trim();
  return { kind: 'cmd', tool, args: formatArgs(tool, rest) };
}

// sigilo + cor por ferramenta. O verbo em minúsculas dá o ar de comando de shell;
// a cor liga a ação à sua natureza (ler=azul, escrever=laranja, rodar=verde…).
const TOOL_META: Record<string, { verb: string; accent: string }> = {
  Bash: { verb: 'bash', accent: 'green' },
  Read: { verb: 'read', accent: 'sky' },
  Edit: { verb: 'edit', accent: 'amber' },
  MultiEdit: { verb: 'edit', accent: 'amber' },
  Write: { verb: 'write', accent: 'orange' },
  NotebookEdit: { verb: 'edit', accent: 'amber' },
  Grep: { verb: 'grep', accent: 'sky' },
  Glob: { verb: 'glob', accent: 'sky' },
  Task: { verb: 'task', accent: 'pink' },
  Agent: { verb: 'agent', accent: 'pink' },
  WebFetch: { verb: 'fetch', accent: 'pink' },
  WebSearch: { verb: 'search', accent: 'pink' },
  TodoWrite: { verb: 'todo', accent: 'amber' },
};

function toolMeta(name: string) {
  return TOOL_META[name] ?? { verb: name.toLowerCase(), accent: 'cream' };
}

// formatArgs extrai a "cauda" legível de cada comando: o comando real do Bash,
// o caminho dos arquivos, o padrão do grep… Cai no JSON cru se não reconhecer.
function formatArgs(tool: string, rest: string): string {
  if (!rest) return '';
  let obj: Record<string, unknown> | null = null;
  try { obj = JSON.parse(rest); } catch { /* truncado/inválido → mostra cru */ }
  if (!obj || typeof obj !== 'object') return rest;
  const s = (k: string) => (typeof obj![k] === 'string' ? (obj![k] as string) : '');
  switch (tool) {
    case 'Bash': return s('command') || rest;
    case 'Read': case 'Write': case 'Edit': case 'MultiEdit': case 'NotebookEdit':
      return shortPath(s('file_path') || s('path') || s('notebook_path'));
    case 'Grep': return [s('pattern'), s('path') && `in ${shortPath(s('path'))}`].filter(Boolean).join('  ');
    case 'Glob': return s('pattern');
    case 'Task': case 'Agent': return s('description') || s('subagent_type');
    case 'WebFetch': case 'WebSearch': return s('url') || s('query');
    default: {
      const parts = Object.entries(obj)
        .filter(([, v]) => typeof v !== 'object')
        .map(([k, v]) => `${k}=${String(v)}`);
      return parts.join(' ') || rest;
    }
  }
}

// encurta caminhos longos para caber numa linha de terminal (mantém a cauda).
function shortPath(p: string): string {
  if (!p) return '';
  const parts = p.split('/');
  return parts.length > 4 ? '…/' + parts.slice(-3).join('/') : p;
}

// ConsoleBlock desenha o transcript de shell: barra-título com os pontos do
// terminal, e cada comando como `$ verbo argumentos` com seu stdout abaixo.
function ConsoleBlock({ rows }: { rows: ConsoleRow[] }) {
  const cmds = rows.filter((r) => r.kind === 'cmd').length;
  return (
    <div className="term-block">
      <div className="term-bar">
        <span className="term-dots" aria-hidden="true"><i /><i /><i /></span>
        <span className="term-bar-label">shell · {cmds} {cmds === 1 ? 'comando' : 'comandos'}</span>
      </div>
      <div className="term-body">
        {rows.map((r, i) =>
          r.kind === 'cmd' ? (
            <div className="term-cmd" key={i}>
              <span className="term-sigil" aria-hidden="true">$</span>
              <span className={`term-verb t-${toolMeta(r.tool).accent}`}>{toolMeta(r.tool).verb}</span>
              {r.args && <span className="term-args">{r.args}</span>}
            </div>
          ) : (
            <pre className="term-out" key={i}>{r.text}</pre>
          ),
        )}
      </div>
    </div>
  );
}

// ChatLine renderiza uma linha de conversa: você → bolha à direita; IA →
// markdown à esquerda; sistema → nota central. (Ferramentas vão no ConsoleBlock.)
function ChatLine({ line }: { line: HistoryLine }) {
  if (line.role === 'system') {
    return <div className="chat-system">{line.text}</div>;
  }
  if (line.role === 'you') {
    return <div className="chat-row chat-you"><div className="chat-bubble">{line.text}</div></div>;
  }
  // ai → markdown
  return (
    <div className="chat-row chat-ai">
      <div className="chat-bubble chat-md">
        <ReactMarkdown remarkPlugins={[remarkGfm]}>{line.text}</ReactMarkdown>
      </div>
    </div>
  );
}
