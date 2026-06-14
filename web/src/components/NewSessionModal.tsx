import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { listAdapters, createFreeSession, createSession } from '../api';
import type { DetectedAdapter, Session } from '../api';
import FolderPicker from './FolderPicker';

interface Props {
  /** When provided, creates a project-scoped session via POST /api/projects/:projectId/sessions */
  projectId?: string;
  /** Markdown content (skill/pipeline) injected into the session primer. */
  skill?: string;
  /** Human label shown as "Starting with: {label}". */
  skillLabel?: string;
  onCreated: (sess: Session) => void;
  onClose: () => void;
}

export default function NewSessionModal({ projectId, skill, skillLabel, onCreated, onClose }: Props) {
  const { t } = useTranslation();
  const [adapters, setAdapters] = useState<DetectedAdapter[]>([]);
  const [adapterId, setAdapterId] = useState('');
  const [dirs, setDirs] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const firstRef = useRef<HTMLSelectElement>(null);

  useEffect(() => {
    listAdapters()
      .then((all) => {
        const present = all.filter((a) => a.installed.present);
        setAdapters(present);
        if (present[0]) setAdapterId(present[0].id);
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    firstRef.current?.focus();
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose();
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose]);

  async function handleCreate() {
    if (!adapterId || busy) return;
    setBusy(true);
    setError(null);
    try {
      let sess: Session;
      if (projectId) {
        sess = await createSession(projectId, adapterId, skill);
      } else {
        sess = await createFreeSession(adapterId, dirs.length > 0 ? dirs : undefined, skill);
      }
      onCreated(sess);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.actionFailed'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="modal-overlay">
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="new-session-modal-title"
      >
        <h3 id="new-session-modal-title" style={{ marginTop: 0 }}>
          {t('sessions.new')}
        </h3>

        {skillLabel && (
          <p
            className="pill"
            style={{ display: 'inline-block', margin: '0 0 0.75rem' }}
          >
            {t('sessions.startingWith', { defaultValue: 'Starting with: {{label}}', label: skillLabel })}
          </p>
        )}

        {error && <p className="error-banner">{error}</p>}

        {adapters.length === 0 ? (
          <p style={{ color: 'var(--muted)' }}>{t('sessions.noCLI')}</p>
        ) : (
          <>
            <label htmlFor="nsm-adapter" style={{ display: 'block', marginBottom: '0.25rem' }}>
              {t('sessions.adapter')}
            </label>
            <select
              id="nsm-adapter"
              ref={firstRef}
              value={adapterId}
              onChange={(e) => setAdapterId(e.target.value)}
              style={{ width: '100%', marginBottom: '1rem' }}
            >
              {adapters.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.id} {a.installed.version}
                </option>
              ))}
            </select>

            {!projectId && (
              <>
                <label style={{ display: 'block', marginBottom: '0.25rem' }}>
                  {t('sessions.linkDirs')}
                </label>
                <div style={{ marginBottom: '1rem' }}>
                  <FolderPicker value={dirs} onChange={setDirs} />
                </div>
              </>
            )}
          </>
        )}

        <div style={{ display: 'flex', gap: '0.75rem', marginTop: '0.5rem' }}>
          <button
            className="btn btn-primary"
            disabled={busy || !adapterId}
            onClick={handleCreate}
            style={{ flex: 1 }}
          >
            {busy ? t('common.loading') : t('common.create')}
          </button>
          <button className="btn btn-secondary" onClick={onClose} style={{ flex: 1 }}>
            {t('common.cancel')}
          </button>
        </div>
      </div>
    </div>
  );
}
