// Cliente tipado para os endpoints /api/chat (Chat de Destilação).
// Endpoints sendo construídos no backend; este módulo compila isolado.

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

export interface ChatScope {
  project_id?: string;
  window_days?: number;
}

export interface ChatThread {
  id: string;
  title?: string;
  scope?: ChatScope | null;
  provider?: string;
  model?: string;
  created_at?: number;
  updated_at?: number;
  message_count?: number;
}

export interface ChatSource {
  session_id: string;
  title?: string;
  adapter?: string;
  snippet?: string;
  score?: number;
}

export type SuggestionType = 'pipeline' | string;

export interface CreatedSuggestion {
  id: string;
  type: SuggestionType;
  title: string;
}

export interface ChatMessage {
  seq: number;
  role: 'user' | 'assistant' | 'system';
  content: string;
  sources?: ChatSource[] | null;
  created_suggestions?: CreatedSuggestion[] | null;
}

export interface ThreadDetail {
  thread: ChatThread;
  messages: ChatMessage[];
}

export interface SendResult {
  assistant: ChatMessage;
  sources: ChatSource[];
  created_suggestions: CreatedSuggestion[];
}

// listAdapterModels busca os modelos disponíveis para um provider.
// Endpoint: GET /api/adapters/{provider}/models. Em 404/erro/resposta
// inesperada retorna [] para a UI degradar p/ texto livre.
export async function listAdapterModels(provider: string): Promise<string[]> {
  if (!provider) return [];
  try {
    const res = await fetch(`${BASE}/adapters/${encodeURIComponent(provider)}/models`, {
      headers: { 'Content-Type': 'application/json' },
    });
    if (!res.ok) return [];
    const data = await res.json();
    const arr = Array.isArray(data) ? data : Array.isArray(data?.models) ? data.models : [];
    return arr
      .map((m: unknown) =>
        typeof m === 'string'
          ? m
          : (m as { id?: string; name?: string })?.id ?? (m as { name?: string })?.name ?? '',
      )
      .filter((s: string) => !!s);
  } catch {
    return [];
  }
}

export interface CreateThreadInput {
  scope?: ChatScope;
  provider?: string;
  model?: string;
}

export function createThread(input: CreateThreadInput = {}): Promise<ChatThread> {
  return req('/chat/threads', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export function listThreads(): Promise<ChatThread[]> {
  return req('/chat/threads').then((r) => (Array.isArray(r) ? (r as ChatThread[]) : []));
}

export function getThread(id: string): Promise<ThreadDetail> {
  return req(`/chat/threads/${encodeURIComponent(id)}`);
}

export interface SendMessageInput {
  text: string;
  provider?: string;
  model?: string;
}

export function sendMessage(threadId: string, input: SendMessageInput): Promise<SendResult> {
  return req(`/chat/threads/${encodeURIComponent(threadId)}/messages`, {
    method: 'POST',
    body: JSON.stringify(input),
  });
}
