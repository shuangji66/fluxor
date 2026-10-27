<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { apiFetch, readErrorMessage } from '../utils/api'
import { ChevronForwardOutline, SyncOutline, LayersOutline } from '@vicons/ionicons5'
import { useProxyStore, type ProxyGroup } from '../store/proxies'
import { useGlobalStore } from '../store/global'
import { useConnectionsStore } from '../store/connections'

const props = defineProps<{
  group: ProxyGroup
}>()

const { t } = useI18n()
const proxyStore = useProxyStore()
const globalStore = useGlobalStore()
const { delays, expandedState, sortOrder, delayThresholds, qualityScores, filterRegex, autoCloseConnections } = storeToRefs(proxyStore)
// 策略组链式解析已下沉至 Store（§4.9），组件只读其缓存结果
const { getEffectiveDelay, getEffectiveNode } = proxyStore

const isTesting = ref(false)

const cardRef = ref<HTMLElement | null>(null)
const headerRef = ref<HTMLElement | null>(null)

// 就近的可滚动祖先：各视图在容器内部滚动（body 恒为 overflow:hidden），
// 组头 sticky top-0 贴的就是这个容器的顶边。
const findScrollParent = (el: HTMLElement): HTMLElement | null => {
  let node: HTMLElement | null = el.parentElement
  while (node) {
    const overflowY = getComputedStyle(node).overflowY
    if (overflowY === 'auto' || overflowY === 'scroll') return node
    node = node.parentElement
  }
  return null
}

// 折叠时保持「组头相对容器顶边的偏移」不变，而不是把卡片甩到列表某个地方：
// 展开期间组头 sticky top-0，滚动时钉在容器顶边下方 stickyGap 处（容器 padding + 边框，
// 实测 25px）；折叠会让组体消失、滚动位置被浏览器夹紧，卡片可能落到容器顶边之上
// （刚折叠的组当场看不见）。因此折叠前先量下组头当时的偏移，折叠后把卡片对回同一偏移：
//   - 组头正钉住时 → 对回粘性位置，与容器顶边保留同样的距离；
//   - 卡片本就没钉住（组头偏移 = 卡片自身偏移）→ 需要移动的量恰好为 0，原位不动。
// 对齐用瞬时 scrollTop 修正（不走平滑滚动），避免出现“滑过去”的观感。
watch(
  () => expandedState.value[props.group.name],
  (newVal, oldVal) => {
    if (oldVal !== true || newVal !== false) return
    const card = cardRef.value
    const header = headerRef.value
    const scroller = card ? findScrollParent(card) : null
    if (!card || !header || !scroller) return
    // 此刻 DOM 仍是展开态，组头若被钉住则此处量到的就是粘性偏移
    const gap = header.getBoundingClientRect().top - scroller.getBoundingClientRect().top
    nextTick(() => {
      const delta = card.getBoundingClientRect().top - scroller.getBoundingClientRect().top
      const shift = delta - gap
      if (shift !== 0) scroller.scrollTop += shift
    })
  }
)

const shouldUseBar = computed(() => {
  return props.group.all.length > 10
})

const getGroupBarSegments = computed(() => {
  const nodes = props.group.all || []
  if (nodes.length === 0) return []
  const { low, mid } = delayThresholds.value
  let green = 0, yellow = 0, red = 0, loading = 0, none = 0
  nodes.forEach(name => {
    const delay = getEffectiveDelay(name)
    if (delay === undefined || delay === null) none++
    else if (delay === 0) loading++
    else if (delay === -1) red++
    else if (delay > 0 && delay <= low) green++
    else if (delay > low && delay <= mid) yellow++
    else red++
  })
  const total = nodes.length
  return [
    { pct: (green / total) * 100, class: 'bg-success' },
    { pct: (yellow / total) * 100, class: 'bg-amber-500' },
    { pct: (red / total) * 100, class: 'bg-red-500' },
    { pct: (loading / total) * 100, class: 'bg-slate-300 dark:bg-slate-700 animate-pulse' },
    { pct: (none / total) * 100, class: 'bg-slate-200 dark:bg-slate-800' }
  ].filter(s => s.pct > 0)
})

