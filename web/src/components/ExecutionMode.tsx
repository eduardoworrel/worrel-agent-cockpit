// Seletor premium do modo de execução de um motor (grava em __trigger).
// Cards com "arte" animada por modo + elemento de tempo. Estilos embutidos
// (escopados por .exmode-*) para não depender de styles.css.

import type { ReactNode } from 'react'

type Mode = {
  value: string
  label: string
  desc: string
  soon?: boolean
  art: ReactNode
}

const MODES: Mode[] = [
  {
    value: 'project_open_close',
    label: 'Ao encerrar a sessão',
    desc: 'Destila quando a sessão termina e, ao abrir o app, recupera as que fecharam sem análise.',
    art: (
      <div className="exmode-art exmode-clock" aria-hidden>
        <span className="exmode-clock-face"><i className="exmode-clock-hand" /></span>
        <em>~2 min</em>
      </div>
    ),
  },
  {
    value: 'realtime',
    label: 'Ao vivo',
    desc: 'Acompanha o stream e destila durante a sessão, em tempo real.',
    soon: true,
    art: (
      <div className="exmode-art exmode-live" aria-hidden>
        <span className="exmode-live-dot" />
        <span className="exmode-live-ring" />
        <span className="exmode-live-ring exmode-live-ring2" />
      </div>
    ),
  },
  {
    value: 'agent_self',
    label: 'O agente decide',
    desc: 'Injeta a regra no início; o próprio agente registra via MCP quando percebe algo.',
    soon: true,
    art: (
      <div className="exmode-art exmode-agent" aria-hidden>
        <span className="exmode-spark" />
        <span className="exmode-spark exmode-spark2" />
        <span className="exmode-spark exmode-spark3" />
      </div>
    ),
  },
  {
    value: 'on_demand',
    label: 'Sob demanda',
    desc: 'Nada automático: roda só quando você clica em “Rodar”.',
    art: (
      <div className="exmode-art exmode-manual" aria-hidden>
        <span className="exmode-play" />
      </div>
    ),
  },
]

export default function ExecutionMode({ value, onChange, allowed }: {
  value: string
  onChange: (v: string) => void
  allowed?: string[] // valores realmente suportados pelo motor; os demais ficam "em breve"
}) {
  return (
    <div className="exmode">
      <style>{EXMODE_CSS}</style>
      <div className="exmode-grid">
        {MODES.map(m => {
          const supported = !allowed || allowed.includes(m.value)
          const disabled = m.soon || !supported
          const on = value === m.value
          return (
            <button
              key={m.value}
              type="button"
              className={`exmode-opt${on ? ' on' : ''}${disabled ? ' soon' : ''}`}
              disabled={disabled}
              onClick={() => !disabled && onChange(m.value)}
            >
              {m.art}
              <b>{m.label}</b>
              <span>{m.desc}</span>
              {disabled && <span className="exmode-badge">em breve</span>}
            </button>
          )
        })}
      </div>
    </div>
  )
}

const EXMODE_CSS = `
.exmode-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.exmode-opt {
  position: relative; display: flex; flex-direction: column; align-items: flex-start; gap: 6px;
  text-align: left; padding: 14px; border-radius: 10px; cursor: pointer;
  border: 1.5px solid var(--line-strong, #3a3a3a); background: var(--surface-sunk, rgba(255,255,255,0.02));
  color: inherit; transition: border-color .18s ease, background .18s ease, transform .18s ease, box-shadow .18s ease;
}
.exmode-opt:hover:not(:disabled) { border-color: var(--orange, #e08a3c); background: var(--fill-amber, rgba(224,138,60,0.08)); transform: translateY(-2px); box-shadow: 0 6px 18px rgba(0,0,0,0.18); }
.exmode-opt.on { border-color: var(--orange, #e08a3c); background: var(--fill-amber, rgba(224,138,60,0.10)); box-shadow: inset 0 0 0 1px var(--orange, #e08a3c); }
.exmode-opt.soon { opacity: 0.6; cursor: not-allowed; }
.exmode-opt b { color: var(--ink, #eee); font-size: 0.95rem; }
.exmode-opt span:not(.exmode-badge) { font-size: 0.78rem; color: var(--muted, #999); line-height: 1.35; }
.exmode-badge {
  position: absolute; top: 10px; right: 10px; font-size: 0.62rem; letter-spacing: .04em; text-transform: uppercase;
  padding: 2px 7px; border-radius: 999px; background: var(--line-strong, #444); color: var(--ink, #ddd);
}
.exmode-art { width: 100%; height: 46px; display: flex; align-items: center; gap: 10px; margin-bottom: 2px; }

/* relógio (frequência) */
.exmode-clock { }
.exmode-clock-face { position: relative; width: 30px; height: 30px; border-radius: 50%; border: 2px solid var(--orange, #e08a3c); display: inline-block; }
.exmode-clock-hand { position: absolute; left: 50%; top: 50%; width: 2px; height: 10px; background: var(--orange, #e08a3c); transform-origin: bottom center; transform: translate(-50%,-100%); animation: exmode-spin 3s linear infinite; border-radius: 2px; }
.exmode-clock em { font-style: normal; font-size: 0.8rem; font-weight: 600; color: var(--orange, #e08a3c); }
@keyframes exmode-spin { to { transform: translate(-50%,-100%) rotate(360deg); } }

/* ao vivo (pulso) */
.exmode-live { position: relative; }
.exmode-live-dot { position: absolute; left: 14px; top: 50%; width: 12px; height: 12px; margin-top: -6px; border-radius: 50%; background: #e0483c; box-shadow: 0 0 8px #e0483c; }
.exmode-live-ring, .exmode-live-ring2 { position: absolute; left: 8px; top: 50%; width: 24px; height: 24px; margin-top: -12px; border-radius: 50%; border: 2px solid #e0483c; opacity: 0; animation: exmode-ping 1.8s ease-out infinite; }
.exmode-live-ring2 { animation-delay: .9s; }
@keyframes exmode-ping { 0% { transform: scale(.4); opacity: .7; } 100% { transform: scale(1.6); opacity: 0; } }

/* agente (faíscas) */
.exmode-agent { position: relative; }
.exmode-spark { position: absolute; top: 50%; left: 16px; width: 7px; height: 7px; border-radius: 50%; background: var(--orange, #e08a3c); animation: exmode-twinkle 1.6s ease-in-out infinite; }
.exmode-spark2 { left: 30px; top: 35%; animation-delay: .4s; }
.exmode-spark3 { left: 24px; top: 70%; animation-delay: .8s; }
@keyframes exmode-twinkle { 0%,100% { transform: scale(.5); opacity: .3; } 50% { transform: scale(1.2); opacity: 1; } }

/* manual (play) */
.exmode-manual { }
.exmode-play { width: 0; height: 0; border-style: solid; border-width: 9px 0 9px 15px; border-color: transparent transparent transparent var(--orange, #e08a3c); transition: transform .18s ease; }
.exmode-opt:hover .exmode-play { transform: scale(1.18); }

@media (max-width: 640px) { .exmode-grid { grid-template-columns: 1fr; } }
`
