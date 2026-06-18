import { useEffect, useState } from 'react'

type ConfigField = { key: string; label: string; type: string; default: string }
type Spec = {
  id: string; name: string; description: string
  triggers: string[]; prompts: ConfigField[]; config: ConfigField[]
  output_type: string; default_on: boolean
}
type EngineItem = { spec: Spec; config: Record<string, string> }

export default function Engines() {
  const [items, setItems] = useState<EngineItem[]>([])
  const [msg, setMsg] = useState('')

  const load = () =>
    fetch('/api/engines').then(r => r.json()).then(setItems).catch(() => setItems([]))
  useEffect(() => { load() }, [])

  const setConfig = (id: string, key: string, value: string) =>
    fetch(`/api/engines/${id}/config`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key, value }),
    }).then(load)

  const run = (id: string) =>
    fetch(`/api/engines/${id}/run`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ project_id: '', session_id: prompt('session_id?') || '' }),
    }).then(r => r.json()).then(() => setMsg(`Rodou ${id}`)).catch((e: unknown) => setMsg(String(e)))

  return (
    <div className="main">
      <div className="page-head"><div><h1>Motores</h1></div></div>
      {msg && <p style={{ color: 'var(--green)' }}>{msg}</p>}
      {items.map(({ spec, config }) => (
        <div key={spec.id} className="card" style={{ maxWidth: '760px', marginBottom: '1rem' }}>
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
          {spec.triggers.length > 0 && (
            <div style={{ marginBottom: '0.75rem' }}>
              <label>Gatilho:{' '}
                <select
                  value={config['__trigger'] ?? spec.triggers[0]}
                  onChange={e => setConfig(spec.id, '__trigger', e.target.value)}
                >
                  {spec.triggers.map(t => <option key={t} value={t}>{t}</option>)}
                </select>
              </label>
            </div>
          )}
          {spec.config.map(f => (
            <div key={f.key} style={{ marginBottom: '0.5rem' }}>
              <label style={{ display: 'block', marginBottom: '0.2rem' }}>{f.label}</label>
              <input
                defaultValue={config[f.key] ?? f.default}
                onBlur={e => setConfig(spec.id, f.key, e.target.value)}
              />
            </div>
          ))}
          {spec.prompts.map(f => (
            <div key={f.key} style={{ marginBottom: '0.75rem' }}>
              <label style={{ display: 'block', marginBottom: '0.2rem' }}>{f.label}</label>
              <textarea
                defaultValue={config[f.key] ?? f.default}
                onBlur={e => setConfig(spec.id, f.key, e.target.value)}
                rows={4} style={{ width: '100%', fontFamily: 'var(--mono)', fontSize: '0.8rem' }}
              />
            </div>
          ))}
          <button className="btn btn-primary" onClick={() => run(spec.id)}>Rodar sob demanda</button>
        </div>
      ))}
    </div>
  )
}
