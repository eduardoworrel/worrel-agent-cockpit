import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { listDirs } from '../api';
import type { DirListing } from '../api';

interface Props {
  value: string[];
  onChange: (next: string[]) => void;
}

export default function FolderPicker({ value, onChange }: Props) {
  const { t } = useTranslation();
  const [listing, setListing] = useState<DirListing | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function navigate(path?: string) {
    setLoading(true);
    setError(null);
    listDirs(path)
      .then((l) => setListing(l))
      .catch((e) => setError(e instanceof Error ? e.message : t('common.actionFailed')))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    navigate(undefined);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function add(path: string) {
    if (!value.includes(path)) onChange([...value, path]);
  }

  function remove(path: string) {
    onChange(value.filter((p) => p !== path));
  }

  return (
    <div className="folder-picker">
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '0.5rem',
          marginBottom: '0.5rem',
          flexWrap: 'wrap',
        }}
      >
        <button
          type="button"
          className="btn btn-secondary"
          disabled={loading || !listing?.parent}
          onClick={() => navigate(listing?.parent)}
        >
          {t('folderPicker.up')}
        </button>
        <span className="mono muted" style={{ fontSize: '0.8rem', wordBreak: 'break-all' }}>
          {listing?.path ?? '…'}
        </span>
      </div>

      {error && <p className="error-banner">{error}</p>}

      <div
        style={{
          border: '1px solid var(--border, #ccc)',
          borderRadius: 6,
          maxHeight: 200,
          overflowY: 'auto',
          marginBottom: '0.5rem',
        }}
      >
        {loading ? (
          <p className="muted" style={{ padding: '0.5rem' }}>{t('common.loading')}</p>
        ) : (listing?.entries.length ?? 0) === 0 ? (
          <p className="muted" style={{ padding: '0.5rem' }}>{t('folderPicker.empty')}</p>
        ) : (
          listing!.entries.map((e) => (
            <div
              key={e.path}
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                gap: '0.5rem',
                padding: '0.25rem 0.5rem',
              }}
            >
              <button
                type="button"
                className="btn-link"
                onClick={() => navigate(e.path)}
                style={{
                  background: 'none',
                  border: 'none',
                  cursor: 'pointer',
                  color: 'var(--ink)',
                  textAlign: 'left',
                  flex: 1,
                  padding: 0,
                }}
                title={e.path}
              >
                📁 {e.name}
              </button>
              <button
                type="button"
                className="btn btn-secondary"
                disabled={value.includes(e.path)}
                onClick={() => add(e.path)}
              >
                {t('folderPicker.add')}
              </button>
            </div>
          ))
        )}
      </div>

      {listing?.path && (
        <button
          type="button"
          className="btn btn-secondary"
          disabled={value.includes(listing.path)}
          onClick={() => add(listing.path)}
          style={{ marginBottom: '0.5rem' }}
        >
          {t('folderPicker.addCurrent')}
        </button>
      )}

      {value.length > 0 && (
        <div>
          <label style={{ display: 'block', marginBottom: '0.25rem', fontSize: '0.85rem' }}>
            {t('folderPicker.linked')}
          </label>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '0.25rem' }}>
            {value.map((p) => (
              <div
                key={p}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  gap: '0.5rem',
                }}
              >
                <span className="mono" style={{ fontSize: '0.8rem', wordBreak: 'break-all' }}>{p}</span>
                <button
                  type="button"
                  className="btn btn-danger"
                  onClick={() => remove(p)}
                  aria-label={t('folderPicker.remove')}
                >
                  ✕
                </button>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
