<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, onActivated } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiFetch } from '../utils/api'
import { CloudDownloadOutline, OptionsOutline, HardwareChipOutline, ShieldCheckmarkOutline, BuildOutline, SearchOutline, SyncOutline, ColorPaletteOutline, SettingsOutline, InformationCircleOutline, DocumentTextOutline } from '@vicons/ionicons5'
import { useGlobalStore } from '../store/global'
import { storeToRefs } from 'pinia'
import { useConfigStore, type ConfigData } from '../store/config'
import { useOverviewStore } from '../store/overview'
import FormSwitch from '../components/FormSwitch.vue'

const { t, locale } = useI18n()
const globalStore = useGlobalStore()
const configStore = useConfigStore()
const overviewStore = useOverviewStore()
const { configs, configsLoading, coreStatus } = storeToRefs(configStore)
const { stats } = storeToRefs(overviewStore)
const fetchConfigs = configStore.fetchConfigs

const coreVersion = computed(() => {
  if (stats.value.coreVersion === '加载中...') return t('common.loading')
  if (stats.value.coreVersion === '未知') return ''
  return 'v' + stats.value.coreVersion
})

const onTproxyPortClick = () => {
  if (configStore.tproxyEnabled) {
    globalStore.showToast(t('config.tproxy_port_readonly_warning'), 'warning')
  }
}

export interface CoreStatus {
  running: boolean
  loading: boolean
}

// DNS 查询测试
const dnsQuery = ref({
  name: '',
  type: 'A',
  result: '',
  loading: false
})

const isUpgrading = ref(false)
const isReloading = ref(false)
const isFlushingFakeIP = ref(false)
const isFlushingDNS = ref(false)
const isUpdatingGeo = ref(false)
const statusTimer = ref<any>(null)

const interfaces = ref<string[]>([])
const fetchInterfaces = async () => {
  try {
    const resp = await apiFetch('/interfaces')
    if (resp.ok) {
      interfaces.value = await resp.json()
    }
  } catch (e) {
    console.error('获取网卡列表失败:', e)
  }
}

// tproxy例外列表弹窗
const showTproxyExceptionsDialog = ref(false)

const tproxyDstExceptionsText = ref('')
const tproxySrcExceptionsText = ref('')
const tproxyProxyLocal = ref(false)

// 在 openTproxyExceptionsDialog 中同时获取例外列表和本机开关
const openTproxyExceptionsDialog = async () => {
  if (configStore.tproxyEnabled) {
    globalStore.showToast(t('config.tproxy_exceptions_disabled_message'), 'warning')
    return
  }
  try {
    const resp = await apiFetch('/config/tproxy/exceptions')
    if (resp.ok) {
      const data = await resp.json()
      tproxyDstExceptionsText.value = (data.dst || []).join('\n')
      // 直接使用后端数据，不额外填充默认值
      tproxySrcExceptionsText.value = (data.src || []).join('\n')
    }
    const resp2 = await apiFetch('/config/tproxy/proxy-local')
    if (resp2.ok) {
      const data2 = await resp2.json()
      tproxyProxyLocal.value = data2.enabled
    }
    showTproxyExceptionsDialog.value = true
  } catch (e) {
    globalStore.showToast(t('common.error'), 'error')
  }
}
// 修改保存函数：同时保存例外列表和本机代理开关（如果变化）
const saveTproxyExceptions = async () => {
  const dstLines = tproxyDstExceptionsText.value.split('\n').map(s => s.trim()).filter(s => s !== '')
  const srcLines = tproxySrcExceptionsText.value.split('\n').map(s => s.trim()).filter(s => s !== '')
  try {
    const resp = await apiFetch('/config/tproxy/exceptions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dst: dstLines, src: srcLines })
    })
    if (resp.ok) {
      // 保存本机代理开关
      await apiFetch('/config/tproxy/proxy-local', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: tproxyProxyLocal.value })
      })
      globalStore.showToast(t('config.tproxy_exceptions_saved'), 'success')
      showTproxyExceptionsDialog.value = false
    } else {
      globalStore.showToast(t('common.operation_failed'), 'error')
    }
  } catch (e) {
    globalStore.showToast(t('common.error'), 'error')
  }
}

