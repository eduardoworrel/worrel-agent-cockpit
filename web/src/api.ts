export interface Project {
  id: string;
  slug: string;
  name: string;
  description: string;
  dirs: string[];
  created_at: number;
  updated_at: number;
}

export interface MemoryVersion {
  id: number;
  project_id: string;
  content: string;
  note: string;
  created_at: number;
}

export interface MemoryEntry {
  id: string;
  project_id: string;
  content: string;
  category: string;
  evidence: string;
  status: string;
  superseded_by: string;
  created_at: number;
}

export interface AskRequest {
  request_id: string;
  session_id: string;
  session_label: string;
  kind: 'permission' | 'choice';
  title: string;
  detail?: string;
  options: string[] | null;
}

export interface Skill {
  id: string;
  project_id: string;
  slug: string;
  name: string;
  content: string;
  created_at: number;
  updated_at: number;
  active_generation: number;
  evolution_policy: string;
  origin: string;
  last_used_at: number;
}

export interface SkillGeneration {
  id: number;
  skill_id: string;
  generation: number;
  evolution_type: string;
  parent_skill_ids: string[];
  diff: string;
  snapshot: string;
  change_summary: string;
  evidence: string;
  authorship: string;
  created_at: number;
}

export interface SkillStats {
  total_uses: number;
  success_count: number;
  error_count: number;
  edge_cases: number;
  avg_duration_ms: number;
  success_rate: number;
  trend: string;
  consec_fail: number;
}

export interface Suggestion {
  id: string;
  project_id: string;
  session_id: string | null;
  skill_id: string | null;
  type: string;
  status: string;
  title: string;
  payload: string;
  evidence: string;
  created_at: number;
  resolved_at: number | null;
}

export interface Session {
  id: string;
  project_id: string;
  adapter: string;
  mode: string;
  title: string;
  status: string;
  started_at: number;
  ended_at: number | null;
  summary: string;
  transcript_pruned: boolean;
  continues?: string | null;
  context_used?: number;
  context_limit?: number;
  workspace_dir?: string;
}

const BASE = '/api';

async function req<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}: ${await res.text()}`);
  if (res.status === 204) return undefined as T;
  return res.json();
}

export function listProjects(): Promise<Project[]> {
  return req('/projects');
}

export interface DirListing {
  path: string;
  parent: string;
  home: string;
  entries: { name: string; path: string }[];
}

export function listDirs(path?: string): Promise<DirListing> {
  const qs = path ? `?path=${encodeURIComponent(path)}` : '';
  return req(`/fs/dirs${qs}`);
}

export function createProject(name: string, description: string, dirs: string[]): Promise<Project> {
  return req('/projects', {
    method: 'POST',
    body: JSON.stringify({ name, description, dirs }),
  });
}

export function getProject(id: string): Promise<Project> {
  return req(`/projects/${id}`);
}

export function updateProject(id: string, name: string, description: string): Promise<void> {
  return req(`/projects/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ name, description }),
  });
}

export function getMemory(projectId: string): Promise<MemoryVersion> {
  return req(`/projects/${projectId}/memory`);
}

export function saveMemory(projectId: string, content: string, note: string): Promise<MemoryVersion> {
  return req(`/projects/${projectId}/memory`, {
    method: 'PUT',
    body: JSON.stringify({ content, note }),
  });
}

export function listMemoryVersions(projectId: string): Promise<MemoryVersion[]> {
  return req(`/projects/${projectId}/memory/versions`);
}

export function revertMemory(projectId: string, versionId: number): Promise<MemoryVersion> {
  return req(`/projects/${projectId}/memory/revert`, {
    method: 'POST',
    body: JSON.stringify({ version_id: versionId }),
  });
}

export function listMemoryEntries(projectId: string): Promise<MemoryEntry[]> {
  return req(`/projects/${projectId}/memory/entries`);
}

export function deleteMemoryEntry(projectId: string, entryId: string): Promise<{ status: string }> {
  return req(`/projects/${projectId}/memory/entries/${entryId}`, { method: 'DELETE' });
}

