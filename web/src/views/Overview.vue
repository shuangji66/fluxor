<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiFetch } from '../utils/api'
import { OpenOutline, SyncOutline, EyeOutline, EyeOffOutline } from '@vicons/ionicons5'
import { storeToRefs } from 'pinia'
import { useOverviewStore } from '../store/overview'
import { useConnectionsStore } from '../store/connections'
import { useGlobalStore } from '../store/global'

const { t } = useI18n()

const overviewStore = useOverviewStore()
const { stats, uiPanel, uploadHistory, downloadHistory, timeHistory } = storeToRefs(overviewStore)
const globalStore = useGlobalStore()

const connectionsStore = useConnectionsStore()
const { connectionsCount, uploadTotal, downloadTotal } = storeToRefs(connectionsStore)

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

const showLocalV4 = ref(false)
const showLocalV6 = ref(false)
const showProxyV4 = ref(true)
const showProxyV6 = ref(true)

// 流量数据点 (最多65个)
const maxPoints = 65
let cachedMaxY = 1024

// Canvas 引用与上下文
const canvasRef = ref<HTMLCanvasElement | null>(null)
let ctx: CanvasRenderingContext2D | null = null
let dpr = window.devicePixelRatio || 1
let resizeObserver: ResizeObserver | null = null
let themeObserver: MutationObserver | null = null

// 交互状态
const tooltip = ref({
  show: false,
  x: 0,
  y: 0,
  time: '',
  up: 0,
  down: 0
})
const hoverIndex = ref<number | null>(null)

// 字节格式转换
const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(Math.abs(bytes) || 1) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

// 动态获取当前主题的网格和文字颜色变量
const getChartColors = () => {
  const style = getComputedStyle(document.documentElement)
  return {
    grid: style.getPropertyValue('--border-color').trim() || '#cbd5e1',
    text: style.getPropertyValue('--text-secondary').trim() || '#94a3b8'
  }
}

