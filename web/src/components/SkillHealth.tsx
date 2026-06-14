import { useTranslation } from 'react-i18next';
import type { SkillStats } from '../api';

// SkillHealth exibe os indicadores de saúde de uma skill (spec §4.2):
// taxa de sucesso, tendência e usos. Componente puramente de exibição.
export default function SkillHealth({ stats }: { stats: SkillStats | undefined }) {
  const { t } = useTranslation();
  if (!stats || stats.total_uses === 0) {
    return <span style={{ color: 'var(--muted)', fontSize: '0.8rem' }}>{t('skills.noUsage')}</span>;
  }
  const rate = Math.round(stats.success_rate * 100);
  const trendKey =
    stats.trend === 'improving' ? 'skills.trendImproving'
      : stats.trend === 'degrading' ? 'skills.trendDegrading'
        : 'skills.trendStable';
  const trendColor =
    stats.trend === 'improving' ? 'var(--green)'
      : stats.trend === 'degrading' ? 'var(--red)'
        : 'var(--muted)';
  return (
    <span style={{ fontSize: '0.8rem', display: 'inline-flex', gap: '0.5rem', alignItems: 'center' }}>
      <span className={`pill ${rate >= 70 ? 'green' : rate >= 40 ? 'amber' : 'pink'}`} title={t('skills.successRate') as string}>{rate}%</span>
      <span style={{ color: trendColor, fontWeight: 600 }} title={t('skills.trend') as string}>{t(trendKey)}</span>
      <span style={{ color: 'var(--muted)' }}>({stats.total_uses})</span>
    </span>
  );
}