export function listSkills(projectId?: string): Promise<Skill[]> {
  const qs = projectId ? `?project_id=${projectId}` : '';
  return req(`/skills${qs}`);
}

export function createSkill(projectId: string, name: string, content: string): Promise<Skill> {
  return req(`/projects/${projectId}/skills`, {
    method: 'POST',
    body: JSON.stringify({ name, content }),
  });
}

export function updateSkill(id: string, name: string, content: string): Promise<Skill> {
  return req(`/skills/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ name, content }),
  });
}

export function listGenerations(skillId: string): Promise<SkillGeneration[]> {
  return req(`/skills/${skillId}/generations`);
}

export function revertGeneration(skillId: string, generation: number): Promise<Skill> {
  return req(`/skills/${skillId}/revert`, {
    method: 'POST',
    body: JSON.stringify({ generation }),
  });
}

export function getSkillStats(skillId: string): Promise<SkillStats> {
  return req(`/skills/${skillId}/stats`);
}

export function setSkillPolicy(skillId: string, policy: string): Promise<Skill> {
  return req(`/skills/${skillId}/policy`, {
    method: 'PUT',
    body: JSON.stringify({ policy }),
  });
}

export function setProjectSkillsPolicy(projectId: string, policy: string): Promise<{ policy: string }> {
  return req(`/projects/${projectId}/skills/policy`, {
    method: 'PUT',
    body: JSON.stringify({ policy }),
  });
}

export function exportSkill(skillId: string): Promise<string> {
  return fetch(`/api/skills/${skillId}/export`).then(r => r.text());
}

export function importSkill(projectId: string, content: string): Promise<Skill> {
  return req(`/projects/${projectId}/skills/import`, {
    method: 'POST',
    body: JSON.stringify({ content }),
  });
}

export function reclassifySuggestion(id: string, type: string): Promise<Suggestion> {
  return req(`/suggestions/${id}/type`, {
    method: 'PUT',
    body: JSON.stringify({ type }),
  });
}

export function listSuggestions(projectId?: string, status?: string): Promise<Suggestion[]> {
  const params = new URLSearchParams();
  if (projectId) params.set('project_id', projectId);
  if (status) params.set('status', status);
  const qs = params.toString() ? `?${params}` : '';
  return req(`/suggestions${qs}`);
}

export function createSuggestion(s: Partial<Suggestion>): Promise<Suggestion> {
  return req('/suggestions', { method: 'POST', body: JSON.stringify(s) });
}

export function acceptSuggestion(
  id: string,
  edited?: { title: string; payload: string },
  supersede?: string,
  as?: 'skill' | 'agente',
): Promise<Suggestion> {
  const params = new URLSearchParams();
  if (supersede) params.set('supersede', supersede);
  if (as) params.set('as', as);
  const q = params.toString();
  const qs = q ? `?${q}` : '';
  return req(`/suggestions/${id}/accept${qs}`, {
    method: 'POST',
    body: JSON.stringify(edited ?? {}),
  });
}

export interface Agent {
  id: string;
  project_id: string;
  slug: string;
  name: string;
  persona: string;
  evidence: string;
  created_at: number;
  updated_at: number;
}

export function listAgents(projectId: string): Promise<Agent[]> {
  return req(`/projects/${projectId}/agents`);
}

export function rejectSuggestion(id: string): Promise<Suggestion> {
  return req(`/suggestions/${id}/reject`, { method: 'POST' });
}

export function deferSuggestion(id: string): Promise<Suggestion> {
  return req(`/suggestions/${id}/defer`, { method: 'POST' });
}

export function listSessions(projectId?: string): Promise<Session[]> {
  const qs = projectId ? `?project_id=${projectId}` : '';
  return req(`/sessions${qs}`);
}

export interface DetectedAdapter {
  id: string;
  installed: { present: boolean; path: string; version: string };
  caps: { hooks: boolean; headless: boolean; own_session_id: boolean; context_measured: boolean };
}

export function listAdapters(): Promise<DetectedAdapter[]> {
  return req('/adapters');
}

export function createSession(
  projectId: string,
  adapter: string,
  opts?: { skill?: string; skillId?: string; agentId?: string }
): Promise<Session> {
  return req(`/projects/${projectId}/sessions`, {
    method: 'POST',
    body: JSON.stringify({
      adapter,
      skill: opts?.skill ?? '',
      skill_id: opts?.skillId ?? '',
      agent_id: opts?.agentId ?? '',
    }),
  });
}

export function killSession(id: string): Promise<void> {
  return req(`/sessions/${id}/kill`, { method: 'POST' });
}

// DistillResult espelha distill.Result do backend.
export interface DistillResult {
  sessions: number;
  created: number;
  duplicates: number;
  dropped: number;
  screened_out: number;
  proactive: number;
  auto_applied: number;
}

// distillSession extrai aprendizados (skills/memórias) de UMA sessão sob demanda;
// cria sugestões pendentes que o usuário aprova no drawer.
export function distillSession(id: string): Promise<DistillResult> {
  return req(`/sessions/${id}/distill`, { method: 'POST' });
}

// pasteImage envia os bytes de uma imagem colada no terminal e recebe o
// caminho absoluto onde o servidor a salvou (dentro do workspace da sessão).
export async function pasteImage(id: string, blob: Blob): Promise<{ path: string }> {
  const res = await fetch(`${BASE}/sessions/${id}/paste-image`, {
    method: 'POST',
    headers: { 'Content-Type': blob.type },
    body: blob,
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}: ${await res.text()}`);
  return res.json();
}

