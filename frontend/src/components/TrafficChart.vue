<script setup lang="ts">
import { nextTick, onActivated, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatBytes, formatSpeed } from '../utils/format'

/**
 * 流量趋势图（Canvas 自绘，maxPoints 个点、右对齐流式推进）。
 *
 * 从概览页整体抽出：量程阶梯、单调插值、网格/刻度、命中测试与 tooltip 是一套自洽的
 * 绘图逻辑（约 400 行），与页面的数据来源无关。抽出来后概览页只负责「把三组历史数组传进来」，
 * 图表的 resize / 主题切换 / KeepAlive 重新激活都能在组件内部自洽处理。
 */
const props = withDefaults(
  defineProps<{
    uploadHistory: number[]
    downloadHistory: number[]
    timeHistory: string[]
    /** 窗口点数：决定 X 轴步长与刻度位置 */
    maxPoints?: number
  }>(),
  { maxPoints: 65 }
)

const { t } = useI18n()
const pointCount = props.maxPoints

// === 绘图区布局（CSS px）===
// 左侧独立 gutter 安置 Y 轴刻度，刻度不再压在数据区上；底部轴区安置时间刻度。
const CHART_H = 200
const GUTTER_L = 52
const PAD_T = 10
const AXIS_B = 22

interface ChartBox {
  plotL: number
  plotR: number
  plotT: number
  plotB: number
  plotW: number
  plotH: number
  stepX: number
}

const chartBox = (w: number): ChartBox => {
  const plotL = GUTTER_L
  const plotR = w - 8
  const plotT = PAD_T
  const plotB = CHART_H - AXIS_B
  const plotW = plotR - plotL
  return { plotL, plotR, plotT, plotB, plotW, plotH: plotB - plotT, stepX: plotW / (pointCount - 1) }
}

// 序列 → 画布 x 坐标的唯一映射（绘制与命中测试共用，禁止各算一套）。
// 右对齐流式：最新点恒贴 plotR，历史向左延伸。窗口被 primeHistory 填满时
// 该式恒等于 plotL + idx * stepX；仅当历史不足（未预填/被清空）时才在右侧紧凑排列，
// 从而永不出现「曲线与右缘脱开」的悬空图形。
const seriesOffsetX = (box: ChartBox, totalLen: number): number =>
  box.plotR - (Math.max(totalLen, 1) - 1) * box.stepX

// 1px 细线对齐到物理像素网格：落在整数坐标上的 1px 线会跨两个物理行渲染成 2px 灰边，
// 轴线尤其明显。先吸附到设备像素中心再回缩放空间，保证在任意 dpr 下都是单一实线。
const crisp = (v: number): number => (Math.round(v * dpr) + 0.5) / dpr

// Y 量程：固定在 1024 进制阶梯上取值，避免出现 "1.95 KB" 这类刻度文本。
const SCALE_LADDER = (() => {
  const out = [1024]
  for (let p = 1; p <= 4; p++) {
    for (const m of [1, 2, 5, 10, 20, 50, 100, 200, 500]) out.push(m * Math.pow(1024, p))
  }
  return out
})()
let scaleMax = 1024
let shrinkStreak = 0

const niceMax = (v: number): number => {
  const t = Math.max(v, 1024)
  return SCALE_LADDER.find(n => n >= t) ?? SCALE_LADDER[SCALE_LADDER.length - 1]
}


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


// 动态获取当前主题的网格和文字颜色变量
const getChartColors = () => {
  const style = getComputedStyle(document.documentElement)
  return {
    grid: style.getPropertyValue('--border-color').trim() || '#cbd5e1',
    text: style.getPropertyValue('--text-secondary').trim() || '#94a3b8'
  }
}


