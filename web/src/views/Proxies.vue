<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiFetch } from '../utils/api'
import { GlobeOutline, RefreshOutline, ChevronForwardOutline } from '@vicons/ionicons5'
import { storeToRefs } from 'pinia'
import { useProxyStore, type ProxyGroup } from '../store/proxies'

const { t } = useI18n()
const proxyStore = useProxyStore()
const { proxyGroups, delays, isLoading } = storeToRefs(proxyStore)

const isTestingGroup = ref<Record<string, boolean>>({})
const isTestingAll = ref(false)
const expandedState = ref<Record<string, boolean>>({})

// 切换代理组中的当前节点选择
const handleSelectProxy = async (groupName: string, proxyName: string) => {
  try {
    const encodedGroup = encodeURIComponent(groupName)
    const resp = await apiFetch(`/proxies/${encodedGroup}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: proxyName })
    })
    if (resp.ok) {
      const group = proxyGroups.value.find(g => g.name === groupName)
      if (group) group.now = proxyName
    }
  } catch (e) {
    console.error('切换代理失败', e)
  }
}

// 测速单个节点
const testDelay = proxyStore.testDelay

// 测速代理组
const handleTestGroup = async (group: ProxyGroup) => {
  if (isTestingGroup.value[group.name]) return
  isTestingGroup.value[group.name] = true
  try {
    await proxyStore.testProxiesWithConcurrency(group.all)
    proxyStore.fetchProxies(true)
  } catch (e) {
    console.error('测速代理组异常', e)
  } finally {
    isTestingGroup.value[group.name] = false
  }
}

// 测速所有节点
const handleTestAll = async () => {
  if (isTestingAll.value) return
  isTestingAll.value = true
  try {
    const allProxies = new Set<string>()
    proxyGroups.value.forEach(g => {
      g.all.forEach(name => allProxies.add(name))
    })
    await proxyStore.testProxiesWithConcurrency(Array.from(allProxies))
    proxyStore.fetchProxies(true)
  } catch (e) {
    console.error('测速所有代理异常', e)
  } finally {
    isTestingAll.value = false
  }
}

// 延迟着色
const getDelayClass = (delay?: number) => {
  if (delay === undefined) return 'hidden'
  if (delay === 0) return 'text-slate-400'
  if (delay === -1) return 'text-red-500'
  if (delay <= 150) return 'text-success'
  if (delay <= 300) return 'text-amber-500'
  return 'text-red-400'
}

const getDelayText = (delay?: number) => {
  if (delay === undefined) return ''
  if (delay === 0) return '...'
  if (delay === -1) return t('proxies.timeout')
  return `${delay}ms`
}

onMounted(() => {
  const hasData = proxyGroups.value.length > 0
  proxyStore.fetchProxies(hasData)
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

    <div v-else class="space-y-6">
      <div v-for="group in proxyGroups" :key="group.name" class="bg-white dark:bg-[#1e293b] p-4 sm:p-5 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm transition-all space-y-4">
        
        <!-- Accordion Header -->
        <div @click="expandedState[group.name] = !expandedState[group.name]" class="flex items-center justify-between gap-4 cursor-pointer select-none pb-2">
          <div class="flex items-center gap-2.5 min-w-0">
            <ChevronForwardOutline class="w-3.5 h-3.5 text-slate-400 shrink-0 transition-transform duration-200" :class="{ 'rotate-90': expandedState[group.name] }" />
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <span class="text-sm font-bold text-slate-800 dark:text-slate-100 truncate">{{ group.name }}</span>
                <span class="px-1.5 py-0.5 text-[9px] font-bold bg-slate-100 dark:bg-slate-800 text-slate-400 dark:text-slate-500 rounded uppercase shrink-0">{{ group.type }}</span>
              </div>
              <div class="text-[10px] text-slate-400 dark:text-slate-500 mt-0.5 truncate">
                {{ t('proxies.current') }}: <span class="font-semibold text-accent select-all">{{ group.now }}</span>
              </div>
            </div>
          </div>
          
          <button @click.stop="handleTestGroup(group)" :disabled="isTestingGroup[group.name]" class="p-2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-850 transition-all shrink-0" :title="t('proxies.test')">
            <RefreshOutline class="w-4 h-4" :class="{ 'animate-spin': isTestingGroup[group.name] }" />
          </button>
        </div>

        <!-- Accordion Body -->
        <div v-if="expandedState[group.name]" class="flex flex-wrap gap-2.5 pt-2 border-t border-slate-100 dark:border-slate-800/80">
          <button v-for="name in group.all" :key="name" @click="handleSelectProxy(group.name, name)" class="px-3 py-1.5 text-xs rounded-xl border font-medium transition-all duration-200 flex items-center gap-2" :class="group.now === name ? 'bg-accent/10 border-accent text-accent shadow-sm' : 'border-slate-200/60 dark:border-slate-800 hover:border-slate-300 dark:hover:border-slate-700 bg-slate-50/50 dark:bg-slate-900/30 text-slate-600 dark:text-slate-300'">
            <span class="truncate max-w-[150px]">{{ name }}</span>
            <span class="text-[10px] font-mono shrink-0" :class="getDelayClass(delays[name])">
              {{ getDelayText(delays[name]) }}
            </span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