export function postHandoff(id: string): Promise<{ old_id: string; new_id: string; summary: string }> {
  return req(`/sessions/${id}/handoff`, { method: 'POST' });
}

export function getPendingAsks(): Promise<AskRequest[]> {
  return req('/asks/pending');
}

export function respondAsk(requestId: string, answer: string): Promise<void> {
  return req(`/asks/${requestId}/respond`, {
    method: 'POST',
    body: JSON.stringify({ answer }),
  });
}

export function getSettings(): Promise<Record<string, string>> {
  return req('/settings');
}

export function putSettings(s: Record<string, string>): Promise<void> {
  return req('/settings', { method: 'PUT', body: JSON.stringify(s) });
}

export interface PromptDef {
  name: string;
  value: string;
  default: string;
  overridden: boolean;
}

export function getPrompts(): Promise<PromptDef[]> {
  return req('/prompts');
}

// savePrompt salva o override; value vazio (ou igual ao default) reseta ao default.
export function savePrompt(name: string, value: string): Promise<unknown> {
  return req(`/prompts/${encodeURIComponent(name)}`, {
    method: 'PUT',
    body: JSON.stringify({ value }),
  });
}

// --- Secrets ---

export interface Secret {
  id: string;
  project_id: string;
  name: string;
  mode: 'value' | 'recipe';
  recipe: string;
  policy: 'always' | 'per_session' | 'per_access';
  injectable: boolean;
  has_value: boolean;
  created_at: number;
  updated_at: number;
}

export interface SecretAudit {
  id: number;
  secret_id: string | null;
  secret_name: string;
  session_id: string | null;
  project_id: string | null;
  action: string;
  detail: string;
  created_at: number;
}

export function listSecrets(projectId: string): Promise<Secret[]> {
  return req(`/projects/${projectId}/secrets`);
}

export function createSecret(projectId: string, data: {
  name: string; mode: string; value?: string; recipe?: string;
  policy?: string; injectable?: boolean;
}): Promise<Secret> {
  return req(`/projects/${projectId}/secrets`, { method: 'POST', body: JSON.stringify(data) });
}

export function deleteSecret(id: string): Promise<void> {
  return req(`/secrets/${id}`, { method: 'DELETE' });
}

export function updateSecretValue(id: string, value: string): Promise<void> {
  return req(`/secrets/${id}/value`, { method: 'PUT', body: JSON.stringify({ value }) });
}

export function updateSecretPolicy(id: string, policy: string, injectable: boolean): Promise<void> {
  return req(`/secrets/${id}/policy`, { method: 'PUT', body: JSON.stringify({ policy, injectable }) });
}

