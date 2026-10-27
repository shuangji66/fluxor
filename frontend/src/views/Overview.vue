<script setup lang="ts">
import { ref, onMounted, onUnmounted, onActivated, onDeactivated, nextTick, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiFetch, withBase } from '../utils/api'
import { formatBytes, formatSpeed } from '../utils/format'
import PageHeader from '../components/PageHeader.vue'
import TrafficChart from '../components/TrafficChart.vue'
import StatCard from '../components/StatCard.vue'
import { modeI18nKey } from '../utils/i18n-keys'
import { OpenOutline, SyncOutline, EyeOutline, EyeOffOutline, GridOutline, GlobeOutline, FlashOutline } from '@vicons/ionicons5'
import { storeToRefs } from 'pinia'
import { useOverviewStore, CORE_VERSION_LOADING, CORE_VERSION_UNKNOWN, NODE_NONE_SELECTED, NODE_CORE_STOPPED } from '../store/overview'
import { useConnectionsStore } from '../store/connections'
import { useGlobalStore } from '../store/global'
import { useConfigStore } from '../store/config'
import { useSubscriptionStore } from '../store/subscription'
import { useProxyStore } from '../store/proxies'

const { t } = useI18n()

const overviewStore = useOverviewStore()
const { stats, uiPanel, uploadHistory, downloadHistory, timeHistory } = storeToRefs(overviewStore)
const globalStore = useGlobalStore()
const configStore = useConfigStore()
const { configs } = storeToRefs(configStore)
const subscriptionStore = useSubscriptionStore()
const { currentConfig } = storeToRefs(subscriptionStore)
const proxyStore = useProxyStore()
const { proxyGroups } = storeToRefs(proxyStore)

const connectionsStore = useConnectionsStore()
const { connectionsCount, uploadTotal, downloadTotal } = storeToRefs(connectionsStore)

const coreVersionDisplay = computed(() => {
  if (stats.value.coreVersion === CORE_VERSION_LOADING) return t('common.loading')
  if (stats.value.coreVersion === CORE_VERSION_UNKNOWN) return t('common.unknown')
  return stats.value.coreVersion
})

// 哨兵 → 文案的统一映射：currentNode / currentGroup 共用同一套占位状态
const placeholderText = (value: string): string | null => {
  if (value === CORE_VERSION_LOADING) return t('common.loading')
  if (value === NODE_CORE_STOPPED) return t('config.core_stopped')
  if (value === NODE_NONE_SELECTED) return t('proxies.empty')
  return null
}

const currentNodeDisplay = computed(() => placeholderText(stats.value.currentNode) ?? stats.value.currentNode)

const currentGroupDisplay = computed(() => placeholderText(stats.value.currentGroup) ?? stats.value.currentGroup)

// 外置面板的入口地址由 withBase 统一拼前缀，不要自己拼 window.BASE_URL：
// BASE_URL 为 '/'（根路径部署，docs/faq.md 支持的部署方式）时会拼出 "//meta/"，
// 那是**协议相对 URL**，浏览器会解析成 https://meta/ —— 点了直接跳到不存在的主机。
// withBase 里专门处理了 BASE === '/' 与末尾斜杠，是唯一该用的地方。

// 流量数据点 (最多65个)
const maxPoints = 65

// 复制文本到剪贴板并提示
const copyText = (text: string | null | undefined, label: string) => {
  if (!text || text === '--' || text === '---' || text === CORE_VERSION_LOADING || text === CORE_VERSION_UNKNOWN) return
  navigator.clipboard.writeText(text).then(() => {
    globalStore.showToast(`${label} ${t('common.copied')}`, 'success')
  }).catch(() => {
    globalStore.showToast(t('common.operation_failed'), 'error')
  })
}

// 获取基础设置
const fetchSubscribeConfig = async () => {
  try {
    const resp = await apiFetch('/subscribe/config')
    if (resp.ok) {
      const cfg = await resp.json()
      uiPanel.value = cfg.ui_panel || 'metacubexd'
    }
  } catch (e) {
    console.warn('获取配置失败，使用默认面板', e)
  }
}

// ===== IP 信息 =====
interface IpInfo {
  localIPv4: string | null
  localIPv6: string | null
  proxyIPv4: string | null
  proxyIPv6: string | null
  localCountry: string | null
  localRegion: string | null
  localIsp: string | null
  proxyCountry: string | null
  proxyRegion: string | null
  proxyIsp: string | null
  proxyPort: number | null
  loading: boolean
  error: boolean
}

// 独立刷新加载状态
const loadingLocalV4 = ref(false)
const loadingLocalV6 = ref(false)
const loadingProxyV4 = ref(false)
const loadingProxyV6 = ref(false)

