import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { answerSecretApproval } from '../api';

interface Props {
  requestId: string;
  secretName: string;
  onDone: () => void;
}

export default function SecretApprovalModal({ requestId, secretName, onDone }: Props) {
  const { t } = useTranslation();
  const denyRef = useRef<HTMLButtonElement>(null);

  async function handle(approved: boolean) {
    try {
      await answerSecretApproval(requestId, approved);
    } catch { /* ignore - may have timed out */ }
    onDone();
  }

  // Foco inicial seguro (negar) + fechar no Esc (equivale a negar).
  useEffect(() => {
    denyRef.current?.focus();
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') handle(false);
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="modal-overlay">
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="secret-approval-title"
      >
        <h3 id="secret-approval-title" style={{ marginTop: 0 }}>{t('secrets.approvalTitle')}</h3>
        <p>{t('secrets.approvalMsg')}</p>
        <p style={{ fontFamily: 'monospace', fontWeight: 'bold', fontSize: '1.1rem' }}>{secretName}</p>
        <div style={{ display: 'flex', gap: '1rem', marginTop: '1.5rem' }}>
          <button className="btn btn-primary" onClick={() => handle(true)} style={{ flex: 1 }}>
            {t('secrets.approve')}
          </button>
          <button ref={denyRef} className="btn btn-danger" onClick={() => handle(false)} style={{ flex: 1 }}>
            {t('secrets.deny')}
          </button>
        </div>
      </div>
    </div>
  );
}
