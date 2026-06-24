<script setup lang="ts">
import { ref, computed } from 'vue'
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
const { delays, allProxiesRaw, expandedState } = storeToRefs(proxyStore)

const isTesting = ref(false)

const shouldUseBar = computed(() => {
  return props.group.all.length > 10
})

const getGroupBarSegments = computed(() => {
  const nodes = props.group.all || []
  if (nodes.length === 0) return []
  
  let green = 0, yellow = 0, red = 0, black = 0, none = 0
  nodes.forEach(name => {
    const delay = delays.value[name]
    if (delay === undefined || delay === null) none++
    else if (delay === 0) black++
    else if (delay === -1) red++
    else if (delay >= 1 && delay <= 200) green++
    else if (delay <= 500) yellow++
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
})

const getGroupDotSegments = computed(() => {
  const nodes = props.group.all || []
  return nodes.map(name => {
    const delay = delays.value[name]
    const isSelected = props.group.now === name
    let colorClass = 'bg-slate-200 dark:bg-slate-800'
    if (delay === 0) colorClass = 'bg-[#1a1a1a]'
    else if (delay === -1) colorClass = 'bg-red-500'
    else if (delay && delay >= 1 && delay <= 200) colorClass = 'bg-success'
    else if (delay && delay <= 500) colorClass = 'bg-amber-500'
    else if (delay && delay > 500) colorClass = 'bg-red-400'
    return { name, isSelected, colorClass }
  })
})

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
  <div class="bg-slate-50/50 dark:bg-slate-900/30 p-4 sm:p-5 rounded-xl border border-slate-200/40 dark:border-slate-800/40 transition-all space-y-4">
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
        
        <button @click.stop="handleTestGroup" :disabled="isTesting" class="p-2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all shrink-0" :title="t('proxies.test')">
          <SyncOutline class="w-4 h-4" :class="{ 'animate-spin': isTesting }" />
        </button>
      </div>

      <!-- Health Indicator -->
      <div class="group-health flex gap-1 items-center flex-wrap w-full mt-1" :class="shouldUseBar ? 'h-1.5 overflow-hidden' : 'h-2'">
        <!-- Bar Segments -->
        <template v-if="shouldUseBar">
          <span
            v-for="(seg, sIdx) in getGroupBarSegments"
            :key="sIdx"
            :style="{ flex: seg.pct }"
            :class="[seg.class, 'h-full', sIdx === 0 ? 'rounded-l-sm' : '', sIdx === getGroupBarSegments.length - 1 ? 'rounded-r-sm' : '']"
          ></span>
        </template>
        <!-- Dot Indicators -->
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

    <!-- Accordion Body -->
    <div v-if="expandedState[group.name]" class="grid grid-cols-2 gap-2.5 pt-4 border-t border-slate-100 dark:border-slate-800/80">
      <div
        v-for="name in group.all"
        :key="name"
        @click="handleSelectProxy(name)"
        class="live-card flex flex-col justify-between p-2.5 text-xs rounded-xl border transition-all duration-300 cursor-pointer min-h-[75px] relative"
        :class="group.now === name
          ? 'bg-accent/10 dark:bg-accent/15 border-accent text-accent shadow-sm ring-1 ring-accent/30 hover:-translate-y-[2px] hover:shadow-md'
          : 'border-slate-200/60 dark:border-slate-800 hover:-translate-y-[2px] hover:shadow-md hover:border-slate-300/80 dark:hover:border-slate-700 bg-slate-50/50 dark:bg-slate-900/30 text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800/50'"
      >
        <!-- Name -->
        <div class="w-full text-left">
          <span class="block truncate text-xs font-bold text-slate-800 dark:text-slate-100 leading-snug" :class="{ '!text-accent': group.now === name }" :title="name">
            {{ name }}
          </span>
        </div>

        <!-- Protocol & Test -->
        <div v-if="allProxiesRaw[name]" class="flex justify-between items-center gap-1.5 mt-2.5 w-full select-none">
          <div class="flex items-center gap-1 min-w-0">
            <span class="bg-slate-200/80 dark:bg-slate-800/80 text-slate-500 dark:text-slate-400 px-1 py-0.5 rounded font-mono uppercase text-[9px] font-bold leading-none truncate">
              {{ allProxiesRaw[name].type || 'DIRECT' }}
            </span>
            <span v-if="allProxiesRaw[name].xudp" class="bg-emerald-500/10 text-emerald-500 dark:text-emerald-400 px-1 py-0.5 rounded font-mono font-extrabold text-[9px] leading-none shrink-0" title="XUDP">X</span>
            <span v-else-if="allProxiesRaw[name].udp" class="bg-blue-500/10 text-blue-500 dark:text-blue-400 px-1 py-0.5 rounded font-mono font-extrabold text-[9px] leading-none shrink-0" title="UDP">U</span>
          </div>
          <span
            class="text-[10px] font-mono shrink-0 select-none px-1.5 py-0.5 rounded-md leading-none text-center min-w-[32px] transition-all hover:scale-105 active:scale-95 border cursor-pointer"
            :class="getDelayClass(delays[name])"
            @click.stop="handleTestSingle(name)"
          >
            {{ getDelayText(delays[name]) }}
          </span>
        </div>

        <!-- History Colors -->
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
