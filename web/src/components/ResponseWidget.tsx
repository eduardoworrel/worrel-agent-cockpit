import { useState } from 'react';
import type { ResponseWidget as Widget } from '../api';

// RESPONSE_WIDGET_ENABLED é a flag do experimento. "Se der ruim, deleta o switch
// e volta ao form de texto": basta pôr false (ou remover este componente do
// InteractionPanel) — as mudanças 1 (request_summary) e 2 (ask_html) não dependem
// disto.
export const RESPONSE_WIDGET_ENABLED = true;

// widgetSupported diz se o widget tem um tipo que sabemos renderizar (e dados
// válidos). O InteractionPanel usa isto para decidir entre o widget e o form de
// texto padrão — evita um widget que renderiza vazio e deixa o usuário sem input.
export function widgetSupported(widget?: Widget): boolean {
  if (!RESPONSE_WIDGET_ENABLED || !widget) return false;
  if (widget.type === 'range') return true;
  if (widget.type === 'options') {
    return Array.isArray(widget.spec?.options) && (widget.spec!.options as unknown[]).length > 0;
  }
  return false;
}

interface Props {
  widget: Widget;
  busy: boolean;
  // Envia a resposta pelo mesmo caminho de texto/escolha já existente.
  onSubmit: (value: string) => void;
}

// ResponseWidget renderiza um controle de resposta dinâmico guiado por
// `widget.type` (switch). O iframe do ask_html é SÓ apresentação; a resposta
// acontece aqui em React — sem <script> no HTML, sem postMessage.
//
// Tipos desconhecidos retornam null → o InteractionPanel cai no form de texto.
export default function ResponseWidget({ widget, busy, onSubmit }: Props) {
  switch (widget.type) {
    case 'range':
      return <RangeWidget widget={widget} busy={busy} onSubmit={onSubmit} />;
    case 'options':
      return <OptionsWidget widget={widget} busy={busy} onSubmit={onSubmit} />;
    default:
      return null;
  }
}

function num(v: unknown, fallback: number): number {
  return typeof v === 'number' && Number.isFinite(v) ? v : fallback;
}

function RangeWidget({ widget, busy, onSubmit }: Props) {
  const min = num(widget.spec?.min, 0);
  const max = num(widget.spec?.max, 100);
  const step = num(widget.spec?.step, 1);
  const [value, setValue] = useState(Math.round((min + max) / 2));
  return (
    <div className="ixp-widget ixp-widget-range">
      <input type="range" min={min} max={max} step={step} value={value}
        onChange={(e) => setValue(Number(e.target.value))} disabled={busy} />
      <span className="ixp-widget-value">{value}</span>
      <button className="btn btn-primary btn-sm" disabled={busy}
        onClick={() => onSubmit(String(value))}>OK</button>
    </div>
  );
}

function OptionsWidget({ widget, busy, onSubmit }: Props) {
  const options = Array.isArray(widget.spec?.options)
    ? (widget.spec!.options as unknown[]).map(String)
    : [];
  if (options.length === 0) return null;
  return (
    <div className="ixp-actions ixp-options ixp-widget-options">
      {options.map((opt) => (
        <button key={opt} className="btn btn-secondary btn-sm" disabled={busy}
          onClick={() => onSubmit(opt)}>{opt}</button>
      ))}
    </div>
  );
}
