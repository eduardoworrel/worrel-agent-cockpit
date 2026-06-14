# worrel — personalized demo video pipeline (Remotion)

A [Remotion](https://www.remotion.dev) project that renders a ~26s branded demo
video for **worrel** (the local-first agent cockpit). The whole video is
parameterized by a JSON props file, so you can generate a different,
personalized video per lead just by swapping the JSON — no code changes.

The look (colors, fonts, components) is lifted from the marketing landing page
(`landing/styles.css`) to stay on-brand: warm-light paper background, ink text,
orange accent, Inter / Inter Tight / JetBrains Mono.

## The story (5 scenes)

1. **Intro** — brand lockup + a personalization slot ("Prepared for {leadName}").
2. **Observe** — sessions from your CLIs (Claude Code, OpenCode, Gemini, Codex, Pi) flowing into a terminal feed.
3. **Distill** — Projects, versioned Memories, and Skills (with lineage) appearing as cards.
4. **Retroactive analysis** — a scan bar sweeps your past history and distills insights.
5. **Outro** — "Private by default. Yours forever." + CTA.

## Install

```bash
cd demo/remotion
npm install
```

## Preview (Remotion Studio)

```bash
npx remotion studio
```

Open the `Demo` composition. The right-hand props panel is generated from the
Zod schema (`src/schema.ts`), so you can tweak `leadName`, `accent`, `cliList`,
`sampleProjects`, etc. live and scrub the timeline.

## Render per lead

Each lead is just a JSON file in `props/`. Render full-res 1080p MP4:

```bash
# generic version
npx remotion render Demo out/generic.mp4 --props=props/generic.json

# a specific lead (Acme Corp, blue accent, custom projects + CTA)
npx remotion render Demo out/acme.mp4 --props=props/acme.json
```

Shortcuts are wired in `package.json`:

```bash
npm run render:generic
npm run render:acme
```

Fast/low-res preview render (what produced `out/sample.mp4`):

```bash
npx remotion render Demo out/sample.mp4 --props=props/generic.json --scale=0.5 --crf=28
```

Single still frame (great for thumbnails / quick checks):

```bash
npx remotion still Demo out/thumb.png --frame=400 --props=props/acme.json
```

## What the JSON personalizes

| Prop             | Type                          | Personalizes                                              |
| ---------------- | ----------------------------- | -------------------------------------------------------- |
| `leadName`       | string                        | "Prepared for …" badge (intro) + outro sign-off          |
| `leadInitials`   | string                        | Avatar monogram in the intro badge                       |
| `accent`         | hex string                    | Highlight color across every scene (kickers, scan, CTA)  |
| `cliList`        | string[]                      | CLI chips + caption in the Observe scene                 |
| `sampleProjects` | {name, blurb, memories, skills}[] | Project cards in the Distill scene                   |
| `skills`         | string[]                      | Skill rows in the Distill scene                          |
| `ctaText`        | string                        | The terminal command shown in the outro CTA              |

To create a new lead: copy `props/generic.json`, edit the values, and pass it
with `--props=`.

## Sample output

`out/sample.mp4` — full 26s timeline at 960×540 (rendered as a fast,
low-res sample). `out/frame-knowledge.png` is a representative still of the
Distill scene with the `acme.json` (blue accent) props.

## Render cost / timing

On an Apple Silicon laptop:

- **Sample** (960×540, 780 frames, crf 28): ~23s wall time, ~1.3 MB.
- **Full 1080p** scales roughly linearly with pixel count (~4×), expect ~1–2 min.

Remotion renders locally via a headless Chromium — no cloud, no API cost. The
only one-time cost is the ~85 MB headless shell download on first run.

## Notes

- All demo data is synthetic (acme-payments, blog-engine, mobile-checkout, etc.).
- `node_modules/` and large `out/*.mp4` are gitignored; `out/sample.mp4` is kept as a checked-in sample.
