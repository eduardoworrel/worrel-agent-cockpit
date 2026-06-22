import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getEngineSettings, setEngineConfigValue, setEngineEnabled } from '../api';

// Harness selecionáveis — espelha internal/engine/engine.go:HarnessOptions.
const HARNESSES = [
  { value: '', label: 'Padrão' },
  { value: 'claude-code', label: 'Claude Code' },
  { value: 'opencode', label: 'opencode' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'codex', label: 'Codex' },
];

// CSS mínimo dos controles (.ec-*). Normalmente vem do EC_CSS do EngineCard, mas
// como HomeEngineConfig pode aparecer SEM um EngineCard na tela, garantimos as
// regras essenciais localmente para o componente ficar sempre utilizável.
const HEC_CSS = `
.ec-switch { flex: none; width: 46px; height: 26px; border-radius: 999px; border: none; cursor: pointer;
  background: var(--line-strong, #444); position: relative; transition: background .2s ease; }
.ec-switch.on { background: var(--orange, #e08a3c); }
.ec-knob { position: absolute; top: 3px; left: 3px; width: 20px; height: 20px; border-radius: 50%; background: #fff;
  transition: transform .2s cubic-bezier(.3,1.4,.5,1); box-shadow: 0 1px 3px rgba(0,0,0,0.4); }
.ec-switch.on .ec-knob { transform: translateX(20px); }
.ec-pills { display: flex; gap: 8px; flex-wrap: wrap; }
.ec-pill { display: inline-flex; align-items: center; gap: 6px; padding: 7px 13px; border-radius: 999px; cursor: pointer;
  color: inherit; border: 1.5px solid var(--line-strong, #3a3a3a); background: var(--surface-sunk, rgba(255,255,255,0.02));
  font-size: 0.85rem; transition: border-color .15s, background .15s, transform .15s; }
.ec-pill:hover:not(:disabled) { border-color: var(--orange, #e08a3c); transform: translateY(-1px); }
.ec-pill.on { border-color: var(--orange, #e08a3c); background: var(--fill-amber, rgba(224,138,60,0.12)); color: var(--ink, #fff); font-weight: 600; }
.ec-input { width: 100%; max-width: 280px; padding: 8px 10px; border-radius: 8px; border: 1.5px solid var(--line-strong, #3a3a3a);
  background: var(--surface-sunk, rgba(255,255,255,0.02)); color: inherit; }
.ec-input:focus { outline: none; border-color: var(--orange, #e08a3c); }
`;

function ModelPicker({ harness, current, onSelect }: { harness: string; current: string; onSelect: (v: string) => void }) {
  const [models, setModels] = useState<string[]>([]);
  useEffect(() => {
    const id = harness || 'claude-code';
    fetch(`/api/adapters/${id}/models`).then((r) => r.json())
      .then((d) => setModels(d.models || [])).catch(() => setModels([]));
  }, [harness]);
  return (
    <select className="ec-input" value={current} onChange={(e) => onSelect(e.target.value)}>
      <option value="">Padrão do harness</option>
      {models.map((m) => <option key={m} value={m}>{m}</option>)}
    </select>
  );
}

// HomeEngineConfig: um motor de IA da Home (summary/interpret) com on/off,
// harness, modelo e ciência de auditoria. Usado no onboarding e em Settings.
export default function HomeEngineConfig({ id, title, description, defaultOn }: {
  id: string; title: string; description: string; defaultOn: boolean;
}) {
  const { t } = useTranslation();
  const [enabled, setEnabled] = useState(defaultOn);
  const [harness, setHarness] = useState('');
  const [model, setModel] = useState('');

  useEffect(() => {
    getEngineSettings(id).then((s) => { setEnabled(s.enabled); setHarness(s.harness); setModel(s.model); }).catch(() => {});
  }, [id]);

  const toggle = (on: boolean) => { setEnabled(on); setEngineEnabled(id, on); };
  const pickHarness = (h: string) => { setHarness(h); setModel(''); setEngineConfigValue(id, 'harness', h); setEngineConfigValue(id, 'model', ''); };
  const pickModel = (m: string) => { setModel(m); setEngineConfigValue(id, 'model', m); };

  return (
    <div className="card" style={{ marginTop: '1rem' }}>
      <style>{HEC_CSS}</style>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: '1rem' }}>
        <div>
          <h3 style={{ margin: 0 }}>{title}</h3>
          <p style={{ margin: '0.3rem 0 0', color: 'var(--muted)', fontSize: '0.85rem' }}>{description}</p>
        </div>
        <button type="button" role="switch" aria-checked={enabled}
          className={`ec-switch${enabled ? ' on' : ''}`} onClick={() => toggle(!enabled)}>
          <span className="ec-knob" />
        </button>
      </div>
      <fieldset style={{ border: 'none', margin: 0, padding: '0.8rem 0 0', opacity: enabled ? 1 : 0.5 }} disabled={!enabled}>
        <div style={{ display: 'grid', gridTemplateColumns: '160px 1fr', gap: '0.8rem', alignItems: 'center', marginBottom: '0.6rem' }}>
          <label style={{ fontWeight: 600, fontSize: '0.9rem' }}>{t('aiCfg.harness', 'Harness')}</label>
          <div className="ec-pills">
            {HARNESSES.map((h) => (
              <button key={h.value} type="button" className={`ec-pill${harness === h.value ? ' on' : ''}`}
                onClick={() => pickHarness(h.value)}>{h.label}</button>
            ))}
          </div>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: '160px 1fr', gap: '0.8rem', alignItems: 'center' }}>
          <label style={{ fontWeight: 600, fontSize: '0.9rem' }}>{t('aiCfg.model', 'Modelo')}</label>
          <ModelPicker harness={harness} current={model} onSelect={pickModel} />
        </div>
      </fieldset>
      <p style={{ marginTop: '0.7rem', fontSize: '0.8rem', color: 'var(--muted)' }}>
        🔒 {t('aiCfg.audit', 'Toda execução desta IA é registrada (prompt e resposta) na aba Atividade — auditoria sempre ativa e não desligável.')}
      </p>
    </div>
  );
}
