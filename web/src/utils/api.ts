import { handleMockFetch, MockWebSocket } from './mock';

const BASE = window.BASE_URL || import.meta.env.BASE_URL || '';

function isMockEnabled(): boolean {
  const devMode = import.meta.env.DEV && localStorage.getItem('MOCK_BACKEND') !== 'false';
  const forceMock = localStorage.getItem('MOCK_BACKEND') === 'true';
  return devMode || forceMock;
}

export function withBase(path: string): string {
  if (!BASE || BASE === '/') return path;
  const base = BASE.replace(/^\/|\/$/g, '');
  return '/' + base + '/' + path.replace(/^\//, '');
}

export async function apiFetch(path: string, options: RequestInit = {}): Promise<Response> {
  if (isMockEnabled()) {
    // 模拟网络延迟（50-150ms 仿真）
    await new Promise(resolve => setTimeout(resolve, 50 + Math.random() * 100));
    return handleMockFetch(path, options);
  }
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
  if (isMockEnabled()) {
    const ws = new MockWebSocket('', path) as any;
    ws.onopen = handlers.onOpen || null;
    ws.onclose = handlers.onClose || null;
    ws.onerror = handlers.onError || null;
    ws.onmessage = (e: any) => {
      try {
        onMessage(e);
      } catch (err) {
        // ignore
      }
    };
    return ws;
  }

  const wsUrl = (location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + withBase(path);
  const ws = new WebSocket(wsUrl);

  // 5秒握手超时计时器
  let isOpened = false;
  const timer = setTimeout(() => {
    if (!isOpened && ws.readyState === WebSocket.CONNECTING) {
      ws.close();
    }
  }, 5000);

  ws.onopen = () => {
    isOpened = true;
    clearTimeout(timer);
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
    clearTimeout(timer);
    if (handlers.onError) handlers.onError(e);
  };

  ws.onclose = (e) => {
    clearTimeout(timer);
    if (handlers.onClose) handlers.onClose(e);
  };

  return ws;
}