// 统一修改配置
const patchConfig = async (payload: Partial<ConfigData>) => {
  try {
    const resp = await apiFetch('/configs', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (resp.ok) {
      fetchConfigs()
    } else {
      globalStore.showToast(t('common.operation_failed'), 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  }
}

const toggleAllowLan = () => {
  patchConfig({ 'allow-lan': configs.value['allow-lan'] })
}

const toggleIPv6 = () => {
  patchConfig({ ipv6: configs.value.ipv6 })
}

const changeMode = () => {
  patchConfig({ mode: configs.value.mode })
}

const changeLogLevel = () => {
  patchConfig({ 'log-level': configs.value['log-level'] })
}

const saveInterface = () => {
  patchConfig({ 'interface-name': configs.value['interface-name'] })
}

const savePorts = async (e?: Event) => {
  if (e && e.type === 'keyup' && e.target instanceof HTMLElement) {
    e.target.blur()
    return
  }

  const port = configs.value.port || 0
  const socksPort = configs.value['socks-port'] || 0
  const redirPort = configs.value['redir-port'] || 0
  const tproxyPort = configs.value['tproxy-port'] || 0
  const mixedPort = configs.value['mixed-port'] || 0

  const ports = [port, socksPort, redirPort, tproxyPort, mixedPort]

  for (const p of ports) {
    if (p !== 0 && (p < 1025 || p > 65535)) {
      globalStore.showToast(t('config.port_invalid_hint'), 'error')
      fetchConfigs(true)
      return
    }
  }

  const activePorts = ports.filter(p => p !== 0)
  if (new Set(activePorts).size !== activePorts.length) {
    globalStore.showToast(t('config.port_duplicate_hint'), 'error')
    fetchConfigs(true)
    return
  }

  // 仅 PATCH 内核运行时配置，不触碰订阅配置的持久化和内存状态
  await patchConfig({
    port,
    'socks-port': socksPort,
    'redir-port': redirPort,
    'tproxy-port': tproxyPort,
    'mixed-port': mixedPort
  })
}

const saveTun = (e?: Event) => {
  if (e && e.type === 'keyup' && e.target instanceof HTMLElement) {
    e.target.blur()
    return
  }
  const isTunEnabled = configs.value.tun.enable
  // 互斥：如果开启了 TUN，自动关闭 TProxy（只关开关，不改端口）
  if (isTunEnabled) {
    configStore.tproxyEnabled = false
  }
  patchConfig({ tun: configs.value.tun })
}

const toggleTProxy = async (enable: boolean) => {
  if (enable) {
    const port = configs.value['tproxy-port'] || 0
    if (port === 0) {
      globalStore.showToast(t('config.tproxy_port_zero_warning'), 'warning')
      // 回退开关状态
      configStore.tproxyEnabled = false
      return
    }
    // 互斥：如果 TUN 开启，关闭 TUN
    if (configs.value.tun.enable) {
      configs.value.tun.enable = false
      await patchConfig({ tun: configs.value.tun })
    }
  }

  // 调用后端更新状态（不涉及端口）
  await apiFetch('/config/tproxy', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enable })
  })
  // 开关状态已由 v-model 双向绑定自动更新，无需额外赋值
}

