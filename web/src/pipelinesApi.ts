import type { Skill } from './api';

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

export interface PipelineStep {
  skill_id: string;
  note: string;
  inputs: string;
  credentials: string;
}

export interface Pipeline {
  id: string;
  project_id: string;
  name: string;
  steps: PipelineStep[];
  content?: string;
  created_at: number;
  updated_at: number;
}

// O backend devolve a skill composta com os passos DENTRO de `metadata` (JSON
// {kind:"pipeline", steps:[...]}). Normaliza para o shape que a UI consome.
function toPipeline(s: Record<string, unknown>): Pipeline {
  let steps: PipelineStep[] = [];
  try {
    const meta = JSON.parse((s.metadata as string) || '{}');
    if (Array.isArray(meta.steps)) steps = meta.steps;
  } catch { /* metadata ausente/inválido → sem etapas */ }
  return {
    id: s.id as string,
    project_id: s.project_id as string,
    name: s.name as string,
    steps,
    content: s.content as string | undefined,
    created_at: (s.created_at as number) ?? 0,
    updated_at: (s.updated_at as number) ?? 0,
  };
}

export interface PipelineInput {
  name: string;
  steps: PipelineStep[];
}

/** Skills do projeto — etapas candidatas para o construtor. */
export function listPipelineSkills(projectId: string): Promise<Skill[]> {
  return req(`/skills?project_id=${projectId}`);
}

/** Pipelines existentes (skills compostas) do projeto. */
export async function listPipelines(projectId: string): Promise<Pipeline[]> {
  const raw = await req<Record<string, unknown>[]>(`/projects/${projectId}/pipelines`);
  return (raw ?? []).map(toPipeline);
}

export async function createPipeline(projectId: string, data: PipelineInput): Promise<Pipeline> {
  return toPipeline(await req(`/projects/${projectId}/pipelines`, {
    method: 'POST',
    body: JSON.stringify(data),
  }));
}

export async function updatePipeline(skillId: string, data: PipelineInput): Promise<Pipeline> {
  return toPipeline(await req(`/pipelines/${skillId}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }));
}
