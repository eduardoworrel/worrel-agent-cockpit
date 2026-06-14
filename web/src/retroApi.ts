// Cliente tipado para os endpoints /api/retro (fase 8 — análise retroativa).

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

export interface CLIInventory {
  sessions: number;
  already_known: number;
  oldest_ms: number;
  newest_ms: number;
}

export interface FolderGroup {
  dir: string;
  sessions: number;
  cli: string;
}

export interface InventoryReport {
  per_cli: Record<string, CLIInventory>;
  folders: FolderGroup[];
  estimated_invocations: number;
}

export interface RetroRun {
  id: string;
  status: string;
  depth: string;
  scope: string;
  budget_per_hour: number;
  budget_total: number;
  llm_calls: number;
  created_at: number;
  updated_at: number;
}

export interface RetroCluster {
  id: string;
  run_id: string;
  name: string;
  description: string;
  existing_project_id: string | null;
  dirs: string;
  session_ids: string;
  decision: string;
  approved_project_id: string | null;
  created_at: number;
}

// Payload do evento WS retro.run.progress emitido pelo executor.
export interface RetroRunProgress {
  run_id: string;
  project_id?: string;
  batch?: number;
  done: number;
  total: number;
}

export interface Scope {
  clis: string[];
  dirs: string[];
  window_days: number;
  adapter?: string;
  model?: string;
}

export interface Suggestion {
  id: string;
  project_id: string;
  type: string;
  status: string;
  title: string;
  payload: string;
  evidence: string;
  origin: string;
  created_at: number;
}

export interface TypeGroup {
  type: string;
  items: Suggestion[];
}

export interface ProjectGroup {
  project_id: string;
  groups: TypeGroup[];
}

// listAdapterModels busca os modelos disponíveis para um provider.
// Endpoint em construção por outro agente: GET /api/adapters/{provider}/models.
// Em 404/erro/resposta inesperada, retorna [] para a UI degradar p/ texto livre.
export async function listAdapterModels(provider: string): Promise<string[]> {
  if (!provider) return [];
  try {
    const res = await fetch(`${BASE}/adapters/${encodeURIComponent(provider)}/models`, {
      headers: { 'Content-Type': 'application/json' },
    });
    if (!res.ok) return [];
    const data = await res.json();
    // Aceita { models: string[] } ou string[] ou [{id|name}].
    const arr = Array.isArray(data) ? data : Array.isArray(data?.models) ? data.models : [];
    return arr
      .map((m: unknown) =>
        typeof m === 'string' ? m : (m as { id?: string; name?: string })?.id ?? (m as { name?: string })?.name ?? '',
      )
      .filter((s: string) => !!s);
  } catch {
    return [];
  }
}

export function inventory(windowDays = 0): Promise<InventoryReport> {
  return req('/retro/inventory', {
    method: 'POST',
    body: JSON.stringify({ window_days: windowDays }),
  });
}

// startInventory dispara o scan assíncrono; o progresso chega via WS (useEvents)
// nos eventos retro.inventory.progress e retro.inventory.done.
export function startInventory(windowDays = 0): Promise<{ status: string }> {
  return req('/retro/inventory/start', {
    method: 'POST',
    body: JSON.stringify({ window_days: windowDays }),
  });
}

export function createRun(scope: Scope, depth: string, budgetPerHour: number, budgetTotal: number): Promise<RetroRun> {
  return req('/retro/runs', {
    method: 'POST',
    body: JSON.stringify({ scope, depth, budget_per_hour: budgetPerHour, budget_total: budgetTotal }),
  });
}

export function listRuns(): Promise<RetroRun[]> {
  return req('/retro/runs');
}

export function getRun(id: string): Promise<RetroRun> {
  return req(`/retro/runs/${id}`);
}

export function cluster(runId: string): Promise<RetroCluster[]> {
  return req(`/retro/runs/${runId}/cluster`, { method: 'POST', body: '{}' });
}

export function listClusters(runId: string): Promise<RetroCluster[]> {
  return req(`/retro/runs/${runId}/clusters`);
}

export function approveCluster(clusterId: string, rename = ''): Promise<{ project_id: string }> {
  return req(`/retro/clusters/${clusterId}/approve`, { method: 'POST', body: JSON.stringify({ rename }) });
}

export function discardCluster(clusterId: string): Promise<unknown> {
  return req(`/retro/clusters/${clusterId}/discard`, { method: 'POST', body: '{}' });
}

export function mergeClusters(runId: string, clusterIds: string[], name = '', existingProjectId = ''): Promise<{ project_id: string }> {
  return req(`/retro/runs/${runId}/merge`, {
    method: 'POST',
    body: JSON.stringify({ cluster_ids: clusterIds, name, existing_project_id: existingProjectId }),
  });
}

export function startRun(runId: string): Promise<unknown> {
  return req(`/retro/runs/${runId}/start`, { method: 'POST', body: '{}' });
}

export function resumeRun(runId: string): Promise<unknown> {
  return req(`/retro/runs/${runId}/resume`, { method: 'POST', body: '{}' });
}

export function pauseRun(runId: string): Promise<unknown> {
  return req(`/retro/runs/${runId}/pause`, { method: 'POST', body: '{}' });
}

export function cancelRun(runId: string): Promise<unknown> {
  return req(`/retro/runs/${runId}/cancel`, { method: 'POST', body: '{}' });
}

export function batchView(runId: string): Promise<ProjectGroup[]> {
  return req(`/retro/runs/${runId}/batch`);
}

export function bulkResolve(runId: string, projectId: string, type: string, action: string): Promise<{ resolved: number }> {
  return req(`/retro/runs/${runId}/bulk`, {
    method: 'POST',
    body: JSON.stringify({ project_id: projectId, type, action }),
  });
}
