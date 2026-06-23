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
          g.kind === 'cmd'
            ? <CommandCard key={i} cmd={g.cmd} />
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
// Cada CHAMADA vira um card de comando isolado; os resultados que a seguem
// (até a próxima chamada) ficam acoplados a ele como saída.
// ─────────────────────────────────────────────────────────────────────────

interface Cmd { tool: string; args: string; out: string[]; }

type Group =
  | { kind: 'cmd'; cmd: Cmd }
  | { kind: 'line'; line: HistoryLine };

// groupHistory transforma cada chamada de ferramenta num card de comando
// próprio (com sua saída acoplada) e deixa você/IA/sistema como linhas avulsas.
function groupHistory(history: HistoryLine[]): Group[] {
  const out: Group[] = [];
  for (const line of history) {
    if (line.role === 'tool') {
      const row = parseToolLine(line.text);
      const last = out[out.length - 1];
      if (row.kind === 'out') {
        // saída acopla ao último comando; se não houver, vira um card de saída solta.
        if (last?.kind === 'cmd') last.cmd.out.push(row.text);
        else out.push({ kind: 'cmd', cmd: { tool: '', args: '', out: [row.text] } });
      } else {
        out.push({ kind: 'cmd', cmd: { tool: row.tool, args: row.args, out: [] } });
      }
    } else {
      out.push({ kind: 'line', line });
    }
  }
  return out;
}

type ParsedRow =
  | { kind: 'cmd'; tool: string; args: string }
  | { kind: 'out'; text: string };

function parseToolLine(text: string): ParsedRow {
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

function toolMeta(name: string): { verb: string; accent: string } {
  if (TOOL_META[name]) return TOOL_META[name];
  // Ferramentas MCP: "mcp__servidor__acao" → mostramos só a ação, em rosa.
  if (name.startsWith('mcp__')) {
    const seg = name.split('__');
    return { verb: seg[seg.length - 1] || name, accent: 'pink' };
  }
  return { verb: name.toLowerCase(), accent: 'cream' };
}

// Campos que costumam carregar a "alma" de um comando — usados quando a
// ferramenta não é uma das conhecidas, para nunca despejar o JSON inteiro.
const PREFERRED_KEYS = [
  'command', 'cmd', 'query', 'prompt', 'pattern', 'url',
  'file_path', 'path', 'description', 'message', 'text', 'name', 'title', 'content',
];

// trunca valores longos para caberem numa linha de comando.
function clip(v: string, max = 240): string {
  const one = v.replace(/\s+/g, ' ').trim();
  return one.length > max ? one.slice(0, max) + '…' : one;
}

// rawField extrai o valor de uma chave string direto do texto JSON, mesmo que
// ele esteja TRUNCADO (o backend corta linhas longas de tool-call e o
// JSON.parse falha). Sem isso, comandos viravam JSON cru na tela.
function rawField(rest: string, keys: string[]): string {
  for (const k of keys) {
    const m = rest.match(new RegExp(`"${k}"\\s*:\\s*"((?:[^"\\\\]|\\\\.)*)"?`));
    if (m && m[1]) {
      // desescapa as sequências JSON (\", \\, \n…) de forma segura.
      try { return clip(JSON.parse(`"${m[1].replace(/\\$/, '')}"`)); }
      catch { return clip(m[1].replace(/\\(["\\/])/g, '$1')); }
    }
  }
  return '';
}

// formatArgs extrai a "cauda" legível de cada comando: o comando real do Bash,
// o caminho dos arquivos, o padrão do grep… Para tools desconhecidas, escolhe
// o campo mais relevante do JSON em vez de despejar o objeto cru.
function formatArgs(tool: string, rest: string): string {
  if (!rest) return '';
  let obj: Record<string, unknown> | null = null;
  try { obj = JSON.parse(rest); } catch { /* truncado/inválido → extrai por regex abaixo */ }
  if (!obj || typeof obj !== 'object') {
    // JSON truncado/inválido: tenta puxar o campo relevante do texto cru antes
    // de desistir, para nunca despejar o objeto na tela.
    const keys = tool === 'Bash' ? ['command', ...PREFERRED_KEYS] : PREFERRED_KEYS;
    return rawField(rest, keys) || clip(rest);
  }
  const s = (k: string) => (typeof obj![k] === 'string' ? (obj![k] as string) : '');
  switch (tool) {
    case 'Bash': return clip(s('command') || rest);
    case 'Read': case 'Write': case 'Edit': case 'MultiEdit': case 'NotebookEdit':
      return shortPath(s('file_path') || s('path') || s('notebook_path'));
    case 'Grep': return [s('pattern'), s('path') && `in ${shortPath(s('path'))}`].filter(Boolean).join('  ');
    case 'Glob': return s('pattern');
    case 'Task': case 'Agent': return clip(s('description') || s('subagent_type'));
    case 'WebFetch': case 'WebSearch': return clip(s('url') || s('query'));
    default: {
      // 1) tenta o primeiro campo "relevante" presente.
      for (const k of PREFERRED_KEYS) {
        const v = s(k);
        if (v) return (k === 'file_path' || k === 'path') ? shortPath(v) : clip(v);
      }
      // 2) senão, compacta só os campos escalares como chave=valor (sem aninhados).
      const parts = Object.entries(obj)
        .filter(([, v]) => v !== null && typeof v !== 'object')
        .map(([k, v]) => `${k}=${clip(String(v), 60)}`);
      return parts.length ? clip(parts.join(' ')) : '';
    }
  }
}

// encurta caminhos longos para caber numa linha de terminal (mantém a cauda).
function shortPath(p: string): string {
  if (!p) return '';
  const parts = p.split('/');
  return parts.length > 4 ? '…/' + parts.slice(-3).join('/') : p;
}

// CommandCard desenha UM comando isolado: cabeçalho com o verbo colorido e o
// argumento, e a saída (se houver) num painel acoplado abaixo.
function CommandCard({ cmd }: { cmd: Cmd }) {
  const meta = cmd.tool ? toolMeta(cmd.tool) : { verb: '', accent: 'cream' };
  return (
    <div className={`cmd-card cmd-${meta.accent}`}>
      {cmd.tool && (
        <div className="cmd-head">
          <span className="cmd-sigil" aria-hidden="true">$</span>
          <span className="cmd-verb">{meta.verb}</span>
          {cmd.args && <span className="cmd-args">{cmd.args}</span>}
        </div>
      )}
      {cmd.out.length > 0 && (
        <pre className="cmd-out">{cmd.out.join('\n')}</pre>
      )}
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
