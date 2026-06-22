<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, onActivated, onDeactivated, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { GlobeOutline, SyncOutline } from '@vicons/ionicons5'
import { storeToRefs } from 'pinia'
import { useProxyStore } from '../store/proxies'
import { useGlobalStore } from '../store/global'
import { useConfigStore } from '../store/config'
import ProxyGroupCard from '../components/ProxyGroupCard.vue'
import { apiFetch } from '../utils/api'

const { t } = useI18n()
const proxyStore = useProxyStore()
const globalStore = useGlobalStore()
const configStore = useConfigStore()

// 从 store 中解构所需状态
const { proxyGroups, delays, isLoading } = storeToRefs(proxyStore)
const { configs, coreStatus, currentConfig } = storeToRefs(configStore)

// 延迟更新实际渲染列表的模式，避开滑块动画的高频帧，确保视觉过渡丝滑
const renderedMode = ref(configs.value.mode || 'Rule')
let modeSwitchTimer: number | null = null

// 监听 configs.value.mode 的变化，延迟 180ms 更新渲染
watch(
  () => configs.value.mode,
  (newMode) => {
    if (modeSwitchTimer) {
      clearTimeout(modeSwitchTimer)
      modeSwitchTimer = null
    }
    if (renderedMode.value === newMode) return

    modeSwitchTimer = window.setTimeout(() => {
      renderedMode.value = newMode
      modeSwitchTimer = null
    }, 180)
  },
  { immediate: true }
)

// 根据内核模式与订阅模式过滤代理组
const filteredGroups = computed(() => {
  const mode = renderedMode.value
  const subMode = currentConfig.value.mode // 'merge' 或 'switch'

  if (mode === 'Direct') return []

  if (mode === 'Global') {
    // 融合订阅下，全局模式展示所有代理组
    if (subMode === 'merge') {
      return proxyGroups.value
    }
    // 切换订阅下，只展示 GLOBAL 组
    return proxyGroups.value.filter(g => g.name.toUpperCase() === 'GLOBAL')
  }

  // 规则模式：隐藏 GLOBAL 组
  return proxyGroups.value.filter(g => g.name.toUpperCase() !== 'GLOBAL')
})

// 将过滤后的组拆分为左右两列（仅当组数 > 6 时使用）
const leftGroups = computed(() => filteredGroups.value.filter((_, index) => index % 2 === 0))
const rightGroups = computed(() => filteredGroups.value.filter((_, index) => index % 2 !== 0))

// 是否使用单列布局（组数 <= 6）
const isSingleColumn = computed(() => filteredGroups.value.length <= 6)

const isTestingAll = ref(false)
let refreshTimer: number | null = null

// 切换运行模式（乐观更新及失败回滚）
const changeMode = async (mode: string) => {
  if (!coreStatus.value.running) {
    globalStore.showToast(t('config.core_not_running'), 'warning')
    return
  }
  if (mode === configs.value.mode) return

  const originalMode = configs.value.mode
  configs.value.mode = mode // 乐观更新，滑块立即响应

  try {
    const resp = await apiFetch('/configs', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode })
    })
    if (resp.ok) {
      globalStore.showToast(t('config.mode_switched'), 'success')
    } else {
      configs.value.mode = originalMode // 失败回滚
      globalStore.showToast(t('common.operation_failed'), 'error')
    }
  } catch (e) {
    configs.value.mode = originalMode // 异常回滚
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
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
  if (modeSwitchTimer) {
    clearTimeout(modeSwitchTimer)
    modeSwitchTimer = null
  }
}

onMounted(async () => {
  const hasData = proxyGroups.value.length > 0
  await proxyStore.fetchProxies(hasData)
  autoTestMissingNodes().catch(e => console.warn('[Proxies] 自动测速失败:', e))
})

onActivated(async () => {
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
    <!-- 顶部栏 -->
    <div class="sticky top-0 z-20 glass-medium shadow-sm p-4 rounded-xl border transition-all">
      <!-- 移动端：flex-col，桌面端：flex-row -->
      <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-3">
        <!-- 第一行：标题 + 全部测速（移动端显示） -->
        <div class="flex items-center justify-between w-full md:w-auto">
          <h3 class="text-base font-semibold flex items-center gap-2">
            <GlobeOutline class="w-5 h-5 text-accent" />
            {{ t('proxies.title') }}
          </h3>
          <!-- 移动端全部测速按钮 -->
          <button
            @click="handleTestAll"
            :disabled="isTestingAll"
            class="md:hidden px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <SyncOutline class="w-3.5 h-3.5" :class="{ 'animate-spin': isTestingAll }" />
            {{ isTestingAll ? t('proxies.testing') : t('proxies.test_all') }}
          </button>
        </div>

        <!-- 第二行：模式切换滑块（移动端占满，桌面端正常） -->
        <div class="flex items-center w-full md:w-auto">
          <div class="flex bg-slate-100 dark:bg-slate-800 rounded-lg p-0.5 w-full md:w-auto transition-all">
            <button
              v-for="modeOption in ['Rule', 'Global', 'Direct']"
              :key="modeOption"
              @click="changeMode(modeOption)"
              :disabled="!coreStatus.running"
              class="flex-1 md:flex-none px-4 py-1.5 text-xs font-semibold rounded-md transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
              :class="configs.mode === modeOption ? 'bg-accent text-white shadow-sm' : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'"
            >
              {{ t(`config.mode_${modeOption.toLowerCase()}`) }}
            </button>
          </div>
        </div>

        <!-- 桌面端全部测速按钮 -->
        <button
          @click="handleTestAll"
          :disabled="isTestingAll"
          class="hidden md:flex px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <SyncOutline class="w-3.5 h-3.5" :class="{ 'animate-spin': isTestingAll }" />
          {{ isTestingAll ? t('proxies.testing') : t('proxies.test_all') }}
        </button>
      </div>
    </div>

    <!-- 骨架屏 -->
    <div v-if="isLoading && proxyGroups.length === 0" class="flex flex-col lg:flex-row gap-6 items-start">
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

    <!-- 空状态 -->
    <div v-else-if="filteredGroups.length === 0" class="p-8 text-center text-slate-400 dark:text-slate-600 text-sm">
      <span v-if="configs.mode === 'Direct'">{{ t('proxies.empty_direct') }}</span>
      <span v-else-if="configs.mode === 'Global'">{{ t('proxies.empty_global') }}</span>
      <span v-else>{{ t('proxies.empty') }}</span>
    </div>

    <!-- 代理组列表（单列或双列） -->
    <div v-else>
      <!-- 单列模式（组数 <= 6） -->
      <div v-if="isSingleColumn" class="space-y-6 w-full">
        <ProxyGroupCard
          v-for="group in filteredGroups"
          :key="group.name"
          :group="group"
        />
      </div>

      <!-- 双列模式（组数 > 6） -->
      <div v-else class="flex flex-col lg:flex-row gap-6 items-start">
        <div class="flex-1 space-y-6 w-full min-w-0">
          <ProxyGroupCard
            v-for="group in leftGroups"
            :key="group.name"
            :group="group"
          />
        </div>
        <div class="flex-1 space-y-6 w-full min-w-0">
          <ProxyGroupCard
            v-for="group in rightGroups"
            :key="group.name"
            :group="group"
          />
        </div>
      </div>
    </div>
  </div>
</template>