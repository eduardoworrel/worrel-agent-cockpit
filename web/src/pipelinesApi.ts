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
  created_at: number;
  updated_at: number;
}

export interface PipelineInput {
  name: string;
  steps: PipelineStep[];
}

/** Skills do projeto — etapas candidatas para o construtor. */
export function listPipelineSkills(projectId: string): Promise<Skill[]> {
  return req(`/projects/${projectId}/skills`);
}

/** Pipelines existentes (skills compostas) do projeto. */
export function listPipelines(projectId: string): Promise<Pipeline[]> {
  return req(`/projects/${projectId}/pipelines`);
}

export function createPipeline(projectId: string, data: PipelineInput): Promise<Pipeline> {
  return req(`/projects/${projectId}/pipelines`, {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export function updatePipeline(skillId: string, data: PipelineInput): Promise<Pipeline> {
  return req(`/pipelines/${skillId}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}
