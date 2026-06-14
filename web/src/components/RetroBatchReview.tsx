import { useEffect, useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { batchView, bulkResolve, type ProjectGroup, type Suggestion } from '../retroApi';

interface Props {
  runId: string;
}

function occurrences(s: Suggestion): number {
  try {
    const pl = JSON.parse(s.payload || '{}');
    return typeof pl.occurrences === 'number' ? pl.occurrences : 1;
  } catch {
    return 1;
  }
}

// Para secret.detected, jamais renderizar valor cru — apenas a evidência mascarada.
function displayEvidence(s: Suggestion): string {
  return s.evidence;
}

export default function RetroBatchReview({ runId }: Props) {
  const { t } = useTranslation();
  const [groups, setGroups] = useState<ProjectGroup[]>([]);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setGroups(await batchView(runId));
    } catch {
      /* noop */
    }
  }, [runId]);

  useEffect(() => {
    load();
  }, [load]);

  async function bulk(projectId: string, type: string, action: string) {
    const confirmKey = action === 'accepted' ? 'retro.batch.confirmAcceptAll' : 'retro.batch.confirmRejectAll';
    if (!window.confirm(t(confirmKey, { type }))) return;
    setBusy(true);
    try {
      await bulkResolve(runId, projectId, type, action);
      await load();
    } finally {
      setBusy(false);
    }
  }

  if (groups.length === 0) {
    return <p className="muted">{t('retro.batch.empty')}</p>;
  }

  return (
    <div className="retro-batch">
      <h3>{t('retro.batch.title')}</h3>
      {groups.map((pg) => (
        <div key={pg.project_id} className="retro-batch-project">
          <h4>{t('retro.batch.project')}: {pg.project_id}</h4>
          {pg.groups.map((tg) => (
            <div key={tg.type} className="retro-batch-type">
              <div className="retro-batch-type-header">
                <strong>{tg.type}</strong> <span className="muted">({tg.items.length})</span>
                <div className="retro-batch-actions">
                  <button className="btn btn-primary" disabled={busy} onClick={() => bulk(pg.project_id, tg.type, 'accepted')}>
                    {t('retro.batch.acceptAll')}
                  </button>
                  <button className="btn btn-danger" disabled={busy} onClick={() => bulk(pg.project_id, tg.type, 'rejected')}>
                    {t('retro.batch.rejectAll')}
                  </button>
                </div>
              </div>
              <ul>
                {tg.items.map((it) => (
                  <li key={it.id}>
                    <span className="retro-item-title">{it.title}</span>
                    {occurrences(it) > 1 && (
                      <span className="badge" title={t('retro.batch.occurrences')}>×{occurrences(it)}</span>
                    )}
                    <div className="retro-item-evidence">{displayEvidence(it)}</div>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}
