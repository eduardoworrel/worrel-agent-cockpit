import ExecutionMode from './ExecutionMode'

export type ConfigOption = { value: string; label: string; description: string }
export type ConfigField = { key: string; label: string; type: string; default: string; options?: ConfigOption[] }
export type Spec = {
  id: string; name: string; description: string
  triggers: string[]; prompts: ConfigField[]; config: ConfigField[]
  output_type: string; default_on: boolean
}
export type EngineItem = { spec: Spec; config: Record<string, string> }

function OptionCards({ options, current, onSelect }: {
  options: { value: string; label: string; description: string }[]
  current: string
  onSelect: (value: string) => void
}) {
  return (
    <div style={{ display: 'grid', gap: '0.4rem' }}>
      {options.map(o => {
        const sel = current === o.value
        return (
          <button key={o.value} type="button" onClick={() => onSelect(o.value)}
            style={{
              textAlign: 'left', cursor: 'pointer', padding: '0.5rem 0.65rem', borderRadius: '6px',
              border: sel ? '2px solid var(--green)' : '1px solid var(--border, #444)',
              background: sel ? 'rgba(80,200,120,0.10)' : 'transparent',
              color: 'inherit',
            }}>
            <div style={{ fontWeight: 600 }}>{o.label}{sel ? ' ✓' : ''}</div>
            <div style={{ fontSize: '0.8rem', color: 'var(--muted)' }}>{o.description}</div>
          </button>
        )
      })}
    </div>
  )
}

export default function EngineCard({ item, setConfig, onRun }: {
  item: EngineItem
  setConfig: (id: string, key: string, value: string) => void
  onRun?: (id: string) => void
}) {
  const { spec, config } = item
  return (
    <div className="card" style={{ maxWidth: '760px', marginBottom: '1rem' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <strong>{spec.name}</strong>
        <label style={{ display: 'flex', alignItems: 'center', gap: '0.4rem' }}>
          <input
            type="checkbox"
            checked={config['__enabled'] === 'true'}
            onChange={e => setConfig(spec.id, '__enabled', e.target.checked ? 'true' : 'false')}
          /> ativo
        </label>
      </div>
      <p style={{ marginTop: '0.4rem', color: 'var(--muted)' }}>{spec.description}</p>

      <div style={{ marginBottom: '0.9rem' }}>
        <label style={{ display: 'block', marginBottom: '0.5rem', fontWeight: 600 }}>Quando executar</label>
        <ExecutionMode
          value={config['__trigger'] ?? spec.triggers[0]}
          allowed={spec.triggers}
          onChange={v => setConfig(spec.id, '__trigger', v)}
        />
      </div>

      {spec.config.map(f => {
        const current = config[f.key] ?? f.default
        return (
          <div key={f.key} style={{ marginBottom: '0.75rem' }}>
            <label style={{ display: 'block', marginBottom: '0.35rem' }}>{f.label}</label>
            {f.options && f.options.length > 0 ? (
              <OptionCards options={f.options} current={current} onSelect={v => setConfig(spec.id, f.key, v)} />
            ) : (
              <input defaultValue={current} onBlur={e => setConfig(spec.id, f.key, e.target.value)} />
            )}
          </div>
        )
      })}

      {spec.prompts.map(f => (
        <div key={f.key} style={{ marginBottom: '0.75rem' }}>
          <label style={{ display: 'block', marginBottom: '0.2rem' }}>{f.label}</label>
          <textarea defaultValue={config[f.key] ?? f.default} onBlur={e => setConfig(spec.id, f.key, e.target.value)}
            rows={4} style={{ width: '100%', fontFamily: 'var(--mono)', fontSize: '0.8rem' }} />
        </div>
      ))}
      {onRun && <button className="btn btn-primary" onClick={() => onRun(spec.id)}>Rodar sob demanda</button>}
    </div>
  )
}