const ipInfo = ref<IpInfo>({
  localIPv4: null,
  localIPv6: null,
  proxyIPv4: null,
  proxyIPv6: null,
  localCountry: null,
  localRegion: null,
  localIsp: null,
  proxyCountry: null,
  proxyRegion: null,
  proxyIsp: null,
  proxyPort: null,
  loading: true,
  error: false
})

const showLocalGroup = ref(false)   // 本地组默认密文
const showProxyGroup = ref(true)    // 代理组默认明文

// 刷新本地组（IPv4 + IPv6）
const refreshLocalGroup = async () => {
  await Promise.all([
    refreshLocalIPv4(),
    refreshLocalIPv6()
  ])
}

// 刷新代理组（IPv4 + IPv6）
const refreshProxyGroup = async () => {
  await Promise.all([
    refreshProxyIPv4(),
    refreshProxyIPv6()
  ])
}

// 独立刷新本机 IPv4
const refreshLocalIPv4 = async () => {
  loadingLocalV4.value = true
  try {
    const resp = await apiFetch('/ipinfo/local/v4')
    if (resp.ok) {
      const data = await resp.json()
      ipInfo.value.localIPv4 = data.ip || null
      ipInfo.value.localCountry = data.country || null
      ipInfo.value.localRegion = data.region || null
      ipInfo.value.localIsp = data.isp || null
    }
  } catch (e) {
    console.warn('刷新本机 IPv4 失败:', e)
  } finally {
    loadingLocalV4.value = false
  }
}

// 独立刷新本机 IPv6
const refreshLocalIPv6 = async () => {
  loadingLocalV6.value = true
  try {
    const resp = await apiFetch('/ipinfo/local/v6')
    if (resp.ok) {
      const data = await resp.json()
      ipInfo.value.localIPv6 = data.ip || null
    }
  } catch (e) {
    console.warn('刷新本机 IPv6 失败:', e)
  } finally {
    loadingLocalV6.value = false
  }
}

// 独立刷新代理 IPv4
const refreshProxyIPv4 = async () => {
  loadingProxyV4.value = true
  try {
    const resp = await apiFetch('/ipinfo/proxy/v4')
    if (resp.ok) {
      const data = await resp.json()
      ipInfo.value.proxyIPv4 = data.ip || null
      ipInfo.value.proxyCountry = data.country || null
      ipInfo.value.proxyRegion = data.region || null
      ipInfo.value.proxyIsp = data.isp || null
    } else {
      ipInfo.value.proxyIPv4 = null
      ipInfo.value.proxyCountry = null
      ipInfo.value.proxyRegion = null
      ipInfo.value.proxyIsp = null
    }
  } catch (e) {
    console.warn('刷新代理 IPv4 失败:', e)
    ipInfo.value.proxyIPv4 = null
    ipInfo.value.proxyCountry = null
    ipInfo.value.proxyRegion = null
    ipInfo.value.proxyIsp = null
  } finally {
    loadingProxyV4.value = false
  }
}

// 独立刷新代理 IPv6
const refreshProxyIPv6 = async () => {
  loadingProxyV6.value = true
  try {
    const resp = await apiFetch('/ipinfo/proxy/v6')
    if (resp.ok) {
      const data = await resp.json()
      ipInfo.value.proxyIPv6 = data.ip || null
    } else {
      ipInfo.value.proxyIPv6 = null
    }
  } catch (e) {
    console.warn('刷新代理 IPv6 失败:', e)
    ipInfo.value.proxyIPv6 = null
  } finally {
    loadingProxyV6.value = false
  }
}

// ===== 代理延迟测试 =====
interface DelayTestResult {
  name: string
  url: string
  delay: number | null
  loading: boolean
  tested: boolean
}

const delayTargets = [
  { name: 'Baidu', url: 'https://www.baidu.com' },
  { name: 'Bilibili', url: 'https://www.bilibili.com' },
  { name: 'Google', url: 'https://www.gstatic.com/generate_204' },
  { name: 'GitHub', url: 'https://github.com' },
  { name: 'YouTube', url: 'https://www.youtube.com' }
]

const delayApiMap: Record<string, string> = {
  'Baidu': '/delaytest/baidu',
  'Bilibili': '/delaytest/bilibili',
  'Google': '/delaytest/google',
  'GitHub': '/delaytest/github',
  'YouTube': '/delaytest/youtube'
}

const delayResults = ref<DelayTestResult[]>(
  delayTargets.map(t => ({ ...t, delay: null, loading: false, tested: false }))
)

const customUrl = ref(localStorage.getItem('fluxor-custom-delay-url') || '')
const customDelay = ref<number | null>(null)
const customLoading = ref(false)
const customTested = ref(false)

