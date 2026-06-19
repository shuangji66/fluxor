<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiFetch, wsConnect } from '../utils/api'
import { OpenOutline, RadioOutline } from '@vicons/ionicons5'
import { storeToRefs } from 'pinia'
import { useOverviewStore } from '../store/overview'

const { t } = useI18n()

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

const overviewStore = useOverviewStore()
const { stats, uiPanel, uploadHistory, downloadHistory } = storeToRefs(overviewStore)

const coreVersionDisplay = computed(() => {
  if (stats.value.coreVersion === '加载中...') return t('common.loading')
  if (stats.value.coreVersion === '未知') return t('common.unknown')
  return stats.value.coreVersion
})

const currentNodeDisplay = computed(() => {
  if (stats.value.currentNode === '加载中...') return t('common.loading')
  if (stats.value.currentNode === '内核未启动') return t('config.core_stopped')
  if (stats.value.currentNode === '暂无选择') return t('proxies.empty')
  return stats.value.currentNode
})

const base = window.BASE_URL || ''

// 流量数据点 (最多60个)
const maxPoints = 60
let cachedMaxY = 1024

// Canvas 引用与上下文
const canvasRef = ref<HTMLCanvasElement | null>(null)
let ctx: CanvasRenderingContext2D | null = null
let dpr = window.devicePixelRatio || 1
let resizeObserver: ResizeObserver | null = null
let themeObserver: MutationObserver | null = null

// WebSocket 长连接实例
let wsTraffic: WebSocket | null = null
let wsConnections: WebSocket | null = null
let wsMemory: WebSocket | null = null
let statusTimer: any = null

// 字节格式转换
const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(Math.abs(bytes) || 1) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

// 获取图表样式色彩
const getChartColors = () => {
  const isDark = document.documentElement.getAttribute('data-theme') !== 'light'
  return {
    grid: isDark ? 'rgba(148,163,184,0.12)' : 'rgba(15,23,42,0.06)',
    upload: '#3b82f6',
    download: '#10b981',
    uploadFill: isDark ? 'rgba(59,130,246,0.08)' : 'rgba(59,130,246,0.05)',
    downloadFill: isDark ? 'rgba(16,185,129,0.08)' : 'rgba(16,185,129,0.05)',
    text: isDark ? '#94a3b8' : '#64748b'
  }
}

// Canvas 绘制折线图
const drawChart = () => {
  if (!ctx || !canvasRef.value) return
  const canvas = canvasRef.value
  const w = canvas.width / dpr
  const h = canvas.height / dpr

  ctx.clearRect(0, 0, canvas.width, canvas.height)
  ctx.save()
  ctx.scale(dpr, dpr)

  if (uploadHistory.value.length < 2 && downloadHistory.value.length < 2) {
    ctx.restore()
    return
  }

  // 动态计算 Y 轴最大值
  let currentMax = 1024
  uploadHistory.value.forEach(v => { if (v > currentMax) currentMax = v })
  downloadHistory.value.forEach(v => { if (v > currentMax) currentMax = v })
  cachedMaxY = Math.max(currentMax, cachedMaxY * 0.95)

  const stepX = w / (maxPoints - 1)
  const colors = getChartColors()
  const chartH = h * 0.9
  const offsetX = (maxPoints - uploadHistory.value.length) * stepX

  // 绘制网格线与 Y 轴刻度
  ctx.strokeStyle = colors.grid
  ctx.lineWidth = 1
  ctx.font = '10px monospace'
  ctx.textAlign = 'right'
  ctx.fillStyle = colors.text
  for (let i = 0; i <= 4; i++) {
    const y = (i / 4) * h
    ctx.beginPath()
    ctx.moveTo(0, y)
    ctx.lineTo(w, y)
    ctx.stroke()
    ctx.fillText(formatBytes(cachedMaxY * (1 - i / 4)), w - 4, y - 3)
  }

  // 绘制面积折线
  const drawArea = (data: number[], strokeColor: string, fillColor: string) => {
    if (data.length < 2) return
    if (!ctx) return
    ctx.beginPath()
    for (let i = 0; i < data.length; i++) {
      const x = offsetX + i * stepX
      const y = h - (data[i] / cachedMaxY) * chartH
      if (i === 0) ctx.moveTo(x, y)
      else ctx.lineTo(x, y)
    }
    ctx.strokeStyle = strokeColor
    ctx.lineWidth = 2
    ctx.stroke()

    const lastX = offsetX + (data.length - 1) * stepX
    ctx.lineTo(lastX, h)
    ctx.lineTo(offsetX, h)
    ctx.closePath()
    ctx.fillStyle = fillColor
    ctx.fill()
  }

  drawArea(uploadHistory.value, colors.upload, colors.uploadFill)
  drawArea(downloadHistory.value, colors.download, colors.downloadFill)

  ctx.restore()
}

