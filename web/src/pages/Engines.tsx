import { useEffect, useState } from 'react'
import EngineCard, { type EngineItem } from '../components/EngineCard'
import OnboardingWizard from '../components/OnboardingWizard'
import { listProjects, listSessions } from '../api'

export default function Engines() {
  const [items, setItems] = useState<EngineItem[]>([])
  const [msg, setMsg] = useState('')
  const [showWizard, setShowWizard] = useState(false)

  const load = () =>
    fetch('/api/engines').then(r => r.json()).then(setItems).catch(() => setItems([]))
  useEffect(() => { load() }, [])

  useEffect(() => {
    if (localStorage.getItem('worrel.onboarding.seen') === '1') return
    Promise.all([listProjects().catch(() => []), listSessions().catch(() => [])])
      .then(([projs, sess]) => {
        const empty = projs.length === 0 && sess.filter(s => s.mode === 'wrapper').length === 0
        if (empty) setShowWizard(true)
      })
  }, [])

  const closeWizard = () => {
    localStorage.setItem('worrel.onboarding.seen', '1')
    setShowWizard(false)
    load()
  }

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

  if (showWizard) return <OnboardingWizard onClose={closeWizard} />

  return (
    <div className="main">
      <div className="page-head">
        <div><h1>Motores</h1></div>
        <button className="btn btn-secondary" onClick={() => setShowWizard(true)}>Configurar motores</button>
      </div>
      {msg && <p style={{ color: 'var(--green)' }}>{msg}</p>}
      {items.map(it => (
        <EngineCard key={it.spec.id} item={it} setConfig={setConfig} onRun={run} />
      ))}
    </div>
  )
}