// 单调三次（Fritsch–Carlson）插值：保形且保峰值。
// 原实现用 quadraticCurveTo 的中点法，下一段曲线以「上一段的中点」为当前点，
// 曲线因此永远画不到数据点本身。实测（64 段采样）：
//   单帧峰值 1MB→渲染 750KB（-25%）、陡升陡降 2MB→1.5MB（-25%）、锯齿 -25%；
//   连续两帧以上的峰值才无损。
// 而单帧突发正是流量曲线的常态形状，等于系统性抹掉了每个毛刺的高度。
// 单调插值对上述场景均 100% 保峰值，且极值点切线归零，不过冲越界。
const monotoneTangents = (points: { x: number; y: number }[]): number[] => {
  const n = points.length
  const m = new Array<number>(n).fill(0)
  if (n < 2) return m

  const dx: number[] = []
  const dy: number[] = []
  const slope: number[] = []
  for (let i = 0; i < n - 1; i++) {
    dx.push(points[i + 1].x - points[i].x)
    dy.push(points[i + 1].y - points[i].y)
    slope.push(dx[i] === 0 ? 0 : dy[i] / dx[i])
  }

  m[0] = slope[0]
  m[n - 1] = slope[n - 2]
  for (let i = 1; i < n - 1; i++) {
    if (slope[i - 1] * slope[i] <= 0) {
      m[i] = 0 // 极值点处切线归零，杜绝过冲
    } else {
      const w1 = 2 * dx[i] + dx[i - 1]
      const w2 = dx[i] + 2 * dx[i - 1]
      m[i] = (w1 + w2) / (w1 / slope[i - 1] + w2 / slope[i])
    }
  }
  return m
}

const tracePath = (points: { x: number; y: number }[]) => {
  if (!ctx) return false
  const n = points.length
  if (n < 2) return false
  const m = monotoneTangents(points)

  ctx.beginPath()
  ctx.moveTo(points[0].x, points[0].y)
  for (let i = 0; i < n - 1; i++) {
    const h = points[i + 1].x - points[i].x
    ctx.bezierCurveTo(
      points[i].x + h / 3, points[i].y + (m[i] * h) / 3,
      points[i + 1].x - h / 3, points[i + 1].y - (m[i + 1] * h) / 3,
      points[i + 1].x, points[i + 1].y
    )
  }
  return true
}

