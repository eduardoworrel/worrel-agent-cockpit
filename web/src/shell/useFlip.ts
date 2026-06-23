import { useLayoutEffect, useRef } from 'react';

// FLIP (First-Last-Invert-Play): anima a reordenação dos elementos registrados.
// Recebe as chaves na ordem atual; quando a posição vertical de um elemento muda
// entre renders, mede o delta e reproduz o deslocamento via Web Animations API —
// assim o grupo "desliza" para a nova posição em vez de teletransportar.
export function useFlip(keys: string[]) {
  const nodes = useRef(new Map<string, HTMLElement>());
  const prev = useRef(new Map<string, number>());

  useLayoutEffect(() => {
    const next = new Map<string, number>();
    nodes.current.forEach((el, key) => next.set(key, el.getBoundingClientRect().top));
    next.forEach((top, key) => {
      const before = prev.current.get(key);
      const el = nodes.current.get(key);
      if (before == null || !el || typeof el.animate !== 'function') return;
      const dy = before - top;
      if (Math.abs(dy) < 1) return;
      el.animate(
        [{ transform: `translateY(${dy}px)` }, { transform: 'translateY(0)' }],
        { duration: 220, easing: 'cubic-bezier(.2,.7,.2,1)' },
      );
    });
    prev.current = next;
  }, [keys.join('|')]);

  // Callback ref por chave: registra/desregistra o nó no mapa.
  return (key: string) => (el: HTMLElement | null) => {
    if (el) nodes.current.set(key, el);
    else nodes.current.delete(key);
  };
}