export function listSecretAudit(projectId: string): Promise<SecretAudit[]> {
  return req(`/projects/${projectId}/secrets/audit`);
}

export function answerSecretApproval(reqId: string, approve: boolean): Promise<void> {
  return req(`/secret-approvals/${reqId}`, { method: 'POST', body: JSON.stringify({ approve }) });
}

export function setInjection(projectId: string, enabled: boolean): Promise<void> {
  return req(`/projects/${projectId}/secrets/injection`, { method: 'PUT', body: JSON.stringify({ enabled }) });
}

export function createFreeSession(adapter: string, dirs?: string[], skill?: string): Promise<Session> {
  return req('/sessions', {
    method: 'POST',
    body: JSON.stringify({ adapter, dirs: dirs ?? [], skill: skill ?? '' }),
  });
}

export function classifySession(id: string, projectId: string): Promise<{ ok: boolean }> {
  return req(`/sessions/${id}/classify`, {
    method: 'POST',
    body: JSON.stringify({ project_id: projectId }),
  });
}

export function promoteSession(id: string, name: string, description: string): Promise<Project> {
  return req(`/sessions/${id}/promote`, {
    method: 'POST',
    body: JSON.stringify({ name, description }),
  });
}

export function listActiveSessions(): Promise<Session[]> {
  return req('/sessions/active');
}

// --- AG-UI interaction channel (Home) ---
// Contrato estável que a Home consome para interagir com a sessão sem ver o
// terminal. Hoje preenchido por um tradutor (transcript/ask); amanhã pelo motor.

export type InteractionState = 'working' | 'awaiting' | 'ended';
export type InterruptKind = 'permission' | 'choice' | 'text';

export interface Interrupt {
  request_id: string;
  kind: InterruptKind;
  prompt: string;
  detail?: string;
  options?: string[];
}

export interface ToolCall {
  name: string;
  summary?: string;
}

export interface HistoryLine {
  role: 'you' | 'ai' | 'tool' | 'system';
  text: string;
}

export interface InteractionSnapshot {
  session_id: string;
  state: InteractionState;
  message?: string;        // última fala/pergunta da IA
  user_message?: string;   // último pedido do usuário
  tool_calls?: ToolCall[]; // o que a IA fez
  progress?: string[];     // resumo narrado por IA (timeline do card)
  history?: HistoryLine[]; // transcript completo (visão de conversa)
  interrupt?: Interrupt;   // pergunta bloqueante pendente
}

// PermissionMode: como o CC trata permissões. 'auto' = o agente decide (pede só
// quando precisa); 'default' = pergunta toda ferramenta; 'bypassPermissions' = nunca.
export type PermissionMode = 'auto' | 'default' | 'acceptEdits' | 'bypassPermissions' | 'plan';

// createEngineSession cria uma sessão dirigida pelo motor stream-json (sem PTY):
// a Home a gerencia 100% pelo canal AG-UI. mode = modo de permissão do CC.
export function createEngineSession(projectId?: string, mode?: PermissionMode): Promise<Session> {
  return req('/sessions/engine', {
    method: 'POST',
    body: JSON.stringify({ project_id: projectId ?? '', mode: mode ?? 'auto' }),
  });
}

export function getInteraction(sessionId: string): Promise<InteractionSnapshot> {
  return req(`/sessions/${sessionId}/interaction`);
}

// respondInteraction responde uma pergunta bloqueante (interrupt) pelo request_id.
export function respondInteraction(sessionId: string, requestId: string, answer: string): Promise<void> {
  return req(`/sessions/${sessionId}/interaction/respond`, {
    method: 'POST',
    body: JSON.stringify({ request_id: requestId, answer }),
  });
}

// sendPrompt injeta um novo prompt no PTY quando a sessão está ociosa.
export function sendPrompt(sessionId: string, text: string): Promise<void> {
  return req(`/sessions/${sessionId}/interaction/prompt`, {
    method: 'POST',
    body: JSON.stringify({ text }),
  });
}