// 内核进程管理
const handleStartCore = async () => {
  coreStatus.value.loading = true
  try {
    const resp = await apiFetch('/core/start', { method: 'POST' })
    const data = await resp.json()
    if (resp.ok && data.status === 'ok') {
      globalStore.showToast(t('config.core_start_success'), 'success')
      configStore.refreshCoreStatus()
      setTimeout(() => {
        fetchConfigs()
        overviewStore.fetchVersionAndStatus()
      }, 1500)
    } else {
      globalStore.showToast(t('config.core_start_failed') + ': ' + (data.message || ''), 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    coreStatus.value.loading = false
  }
}

const handleStopCore = async () => {
  const ok = await globalStore.showConfirm({
    message: t('config.confirm_stop_core'),
    type: 'danger'
  })
  if (!ok) return
  coreStatus.value.loading = true
  try {
    const resp = await apiFetch('/core/stop', { method: 'POST' })
    const data = await resp.json()
    if (resp.ok && data.status === 'ok') {
      globalStore.showToast(t('config.core_stopped_success'), 'success')
      configStore.refreshCoreStatus()
      configsLoading.value = true
      configs.value = {
        'allow-lan': false,
        ipv6: false,
        mode: 'Rule',
        'log-level': 'silent',
        'interface-name': '',
        tun: { enable: false, stack: 'System', device: '' },
        port: 0,
        'socks-port': 0,
        'redir-port': 0,
        'tproxy-port': 0,
        'mixed-port': 0
      }
    } else {
      globalStore.showToast(t('config.core_stop_failed') + ': ' + (data.message || ''), 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    coreStatus.value.loading = false
  }
}

const handleRestartCore = async () => {
  const ok = await globalStore.showConfirm({
    message: t('config.confirm_restart'),
    type: 'warning'
  })
  if (!ok) return
  coreStatus.value.loading = true
  try {
    const resp = await apiFetch('/restart', { method: 'POST' })
    if (resp.ok) {
      globalStore.showToast(t('config.restart_sent'), 'success')
      setTimeout(() => {
        fetchConfigs()
        overviewStore.fetchVersionAndStatus()
      }, 1500)
    } else {
      globalStore.showToast(t('config.restart_failed'), 'error')
      coreStatus.value.loading = false
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
    coreStatus.value.loading = false
  }
}

const handleReloadConfig = async () => {
  isReloading.value = true
  try {
    const resp = await apiFetch('/configs', { method: 'PUT' })
    if (resp.ok) {
      globalStore.showToast(t('config.reload_success'), 'success')
      fetchConfigs()
    } else {
      globalStore.showToast(t('config.reload_failed'), 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    isReloading.value = false
  }
}

const handleFlushFakeIP = async () => {
  isFlushingFakeIP.value = true
  try {
    const resp = await apiFetch('/cache/fakeip/flush', { method: 'POST' })
    if (resp.ok) globalStore.showToast(t('config.flush_fakeip_success'), 'success')
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    isFlushingFakeIP.value = false
  }
}

const handleFlushDNS = async () => {
  isFlushingDNS.value = true
  try {
    const resp = await apiFetch('/cache/dns/flush', { method: 'POST' })
    if (resp.ok) globalStore.showToast(t('config.flush_dns_success'), 'success')
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    isFlushingDNS.value = false
  }
}

const handleUpdateGeo = async () => {
  isUpdatingGeo.value = true
  try {
    let resp = await apiFetch('/providers/geo', { method: 'POST' }).catch(() => null)
    if (!resp || !resp.ok) {
      resp = await apiFetch('/configs/geo', { method: 'POST' })
    }
    if (resp.ok) {
      globalStore.showToast(t('config.update_geo_sent'), 'success')
    } else {
      globalStore.showToast(t('config.update_geo_failed'), 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    isUpdatingGeo.value = false
  }
}

const handleUpgradeCore = async () => {
  isUpgrading.value = true
  try {
    const resp = await apiFetch('/upgrade', { method: 'POST' })
    if (resp.ok) {
      overviewStore.fetchVersionAndStatus()
    }
  } catch (e) {
    // 忽略升级中可能的502或连接中断
  } finally {
    isUpgrading.value = false
  }
}

const handleDNSQuery = async (e?: Event) => {
  if (!dnsQuery.value.name.trim()) return

  // 主动释放焦点，在移动端自动收起虚拟键盘，以便用户看清下方的解析结果
  if (e && e.target instanceof HTMLElement) {
    e.target.blur()
  }

  dnsQuery.value.loading = true
  dnsQuery.value.result = ''
  try {
    const query = `name=${encodeURIComponent(dnsQuery.value.name)}&type=${dnsQuery.value.type}`
    const resp = await apiFetch(`/dns/query?${query}`)
    if (resp.ok) {
      const data = await resp.json()
      if (data.Status === 0 && data.Answer && data.Answer.length > 0) {
        dnsQuery.value.result = data.Answer.map((a: any) => a.data).join('\n')
      } else {
        dnsQuery.value.result = JSON.stringify(data, null, 2)
      }
    } else {
      dnsQuery.value.result = t('config.dns_query_failed')
    }
  } catch (e) {
    dnsQuery.value.result = `${t('config.dns_query_failed')}: ${(e as Error).message}`
  } finally {
    dnsQuery.value.loading = false
  }
}

// 处理本机代理开关切换（开启时弹窗确认）
const handleProxyLocalToggle = async (newVal: boolean) => {
  if (newVal) {
    // 尝试开启 -> 弹窗警告
    const confirmed = await globalStore.showConfirm({
      title: t('common.warning'),
      message: t('config.tproxy_proxy_local_warning'),
      type: 'warning'
    })
    if (confirmed) {
      tproxyProxyLocal.value = true
    }
    // 取消则保持原值（false）
  } else {
    // 关闭直接生效
    tproxyProxyLocal.value = false
  }
}

// 处理 TUN 开关切换（开启时弹窗确认）
const handleTunToggle = async (newVal: boolean) => {
  if (newVal) {
    // 尝试开启 -> 弹窗警告
    const confirmed = await globalStore.showConfirm({
      title: t('common.warning'),
      message: t('config.tun_enable_warning'),
      type: 'warning'
    })
    if (confirmed) {
      configs.value.tun.enable = true
      saveTun() // 内部处理互斥并 patch 后端
    }
    // 取消则保持原值（false）
  } else {
    // 关闭直接生效
    configs.value.tun.enable = false
    saveTun()
  }
}

const changeLang = () => {
  localStorage.setItem('lang', locale.value)
}

// 添加加载状态
const tproxyLoading = ref(true)

onMounted(async () => {
  fetchConfigs()
  fetchInterfaces()
  
  // 等待 TProxy 状态加载完成，并设置超时保护
  await Promise.race([
    configStore.fetchTproxyState(),
    new Promise(r => setTimeout(r, 3000))
  ])
  tproxyLoading.value = false
})


onUnmounted(() => {
})

onActivated(() => {
  configStore.fetchTproxyState()
})

</script>

<template>
  <div class="flex flex-col flex-1 min-h-0 gap-4 h-full">
    <!-- 顶部操作栏 -->
    <div class="glass-medium shadow-none px-6 py-3 md:py-0 rounded-xl border border-slate-200/50 dark:border-slate-800/50 flex flex-wrap gap-4 items-center justify-between transition-all shrink-0 h-auto min-h-[56px] md:h-[56px]">
      <h3 class="text-base font-semibold flex items-center gap-2">
        <SettingsOutline class="w-5 h-5 text-accent" />
        {{ t('nav.config') }}
      </h3>

      <!-- 文档 + 关于 按钮组（紧挨） -->
        <div class="flex items-center gap-1">
          <a href="https://ttq.fjb.dpdns.org" target="_blank" rel="noopener noreferrer"
            class="flex items-center justify-center text-xs font-semibold rounded-xl bg-slate-50 hover:bg-slate-100 dark:bg-slate-800/40 dark:hover:bg-slate-800/80 transition-all text-slate-600 dark:text-slate-300 hover:scale-105 active:scale-95 border border-slate-200/50 dark:border-slate-800/30 px-3 py-1.5 gap-1.5 group"
            :title="t('nav.docs')">
            <DocumentTextOutline class="w-4 h-4 shrink-0 transition-transform duration-300 group-hover:scale-110" />
            <span>{{ t('nav.docs') }}</span>
          </a>
    
          <button @click="globalStore.showAbout = true"
            class="flex items-center justify-center text-xs font-semibold rounded-xl bg-slate-50 hover:bg-slate-100 dark:bg-slate-800/40 dark:hover:bg-slate-800/80 transition-all text-slate-600 dark:text-slate-300 hover:scale-105 active:scale-95 border border-slate-200/50 dark:border-slate-800/30 px-3 py-1.5 gap-1.5 group"
            :title="t('about.title')">
            <InformationCircleOutline class="w-4 h-4 shrink-0 transition-transform duration-300 group-hover:rotate-12" />
            <span>{{ t('about.title') }}</span>
          </button>
        </div>
    </div>

    <!-- 核心状态加载中的优雅 Loading 占位 -->
    <div v-if="coreStatus.loading" class="flex-1 flex flex-col items-center justify-center gap-3 select-none">
      <div class="w-7 h-7 border-2 border-slate-200 dark:border-slate-800 !border-t-accent rounded-full animate-spin"></div>
      <span class="text-xs font-bold text-slate-400 dark:text-slate-500 tracking-wider">正在加载系统参数...</span>
    </div>

    <!-- 加载完成后的内滚动内容区 (已升级为统一大内容卡片) -->
    <div v-else class="flex-1 min-h-0 overflow-y-auto glass-medium shadow-none rounded-xl border border-slate-200/50 dark:border-slate-800/50 p-6">
      <div class="grid grid-cols-1 gap-6 items-start w-full animate-[fadeIn_0.25s_ease-out]"
        :class="[
          coreStatus.running 
            ? 'md:grid-cols-2 lg:grid-cols-3' 
            : 'md:grid-cols-2 max-w-4xl mx-auto'
        ]">
        <!-- 1. 配置参数面板区（常规参数，内核启动时显示） -->
        <div v-if="coreStatus.running"
          class="live-card bg-slate-50/50 dark:bg-slate-900/30 p-6 rounded-xl border border-slate-200/40 dark:border-slate-800/40 hover:border-slate-300/80 dark:hover:border-slate-700/80 hover:-translate-y-[3px] hover:shadow-md hover:bg-slate-100/80 dark:hover:bg-slate-900/80 duration-300 space-y-5 h-full transition-all flex flex-col relative">
          <!-- 同步配置遮罩屏 -->
          <div v-if="configsLoading"
            class="absolute inset-0 glass-light z-30 flex flex-col items-center justify-center rounded-2xl gap-2 select-none border shadow-sm transition-all duration-300">
            <div class="w-5 h-5 border-2 border-slate-200 dark:border-slate-700 !border-t-accent rounded-full animate-spin"></div>
            <span class="text-xs font-bold text-slate-500 dark:text-slate-400 tracking-wider">{{ t('config.syncing_configs') }}</span>
          </div>

          <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
            <OptionsOutline class="w-4 h-4 text-accent" />
            {{ t('config.general_settings') }}
          </h4>

          <div class="flex items-center justify-between">
            <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.allow_lan') }}</label>
            <FormSwitch v-model="configs['allow-lan']" @update:model-value="toggleAllowLan" />
          </div>

          <div class="flex items-center justify-between">
            <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.ipv6_toggle') }}</label>
            <FormSwitch v-model="configs.ipv6" @update:model-value="toggleIPv6" />
          </div>

          <div class="flex flex-col gap-1.5">
            <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.mode') }}</label>
            <select v-model="configs.mode" @change="changeMode"
              class="px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full">
              <option value="Rule">{{ t('config.mode_rule') }}</option>
              <option value="Global">{{ t('config.mode_global') }}</option>
              <option value="Direct">{{ t('config.mode_direct') }}</option>
            </select>
          </div>

          <div class="flex flex-col gap-1.5">
            <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.log_level') }}</label>
            <select v-model="configs['log-level']" @change="changeLogLevel"
              class="px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full">
              <option value="silent">Silent</option>
              <option value="info">Info</option>
              <option value="warning">Warning</option>
              <option value="error">Error</option>
              <option value="debug">Debug</option>
            </select>
          </div>
        </div>

        <!-- 2. 端口设置（内核启动时显示） -->
        <div v-if="coreStatus.running"
          class="live-card bg-slate-50/50 dark:bg-slate-900/30 p-6 rounded-xl border border-slate-200/40 dark:border-slate-800/40 hover:border-slate-300/80 dark:hover:border-slate-700/80 hover:-translate-y-[3px] hover:shadow-md hover:bg-slate-100/80 dark:hover:bg-slate-900/80 duration-300 space-y-5 h-full transition-all flex flex-col relative">
          <!-- 同步配置遮罩屏 -->
          <div v-if="configsLoading"
            class="absolute inset-0 glass-light z-30 flex flex-col items-center justify-center rounded-2xl gap-2 select-none border shadow-sm transition-all duration-300">
            <div class="w-5 h-5 border-2 border-slate-200 dark:border-slate-700 !border-t-accent rounded-full animate-spin"></div>
            <span class="text-xs font-bold text-slate-500 dark:text-slate-400 tracking-wider">{{ t('config.syncing_configs') }}</span>
          </div>

          <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
            <HardwareChipOutline class="w-4 h-4 text-accent" />
            {{ t('config.port_settings') }}
          </h4>

          <div class="grid grid-cols-2 gap-4">
            <div class="flex flex-col gap-1">
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.mixed_port') }}</label>
              <input type="number" v-model.number="configs['mixed-port']" min="0" max="65535" step="1" @blur="savePorts" @keyup.enter="savePorts" :placeholder="t('config.port_disabled_hint')"
                class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full" />
            </div>
            <div class="flex flex-col gap-1">
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.http_port') }}</label>
              <input type="number" v-model.number="configs.port" min="0" max="65535" step="1" @blur="savePorts" @keyup.enter="savePorts" :placeholder="t('config.port_disabled_hint')"
                class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full" />
            </div>
            <div class="flex flex-col gap-1">
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.socks_port') }}</label>
              <input type="number" v-model.number="configs['socks-port']" min="0" max="65535" step="1" @blur="savePorts" @keyup.enter="savePorts" :placeholder="t('config.port_disabled_hint')"
                class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full" />
            </div>
            <div class="flex flex-col gap-1">
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.redir_port') }}</label>
              <input type="number" v-model.number="configs['redir-port']" min="0" max="65535" step="1" @blur="savePorts" @keyup.enter="savePorts" :placeholder="t('config.port_disabled_hint')"
                class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full" />
            </div>
            <div class="flex flex-col gap-1 col-span-2">
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.tproxy_port') }}</label>
              <input type="number" v-model.number="configs['tproxy-port']" 
                min="0" max="65535" step="1"
                @blur="savePorts" @keyup.enter="savePorts" 
                :placeholder="t('config.port_disabled_hint')"
                :readonly="configStore.tproxyEnabled"
                @click="onTproxyPortClick"
                class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full"
                :class="configStore.tproxyEnabled ? 'cursor-not-allowed opacity-60' : ''" />
            </div>
          </div>
        </div>

        <!-- 3. TUN与网卡设置（内核启动时显示） -->
        <div v-if="coreStatus.running"
          class="live-card bg-slate-50/50 dark:bg-slate-900/30 p-6 rounded-xl border border-slate-200/40 dark:border-slate-800/40 hover:border-slate-300/80 dark:hover:border-slate-700/80 hover:-translate-y-[3px] hover:shadow-md hover:bg-slate-100/80 dark:hover:bg-slate-900/80 duration-300 space-y-5 h-full transition-all flex flex-col relative">
          <!-- 同步配置遮罩屏 -->
          <div v-if="configsLoading"
            class="absolute inset-0 glass-light z-30 flex flex-col items-center justify-center rounded-2xl gap-2 select-none border shadow-sm transition-all duration-300">
            <div class="w-5 h-5 border-2 border-slate-200 dark:border-slate-700 !border-t-accent rounded-full animate-spin"></div>
            <span class="text-xs font-bold text-slate-500 dark:text-slate-400 tracking-wider">{{ t('config.syncing_configs') }}</span>
          </div>

          <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
            <ShieldCheckmarkOutline class="w-4 h-4 text-accent" />
            {{ t('config.tun_settings') }}
          </h4>

          <!-- TUN 模式 -->
          <div class="flex items-center justify-between">
            <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.tun_enable') }}</label>
            <FormSwitch :model-value="configs.tun.enable" @update:model-value="handleTunToggle"/>
          </div>
          <div class="grid grid-cols-2 gap-4">
            <div class="flex flex-col gap-1">
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.tun_stack') }}</label>
              <select v-model="configs.tun.stack" @change="saveTun"
                class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full">
                <option value="gVisor">gVisor</option>
                <option value="System">System</option>
                <option value="Mixed">Mixed</option>
              </select>
            </div>
            <div class="flex flex-col gap-1">
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.tun_device') }}</label>
              <input type="text" v-model="configs.tun.device" @blur="saveTun" @keyup.enter="saveTun" :placeholder="t('config.interface_name_placeholder')"
                class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full" />
            </div>
          </div>

          <div class="flex flex-col gap-1">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.interface_name') }}</label>
            <select v-model="configs['interface-name']" @change="saveInterface"
              class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full">
              <option value="">{{ t('config.interface_name_auto') }}</option>
              <option v-for="iface in interfaces" :key="iface" :value="iface">{{ iface }}</option>
            </select>
          </div>

          <!-- 分割线 -->
          <div class="h-px bg-slate-200 dark:bg-slate-700 my-1.5"></div>

          <!-- TProxy 透明代理 -->
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">
                {{ t('config.tproxy_enable') }}
              </label>
              <button
                @click="openTproxyExceptionsDialog"
                class="p-1 text-slate-400 hover:text-accent rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all"
                :class="configStore.tproxyEnabled ? 'opacity-40 cursor-not-allowed' : 'hover:text-accent'"
                :title="configStore.tproxyEnabled ? t('config.tproxy_exceptions_disabled_message') : t('config.tproxy_exceptions_title')"
              >
                <SettingsOutline class="w-4 h-4" />
              </button>
            </div>
            <div v-if="tproxyLoading" class="w-7 h-4 rounded-full bg-slate-200 dark:bg-slate-700 animate-pulse"></div>
            <FormSwitch v-model="configStore.tproxyEnabled" @update:model-value="toggleTProxy" />
          </div>
        </div>

        <!-- 4. 运维控制（始终显示） -->
        <div
          class="live-card bg-slate-50/50 dark:bg-slate-900/30 p-6 rounded-xl border border-slate-200/40 dark:border-slate-800/40 hover:border-slate-300/80 dark:hover:border-slate-700/80 hover:-translate-y-[3px] hover:shadow-md hover:bg-slate-100/80 dark:hover:bg-slate-900/80 duration-300 space-y-5 h-full transition-all flex flex-col">
          <div class="border-b border-slate-100 dark:border-slate-800 pb-4">
            <h4 class="font-bold text-sm flex items-center gap-2">
              <BuildOutline class="w-4 h-4 text-accent" />
              {{ t('config.advanced_maintenance') }}
            </h4>
          </div>

          <div class="space-y-4 flex-1 flex flex-col justify-between">
            <!-- 内核状态 -->
            <div class="flex items-center justify-between px-3.5 py-2.5 bg-slate-50 dark:bg-slate-900/40 rounded-xl border border-slate-100 dark:border-slate-800/80">
              <span class="text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('config.core_status') }}</span>
              <div class="flex items-center gap-2.5 text-xs">
                <span class="w-2 h-2 rounded-full flex shrink-0"
                  :class="coreStatus.loading ? 'bg-slate-400 animate-pulse' : (coreStatus.running ? 'bg-success' : 'bg-red-500')"></span>
                <span class="font-bold text-slate-700 dark:text-slate-200">
                  {{ coreStatus.loading ? t('config.core_checking') : (coreStatus.running ? t('config.core_running') : t('config.core_stopped')) }}
                </span>
                <span v-if="coreStatus.running && stats.coreVersion !== '未知' && stats.coreVersion !== '加载中...'"
                  class="px-1.5 py-0.5 font-mono text-[10px] bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 rounded">
                  {{ coreVersion }}
                </span>
              </div>
            </div>

            <!-- 内核核心控制 -->
            <div class="grid gap-3 w-full"
              :class="coreStatus.running ? 'grid-cols-3' : 'grid-cols-2'">
              <button v-if="!coreStatus.running" @click="handleStartCore" :disabled="coreStatus.loading"
                class="py-2 bg-success hover:bg-success-hover text-white text-xs font-semibold rounded-xl shadow-md shadow-success/15 hover:shadow-success/25 transition-all flex items-center justify-center gap-1.5 w-full">
                <SyncOutline v-if="coreStatus.loading" class="w-3.5 h-3.5 animate-spin inline-block" />
                {{ coreStatus.loading ? t('config.core_starting') : t('config.start_core') }}
              </button>
              <template v-else>
                <button @click="handleStopCore" :disabled="coreStatus.loading"
                  class="py-2 bg-red-500 hover:bg-red-600 text-white text-xs font-semibold rounded-xl shadow-md shadow-red-500/15 hover:shadow-red-500/25 transition-all flex items-center justify-center gap-1.5 w-full">
                  <SyncOutline v-if="coreStatus.loading" class="w-3.5 h-3.5 animate-spin inline-block" />
                  {{ coreStatus.loading ? t('config.core_stopping') : t('config.stop_core') }}
                </button>
                <button @click="handleRestartCore" :disabled="coreStatus.loading"
                  class="py-2 bg-amber-500 hover:bg-amber-600 text-white text-xs font-semibold rounded-xl shadow-md shadow-amber-500/15 hover:shadow-amber-500/25 transition-all flex items-center justify-center w-full">
                  {{ t('config.restart') }}
                </button>
              </template>
              <button @click="handleUpgradeCore" :disabled="isUpgrading || !coreStatus.running"
                class="py-2 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-xl shadow-md shadow-accent/15 hover:shadow-accent/25 transition-all flex items-center justify-center w-full disabled:opacity-50 disabled:cursor-not-allowed">
                {{ isUpgrading ? t('config.upgrading_core') : t('config.upgrade_core') }}
              </button>
            </div>
   
            <!-- 分割线 -->
            <div class="h-px bg-slate-200 dark:bg-slate-700 my-1.5"></div>
   
            <!-- 常规运维 -->
            <div class="grid grid-cols-2 gap-3">
              <button @click="handleReloadConfig" :disabled="!coreStatus.running || isReloading"
                class="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-xl text-slate-700 dark:text-slate-200 transition-all border border-slate-200/20 disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-1.5">
                <div v-if="isReloading" class="w-3 h-3 border border-slate-300 dark:border-slate-600 !border-t-accent rounded-full animate-spin"></div>
                {{ isReloading ? t('config.reloading') : t('config.reload') }}
              </button>
              <button @click="handleFlushFakeIP" :disabled="!coreStatus.running || isFlushingFakeIP"
                class="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-xl text-slate-700 dark:text-slate-200 transition-all border border-slate-200/20 disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-1.5">
                <div v-if="isFlushingFakeIP" class="w-3 h-3 border border-slate-300 dark:border-slate-600 !border-t-accent rounded-full animate-spin"></div>
                {{ isFlushingFakeIP ? t('config.flushing') : t('config.flush_fakeip') }}
              </button>
              <button @click="handleFlushDNS" :disabled="!coreStatus.running || isFlushingDNS"
                class="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-xl text-slate-700 dark:text-slate-200 transition-all border border-slate-200/20 disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-1.5">
                <div v-if="isFlushingDNS" class="w-3 h-3 border border-slate-300 dark:border-slate-600 !border-t-accent rounded-full animate-spin"></div>
                {{ isFlushingDNS ? t('config.flushing') : t('config.flush_dns') }}
              </button>
              <button @click="handleUpdateGeo" :disabled="!coreStatus.running || isUpdatingGeo"
                class="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-xl text-slate-700 dark:text-slate-200 transition-all border border-slate-200/20 disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-1.5">
                <div v-if="isUpdatingGeo" class="w-3 h-3 border border-slate-300 dark:border-slate-600 !border-t-accent rounded-full animate-spin"></div>
                {{ isUpdatingGeo ? t('config.upgrading_core') : t('config.update_geo') }}
              </button>
            </div>
          </div>
        </div>

        <!-- 5. 界面设置（始终显示） -->
        <div
          class="live-card bg-slate-50/50 dark:bg-slate-900/30 p-6 rounded-xl border border-slate-200/40 dark:border-slate-800/40 hover:border-slate-300/80 dark:hover:border-slate-700/80 hover:-translate-y-[3px] hover:shadow-md hover:bg-slate-100/80 dark:hover:bg-slate-900/80 duration-300 space-y-5 h-full transition-all flex flex-col">
          <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
            <ColorPaletteOutline class="w-4 h-4 text-accent" />
            {{ t('config.interface_settings') }}
          </h4>

          <div class="space-y-4 flex-1">
            <div class="flex flex-col gap-1.5">
              <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.language') }}</label>
              <select v-model="locale" @change="changeLang"
                class="px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full">
                <option value="zh">{{ t('config.lang_zh') }}</option>
                <option value="en">{{ t('config.lang_en') }}</option>
              </select>
            </div>

            <div class="flex flex-col gap-1.5">
              <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.theme') }}</label>
              <select v-model="globalStore.theme"
                class="px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full">
                <option value="light">{{ t('config.theme_light') }}</option>
                <option value="dark">{{ t('config.theme_dark') }}</option>
                <option value="purple">{{ t('config.theme_purple') }}</option>
                <option value="pink">{{ t('config.theme_pink') }}</option>
                <option value="green">{{ t('config.theme_green') }}</option>
                <option value="blue">{{ t('config.theme_blue') }}</option>
                <option value="system">{{ t('config.theme_system') }}</option>
              </select>
            </div>

            <div class="flex flex-col gap-1.5">
              <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.start_page') }}</label>
              <select v-model="globalStore.startPage"
                class="px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full">
                <option value="last">{{ t('config.start_page_last') }}</option>
                <option value="overview">{{ t('nav.overview') }}</option>
                <option value="proxies">{{ t('nav.proxies') }}</option>
                <option value="subscription">{{ t('nav.subscription') }}</option>
                <option value="rules">{{ t('nav.rules') }}</option>
                <option value="connections">{{ t('nav.connections') }}</option>
                <option value="logs">{{ t('nav.logs') }}</option>
                <option value="config">{{ t('nav.config') }}</option>
              </select>
            </div>
          </div>
        </div>

        <!-- 6. DNS 查询（内核启动时显示） -->
        <div v-if="coreStatus.running"
          class="live-card bg-slate-50/50 dark:bg-slate-900/30 p-6 rounded-xl border border-slate-200/40 dark:border-slate-800/40 hover:border-slate-300/80 dark:hover:border-slate-700/80 hover:-translate-y-[3px] hover:shadow-md hover:bg-slate-100/80 dark:hover:bg-slate-900/80 duration-300 space-y-4 h-full transition-all flex flex-col">
          <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
            <SearchOutline class="w-4 h-4 text-accent" />
            {{ t('config.dns_query') }}
          </h4>

          <div class="flex-1 flex flex-col justify-between gap-4">
            <div class="flex flex-col gap-2">
              <input type="text" v-model="dnsQuery.name" :placeholder="t('config.dns_placeholder')"
                @keyup.enter="handleDNSQuery"
                class="w-full px-4 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none" />
              <div class="flex gap-2">
                <select v-model="dnsQuery.type"
                  class="flex-1 px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none">
                  <option value="A">A</option>
                  <option value="AAAA">AAAA</option>
                  <option value="MX">MX</option>
                  <option value="TXT">TXT</option>
                </select>
                <button @click="handleDNSQuery" :disabled="dnsQuery.loading"
                  class="flex-[2] py-2 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all flex items-center justify-center gap-1">
                  {{ dnsQuery.loading ? t('config.dns_querying') : t('config.dns_query_btn') }}
                </button>
              </div>
            </div>

            <pre
              class="p-4 bg-slate-50 dark:bg-slate-900/50 font-mono text-xs rounded-xl overflow-y-auto whitespace-pre-wrap break-all h-28 border border-slate-200 dark:border-slate-800 transition-all flex-1"
              :class="dnsQuery.result ? 'text-emerald-700 dark:text-emerald-400' : 'text-slate-400 dark:text-slate-500 italic flex items-center justify-center select-none'">{{ dnsQuery.result || t('config.dns_result_default') }}</pre>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- ====== 新增 TProxy 例外列表弹窗 ====== -->
  <Teleport to="body">
    <div v-if="showTproxyExceptionsDialog" class="fixed inset-0 glass-mask z-[9999] flex items-center justify-center p-4" @click.self="showTproxyExceptionsDialog = false">
      <div class="glass-heavy w-full max-w-lg rounded-[20px] shadow-2xl border p-6 flex flex-col gap-4 animate-[zoomIn_0.15s_ease-out]">
        <h4 class="text-lg font-bold">{{ t('config.tproxy_exceptions_title') }}</h4>

        <div class="space-y-4">
          <!-- 目的例外 -->
          <div>
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">
              {{ t('config.tproxy_dst_exceptions_label') }}
            </label>
            <p class="text-[11px] text-slate-400 dark:text-slate-500 mt-0.5">{{ t('config.tproxy_dst_exceptions_hint') }}</p>
            <textarea
              v-model="tproxyDstExceptionsText"
              rows="6"
              class="w-full p-3 text-sm font-mono rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none resize-y min-h-[100px]"
              :placeholder="t('config.tproxy_dst_exceptions_placeholder')"
            ></textarea>
          </div>

          <!-- 源例外 -->
          <div class="pt-2 border-t border-slate-100 dark:border-slate-800/60">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">
              {{ t('config.tproxy_src_exceptions_label') }}
            </label>
            <p class="text-[11px] text-slate-400 dark:text-slate-500 mt-0.5">{{ t('config.tproxy_src_exceptions_hint') }}</p>
            <textarea
              v-model="tproxySrcExceptionsText"
              rows="6"
              class="w-full p-3 text-sm font-mono rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none resize-y min-h-[100px]"
              :placeholder="t('config.tproxy_src_exceptions_placeholder')"
            ></textarea>
          </div>

          <!-- 本机代理开关 -->
          <div class="flex items-center justify-between pt-2 border-t border-slate-100 dark:border-slate-800/60">
            <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">
              {{ t('config.tproxy_proxy_local_label') }}
            </label>
              <FormSwitch
                :model-value="tproxyProxyLocal"
                @update:model-value="handleProxyLocalToggle"
              />
          </div>
        </div>

        <div class="flex justify-end gap-2.5 pt-3 border-t border-slate-100 dark:border-slate-800/60">
          <button @click="showTproxyExceptionsDialog = false" class="px-4 py-2 text-sm font-semibold rounded-xl bg-white border border-slate-200 hover:bg-slate-50 dark:bg-slate-800 dark:border-slate-700 dark:hover:bg-slate-700/60 text-slate-600 dark:text-slate-300 transition-all">
            {{ t('common.cancel') }}
          </button>
          <button @click="saveTproxyExceptions" class="px-4 py-2 text-sm font-semibold rounded-xl bg-accent hover:bg-accent-hover text-white transition-all shadow-md shadow-accent/15">
            {{ t('common.save') }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