// 绘制贝塞尔曲线与面积填充
const drawSmoothArea = (
  data: number[],
  strokeColor: string,
  topGradientColor: string,
  bottomGradientColor: string,
  offsetX: number,
  stepX: number,
  h: number,
  chartH: number
) => {
  if (data.length < 2) return
  if (!ctx) return
  
  ctx.save()
  
  // 构造曲线数据点数组
  const points = data.map((val, idx) => ({
    x: offsetX + idx * stepX,
    y: h - 25 - (val / cachedMaxY) * chartH
  }))
  
  // 1. 绘制面积填充
  ctx.beginPath()
  ctx.moveTo(points[0].x, h - 25)
  ctx.lineTo(points[0].x, points[0].y)
  
  for (let i = 0; i < points.length - 1; i++) {
    const xc = (points[i].x + points[i + 1].x) / 2
    const yc = (points[i].y + points[i + 1].y) / 2
    ctx.quadraticCurveTo(points[i].x, points[i].y, xc, yc)
  }
  ctx.lineTo(points[points.length - 1].x, points[points.length - 1].y)
  ctx.lineTo(points[points.length - 1].x, h - 25)
  ctx.closePath()
  
  const grad = ctx.createLinearGradient(0, h - 25 - chartH, 0, h - 25)
  grad.addColorStop(0, topGradientColor)
  grad.addColorStop(1, bottomGradientColor)
  ctx.fillStyle = grad
  ctx.fill()
  
  // 2. 绘制描边曲线
  ctx.beginPath()
  ctx.moveTo(points[0].x, points[0].y)
  for (let i = 0; i < points.length - 1; i++) {
    const xc = (points[i].x + points[i + 1].x) / 2
    const yc = (points[i].y + points[i + 1].y) / 2
    ctx.quadraticCurveTo(points[i].x, points[i].y, xc, yc)
  }
  ctx.lineTo(points[points.length - 1].x, points[points.length - 1].y)
  ctx.strokeStyle = strokeColor
  ctx.lineWidth = 1.75
  ctx.stroke()
  
  ctx.restore()
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

  const totalLen = uploadHistory.value.length
  if (totalLen < 2 && downloadHistory.value.length < 2) {
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
  
  // 底部预留 25px 给 X 轴刻度，上部留 10px 缓冲，图表真实高度
  const chartH = h - 35
  const offsetX = (maxPoints - totalLen) * stepX

  // 1. 绘制网格线与 Y 轴刻度（半透明精致虚线）
  ctx.save()
  ctx.strokeStyle = colors.grid
  ctx.globalAlpha = 0.3
  ctx.lineWidth = 1
  ctx.setLineDash([4, 4])
  ctx.font = '10px monospace'
  ctx.textAlign = 'right'
  ctx.fillStyle = colors.text
  
  for (let i = 0; i <= 4; i++) {
    const y = 10 + (i / 4) * chartH
    ctx.beginPath()
    ctx.moveTo(0, y)
    ctx.lineTo(w - 5, y)
    ctx.stroke()
    
    // 绘制坐标刻度文本（恢复不透明度并使用实线）
    ctx.save()
    ctx.globalAlpha = 1.0
    ctx.setLineDash([])
    ctx.fillText(formatBytes(cachedMaxY * (1 - i / 4)), w - 8, y - 3)
    ctx.restore()
  }
  ctx.restore()

  // 2. 绘制平滑渐变曲线
  drawSmoothArea(uploadHistory.value, '#3b82f6', 'rgba(59, 130, 246, 0.18)', 'rgba(59, 130, 246, 0.0)', offsetX, stepX, h, chartH)
  drawSmoothArea(downloadHistory.value, '#10b981', 'rgba(16, 185, 129, 0.18)', 'rgba(16, 185, 129, 0.0)', offsetX, stepX, h, chartH)

  // 3. 绘制 X 轴动态时间刻度（相对时间）
  ctx.font = '10px monospace'
  ctx.fillStyle = colors.text
  ctx.textAlign = 'center'
  
  const timeLabels = [60, 45, 30, 15, 0]
  const lastIdx = totalLen - 1
  for (const sec of timeLabels) {
    const idx = lastIdx - sec
    if (idx >= 0 && idx < totalLen) {
      const x = offsetX + idx * stepX
      ctx.fillText(sec + 's', x, h - 6)
    }
  }

  // 4. 绘制悬浮指示器
  if (hoverIndex.value !== null && hoverIndex.value < totalLen) {
    const idx = hoverIndex.value
    const x = offsetX + idx * stepX
    
    ctx.save()
    ctx.strokeStyle = colors.grid
    ctx.lineWidth = 1
    ctx.beginPath()
    ctx.moveTo(x, 10)
    ctx.lineTo(x, h - 25)
    ctx.stroke()
    ctx.restore()
  }

  ctx.restore()
}

// 交互事件处理
const updateHoverState = (x: number, y: number) => {
  if (!canvasRef.value || uploadHistory.value.length === 0) return
  const canvas = canvasRef.value
  const w = canvas.width / dpr
  const h = canvas.height / dpr
  
  const chartH = h - 35
  const stepX = w / (maxPoints - 1)
  const offsetX = (maxPoints - uploadHistory.value.length) * stepX
  
  const rawIdx = Math.round((x - offsetX) / stepX)
  const idx = Math.max(0, Math.min(uploadHistory.value.length - 1, rawIdx))
  
  const pointX = offsetX + idx * stepX
  if (Math.abs(x - pointX) < stepX * 1.5) {
    hoverIndex.value = idx
    tooltip.value.show = true
    tooltip.value.x = pointX
    
    const upVal = uploadHistory.value[idx] || 0
    const downVal = downloadHistory.value[idx] || 0
    const upY = h - 25 - (upVal / cachedMaxY) * chartH
    const downY = h - 25 - (downVal / cachedMaxY) * chartH
    tooltip.value.y = Math.min(upY, downY) - 12
    
    tooltip.value.time = timeHistory.value[idx] || ''
    tooltip.value.up = upVal
    tooltip.value.down = downVal
  } else {
    hoverIndex.value = null
    tooltip.value.show = false
  }
  drawChart()
}

const handleMouseMove = (e: MouseEvent) => {
  updateHoverState(e.offsetX, e.offsetY)
}

const handleTouchMove = (e: TouchEvent) => {
  if (!canvasRef.value) return
  const rect = canvasRef.value.getBoundingClientRect()
  const touch = e.touches[0]
  const x = touch.clientX - rect.left
  const y = touch.clientY - rect.top
  updateHoverState(x, y)
}

const handleMouseLeave = () => {
  hoverIndex.value = null
  tooltip.value.show = false
  drawChart()
}

// 监听历史队列自动重绘
watch(uploadHistory, () => {
  drawChart()
}, { deep: true })

// 获取基础设置
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
    if (w === 0) return
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

// ===== IP 信息 =====
interface IpInfo {
  localIPv4: string | null
  localIPv6: string | null
  proxyIPv4: string | null
  proxyIPv6: string | null
  proxyPort: number | null
  loading: boolean
  error: boolean
}

// 独立刷新加载状态
const loadingLocalV4 = ref(false)
const loadingLocalV6 = ref(false)
const loadingProxyV4 = ref(false)
const loadingProxyV6 = ref(false)

const ipInfo = ref<IpInfo>({
  localIPv4: null,
  localIPv6: null,
  proxyIPv4: null,
  proxyIPv6: null,
  proxyPort: null,
  loading: true,
  error: false
})

const isAnyIpLoading = computed(() => {
  return loadingLocalV4.value || loadingLocalV6.value || loadingProxyV4.value || loadingProxyV6.value
})

// 获取公网 IP 信息（并发流式拉取，互不阻塞）
const fetchPublicIP = async () => {
  refreshLocalIPv4()
  refreshLocalIPv6()
  refreshProxyIPv4()
  refreshProxyIPv6()
}

// 独立刷新本机 IPv4
const refreshLocalIPv4 = async () => {
  loadingLocalV4.value = true
  try {
    const resp = await apiFetch('/ipinfo/local/v4')
    if (resp.ok) {
      const data = await resp.json()
      ipInfo.value.localIPv4 = data.ip || null
    }
  } catch (e) {
    console.warn('刷新本机 IPv4 失败:', e)
  } finally {
    loadingLocalV4.value = false
  }
}

// 独立刷新本机 IPv6
const refreshLocalIPv6 = async () => {
  loadingLocalV6.value = true
  try {
    const resp = await apiFetch('/ipinfo/local/v6')
    if (resp.ok) {
      const data = await resp.json()
      ipInfo.value.localIPv6 = data.ip || null
    }
  } catch (e) {
    console.warn('刷新本机 IPv6 失败:', e)
  } finally {
    loadingLocalV6.value = false
  }
}

// 独立刷新代理 IPv4
const refreshProxyIPv4 = async () => {
  loadingProxyV4.value = true
  try {
    const resp = await apiFetch('/ipinfo/proxy/v4')
    if (resp.ok) {
      const data = await resp.json()
      ipInfo.value.proxyIPv4 = data.ip || null
    } else {
      ipInfo.value.proxyIPv4 = null
    }
  } catch (e) {
    console.warn('刷新代理 IPv4 失败:', e)
    ipInfo.value.proxyIPv4 = null
  } finally {
    loadingProxyV4.value = false
  }
}

// 独立刷新代理 IPv6
const refreshProxyIPv6 = async () => {
  loadingProxyV6.value = true
  try {
    const resp = await apiFetch('/ipinfo/proxy/v6')
    if (resp.ok) {
      const data = await resp.json()
      ipInfo.value.proxyIPv6 = data.ip || null
    } else {
      ipInfo.value.proxyIPv6 = null
    }
  } catch (e) {
    console.warn('刷新代理 IPv6 失败:', e)
    ipInfo.value.proxyIPv6 = null
  } finally {
    loadingProxyV6.value = false
  }
}

// ===== 代理延迟测试 =====
interface DelayTestResult {
  name: string
  url: string
  delay: number | null
  loading: boolean
}

const delayTargets = [
  { name: 'Google', url: 'https://www.gstatic.com/generate_204' },
  { name: 'Microsoft', url: 'https://www.microsoft.com' },
  { name: 'Apple', url: 'https://www.apple.com' },
  { name: 'YouTube', url: 'https://www.youtube.com' }
]

const delayResults = ref<DelayTestResult[]>(
  delayTargets.map(t => ({ ...t, delay: null, loading: false }))
)

const isTestingDelay = ref(false)

// 目标名称到后端API路径的映射
const delayApiMap: Record<string, string> = {
    'Google': '/delaytest/google',
    'Microsoft': '/delaytest/microsoft',
    'Apple': '/delaytest/apple',
    'YouTube': '/delaytest/youtube'
}

// 测试单个目标的延迟（通过后端代理）
const testSingleDelay = async (index: number) => {
    const item = delayResults.value[index]
    if (item.loading) return
    item.loading = true
    item.delay = null

    try {
        const apiPath = delayApiMap[item.name]
        if (!apiPath) {
            item.delay = null
            return
        }
        const resp = await apiFetch(apiPath + '?timeout=5000')
        if (resp.ok) {
            const data = await resp.json()
            // 后端可能返回 delay 为 null 或数字
            if (data.delay !== undefined && data.delay !== null && data.delay > 0) {
                item.delay = data.delay
            } else {
                item.delay = null // 超时或错误
            }
        } else {
            item.delay = null
        }
    } catch (e) {
        console.warn('测试延迟失败:', e)
        item.delay = null
    } finally {
        item.loading = false
    }
}

// 测试全部目标
const testAllDelays = async () => {
  if (isTestingDelay.value) return
  isTestingDelay.value = true

  const concurrency = 2
  const queue = delayResults.value.map((_, idx) => idx)
  const workers = Array.from({ length: Math.min(concurrency, queue.length) }, async () => {
    while (queue.length > 0) {
      const idx = queue.shift()
      if (idx !== undefined) {
        await testSingleDelay(idx)
      }
    }
  })
  await Promise.all(workers)

  isTestingDelay.value = false
}

// 获取延迟显示
const getDelayDisplay = (result: DelayTestResult) => {
  if (result.loading) return { text: t('proxies.testing'), color: 'text-slate-400' }
  if (result.delay === null) return { text: t('proxies.timeout'), color: 'text-red-500' }
  if (result.delay < 150) return { text: `${result.delay}ms`, color: 'text-success' }
  if (result.delay < 300) return { text: `${result.delay}ms`, color: 'text-amber-500' }
  return { text: `${result.delay}ms`, color: 'text-red-400' }
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

    overviewStore.subscribeStatus()
    overviewStore.subscribeTraffic()
    overviewStore.subscribeMemory()
    connectionsStore.subscribe()
  })
  fetchPublicIP()
})

