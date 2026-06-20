<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiFetch } from '../utils/api'
import { GlobeOutline, RefreshOutline, ChevronForwardOutline } from '@vicons/ionicons5'
import { storeToRefs } from 'pinia'
import { useProxyStore, type ProxyGroup } from '../store/proxies'
import { useGlobalStore } from '../store/global'

const { t } = useI18n()
const proxyStore = useProxyStore()
const globalStore = useGlobalStore()
const { proxyGroups, delays, allProxiesRaw, isLoading, expandedState } = storeToRefs(proxyStore)

const isTestingGroup = ref<Record<string, boolean>>({})
const isTestingAll = ref(false)
let refreshTimer: number | null = null

// 判断代理组是否应使用长条混合比例健康度
const shouldUseBar = (groupName: string): boolean => {
  if (!groupName) return false
  const barKeywords = ['自动选择', '手动选择', '地区选择', '美国', '日本', '香港', '新加坡', '台湾']
  return barKeywords.some(kw => groupName.includes(kw)) || groupName === 'GLOBAL'
}

// 获取节点最新延迟数值
const getLatestDelay = (name: string): number | null => {
  const node = allProxiesRaw.value[name]
  if (!node || !node.history || node.history.length === 0) return null
  const sorted = [...node.history].sort((a: any, b: any) => new Date(a.time).getTime() - new Date(b.time).getTime())
  const last = sorted[sorted.length - 1]
  return last.delay > 0 ? last.delay : -1
}

// 转换历史延迟为颜色类名
const getHistoryColorClass = (delay?: number | null) => {
  if (delay === undefined || delay === null) return 'bg-slate-200 dark:bg-slate-800'
  if (delay === 0) return 'bg-[#1a1a1a]'
  if (delay === -1) return 'bg-red-500'
  if (delay <= 150) return 'bg-success'
  if (delay <= 300) return 'bg-amber-500'
  return 'bg-red-400'
}

// 计算组混合比例长条线段
const getGroupBarSegments = (group: ProxyGroup) => {
  const nodes = group.all || []
  if (nodes.length === 0) return []
  
  let green = 0, yellow = 0, red = 0, black = 0, none = 0
  
  nodes.forEach(name => {
    const delay = getLatestDelay(name)
    if (delay === null) none++
    else if (delay === 0) black++
    else if (delay === -1) red++
    else if (delay >= 1 && delay <= 150) green++
    else if (delay <= 300) yellow++
    else red++
  })
  
  const total = nodes.length
  return [
    { pct: (green / total) * 100, class: 'bg-success' },
    { pct: (yellow / total) * 100, class: 'bg-amber-500' },
    { pct: (red / total) * 100, class: 'bg-red-500' },
    { pct: (black / total) * 100, class: 'bg-[#1a1a1a]' },
    { pct: (none / total) * 100, class: 'bg-slate-200 dark:bg-slate-800' }
  ].filter(s => s.pct > 0)
}

// 计算组内每个小圆点的状态信息
const getGroupDotSegments = (group: ProxyGroup) => {
  const nodes = group.all || []
  return nodes.map(name => {
    const delay = getLatestDelay(name)
    const isSelected = group.now === name
    let colorClass = 'bg-slate-200 dark:bg-slate-800'
    if (delay === 0) colorClass = 'bg-[#1a1a1a]'
    else if (delay === -1) colorClass = 'bg-red-500'
    else if (delay && delay >= 1 && delay <= 150) colorClass = 'bg-success'
    else if (delay && delay <= 300) colorClass = 'bg-amber-500'
    else if (delay && delay > 300) colorClass = 'bg-red-400'
    return {
      name,
      isSelected,
      colorClass
    }
  })
}

