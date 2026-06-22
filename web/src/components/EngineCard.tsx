import { useEffect, useState } from 'react'

export type ConfigOption = { value: string; label: string; description: string }
export type ConfigField = { key: string; label: string; type: string; default: string; options?: ConfigOption[] }
export type Spec = {
  id: string; name: string; description: string
  triggers: string[]; prompts: ConfigField[]; config: ConfigField[]
  output_type: string; default_on: boolean
}
export type EngineItem = { spec: Spec; config: Record<string, string> }

// Modos de execução (gravam em __trigger). Descrições no front (universais).
const EXEC_MODES: ConfigOption[] = [
  { value: 'project_open_close', label: 'Ao encerrar', description: 'Destila quando a sessão termina. Ao abrir o app, recupera as que fecharam sem análise.' },
  { value: 'realtime', label: 'Ao vivo', description: 'Destila durante a sessão, revisitando as sessões em andamento periodicamente.' },
  { value: 'agent_self', label: 'Agente decide', description: 'Injeta a regra no início; o próprio agente registra (via MCP) quando percebe algo.' },
  { value: 'on_demand', label: 'Sob demanda', description: 'Nada automático: você dispara com o botão Rodar agora.' },
]

// Pills numa linha; cada opção com tooltip (ⓘ). allowed: valores habilitados.
function Pills({ options, current, onSelect, allowed }: {
  options: ConfigOption[]
  current: string
  onSelect: (value: string) => void
  allowed?: string[]
}) {
  return (
    <div className="ec-pills">
      {options.map(o => {
        const ok = !allowed || allowed.includes(o.value)
        return (
          <button key={o.value} type="button" disabled={!ok}
            className={`ec-pill${current === o.value ? ' on' : ''}${!ok ? ' off' : ''}`}
            onClick={() => ok && onSelect(o.value)}>
            <span>{o.label}</span>
            <i className="ec-info" data-tip={ok ? o.description : 'Indisponível neste motor'}>ⓘ</i>
          </button>
        )
      })}
    </div>
  )
}

// Dropdown de modelo: lista os modelos disponíveis do harness escolhido.
function ModelPicker({ harness, current, onSelect }: {
  harness: string
  current: string
  onSelect: (v: string) => void
}) {
  const [models, setModels] = useState<string[]>([])
  useEffect(() => {
    const id = harness || 'claude-code'
    fetch(`/api/adapters/${id}/models`).then(r => r.json())
      .then(d => setModels(d.models || [])).catch(() => setModels([]))
  }, [harness])
  return (
    <select className="ec-input" value={current} onChange={e => onSelect(e.target.value)}>
      <option value="">Padrão do harness</option>
      {models.map(m => <option key={m} value={m}>{m}</option>)}
    </select>
  )
}

export default function EngineCard({ item, setConfig, onRun }: {
  item: EngineItem
  setConfig: (id: string, key: string, value: string) => void
  onRun?: (id: string) => void
}) {
  const { spec, config } = item
  const enabled = config['__enabled'] === 'true'
  const usesLLM = (config['detection_mode'] ?? 'hybrid') !== 'heuristic_only'
  const fields = (spec.config ?? []).filter(f => usesLLM || (f.key !== 'harness' && f.key !== 'model'))
  const prompts = spec.prompts ?? []

  return (
    <div className="ec">
      <style>{EC_CSS}</style>

      <header className="ec-head">
        <div>
          <h3>{spec.name}</h3>
          <p>{spec.description}</p>
        </div>
        <button type="button" role="switch" aria-checked={enabled}
          className={`ec-switch${enabled ? ' on' : ''}`}
          onClick={() => setConfig(spec.id, '__enabled', enabled ? 'false' : 'true')}>
          <span className="ec-knob" />
        </button>
      </header>

      <fieldset className="ec-section" disabled={!enabled} style={{ opacity: enabled ? 1 : 0.5 }}>
        {onRun && (
          <div className="ec-run">
            <button type="button" className="btn btn-primary" onClick={() => onRun(spec.id)}>▶ Rodar agora</button>
            <span>Dispara o motor uma vez agora (o modo “sob demanda” usa este botão).</span>
          </div>
        )}

        <div className="ec-field ec-inline">
          <label>Quando executar</label>
          <Pills options={EXEC_MODES} current={config['__trigger'] ?? spec.triggers[0]} allowed={spec.triggers}
            onSelect={v => setConfig(spec.id, '__trigger', v)} />
        </div>

        {fields.map(f => {
          const current = config[f.key] ?? f.default
          return (
            <div key={f.key} className="ec-field ec-inline">
              <label>{f.label}</label>
              {f.key === 'model' ? (
                <ModelPicker harness={config['harness'] ?? ''} current={current} onSelect={v => setConfig(spec.id, f.key, v)} />
              ) : f.options && f.options.length > 0 ? (
                <Pills options={f.options} current={current} onSelect={v => setConfig(spec.id, f.key, v)} />
              ) : (
                <input className="ec-input" defaultValue={current} onBlur={e => setConfig(spec.id, f.key, e.target.value)} />
              )}
            </div>
          )
        })}

        {prompts.map(f => (
          <div key={f.key} className="ec-field">
            <label>{f.label}</label>
            <textarea className="ec-textarea" defaultValue={config[f.key] ?? f.default}
              onBlur={e => setConfig(spec.id, f.key, e.target.value)} rows={4} />
          </div>
        ))}
      </fieldset>
    </div>
  )
}

