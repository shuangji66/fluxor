import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { createRealtimeStream } from '../utils/realtimeStream'
import type { ConnectionsFrame, ConnectionWire, ConnectionMetadata } from '../types/api'

export type { ConnectionMetadata }

export interface ConnectionItem {
  id: string
  metadata: ConnectionMetadata
  rule: string
  rulePayload?: string
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

  let isFirstFrame = true
  const prevSnapshot = new Map<string, { upload: number, download: number, timestamp: number }>()

  watch(isPaused, (newVal) => {
    if (!newVal) {
      isFirstFrame = true
    }
  })

  // 连接流：引用计数/防抖断开/断线重连由 createRealtimeStream 统一负责，
  // 本 store 只关心「一帧数据怎么算」。
  const stream = createRealtimeStream({
    path: '/connections',
    onMessage: (e: MessageEvent) => {
      if (isPaused.value) return
      try {
        const data = JSON.parse(e.data) as ConnectionsFrame
        const newActiveList: ConnectionWire[] = data.connections || []
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
            metadata: conn.metadata || {},
            chains: conn.chains || [],
            rule: conn.rule || '',
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
    },
    onStatusChange: (connected) => {
      isWsConnected.value = connected
      if (connected) {
        // 断线重连同样属于「首帧」：不清标记的话，重连后的第一帧会拿断线前的
        // 列表做差集，把整批陈旧连接误判成「刚刚关闭」。旧实现在每次 connect()
        // 开头重置该标记，迁到流封装后只能挂在 open 上。
        isFirstFrame = true
      } else {
        prevSnapshot.clear()
      }
    },
    onIdle: () => {
      activeConnections.value = []
      connectionsCount.value = 0
    },
  })

  // 首帧标记：重连/暂停恢复后重新起算，避免把陈旧快照当成「连接已关闭」
  const subscribe = () => {
    isFirstFrame = true
    stream.subscribe()
  }

  const isWsConnected = stream.connected
  const unsubscribe = stream.unsubscribe

  // 立即断开并清空快照（页面销毁等场景）；正常切页走 unsubscribe 的防抖断开
  const disconnect = () => {
    stream.disconnect()
    prevSnapshot.clear()
    activeConnections.value = []
    connectionsCount.value = 0
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