// 监听自定义延迟测试网址的变化并自动持久化
watch(customUrl, (newVal) => {
  localStorage.setItem('fluxor-custom-delay-url', newVal)
})

const isTestingDelay = ref(false)

// 测试单个目标的延迟（通过后端代理）
const testSingleDelay = async (index: number) => {
  const item = delayResults.value[index]
  if (item.loading) return
  item.loading = true
  item.delay = null

  try {
    const apiPath = delayApiMap[item.name]
    const fetchUrl = apiPath
      ? apiPath + '?timeout=5000'
      : `/delaytest/custom?url=${encodeURIComponent(item.url)}&timeout=5000`

    const resp = await apiFetch(fetchUrl)
    if (resp.ok) {
      const data = await resp.json()
      // 后端可能返回 delay 为 null 或数字
      if (data.delay !== undefined && data.delay !== null && data.delay > 0) {
        item.delay = data.delay
      } else {
        item.delay = null // 超时或错误
      }
    } else {
      item.delay = null
    }
  } catch (e) {
    console.warn('测试延迟失败:', e)
    item.delay = null
  } finally {
    item.loading = false
    item.tested = true
  }
}

// 自定义目标测试延迟
const testCustomDelay = async () => {
  if (!customUrl.value.trim()) return
  customLoading.value = true
  customDelay.value = null
  try {
    const resp = await apiFetch(`/delaytest/custom?url=${encodeURIComponent(customUrl.value)}&timeout=5000`)
    if (resp.ok) {
      const data = await resp.json()
      if (data.delay !== undefined && data.delay !== null && data.delay > 0) {
        customDelay.value = data.delay
      } else {
        customDelay.value = null
      }
    } else {
      customDelay.value = null
    }
  } catch (e) {
    console.warn('自定义测试失败:', e)
    customDelay.value = null
  } finally {
    customLoading.value = false
    customTested.value = true
  }
}

// 测试全部目标（包括固定项和自定义项）
const testAllDelays = async () => {
  if (isTestingDelay.value) return
  isTestingDelay.value = true

  const concurrency = 2
  const queue = delayResults.value.map((_, idx) => idx)
  const workers = Array.from({ length: Math.min(concurrency, queue.length) }, async () => {
    while (queue.length > 0) {
      const idx = queue.shift()
      if (idx !== undefined) {
        await testSingleDelay(idx)
      }
    }
  })
  await Promise.all(workers)

  // 固定测试完成后，测试自定义地址（如果有）
  if (customUrl.value.trim()) {
    await testCustomDelay()
  }

  isTestingDelay.value = false
}

// 封装自定义延迟测试项为一个虚拟的测试结果，以便复用徽章样式
const customResult = computed<DelayTestResult>(() => ({
  name: 'Custom',
  url: customUrl.value,
  delay: customDelay.value,
  loading: customLoading.value,
  tested: customTested.value
}))

// 获取延迟显示及对应的微徽章样式
const getDelayDisplay = (result: DelayTestResult) => {
  const baseBadge = 'px-2.5 py-0.5 rounded-full text-xs font-bold font-mono tracking-wide shadow-sm border transition-all duration-300'
  if (result.loading) {
    return {
      text: t('proxies.testing'),
      class: `${baseBadge} bg-slate-500/10 text-slate-500 border-slate-500/15 dark:bg-slate-800/60 dark:text-slate-400 dark:border-slate-700/50`
    }
  }
  if (!result.tested) {
    return {
      text: '---',
      class: `${baseBadge} bg-slate-500/5 text-slate-400 border-slate-200/50 dark:bg-slate-800/30 dark:text-slate-500 dark:border-slate-800/40`
    }
  }
  if (result.delay === null) {
    return {
      text: t('proxies.timeout'),
      class: `${baseBadge} bg-rose-500/10 text-rose-600 border-rose-500/20 dark:bg-rose-950/20 dark:text-rose-400 dark:border-rose-900/30`
    }
  }
  if (result.delay < 200) {
    return {
      text: `${result.delay}ms`,
      class: `${baseBadge} bg-emerald-500/10 text-emerald-600 border-emerald-500/20 dark:bg-emerald-950/20 dark:text-emerald-400 dark:border-emerald-900/30`
    }
  }
  if (result.delay < 500) {
    return {
      text: `${result.delay}ms`,
      class: `${baseBadge} bg-amber-500/10 text-amber-600 border-amber-500/20 dark:bg-amber-950/20 dark:text-amber-400 dark:border-amber-900/30`
    }
  }
  return {
    text: `${result.delay}ms`,
    class: `${baseBadge} bg-rose-500/10 text-rose-600 border-rose-500/20 dark:bg-rose-950/20 dark:text-rose-400 dark:border-rose-900/30`
  }
}

