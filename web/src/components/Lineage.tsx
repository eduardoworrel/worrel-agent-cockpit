import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { listGenerations, revertGeneration } from '../api';
import type { SkillGeneration } from '../api';
import { FanLineage } from './Fan';

// Lineage renderiza a árvore de gerações de uma skill (spec §7): por nó, tipo,
// resumo, autoria, diff e evidência, com ação de reverter em um clique.
export default function Lineage({ skillId, activeGeneration, onReverted }: { skillId: string; activeGeneration?: number; onReverted?: () => void }) {
  const { t } = useTranslation();
  const [gens, setGens] = useState<SkillGeneration[]>([]);
  // A geração ativa vem do skill (skills.active_generation) via prop — o endpoint
  // /generations não devolve flag de ativa. Após reverter, atualizamos localmente.
  const [active, setActive] = useState<number>(activeGeneration ?? 0);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (typeof activeGeneration === 'number') setActive(activeGeneration);
  }, [activeGeneration]);

  async function load() {
    const g = await listGenerations(skillId);
    setGens(g);
  }
  useEffect(() => { load(); /* eslint-disable-next-line */ }, [skillId]);

  function typeLabel(tp: string): string {
    const key = `suggestions.suggestionType.skill_${tp}`;
    const label = t(key);
    return label.includes('suggestionType') ? tp : label;
  }

  function authorshipLabel(a: string): string {
    if (a === 'engine_auto') return t('lineage.authorshipEngineAuto');
    if (a === 'engine_approved') return t('lineage.authorshipEngineApproved');
    return t('lineage.authorshipHuman');
  }

  async function handleRevert(generation: number) {
    if (busy) return;
    setBusy(true);
    try {
      const sk = await revertGeneration(skillId, generation);
      setActive(sk.active_generation);
      await load();
      onReverted?.();
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="lineage">
      <h4>{t('lineage.title')}</h4>
      {gens.length > 0 && (
        <div className="lineage-head">
          <FanLineage
            gens={gens.map((g) => ({
              generation: g.generation,
              evolution_type: g.evolution_type,
              active: g.generation === active,
            }))}
          />
        </div>
      )}
      {gens.length === 0 && <p style={{ color: 'var(--muted)' }}>—</p>}
      <ol style={{ listStyle: 'none', padding: 0, margin: 0 }}>
        {gens.map((g) => (
          <li key={g.id} className={`card${g.generation === active ? ' active-generation' : ''}`} style={{ marginBottom: '0.5rem', padding: '0.6rem' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '0.5rem' }}>
              <span>
                <strong>{t('lineage.generation')} {g.generation}</strong>{' '}
                {g.generation === active && <span className="badge">{t('lineage.activeBadge')}</span>}{' '}
                <span className="pill" data-type={g.evolution_type}>{typeLabel(g.evolution_type)}</span>{' '}
                <span style={{ fontSize: '0.8rem', color: 'var(--muted)' }}>{authorshipLabel(g.authorship)}</span>
              </span>
              <button className="btn btn-secondary" disabled={busy} onClick={() => handleRevert(g.generation)}>
                {t('lineage.revert')}
              </button>
            </div>
            {g.change_summary && <p style={{ fontSize: '0.85rem', margin: '0.4rem 0 0' }}>{g.change_summary}</p>}
            {g.parent_skill_ids && g.parent_skill_ids.length > 0 && (
              <p style={{ fontSize: '0.78rem', color: 'var(--muted)', margin: '0.2rem 0 0' }}>
                {t('lineage.parents')}: {g.parent_skill_ids.join(', ')}
              </p>
            )}
            {g.evidence && (
              <p style={{ fontSize: '0.78rem', color: 'var(--muted)', margin: '0.2rem 0 0' }}>
                {t('lineage.evidence')}: {g.evidence}
              </p>
            )}
            {g.diff && (
              <pre className="mono" style={{ fontSize: '0.74rem', background: 'var(--surface-warm)', border: '1px solid var(--line)', color: 'var(--ink-soft)', padding: '8px 10px', borderRadius: 'var(--r-sm)', whiteSpace: 'pre-wrap', margin: '6px 0 0' }}>{g.diff}</pre>
            )}
          </li>
        ))}
      </ol>
      {active > 0 && <p style={{ fontSize: '0.8rem', color: 'var(--muted)' }}>{t('lineage.activeNow', { n: active })}</p>}
    </div>
  );
}
