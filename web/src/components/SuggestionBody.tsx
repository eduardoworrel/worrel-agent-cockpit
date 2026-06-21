import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Suggestion, Skill } from '../api';

// SuggestionBody renders a suggestion's payload in a human, type-aware way.
// The payload is JSON; we parse defensively and fall back to the raw text if
// parsing fails or the type is unknown, so it never throws.

interface Props {
  sg: Suggestion;
  // Optional: known skills, used to diff a correction against the live content.
  skills?: Skill[];
}

const PREVIEW_LINES = 12;

// --- shared bits -----------------------------------------------------------

function fieldBoxStyle(): React.CSSProperties {
  return {
    background: 'var(--surface-warm)',
    border: '1px solid var(--line)',
    borderRadius: 'var(--r-sm)',
    padding: '10px 12px',
    margin: '8px 0 0',
  };
}

// ContentPreview shows pre-formatted text, clamped to ~12 lines with a fade
// and a "see more" toggle. Plain monospace; treats the text as light markdown
// (no heavy parsing — just preserves line breaks and looks intentional).
function ContentPreview({ text }: { text: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const lines = text.replace(/\r\n/g, '\n').split('\n');
  const clamp = lines.length > PREVIEW_LINES;
  const shown = open || !clamp ? text : lines.slice(0, PREVIEW_LINES).join('\n');

  return (
    <div style={{ margin: '8px 0 0' }}>
      <div style={{ position: 'relative' }}>
        <pre
          className="mono"
          style={{
            fontSize: '0.78rem',
            background: 'var(--surface-warm)',
            border: '1px solid var(--line)',
            color: 'var(--ink-soft)',
            padding: '10px 12px',
            borderRadius: 'var(--r-sm)',
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            margin: 0,
            maxHeight: open ? 'none' : undefined,
          }}
        >
          {shown}
        </pre>
        {clamp && !open && (
          <div
            aria-hidden
            style={{
              position: 'absolute',
              left: 1,
              right: 1,
              bottom: 1,
              height: 40,
              borderRadius: '0 0 var(--r-sm) var(--r-sm)',
              background: 'linear-gradient(to bottom, transparent, var(--surface-warm))',
              pointerEvents: 'none',
            }}
          />
        )}
      </div>
      {clamp && (
        <button
          type="button"
          className="btn btn-secondary"
          style={{ fontSize: '0.78rem', marginTop: 6, padding: '2px 10px' }}
          onClick={() => setOpen((v) => !v)}
        >
          {open ? t('suggestions.seeLess') : t('suggestions.seeMore')}
        </button>
      )}
    </div>
  );
}

function Label({ children }: { children: React.ReactNode }) {
  return (
    <div
      style={{
        fontSize: '0.72rem',
        textTransform: 'uppercase',
        letterSpacing: '0.06em',
        color: 'var(--muted)',
        fontWeight: 600,
        marginTop: 10,
      }}
    >
      {children}
    </div>
  );
}

function RawFallback({ text }: { text: string }) {
  return (
    <pre
      className="mono"
      style={{
        fontSize: '0.78rem',
        background: 'var(--surface-warm)',
        border: '1px solid var(--line)',
        color: 'var(--ink-soft)',
        padding: '10px 12px',
        borderRadius: 'var(--r-sm)',
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
        margin: '8px 0 0',
      }}
    >
      {text}
    </pre>
  );
}

// --- diff -------------------------------------------------------------------

type DiffRow = { kind: 'same' | 'add' | 'del'; text: string };

// lineDiff: a tiny LCS-based line diff (pure JS, no deps). Lines only in the
// new version are "add" (green); only in the old are "del" (pink).
function lineDiff(oldText: string, newText: string): DiffRow[] {
  const a = oldText.replace(/\r\n/g, '\n').split('\n');
  const b = newText.replace(/\r\n/g, '\n').split('\n');
  const n = a.length;
  const m = b.length;
  // LCS length table.
  const lcs: number[][] = Array.from({ length: n + 1 }, () => new Array(m + 1).fill(0));
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      lcs[i][j] = a[i] === b[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1]);
    }
  }
  const rows: DiffRow[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      rows.push({ kind: 'same', text: a[i] });
      i++;
      j++;
    } else if (lcs[i + 1][j] >= lcs[i][j + 1]) {
      rows.push({ kind: 'del', text: a[i] });
      i++;
    } else {
      rows.push({ kind: 'add', text: b[j] });
      j++;
    }
  }
  while (i < n) rows.push({ kind: 'del', text: a[i++] });
  while (j < m) rows.push({ kind: 'add', text: b[j++] });
  return rows;
}