const getGroupDotSegments = computed(() => {
  const nodes = props.group.all || []
  const { low, mid } = delayThresholds.value
  return nodes.map(name => {
    const delay = getEffectiveDelay(name)
    const isSelected = props.group.now === name
    let colorClass = 'bg-slate-200 dark:bg-slate-800'
    if (delay === 0) colorClass = 'bg-slate-300 dark:bg-slate-700 animate-pulse'
    else if (delay === -1) colorClass = 'bg-red-500'
    else if (delay && delay > 0 && delay <= low) colorClass = 'bg-success'
    else if (delay && delay > low && delay <= mid) colorClass = 'bg-amber-500'
    else if (delay && delay > mid) colorClass = 'bg-red-400'
    return { name, isSelected, colorClass }
  })
})

// ===== 排序计算属性（增强版） =====
const sortedNodes = computed(() => {
  let nodes = props.group.all

  // 应用正则过滤（如果存在）
  const regexStr = filterRegex.value
  if (regexStr) {
    try {
      const regex = new RegExp(regexStr)
      nodes = nodes.filter(name => !regex.test(name))
    } catch (e) {
      console.warn('Invalid filter regex:', regexStr)
    }
  }

  const order = sortOrder.value

  // 提取纯文本排序键（去除 Emoji、特殊符号，保留字母数字汉字空格连字符点）
  const getSortKey = (name: string) =>
    name.replace(/[^\p{L}\p{N}\s\-.]/gu, '').trim()

  if (order === 'default') return nodes

  if (order === 'name') {
    return [...nodes].sort((a, b) =>
      getSortKey(a).localeCompare(getSortKey(b))
    )
  }

  if (order === 'delay') {
    return [...nodes].sort((a, b) => {
      const da = getEffectiveDelay(a)
      const db = getEffectiveDelay(b)
      const getVal = (d: number | undefined) => {
        if (d === undefined || d === null || d <= 0) return Infinity
        return d
      }
      const va = getVal(da)
      const vb = getVal(db)
      if (va !== vb) return va - vb
      // 延迟相同，按纯净名称排序
      return getSortKey(a).localeCompare(getSortKey(b))
    })
  }

  if (order === 'quality') {
    return [...nodes].sort((a, b) => {
      const sa = qualityScores.value[a] ?? 0
      const sb = qualityScores.value[b] ?? 0
      if (sa !== sb) return sb - sa // 降序
      return a.localeCompare(b)
    })
  }

  return nodes
})

// 渲染行视图模型：一次遍历把每个节点的"有效节点 + 有效延迟 + 质量分"算好，
// 模板只读扁平字段。此前模板内直接调用 getEffectiveNode/getEffectiveDelay
// 各 9 次与 2 次（每节点每帧），现降为 O(1) 属性读取。
const nodeRows = computed(() =>
  sortedNodes.value.map(name => ({
    name,
    node: getEffectiveNode(name),
    delay: getEffectiveDelay(name),
    quality: qualityScores.value[name],
  }))
)

const gridRef = ref<HTMLElement | null>(null)
// 展开时若节点数 > 10 且当前选中节点不在视口内，滚动到该节点（仅展开方向；
// 折叠一律原位进行，不触发任何滚动）
watch(
  () => expandedState.value[props.group.name],
  (isExpanded) => {
    if (isExpanded && props.group.all.length > 10) {
      nextTick(() => {
        if (!gridRef.value) return
        const selectedEl = gridRef.value.querySelector('.border-accent')
        if (selectedEl) {
          const rect = selectedEl.getBoundingClientRect()
          const isVisible = rect.top >= 0 && rect.bottom <= window.innerHeight
          if (!isVisible) {
            selectedEl.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'smooth' })
          }
        }
      })
    }
  },
  { immediate: true }
)

