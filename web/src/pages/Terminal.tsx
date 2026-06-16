import { useEffect, useRef, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { killSession, postHandoff, pasteImage } from '../api';
import { useEvents } from '../useEvents';
import '@xterm/xterm/css/xterm.css';

export default function Terminal() {
  const { id } = useParams<{ id: string }>();
  const { t } = useTranslation();
  const navigate = useNavigate();
  const ref = useRef<HTMLDivElement>(null);

  const [contextUsed, setContextUsed] = useState(0);
  const [contextLimit, setContextLimit] = useState(0);
  const [showHandoffBanner, setShowHandoffBanner] = useState(false);
  const [handoffBusy, setHandoffBusy] = useState(false);
  const [killBusy, setKillBusy] = useState(false);

  const handleKill = async () => {
    if (!id || killBusy) return;
    setKillBusy(true);
    try {
      await killSession(id);
      // sessão encerrada no backend → sai do terminal (o cleanup do effect
      // fecha o WebSocket). Volta para a lista de sessões.
      navigate('/sessions');
    } catch {
      setKillBusy(false);
    }
  };

  useEvents((ev) => {
    if (!id) return;
    if (ev.type === 'session.context') {
      const p = ev.payload as { session_id: string; used: number; limit: number };
      if (p.session_id === id) {
        setContextUsed(p.used);
        setContextLimit(p.limit);
      }
    }
    if (ev.type === 'session.context_high') {
      const p = ev.payload as { session_id: string };
      if (p.session_id === id) {
        setShowHandoffBanner(true);
      }
    }
  });

  const handleHandoff = async () => {
    if (!id) return;
    setHandoffBusy(true);
    try {
      const result = await postHandoff(id);
      navigate(`/sessions/${result.new_id}`);
    } catch {
      setHandoffBusy(false);
    }
  };

  useEffect(() => {
    if (!ref.current || !id) return;
    const el = ref.current;
    const term = new XTerm({
      fontFamily: "'JetBrains Mono Variable', ui-monospace, monospace",
      fontSize: 13,
      cursorBlink: true,
      // Tema claro, alinhado à paleta do site (var(--surface-sunk), --ink-soft etc.).
      // Cores ANSI escurecidas o suficiente para legibilidade sobre fundo claro.
      theme: {
        background: '#f7f2e8',
        foreground: '#3a342b',
        cursor: '#191510',
        cursorAccent: '#f7f2e8',
        selectionBackground: '#ffe3a0',
        selectionForeground: '#191510',
        black: '#191510',
        red: '#c62f2f',
        green: '#1f9d57',
        yellow: '#b07d0a',
        blue: '#1f7fc4',
        magenta: '#a64ca6',
        cyan: '#0e8a8a',
        white: '#6e6557',
        brightBlack: '#8a8173',
        brightRed: '#e23b3b',
        brightGreen: '#24b365',
        brightYellow: '#c9920a',
        brightBlue: '#2fa4ee',
        brightMagenta: '#c25fc2',
        brightCyan: '#11a3a3',
        brightWhite: '#191510',
      },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(ref.current);
    fit.fit();

    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    const ws = new WebSocket(`${proto}://${location.host}/api/sessions/${id}/term`);
    ws.binaryType = 'arraybuffer';

    ws.onmessage = (e) => {
      if (typeof e.data === 'string') term.write(e.data);
      else term.write(new Uint8Array(e.data));
    };

    let disposed = false;
    const note = (msg: string) => {
      if (!disposed) term.write(`\r\n\x1b[2m[worrel] ${msg}\x1b[0m\r\n`);
    };
    ws.onclose = () => note(t('terminal.connectionClosed'));
    ws.onerror = () => note(t('terminal.connectionError'));

    const sendResize = () => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
      }
    };

    ws.onopen = () => {
      sendResize();
      term.focus();
    };

    const dataDisp = term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'stdin', data }));
      }
    });

    // Colar imagem (Ctrl/Cmd+V): o xterm só cola texto e o clipboard do browser
    // não chega ao PTY no servidor. Interceptamos um item image/* do clipboard,
    // subimos os bytes (que o servidor salva no workspace) e injetamos o caminho
    // resultante no stdin — a CLI (ex. claude-code) anexa a imagem pelo path.
    const onPaste = (e: ClipboardEvent) => {
      const item = Array.from(e.clipboardData?.items ?? []).find((it) => it.type.startsWith('image/'));
      if (!item) return; // texto normal → deixa o xterm cuidar
      e.preventDefault();
      e.stopPropagation();
      const blob = item.getAsFile();
      if (!blob) return;
      pasteImage(id, blob)
        .then(({ path }) => {
          if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'stdin', data: path }));
          }
        })
        .catch(() => note(t('terminal.pasteImageError')));
    };
    el.addEventListener('paste', onPaste, true);

    const refit = () => { fit.fit(); sendResize(); };
    window.addEventListener('resize', refit);

    // Re-ajusta o número de linhas sempre que o container muda de tamanho
    // (header/banner aparecendo, layout assentando após o mount, etc.).
    // Sem isso o xterm mantém a contagem inicial de linhas e o final do
    // terminal fica fora da área visível, sem rolagem.
    const ro = new ResizeObserver(() => refit());
    ro.observe(el);

    return () => {
      disposed = true;
      window.removeEventListener('resize', refit);
      el.removeEventListener('paste', onPaste, true);
      ro.disconnect();
      dataDisp.dispose();
      ws.close();
      term.dispose();
    };
  }, [id]);

  const contextPct = contextLimit > 0 ? Math.round(contextUsed * 100 / contextLimit) : 0;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0, overflow: 'hidden' }}>
      <div style={{
        padding: '12px 20px', display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap',
        background: 'var(--surface)', borderBottom: '1px solid var(--line)',
      }}>
        <strong style={{ color: 'var(--ink)', fontFamily: 'var(--display)' }}>{t('terminal.title')}</strong>
        <button className="btn btn-danger btn-sm" disabled={killBusy} onClick={handleKill}>{t('terminal.kill')}</button>
        {contextLimit > 0 && (
          <span className="mono" style={{ fontSize: '0.78rem', color: 'var(--muted)', marginLeft: 'auto', display: 'inline-flex', alignItems: 'center', gap: 8 }}>
            {t('sessions.contextBar', { used: contextUsed, limit: contextLimit })}
            <span style={{
              display: 'inline-block', width: 72, height: 7,
              background: 'var(--surface-warm)', border: '1px solid var(--line)', borderRadius: 'var(--r-pill)', overflow: 'hidden'
            }}>
              <span style={{
                display: 'block', height: '100%',
                width: `${contextPct}%`,
                background: contextPct >= 80 ? 'var(--red)' : 'var(--sky)',
              }} />
            </span>
            {contextPct}%
          </span>
        )}
      </div>
      {showHandoffBanner && (
        <div style={{
          padding: '10px 20px', background: 'var(--fill-amber)',
          borderBottom: '1px solid var(--amber)',
          display: 'flex', gap: 12, alignItems: 'center',
        }}>
          <span style={{ color: '#7a5800', fontWeight: 500 }}>{t('handoff.banner')}</span>
          <button
            className="btn btn-primary btn-sm"
            disabled={handoffBusy}
            onClick={handleHandoff}
          >
            {t('handoff.start')}
          </button>
        </div>
      )}
      <div ref={ref} style={{ flex: 1, minHeight: 0, overflow: 'hidden', background: '#f7f2e8', padding: 12 }} />
    </div>
  );
}