function DiffView({ oldText, newText }: { oldText: string; newText: string }) {
  const { t } = useTranslation();
  const rows = useMemo(() => lineDiff(oldText, newText), [oldText, newText]);
  const adds = rows.filter((r) => r.kind === 'add').length;
  const dels = rows.filter((r) => r.kind === 'del').length;

  return (
    <div style={{ margin: '8px 0 0' }}>
      <div style={{ display: 'flex', gap: 12, fontSize: '0.74rem', marginBottom: 6 }}>
        <span style={{ color: 'var(--green)', fontWeight: 600 }}>+{adds} {t('suggestions.added')}</span>
        <span style={{ color: 'var(--pink)', fontWeight: 600 }}>−{dels} {t('suggestions.removed')}</span>
      </div>
      <div
        className="mono"
        style={{
          fontSize: '0.76rem',
          border: '1px solid var(--line)',
          borderRadius: 'var(--r-sm)',
          overflow: 'hidden',
        }}
      >
        {rows.map((r, idx) => (
          <div
            key={idx}
            style={{
              display: 'flex',
              gap: 8,
              padding: '1px 10px',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
              background:
                r.kind === 'add' ? 'var(--fill-green)' : r.kind === 'del' ? 'var(--fill-pink)' : 'transparent',
              color:
                r.kind === 'add' ? '#146b3a' : r.kind === 'del' ? '#a01464' : 'var(--ink-soft)',
            }}
          >
            <span style={{ userSelect: 'none', opacity: 0.6, minWidth: 10 }}>
              {r.kind === 'add' ? '+' : r.kind === 'del' ? '−' : ' '}
            </span>
            <span style={{ flex: 1 }}>{r.text || ' '}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

// --- main -------------------------------------------------------------------

export default function SuggestionBody({ sg, skills }: Props) {
  const { t } = useTranslation();

  let data: Record<string, unknown> | null = null;
  try {
    const parsed = JSON.parse(sg.payload);
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      data = parsed as Record<string, unknown>;
    }
  } catch {
    data = null;
  }

  // Invalid JSON → never break, show raw.
  if (!data) return <RawFallback text={sg.payload} />;

  const str = (k: string): string => (typeof data![k] === 'string' ? (data![k] as string) : '');

  // Normalize the type so matching is robust to separator (dot vs underscore)
  // and stray whitespace/casing. The backend emits dot-form (skill.learned,
  // skill.correction, skill.variant) but legacy/auto paths may differ; we
  // classify by intent so no skill suggestion ever falls through to raw JSON.
  const norm = sg.type.trim().toLowerCase().replace(/[._-]+/g, '.');
  const kind = (() => {
    if (norm === 'skill.correction' || norm === 'update.skill') return 'skill.correction';
    if (norm === 'skill.variant') return 'skill.variant';
    if (norm === 'skill.learned' || norm === 'create.skill' || norm === 'skill') return 'skill.learned';
    if (norm === 'add.memory' || norm === 'add.correction') return 'add_memory';
    if (norm === 'add.memory.entry') return 'add_memory_entry';
    if (norm === 'create.project') return 'create_project';
    if (norm === 'secret.detected') return 'secret.detected';
    // Any other skill-family type still renders as a skill rather than raw JSON:
    // a correction if it carries a skill_id, otherwise a learned skill.
    if (norm.includes('skill')) return sg.skill_id ? 'skill.correction' : 'skill.learned';
    return norm;
  })();

  switch (kind) {
    case 'skill.learned':
    case 'create_skill': {
      const name = str('name') || sg.title;
      const content = str('content');
      return (
        <div>
          <div style={{ fontWeight: 600, color: 'var(--ink)' }}>
            {t('suggestions.newSkill')}: «{name}»
          </div>
          {content ? <ContentPreview text={content} /> : <RawFallback text={sg.payload} />}
        </div>
      );
    }

    case 'skill.correction':
    case 'update_skill': {
      const proposed = str('content');
      if (!proposed) return <RawFallback text={sg.payload} />;
      const current = skills?.find((s) => s.id === sg.skill_id)?.content;
      if (typeof current === 'string') {
        return (
          <div>
            <Label>{t('suggestions.currentVsProposed')}</Label>
            <DiffView oldText={current} newText={proposed} />
          </div>
        );
      }
      // No current content available — show proposed legibly with a notice.
      return (
        <div>
          <Label>{t('suggestions.proposedContent')}</Label>
          <ContentPreview text={proposed} />
        </div>
      );
    }

    case 'skill.variant': {
      const name = str('name') || sg.title;
      const content = str('content');
      const parents = Array.isArray(data.parent_skill_ids)
        ? (data.parent_skill_ids as unknown[]).map(String)
        : [];
      const parentNames = parents
        .map((pid) => skills?.find((s) => s.id === pid)?.name ?? pid)
        .filter(Boolean);
      return (
        <div>
          <div style={{ fontWeight: 600, color: 'var(--ink)' }}>
            {t('suggestions.variantLabel')}: «{name}»
          </div>
          {parentNames.length > 0 && (
            <div style={{ fontSize: '0.82rem', color: 'var(--muted)', marginTop: 2 }}>
              {t('suggestions.fromParent')} «{parentNames.join('», «')}»
            </div>
          )}
          {content ? <ContentPreview text={content} /> : <RawFallback text={sg.payload} />}
        </div>
      );
    }

    case 'add_memory':
    case 'add_correction': {
      const content = str('content') || str('text') || str('note');
      if (!content) return <RawFallback text={sg.payload} />;
      return (
        <div>
          <Label>{t('suggestions.memoryEntry')}</Label>
          <ContentPreview text={content} />
        </div>
      );
    }

    case 'add_memory_entry': {
      const content = str('content');
      const category = str('category');
      if (!content) return <RawFallback text={sg.payload} />;
      return (
        <div>
          {category && (
            <div style={{ fontSize: '0.78rem', color: 'var(--muted)', marginBottom: 2 }}>
              {category}
            </div>
          )}
          <Label>{t('suggestions.memoryEntry')}</Label>
          <ContentPreview text={content} />
        </div>
      );
    }

    case 'create_project': {
      const name = str('name');
      const desc = str('description');
      const dirs = Array.isArray(data.dirs) ? (data.dirs as unknown[]).map(String) : [];
      return (
        <div style={fieldBoxStyle()}>
          {name && (
            <div style={{ display: 'flex', gap: 8, marginBottom: 4 }}>
              <span style={{ color: 'var(--muted)', minWidth: 84, fontSize: '0.82rem' }}>
                {t('suggestions.projectName')}
              </span>
              <span style={{ color: 'var(--ink)', fontWeight: 600 }}>{name}</span>
            </div>
          )}
          {desc && (
            <div style={{ display: 'flex', gap: 8, marginBottom: 4 }}>
              <span style={{ color: 'var(--muted)', minWidth: 84, fontSize: '0.82rem' }}>
                {t('suggestions.projectDescription')}
              </span>
              <span style={{ color: 'var(--ink-soft)', fontSize: '0.88rem' }}>{desc}</span>
            </div>
          )}
          {dirs.length > 0 && (
            <div style={{ display: 'flex', gap: 8 }}>
              <span style={{ color: 'var(--muted)', minWidth: 84, fontSize: '0.82rem' }}>
                {t('suggestions.dirs')}
              </span>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                {dirs.map((d) => (
                  <span key={d} className="pill mono" style={{ fontSize: '0.74rem' }}>
                    {d}
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>
      );
    }

    case 'secret.detected':
    case 'secret_detected': {
      const name = str('name') || str('key') || sg.title;
      return (
        <div
          style={{
            ...fieldBoxStyle(),
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            background: 'var(--fill-amber)',
            borderColor: 'transparent',
          }}
        >
          <span aria-hidden style={{ fontSize: '1rem' }}>🔒</span>
          <div>
            <div style={{ fontWeight: 600, color: 'var(--ink)' }}>
              {t('suggestions.secretDetected')}: {name}
            </div>
            <div style={{ fontSize: '0.8rem', color: 'var(--muted)' }}>
              {t('suggestions.maskedValue')} ••••••••
            </div>
          </div>
        </div>
      );
    }

    default:
      return <RawFallback text={sg.payload} />;
  }
}
