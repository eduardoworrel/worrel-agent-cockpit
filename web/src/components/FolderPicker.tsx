import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { listDirs } from '../api';
import type { DirListing } from '../api';

interface Props {
  value: string[];
  onChange: (next: string[]) => void;
}

// Quebra um caminho absoluto em segmentos clicáveis relativos ao home.
// Devolve [{label, path}] do home (⌂) até a pasta atual.
function crumbsFor(listing: DirListing | null): { label: string; path: string }[] {
  if (!listing) return [];
  const { home, path } = listing;
  const out: { label: string; path: string }[] = [{ label: '⌂', path: home }];
  if (path === home || !path.startsWith(home)) return out;
  const rest = path.slice(home.length).replace(/^\/+/, '');
  let acc = home;
  for (const seg of rest.split('/').filter(Boolean)) {
    acc = `${acc}/${seg}`;
    out.push({ label: seg, path: acc });
  }
  return out;
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

  const crumbs = useMemo(() => crumbsFor(listing), [listing]);

  function toggle(path: string) {
    if (value.includes(path)) onChange(value.filter((p) => p !== path));
    else onChange([...value, path]);
  }

  const currentPicked = !!listing && value.includes(listing.path);
  const atHome = !!listing && listing.path === listing.home;

  return (
    <div className="fx">
      {/* barra de navegação: home · subir · trilha de migalhas */}
      <div className="fx-bar">
        <button
          type="button"
          className="fx-nav"
          title={t('folderPicker.home')}
          disabled={loading || atHome}
          onClick={() => navigate(listing?.home)}
        >⌂</button>
        <button
          type="button"
          className="fx-nav"
          title={t('folderPicker.up')}
          disabled={loading || !listing?.parent}
          onClick={() => navigate(listing?.parent)}
        >↑</button>
        <nav className="fx-crumbs" aria-label="caminho">
          {crumbs.map((c, i) => (
            <span key={c.path} className="fx-crumb-wrap">
              {i > 0 && <span className="fx-sep">/</span>}
              <button
                type="button"
                className="fx-crumb"
                disabled={loading || c.path === listing?.path}
                onClick={() => navigate(c.path)}
                title={c.path}
              >{c.label}</button>
            </span>
          ))}
        </nav>
      </div>

      {error && <p className="error-banner">{error}</p>}

      {/* lista de pastas — estilo explorador */}
      <div className="fx-list" role="listbox" aria-label={t('modal.dirs')}>
        {loading ? (
          <div className="fx-empty">{t('common.loading')}</div>
        ) : (listing?.entries.length ?? 0) === 0 ? (
          <div className="fx-empty">{t('folderPicker.empty')}</div>
        ) : (
          listing!.entries.map((e) => {
            const picked = value.includes(e.path);
            return (
              <div
                key={e.path}
                className={`fx-row${picked ? ' is-picked' : ''}`}
                onDoubleClick={() => navigate(e.path)}
              >
                <button
                  type="button"
                  className="fx-check"
                  aria-pressed={picked}
                  title={picked ? t('folderPicker.remove') : t('folderPicker.add')}
                  onClick={() => toggle(e.path)}
                >{picked ? '✓' : ''}</button>
                <button
                  type="button"
                  className="fx-name"
                  onClick={() => navigate(e.path)}
                  title={e.path}
                >
                  <span className="fx-icon">📁</span>
                  <span className="fx-label">{e.name}</span>
                </button>
                <span className="fx-enter" aria-hidden>›</span>
              </div>
            );
          })
        )}
      </div>

      {/* rodapé: selecionar a pasta atual + contagem */}
      <div className="fx-foot">
        <button
          type="button"
          className="btn btn-secondary btn-sm"
          disabled={!listing}
          onClick={() => listing && toggle(listing.path)}
        >
          {currentPicked ? t('folderPicker.removeCurrent') : t('folderPicker.addCurrent')}
        </button>
        <span className="fx-hint muted">{t('folderPicker.hint')}</span>
      </div>

      {/* pastas selecionadas */}
      {value.length > 0 && (
        <div className="fx-picked">
          <div className="fx-picked-head">{t('folderPicker.linked')} · {value.length}</div>
          <div className="fx-chips">
            {value.map((p) => (
              <span key={p} className="fx-chip" title={p}>
                <span className="fx-chip-path">{p}</span>
                <button
                  type="button"
                  className="fx-chip-x"
                  onClick={() => toggle(p)}
                  aria-label={t('folderPicker.remove')}
                >✕</button>
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
