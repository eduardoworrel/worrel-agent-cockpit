import { useEffect, useRef } from 'react';

export interface WsEvent { type: string; payload: unknown; }

export function useEvents(onEvent: (ev: WsEvent) => void) {
  const cbRef = useRef(onEvent);
  cbRef.current = onEvent;
  useEffect(() => {
    let ws: WebSocket;
    let delay = 1000;
    let stopped = false;
    function connect() {
      const proto = location.protocol === 'https:' ? 'wss' : 'ws';
      ws = new WebSocket(`${proto}://${location.host}/api/events`);
      ws.onmessage = (e) => { try { cbRef.current(JSON.parse(e.data)); } catch {} };
      ws.onclose = () => { if (!stopped) { setTimeout(connect, delay); delay = Math.min(delay * 2, 5000); } };
      ws.onopen = () => { delay = 1000; };
    }
    connect();
    return () => { stopped = true; ws?.close(); };
  }, []);
}
