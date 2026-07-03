<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, onActivated, onDeactivated, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { GlobeOutline, SyncOutline, SettingsOutline } from '@vicons/ionicons5'
import { storeToRefs } from 'pinia'
import { useProxyStore } from '../store/proxies'
import { useGlobalStore } from '../store/global'
import { useConfigStore } from '../store/config'
import { useSubscriptionStore } from '../store/subscription'
import ProxyGroupCard from '../components/ProxyGroupCard.vue'
import { apiFetch } from '../utils/api'

const { t } = useI18n()
const proxyStore = useProxyStore()
const globalStore = useGlobalStore()
const configStore = useConfigStore()
const subscriptionStore = useSubscriptionStore()

const { proxyGroups, delays, isLoading, sortOrder, delayThresholds, historyCount, filterRegex } = storeToRefs(proxyStore)
const { setSortOrder, updateSettings, fetchQualityScores, setFilterRegex } = proxyStore
const { coreStatus, configs } = storeToRefs(configStore)
const { currentConfig } = storeToRefs(subscriptionStore)

// ===== 设置弹窗 =====
const showSettingsDialog = ref(false)
const settingsForm = ref({
  sort: 'default' as 'default' | 'name' | 'delay' | 'quality',
  thresholdLow: 200,
  thresholdMid: 500,
  historyCount: 5,
  filterRegex: '',
})

// 打开弹窗时禁止 body 滚动
watch(showSettingsDialog, (val) => {
  if (val) {
    document.body.classList.add('overflow-hidden')
  } else {
    document.body.classList.remove('overflow-hidden')
  }
})

const openSettingsDialog = () => {
  settingsForm.value = {
    sort: sortOrder.value,
    thresholdLow: delayThresholds.value.low,
    thresholdMid: delayThresholds.value.mid,
    historyCount: historyCount.value,
    filterRegex: filterRegex.value,
  }
  showSettingsDialog.value = true
}

const saveSettings = async () => {
  setSortOrder(settingsForm.value.sort)
  updateSettings(
    { low: settingsForm.value.thresholdLow, mid: settingsForm.value.thresholdMid },
    settingsForm.value.historyCount
  )
  setFilterRegex(settingsForm.value.filterRegex)
  showSettingsDialog.value = false
  globalStore.showToast(t('proxies.settings_saved'), 'success')
}

// 桌面端检测
const isDesktop = ref(window.innerWidth >= 768)
const onResize = () => { isDesktop.value = window.innerWidth >= 768 }

const renderedMode = ref(configs.value.mode || 'Rule')
let modeSwitchTimer: number | null = null

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

const filteredGroups = computed(() => {
  const mode = renderedMode.value
  const subMode = currentConfig.value.mode

  // Direct 模式下没有组
  if (mode === 'Direct') return []

  // 先获取基于模式的候选组（不进行 hidden 过滤）
  let groups = proxyGroups.value

  if (mode === 'Global') {
    // 如果订阅模式是 merge，保留全部；否则只保留 GLOBAL 组
    if (subMode !== 'merge') {
      groups = groups.filter(g => g.name.toUpperCase() === 'GLOBAL')
    }
    // 否则 groups 保持全部
  } else {
    // Rule 模式：排除 GLOBAL 组
    groups = groups.filter(g => g.name.toUpperCase() !== 'GLOBAL')
  }

  // 最后统一过滤掉 hidden 为 true 的组
  return groups.filter(g => !g.hidden)
})

// 是否启用双列独立布局：组数 > 6 且桌面端
const showTwoColumns = computed(() => filteredGroups.value.length > 6 && isDesktop.value)

// 按奇偶拆分为两列（保持原顺序交替）
const leftColumn = computed(() => filteredGroups.value.filter((_, i) => i % 2 === 0))
const rightColumn = computed(() => filteredGroups.value.filter((_, i) => i % 2 === 1))

const isTestingAll = ref(false)
let refreshTimer: number | null = null
let statusTimer: any = null

const changeMode = async (mode: string) => {
  if (!coreStatus.value.running) {
    globalStore.showToast(t('config.core_not_running'), 'warning')
    return
  }
  if (mode === configs.value.mode) return
  const originalMode = configs.value.mode
  configs.value.mode = mode
  try {
    const resp = await apiFetch('/configs', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode })
    })
    if (resp.ok) {
      globalStore.showToast(t('config.mode_switched'), 'success')
    } else {
      configs.value.mode = originalMode
      globalStore.showToast(t('common.operation_failed'), 'error')
    }
  } catch (e) {
    configs.value.mode = originalMode
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  }
}

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
  window.addEventListener('resize', onResize)
  const hasData = proxyGroups.value.length > 0
  await proxyStore.fetchProxies(hasData)
})