// 绘制曲线与面积填充（基线为 plotB，面积自曲线垂落到基线）
const drawSmoothArea = (
  data: number[],
  strokeColor: string,
  topGradientColor: string,
  bottomGradientColor: string,
  box: ChartBox
) => {
  if (data.length < 2 || !ctx) return
  const { plotB, plotH, stepX } = box
  const offsetX = seriesOffsetX(box, data.length)

  const points = data.map((val, idx) => ({
    x: offsetX + idx * stepX,
    y: plotB - (Math.min(val, scaleMax) / scaleMax) * plotH
  }))

  ctx.save()

  // 1. 面积填充
  if (!tracePath(points)) {
    ctx.restore()
    return
  }
  ctx.lineTo(points[points.length - 1].x, plotB)
  ctx.lineTo(points[0].x, plotB)
  ctx.closePath()

  const grad = ctx.createLinearGradient(0, box.plotT, 0, plotB)
  grad.addColorStop(0, topGradientColor)
  grad.addColorStop(1, bottomGradientColor)
  ctx.fillStyle = grad
  ctx.fill()

  // 2. 描边
  if (tracePath(points)) {
    ctx.strokeStyle = strokeColor
    ctx.lineWidth = 1.75
    ctx.lineJoin = 'round'
    ctx.stroke()
  }

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

  const totalLen = props.uploadHistory.length
  if (totalLen < 2) {
    ctx.restore()
    return
  }

  const colors = getChartColors()
  const box = chartBox(w)
  const { plotL, plotR, plotT, plotB, plotH, stepX } = box

  // 1. 网格线（降权为极浅虚线；基线不在此处，由第 2 步单独绘制）
  ctx.save()
  ctx.strokeStyle = colors.grid
  ctx.globalAlpha = 0.16
  ctx.lineWidth = 1
  ctx.setLineDash([4, 4])
  for (let i = 0; i < 4; i++) {
    const y = crisp(plotT + (i / 4) * plotH)
    ctx.beginPath()
    ctx.moveTo(plotL, y)
    ctx.lineTo(plotR, y)
    ctx.stroke()
  }
  ctx.restore()

  // 2. 基线（零线）：唯一的一条实线重线，从左侧 gutter 画到右缘，与 Y 轴竖线
  //    正交构成完整坐标基准。必须画在曲线之前：面积渐变在底端 alpha 已为 0，
  //    不会遮挡它；反之若画在之后，会把贴零的曲线整段吃掉。
  ctx.save()
  ctx.strokeStyle = colors.grid
  ctx.globalAlpha = 0.85
  ctx.lineWidth = 1
  ctx.setLineDash([])
  const baseY = crisp(plotB)
  ctx.beginPath()
  ctx.moveTo(crisp(plotL), baseY)
  ctx.lineTo(crisp(plotR), baseY)
  ctx.stroke()
  ctx.restore()

  // 3. 曲线与面积
  //    注：y 由 clamp 后的数值算出，恒落在 [plotT, plotB] 内（单调插值在值域上不过冲），
  //    x 恒落在 [plotL, plotR] 内，因此并不存在越界溢出。此处 clip 只是防御性兜底：
  //    按描边半宽（0.875px）外扩，保证任何情况下都不会裁掉线宽本身。
  ctx.save()
  ctx.beginPath()
  const bleed = 0.875
  ctx.rect(plotL - bleed, plotT - bleed, plotR - plotL + bleed * 2, plotB - plotT + bleed * 2)
  ctx.clip()
  drawSmoothArea(props.uploadHistory, '#3b82f6', 'rgba(59, 130, 246, 0.18)', 'rgba(59, 130, 246, 0.0)', box)
  drawSmoothArea(props.downloadHistory, '#10b981', 'rgba(16, 185, 129, 0.18)', 'rgba(16, 185, 129, 0.0)', box)
  ctx.restore()

  // 4. Y 轴刻度：画在 gutter 内，右对齐，不叠数据；刻度值落在 1024 阶梯上
  ctx.save()
  ctx.font = '10px monospace'
  ctx.fillStyle = colors.text
  ctx.textAlign = 'right'
  ctx.textBaseline = 'middle'
  for (let i = 0; i <= 4; i++) {
    const y = plotT + (i / 4) * plotH
    // 顶端/底端刻度内缩，避免贴边被裁
    const ty = i === 0 ? y + 5 : i === 4 ? y - 5 : y
    ctx.fillText(formatBytes(scaleMax * (1 - i / 4)), plotL - 8, ty)
  }
  // Y 轴竖线，让曲线有明确的左边界
  ctx.globalAlpha = 0.35
  ctx.strokeStyle = colors.grid
  ctx.setLineDash([])
  ctx.beginPath()
  ctx.moveTo(crisp(plotL), plotT)
  ctx.lineTo(crisp(plotL), plotB)
  ctx.stroke()
  ctx.restore()

  // 5. X 轴时间刻度：真实时钟（HH:MM:SS），位置由「窗口内第几个点」决定，
  //    与数据量无关，因而永不漂移；窗口固定为 maxPoints 时恒等于左缘起点。
  ctx.save()
  ctx.font = '10px monospace'
  ctx.fillStyle = colors.text
  ctx.textBaseline = 'alphabetic'
  const offsetX = seriesOffsetX(box, totalLen)
  const tickCount = Math.max(2, Math.min(5, Math.round(box.plotW / 110)))
  for (let i = 0; i < tickCount; i++) {
    const t = i / (tickCount - 1)
    const idx = Math.round(t * (pointCount - 1))
    if (idx >= totalLen) continue // 窗口未填满时不画到空白区
    const label = props.timeHistory[idx] || ''
    if (!label) continue
    const x = offsetX + idx * stepX
    // 首末标签分别左/右对齐，避免出界
    if (i === 0) {
      ctx.textAlign = 'left'
      ctx.fillText(label, plotL, h - 6)
    } else if (i === tickCount - 1) {
      ctx.textAlign = 'right'
      ctx.fillText(label, plotR, h - 6)
    } else {
      ctx.textAlign = 'center'
      ctx.fillText(label, x, h - 6)
    }
  }
  ctx.restore()

  // 6. 悬浮指示器（与曲线共用 offsetX，保证指示线精确落在被高亮的数据点上）
  if (hoverIndex.value !== null && hoverIndex.value < totalLen) {
    const x = crisp(offsetX + hoverIndex.value * stepX)
    ctx.save()
    ctx.strokeStyle = colors.grid
    ctx.globalAlpha = 0.6
    ctx.lineWidth = 1
    ctx.beginPath()
    ctx.moveTo(x, plotT)
    ctx.lineTo(x, plotB)
    ctx.stroke()
    ctx.restore()
  }

  ctx.restore()
}

