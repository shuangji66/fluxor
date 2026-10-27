// mock 仅在 DEV 下动态加载：生产构建中 `import.meta.env.DEV` 静态折叠为 false，
// 整个 mock 模块被摇树移除，杜绝离线模拟器进入生产包（安全）
const mock = import.meta.env.DEV ? await import('./mock') : null;

const rawBase = window.BASE_URL || import.meta.env.BASE_URL || '';
// 根路径部署时 import.meta.env.BASE_URL 为 './'，须归一化为空串，
// 否则 withBase 会拼出 '/./configs' 这类非法路径
const BASE = rawBase === './' || rawBase === '.' ? '' : rawBase;

function isMockEnabled(): boolean {
  return mock !== null && localStorage.getItem('MOCK_BACKEND') !== 'false';
}

export function withBase(path: string): string {
  if (!BASE || BASE === '/') return path;
  const base = BASE.replace(/^\/|\/$/g, '');
  return '/' + base + '/' + path.replace(/^\//, '');
}

export async function apiFetch(path: string, options: RequestInit = {}): Promise<Response> {
  if (isMockEnabled() && mock) {
    // 模拟网络延迟（50-150ms 仿真）
    await new Promise(resolve => setTimeout(resolve, 50 + Math.random() * 100));
    return mock.handleMockFetch(path, options);
  }
  try {
    const url = withBase(path);
    const resp = await fetch(url, options);
    return resp;
  } catch (err: any) {
    // 这里刻意**不**拼任何文案：apiFetch 是底层工具，没有语言上下文，硬编码中文会在
    // 英文界面下漏出中文（规约：用户可见文案必须走 i18n）。各调用方已经用
    // t('common.error') / t('common.network_error') 做了本地化前缀，这里只保留
    // 浏览器给出的技术原因（英文），拼出来是「错误: Failed to fetch」——
    // 比原来「错误: 网络错误: Failed to fetch」的双重前缀更清楚，也便于搜索排查。
    throw new Error(err?.message || String(err));
  }
}

/**
 * 从错误响应里取出后端给出的原因。
 *
 * 后端每个失败分支都写了可读的原因（httpx.WriteJSONError 的形状是
 * {"status":"error","message":"..."}，内核透传的失败也带 message），前端许多调用点
 * 却只提示一句「操作失败」，把这条最有用的信息丢掉了。
 *
 * 用 resp.clone() 读取，避免消费掉调用方的响应体（同一个 Response 的 body 只能读一次）。
 * 取不到时退回 `HTTP <status>`，至少能区分 400/404/500。
 */
export async function readErrorMessage(resp: Response): Promise<string> {
  const fallback = `HTTP ${resp.status}`;
  try {
    const data = await resp.clone().json();
    if (data && typeof data.message === 'string' && data.message.trim() !== '') {
      return data.message;
    }
  } catch {
    // 响应体不是 JSON（空体 / 上游错误页）：退回状态码
  }
  return fallback;
}

/**
 * 请求 JSON 接口的便捷封装：非 2xx 时抛出**带后端原因**的错误。
 *
 * 项目里 60 多处手写 `const resp = await apiFetch(...); if (!resp.ok) {...}; await resp.json()`，
 * 其中不少在失败分支只提示「操作失败」，把后端返回的 `{"message":"..."}` 丢掉
 * （比如内核拒绝配置的原因、端口冲突、订阅名校验失败）。这里把「判状态码 + 解析」
 * 合成一步，错误直接由 catch 统一提示。
 *
 * 空响应体（204 / 只回状态码的接口）返回 undefined，避免 JSON.parse('') 抛语法错。
 */
export async function apiJson<T>(path: string, options: RequestInit = {}): Promise<T> {
  const resp = await apiFetch(path, options)
  if (!resp.ok) throw new Error(await readErrorMessage(resp))
  if (resp.status === 204) return undefined as T
  const text = await resp.text()
  if (text.trim() === '') return undefined as T
  return JSON.parse(text) as T
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
  if (isMockEnabled() && mock) {
    const ws = new mock.MockWebSocket('', path) as any;
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

export interface SseHandlers {
  onOpen?: () => void;
  onError?: (ev: Event) => void;
}

// sseConnect 建立 SSE 连接，按事件名分发消息。
//
// 用于后端的内核状态推送（/core/events）。与 wsConnect 不同，SSE 不做
// 握手超时处理：浏览器原生 EventSource 自带断线重连（默认约 3 秒），
// 若在此再叠加超时关闭反而会干扰其重连节奏。
export function sseConnect(
  path: string,
  eventName: string,
  onEvent: (data: any) => void,
  handlers: SseHandlers = {}
): { close: () => void } {
  if (isMockEnabled() && mock) {
    return mock.createMockSse(path, onEvent, handlers);
  }

  const es = new EventSource(withBase(path));

  es.addEventListener('open', () => {
    if (handlers.onOpen) handlers.onOpen();
  });

  es.addEventListener(eventName, (ev: MessageEvent) => {
    try {
      onEvent(JSON.parse(ev.data));
    } catch (err) {
      // 单条负载异常不应中断整个流
    }
  });

  es.addEventListener('error', (ev: Event) => {
    if (handlers.onError) handlers.onError(ev);
  });

  return {
    close: () => es.close()
  };
}
