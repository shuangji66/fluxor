<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, onActivated, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { GlobeOutline, SyncOutline, SettingsOutline } from '@vicons/ionicons5'
import { storeToRefs } from 'pinia'
import { useProxyStore, DEFAULT_DELAY_TEST_URL, normalizeDelayTestUrl } from '../store/proxies'
import { useGlobalStore } from '../store/global'
import { useConfigStore } from '../store/config'
import { useSubscriptionStore } from '../store/subscription'
import ProxyGroupCard from '../components/ProxyGroupCard.vue'
import PageHeader from '../components/PageHeader.vue'
import AppModal from '../components/AppModal.vue'
import { mapWithConcurrency } from '../utils/concurrency'
import { apiFetch, readErrorMessage } from '../utils/api'
import { modeI18nKey } from '../utils/i18n-keys'
import { useViewActive } from '../composables/useViewActive'

const { t } = useI18n()
const proxyStore = useProxyStore()
const globalStore = useGlobalStore()
const configStore = useConfigStore()
const subscriptionStore = useSubscriptionStore()

const { proxyGroups, isLoading, sortOrder, delayThresholds, historyCount, filterRegex } = storeToRefs(proxyStore)
const { setSortOrder, updateSettings, setFilterRegex } = proxyStore
const { coreStatus, configs } = storeToRefs(configStore)
const { currentConfig } = storeToRefs(subscriptionStore)

// ===== 设置弹窗 =====
const showSettingsDialog = ref(false)
const isActive = useViewActive()

// 测速地址预设：都是会快速返回 204/2xx 的轻量端点。
// 之所以给预设而不是只给一个空输入框：这个地址填错的代价是「所有节点看起来都超时」，
// 大多数用户并不知道该填什么；预设覆盖常见场景，自定义留给有特殊需求的人。
const TEST_URL_CUSTOM = '__custom__'
const TEST_URL_PRESETS = [
  { value: DEFAULT_DELAY_TEST_URL, labelKey: 'proxies.test_url_preset_gstatic' },
  { value: 'https://cp.cloudflare.com/generate_204', labelKey: 'proxies.test_url_preset_cloudflare' },
  { value: 'https://captive.apple.com/hotspot-detect.html', labelKey: 'proxies.test_url_preset_apple' },
]

const settingsForm = ref({
  sort: 'default' as 'default' | 'name' | 'delay' | 'quality',
  thresholdLow: 200,
  thresholdMid: 500,
  historyCount: 5,
  filterRegex: '',
  autoCloseConnections: true, 
  testUrl: DEFAULT_DELAY_TEST_URL,
  // 预设下拉的当前选项：命中预设就是该 URL，否则是「自定义」
  testUrlPreset: DEFAULT_DELAY_TEST_URL as string,
})

// 选中预设即写入对应地址；选「自定义」只切换输入框可见性，保留当前值供用户改写
const onTestUrlPresetChange = () => {
  const preset = TEST_URL_PRESETS.find(p => p.value === settingsForm.value.testUrlPreset)
  if (preset) settingsForm.value.testUrl = preset.value
}

// 打开弹窗时禁止 body 滚动。
// 必须同时考虑视图激活态：KeepAlive 停用本视图时 onUnmounted 不会触发，
// 若只看 showSettingsDialog，弹窗关闭前切页会把 overflow-hidden 永久留在 body 上。
const applyBodyScrollLock = () => {
  document.body.classList.toggle('overflow-hidden', showSettingsDialog.value && isActive.value)
}
watch([showSettingsDialog, isActive], applyBodyScrollLock)
onUnmounted(() => document.body.classList.remove('overflow-hidden'))

const openSettingsDialog = () => {
  const currentTestUrl = proxyStore.testUrl
  settingsForm.value = {
    sort: sortOrder.value,
    thresholdLow: delayThresholds.value.low,
    thresholdMid: delayThresholds.value.mid,
    historyCount: historyCount.value,
    filterRegex: filterRegex.value,
    autoCloseConnections: proxyStore.autoCloseConnections, 
    testUrl: currentTestUrl,
    testUrlPreset: TEST_URL_PRESETS.some(p => p.value === currentTestUrl) ? currentTestUrl : TEST_URL_CUSTOM,
  }
  showSettingsDialog.value = true
}

const saveSettings = async () => {
  // 先校验测速地址：非法时一处设置都不落，弹窗保持打开让用户直接改。
  // （若先保存其余设置再拒绝，用户看到「保存失败」但实际上别的项已经生效了。）
  const testUrl = normalizeDelayTestUrl(settingsForm.value.testUrl)
  if (!testUrl) {
    globalStore.showToast(t('proxies.test_url_invalid'), 'error')
    return
  }
  proxyStore.setTestUrl(testUrl)
  setSortOrder(settingsForm.value.sort)
  updateSettings(
    { low: settingsForm.value.thresholdLow, mid: settingsForm.value.thresholdMid },
    settingsForm.value.historyCount
  )
  setFilterRegex(settingsForm.value.filterRegex)
  proxyStore.setAutoCloseConnections(settingsForm.value.autoCloseConnections) 
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

//let statusTimer: any = null

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
      // 运行模式是内核配置项之一，配置页也显示它；同样登记待办让那一页切过去时补拉
      configStore.markCoreConfigStale()
    } else {
      configs.value.mode = originalMode
      globalStore.showToast(
        `${t('common.operation_failed')}: ${await readErrorMessage(resp)}`,
        'error'
      )
    }
  } catch (e) {
    configs.value.mode = originalMode
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  }
}