// 格式化当前订阅名称（融合 / 自定义模式没有「当前订阅」，显示模式名）
const currentSubscriptionDisplay = computed(() => {
  if (currentConfig.value.mode === 'merge') {
    return t('subscription.mode_merge') || 'Merge'
  }
  if (currentConfig.value.mode === 'custom') {
    return t('subscription.mode_custom') || 'Custom'
  }
  return currentConfig.value.active_subscription || '--'
})

// 监听代理组变化，同步更新节点信息
watch(
  () => proxyGroups.value,
  () => {
    overviewStore.syncCurrentNodeFromProxyStore()
  },
  { deep: true, immediate: true }  // immediate 确保挂载时立即同步
)

onMounted(() => {
  nextTick(() => {
    fetchSubscribeConfig()
    // 先以 0 预填满窗口，再初始化画布：首屏即为固定骨架 + 可见基线，
    // 曲线自左向右延伸，而不是等数据攒够后从右缘「长」出来。
    overviewStore.primeHistory(maxPoints)
    refreshLocalGroup()
    refreshProxyGroup()
  })
})

// 数据订阅跟随**视图激活态**，而不是挂载态。
//
// 本视图在 <KeepAlive :max="7"> 内：切到别的标签页只触发 onDeactivated，onUnmounted
// 要等缓存被淘汰或应用销毁才触发。把订阅写成 onMounted/onUnmounted 的后果是——只要
// 用户进过一次概览页，流量、内存、连接三路 WebSocket 就在**整个会话**里一直开着：
// 每次流量事件（秒级）都会触发历史入队与 Canvas 重绘，用户在别的页面上也在持续付出
// CPU 与电量（移动端尤其明显）。store 侧本来就有引用计数与 3s 去抖，天然适配这种
// 「页面级开关」；首次挂载时 onActivated 紧跟 onMounted 触发，因此不必在两处都订阅。
onActivated(() => {
  overviewStore.subscribeStatus()
  overviewStore.subscribeTraffic()
  overviewStore.subscribeMemory()
  connectionsStore.subscribe()

  // 本页也读内核常规配置（「代理模式」那一栏显示的就是 configs.mode），而它会被
  // 别处改掉：订阅中心「保存并应用」会重写 config.yaml 并重载内核，模板把 mode 固定
  // 写成 rule，于是用户选过的 Global/Direct 会被改回 rule。标记由三个页面共用，
  // 谁先激活谁补拉，另外两页读的是同一份 store 状态，也就一起新鲜了。
  if (configStore.consumeCoreConfigStale()) {
    configStore.fetchConfigs(true, true)
  }
})

onDeactivated(() => {
  overviewStore.unsubscribeStatus()
  overviewStore.unsubscribeTraffic()
  overviewStore.unsubscribeMemory()
  connectionsStore.unsubscribe()
})

onUnmounted(() => {
  // 应用直接销毁（没有先触发 onDeactivated）时也要释放订阅；store 的退订是引用计数
  // 且可重复调用，多调一次是安全的
  overviewStore.unsubscribeStatus()
  overviewStore.unsubscribeTraffic()
  overviewStore.unsubscribeMemory()
  connectionsStore.unsubscribe()
})
</script>

