<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { apiFetch } from '../utils/api'
import { ChevronForwardOutline, SyncOutline } from '@vicons/ionicons5'
import { useProxyStore, type ProxyGroup } from '../store/proxies'
import { useGlobalStore } from '../store/global'

const props = defineProps<{
  group: ProxyGroup
}>()

const { t } = useI18n()
const proxyStore = useProxyStore()
const globalStore = useGlobalStore()
const { delays, allProxiesRaw, expandedState, sortOrder, delayThresholds, qualityScores, filterRegex } = storeToRefs(proxyStore)

const isTesting = ref(false)

// 卡片容器引用（用于折叠后滚动）
const cardRef = ref<HTMLElement | null>(null)

// 折叠时滚动到顶部
watch(
  () => expandedState.value[props.group.name],
  (newVal, oldVal) => {
    if (oldVal === true && newVal === false) {
      nextTick(() => {
        cardRef.value?.scrollIntoView({ block: 'start', inline: 'nearest', behavior: 'smooth' })
      })
    }
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
    const delay = delays.value[name]
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
    const delay = delays.value[name]
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
        // 无效正则，忽略过滤
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
      const da = delays.value[a]
      const db = delays.value[b]
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

const gridRef = ref<HTMLElement | null>(null)
// 监听展开状态，当展开且节点数 > 10 时，滚动到选中节点
watch(
  () => expandedState.value[props.group.name],
  (isExpanded) => {
    if (isExpanded && props.group.all.length > 10) {
      nextTick(() => {
        if (!gridRef.value) return
        // 查找当前选中的节点（拥有 border-accent 类的元素）
        const selectedEl = gridRef.value.querySelector('.border-accent')
        if (selectedEl) {
          // 检查是否在视口内，若不在则滚动到视口中央
          const rect = selectedEl.getBoundingClientRect()
          const isVisible = rect.top >= 0 && rect.bottom <= window.innerHeight
          if (!isVisible) {
            selectedEl.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'smooth' })
          }
        }
      })
    }
  },
  { immediate: true }  // 组件挂载时若已展开也立即执行
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
    await proxyStore.testProxiesWithConcurrency(props.group.all)
    proxyStore.fetchProxies(true)
    globalStore.showToast(t('proxies.test_complete'), 'success')
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
    <!-- 头部：展开时粘性，使用当前主题的卡片背景色 var(--bg-card) 进行完美适配 -->
    <div
      class="px-4 sm:px-5 pt-4 sm:pt-5 pb-2 rounded-t-xl cursor-pointer select-none transition-shadow duration-200"
      :class="[
        expandedState[group.name]
          ? 'sticky top-0 z-10 shadow-sm bg-[var(--bg-card)] border-b border-slate-100 dark:border-slate-800/80'
          : 'bg-slate-50/50 dark:bg-slate-900/30'
      ]"
      @click="expandedState[group.name] = !expandedState[group.name]"
    >
      <div class="flex flex-col gap-2">
        <!-- 原有头部布局 -->
        <div class="flex items-center justify-between gap-4">
          <div class="flex items-center gap-2.5 min-w-0">
            <ChevronForwardOutline class="w-3.5 h-3.5 text-slate-400 shrink-0 transition-transform duration-200" :class="{ 'rotate-90': expandedState[group.name] }" />
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <span class="text-sm font-bold text-slate-800 dark:text-slate-100 truncate">{{ group.name }}</span>
                <span class="px-2 py-0.5 text-[11px] font-extrabold bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 rounded uppercase shrink-0">{{ group.type }}</span>
              </div>
              <div class="text-xs text-slate-500 dark:text-slate-400 mt-0.5 truncate">
                {{ t('proxies.current') }}: <span class="font-bold text-accent select-all">{{ group.now }}</span>
              </div>
            </div>
          </div>

          <div class="flex items-center gap-1.5">
            <!-- 质量评分按钮（仅质量排序时可见） -->
            <button
              v-if="sortOrder === 'quality'"
              @click.stop="proxyStore.fetchQualityScores()"
              class="p-1.5 text-slate-400 hover:text-accent rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all shrink-0"
              :title="t('proxies.quality_score')"
            >
              <svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M12 2L2 7l10 5 10-5-10-5z"/>
                <path d="M2 17l10 5 10-5"/>
                <path d="M2 12l10 5 10-5"/>
              </svg>
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

    <!-- 主体（节点网格）：展开时显示 -->
    <div v-if="expandedState[group.name]" ref="gridRef" class="grid grid-cols-2 gap-2.5 px-4 sm:px-5 pb-4 sm:pb-5 pt-4 border-t border-slate-100 dark:border-slate-800/80">
      <div
        v-for="name in sortedNodes"
        :key="name"
        @click="handleSelectProxy(name)"
        class="live-card flex flex-col justify-between p-2.5 text-xs rounded-xl border transition-all duration-300 cursor-pointer min-h-[75px] relative"
        :class="group.now === name
          ? 'bg-accent/10 dark:bg-accent/15 border-accent text-accent shadow-sm ring-1 ring-accent/30 hover:-translate-y-[2px] hover:shadow-md'
          : 'border-slate-200/60 dark:border-slate-800 hover:-translate-y-[2px] hover:shadow-md hover:border-slate-300/80 dark:hover:border-slate-700 bg-slate-50/50 dark:bg-slate-900/30 text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800/50'"
      >
        <div class="w-full text-left">
          <span class="block truncate text-xs font-bold text-slate-800 dark:text-slate-100 leading-snug" :class="{ '!text-accent': group.now === name }" :title="name">
            {{ name }}
          </span>
        </div>
        <div v-if="allProxiesRaw[name]" class="flex justify-between items-center gap-1.5 mt-2.5 w-full select-none">
          <div class="flex items-center gap-1 min-w-0">
            <span class="bg-slate-200/80 dark:bg-slate-800/80 text-slate-500 dark:text-slate-400 px-1 py-0.5 rounded font-mono uppercase text-[9px] font-bold leading-none truncate">
              {{ allProxiesRaw[name].type || 'DIRECT' }}
            </span>
            <span v-if="allProxiesRaw[name].xudp" class="bg-emerald-500/10 text-emerald-500 dark:text-emerald-400 px-1 py-0.5 rounded font-mono font-extrabold text-[9px] leading-none shrink-0" title="XUDP">X</span>
            <span v-else-if="allProxiesRaw[name].udp" class="bg-blue-500/10 text-blue-500 dark:text-blue-400 px-1 py-0.5 rounded font-mono font-extrabold text-[9px] leading-none shrink-0" title="UDP">U</span>
          </div>
          <div class="flex items-center gap-1 shrink-0">
            <!-- 质量分数：与测速按钮相同样式 -->
            <span v-if="sortOrder === 'quality' && qualityScores[name] !== undefined" 
                  class="text-[10px] font-mono shrink-0 select-none px-1.5 py-0.5 rounded-md leading-none text-center bg-accent/10 text-accent border border-accent/20">
              {{ qualityScores[name] }}
            </span>
            <span
              class="text-[10px] font-mono shrink-0 select-none px-1.5 py-0.5 rounded-md leading-none text-center min-w-[32px] transition-all hover:scale-105 active:scale-95 border cursor-pointer"
              :class="getDelayClass(delays[name])"
              @click.stop="handleTestSingle(name)"
            >
              {{ getDelayText(delays[name]) }}
            </span>
          </div>
        </div>
        <div v-if="allProxiesRaw[name]" class="flex gap-[2px] w-full mt-2 h-1 overflow-hidden">
          <template v-if="allProxiesRaw[name].recentColors && allProxiesRaw[name].recentColors.length > 0">
            <span
              v-for="(hist, hIdx) in allProxiesRaw[name].recentColors"
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