// 组间并发上限：内核的 /group/{name}/delay 会遍历组内全部节点，全量并发会把内核打满，
// 纯串行又太慢（组多时一轮要等十几秒）。4 路是折中值。
const TEST_ALL_CONCURRENCY = 4

const handleTestAll = async () => {
  if (isTestingAll.value) return
  isTestingAll.value = true
  globalStore.showToast(t('proxies.testing_all'), 'info')
  try {
    const groups = proxyGroups.value // 所有组
    // 统一取 Store 的测速地址：与单节点/单组测速共用同一设置，不再各自硬编码
    const testURL = proxyStore.getDelayTestUrl()
    const results = await mapWithConcurrency(groups, TEST_ALL_CONCURRENCY, async (group) => {
      const endpoint = `/group/${encodeURIComponent(group.name)}/delay?url=${encodeURIComponent(testURL)}&timeout=5000`
      const resp = await apiFetch(endpoint)
      if (!resp.ok) {
        console.warn(`组 ${group.name} 测速失败`)
        return false
      }
      // 逐组落地结果，测速过程中列表就在变色（整体刷新放到最后统一做）
      proxyStore.applyDelayMap(await resp.json())
      return true
    })
    // null = 该组任务抛错（如网络中断），必须计入失败，否则会误报「测速完成」
    const failedGroups = results.filter((ok) => !ok).length
    // 全程只刷新一次：旧实现每完成一组就整表重取，组多时列表会持续重排闪烁
    await proxyStore.fetchProxies(true)
    if (groups.length > 0 && failedGroups === groups.length) {
      globalStore.showToast(t('common.operation_failed'), 'error')
    } else if (failedGroups > 0) {
      globalStore.showToast(`${t('proxies.test_complete')} (${failedGroups})`, 'warning')
    } else {
      globalStore.showToast(t('proxies.test_complete'), 'success')
    }
  } catch (e) {
    globalStore.showToast(t('common.operation_failed'), 'error')
  } finally {
    isTestingAll.value = false
  }
}

onMounted(async () => {
  window.addEventListener('resize', onResize)
})

// 兜底：按质量排序依赖质量分数，而它是内存态（见 store/ensureQualityScores）。
// 启动阶段若内核尚未就绪，首次拉取会落空；本页每次被 KeepAlive 激活时补一次，
// 已有分数则不发请求，因此正常切页不会产生额外内核调用。
//
// 另一个来源是配置页对内核运行态的重建（重载 / 启动 / 重启 / 升级内核）留下的过期标记
// （见 store/proxies）：只有真正切到本页（KeepAlive 的 onActivated）才消费它并静默补拉
// 一次代理，用户不切过来就不产生任何请求。此处刻意不加「首次激活跳过」的守卫——
// 本页首次挂载时并不自己拉代理（启动那次由 App.vue 发起），若那次运行态重建发生在其之前，
// 快照确实已过期，必须补拉；此时多一次请求好过留下一份陈旧的代理列表。
onActivated(async () => {
  if (proxyStore.consumeNeedsRefresh()) {
    // 静默刷新：沿用已有快照，避免切页瞬间闪出加载态
    const ok = await proxyStore.fetchProxies(true)
    // 失败即把标记放回去：内核可能仍在启动/重启/升级中（此时请求拿不到数据，
    // store 也会保留旧快照）。放回标记让用户下次切入本页时自动重试，
    // 而不是把这份旧快照一直挂到刷新整个页面为止。
    if (!ok) proxyStore.markNeedsRefresh()
  }
  // 内核常规配置（本页的模式控件显示的就是内核的 Rule/Global/Direct）也可能被别处改掉：
  // 订阅中心「保存并应用」会重写 config.yaml 并重载内核，而模板把 mode 固定写成 rule，
  // 于是用户在这里选过的 Global/Direct 会被悄悄改回 rule —— 不补拉的话控件会一直高亮
  // 一个内核已经不用的模式，点击时还会因为 `mode === configs.mode` 而直接短路。
  if (configStore.consumeCoreConfigStale()) {
    configStore.fetchConfigs(true, true)
  }
  proxyStore.ensureQualityScores()
})

onUnmounted(() => {
  window.removeEventListener('resize', onResize)
})
</script>

