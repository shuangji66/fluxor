<script setup lang="ts">
import { ref, onMounted, onUnmounted, onActivated, onDeactivated, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { GlobeOutline, SyncOutline } from '@vicons/ionicons5'
import { storeToRefs } from 'pinia'
import { useProxyStore } from '../store/proxies'
import { useGlobalStore } from '../store/global'
import ProxyGroupCard from '../components/ProxyGroupCard.vue'

const { t } = useI18n()
const proxyStore = useProxyStore()
const globalStore = useGlobalStore()
const { proxyGroups, delays, isLoading } = storeToRefs(proxyStore)

// 将代理组拆分为左右两列，保证左右高度平衡且横向交替排列
const leftGroups = computed(() => proxyGroups.value.filter((_, index) => index % 2 === 0))
const rightGroups = computed(() => proxyGroups.value.filter((_, index) => index % 2 !== 0))

const isTestingAll = ref(false)
let refreshTimer: number | null = null

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
    globalStore.showToast(t('common.operation_failed'), 'error')
  } finally {
    isTestingAll.value = false
  }
}

// 自动测速（后台静默）
const autoTestMissingNodes = async () => {
  const allNodes = new Set<string>()
  proxyGroups.value.forEach(g => {
    if (['Selector', 'URLTest', 'Fallback'].includes(g.type)) {
      g.all.forEach(name => allNodes.add(name))
    }
  })

  const needTest: string[] = []
  allNodes.forEach(name => {
    if (delays.value[name] === undefined) {
      needTest.push(name)
    }
  })

  if (needTest.length === 0) return
  await proxyStore.testProxiesWithConcurrency(needTest)
  proxyStore.fetchProxies(true)
}

// 定时器管理
const startRefreshTimer = () => {
  if (refreshTimer) clearInterval(refreshTimer)
  refreshTimer = window.setInterval(() => {
    proxyStore.fetchProxies(true)
  }, 10000)
}

const stopRefreshTimer = () => {
  if (refreshTimer) {
    clearInterval(refreshTimer)
    refreshTimer = null
  }
}

onMounted(async () => {
  const hasData = proxyGroups.value.length > 0
  await proxyStore.fetchProxies(hasData)
  autoTestMissingNodes().catch(e => console.warn('[Proxies] 自动测速失败:', e))
})

onActivated(async () => {
  // 激活时立即静默刷新，拉取最新节点
  await proxyStore.fetchProxies(true)
  autoTestMissingNodes().catch(e => console.warn('[Proxies] 自动测速失败:', e))
  startRefreshTimer()
})

onDeactivated(() => {
  stopRefreshTimer()
})

onUnmounted(() => {
  stopRefreshTimer()
})
</script>

<template>
  <div class="space-y-6">
    <div class="sticky top-0 z-20 glass-medium shadow-sm p-4 rounded-xl border flex items-center justify-between transition-all">
      <h3 class="text-base font-semibold flex items-center gap-2">
        <GlobeOutline class="w-5 h-5 text-accent" />
        {{ t('proxies.title') }}
      </h3>

      <button @click="handleTestAll" :disabled="isTestingAll" class="px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all flex items-center gap-1.5">
        <SyncOutline class="w-3.5 h-3.5" :class="{ 'animate-spin': isTestingAll }" />
        {{ isTestingAll ? t('proxies.testing') : t('proxies.test_all') }}
      </button>
    </div>

    <!-- 骨架屏：首屏加载且无缓存时渲染 -->
    <div v-if="isLoading && proxyGroups.length === 0" class="flex flex-col lg:flex-row gap-6 items-start">
      <!-- 骨架屏左列 -->
      <div class="flex-1 space-y-6 w-full min-w-0">
        <div v-for="i in 2" :key="'skeleton-l-' + i" class="bg-white dark:bg-[#1e293b] p-4 sm:p-5 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm space-y-4 animate-pulse select-none">
          <div class="flex items-center justify-between gap-4">
            <div class="flex items-center gap-2.5 min-w-0 flex-1">
              <div class="w-3.5 h-3.5 bg-slate-200 dark:bg-slate-800 rounded shrink-0"></div>
              <div class="flex-1 space-y-2">
                <div class="flex items-center gap-2">
                  <div class="h-4 bg-slate-200 dark:bg-slate-800 rounded w-24"></div>
                  <div class="h-4 bg-slate-200 dark:bg-slate-800 rounded w-16"></div>
                </div>
                <div class="h-3 bg-slate-200 dark:bg-slate-800 rounded w-32 mt-1"></div>
              </div>
            </div>
            <div class="w-8 h-8 bg-slate-200 dark:bg-slate-800 rounded-lg shrink-0"></div>
          </div>
          <div class="h-1.5 bg-slate-100 dark:bg-slate-800/50 rounded w-full"></div>
        </div>
      </div>
      <!-- 骨架屏右列 -->
      <div class="flex-1 space-y-6 w-full min-w-0">
        <div v-for="i in 2" :key="'skeleton-r-' + i" class="bg-white dark:bg-[#1e293b] p-4 sm:p-5 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm space-y-4 animate-pulse select-none">
          <div class="flex items-center justify-between gap-4">
            <div class="flex items-center gap-2.5 min-w-0 flex-1">
              <div class="w-3.5 h-3.5 bg-slate-200 dark:bg-slate-800 rounded shrink-0"></div>
              <div class="flex-1 space-y-2">
                <div class="flex items-center gap-2">
                  <div class="h-4 bg-slate-200 dark:bg-slate-800 rounded w-28"></div>
                  <div class="h-4 bg-slate-200 dark:bg-slate-800 rounded w-14"></div>
                </div>
                <div class="h-3 bg-slate-200 dark:bg-slate-800 rounded w-36 mt-1"></div>
              </div>
            </div>
            <div class="w-8 h-8 bg-slate-200 dark:bg-slate-800 rounded-lg shrink-0"></div>
          </div>
          <div class="h-1.5 bg-slate-100 dark:bg-slate-800/50 rounded w-full"></div>
        </div>
      </div>
    </div>
    
    <div v-else-if="proxyGroups.length === 0" class="p-8 text-center text-slate-400 dark:text-slate-600 text-sm">
      {{ t('proxies.empty') }}
    </div>

    <!-- 响应式双列布局 -->
    <div v-else class="flex flex-col lg:flex-row gap-6 items-start">
      <!-- 左列 -->
      <div class="flex-1 space-y-6 w-full min-w-0">
        <ProxyGroupCard
          v-for="group in leftGroups"
          :key="group.name"
          :group="group"
        />
      </div>

      <!-- 右列 -->
      <div class="flex-1 space-y-6 w-full min-w-0">
        <ProxyGroupCard
          v-for="group in rightGroups"
          :key="group.name"
          :group="group"
        />
      </div>
    </div>
  </div>
</template>