const handleSelectProxy = async (proxyName: string) => {
  if (delays.value[proxyName] === 0) {
    globalStore.showToast(t('proxies.testing'), 'warning')
    return
  }
  const originalNow = props.group.now
  props.group.now = proxyName // 乐观更新

  try {
    const encodedGroup = encodeURIComponent(props.group.name)
    const resp = await apiFetch(`/proxies/${encodedGroup}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: proxyName })
    })
    if (resp.ok) {
      globalStore.showToast(`${t('proxies.switched')}: ${props.group.name} → ${proxyName}`, 'success')
    } else {
      props.group.now = originalNow
      globalStore.showToast(t('proxies.switch_failed'), 'error')
    }
  } catch (e: any) {
    props.group.now = originalNow
    globalStore.showToast(t('proxies.switch_failed') + ': ' + e.message, 'error')
  } finally {
    // 刷新代理数据，同步概览页
    await proxyStore.fetchProxies(true)

    // 仅在开关打开时执行
    if (autoCloseConnections.value) {
      const connectionsStore = useConnectionsStore()
      const connsToClose = connectionsStore.activeConnections.filter(conn => {
        if (!conn.chains || !Array.isArray(conn.chains)) return false
        // 忽略大小写匹配，便于比较
        const chainsUpper = conn.chains.map(c => c.toUpperCase())
        // 如果链路中包含新节点，则不关闭
        if (chainsUpper.includes(proxyName.toUpperCase())) return false
        // 如果链路中包含 DIRECT，则不关闭（符合“除了包含 DIRECT 的链路”）
        if (chainsUpper.includes('DIRECT')) return false
        // 否则关闭
        return true
      })
      if (connsToClose.length > 0) {
        // 异步并发关闭，不阻塞主流程
        Promise.all(
          connsToClose.map(conn =>
            apiFetch(`/connections/${conn.id}`, { method: 'DELETE' }).catch(() => {})
          )
        )
      }
    }
  }
}

const handleTestSingle = async (proxyName: string) => {
  if (delays.value[proxyName] === 0) return
  await proxyStore.testDelay(proxyName)
  proxyStore.fetchProxies(true)
}

const handleTestGroup = async () => {
  if (isTesting.value) return
  isTesting.value = true
  try {
    const groupName = props.group.name
    const testURL = 'http://www.gstatic.com/generate_204'
    const timeout = 5000
    const endpoint = `/group/${encodeURIComponent(groupName)}/delay?url=${encodeURIComponent(testURL)}&timeout=${timeout}`
    const resp = await apiFetch(endpoint)
    if (resp.ok) {
      // 延迟映射与「全部测速」共用 store 的同一套判定（正值=毫秒、0=测速中、其余=超时）
      proxyStore.applyDelayMap(await resp.json())
      // 刷新全部数据（色条、历史等也会更新）
      proxyStore.fetchProxies(true)
      globalStore.showToast(t('proxies.test_complete'), 'success')
    } else {
      globalStore.showToast(`${t('common.operation_failed')}: ${await readErrorMessage(resp)}`, 'error')
    }
  } catch (e) {
    globalStore.showToast(t('common.operation_failed'), 'error')
  } finally {
    isTesting.value = false
  }
}

const getDelayClass = (delay?: number) => {
  if (delay === undefined) return 'bg-slate-100/80 dark:bg-slate-800/80 border-slate-200 dark:border-slate-700 text-slate-500 dark:text-slate-400 hover:bg-accent hover:text-white hover:border-accent'
  if (delay === 0) return 'bg-slate-100 dark:bg-slate-800 border-slate-200 dark:border-slate-700 text-slate-400 animate-pulse'
  if (delay === -1) return 'bg-red-500/10 border-red-500/20 text-red-500 dark:text-red-400 hover:bg-red-500 hover:text-white hover:border-red-500'
  if (delay <= 200) return 'bg-success/10 border-success/20 text-success dark:text-success hover:bg-success hover:text-white hover:border-success'
  if (delay <= 500) return 'bg-amber-500/10 border-amber-500/20 text-amber-500 dark:text-amber-400 hover:bg-amber-500 hover:text-white hover:border-amber-500'
  return 'bg-red-500/10 border-red-500/20 text-red-400 dark:text-red-400 hover:bg-red-500 hover:text-white hover:border-red-500'
}

const getDelayText = (delay?: number) => {
  if (delay === undefined) return t('proxies.test')
  if (delay === 0) return '...'
  if (delay === -1) return t('proxies.timeout')
  return `${delay}ms`
}
</script>

<template>
  <div ref="cardRef" class="bg-slate-50/50 dark:bg-slate-900/30 rounded-xl border border-slate-200/40 dark:border-slate-800/40 transition-all relative">
    <!-- 头部：展开时粘性（折叠后按它的偏移对齐，见上方 watcher） -->
    <div
      ref="headerRef"
      class="px-4 sm:px-5 pt-4 sm:pt-5 pb-2 rounded-t-xl cursor-pointer select-none transition-shadow duration-200"
      :class="[
        expandedState[group.name]
          ? 'sticky top-0 z-10 shadow-sm bg-[var(--bg-card)] border-b border-slate-100 dark:border-slate-800/80'
          : 'bg-slate-50/50 dark:bg-slate-900/30'
      ]"
      @click="expandedState[group.name] = !expandedState[group.name]"
    >
      <div class="flex flex-col gap-2">
        <div class="flex items-center justify-between gap-4">
          <div class="flex items-center gap-2.5 min-w-0">
            <ChevronForwardOutline class="w-3.5 h-3.5 text-slate-400 shrink-0 transition-transform duration-200" :class="{ 'rotate-90': expandedState[group.name] }" />
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <span class="text-sm font-bold text-slate-800 dark:text-slate-100 truncate">{{ group.name }}</span>
                <span class="px-2 py-0.5 text-[11px] font-extrabold bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 rounded uppercase shrink-0">{{ group.type }}</span>
              </div>
              <div class="text-xs text-slate-500 dark:text-slate-400 mt-0.5 truncate">
                {{ t('proxies.current') }}: <span class="font-bold text-accent select-none">{{ group.now }}</span>
              </div>
            </div>
          </div>

          <div class="flex items-center gap-1.5">
            <button
              v-if="sortOrder === 'quality'"
              @click.stop="proxyStore.fetchQualityScores()"
              class="p-1.5 text-slate-400 hover:text-accent rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all shrink-0"
              :title="t('proxies.quality_score')"
            >
              <LayersOutline class="w-4 h-4" />
            </button>
            <button @click.stop="handleTestGroup" :disabled="isTesting" class="p-2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all shrink-0" :title="t('proxies.test')">
              <SyncOutline class="w-4 h-4" :class="{ 'animate-spin': isTesting }" />
            </button>
          </div>
        </div>

        <!-- Health Indicator -->
        <div class="group-health flex items-center flex-wrap w-full mt-1" :class="shouldUseBar ? 'h-1.5 overflow-hidden gap-0' : 'h-2 gap-1'">
          <template v-if="shouldUseBar">
            <span
              v-for="(seg, sIdx) in getGroupBarSegments"
              :key="sIdx"
              :style="{ flex: seg.pct }"
              :class="[seg.class, 'h-full', sIdx === 0 ? 'rounded-l-sm' : '', sIdx === getGroupBarSegments.length - 1 ? 'rounded-r-sm' : '']"
            ></span>
          </template>
          <template v-else>
            <span
              v-for="(dot, dIdx) in getGroupDotSegments"
              :key="dIdx"
              :class="[dot.colorClass, 'w-2 h-2 rounded-full flex-shrink-0 relative']"
              :title="dot.name"
            >
              <span v-if="dot.isSelected" class="absolute top-[2px] left-[2px] w-1 h-1 rounded-full bg-white"></span>
            </span>
          </template>
        </div>
      </div>
    </div>

    <!-- 主体（节点网格）：展开时显示（展开可滚动到当前选中节点，折叠不触发滚动） -->
    <div v-if="expandedState[group.name]" ref="gridRef" class="grid grid-cols-2 gap-2.5 px-4 sm:px-5 pb-4 sm:pb-5 pt-4 border-t border-slate-100 dark:border-slate-800/80">
      <div
        v-for="row in nodeRows"
        :key="row.name"
        @click="handleSelectProxy(row.name)"
        class="live-card live-card--lift-2 flex flex-col justify-between p-2.5 text-xs rounded-xl border transition-all duration-300 cursor-pointer min-h-[75px] relative"
        :class="group.now === row.name
          ? 'bg-accent/10 dark:bg-accent/15 border-accent text-accent shadow-sm ring-1 ring-accent/30 hover:-translate-y-[2px] hover:shadow-md'
          : 'border-slate-200/60 dark:border-slate-800 hover:-translate-y-[2px] hover:shadow-md hover:border-slate-300/80 dark:hover:border-slate-700 bg-slate-50/50 dark:bg-slate-900/30 text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800/50'"
      >
        <div class="w-full text-left">
          <span class="block truncate text-xs font-bold text-slate-800 dark:text-slate-100 leading-snug" :class="{ '!text-accent': group.now === row.name }" :title="row.name">
            {{ row.name }}
          </span>
        </div>

        <!-- 实际节点数据（链式解析结果由 Store 预计算） -->
        <div v-if="row.node" class="flex justify-between items-center gap-1.5 mt-2.5 w-full select-none">
          <div class="flex items-center gap-1 min-w-0">
            <span class="bg-slate-200/80 dark:bg-slate-800/80 text-slate-500 dark:text-slate-400 px-1 py-0.5 rounded font-mono uppercase text-[9px] font-bold leading-none truncate">
              {{ row.node.type || 'DIRECT' }}
            </span>
            <span v-if="row.node.xudp" class="bg-emerald-500/10 text-emerald-500 dark:text-emerald-400 px-1 py-0.5 rounded font-mono font-extrabold text-[9px] leading-none shrink-0" title="XUDP">X</span>
            <span v-else-if="row.node.udp" class="bg-blue-500/10 text-blue-500 dark:text-blue-400 px-1 py-0.5 rounded font-mono font-extrabold text-[9px] leading-none shrink-0" title="UDP">U</span>
          </div>
          <div class="flex items-center gap-1 shrink-0">
            <span v-if="sortOrder === 'quality' && row.quality !== undefined" 
                  class="text-[10px] font-mono shrink-0 select-none px-1.5 py-0.5 rounded-md leading-none text-center bg-accent/10 text-accent border border-accent/20">
              {{ row.quality }}
            </span>
            <span
              class="text-[10px] font-mono shrink-0 select-none px-1.5 py-0.5 rounded-md leading-none text-center min-w-[32px] transition-all hover:scale-105 active:scale-95 border cursor-pointer"
              :class="getDelayClass(row.delay)"
              @click.stop="handleTestSingle(row.name)"
            >
              {{ getDelayText(row.delay) }}
            </span>
          </div>
        </div>

        <!-- 历史色条 -->
        <div v-if="row.node" class="flex gap-[2px] w-full mt-2 h-1 overflow-hidden">
          <template v-if="row.node.recentColors && row.node.recentColors.length > 0">
            <span
              v-for="(hist, hIdx) in row.node.recentColors"
              :key="hIdx"
              :class="[hist.colorClass, 'flex-1 h-full rounded-sm']"
              :title="hist.title"
            ></span>
          </template>
          <template v-else>
            <span class="flex-1 h-full bg-slate-200/60 dark:bg-slate-800/40 rounded-sm"></span>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>