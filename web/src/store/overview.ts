import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface DashboardStats {
  uploadSpeed: number
  downloadSpeed: number
  uploadTotal: number
  downloadTotal: number
  memory: number
  connectionsCount: number
  coreVersion: string
  currentNode: string
}

export const useOverviewStore = defineStore('overview', () => {
  const stats = ref<DashboardStats>({
    uploadSpeed: 0,
    downloadSpeed: 0,
    uploadTotal: 0,
    downloadTotal: 0,
    memory: 0,
    connectionsCount: 0,
    coreVersion: '加载中...',
    currentNode: '加载中...'
  })

  const uploadHistory = ref<number[]>([])
  const downloadHistory = ref<number[]>([])
  const uiPanel = ref('metacubexd')

  // 将数据压入历史队列（最长为60个点）
  const pushHistory = (up: number, down: number, maxPoints = 60) => {
    uploadHistory.value.push(up)
    downloadHistory.value.push(down)
    if (uploadHistory.value.length > maxPoints) uploadHistory.value.shift()
    if (downloadHistory.value.length > maxPoints) downloadHistory.value.shift()
  }

  return {
    stats,
    uploadHistory,
    downloadHistory,
    uiPanel,
    pushHistory
  }
})
