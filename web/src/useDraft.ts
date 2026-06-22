import { useCallback, useEffect, useState } from 'react';

// useDraft mantém um rascunho de texto por chave (ex.: id da sessão) no
// localStorage. Assim você navega entre terminais sem perder o que digitou;
// o rascunho some quando é esvaziado/enviado.
const PREFIX = 'cockpit.draft.';

export function useDraft(key: string | undefined): [string, (v: string) => void, () => void] {
  const storageKey = key ? PREFIX + key : '';
  const [text, setTextState] = useState('');

  // Ao trocar de sessão, recarrega o rascunho daquela sessão (ou vazio).
  useEffect(() => {
    if (!storageKey) { setTextState(''); return; }
    setTextState(localStorage.getItem(storageKey) ?? '');
  }, [storageKey]);

  const setText = useCallback((v: string) => {
    setTextState(v);
    if (!storageKey) return;
    if (v) localStorage.setItem(storageKey, v);
    else localStorage.removeItem(storageKey);
  }, [storageKey]);

  const clear = useCallback(() => {
    setTextState('');
    if (storageKey) localStorage.removeItem(storageKey);
  }, [storageKey]);

  return [text, setText, clear];
}
