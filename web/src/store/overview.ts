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
  currentGroup: string
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
    currentNode: '加载中...',
    currentGroup: '加载中...'
  })

  const uploadHistory = ref<number[]>([])
  const downloadHistory = ref<number[]>([])
  const timeHistory = ref<string[]>([])
  const uiPanel = ref('metacubexd')
  const isTrafficConnected = ref(false)
  const isMemoryConnected = ref(false)

  // === Traffic WS ===
  let wsTraffic: WebSocket | null = null
  const trafficSubscribers = ref(0)
  let trafficDebounce: any = null

  const connectTraffic = () => {
    if (wsTraffic) return
    wsTraffic = wsConnect('/traffic', (e: MessageEvent) => {
      isTrafficConnected.value = true
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
      onOpen: () => {
        isTrafficConnected.value = true
      },
      onClose: () => {
        wsTraffic = null
        isTrafficConnected.value = false
        if (trafficSubscribers.value > 0) {
          setTimeout(() => {
            if (trafficSubscribers.value > 0) connectTraffic()
          }, 5000)
        } else {
          stats.value.uploadSpeed = 0
          stats.value.downloadSpeed = 0
        }
      },
      onError: () => {
        wsTraffic = null
        isTrafficConnected.value = false
      }
    })
  }

  const disconnectTraffic = () => {
    if (wsTraffic) {
      wsTraffic.close()
      wsTraffic = null
    }
    isTrafficConnected.value = false
    stats.value.uploadSpeed = 0
    stats.value.downloadSpeed = 0
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
      isMemoryConnected.value = true
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
      onOpen: () => {
        isMemoryConnected.value = true
      },
      onClose: () => {
        wsMemory = null
        isMemoryConnected.value = false
        if (memorySubscribers.value > 0) {
          setTimeout(() => {
            if (memorySubscribers.value > 0) connectMemory()
          }, 5000)
        } else {
          stats.value.memory = 0
        }
      },
      onError: () => {
        wsMemory = null
        isMemoryConnected.value = false
      }
    })
  }

  const disconnectMemory = () => {
    if (wsMemory) {
      wsMemory.close()
      wsMemory = null
    }
    isMemoryConnected.value = false
    stats.value.memory = 0
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

  // 防并发状态锁，避免重叠发起轮询请求
  let isFetchingStatus = false

  const fetchVersionAndStatus = async () => {
    if (isFetchingStatus) return
    isFetchingStatus = true
    try {
      // 优化：仅当未成功获取过版本号时才拉取，避免每次轮询重复发送静态请求
      const hasVersion = stats.value.coreVersion !== '加载中...' && stats.value.coreVersion !== '未知'
      const [versionResp, statusResp, proxiesResp] = await Promise.all([
        hasVersion ? Promise.resolve(null) : apiFetch('/version').catch(() => null),
        apiFetch('/core/status').catch(() => null),
        apiFetch('/proxies').catch(() => null)
      ])

      let isRunning = false
      if (statusResp && statusResp.ok) {
        const s = await statusResp.json()
        isRunning = s.running
      }

      if (!isRunning) {
        stats.value.currentNode = '内核未启动'
        stats.value.currentGroup = '内核未启动'
        stats.value.uploadSpeed = 0
        stats.value.downloadSpeed = 0
        stats.value.memory = 0
        // 内核停止时，重置版本号为“加载中...”，以便下次启动时能重新请求
        stats.value.coreVersion = '加载中...'
      } else {
        if (!hasVersion) {
          if (versionResp && versionResp.ok) {
            const v = await versionResp.json()
            stats.value.coreVersion = (v.version || '').replace(/^v/, '')
          } else {
            stats.value.coreVersion = '未知'
          }
        }

        // 在 fetchVersionAndStatus 中，替换查找主组的逻辑
        if (proxiesResp && proxiesResp.ok) {
          const data = await proxiesResp.json()
          const entries = Object.entries(data.proxies || {}) as [string, any][]
          // 优先查找名称包含"节点选择"的 Selector 组
          let targetGroup = entries.find(([, g]) => g.type === 'Selector' && g.name && g.name.includes('节点选择'))
          // 若没有，则取第一个非 GLOBAL 的 Selector 组
          if (!targetGroup) {
            targetGroup = entries.find(([, g]) => g.type === 'Selector' && g.name && g.name !== 'GLOBAL')
          }
          if (targetGroup) {
            const selected = targetGroup[1].now || '-'
            stats.value.currentNode = recursiveResolveNode(data.proxies, selected)
            stats.value.currentGroup = targetGroup[0]
          } else {
            stats.value.currentNode = '暂无选择'
            stats.value.currentGroup = '暂无选择'
          }
        }
      }
    } catch (e) {
      console.warn('定时获取状态异常', e)
      stats.value.currentNode = '内核未启动'
      stats.value.currentGroup = '内核未启动'
      stats.value.uploadSpeed = 0
      stats.value.downloadSpeed = 0
      stats.value.memory = 0
    } finally {
      isFetchingStatus = false
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

  // 将数据压入历史队列（最长为65个点）
  const pushHistory = (up: number, down: number, maxPoints = 65) => {
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
    isTrafficConnected,
    isMemoryConnected,
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
