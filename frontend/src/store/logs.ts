import { defineStore } from 'pinia'
import { ref } from 'vue'
import { createRealtimeStream } from '../utils/realtimeStream'

export interface LogItem {
  id: number
  type: string
  payload: string
  time: string
}

export const useLogStore = defineStore('logs', () => {
  const logs = ref<LogItem[]>([])
  const autoScroll = ref(true)
  const isPaused = ref(false)


  // 格式化时间为 HH:mm:ss.SSS
  const formatTime = (date: Date) => {
    const pad = (n: number) => String(n).padStart(2, '0')
    const ms = String(date.getMilliseconds()).padStart(3, '0')
    return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}.${ms}`
  }

  // === 批量入队 ===
  //
  // 日志是逐条推送的，而每条都是一次响应式写入：旧实现直接 push 到 logs，
  // 满 2000 条后还要 shift() 重建整个数组索引——高密度日志（debug 全开时每秒数十条）
  // 下，每条日志都会触发 2000 项数组的 index 重建与列表 patch，明显掉帧。
  //
  // 现在先进缓冲区，每 100ms 合并成一次数组替换：一次渲染消费一批日志，
  // 超限用一次 slice 截断，不再逐条 shift。暂停/清空时同步丢弃缓冲。
  const MAX_LOGS = 2000
  const FLUSH_INTERVAL_MS = 100
  let pending: LogItem[] = []
  let flushTimer: number | null = null

  const flushPending = () => {
    flushTimer = null
    if (pending.length === 0) return
    const merged = logs.value.concat(pending)
    pending = []
    logs.value = merged.length > MAX_LOGS ? merged.slice(merged.length - MAX_LOGS) : merged
  }

  const scheduleFlush = () => {
    if (flushTimer !== null) return
    flushTimer = window.setTimeout(flushPending, FLUSH_INTERVAL_MS)
  }

  // 添加单条日志（入缓冲，由定时器合并）
  const addLog = (log: LogItem) => {
    if (isPaused.value) return
    pending.push(log)
    scheduleFlush()
  }

  // 清空日志：连缓冲一起清，否则清空后 100ms 内会「复活」一批
  const clearLogs = () => {
    pending = []
    if (flushTimer !== null) {
      clearTimeout(flushTimer)
      flushTimer = null
    }
    logs.value = []
  }

  // 日志流：引用计数与断线重连交给 createRealtimeStream；
  // 日志是高频流，重连用指数退避（1s → 30s），避免内核未就绪时每秒刷一次。
  const stream = createRealtimeStream({
    path: '/logs',
    reconnect: { initialDelayMs: 1000, maxDelayMs: 30000, factor: 2 },
    onMessage: (e: MessageEvent) => {
      let item: LogItem
      try {
        const data = JSON.parse(e.data)
        item = {
          id: Date.now() + Math.random(),
          type: data.type || 'info',
          payload: typeof data.payload === 'string' ? data.payload : JSON.stringify(data.payload ?? data),
          time: formatTime(new Date())
        }
      } catch (err) {
        item = {
          id: Date.now() + Math.random(),
          type: 'info',
          payload: e.data,
          time: formatTime(new Date())
        }
      }
      addLog(item)
    },
  })

  const subscribe = stream.subscribe
  const unsubscribe = stream.unsubscribe

  return {
    logs,
    autoScroll,
    isPaused,
    isConnected: stream.connected,
    addLog,
    clearLogs,
    subscribe,
    unsubscribe
  }
})