// 交互事件处理（与 drawChart 复用同一套坐标映射，量程不再随鼠标移动变化）
// 仅需横坐标：tooltip 的纵向位置由数据值推导，与指针 y 无关。
const updateHoverState = (x: number) => {
  if (!canvasRef.value || props.uploadHistory.length === 0) return
  const canvas = canvasRef.value
  const w = canvas.width / dpr

  const box = chartBox(w)
  const { plotB, plotH, stepX, plotR } = box
  const totalLen = props.uploadHistory.length
  const offsetX = seriesOffsetX(box, totalLen)

  const rawIdx = Math.round((x - offsetX) / stepX)
  const idx = Math.max(0, Math.min(totalLen - 1, rawIdx))

  const pointX = offsetX + idx * stepX
  // 命中范围以曲线实际占据的区间为准（右对齐流式下右侧可能是空白区）
  if (x >= offsetX - stepX / 2 && x <= plotR + stepX / 2) {
    hoverIndex.value = idx
    tooltip.value.show = true
    tooltip.value.x = pointX

    const upVal = props.uploadHistory[idx] || 0
    const downVal = props.downloadHistory[idx] || 0
    const upY = plotB - (Math.min(upVal, scaleMax) / scaleMax) * plotH
    const downY = plotB - (Math.min(downVal, scaleMax) / scaleMax) * plotH
    tooltip.value.y = Math.min(upY, downY) - 12

    tooltip.value.time = props.timeHistory[idx] || ''
    tooltip.value.up = upVal
    tooltip.value.down = downVal
  } else {
    hoverIndex.value = null
    tooltip.value.show = false
  }
  drawChart()
}

const handleMouseMove = (e: MouseEvent) => {
  updateHoverState(e.offsetX)
}

const handleTouchMove = (e: TouchEvent) => {
  if (!canvasRef.value) return
  const rect = canvasRef.value.getBoundingClientRect()
  const touch = e.touches[0]
  const x = touch.clientX - rect.left
  updateHoverState(x)
}

const handleMouseLeave = () => {
  hoverIndex.value = null
  tooltip.value.show = false
  drawChart()
}

// 量程更新与重绘解耦：仅在数据点入队时评估，避免鼠标移动（高频调用 drawChart）
// 反复触发缩容导致 Y 轴量程抖动。
// 扩容立即生效；缩容需峰值连续 30 个点低于量程一半，才回落到匹配档位。
const updateScale = () => {
  let peak = 0
  for (const v of props.uploadHistory) if (v > peak) peak = v
  for (const v of props.downloadHistory) if (v > peak) peak = v

  const next = niceMax(peak)
  if (next > scaleMax) {
    scaleMax = next
    shrinkStreak = 0
    return
  }
  if (peak < scaleMax * 0.5) {
    shrinkStreak++
    if (shrinkStreak >= 30) {
      scaleMax = niceMax(peak * 1.2)
      shrinkStreak = 0
    }
  } else {
    shrinkStreak = 0
  }
}