<template>
  <div class="flex flex-col flex-1 min-h-0 gap-4 h-full">
    <!-- 顶部独立悬浮操作栏 -->
    <PageHeader :title="t('nav.overview')" :icon="GridOutline" />

    <!-- 内容区域内滚动容器 (已升级为统一大内容卡片) -->
    <div
      class="flex-1 min-h-0 overflow-y-auto glass-medium shadow-none rounded-xl border border-slate-200/50 dark:border-slate-800/50 p-6 space-y-6 pr-4">
      <!-- 8 个统计卡片 -->
      <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
        <StatCard
          :label="t('overview.upload_speed')"
          :value="formatSpeed(stats.uploadSpeed)"
          value-class="text-blue-500"
        />
        <StatCard
          :label="t('overview.download_speed')"
          :value="formatSpeed(stats.downloadSpeed)"
          value-class="text-success"
        />
        <StatCard :label="t('overview.upload_total')" :value="formatBytes(uploadTotal)" />
        <StatCard :label="t('overview.download_total')" :value="formatBytes(downloadTotal)" />
        <StatCard
          :label="t('overview.memory_usage')"
          :value="stats.memory > 0 ? formatBytes(stats.memory) : 'N/A'"
        />
        <StatCard
          :label="t('overview.active_connections')"
          :value="connectionsCount"
          clickable
          @click="globalStore.activeTab = 'connections'"
        />
        <StatCard
          :label="t('overview.core_version')"
          :value="coreVersionDisplay"
          value-class="truncate"
          :title="coreVersionDisplay"
        />
        <div
          class="bg-slate-50/50 dark:bg-slate-900/30 p-4 rounded-xl border border-slate-200/40 dark:border-slate-800/40 transition-all">
          <a :href="withBase(uiPanel === 'zashboard' ? '/zash/' : '/meta/')" target="_blank"
            class="block text-slate-800 dark:text-slate-100 decoration-transparent">
            <div class="flex justify-between items-center">
              <span class="text-xs sm:text-sm font-bold text-slate-500 dark:text-slate-400">{{
                t('overview.external_control') }}</span>
              <OpenOutline class="w-3.5 h-3.5 text-slate-400" />
            </div>
            <div class="text-base sm:text-lg font-extrabold text-accent mt-1 select-none">{{ uiPanel === 'zashboard' ?
              'Zashboard' : 'MetaCubeXD' }}</div>
          </a>
        </div>
      </div>

      <!-- 当前节点与流量趋势图左右布局 -->
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <!-- 当前节点卡片 -->
        <div
          class="lg:col-span-1 bg-slate-50/50 dark:bg-slate-900/30 p-6 rounded-xl border border-slate-200/40 dark:border-slate-800/40 transition-all flex flex-col h-full min-h-[160px] lg:min-h-0 relative overflow-hidden">
          <!-- 背景装饰水印 -->
          <GlobeOutline
            class="absolute right-[-14px] bottom-[-14px] w-32 h-32 text-slate-400/[0.04] dark:text-slate-500/[0.04] pointer-events-none rotate-12 z-0" />

          <div class="z-10 relative flex-1 flex flex-col">
            <!-- 标题 -->
            <h4 class="text-sm font-bold text-slate-500 dark:text-slate-400 mb-3">{{ t('overview.proxy_info') }}</h4>

            <div class="flex-1 flex flex-col justify-center gap-3">
              <!-- 订阅 -->
              <div class="glass-light border !rounded-lg px-3.5 py-2.5 flex justify-between items-center">
                <span class="text-xs font-bold text-slate-500 dark:text-slate-400 shrink-0">{{
                  t('overview.subscription') }}</span>
                <div class="flex-1 min-w-0 ml-4 flex justify-end">
                  <span
                    class="text-xs font-semibold text-slate-800 dark:text-slate-100 overflow-x-auto whitespace-nowrap text-right"
                    :title="currentSubscriptionDisplay">{{ currentSubscriptionDisplay }}</span>
                </div>
              </div>
              <!-- 代理模式 -->
              <div class="glass-light border !rounded-lg px-3.5 py-2.5 flex justify-between items-center">
                <span class="text-xs font-bold text-slate-500 dark:text-slate-400 shrink-0">{{ t('overview.proxy_mode') }}</span>
                <div class="flex-1 min-w-0 ml-4 flex justify-end">
                  <span class="text-xs font-semibold text-slate-800 dark:text-slate-100 overflow-x-auto whitespace-nowrap text-right">
                    {{ t(modeI18nKey(configs.mode)) }}
                  </span>
                </div>
              </div>
              <!-- 代理组 -->
              <div class="glass-light border !rounded-lg px-3.5 py-2.5 flex justify-between items-center">
                <span class="text-xs font-bold text-slate-500 dark:text-slate-400 shrink-0">{{ t('overview.proxy_group')
                  }}</span>
                <div class="flex-1 min-w-0 ml-4 flex justify-end">
                  <span
                    class="text-xs font-semibold text-slate-800 dark:text-slate-100 overflow-x-auto whitespace-nowrap text-right"
                    :title="currentGroupDisplay">{{ currentGroupDisplay }}</span>
                </div>
              </div>
              <!-- 当前节点 -->
              <div class="glass-light border !rounded-lg px-3.5 py-2.5 flex justify-between items-center">
                <span class="text-xs font-bold text-slate-500 dark:text-slate-400 shrink-0">{{
                  t('overview.current_node') }}</span>
                <div class="flex-1 min-w-0 ml-4 flex justify-end">
                  <span
                    class="text-xs font-semibold text-accent overflow-x-auto whitespace-nowrap text-right"
                    :title="currentNodeDisplay">{{ currentNodeDisplay }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 流量趋势图（绘制、量程、命中测试与 tooltip 都在组件内） -->
        <TrafficChart
          :upload-history="uploadHistory"
          :download-history="downloadHistory"
          :time-history="timeHistory"
          :max-points="maxPoints"
        />
      </div>

      <!-- 双列卡片：IP 信息 + 延迟测试 -->
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">

        <!-- IP 信息卡（分组设计，上下两部分紧凑排版） -->
        <div
          class="bg-slate-50/50 dark:bg-slate-900/30 p-5 rounded-xl border border-slate-200 dark:border-slate-800 transition-all flex flex-col h-full min-h-[340px] gap-4">
          <!-- 本地 IP 信息组 -->
          <div class="flex-1 flex flex-col min-h-0">
            <div class="flex items-center justify-between mb-2">
              <h4 class="text-sm font-bold text-slate-500 dark:text-slate-400 flex items-center">
                {{ t('overview.local_ip_info') }}
              </h4>
              <div class="flex items-center gap-1.5">
                <!-- 眼睛按钮 -->
                <button @click="showLocalGroup = !showLocalGroup"
                  class="p-1.5 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all"
                  :title="showLocalGroup ? t('common.hide') : t('common.show')">
                  <EyeOutline v-if="showLocalGroup"
                    class="w-4 h-4 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200" />
                  <EyeOffOutline v-else class="w-4 h-4 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200" />
                </button>
                <!-- 刷新按钮 -->
                <button @click="refreshLocalGroup" :disabled="loadingLocalV4 || loadingLocalV6"
                  class="p-1.5 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all disabled:opacity-50"
                  :title="t('common.refresh')">
                  <SyncOutline class="w-4 h-4 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200"
                    :class="{ 'animate-spin': loadingLocalV4 || loadingLocalV6 }" />
                </button>
              </div>
            </div>

            <!-- 本地 IP 列表，使用紧凑排列 -->
            <div class="flex flex-col gap-2.5 py-1">
              <!-- IPv4 行项 -->
              <div class="flex justify-between items-center">
                <span class="text-xs font-bold text-slate-500 dark:text-slate-400 flex-shrink-0">IPv4</span>
                <div
                  @click="copyText(ipInfo.localIPv4, t('overview.copy_label_local_ipv4'))"
                  class="px-2.5 py-0.5 rounded-lg bg-slate-100 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-800/50 flex-1 min-w-0 ml-4 max-w-max flex justify-end shadow-sm cursor-pointer hover:bg-slate-200 dark:hover:bg-slate-700/50 active:scale-95 transition-all"
                  :title="ipInfo.localIPv4 && ipInfo.localIPv4 !== '--' ? t('overview.click_copy_ipv4') : ''">
                  <span
                    class="font-bold text-xs text-slate-800 dark:text-slate-100 select-none overflow-x-auto whitespace-nowrap text-right">
                    {{ showLocalGroup ? (ipInfo.localIPv4 || '--') : (ipInfo.localIPv4 ? '••••••••' : '--') }}
                  </span>
                </div>
              </div>

              <!-- IPv6 行项 -->
              <div class="flex justify-between items-center">
                <span class="text-xs font-bold text-slate-500 dark:text-slate-400 flex-shrink-0">IPv6</span>
                <div
                  @click="copyText(ipInfo.localIPv6, t('overview.copy_label_local_ipv6'))"
                  class="px-2.5 py-0.5 rounded-lg bg-slate-100 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-800/50 flex-1 min-w-0 ml-4 max-w-max flex justify-end shadow-sm cursor-pointer hover:bg-slate-200 dark:hover:bg-slate-700/50 active:scale-95 transition-all"
                  :title="ipInfo.localIPv6 && ipInfo.localIPv6 !== '--' ? t('overview.click_copy_ipv6') : ''">
                  <span
                    class="font-bold text-xs text-slate-800 dark:text-slate-100 select-none overflow-x-auto whitespace-nowrap text-right">
                    {{ showLocalGroup ? (ipInfo.localIPv6 || '--') : (ipInfo.localIPv6 ? '••••••••' : '--') }}
                  </span>
                </div>
              </div>

              <!-- 地理信息 行项 -->
              <div class="flex justify-between items-center">
                <span class="text-xs font-bold text-slate-500 dark:text-slate-400 flex-shrink-0">{{
                  t('overview.geo_info') }}</span>
                <div
                  @click="copyText((ipInfo.localCountry || '') + ' / ' + (ipInfo.localRegion || '') + ' / ' + (ipInfo.localIsp || ''), t('overview.copy_label_local_geo'))"
                  class="px-2.5 py-0.5 rounded-lg bg-slate-100 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-800/50 flex-1 min-w-0 ml-4 max-w-max flex justify-end shadow-sm cursor-pointer hover:bg-slate-200 dark:hover:bg-slate-700/50 active:scale-95 transition-all"
                  :title="ipInfo.localCountry ? t('overview.click_copy_geo') : ''">
                  <span
                    class="font-bold text-xs text-slate-800 dark:text-slate-100 select-none overflow-x-auto whitespace-nowrap text-right">
                    {{ showLocalGroup ? ((ipInfo.localCountry || '---') + ' / ' + (ipInfo.localRegion || '---') + ' / '
                      + (ipInfo.localIsp || '---')) : '••••••••' }}
                  </span>
                </div>
              </div>
            </div>
          </div>

          <!-- 代理 IP 信息组 -->
          <div class="flex-1 flex flex-col min-h-0 border-t border-slate-200/50 dark:border-slate-800/40 pt-4">
            <div class="flex items-center justify-between mb-2">
              <h4 class="text-sm font-bold text-slate-500 dark:text-slate-400 flex items-center">
                {{ t('overview.proxy_ip_info') }}
              </h4>
              <div class="flex items-center gap-1.5">
                <!-- 眼睛按钮 -->
                <button @click="showProxyGroup = !showProxyGroup"
                  class="p-1.5 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all"
                  :title="showProxyGroup ? t('common.hide') : t('common.show')">
                  <EyeOutline v-if="showProxyGroup"
                    class="w-4 h-4 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200" />
                  <EyeOffOutline v-else class="w-4 h-4 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200" />
                </button>
                <!-- 刷新按钮 -->
                <button @click="refreshProxyGroup" :disabled="loadingProxyV4 || loadingProxyV6"
                  class="p-1.5 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all disabled:opacity-50"
                  :title="t('common.refresh')">
                  <SyncOutline class="w-4 h-4 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200"
                    :class="{ 'animate-spin': loadingProxyV4 || loadingProxyV6 }" />
                </button>
              </div>
            </div>

            <!-- 代理 IP 列表，使用紧凑排列 -->
            <div class="flex flex-col gap-2.5 py-1">
              <!-- 代理 IPv4 行项 -->
              <div class="flex justify-between items-center">
                <span class="text-xs font-bold text-slate-500 dark:text-slate-400 flex-shrink-0">IPv4</span>
                <div
                  @click="copyText(ipInfo.proxyIPv4, t('overview.copy_label_proxy_ipv4'))"
                  class="px-2.5 py-0.5 rounded-lg bg-slate-100 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-800/50 flex-1 min-w-0 ml-4 max-w-max flex justify-end shadow-sm cursor-pointer hover:bg-slate-200 dark:hover:bg-slate-700/50 active:scale-95 transition-all"
                  :title="ipInfo.proxyIPv4 && ipInfo.proxyIPv4 !== '--' ? t('overview.click_copy_ipv4') : ''">
                  <span
                    class="font-bold text-xs text-slate-800 dark:text-slate-100 select-none overflow-x-auto whitespace-nowrap text-right">
                    {{ showProxyGroup ? (ipInfo.proxyIPv4 || '--') : (ipInfo.proxyIPv4 ? '••••••••' : '--') }}
                  </span>
                </div>
              </div>

              <!-- 代理 IPv6 行项 -->
              <div class="flex justify-between items-center">
                <span class="text-xs font-bold text-slate-500 dark:text-slate-400 flex-shrink-0">IPv6</span>
                <div
                  @click="copyText(ipInfo.proxyIPv6, t('overview.copy_label_proxy_ipv6'))"
                  class="px-2.5 py-0.5 rounded-lg bg-slate-100 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-800/50 flex-1 min-w-0 ml-4 max-w-max flex justify-end shadow-sm cursor-pointer hover:bg-slate-200 dark:hover:bg-slate-700/50 active:scale-95 transition-all"
                  :title="ipInfo.proxyIPv6 && ipInfo.proxyIPv6 !== '--' ? t('overview.click_copy_ipv6') : ''">
                  <span
                    class="font-bold text-xs text-slate-800 dark:text-slate-100 select-none overflow-x-auto whitespace-nowrap text-right">
                    {{ showProxyGroup ? (ipInfo.proxyIPv6 || '--') : (ipInfo.proxyIPv6 ? '••••••••' : '--') }}
                  </span>
                </div>
              </div>

              <!-- 代理地理信息 行项 -->
              <div class="flex justify-between items-center">
                <span class="text-xs font-bold text-slate-500 dark:text-slate-400 flex-shrink-0">{{
                  t('overview.geo_info') }}</span>
                <div
                  @click="copyText((ipInfo.proxyCountry || '') + ' / ' + (ipInfo.proxyRegion || '') + ' / ' + (ipInfo.proxyIsp || ''), t('overview.copy_label_proxy_geo'))"
                  class="px-2.5 py-0.5 rounded-lg bg-slate-100 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-800/50 flex-1 min-w-0 ml-4 max-w-max flex justify-end shadow-sm cursor-pointer hover:bg-slate-200 dark:hover:bg-slate-700/50 active:scale-95 transition-all"
                  :title="ipInfo.proxyCountry ? t('overview.click_copy_proxy_geo') : ''">
                  <span
                    class="font-bold text-xs text-slate-800 dark:text-slate-100 select-none overflow-x-auto whitespace-nowrap text-right">
                    {{ showProxyGroup ? ((ipInfo.proxyCountry || '---') + ' / ' + (ipInfo.proxyRegion || '---') + ' / '
                      + (ipInfo.proxyIsp || '---')) : '••••••••' }}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 代理延迟测试卡 -->
        <div
          class="bg-slate-50/50 dark:bg-slate-900/30 p-5 rounded-xl border border-slate-200 dark:border-slate-800 transition-all flex flex-col justify-between">
          <div>
            <div class="flex items-center justify-between mb-4">
              <h4 class="text-sm font-bold text-slate-500 dark:text-slate-400 flex items-center">
                {{ t('overview.proxy_delay_test') }}
              </h4>
              <button @click="testAllDelays" :disabled="isTestingDelay"
                class="px-3.5 py-1.5 text-xs font-semibold rounded-lg bg-accent hover:bg-accent-hover text-white transition-all disabled:opacity-50 disabled:cursor-not-allowed shadow-sm active:scale-95">
                {{ isTestingDelay ? t('overview.testing') : t('overview.test_all') }}
              </button>
            </div>

            <div class="space-y-3">
              <!-- 自定义测试组件 -->
              <div class="flex items-center gap-3 px-3">
                <input v-model="customUrl" type="text" :placeholder="t('overview.custom_placeholder')"
                  class="flex-1 px-3 py-2 text-xs font-semibold text-slate-700 dark:text-slate-200 rounded-lg border border-slate-200 dark:border-slate-800/50 bg-slate-100 dark:bg-slate-800/40 focus:bg-transparent outline-none focus:border-accent focus:ring-1 focus:ring-accent/20 transition-all placeholder-slate-400/50"
                  @keyup.enter="testCustomDelay" />
                <div class="flex items-center gap-3 shrink-0">
                  <span :class="getDelayDisplay(customResult).class"
                    class="cursor-pointer hover:scale-[1.03] active:scale-95 animate-[fadeIn_0.2s]" @click="testCustomDelay">
                    {{ getDelayDisplay(customResult).text }}
                  </span>
                  <button @click="testCustomDelay" :disabled="customLoading || !customUrl.trim()"
                    class="w-7 h-7 rounded-lg bg-slate-50 hover:bg-slate-100 dark:bg-slate-800 dark:hover:bg-slate-700/80 flex items-center justify-center transition-all border border-slate-200/50 dark:border-slate-700/40 disabled:opacity-40 disabled:cursor-not-allowed shrink-0"
                    :title="t('overview.test_custom_title')">
                    <FlashOutline v-if="!customLoading" class="w-3.5 h-3.5 text-slate-500 dark:text-slate-400" />
                    <div v-else
                      class="w-3 h-3 border-2 border-slate-300 dark:border-slate-600 !border-t-accent rounded-full animate-spin">
                    </div>
                  </button>
                </div>
              </div>

              <!-- 虚线分水岭 -->
              <div class="border-t border-dashed border-slate-200/20 dark:border-slate-800/30 my-2.5"></div>

              <!-- 固定测试列表 -->
              <div class="space-y-1.5">
                <div v-for="(result, idx) in delayResults" :key="result.name"
                  class="flex items-center justify-between py-1.5 px-3 rounded-xl transition-all border border-transparent group">
                  <span class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ result.name }}</span>
                  <div class="flex items-center gap-3">
                    <span :class="getDelayDisplay(result).class"
                      class="cursor-pointer hover:scale-[1.03] active:scale-95" @click="testSingleDelay(idx)">
                      {{ getDelayDisplay(result).text }}
                    </span>
                    <button @click="testSingleDelay(idx)" :disabled="result.loading"
                      class="w-7 h-7 rounded-lg bg-slate-50 hover:bg-slate-100 dark:bg-slate-800 dark:hover:bg-slate-700/80 flex items-center justify-center transition-all border border-slate-200/50 dark:border-slate-700/40 disabled:opacity-40 shrink-0"
                      :title="t('overview.test_single_title', { name: result.name })">
                      <FlashOutline v-if="!result.loading"
                        class="w-3.5 h-3.5 text-slate-500 dark:text-slate-400 transition-transform duration-300 group-hover:scale-110" />
                      <div v-else
                        class="w-3 h-3 border-2 border-slate-300 dark:border-slate-600 !border-t-accent rounded-full animate-spin">
                      </div>
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>