<template>
  <div class="flex flex-col flex-1 min-h-0 gap-4 h-full">
    <!-- 顶部工具栏：移动两行，桌面一行 -->
    <PageHeader :icon="GlobeOutline" gap="gap-3 md:gap-4">
      <template #title>
        {{ t('proxies.title') }}
        <!-- 齿轮按钮：代理设置入口，紧贴标题右侧 -->
        <button
          @click="openSettingsDialog"
          class="-my-1 p-1.5 text-slate-500 hover:text-accent rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
          :title="t('proxies.settings_title')"
        >
          <SettingsOutline class="w-4 h-4" />
        </button>
      </template>

      <!-- 模式切换：桌面端居中，移动端独占一行 -->
      <div class="flex justify-center order-last w-full md:order-none md:w-auto md:flex-1">
        <div class="flex bg-slate-100 dark:bg-slate-800 rounded-lg p-0.5 w-full md:w-auto transition-all">
          <button
            v-for="modeOption in ['Rule', 'Global', 'Direct']"
            :key="modeOption"
            @click="changeMode(modeOption)"
            :disabled="!coreStatus.running"
            class="flex-1 md:flex-none px-4 md:px-6 py-1.5 text-xs font-semibold rounded-md transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
            :class="configs.mode === modeOption ? 'bg-accent text-white shadow-sm' : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'"
          >
            {{ t(modeI18nKey(modeOption)) }}
          </button>
        </div>
      </div>

      <template #actions>
        <button
          @click="handleTestAll"
          :disabled="isTestingAll"
          class="px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap"
        >
          <SyncOutline class="w-3.5 h-3.5" :class="{ 'animate-spin': isTestingAll }" />
          {{ isTestingAll ? t('proxies.testing') : t('proxies.test_all') }}
        </button>
      </template>
    </PageHeader>

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
    <AppModal
      :open="isActive && showSettingsDialog"
      :label="t('proxies.settings_title')"
      panel-class="w-full max-w-[92vw] sm:max-w-sm rounded-[20px] p-6 flex flex-col gap-4 max-h-[90vh]"
      @close="showSettingsDialog = false"
    >
      <h4 class="text-lg font-bold text-slate-800 dark:text-slate-100">{{ t('proxies.settings_title') }}</h4>

      <!-- 表单区：只有这里滚动，底部操作按钮始终留在视口内。
           pr-2 给滚动条留位：覆盖式滚动条会压住输入框/开关的右边缘；
           pb-2 让最后一项（正则输入框）的底边不贴着滚动区边界被裁掉。 -->
      <div class="flex-1 min-h-0 overflow-y-auto flex flex-col gap-4 pr-2 pb-2">

        <!-- 新增：自动断开连接开关 -->
        <div class="flex items-center justify-between py-1.5">
          <label class="text-xs font-semibold text-slate-600 dark:text-slate-400 select-none cursor-pointer">
            {{ t('proxies.auto_close_connections') }}
          </label>
          <div class="relative inline-flex items-center cursor-pointer" @click="settingsForm.autoCloseConnections = !settingsForm.autoCloseConnections">
            <div class="w-9 h-5 rounded-full transition-colors duration-200 ease-in-out"
                 :class="settingsForm.autoCloseConnections ? 'bg-accent' : 'bg-slate-300 dark:bg-slate-600'"></div>
            <div class="absolute left-0.5 top-0.5 w-4 h-4 rounded-full bg-white shadow transform transition-transform duration-200 ease-in-out"
                 :class="settingsForm.autoCloseConnections ? 'translate-x-4' : 'translate-x-0'"></div>
          </div>
        </div>

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

        <!-- 测速地址：预设下拉 + 自定义输入。
             单节点 / 单组 / 全部测速共用这一个地址（见 store/proxies.ts）。 -->
        <div class="flex flex-col gap-1.5">
          <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('proxies.test_url') }}</label>
          <select
            v-model="settingsForm.testUrlPreset"
            @change="onTestUrlPresetChange"
            class="w-full bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg px-3.5 py-2 text-sm focus:ring-2 focus:ring-accent outline-none"
          >
            <option v-for="preset in TEST_URL_PRESETS" :key="preset.value" :value="preset.value">
              {{ t(preset.labelKey) }}
            </option>
            <option :value="TEST_URL_CUSTOM">{{ t('proxies.test_url_custom') }}</option>
          </select>
          <input
            v-if="settingsForm.testUrlPreset === TEST_URL_CUSTOM"
            v-model="settingsForm.testUrl"
            type="text"
            spellcheck="false"
            autocapitalize="off"
            :placeholder="DEFAULT_DELAY_TEST_URL"
            class="w-full bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg px-3.5 py-2 text-sm font-mono focus:ring-2 focus:ring-accent outline-none"
          />
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
      </div>

      <div class="flex justify-end gap-2.5 pt-3 border-t border-slate-100 dark:border-slate-800/60 shrink-0">
        <button
          @click="showSettingsDialog = false"
          class="px-4 py-2 text-xs font-semibold rounded-xl bg-white border border-slate-200 hover:bg-slate-50 dark:bg-slate-800 dark:border-slate-700 dark:hover:bg-slate-700/60 text-slate-600 dark:text-slate-300 transition-all"
        >
          {{ t('common.cancel') }}
        </button>
        <button
          @click="saveSettings"
          class="px-4 py-2 text-xs font-semibold rounded-xl bg-accent hover:bg-accent-hover text-white transition-all shadow-md shadow-accent/15"
        >
          {{ t('common.save') }}
        </button>
      </div>
    </AppModal>

  </div>
</template>