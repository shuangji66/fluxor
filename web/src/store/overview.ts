import { defineStore } from 'pinia'
import { ref } from 'vue'
import { wsConnect, apiFetch } from '../utils/api'

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
  const timeHistory = ref<string[]>([])
  const uiPanel = ref('metacubexd')

  // === Traffic WS ===
  let wsTraffic: WebSocket | null = null
  const trafficSubscribers = ref(0)
  let trafficDebounce: any = null

  const connectTraffic = () => {
    if (wsTraffic) return
    wsTraffic = wsConnect('/traffic', (e: MessageEvent) => {
      let up = 0, down = 0
      if (typeof e.data === 'string') {
        const d = JSON.parse(e.data)
        up = d.up || d.upload || 0
        down = d.down || d.download || 0
      } else if (e.data instanceof ArrayBuffer) {
        const v = new DataView(e.data)
        up = Number(v.getBigUint64(0, false))
        down = Number(v.getBigUint64(8, false))
      }
      stats.value.uploadSpeed = up
      stats.value.downloadSpeed = down
      pushHistory(up, down)
    }, {
      onClose: () => {
        wsTraffic = null
        if (trafficSubscribers.value > 0) {
          setTimeout(() => {
            if (trafficSubscribers.value > 0) connectTraffic()
          }, 5000)
        }
      },
      onError: () => {
        wsTraffic = null
      }
    })
  }

  const disconnectTraffic = () => {
    if (wsTraffic) {
      wsTraffic.close()
      wsTraffic = null
    }
  }

  const subscribeTraffic = () => {
    if (trafficDebounce) {
      clearTimeout(trafficDebounce)
      trafficDebounce = null
    }
    trafficSubscribers.value++
    if (trafficSubscribers.value === 1) {
      connectTraffic()
    }
  }

  const unsubscribeTraffic = () => {
    trafficSubscribers.value = Math.max(0, trafficSubscribers.value - 1)
    if (trafficSubscribers.value === 0) {
      trafficDebounce = setTimeout(() => {
        if (trafficSubscribers.value === 0) {
          disconnectTraffic()
        }
      }, 3000)
    }
  }

  // === Memory WS ===
  let wsMemory: WebSocket | null = null
  const memorySubscribers = ref(0)
  let memoryDebounce: any = null

  const connectMemory = () => {
    if (wsMemory) return
    wsMemory = wsConnect('/memory', (e: MessageEvent) => {
      let mem = 0
      if (typeof e.data === 'string') {
        const d = JSON.parse(e.data)
        mem = d.inuse || d.memory || 0
      } else if (e.data instanceof ArrayBuffer) {
        mem = Number(new DataView(e.data).getBigUint64(0, false))
      }
      if (mem > 0) {
        stats.value.memory = mem
      }
    }, {
      onClose: () => {
        wsMemory = null
        if (memorySubscribers.value > 0) {
          setTimeout(() => {
            if (memorySubscribers.value > 0) connectMemory()
          }, 5000)
        }
      },
      onError: () => {
        wsMemory = null
      }
    })
  }

  const disconnectMemory = () => {
    if (wsMemory) {
      wsMemory.close()
      wsMemory = null
    }
  }

  const subscribeMemory = () => {
    if (memoryDebounce) {
      clearTimeout(memoryDebounce)
      memoryDebounce = null
    }
    memorySubscribers.value++
    if (memorySubscribers.value === 1) {
      connectMemory()
    }
  }

  const unsubscribeMemory = () => {
    memorySubscribers.value = Math.max(0, memorySubscribers.value - 1)
    if (memorySubscribers.value === 0) {
      memoryDebounce = setTimeout(() => {
        if (memorySubscribers.value === 0) {
          disconnectMemory()
        }
      }, 3000)
    }
  }

  // === Status Polling ===
  const statusSubscribers = ref(0)
  let statusTimer: any = null

  const recursiveResolveNode = (proxies: Record<string, any>, selected: string): string => {
    const entries = Object.entries(proxies || {}) as [string, any][]
    let current = selected
    let maxLoop = 10
    while (maxLoop-- > 0) {
      const found = entries.find(([name, g]) => name === current && (g.type === 'Selector' || g.type === 'URLTest'))
      if (found) {
        current = found[1].now || '-'
      } else {
        break
      }
    }
    return current
  }

  const fetchVersionAndStatus = async () => {
    try {
      const [versionResp, statusResp, proxiesResp] = await Promise.all([
        apiFetch('/version').catch(() => null),
        apiFetch('/core/status').catch(() => null),
        apiFetch('/proxies').catch(() => null)
      ])

      if (versionResp && versionResp.ok) {
        const v = await versionResp.json()
        stats.value.coreVersion = (v.version || '').replace(/^v/, '')
      } else {
        stats.value.coreVersion = '未知'
      }

      if (statusResp && statusResp.ok) {
        const s = await statusResp.json()
        if (!s.running) {
          stats.value.currentNode = '内核未启动'
        }
      }

      if (proxiesResp && proxiesResp.ok) {
        const data = await proxiesResp.json()
        const entries = Object.entries(data.proxies || {}) as [string, any][]
        const mainGroup = entries.find(([, g]) => g.type === 'Selector' && g.name && g.name.includes('节点选择'))
        if (mainGroup) {
          stats.value.currentNode = recursiveResolveNode(data.proxies, mainGroup[1].now || '-')
        } else {
          stats.value.currentNode = '暂无选择'
        }
      }
    } catch (e) {
      console.warn('定时获取状态异常', e)
    }
  }

  const subscribeStatus = () => {
    statusSubscribers.value++
    if (statusSubscribers.value === 1) {
      fetchVersionAndStatus()
      statusTimer = setInterval(fetchVersionAndStatus, 10000)
    }
  }

  const unsubscribeStatus = () => {
    statusSubscribers.value = Math.max(0, statusSubscribers.value - 1)
    if (statusSubscribers.value === 0) {
      if (statusTimer) {
        clearInterval(statusTimer)
        statusTimer = null
      }
    }
  }

  // 将数据压入历史队列（最长为60个点）
  const pushHistory = (up: number, down: number, maxPoints = 60) => {
    uploadHistory.value.push(up)
    downloadHistory.value.push(down)
    
    const now = new Date()
    const timeStr = `${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}:${String(now.getSeconds()).padStart(2, '0')}`
    timeHistory.value.push(timeStr)

    if (uploadHistory.value.length > maxPoints) uploadHistory.value.shift()
    if (downloadHistory.value.length > maxPoints) downloadHistory.value.shift()
    if (timeHistory.value.length > maxPoints) timeHistory.value.shift()
  }

  return {
    stats,
    uploadHistory,
    downloadHistory,
    timeHistory,
    uiPanel,
    pushHistory,
    subscribeTraffic,
    unsubscribeTraffic,
    subscribeMemory,
    unsubscribeMemory,
    subscribeStatus,
    unsubscribeStatus,
    fetchVersionAndStatus
  }
})
