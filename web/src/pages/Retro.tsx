import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { listRuns, type RetroRun } from '../retroApi';
import RetroWizard from '../components/RetroWizard';

export default function Retro() {
  const { t } = useTranslation();
  const [runs, setRuns] = useState<RetroRun[]>([]);

  useEffect(() => {
    listRuns().then(setRuns).catch(() => {});
  }, []);

  return (
    <div className="main retro-page">
      <div className="page-head"><div><h1>{t('retro.title')}</h1></div></div>
      <RetroWizard />

      <section className="retro-history" style={{ marginTop: 32 }}>
        <h2>{t('retro.history.title')}</h2>
        {runs.length === 0 ? (
          <p className="muted">{t('retro.history.empty')}</p>
        ) : (
          <table className="card" style={{ padding: 0, overflow: 'hidden' }}>
            <thead>
              <tr>
                <th>{t('retro.history.date')}</th>
                <th>{t('retro.history.status')}</th>
                <th>{t('retro.history.depth')}</th>
                <th>{t('retro.history.llmCalls')}</th>
              </tr>
            </thead>
            <tbody>
              {runs.map((r) => (
                <tr key={r.id}>
                  <td>{new Date(r.created_at).toLocaleString()}</td>
                  <td>{r.status}</td>
                  <td>{r.depth}</td>
                  <td>{r.llm_calls}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  );
}
