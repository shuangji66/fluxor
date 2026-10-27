import { ref, type Ref } from 'vue'
import { wsConnect } from './api'

/**
 * 一路实时流（WebSocket）的生命周期封装。
 *
 * 项目里有四路实时流（traffic / memory / connections / logs），此前各写了一份
 * 「订阅计数 + 防抖断开 + 断线重连」的样板：逻辑相同、细节却已经跑偏
 * （日志用指数退避、其余固定 5 秒；有的 onError 置空句柄、有的没有）。
 * 一旦某处发现「重连后事件重复订阅」这类问题，要改四处。
 *
 * 本封装固定这些语义：
 * - 引用计数：第一个订阅者接入才建立连接，最后一个退订后防抖 3 秒再断开
 *   （切页/切标签时短暂退订不应触发重连）；
 * - 断开重连：仅在仍有订阅者时重连，退避按配置（默认固定 5s，日志用指数退避）；
 *   收到 `open` 后退避归零；
 * - 状态单一来源：`connected` 由本封装维护，调用方不要再自行维护布尔量；
 * - 幂等：`disconnect()` 可重复调用，定时器都会被清理。
 */
export interface RealtimeStreamOptions {
  /** 后端路由，如 `/traffic` */
  path: string
  /** 单帧处理（不做 try/catch，异常会中断本帧，由 wsConnect 内部捕获） */
  onMessage: (ev: MessageEvent) => void
  /** 连接状态变化（UI 展示用） */
  onStatusChange?: (connected: boolean) => void
  /** 已退订且连接关闭后的清理（例如把速度/内存归零） */
  onIdle?: () => void
  /** 重连节奏：默认固定 5 秒；日志流用 { initialDelayMs: 1000, maxDelayMs: 30000, factor: 2 } */
  reconnect?: { initialDelayMs: number; maxDelayMs?: number; factor?: number }
  /** 退订到断开的防抖，默认 3000ms */
  idleDisconnectDelayMs?: number
}

export interface RealtimeStream {
  connected: Ref<boolean>
  subscriberCount: Ref<number>
  subscribe: () => void
  unsubscribe: () => void
  /** 立即断开（忽略订阅计数），用于页面销毁等场景 */
  disconnect: () => void
}

export function createRealtimeStream(options: RealtimeStreamOptions): RealtimeStream {
  const {
    path,
    onMessage,
    onStatusChange,
    onIdle,
    reconnect = { initialDelayMs: 5000 },
    idleDisconnectDelayMs = 3000,
  } = options

  const maxDelay = reconnect.maxDelayMs ?? reconnect.initialDelayMs
  const factor = reconnect.factor ?? 1

  const connected = ref(false)
  const subscriberCount = ref(0)
  let ws: WebSocket | null = null
  let reconnectDelay = reconnect.initialDelayMs
  let reconnectTimer: number | null = null
  let idleTimer: number | null = null

  const setConnected = (value: boolean) => {
    if (connected.value === value) return
    connected.value = value
    onStatusChange?.(value)
  }

  const clearTimer = (timer: number | null) => {
    if (timer !== null) clearTimeout(timer)
    return null
  }

  const connect = () => {
    if (ws || reconnectTimer !== null) return
    ws = wsConnect(path, onMessage, {
      onOpen: () => {
        reconnectDelay = reconnect.initialDelayMs
        setConnected(true)
      },
      onError: () => {
        ws = null
        setConnected(false)
      },
      onClose: () => {
        ws = null
        setConnected(false)
        if (subscriberCount.value === 0) {
          onIdle?.()
          return
        }
        // 仍有订阅者：按退避重连（指数退避用于日志这类高频流，避免服务未就绪时刷屏）
        reconnectTimer = window.setTimeout(() => {
          reconnectTimer = null
          if (subscriberCount.value > 0) connect()
        }, reconnectDelay)
        reconnectDelay = Math.min(reconnectDelay * factor, maxDelay)
      },
    })
  }

  const disconnect = () => {
    idleTimer = clearTimer(idleTimer)
    reconnectTimer = clearTimer(reconnectTimer)
    reconnectDelay = reconnect.initialDelayMs
    if (ws) {
      ws.close()
      ws = null
    }
    setConnected(false)
  }

  const subscribe = () => {
    idleTimer = clearTimer(idleTimer)
    subscriberCount.value++
    if (subscriberCount.value === 1) connect()
  }

  const unsubscribe = () => {
    subscriberCount.value = Math.max(0, subscriberCount.value - 1)
    if (subscriberCount.value > 0) return
    idleTimer = window.setTimeout(() => {
      idleTimer = null
      if (subscriberCount.value === 0) disconnect()
    }, idleDisconnectDelayMs)
  }

  return { connected, subscriberCount, subscribe, unsubscribe, disconnect }
}