onUnmounted(() => {
  overviewStore.unsubscribeStatus()
  overviewStore.unsubscribeTraffic()
  overviewStore.unsubscribeMemory()
  connectionsStore.unsubscribe()

  if (resizeObserver) resizeObserver.disconnect()
  if (themeObserver) themeObserver.disconnect()
})
</script>

<template>
  <div class="space-y-6">
    <!-- 8 个统计卡片 -->
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
      <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all">
        <div class="text-xs sm:text-sm font-bold text-slate-500 dark:text-slate-400">{{ t('overview.upload_speed') }}</div>
        <div class="text-base sm:text-lg font-extrabold text-blue-500 mt-1 select-all">{{ formatBytes(stats.uploadSpeed) }}/s</div>
      </div>
      <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all">
        <div class="text-xs sm:text-sm font-bold text-slate-500 dark:text-slate-400">{{ t('overview.download_speed') }}</div>
        <div class="text-base sm:text-lg font-extrabold text-success mt-1 select-all">{{ formatBytes(stats.downloadSpeed) }}/s</div>
      </div>
      <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all">
        <div class="text-xs sm:text-sm font-bold text-slate-500 dark:text-slate-400">{{ t('overview.upload_total') }}</div>
        <div class="text-base sm:text-lg font-extrabold mt-1 select-all">{{ formatBytes(uploadTotal) }}</div>
      </div>
      <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all">
        <div class="text-xs sm:text-sm font-bold text-slate-500 dark:text-slate-400">{{ t('overview.download_total') }}</div>
        <div class="text-base sm:text-lg font-extrabold mt-1 select-all">{{ formatBytes(downloadTotal) }}</div>
      </div>
      <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all">
        <div class="text-xs sm:text-sm font-bold text-slate-500 dark:text-slate-400">{{ t('overview.memory_usage') }}</div>
        <div class="text-base sm:text-lg font-extrabold mt-1 select-all">{{ stats.memory > 0 ? formatBytes(stats.memory) : 'N/A' }}</div>
      </div>
      <div @click="globalStore.activeTab = 'connections'" class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all cursor-pointer hover:border-accent/40 hover:shadow-md active:scale-[0.98]">
        <div class="text-xs sm:text-sm font-bold text-slate-500 dark:text-slate-400">{{ t('overview.active_connections') }}</div>
        <div class="text-base sm:text-lg font-extrabold mt-1 select-all">{{ connectionsCount }}</div>
      </div>
      <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all">
        <div class="text-xs sm:text-sm font-bold text-slate-500 dark:text-slate-400">{{ t('overview.core_version') }}</div>
        <div class="text-base sm:text-lg font-extrabold mt-1 select-all truncate" :title="coreVersionDisplay">{{ coreVersionDisplay }}</div>
      </div>
      <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all">
        <a :href="`${base}${uiPanel === 'zashboard' ? '/zash/' : '/meta/'}`" target="_blank" class="block text-slate-800 dark:text-slate-100 decoration-transparent">
          <div class="flex justify-between items-center">
            <span class="text-xs sm:text-sm font-bold text-slate-500 dark:text-slate-400">{{ t('overview.external_control') }}</span>
            <OpenOutline class="w-3.5 h-3.5 text-slate-400" />
          </div>
          <div class="text-base sm:text-lg font-extrabold text-accent mt-1 select-none">{{ uiPanel === 'zashboard' ? 'Zashboard' : 'MetaCubeXD' }}</div>
        </a>
      </div>
    </div>

    <!-- 当前节点 -->
    <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all flex items-center justify-between gap-4">
      <span class="text-sm font-extrabold text-slate-700 dark:text-slate-300">{{ t('overview.current_node') }}</span>
      <div class="text-base font-bold text-accent break-all select-all">{{ currentNodeDisplay }}</div>
    </div>

    <!-- 流量趋势图 -->
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
      <div class="relative overflow-visible">
        <canvas
          ref="canvasRef"
          @mousemove="handleMouseMove"
          @mouseleave="handleMouseLeave"
          @touchmove="handleTouchMove"
          @touchend="handleMouseLeave"
          class="cursor-crosshair block w-full"
        ></canvas>
        
        <!-- Tooltip -->
        <div
          v-show="tooltip.show"
          :style="{ left: tooltip.x + 'px', top: tooltip.y + 'px' }"
          class="absolute pointer-events-none transform -translate-x-1/2 -translate-y-[100%] z-30 transition-[left,top] duration-75 min-w-[125px] rounded-xl px-3 py-2 text-[11px] font-medium glass-medium shadow-xl border text-slate-800 dark:text-slate-200"
        >
          <div class="font-bold border-b border-slate-200/30 dark:border-slate-700/30 pb-1 mb-1 text-slate-500 dark:text-slate-400 text-[10px] text-center">
            {{ tooltip.time }}
          </div>
          <div class="space-y-0.5">
            <div class="flex justify-between items-center gap-4">
              <span class="flex items-center gap-1 text-blue-500">
                <span class="w-1.5 h-1.5 rounded-full bg-blue-500"></span>{{ t('overview.upload') }}
              </span>
              <span class="font-bold font-mono">{{ formatBytes(tooltip.up) }}/s</span>
            </div>
            <div class="flex justify-between items-center gap-4">
              <span class="flex items-center gap-1 text-success">
                <span class="w-1.5 h-1.5 rounded-full bg-success"></span>{{ t('overview.download') }}
              </span>
              <span class="font-bold font-mono">{{ formatBytes(tooltip.down) }}/s</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 双列卡片：IP 信息 + 延迟测试 -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
      
        <!-- IP 信息卡 -->
    <div class="bg-white dark:bg-[#1e293b] p-5 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all">
        <div class="flex items-center justify-between mb-3">
            <h4 class="text-sm font-bold text-slate-500 dark:text-slate-400 flex items-center gap-2">
                <span class="w-1.5 h-1.5 rounded-full bg-accent"></span>
                {{ t('overview.ip_info') }}
            </h4>
            <button 
                @click="fetchPublicIP" 
                :disabled="isAnyIpLoading"
                class="p-1.5 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all disabled:opacity-50"
                :title="t('common.refresh')"
            >
                <SyncOutline class="w-4 h-4 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200" :class="{ 'animate-spin': isAnyIpLoading }" />
            </button>
        </div>
        
        <div class="space-y-1.5 text-sm">
            <!-- 本机 IPv4 -->
            <div class="flex justify-between items-center py-1 border-b border-slate-100 dark:border-slate-800/50">
                <span class="text-slate-500 dark:text-slate-400 flex-shrink-0">
                    {{ t('overview.local_ip') }} (IPv4)
                    <button @click="showLocalV4 = !showLocalV4" class="ml-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 focus:outline-none" :title="showLocalV4 ? t('common.hide') : t('common.show')">
                        <EyeOutline v-if="showLocalV4" class="w-3.5 h-3.5" />
                        <EyeOffOutline v-else class="w-3.5 h-3.5" />
                    </button>
                </span>
                <div class="flex items-center gap-2 min-w-0 flex-1 justify-end">
                    <span class="font-mono font-bold text-slate-800 dark:text-slate-100 select-all overflow-x-auto whitespace-nowrap max-w-[140px] sm:max-w-[200px] md:max-w-full">
                        {{ showLocalV4 ? (ipInfo.localIPv4 || '--') : (ipInfo.localIPv4 ? '••••••••' : '--') }}
                    </span>
                    <button @click="refreshLocalIPv4" :disabled="loadingLocalV4" class="p-1 rounded hover:bg-slate-200 dark:hover:bg-slate-700 transition-all disabled:opacity-50 flex-shrink-0" :title="t('common.refresh')">
                        <SyncOutline class="w-3.5 h-3.5 text-slate-400" :class="{ 'animate-spin': loadingLocalV4 }" />
                    </button>
                </div>
            </div>
            <!-- 本机 IPv6 -->
            <div class="flex justify-between items-center py-1 border-b border-slate-100 dark:border-slate-800/50">
                <span class="text-slate-500 dark:text-slate-400 flex-shrink-0">
                    {{ t('overview.local_ip') }} (IPv6)
                    <button @click="showLocalV6 = !showLocalV6" class="ml-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 focus:outline-none" :title="showLocalV6 ? t('common.hide') : t('common.show')">
                        <EyeOutline v-if="showLocalV6" class="w-3.5 h-3.5" />
                        <EyeOffOutline v-else class="w-3.5 h-3.5" />
                    </button>
                </span>
                <div class="flex items-center gap-2 min-w-0 flex-1 justify-end">
                    <span class="font-mono font-bold text-slate-800 dark:text-slate-100 select-all overflow-x-auto whitespace-nowrap max-w-[140px] sm:max-w-[200px] md:max-w-full">
                        {{ showLocalV6 ? (ipInfo.localIPv6 || '--') : (ipInfo.localIPv6 ? '••••••••' : '--') }}
                    </span>
                    <button @click="refreshLocalIPv6" :disabled="loadingLocalV6" class="p-1 rounded hover:bg-slate-200 dark:hover:bg-slate-700 transition-all disabled:opacity-50 flex-shrink-0" :title="t('common.refresh')">
                        <SyncOutline class="w-3.5 h-3.5 text-slate-400" :class="{ 'animate-spin': loadingLocalV6 }" />
                    </button>
                </div>
            </div>
            <!-- 代理 IPv4 -->
            <div class="flex justify-between items-center py-1 border-b border-slate-100 dark:border-slate-800/50">
                <span class="text-slate-500 dark:text-slate-400 flex-shrink-0">
                    {{ t('overview.proxy_ip') }} (IPv4)
                    <button @click="showProxyV4 = !showProxyV4" class="ml-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 focus:outline-none" :title="showProxyV4 ? t('common.hide') : t('common.show')">
                        <EyeOutline v-if="showProxyV4" class="w-3.5 h-3.5" />
                        <EyeOffOutline v-else class="w-3.5 h-3.5" />
                    </button>
                </span>
                <div class="flex items-center gap-2 min-w-0 flex-1 justify-end">
                    <span class="font-mono font-bold text-slate-800 dark:text-slate-100 select-all overflow-x-auto whitespace-nowrap max-w-[140px] sm:max-w-[200px] md:max-w-full">
                        {{ showProxyV4 ? (ipInfo.proxyIPv4 || '--') : (ipInfo.proxyIPv4 ? '••••••••' : '--') }}
                    </span>
                    <button @click="refreshProxyIPv4" :disabled="loadingProxyV4" class="p-1 rounded hover:bg-slate-200 dark:hover:bg-slate-700 transition-all disabled:opacity-50 flex-shrink-0" :title="t('common.refresh')">
                        <SyncOutline class="w-3.5 h-3.5 text-slate-400" :class="{ 'animate-spin': loadingProxyV4 }" />
                    </button>
                </div>
            </div>
            <!-- 代理 IPv6 -->
            <div class="flex justify-between items-center py-1 border-b border-slate-100 dark:border-slate-800/50">
                <span class="text-slate-500 dark:text-slate-400 flex-shrink-0">
                    {{ t('overview.proxy_ip') }} (IPv6)
                    <button @click="showProxyV6 = !showProxyV6" class="ml-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 focus:outline-none" :title="showProxyV6 ? t('common.hide') : t('common.show')">
                        <EyeOutline v-if="showProxyV6" class="w-3.5 h-3.5" />
                        <EyeOffOutline v-else class="w-3.5 h-3.5" />
                    </button>
                </span>
                <div class="flex items-center gap-2 min-w-0 flex-1 justify-end">
                    <span class="font-mono font-bold text-slate-800 dark:text-slate-100 select-all overflow-x-auto whitespace-nowrap max-w-[140px] sm:max-w-[200px] md:max-w-full">
                        {{ showProxyV6 ? (ipInfo.proxyIPv6 || '--') : (ipInfo.proxyIPv6 ? '••••••••' : '--') }}
                    </span>
                    <button @click="refreshProxyIPv6" :disabled="loadingProxyV6" class="p-1 rounded hover:bg-slate-200 dark:hover:bg-slate-700 transition-all disabled:opacity-50 flex-shrink-0" :title="t('common.refresh')">
                        <SyncOutline class="w-3.5 h-3.5 text-slate-400" :class="{ 'animate-spin': loadingProxyV6 }" />
                    </button>
                </div>
            </div>
        </div>
    </div>

      <!-- 代理延迟测试卡 -->
      <div class="bg-white dark:bg-[#1e293b] p-5 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all">
          <div class="flex items-center justify-between mb-3">
              <h4 class="text-sm font-bold text-slate-500 dark:text-slate-400 flex items-center gap-2">
                  <span class="w-1.5 h-1.5 rounded-full bg-success"></span>
                  {{ t('overview.proxy_delay_test') }}
              </h4>
              <button 
                  @click="testAllDelays" 
                  :disabled="isTestingDelay"
                  class="px-3 py-1 text-xs font-semibold rounded-lg bg-accent hover:bg-accent-hover text-white transition-all disabled:opacity-50 disabled:cursor-not-allowed"
              >
                  {{ isTestingDelay ? t('overview.testing') : t('overview.test_all') }}
              </button>
          </div>
          
          <div class="space-y-2">
              <div 
                  v-for="(result, idx) in delayResults" 
                  :key="result.name"
                  class="flex items-center justify-between py-1.5 px-2 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-800/50 transition-colors"
              >
                  <span class="text-sm font-semibold text-slate-700 dark:text-slate-300">{{ result.name }}</span>
                  <div class="flex items-center gap-3">
                      <span 
                          class="text-sm font-mono font-bold cursor-pointer hover:underline"
                          :class="getDelayDisplay(result).color"
                          @click="testSingleDelay(idx)"
                      >
                          {{ getDelayDisplay(result).text }}
                      </span>

                      <button 
                          @click="testSingleDelay(idx)" 
                          :disabled="result.loading"
                          class="w-6 h-6 rounded-lg bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 flex items-center justify-center transition-all disabled:opacity-50"
                          :title="t('overview.test_single_title', { name: result.name })"
                      >
                          <svg v-if="!result.loading" class="w-3.5 h-3.5 text-slate-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
                          </svg>
                          <div v-else class="w-3 h-3 border-2 border-slate-300 dark:border-slate-600 !border-t-accent rounded-full animate-spin"></div>
                      </button>
                  </div>
              </div>
          </div>
      </div>
    </div>
  </div>
</template>