const updateTrafficChart = (up: number, down: number) => {
  overviewStore.pushHistory(up, down, maxPoints)
  drawChart()
}

// 递归查找当前节点的物理出口
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

// 获取基础设置（面板和订阅配置相关）
const fetchSubscribeConfig = async () => {
  try {
    const resp = await apiFetch('/subscribe/config')
    if (resp.ok) {
      const cfg = await resp.json()
      uiPanel.value = cfg.ui_panel || 'metacubexd'
    }
  } catch (e) {
    console.warn('获取配置失败，使用默认面板', e)
  }
}

// 获取版本、内核状态和代理组活跃选择
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

// 建立 WebSocket 连接组
const initWebSockets = () => {
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
    updateTrafficChart(up, down)
  })

  wsConnections = wsConnect('/connections', (e: MessageEvent) => {
    try {
      const d = JSON.parse(e.data)
      stats.value.connectionsCount = d.connections ? d.connections.length : 0
      if (d.uploadTotal !== undefined && d.downloadTotal !== undefined) {
        stats.value.uploadTotal = d.uploadTotal
        stats.value.downloadTotal = d.downloadTotal
      }
    } catch (err) {}
  })

  wsMemory = wsConnect('/memory', (e: MessageEvent) => {
    let mem = 0
    if (typeof e.data === 'string') {
      const d = JSON.parse(e.data)
      mem = d.inuse || d.memory || 0
    } else if (e.data instanceof ArrayBuffer) {
      mem = Number(new DataView(e.data).getBigUint64(0, false))
    }
    stats.value.memory = mem
  })
}

// 初始化 Canvas
const initCanvas = () => {
  const canvas = canvasRef.value
  if (!canvas) return
  ctx = canvas.getContext('2d')
  dpr = window.devicePixelRatio || 1

  const resize = () => {
    const parent = canvas.parentElement
    if (!parent) return
    const w = parent.clientWidth
    const h = 260
    canvas.style.width = w + 'px'
    canvas.style.height = h + 'px'
    canvas.width = w * dpr
    canvas.height = h * dpr
    drawChart()
  }

  if (resizeObserver) resizeObserver.disconnect()
  resizeObserver = new ResizeObserver(resize)
  if (canvas.parentElement) {
    resizeObserver.observe(canvas.parentElement)
  }
  resize()
}

// 监听主题变化
const observeTheme = () => {
  if (themeObserver) themeObserver.disconnect()
  themeObserver = new MutationObserver(() => {
    drawChart()
  })
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
}

onMounted(() => {
  nextTick(() => {
    fetchSubscribeConfig()
    initCanvas()
    observeTheme()
    initWebSockets()
    fetchVersionAndStatus()
    statusTimer = setInterval(fetchVersionAndStatus, 10000)
  })
})

