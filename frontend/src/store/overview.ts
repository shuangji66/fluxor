import { defineStore } from 'pinia'
import { ref } from 'vue'
import { sseConnect, apiFetch } from '../utils/api'
import { createRealtimeStream } from '../utils/realtimeStream'
import { useProxyStore } from './proxies'

export interface DashboardStats {
  uploadSpeed: number
  downloadSpeed: number
  memory: number
  coreVersion: string
  currentNode: string
  currentGroup: string
  running: boolean
}

// === 状态哨兵 ===
//
// coreVersion / currentNode / currentGroup 是"状态"而非"文案"：它们既可能承载
// 真实值（内核版本号、节点名），也可能承载"尚未取到/未知/无选择"这类占位状态。
// 占位状态一律使用下列哨兵常量（以 \u0000 开头，不可能与真实版本号或节点名冲突），
// 由视图层映射为 i18n 文案——禁止在 Store 中写入任何自然语言字符串，
// 否则切换语言时中文会直接漏到界面上。
export const CORE_VERSION_LOADING = '\u0000loading'
export const CORE_VERSION_UNKNOWN = '\u0000unknown'
export const NODE_NONE_SELECTED = '\u0000none'
export const NODE_CORE_STOPPED = '\u0000stopped'

export const useOverviewStore = defineStore('overview', () => {
  const stats = ref<DashboardStats>({
    uploadSpeed: 0,
    downloadSpeed: 0,
    memory: 0,
    coreVersion: CORE_VERSION_LOADING,
    currentNode: CORE_VERSION_LOADING,
    currentGroup: CORE_VERSION_LOADING,
    running: false
  })

  const uploadHistory = ref<number[]>([])
  const downloadHistory = ref<number[]>([])
  const timeHistory = ref<string[]>([])
  const uiPanel = ref('metacubexd')

  // === Traffic / Memory 实时流 ===
  //
  // 两者的生命周期（引用计数、防抖断开、断线重连）完全一样，差别只在帧格式与展示字段，
  // 因此共用 createRealtimeStream，只写「一帧怎么解析」。概览页按激活态订阅/退订，
  // 切走页面即断开，不会在别的页面持续重绘图表。
  const trafficStream = createRealtimeStream({
    path: '/traffic',
    onMessage: (e: MessageEvent) => {
      let up = 0
      let down = 0
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
    },
    onIdle: () => {
      stats.value.uploadSpeed = 0
      stats.value.downloadSpeed = 0
    },
  })

  const memoryStream = createRealtimeStream({
    path: '/memory',
    onMessage: (e: MessageEvent) => {
      let mem = 0
      if (typeof e.data === 'string') {
        const d = JSON.parse(e.data)
        mem = d.inuse || d.memory || 0
      } else if (e.data instanceof ArrayBuffer) {
        mem = Number(new DataView(e.data).getBigUint64(0, false))
      }
      if (mem > 0) stats.value.memory = mem
    },
    onIdle: () => {
      stats.value.memory = 0
    },
  })

  const isTrafficConnected = trafficStream.connected
  const isMemoryConnected = memoryStream.connected
  const subscribeTraffic = trafficStream.subscribe
  const unsubscribeTraffic = trafficStream.unsubscribe
  const subscribeMemory = memoryStream.subscribe
  const unsubscribeMemory = memoryStream.unsubscribe

 const syncCurrentNodeFromProxyStore = () => {
    const proxyStore = useProxyStore()
    const groups = proxyStore.proxyGroups
    const allProxies = proxyStore.allProxiesRaw

    // 查找目标组（优先“节点选择”）
    let targetGroup = groups.find(g => g.name.includes('节点选择'))
    if (!targetGroup) {
      targetGroup = groups.find(g => g.type === 'Selector' && g.name !== 'GLOBAL')
    }
    if (targetGroup) {
      stats.value.currentGroup = targetGroup.name
      const selected = targetGroup.now || '-'
      // 递归解析实际节点
      let current = selected
      let maxLoop = 10
      while (maxLoop-- > 0) {
        const node = allProxies[current]
        if (node && (node.type === 'Selector' || node.type === 'URLTest') && node.now) {
          current = node.now
        } else {
          break
        }
      }
      stats.value.currentNode = current
    } else {
      stats.value.currentGroup = NODE_NONE_SELECTED
      stats.value.currentNode = NODE_NONE_SELECTED
    }
  }

  // === Core status (SSE 推送) ===
  //
  // 内核运行状态由后端经 SSE（/core/events）推送，不再轮询、启动时也不再预请求
  // /core/status：该端点接入即下发一次带内核版本的快照，已覆盖首次加载所需信息。
  // 订阅者计数 + 防抖断开，与其它实时流保持一致的生命周期语义。
  const statusSubscribers = ref(0)
  let statusSse: { close: () => void } | null = null
  let statusDisconnectTimer: any = null

  // 快照看门狗：SSE 若因中间代理缓冲/剥离而迟迟不推送，则降级拉取一次。
  // 正常情况下事件即刻到达、看门狗被清除，不会产生额外请求。
  const SSE_SNAPSHOT_TIMEOUT = 5000
  let statusSnapshotTimer: any = null

  const clearSnapshotTimer = () => {
    if (statusSnapshotTimer) {
      clearTimeout(statusSnapshotTimer)
      statusSnapshotTimer = null
    }
  }

  // 内核版本：由内核原生 API /version 提供。
  //
  // 不随 SSE 事件下发——SSE 只承载运行状态。此处在内核为「运行中」时补取一次，
  // 取到即不再重复请求（loading / unknown / 空 视为尚未取到）。
  const ensureCoreVersion = async () => {
    if (stats.value.coreVersion !== CORE_VERSION_LOADING && stats.value.coreVersion !== CORE_VERSION_UNKNOWN && stats.value.coreVersion !== '') {
      return
    }
    try {
      const resp = await apiFetch('/version')
      if (resp.ok) {
        const v = await resp.json()
        stats.value.coreVersion = (v.version || '').replace(/^v/, '')
      } else {
        stats.value.coreVersion = CORE_VERSION_UNKNOWN
      }
    } catch {
      stats.value.coreVersion = CORE_VERSION_UNKNOWN
    }
  }

  // 按运行状态重置派生显示
  const applyRunningState = (running: boolean) => {
    stats.value.running = running
    if (!running) {
      stats.value.currentNode = NODE_CORE_STOPPED
      stats.value.currentGroup = NODE_CORE_STOPPED
      stats.value.uploadSpeed = 0
      stats.value.downloadSpeed = 0
      stats.value.memory = 0
      stats.value.coreVersion = CORE_VERSION_LOADING
      return
    }
    // 内核运行中：若尚未取得版本则补取一次
    ensureCoreVersion()
  }

  const onStatusEvent = (data: any) => {
    clearSnapshotTimer() // 已收到推送，看门狗不再需要
    applyRunningState(!!data?.running)
  }

  const startStatusSse = () => {
    statusSse = sseConnect('/core/events', 'core-state', onStatusEvent)
    // 兜底：若 5 秒内没有任何事件（含接入快照），降级为一次 /core/status
    clearSnapshotTimer()
    statusSnapshotTimer = setTimeout(() => {
      statusSnapshotTimer = null
      if (statusSubscribers.value > 0) {
        fetchVersionAndStatus()
      }
    }, SSE_SNAPSHOT_TIMEOUT)
  }

  // fetchVersionAndStatus 主动拉取一次状态与内核版本。
  //
  // 用途：① SSE 建连失败/被缓冲时的降级兜底；② 内核启停、重启、升级等操作后
  // 立即确认结果，无需等待推送。
  const fetchVersionAndStatus = async () => {
    try {
      const resp = await apiFetch('/core/status')
      if (resp.ok) {
        const s = await resp.json()
        const running = !!s.running
        stats.value.running = running
        if (running) {
          await ensureCoreVersion()
        } else {
          applyRunningState(false)
        }
      }
    } catch (e) {
      console.warn('获取内核状态失败', e)
      applyRunningState(false)
    }
  }

  const subscribeStatus = () => {
    statusSubscribers.value++
    if (statusSubscribers.value === 1) {
      // 取消待执行的防抖断开
      if (statusDisconnectTimer) {
        clearTimeout(statusDisconnectTimer)
        statusDisconnectTimer = null
      }
      startStatusSse()
    }
  }

  const unsubscribeStatus = () => {
    statusSubscribers.value = Math.max(0, statusSubscribers.value - 1)
    if (statusSubscribers.value === 0) {
      clearSnapshotTimer()
      // 防抖 3 秒：页面快速切换时不至于反复断开/重建 SSE
      statusDisconnectTimer = setTimeout(() => {
        if (statusSubscribers.value === 0 && statusSse) {
          statusSse.close()
          statusSse = null
        }
        statusDisconnectTimer = null
      }, 3000)
    }
  }

  const MAX_POINTS = 65

  // 将数据压入历史队列（最长为65个点）
  const pushHistory = (up: number, down: number, maxPoints = MAX_POINTS) => {
    uploadHistory.value.push(up)
    downloadHistory.value.push(down)
    
    const now = new Date()
    const timeStr = `${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}:${String(now.getSeconds()).padStart(2, '0')}`
    timeHistory.value.push(timeStr)

    if (uploadHistory.value.length > maxPoints) uploadHistory.value.shift()
    if (downloadHistory.value.length > maxPoints) downloadHistory.value.shift()
    if (timeHistory.value.length > maxPoints) timeHistory.value.shift()
  }

  // 以零值预填满整个窗口，使首屏即为完整骨架（曲线贴基线开始向右延伸），
  // 而非先留白、再从右缘把整条曲线「长」出来。时刻表按 1s 间隔回推，保证时间轴连续可读。
  const primeHistory = (maxPoints = MAX_POINTS) => {
    if (uploadHistory.value.length > 0) return
    const now = Date.now()
    const ups: number[] = []
    const downs: number[] = []
    const times: string[] = []
    for (let i = maxPoints - 1; i >= 0; i--) {
      const d = new Date(now - i * 1000)
      ups.push(0)
      downs.push(0)
      times.push(
        `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`
      )
    }
    uploadHistory.value = ups
    downloadHistory.value = downs
    timeHistory.value = times
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
    primeHistory,
    subscribeTraffic,
    unsubscribeTraffic,
    subscribeMemory,
    unsubscribeMemory,
    subscribeStatus,
    unsubscribeStatus,
    syncCurrentNodeFromProxyStore,
    fetchVersionAndStatus
  }
})
