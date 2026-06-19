import { defineStore } from 'pinia'
import { ref } from 'vue'

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

  // 添加单条日志
  const addLog = (log: LogItem, maxLogs = 500) => {
    logs.value.push(log)
    if (logs.value.length > maxLogs) {
      logs.value.shift()
    }
  }

  // 清空日志
  const clearLogs = () => {
    logs.value = []
  }

  return {
    logs,
    autoScroll,
    isPaused,
    addLog,
    clearLogs
  }
})