onUnmounted(() => {
  if (wsTraffic) wsTraffic.close()
  if (wsConnections) wsConnections.close()
  if (wsMemory) wsMemory.close()
  if (statusTimer) clearInterval(statusTimer)
  if (resizeObserver) resizeObserver.disconnect()
  if (themeObserver) themeObserver.disconnect()
})
</script>

<template>
  <div class="space-y-6">
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
      <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all">
        <div class="text-[10px] sm:text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('overview.upload_speed') }}</div>
        <div class="text-sm sm:text-base font-bold text-blue-500 mt-1 select-all">{{ formatBytes(stats.uploadSpeed) }}/s</div>
      </div>
      <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all">
        <div class="text-[10px] sm:text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('overview.download_speed') }}</div>
        <div class="text-sm sm:text-base font-bold text-success mt-1 select-all">{{ formatBytes(stats.downloadSpeed) }}/s</div>
      </div>
      <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all">
        <div class="text-[10px] sm:text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('overview.upload_total') }}</div>
        <div class="text-sm sm:text-base font-bold mt-1 select-all">{{ formatBytes(stats.uploadTotal) }}</div>
      </div>
      <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all">
        <div class="text-[10px] sm:text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('overview.download_total') }}</div>
        <div class="text-sm sm:text-base font-bold mt-1 select-all">{{ formatBytes(stats.downloadTotal) }}</div>
      </div>
      <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all">
        <div class="text-[10px] sm:text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('overview.memory_usage') }}</div>
        <div class="text-sm sm:text-base font-bold mt-1 select-all">{{ stats.memory > 0 ? formatBytes(stats.memory) : 'N/A' }}</div>
      </div>
      <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all">
        <div class="text-[10px] sm:text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('overview.active_connections') }}</div>
        <div class="text-sm sm:text-base font-bold mt-1 select-all">{{ stats.connectionsCount }}</div>
      </div>
      <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all">
        <div class="text-[10px] sm:text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('overview.core_version') }}</div>
        <div class="text-sm sm:text-base font-bold mt-1 select-all truncate" :title="coreVersionDisplay">{{ coreVersionDisplay }}</div>
      </div>
      <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all">
        <a :href="`${base}${uiPanel === 'zashboard' ? '/zash/' : '/meta/'}`" target="_blank" class="block text-slate-800 dark:text-slate-100 decoration-transparent">
          <div class="flex justify-between items-center">
            <span class="text-[10px] sm:text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('overview.external_control') }}</span>
            <OpenOutline class="w-3.5 h-3.5 text-slate-400" />
          </div>
          <div class="text-sm sm:text-base font-bold text-accent mt-1 select-none">{{ uiPanel === 'zashboard' ? 'Zashboard' : 'MetaCubeXD' }}</div>
        </a>
      </div>
    </div>

    <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all flex items-center justify-between gap-4">
      <div class="flex items-center gap-2">
        <RadioOutline class="w-5 h-5 text-accent" />
        <span class="text-xs font-bold text-slate-700 dark:text-slate-300">{{ t('overview.current_node') }}</span>
      </div>
      <div class="text-sm font-semibold text-accent break-all select-all">{{ currentNodeDisplay }}</div>
    </div>

    <div class="bg-white dark:bg-[#1e293b] p-6 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all space-y-4">
      <div class="flex justify-between items-center">
        <h4 class="font-bold text-sm">{{ t('overview.traffic_trend') }}</h4>
        <div class="flex gap-4 text-xs font-semibold">
          <span class="flex items-center gap-1.5 text-blue-500">
            <span class="w-3 h-3 bg-blue-500/20 border border-blue-500/40 rounded"></span> {{ t('overview.upload') }}
          </span>
          <span class="flex items-center gap-1.5 text-success">
            <span class="w-3 h-3 bg-success/20 border border-success/40 rounded"></span> {{ t('overview.download') }}
          </span>
        </div>
      </div>
      <div>
        <canvas ref="canvasRef"></canvas>
      </div>
    </div>
  </div>
</template>