// 监听历史队列自动重绘
watch(() => props.uploadHistory, () => {
  updateScale()
  drawChart()
}, { deep: true })


// 监听主题变化
const observeTheme = () => {
  if (themeObserver) themeObserver.disconnect()
  themeObserver = new MutationObserver(() => {
    drawChart()
  })
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
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
    const h = CHART_H
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


// 画布初始化、主题切换重绘、KeepAlive 重新激活重绘都在组件内部闭环。
// 主题用 MutationObserver 监听 <html data-theme>：颜色来自 CSS 变量，
// 主题一变必须重画，否则曲线还停在上一个主题的配色上。
onMounted(() => {
  nextTick(() => {
    initCanvas()
    observeTheme()
  })
})

onActivated(() => {
  // KeepAlive 缓存期间画布尺寸可能变化（切换侧边栏、窗口 resize），重新激活时补画一次
  nextTick(() => drawChart())
})

onUnmounted(() => {
  if (resizeObserver) resizeObserver.disconnect()
  if (themeObserver) themeObserver.disconnect()
})
</script>

<template>
          <!-- 流量趋势图 -->
          <div
            class="lg:col-span-1 bg-slate-50/50 dark:bg-slate-900/30 p-6 rounded-xl border border-slate-200/40 dark:border-slate-800/40 transition-all space-y-4">
            <div class="flex justify-between items-center">
              <h4 class="font-bold text-sm">{{ t('overview.traffic_trend') }}</h4>
              <div class="flex gap-4 text-xs font-semibold">
                <span class="flex items-center gap-1.5 text-blue-500">
                  <span class="w-3 h-3 bg-blue-500/20 border border-blue-500/40 rounded"></span> {{ t('overview.upload')
                  }}
                </span>
                <span class="flex items-center gap-1.5 text-success">
                  <span class="w-3 h-3 bg-success/20 border border-success/40 rounded"></span> {{ t('overview.download')
                  }}
                </span>
              </div>
            </div>
            <div class="relative overflow-visible">
              <canvas ref="canvasRef" @mousemove="handleMouseMove" @mouseleave="handleMouseLeave"
                @touchmove="handleTouchMove" @touchend="handleMouseLeave" class="cursor-crosshair block w-full"></canvas>

              <!-- Tooltip -->
              <div v-show="tooltip.show" :style="{ left: tooltip.x + 'px', top: tooltip.y + 'px' }"
                class="absolute pointer-events-none transform -translate-x-1/2 -translate-y-[100%] z-30 transition-[left,top] duration-75 min-w-[125px] rounded-xl px-3 py-2 text-[11px] font-medium glass-medium shadow-xl border text-slate-800 dark:text-slate-200">
                <div
                  class="font-bold border-b border-slate-200/30 dark:border-slate-700/30 pb-1 mb-1 text-slate-500 dark:text-slate-400 text-[10px] text-center">
                  {{ tooltip.time }}
                </div>
                <div class="space-y-0.5">
                  <div class="flex justify-between items-center gap-4">
                    <span class="flex items-center gap-1 text-blue-500">
                      <span class="w-1.5 h-1.5 rounded-full bg-blue-500"></span>{{ t('overview.upload') }}
                    </span>
                    <span class="font-bold font-mono">{{ formatSpeed(tooltip.up) }}</span>
                  </div>
                  <div class="flex justify-between items-center gap-4">
                    <span class="flex items-center gap-1 text-success">
                      <span class="w-1.5 h-1.5 rounded-full bg-success"></span>{{ t('overview.download') }}
                    </span>
                    <span class="font-bold font-mono">{{ formatSpeed(tooltip.down) }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
</template>
