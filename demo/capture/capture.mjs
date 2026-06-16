#!/usr/bin/env node
// V2 personalized Worrel demo capture.
// Drives the REAL Worrel cockpit (Go binary + embedded UI) with Playwright and
// records an MP4/webm walkthrough, personalized per lead.
//
// HARD RULE — zero personal data:
//   - Runs Worrel against a FRESH EMPTY temp data dir.
//   - Points WORREL_CLAUDE_PROJECTS at a FAKE empty dir we fill with synthetic
//     transcripts only. The real ~/.claude (and friends) are never read.
//
// Usage:
//   node capture.mjs --lead leads/acme.json [--port 7799] [--keep-data]
//
import { chromium } from 'playwright';
import { spawn, execSync } from 'node:child_process';
import { mkdtempSync, rmSync, mkdirSync, writeFileSync, readFileSync, existsSync, renameSync, readdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(__dirname, '..', '..');
const OUT_DIR = path.join(__dirname, 'out');

// ---------- args ----------
function arg(name, def) {
  const i = process.argv.indexOf(`--${name}`);
  if (i >= 0 && i + 1 < process.argv.length) return process.argv[i + 1];
  return def;
}
const hasFlag = (name) => process.argv.includes(`--${name}`);

const leadPath = arg('lead', 'leads/acme.json');
const PORT = parseInt(arg('port', '7799'), 10);
const KEEP_DATA = hasFlag('keep-data');
const lead = JSON.parse(readFileSync(path.resolve(__dirname, leadPath), 'utf8'));
const BASE = `http://127.0.0.1:${PORT}`;

console.log(`[capture] lead="${lead.leadName}" accent=${lead.accent} port=${PORT}`);

// ---------- ensure binary ----------
const BIN = path.join(REPO_ROOT, 'bin', 'worrel');
if (!existsSync(BIN)) {
  console.log('[capture] bin/worrel missing — building (make build)…');
  execSync('make build', { cwd: REPO_ROOT, stdio: 'inherit' });
}

// ---------- isolated, EMPTY dirs ----------
// fakeHome isolates EVERY CLI adapter: claude-code, opencode, gemini, codex, pidev
// all resolve their history roots from os.UserHomeDir() ($HOME). By overriding
// HOME the binary can never read the operator's real agent history.
const fakeHome = mkdtempSync(path.join(tmpdir(), 'worrel-demo-home-'));
const dataDir = mkdtempSync(path.join(tmpdir(), 'worrel-demo-data-'));
const claudeDir = path.join(fakeHome, '.claude', 'projects'); // explicit + under fake HOME
mkdirSync(claudeDir, { recursive: true });
console.log(`[capture] fake HOME  : ${fakeHome}`);
console.log(`[capture] data dir   : ${dataDir}`);
console.log(`[capture] fake CLAUDE: ${claudeDir}`);

// ---------- synthetic transcripts (so Retro inventory has real numbers) ----------
// Claude Code layout: <root>/<slug>/<sessionId>.jsonl
function seedTranscripts() {
  const sessions = [
    { proj: 'acme-payments', msgs: [
      ['user', 'set up the staging deploy pipeline for the payments service'],
      ['assistant', 'I built the deploy-staging make target and gated it behind the e2e suite.'],
    ]},
    { proj: 'acme-payments', msgs: [
      ['user', 'rotate the gateway api keys safely'],
      ['assistant', 'Minted a new key in the vault, rolled it to staging, verified, scheduled the old one for revocation.'],
    ]},
    { proj: 'blog-engine', msgs: [
      ['user', 'make sure we run e2e before merging'],
      ['assistant', 'Added a pre-merge check that runs npm run e2e and blocks on failures.'],
    ]},
    { proj: 'mobile-checkout', msgs: [
      ['user', 'persist the cart between app launches'],
      ['assistant', 'Cart state now persists in encrypted local storage and replays on launch.'],
    ]},
  ];
  let n = 0;
  for (const s of sessions) {
    const slug = '-Users-demo-' + s.proj;
    const dir = path.join(claudeDir, slug);
    mkdirSync(dir, { recursive: true });
    const sid = `demo-${s.proj}-${n++}`;
    const lines = [];
    const ts = new Date().toISOString();
    for (const [role, text] of s.msgs) {
      lines.push(JSON.stringify({
        type: role,
        sessionId: sid,
        cwd: `/Users/demo/${s.proj}`,
        timestamp: ts,
        message: { role, content: [{ type: 'text', text }] },
      }));
    }
    writeFileSync(path.join(dir, `${sid}.jsonl`), lines.join('\n') + '\n');
  }
  console.log(`[capture] seeded ${sessions.length} synthetic transcripts`);

  // The opencode adapter opens ~/.local/share/opencode/opencode.db read-only and
  // errors if it is missing (which would break the Retro inventory). Create an
  // EMPTY but valid opencode DB under the fake HOME so the inventory cleanly
  // reports zero opencode sessions (no real data is ever read).
  const ocDir = path.join(fakeHome, '.local', 'share', 'opencode');
  mkdirSync(ocDir, { recursive: true });
  const ocDb = path.join(ocDir, 'opencode.db');
  try {
    execSync(
      `sqlite3 "${ocDb}" "CREATE TABLE session(id TEXT PRIMARY KEY, directory TEXT, title TEXT, time_created INTEGER, time_updated INTEGER); CREATE TABLE message(id TEXT PRIMARY KEY, session_id TEXT, data TEXT, time_created INTEGER); CREATE TABLE part(id TEXT PRIMARY KEY, message_id TEXT, data TEXT);"`,
      { stdio: 'ignore' },
    );
  } catch {
    console.warn('[capture] sqlite3 unavailable — opencode inventory may show an error in the video');
  }
}

// ---------- launch worrel ----------
let proc;
function launch() {
  proc = spawn(BIN, ['-addr', `127.0.0.1:${PORT}`, '-data', dataDir, '-no-open'], {
    // WORREL_MASTER_PASSWORD forces the scrypt vault path so the binary never
    // touches the macOS Keychain (which is unavailable under the fake HOME and
    // would otherwise block startup).
    env: {
      ...process.env,
      HOME: fakeHome,
      WORREL_CLAUDE_PROJECTS: claudeDir,
      WORREL_MASTER_PASSWORD: 'demo',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  proc.stdout.on('data', (d) => process.env.DEBUG && process.stdout.write(`[worrel] ${d}`));
  proc.stderr.on('data', (d) => process.env.DEBUG && process.stderr.write(`[worrel] ${d}`));
}

async function waitHealthy(timeoutMs = 15000) {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    try {
      const r = await fetch(`${BASE}/api/health`);
      if (r.ok) return;
    } catch { /* not up yet */ }
    await sleep(250);
  }
  throw new Error('worrel did not become healthy');
}

// ---------- seeding via HTTP API ----------
async function api(method, p, body) {
  const r = await fetch(`${BASE}${p}`, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!r.ok) throw new Error(`${method} ${p} -> ${r.status} ${await r.text()}`);
  return r.json();
}

async function seedData() {
  const created = [];
  for (const pj of lead.projects) {
    const proj = await api('POST', '/api/projects', { name: pj.name, description: pj.description || '' });
    if (pj.memory) await api('PUT', `/api/projects/${proj.id}/memory`, { content: pj.memory, note: 'seed' });
    for (const sk of (pj.skills || [])) {
      await api('POST', `/api/projects/${proj.id}/skills`, { name: sk.name, content: sk.content });
    }
    created.push(proj);
  }
  // A few pending suggestions on the lead's first project so the Suggestions
  // screen is populated for the walkthrough.
  const p0 = created[0];
  await api('POST', '/api/suggestions', {
    project_id: p0.id, type: 'skill.learned', origin: 'incremental',
    title: 'Deploy staging pipeline',
    payload: JSON.stringify({ name: 'Deploy staging pipeline', content: '# Deploy staging pipeline\nBuild, push image, run `make deploy-staging`, wait for health check green.' }),
    evidence: 'Observed across 2 recent sessions.',
  });
  await api('POST', '/api/suggestions', {
    project_id: p0.id, type: 'add_memory', origin: 'incremental',
    title: 'Always use integer cents for money',
    payload: JSON.stringify({ content: 'All monetary values are handled as integer cents; never floats.' }),
    evidence: 'Repeated correction in payment code reviews.',
  });
  console.log(`[capture] seeded ${created.length} projects + suggestions`);
  return created;
}

// ---------- helpers ----------
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// Inject a personalized brand overlay (lead name + accent) into the page.
async function injectOverlay(page) {
  await page.evaluate(({ leadName, accent }) => {
    document.querySelectorAll('#worrel-demo-overlay').forEach((e) => e.remove());
    const el = document.createElement('div');
    el.id = 'worrel-demo-overlay';
    el.innerHTML = `
      <div class="wdo-card">
        <div class="wdo-dot"></div>
        <div class="wdo-txt"><span class="wdo-small">Personalized demo for</span><strong>${leadName}</strong></div>
      </div>`;
    const style = document.createElement('style');
    style.textContent = `
      #worrel-demo-overlay{position:fixed;bottom:18px;right:18px;z-index:99999;font-family:Inter,system-ui,sans-serif;animation:wdoIn .5s ease}
      #worrel-demo-overlay .wdo-card{display:flex;align-items:center;gap:10px;background:rgba(255,252,248,.92);backdrop-filter:blur(8px);border:1px solid ${accent}55;border-left:4px solid ${accent};border-radius:12px;padding:10px 14px;box-shadow:0 8px 30px rgba(0,0,0,.12)}
      #worrel-demo-overlay .wdo-dot{width:10px;height:10px;border-radius:50%;background:${accent};box-shadow:0 0 0 4px ${accent}33}
      #worrel-demo-overlay .wdo-small{display:block;font-size:10px;letter-spacing:.04em;text-transform:uppercase;color:#8a7a6a}
      #worrel-demo-overlay strong{font-size:15px;color:#2b2018}
      @keyframes wdoIn{from{opacity:0;transform:translateY(-8px)}to{opacity:1;transform:none}}`;
    document.head.appendChild(style);
    document.body.appendChild(el);
  }, { leadName: lead.leadName, accent: lead.accent });
}

// The UI uses BrowserRouter — navigate to real paths (SPA fallback serves index).
async function goto(page, route) {
  await page.goto(`${BASE}${route}`, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('.sidebar', { timeout: 8000 });
  await injectOverlay(page);
  await sleep(600);
}

// Title card composed in-page (intro/outro) reusing warm-light brand.
async function showCard(page, title, subtitle, ms = 2600) {
  await page.evaluate(({ title, subtitle, accent, leadName }) => {
    document.querySelectorAll('#worrel-demo-card').forEach((e) => e.remove());
    const wrap = document.createElement('div');
    wrap.id = 'worrel-demo-card';
    wrap.innerHTML = `
      <div class="wdc-inner">
        <div class="wdc-badge" style="--a:${accent}">WORREL · ${leadName}</div>
        <h1>${title}</h1>
        <p>${subtitle}</p>
      </div>`;
    const style = document.createElement('style');
    style.textContent = `
      #worrel-demo-card{position:fixed;inset:0;z-index:100000;display:flex;align-items:center;justify-content:center;
        background:radial-gradient(120% 120% at 80% 0%, #fff6ec 0%, #fdeede 45%, #fce3cf 100%);font-family:'Inter Tight',Inter,system-ui,sans-serif;animation:wdcIn .6s ease}
      #worrel-demo-card .wdc-inner{text-align:center;max-width:760px;padding:0 32px}
      #worrel-demo-card .wdc-badge{display:inline-block;font-size:12px;font-weight:600;letter-spacing:.12em;color:#fff;background:var(--a);padding:6px 14px;border-radius:999px;margin-bottom:22px}
      #worrel-demo-card h1{font-size:52px;line-height:1.05;margin:0 0 16px;color:#241a12;font-weight:700}
      #worrel-demo-card p{font-size:21px;color:#6b5847;margin:0}
      @keyframes wdcIn{from{opacity:0;transform:scale(.98)}to{opacity:1;transform:none}}`;
    document.head.appendChild(style);
    document.body.appendChild(wrap);
  }, { title, subtitle, accent: lead.accent, leadName: lead.leadName });
  await sleep(ms);
  await page.evaluate(() => document.querySelectorAll('#worrel-demo-card').forEach((e) => e.remove()));
}

// Gentle scroll so panels with content read on video.
async function slowScroll(page, ms = 1800) {
  const steps = 6;
  for (let i = 0; i < steps; i++) {
    await page.mouse.wheel(0, 220);
    await sleep(ms / steps);
  }
}

// ---------- the roteiro ----------
async function run() {
  mkdirSync(OUT_DIR, { recursive: true });
  const browser = await chromium.launch();
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 2,
    locale: 'en-US',
    recordVideo: { dir: OUT_DIR, size: { width: 1440, height: 900 } },
  });
  // Force English UI (i18next-browser-languagedetector reads i18nextLng).
  await context.addInitScript(() => {
    try { localStorage.setItem('i18nextLng', 'en'); } catch {}
  });
  const page = await context.newPage();

  // Scene 1 — intro card
  await page.goto(`${BASE}/`, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('.app-layout, .sidebar', { timeout: 10000 });
  await showCard(page, `Meet Worrel, ${lead.leadName}`, 'The cockpit that learns from your AI coding agents.', 3000);

  // Scene 2 — dashboard / panel with the lead's projects
  await goto(page, '/');
  await sleep(1800);
  await slowScroll(page);
  await sleep(1200);

  // Scene 3 — open a project -> memory + skills
  const projects = await api('GET', '/api/projects');
  const first = projects[0];
  await goto(page, `/projects/${first.id}`);
  await page.waitForSelector('h1, .page-head', { timeout: 8000 });
  await sleep(2200);
  await slowScroll(page, 2400);
  await sleep(1600);

  // Scene 4 — retroactive analysis (inventory + history; no LLM run)
  await goto(page, '/retro');
  await page.waitForSelector('.retro-page, h1', { timeout: 8000 });
  await sleep(2600); // inventory of the synthetic transcripts loads
  await slowScroll(page, 1800);
  await sleep(1600);

  // Scene 5 — suggestions to review
  await goto(page, '/suggestions');
  await page.waitForSelector('h1', { timeout: 8000 });
  await sleep(2600);
  await slowScroll(page, 1800);
  await sleep(1400);

  // Scene 6 — outro card
  await showCard(page, `Built for ${lead.leadName}`, 'Every agent, one memory. Ship faster with Worrel.', 3200);

  await context.close(); // flush video
  await browser.close();

  // Rename newest webm -> sample.<ext>
  const vids = readdirSync(OUT_DIR).filter((f) => f.endsWith('.webm'));
  vids.sort();
  const newest = vids[vids.length - 1];
  if (!newest) throw new Error('no video produced');
  const src = path.join(OUT_DIR, newest);
  let finalPath = path.join(OUT_DIR, 'sample.mp4');
  // Try to transcode to MP4 with ffmpeg; fall back to keeping webm.
  try {
    execSync(`ffmpeg -y -i "${src}" -c:v libx264 -pix_fmt yuv420p -movflags +faststart "${finalPath}"`, { stdio: 'ignore' });
    rmSync(src);
  } catch {
    finalPath = path.join(OUT_DIR, 'sample.webm');
    renameSync(src, finalPath);
    console.log('[capture] ffmpeg not available — kept .webm');
  }
  console.log(`[capture] video -> ${finalPath}`);
  return finalPath;
}

// ---------- orchestrate ----------
(async () => {
  let code = 0;
  try {
    seedTranscripts();
    launch();
    await waitHealthy();
    await seedData();
    const out = await run();
    console.log(`\n[capture] DONE: ${out}`);
  } catch (e) {
    code = 1;
    console.error('[capture] FAILED:', e);
  } finally {
    if (proc) proc.kill('SIGTERM');
    if (!KEEP_DATA) {
      rmSync(dataDir, { recursive: true, force: true });
      rmSync(fakeHome, { recursive: true, force: true });
      console.log('[capture] cleaned ephemeral dirs');
    } else {
      console.log(`[capture] kept dirs: ${dataDir} ${fakeHome}`);
    }
    process.exit(code);
  }
})();
