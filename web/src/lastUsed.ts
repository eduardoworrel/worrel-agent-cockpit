// lastUsed: persistência leve do "último usado" por provider/projeto, para
// ordenar a nova sessão pelas escolhas mais recentes do usuário.
//
// O estado vive em localStorage num único mapa keyed por "provider:<id>" /
// "project:<id>" → epoch ms. Todo acesso é envolto em try/catch: em ambientes
// sem localStorage (SSR, modo privado) a feature simplesmente degrada (ordem de
// chegada original), sem quebrar o wizard.

const STORAGE_KEY = 'worrel.lastUsed';

export type LastUsedKind = 'provider' | 'project';

function load(): Record<string, number> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === 'object' ? (parsed as Record<string, number>) : {};
  } catch {
    return {};
  }
}

function keyFor(kind: LastUsedKind, id: string): string {
  return `${kind}:${id}`;
}

// markUsed grava Date.now() para o par (kind, id). Falhas de storage são
// silenciosas (degradação graciosa).
export function markUsed(kind: LastUsedKind, id: string): void {
  if (!id) return;
  try {
    const map = load();
    map[keyFor(kind, id)] = Date.now();
    localStorage.setItem(STORAGE_KEY, JSON.stringify(map));
  } catch {
    /* ignore */
  }
}

// orderBy devolve uma NOVA lista ordenada por timestamp de uso desc. Itens sem
// registro vão para o fim preservando a ordem de chegada (ordenação estável).
export function orderBy<T>(kind: LastUsedKind, items: T[], getId: (item: T) => string): T[] {
  const map = load();
  // Index original para desempate estável (Array.sort não é garantidamente
  // estável em toda engine; ancoramos no índice de chegada).
  return items
    .map((item, index) => ({ item, index, ts: map[keyFor(kind, getId(item))] ?? -1 }))
    .sort((a, b) => {
      if (a.ts !== b.ts) return b.ts - a.ts; // mais recente primeiro
      return a.index - b.index; // sem registro / empate: ordem de chegada
    })
    .map((entry) => entry.item);
}
