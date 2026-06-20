<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiFetch } from '../utils/api'
import { OpenOutline } from '@vicons/ionicons5'
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

// 获取图表样式色彩
const getChartColors = () => {
  const isDark = document.documentElement.classList.contains('dark')
  return {
    grid: isDark ? 'rgba(148,163,184,0.08)' : 'rgba(15,23,42,0.04)',
    text: isDark ? '#64748b' : '#94a3b8'
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

  // 1. 绘制网格线与 Y 轴刻度（精致虚线）
  ctx.strokeStyle = colors.grid
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
    
    // 实线画文本（清除虚线设置）
    ctx.save()
    ctx.setLineDash([])
    ctx.fillText(formatBytes(cachedMaxY * (1 - i / 4)), w - 8, y - 3)
    ctx.restore()
  }
  ctx.setLineDash([]) // 清除虚线设置

  // 2. 绘制平滑渐变曲线
  // 上传：蓝色 #3b82f6；下载：绿色 #10b981
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
    
    // 绘制垂直引导实线（竖线）
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

    // 订阅数据流与状态轮询
    overviewStore.subscribeStatus()
    overviewStore.subscribeTraffic()
    overviewStore.subscribeMemory()
    connectionsStore.subscribe()
  })
})

onUnmounted(() => {
  // 取消订阅
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

    <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all flex items-center justify-between gap-4">
      <span class="text-sm font-extrabold text-slate-700 dark:text-slate-300">{{ t('overview.current_node') }}</span>
      <div class="text-base font-bold text-accent break-all select-all">{{ currentNodeDisplay }}</div>
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
      <div class="relative overflow-visible">
        <canvas
          ref="canvasRef"
          @mousemove="handleMouseMove"
          @mouseleave="handleMouseLeave"
          @touchmove="handleTouchMove"
          @touchend="handleMouseLeave"
          class="cursor-crosshair block w-full"
        ></canvas>
        
        <!-- Interactive Tooltip Card -->
        <div
          v-show="tooltip.show"
          :style="{ left: tooltip.x + 'px', top: tooltip.y + 'px' }"
          class="absolute pointer-events-none transform -translate-x-1/2 -translate-y-[100%] z-30 transition-[left,top] duration-75 min-w-[125px] rounded-xl px-3 py-2 text-[11px] font-medium backdrop-blur-md bg-white/90 dark:bg-slate-900/90 shadow-xl border border-slate-200/50 dark:border-slate-800/50 text-slate-800 dark:text-slate-200"
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
  </div>
</template>
