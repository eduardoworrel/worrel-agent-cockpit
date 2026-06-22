import { useEffect, useState } from 'react'
import EngineCard, { type EngineItem } from './EngineCard'

export default function OnboardingWizard({ onClose }: { onClose: () => void }) {
  const [items, setItems] = useState<EngineItem[]>([])
  const [step, setStep] = useState(0)

  const load = () => fetch('/api/engines').then(r => r.json()).then(setItems).catch(() => setItems([]))
  useEffect(() => { load() }, [])

  const setConfig = (id: string, key: string, value: string) =>
    fetch(`/api/engines/${id}/config`, {
      method: 'PUT', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key, value }),
    }).then(load)

  // passos: 0 = boas-vindas; 1..N = um motor; N+1 = resumo
  const total = items.length + 2
  const isWelcome = step === 0
  const isSummary = step === items.length + 1
  const engine = !isWelcome && !isSummary ? items[step - 1] : undefined

  return (
    <div className="onboarding-overlay" style={{ position: 'fixed', inset: 0, background: 'var(--bg)', overflow: 'auto', zIndex: 50, padding: '2rem' }}>
      <div style={{ maxWidth: '800px', margin: '0 auto' }}>
        <div style={{ color: 'var(--muted)', fontSize: '0.85rem' }}>Passo {step + 1} de {total}</div>
        {isWelcome && (
          <div className="card">
            <h1>Bem-vindo ao worrel</h1>
            <p>O worrel observa suas sessões e destila conhecimento reutilizável (memória, skills, agentes) usando motores que você ativa e configura. Vamos configurar os motores.</p>
          </div>
        )}
        {engine && (
          <div>
            <h2>{engine.spec.name}</h2>
            <EngineCard item={engine} setConfig={setConfig} />
          </div>
        )}
        {isSummary && (
          <div className="card">
            <h2>Tudo pronto</h2>
            <ul>
              {items.map(it => (
                <li key={it.spec.id}>{it.spec.name}: {it.config['__enabled'] === 'true' ? 'ativo' : 'desligado'}</li>
              ))}
            </ul>
          </div>
        )}
        <div style={{ display: 'flex', gap: '0.5rem', marginTop: '1rem' }}>
          {step > 0 && <button className="btn btn-secondary" onClick={() => setStep(step - 1)}>Voltar</button>}
          {step < total - 1 && <button className="btn btn-primary" onClick={() => setStep(step + 1)}>Avançar</button>}
          {isSummary && <button className="btn btn-primary" onClick={onClose}>Concluir</button>}
          <button className="btn btn-secondary" style={{ marginLeft: 'auto' }} onClick={onClose}>Pular</button>
        </div>
      </div>
    </div>
  )
}
