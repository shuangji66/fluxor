import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { wsConnect } from '../utils/api'

export interface ConnectionMetadata {
  host: string
  destinationIP: string
  destinationPort: number
  sourcePort: number
  type: string
  network: string
}

export interface ConnectionItem {
  id: string
  metadata: ConnectionMetadata
  rule: string
  chains: string[]
  upload: number
  download: number
  start: string
  speedUp?: number
  speedDown?: number
  closedAt?: string
  closedAtTimestamp?: number
}

export const useConnectionsStore = defineStore('connections', () => {
  const activeConnections = ref<ConnectionItem[]>([])
  const closedConnections = ref<ConnectionItem[]>([])
  const isPaused = ref(false)
  
  // 共享统计指标
  const connectionsCount = ref(0)
  const uploadTotal = ref(0)
  const downloadTotal = ref(0)

  let ws: WebSocket | null = null
  const subscriberCount = ref(0)
  const isWsConnected = ref(false)
  let debounceTimeout: any = null
  let isFirstFrame = true
  const prevSnapshot = new Map<string, { upload: number, download: number, timestamp: number }>()

  watch(isPaused, (newVal) => {
    if (!newVal) {
      isFirstFrame = true
    }
  })

  // 建立 WebSocket 连接
  const connect = () => {
    if (ws) return
    isFirstFrame = true
    ws = wsConnect('/connections', (e: MessageEvent) => {
      if (isPaused.value) return
      try {
        const data = JSON.parse(e.data)
        const newActiveList: any[] = data.connections || []
        const now = performance.now()

        connectionsCount.value = newActiveList.length
        if (data.uploadTotal !== undefined && data.downloadTotal !== undefined) {
          uploadTotal.value = data.uploadTotal
          downloadTotal.value = data.downloadTotal
        }

        // 计算瞬时速率
        const newActiveMap = new Map<string, ConnectionItem>()
        newActiveList.forEach(conn => {
          const prev = prevSnapshot.get(conn.id)
          let speedUp = 0
          let speedDown = 0
          if (prev) {
            const timeDiff = (now - prev.timestamp) / 1000
            if (timeDiff > 0.05) {
              speedUp = Math.max(0, (conn.upload - prev.upload) / timeDiff)
              speedDown = Math.max(0, (conn.download - prev.download) / timeDiff)
            }
          }
          newActiveMap.set(conn.id, {
            ...conn,
            speedUp,
            speedDown
          })
        })

        // 仅在非首帧进行差集归档，防止重连或暂停恢复时陈旧快照误判
        if (!isFirstFrame) {
          activeConnections.value.forEach(prevConn => {
            if (!newActiveMap.has(prevConn.id)) {
              closedConnections.value.unshift({
                ...prevConn,
                speedUp: 0,
                speedDown: 0,
                closedAt: new Date().toLocaleTimeString(),
                closedAtTimestamp: Date.now()
              })
            }
          })

          if (closedConnections.value.length > 100) {
            closedConnections.value = closedConnections.value.slice(0, 100)
          }
        } else {
          isFirstFrame = false
        }

        // 更新上一帧快照
        prevSnapshot.clear()
        newActiveList.forEach(conn => {
          prevSnapshot.set(conn.id, {
            upload: conn.upload,
            download: conn.download,
            timestamp: now
          })
        })

        activeConnections.value = Array.from(newActiveMap.values())
      } catch (err) {
        console.warn('解析连接失败', err)
      }
    }, {
      onOpen: () => {
        isWsConnected.value = true
      },
      onClose: () => {
        ws = null
        isWsConnected.value = false
        prevSnapshot.clear()
        if (subscriberCount.value > 0) {
          setTimeout(() => {
            if (subscriberCount.value > 0) connect()
          }, 5000)
        } else {
          activeConnections.value = []
          connectionsCount.value = 0
        }
      },
      onError: () => {
        ws = null
        isWsConnected.value = false
      }
    })
  }

  // 关闭连接
  const disconnect = () => {
    if (ws) {
      ws.close()
      ws = null
    }
    isWsConnected.value = false
    prevSnapshot.clear()
    activeConnections.value = []
    connectionsCount.value = 0
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

  // 清空已关闭记录
  const clearClosedConnections = () => {
    closedConnections.value = []
  }

  // 移除单条已关闭记录
  const removeClosedConnection = (id: string) => {
    closedConnections.value = closedConnections.value.filter(c => c.id !== id)
  }

  return {
    activeConnections,
    closedConnections,
    connectionsCount,
    uploadTotal,
    downloadTotal,
    isPaused,
    isWsConnected,
    subscribe,
    unsubscribe,
    clearClosedConnections,
    removeClosedConnection
  }
})
