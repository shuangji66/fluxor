import { defineStore } from 'pinia'
import { ref } from 'vue'
import { wsConnect } from '../utils/api'

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

  let ws: WebSocket | null = null
  const subscriberCount = ref(0)
  let debounceTimeout: any = null
  let reconnectDelay = 1000
  const MAX_RECONNECT_DELAY = 30000

  // 格式化时间为 HH:mm:ss.SSS
  const formatTime = (date: Date) => {
    const pad = (n: number) => String(n).padStart(2, '0')
    const ms = String(date.getMilliseconds()).padStart(3, '0')
    return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}.${ms}`
  }

  // 添加单条日志
  const addLog = (log: LogItem, maxLogs = 2000) => {
    logs.value.push(log)
    if (logs.value.length > maxLogs) {
      logs.value.shift()
    }
  }

  // 清空日志
  const clearLogs = () => {
    logs.value = []
  }

  // 建立 WebSocket 连接
  const connect = () => {
    if (ws) return
    ws = wsConnect('/logs', (e: MessageEvent) => {
      if (isPaused.value) return
      let item: LogItem
      try {
        const data = JSON.parse(e.data)
        item = {
          id: Date.now() + Math.random(),
          type: data.type || 'info',
          payload: data.payload || data,
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
    }, {
      onOpen: () => {
        reconnectDelay = 1000
      },
      onClose: () => {
        ws = null
        if (subscriberCount.value > 0) {
          setTimeout(() => {
            if (subscriberCount.value > 0) connect()
          }, reconnectDelay)
          reconnectDelay = Math.min(reconnectDelay * 2, MAX_RECONNECT_DELAY)
        }
      },
      onError: () => {
        ws = null
      }
    })
  }

  // 关闭连接
  const disconnect = () => {
    if (ws) {
      ws.close()
      ws = null
    }
  }

  // 增加引用订阅计数
  const subscribe = () => {
    if (debounceTimeout) {
      clearTimeout(debounceTimeout)
      debounceTimeout = null
    }
    subscriberCount.value++
    if (subscriberCount.value === 1) {
      connect()
    }
  }

  // 减少订阅并防抖断开
  const unsubscribe = () => {
    subscriberCount.value = Math.max(0, subscriberCount.value - 1)
    if (subscriberCount.value === 0) {
      debounceTimeout = setTimeout(() => {
        if (subscriberCount.value === 0) {
          disconnect()
        }
      }, 3000)
    }
  }

  return {
    logs,
    autoScroll,
    isPaused,
    addLog,
    clearLogs,
    subscribe,
    unsubscribe
  }
})