// 切换代理组中的当前节点选择
const handleSelectProxy = async (groupName: string, proxyName: string) => {
  // 如果节点正在测速，拦截切换请求
  if (delays.value[proxyName] === 0) {
    globalStore.showToast(t('proxies.testing'), 'warning')
    return
  }
  const group = proxyGroups.value.find(g => g.name === groupName)
  if (!group) return

  const originalNow = group.now
  // 乐观更新：即时修改本地选中项状态
  group.now = proxyName

  try {
    const encodedGroup = encodeURIComponent(groupName)
    const resp = await apiFetch(`/proxies/${encodedGroup}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: proxyName })
    })
    if (resp.ok) {
      globalStore.showToast(`${t('proxies.switched')}: ${groupName} → ${proxyName}`, 'success')
    } else {
      // 失败回滚
      group.now = originalNow
      globalStore.showToast(t('proxies.switch_failed'), 'error')
    }
  } catch (e: any) {
    // 异常回滚
    group.now = originalNow
    console.error('切换代理失败', e)
    globalStore.showToast(t('proxies.switch_failed') + ': ' + e.message, 'error')
  }
}

// 测速单个节点
const handleTestSingle = async (proxyName: string) => {
  if (delays.value[proxyName] === 0) return
  await proxyStore.testDelay(proxyName)
  // 静默刷新数据，以更新最近 5 次历史测速的色块状态
  proxyStore.fetchProxies(true)
}

// 测速代理组
const handleTestGroup = async (group: ProxyGroup) => {
  if (isTestingGroup.value[group.name]) return
  isTestingGroup.value[group.name] = true
  try {
    await proxyStore.testProxiesWithConcurrency(group.all)
    proxyStore.fetchProxies(true)
    globalStore.showToast(t('proxies.test_complete'), 'success')
  } catch (e) {
    console.error('测速代理组异常', e)
    globalStore.showToast(t('common.operation_failed'), 'error')
  } finally {
    isTestingGroup.value[group.name] = false
  }
}

// 测速所有节点
const handleTestAll = async () => {
  if (isTestingAll.value) return
  isTestingAll.value = true
  globalStore.showToast(t('proxies.testing_all'), 'info')
  try {
    const allProxies = new Set<string>()
    proxyGroups.value.forEach(g => {
      g.all.forEach(name => allProxies.add(name))
    })
    await proxyStore.testProxiesWithConcurrency(Array.from(allProxies))
    proxyStore.fetchProxies(true)
    globalStore.showToast(t('proxies.test_complete'), 'success')
  } catch (e) {
    console.error('测速所有代理异常', e)
    globalStore.showToast(t('common.operation_failed'), 'error')
  } finally {
    isTestingAll.value = false
  }
}

// 自动测速（无 Toast，仅对没有有效延迟记录的节点）
const autoTestMissingNodes = async () => {
  const allNodes = new Set<string>()
  proxyGroups.value.forEach(g => {
    if (['Selector', 'URLTest', 'Fallback'].includes(g.type)) {
      g.all.forEach(name => allNodes.add(name))
    }
  })

  const needTest: string[] = []
  allNodes.forEach(name => {
    const d = delays.value[name]
    // 优化：仅对从未测速过（undefined）的节点自动测速，排除已超时或有结果的节点以节约开销
    if (d === undefined) {
      needTest.push(name)
    }
  })

  if (needTest.length === 0) return

  await proxyStore.testProxiesWithConcurrency(needTest)
  proxyStore.fetchProxies(true)
}

// 延迟着色
const getDelayClass = (delay?: number) => {
  if (delay === undefined) {
    return 'bg-slate-100/80 dark:bg-slate-800/80 border-slate-200 dark:border-slate-700 text-slate-500 dark:text-slate-400 hover:bg-accent hover:text-white hover:border-accent'
  }
  if (delay === 0) {
    return 'bg-slate-100 dark:bg-slate-800 border-slate-200 dark:border-slate-700 text-slate-400 animate-pulse'
  }
  if (delay === -1) {
    return 'bg-red-500/10 border-red-500/20 text-red-500 dark:text-red-400 hover:bg-red-500 hover:text-white hover:border-red-500'
  }
  if (delay <= 150) {
    return 'bg-success/10 border-success/20 text-success dark:text-success hover:bg-success hover:text-white hover:border-success'
  }
  if (delay <= 300) {
    return 'bg-amber-500/10 border-amber-500/20 text-amber-500 dark:text-amber-400 hover:bg-amber-500 hover:text-white hover:border-amber-500'
  }
  return 'bg-red-500/10 border-red-500/20 text-red-400 dark:text-red-400 hover:bg-red-500 hover:text-white hover:border-red-500'
}

const getDelayText = (delay?: number) => {
  if (delay === undefined) return t('proxies.test')
  if (delay === 0) return '...'
  if (delay === -1) return t('proxies.timeout')
  return `${delay}ms`
}

onMounted(async () => {
  const hasData = proxyGroups.value.length > 0
  await proxyStore.fetchProxies(hasData)

  // 触发自动测速（后台静默）
  autoTestMissingNodes().catch(e => console.warn('[Proxies] 自动测速失败:', e))

  // 每10秒后台静默更新一次代理状态
  refreshTimer = window.setInterval(() => {
    proxyStore.fetchProxies(true)
  }, 10000)
})

onUnmounted(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer)
  }
})
</script>

<template>
  <div class="space-y-6">
    <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm flex items-center justify-between transition-all">
      <h3 class="text-base font-semibold flex items-center gap-2">
        <GlobeOutline class="w-5 h-5 text-accent" />
        {{ t('proxies.title') }}
      </h3>

      <button @click="handleTestAll" :disabled="isTestingAll" class="px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all flex items-center gap-1.5">
        <RefreshOutline class="w-3.5 h-3.5" :class="{ 'animate-spin': isTestingAll }" />
        {{ isTestingAll ? t('proxies.testing') : t('proxies.test_all') }}
      </button>
    </div>

    <div v-if="isLoading && proxyGroups.length === 0" class="p-8 text-center text-slate-400 dark:text-slate-600 text-sm">
      {{ t('common.loading') }}
    </div>
    <div v-else-if="proxyGroups.length === 0" class="p-8 text-center text-slate-400 dark:text-slate-600 text-sm">
      {{ t('proxies.empty') }}
    </div>

    <!-- 响应式双列/多列网格布局 -->
    <div v-else class="grid grid-cols-1 lg:grid-cols-2 gap-6 items-start">
      <div v-for="group in proxyGroups" :key="group.name" class="bg-white dark:bg-[#1e293b] p-4 sm:p-5 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all space-y-4">
        
        <!-- Accordion Header -->
        <div @click="expandedState[group.name] = !expandedState[group.name]" class="flex flex-col gap-2 cursor-pointer select-none pb-2">
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
            
            <button @click.stop="handleTestGroup(group)" :disabled="isTestingGroup[group.name]" class="p-2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all shrink-0" :title="t('proxies.test')">
              <RefreshOutline class="w-4 h-4" :class="{ 'animate-spin': isTestingGroup[group.name] }" />
            </button>
          </div>

          <!-- 组健康度指示器 -->
          <div class="group-health flex gap-1 items-center flex-wrap w-full mt-1"
            :class="shouldUseBar(group.name) ? 'h-1.5 overflow-hidden' : 'h-2'">
            <!-- 比例线段条 (Bar) -->
            <template v-if="shouldUseBar(group.name)">
              <span
                v-for="(seg, sIdx) in getGroupBarSegments(group)"
                :key="sIdx"
                :style="{ flex: seg.pct }"
                :class="[seg.class, 'h-full', sIdx === 0 ? 'rounded-l-sm' : '', sIdx === getGroupBarSegments(group).length - 1 ? 'rounded-r-sm' : '']"
              ></span>
            </template>
            <!-- 离散状态小圆点队列 (Dots) -->
            <template v-else>
              <span
                v-for="(dot, dIdx) in getGroupDotSegments(group)"
                :key="dIdx"
                :class="[dot.colorClass, 'w-2 h-2 rounded-full flex-shrink-0 relative']"
                :title="dot.name"
              >
                <span v-if="dot.isSelected" class="absolute top-[2px] left-[2px] w-1 h-1 rounded-full bg-white"></span>
              </span>
            </template>
          </div>
        </div>

        <!-- Accordion Body (节点卡片网格) -->
        <div v-if="expandedState[group.name]" class="grid grid-cols-2 sm:grid-cols-3 gap-2.5 pt-4 border-t border-slate-100 dark:border-slate-800/80">
          <div
            v-for="name in group.all"
            :key="name"
            @click="handleSelectProxy(group.name, name)"
            class="flex flex-col justify-between p-2.5 text-xs rounded-xl border transition-all duration-200 cursor-pointer min-h-[75px]"
            :class="group.now === name
              ? 'bg-accent/5 border-accent text-accent shadow-sm ring-1 ring-accent/30'
              : 'border-slate-200/60 dark:border-slate-800 hover:border-slate-300 dark:hover:border-slate-700 bg-slate-50/50 dark:bg-slate-900/30 text-slate-700 dark:text-slate-300'"
          >
            <!-- 节点名称 & 单独测速按钮 -->
            <div class="flex justify-between items-start gap-2 w-full">
              <span class="truncate text-xs font-bold text-slate-800 dark:text-slate-100 flex-1 leading-snug" :class="{ '!text-accent': group.now === name }" :title="name">{{ name }}</span>
              <span
                class="text-[10px] font-mono shrink-0 select-none px-1.5 py-0.5 rounded-md leading-none text-center min-w-[32px] transition-all hover:scale-105 active:scale-95 border cursor-pointer"
                :class="getDelayClass(delays[name])"
                @click.stop="handleTestSingle(name)"
              >
                {{ getDelayText(delays[name]) }}
              </span>
            </div>

            <!-- 协议类型名称 & UDP / XUDP 标签徽章 -->
            <div
              v-if="allProxiesRaw[name]"
              class="flex gap-1 font-bold text-[10px] mt-2 flex-wrap select-none"
            >
              <span class="bg-slate-200/80 dark:bg-slate-800/80 text-slate-500 dark:text-slate-400 px-1 py-0.5 rounded font-mono uppercase">
                {{ allProxiesRaw[name].type || 'DIRECT' }}
              </span>
              <span
                v-if="allProxiesRaw[name].udp"
                class="bg-blue-500/15 text-blue-500 dark:text-blue-400 px-1.5 py-0.5 rounded"
              >
                UDP
              </span>
              <span
                v-if="allProxiesRaw[name].xudp"
                class="bg-emerald-500/15 text-emerald-500 dark:text-emerald-400 px-1.5 py-0.5 rounded"
              >
                XUDP
              </span>
            </div>

            <!-- 最近 5 次历史测速状态色块条 -->
            <div
              v-if="allProxiesRaw[name]"
              class="flex gap-[2px] w-full mt-2 h-1 overflow-hidden"
            >
              <template v-if="allProxiesRaw[name].history && allProxiesRaw[name].history.length > 0">
                <span
                  v-for="(hist, hIdx) in [...allProxiesRaw[name].history].sort((a, b) => new Date(a.time).getTime() - new Date(b.time).getTime()).slice(-5)"
                  :key="hIdx"
                  :class="[getHistoryColorClass(hist.delay), 'flex-1 h-full rounded-sm']"
                  :title="`${hist.time}: ${hist.delay}ms`"
                ></span>
              </template>
              <template v-else>
                <span class="flex-1 h-full bg-slate-200/60 dark:bg-slate-800/40 rounded-sm"></span>
              </template>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
