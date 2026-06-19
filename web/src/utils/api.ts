const BASE = window.BASE_URL || '';

function withBase(path: string): string {
  if (!BASE || BASE === '/') return path;
  return BASE.replace(/\/$/, '') + '/' + path.replace(/^\//, '');
}

export async function apiFetch(path: string, options: RequestInit = {}): Promise<Response> {
  try {
    const url = withBase(path);
    const resp = await fetch(url, options);
    return resp;
  } catch (err: any) {
    throw new Error('网络错误: ' + err.message);
  }
}

export interface WsHandlers {
  onOpen?: () => void;
  onError?: (ev: Event) => void;
  onClose?: (ev: CloseEvent) => void;
}

export function wsConnect(
  path: string,
  onMessage: (ev: MessageEvent) => void,
  handlers: WsHandlers = {}
): WebSocket {
  const wsUrl = (location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + withBase(path);
  const ws = new WebSocket(wsUrl);

  ws.onopen = () => {
    if (handlers.onOpen) handlers.onOpen();
  };

  ws.onmessage = (e) => {
    try {
      onMessage(e);
    } catch (err) {
      // ignore
    }
  };

  ws.onerror = (e) => {
    if (handlers.onError) handlers.onError(e);
  };

  ws.onclose = (e) => {
    if (handlers.onClose) handlers.onClose(e);
  };

  return ws;
}