onActivated(async () => {
  await proxyStore.fetchProxies(true)
  startRefreshTimer()
})

onDeactivated(() => {
  stopRefreshTimer()
})

onUnmounted(() => {
  window.removeEventListener('resize', onResize)
  stopRefreshTimer()
})
</script>

<template>
  <div class="flex flex-col flex-1 min-h-0 gap-4 h-full">
    <!-- 顶部工具栏：移动两行，桌面一行 -->
    <div class="glass-medium shadow-none px-6 py-3 rounded-xl border border-slate-200/50 dark:border-slate-800/50 transition-all shrink-0">
      <!-- 移动端布局（两行） -->
      <div class="flex md:hidden flex-col w-full gap-3">
        <div class="flex items-center justify-between">
          <h3 class="text-base font-semibold flex items-center gap-2">
            <GlobeOutline class="w-5 h-5 text-accent" />
            {{ t('proxies.title') }}
          </h3>
          <div class="flex items-center gap-2">
            <!-- 齿轮按钮 -->
            <button
              @click="openSettingsDialog"
              class="p-2 text-slate-500 hover:text-accent rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
              :title="t('proxies.settings_title')"
            >
              <SettingsOutline class="w-5 h-5" />
            </button>
            <!-- 全部测速按钮 -->
            <button
              @click="handleTestAll"
              :disabled="isTestingAll"
              class="px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap"
            >
              <SyncOutline class="w-3.5 h-3.5" :class="{ 'animate-spin': isTestingAll }" />
              {{ isTestingAll ? t('proxies.testing') : t('proxies.test_all') }}
            </button>
          </div>
        </div>
        <div class="flex justify-center">
          <div class="flex bg-slate-100 dark:bg-slate-800 rounded-lg p-0.5 w-full sm:w-auto transition-all">
            <button
              v-for="modeOption in ['Rule', 'Global', 'Direct']"
              :key="modeOption"
              @click="changeMode(modeOption)"
              :disabled="!coreStatus.running"
              class="flex-1 sm:flex-none px-4 py-1.5 text-xs font-semibold rounded-md transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
              :class="configs.mode === modeOption ? 'bg-accent text-white shadow-sm' : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'"
            >
              {{ t(`config.mode_${modeOption.toLowerCase()}`) }}
            </button>
          </div>
        </div>
      </div>

      <!-- 桌面端布局（一行） -->
      <div class="hidden md:flex items-center justify-between w-full">
        <h3 class="text-base font-semibold flex items-center gap-2">
          <GlobeOutline class="w-5 h-5 text-accent" />
          {{ t('proxies.title') }}
        </h3>
        <div class="flex-1 flex justify-center">
          <div class="flex bg-slate-100 dark:bg-slate-800 rounded-lg p-0.5 transition-all">
            <button
              v-for="modeOption in ['Rule', 'Global', 'Direct']"
              :key="modeOption"
              @click="changeMode(modeOption)"
              :disabled="!coreStatus.running"
              class="px-4 py-1.5 text-xs font-semibold rounded-md transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
              :class="configs.mode === modeOption ? 'bg-accent text-white shadow-sm' : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'"
            >
              {{ t(`config.mode_${modeOption.toLowerCase()}`) }}
            </button>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <button
            @click="openSettingsDialog"
            class="p-2 text-slate-500 hover:text-accent rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
            :title="t('proxies.settings_title')"
          >
            <SettingsOutline class="w-5 h-5" />
          </button>
          <button
            @click="handleTestAll"
            :disabled="isTestingAll"
            class="px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap"
          >
            <SyncOutline class="w-3.5 h-3.5" :class="{ 'animate-spin': isTestingAll }" />
            {{ isTestingAll ? t('proxies.testing') : t('proxies.test_all') }}
          </button>
        </div>
      </div>
    </div>

    <!-- 内容区域 -->
    <div class="flex-1 min-h-0 overflow-y-auto glass-medium shadow-none rounded-xl border border-slate-200/50 dark:border-slate-800/50 p-6 space-y-6 pr-4" style="scrollbar-gutter: stable">
      <!-- 骨架屏 -->
      <div v-if="isLoading && proxyGroups.length === 0" class="flex flex-col lg:flex-row gap-4 items-start">
        <div class="flex-1 space-y-4 w-full min-w-0">
          <div v-for="i in 2" :key="'skeleton-l-' + i" class="bg-slate-50/50 dark:bg-slate-900/30 p-4 sm:p-5 rounded-xl border border-slate-200/40 dark:border-slate-800/40 space-y-4 animate-pulse select-none">
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
        <div class="flex-1 space-y-4 w-full min-w-0">
          <div v-for="i in 2" :key="'skeleton-r-' + i" class="bg-slate-50/50 dark:bg-slate-900/30 p-4 sm:p-5 rounded-xl border border-slate-200/40 dark:border-slate-800/40 space-y-4 animate-pulse select-none">
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

      <!-- 代理组列表 -->
      <div v-else>
        <!-- 单列模式 -->
        <div v-if="!showTwoColumns" class="space-y-4 w-full">
          <ProxyGroupCard
            v-for="group in filteredGroups"
            :key="group.name"
            :group="group"
          />
        </div>

        <!-- 双列模式 -->
        <div v-else class="flex gap-4 items-start">
          <div class="w-1/2 space-y-4">
            <ProxyGroupCard
              v-for="group in leftColumn"
              :key="group.name"
              :group="group"
            />
          </div>
          <div class="w-1/2 space-y-4">
            <ProxyGroupCard
              v-for="group in rightColumn"
              :key="group.name"
              :group="group"
            />
          </div>
        </div>
      </div>
    </div>

    <!-- 设置弹窗 -->
    <Teleport to="body">
      <div
        v-if="showSettingsDialog"
        class="fixed inset-0 z-[9999] glass-mask flex items-center justify-center p-4"
        @click.self="showSettingsDialog = false"
      >
        <div class="glass-heavy w-full max-w-[92vw] sm:max-w-sm rounded-[20px] shadow-2xl border p-6 flex flex-col gap-4 animate-[zoomIn_0.15s_ease-out] max-h-[90vh] overflow-y-auto">
          <h4 class="text-lg font-bold text-slate-800 dark:text-slate-100">{{ t('proxies.settings_title') }}</h4>

          <!-- 排序 -->
          <div class="flex flex-col gap-1.5">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('proxies.sort_order') }}</label>
            <select
              v-model="settingsForm.sort"
              class="w-full bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg px-3.5 py-2 text-sm focus:ring-2 focus:ring-accent outline-none"
            >
              <option value="default">{{ t('proxies.sort_default') }}</option>
              <option value="name">{{ t('proxies.sort_name') }}</option>
              <option value="delay">{{ t('proxies.sort_delay') }}</option>
              <option value="quality">{{ t('proxies.sort_quality') }}</option>
            </select>
          </div>

          <!-- 低延迟阈值 -->
          <div class="flex flex-col gap-1.5">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('proxies.threshold_low') }}</label>
            <input
              type="number"
              v-model.number="settingsForm.thresholdLow"
              min="1"
              class="w-full bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg px-3.5 py-2 text-sm focus:ring-2 focus:ring-accent outline-none"
            />
          </div>

          <!-- 中延迟阈值 -->
          <div class="flex flex-col gap-1.5">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('proxies.threshold_mid') }}</label>
            <input
              type="number"
              v-model.number="settingsForm.thresholdMid"
              min="1"
              class="w-full bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg px-3.5 py-2 text-sm focus:ring-2 focus:ring-accent outline-none"
            />
          </div>

          <!-- 历史条数 -->
          <div class="flex flex-col gap-1.5">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('proxies.history_count') }}</label>
            <input
              type="number"
              v-model.number="settingsForm.historyCount"
              min="5"
              max="10"
              step="1"
              class="w-full bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg px-3.5 py-2 text-sm focus:ring-2 focus:ring-accent outline-none"
            />
          </div>

          <!-- 节点过滤正则 -->
          <div class="flex flex-col gap-1.5">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('proxies.filter_regex') }}</label>
            <textarea
              v-model="settingsForm.filterRegex"
              :placeholder="t('proxies.filter_regex_placeholder')"
              rows="2"
              class="w-full bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg px-3.5 py-2 text-sm focus:ring-2 focus:ring-accent outline-none font-mono resize-y min-h-[3.5rem]"
            ></textarea>
          </div>

          <div class="flex justify-end gap-2.5 pt-3 border-t border-slate-100 dark:border-slate-800/60">
            <button
              @click="showSettingsDialog = false"
              class="px-4 py-2 text-sm font-semibold rounded-xl bg-white border border-slate-200 hover:bg-slate-50 dark:bg-slate-800 dark:border-slate-700 dark:hover:bg-slate-700/60 text-slate-600 dark:text-slate-300 transition-all"
            >
              {{ t('common.cancel') }}
            </button>
            <button
              @click="saveSettings"
              class="px-4 py-2 text-sm font-semibold rounded-xl bg-accent hover:bg-accent-hover text-white transition-all shadow-md shadow-accent/15"
            >
              {{ t('common.save') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>