const EC_CSS = `
.ec { max-width: 820px; margin: 0 auto 1.25rem; border: 1px solid var(--line-strong, #333); border-radius: 14px;
  background: var(--surface, rgba(255,255,255,0.015)); padding: 18px 20px; box-shadow: 0 2px 10px rgba(0,0,0,0.10); }
.ec-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
.ec-head h3 { margin: 0; font-family: var(--display, inherit); font-size: 1.15rem; color: var(--ink, #eee); }
.ec-head p { margin: 4px 0 0; color: var(--muted, #999); font-size: 0.85rem; max-width: 56ch; }
.ec-switch { flex: none; width: 46px; height: 26px; border-radius: 999px; border: none; cursor: pointer;
  background: var(--line-strong, #444); position: relative; transition: background .2s ease; }
.ec-switch.on { background: var(--orange, #e08a3c); }
.ec-knob { position: absolute; top: 3px; left: 3px; width: 20px; height: 20px; border-radius: 50%; background: #fff;
  transition: transform .2s cubic-bezier(.3,1.4,.5,1); box-shadow: 0 1px 3px rgba(0,0,0,0.4); }
.ec-switch.on .ec-knob { transform: translateX(20px); }
.ec-section { border: none; margin: 0; padding: 16px 0 0; min-inline-size: auto; transition: opacity .25s ease; }
.ec-run { display: flex; align-items: center; gap: 12px; margin-bottom: 18px; }
.ec-run span { font-size: 0.78rem; color: var(--muted, #999); }
.ec-field { margin-bottom: 14px; }
.ec-field > label { display: block; margin-bottom: 8px; font-weight: 600; font-size: 0.9rem; color: var(--ink, #ddd); }
.ec-inline { display: grid; grid-template-columns: 160px 1fr; align-items: center; gap: 14px; }
.ec-inline > label { margin-bottom: 0; }
.ec-pills { display: flex; gap: 8px; flex-wrap: wrap; }
.ec-pill { display: inline-flex; align-items: center; gap: 6px; padding: 7px 13px; border-radius: 999px; cursor: pointer;
  color: inherit; border: 1.5px solid var(--line-strong, #3a3a3a); background: var(--surface-sunk, rgba(255,255,255,0.02));
  font-size: 0.85rem; transition: border-color .15s, background .15s, transform .15s; }
.ec-pill:hover:not(:disabled) { border-color: var(--orange, #e08a3c); transform: translateY(-1px); }
.ec-pill.on { border-color: var(--orange, #e08a3c); background: var(--fill-amber, rgba(224,138,60,0.12)); color: var(--ink, #fff); font-weight: 600; }
.ec-pill.off { opacity: 0.4; cursor: not-allowed; }
.ec-info { position: relative; font-style: normal; font-size: 0.72rem; opacity: 0.55; cursor: help; }
.ec-info:hover { opacity: 1; }
.ec-info:hover::after { content: attr(data-tip); position: absolute; bottom: 150%; left: 50%; transform: translateX(-50%);
  width: 240px; padding: 8px 10px; border-radius: 8px; background: var(--surface, #222); color: var(--ink, #eee);
  border: 1px solid var(--line-strong, #444); font-size: 0.76rem; font-weight: 400; line-height: 1.4; z-index: 20;
  box-shadow: 0 8px 24px rgba(0,0,0,0.35); white-space: normal; text-align: left; }
.ec-input { width: 100%; max-width: 280px; padding: 8px 10px; border-radius: 8px; border: 1.5px solid var(--line-strong, #3a3a3a);
  background: var(--surface-sunk, rgba(255,255,255,0.02)); color: inherit; }
.ec-input:focus { outline: none; border-color: var(--orange, #e08a3c); }
.ec-textarea { width: 100%; padding: 10px 12px; border-radius: 8px; border: 1.5px solid var(--line-strong, #3a3a3a);
  background: var(--surface-sunk, rgba(255,255,255,0.02)); color: inherit; font-family: var(--mono, monospace); font-size: 0.8rem; line-height: 1.5; resize: vertical; }
.ec-textarea:focus { outline: none; border-color: var(--orange, #e08a3c); }
@media (max-width: 640px) { .ec-inline { grid-template-columns: 1fr; align-items: start; } }
`
