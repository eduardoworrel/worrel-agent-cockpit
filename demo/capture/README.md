# Worrel demo capture (V2)

Personalized product-demo videos by driving the **real** Worrel cockpit with
Playwright and recording an MP4/webm. No mockups, no fake screens — it boots the
actual Go binary (with the embedded UI), seeds a synthetic dataset, walks the
real UI, and records the session.

## What it does

1. Builds `bin/worrel` if missing (`make build`), then boots an **ephemeral**
   Worrel instance against a fresh temp data dir.
2. Seeds a neutral synthetic dataset (projects, memories, skills, suggestions)
   via the HTTP API, plus synthetic Claude Code transcripts so the
   **Retroactive analysis** screen shows real numbers.
3. Drives the real UI through a scripted roteiro, personalized per lead.
4. Records the walkthrough and writes `out/sample.mp4` (or `.webm` if `ffmpeg`
   is not installed).

## Prerequisites

- Node 18+ and npm
- Go toolchain (only if `bin/worrel` is not already built)
- `sqlite3` CLI (used to create an empty opencode DB so the inventory is clean)
- `ffmpeg` (optional — transcodes the recording to MP4; otherwise a `.webm` is kept)

## Install

```bash
cd demo/capture
npm install        # installs Playwright and downloads Chromium
```

## Run

```bash
node capture.mjs --lead leads/acme.json
# other options:
#   --port 7799      port for the ephemeral instance (default 7799)
#   --keep-data      keep the temp dirs after the run (debugging)
```

Output: `out/sample.mp4`.

## Personalization

Each lead is a JSON file (`leads/*.json`):

```json
{
  "leadName": "Acme Corp",
  "accent": "#ff7a18",
  "projects": [
    {
      "name": "acme-payments",
      "description": "...",
      "memory": "## Conventions\n- ...",
      "skills": [{ "name": "Deploy staging pipeline", "content": "# ..." }]
    }
  ]
}
```

- `leadName` drives the intro/outro title cards and an on-screen brand overlay
  ("Personalized demo for <leadName>").
- `accent` recolors the overlay and title-card badge.
- `projects[]` become the real projects the walkthrough opens (the first one is
  the "hero" project and carries the seeded suggestions).

Two examples are included: `leads/acme.json` and `leads/globex.json`.

## The roteiro (what the video shows)

1. **Intro card** — "Meet Worrel, <leadName>".
2. **Dashboard** — the lead's seeded projects, the pending-suggestions badge.
3. **Project** — opens the hero project: Memory tab (seeded content + version
   history), with the other tabs (Skills, Sessions, Suggestions, Secrets) visible.
4. **Retroactive analysis** — scope & budget computed from the synthetic
   transcripts (claude-code sessions; opencode 0), the run wizard, run history.
5. **Suggestions** — pending suggestions (a memory entry and a learned skill)
   with the Accept / Edit / Reject / Defer controls.
6. **Outro card** — "Built for <leadName>".

> Note: the Retro wizard's **Execute** step spawns a real headless LLM run
> (the `claude` binary) and is intentionally NOT triggered — the roteiro shows
> the inventory and history instead, keeping the capture deterministic and free.

## Zero real data — how isolation is guaranteed

The hard rule is that the operator's real agent history is never read. The script
enforces this with three layers:

1. **Fresh empty data dir** — Worrel runs against a brand-new `mktemp` directory,
   so its SQLite DB starts empty.
2. **Fake `HOME`** — the binary is launched with `HOME` pointed at a temp dir.
   Every CLI adapter (claude-code, opencode, gemini, codex, pidev) resolves its
   history root from `os.UserHomeDir()`, so under the fake HOME none of the real
   roots (`~/.claude`, `~/.codex`, `~/.gemini`, `~/.local/share/opencode`, …) are
   reachable. The only history present is the synthetic transcripts the script
   writes into `$HOME/.claude/projects` (plus an empty opencode DB).
3. **`WORREL_MASTER_PASSWORD=demo`** — forces the scrypt vault path so the binary
   never touches the macOS Keychain (which is both unavailable under the fake
   HOME and would otherwise pop a system prompt).

All temp dirs are deleted at the end of the run (unless `--keep-data`). The
synthetic dataset is deliberately generic: `acme-payments`, `blog-engine`,
`mobile-checkout`, with skills like "Deploy staging pipeline" and "Rotate API
keys".
