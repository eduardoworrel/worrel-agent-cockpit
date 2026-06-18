import { useTranslation } from 'react-i18next';
import { FanMark } from '../components/Fan';

interface Props {
  onNewSession: () => void;
}

// EmptyState é a tela de primeiro uso: sem sidebar, sem drawer. Uma decisão.
export default function EmptyState({ onNewSession }: Props) {
  const { t } = useTranslation();
  return (
    <div className="empty-state">
      <FanMark size={36} />
      <div className="empty-state-copy">
        <h1>{t('onboarding.title')}</h1>
        <p>{t('onboarding.subtitle')}</p>
      </div>
      <div className="empty-state-actions">
        <button className="btn btn-primary" onClick={onNewSession}>
          {t('onboarding.startSession')}
        </button>
      </div>
      <p className="empty-state-hint">{t('onboarding.hint')}</p>
    </div>
  );
}
