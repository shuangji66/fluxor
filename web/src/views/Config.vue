<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiFetch } from '../utils/api'
import { CloudDownloadOutline, OptionsOutline, HardwareChipOutline, ShieldCheckmarkOutline, BuildOutline, SearchOutline, RefreshOutline, ColorPaletteOutline } from '@vicons/ionicons5'
import { useGlobalStore } from '../store/global'
import { storeToRefs } from 'pinia'
import { useConfigStore, type ConfigData } from '../store/config'
import { useOverviewStore } from '../store/overview'

const { t, locale } = useI18n()
const globalStore = useGlobalStore()
const configStore = useConfigStore()
const overviewStore = useOverviewStore()
const { coreStatus, configs, configsLoading } = storeToRefs(configStore)
const { stats } = storeToRefs(overviewStore)

const fetchCoreStatus = configStore.fetchCoreStatus
const fetchConfigs = configStore.fetchConfigs

const coreVersion = computed(() => {
  if (stats.value.coreVersion === '加载中...') return t('common.loading')
  if (stats.value.coreVersion === '未知') return ''
  return 'v' + stats.value.coreVersion
})

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

const mixedPortInput = ref<string>('')
const portInput = ref<string>('')
const socksPortInput = ref<string>('')
const redirPortInput = ref<string>('')
const tproxyPortInput = ref<string>('')

const syncPortsFromConfigs = () => {
  const cfg = configs.value
  mixedPortInput.value = cfg['mixed-port'] ? String(cfg['mixed-port']) : ''
  portInput.value = cfg.port ? String(cfg.port) : ''
  socksPortInput.value = cfg['socks-port'] ? String(cfg['socks-port']) : ''
  redirPortInput.value = cfg['redir-port'] ? String(cfg['redir-port']) : ''
  tproxyPortInput.value = cfg['tproxy-port'] ? String(cfg['tproxy-port']) : ''
}

watch(() => configs.value, () => {
  syncPortsFromConfigs()
}, { deep: true, immediate: true })


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

const savePorts = () => {
  const getPortVal = (val: string) => {
    const p = parseInt(val)
    return isNaN(p) ? 0 : p
  }

  const port = getPortVal(portInput.value)
  const socksPort = getPortVal(socksPortInput.value)
  const redirPort = getPortVal(redirPortInput.value)
  const tproxyPort = getPortVal(tproxyPortInput.value)
  const mixedPort = getPortVal(mixedPortInput.value)

  const ports = [port, socksPort, redirPort, tproxyPort, mixedPort]

  for (const p of ports) {
    if (p !== 0 && (p < 1025 || p > 65535)) {
      globalStore.showToast(t('config.port_invalid_hint'), 'error')
      syncPortsFromConfigs()
      return
    }
  }

  const activePorts = ports.filter(p => p !== 0)
  if (new Set(activePorts).size !== activePorts.length) {
    globalStore.showToast(t('config.port_duplicate_hint'), 'error')
    syncPortsFromConfigs()
    return
  }

  // 检查是否有实际变更，如果没有则不请求后端
  if (
    port === configs.value.port &&
    socksPort === configs.value['socks-port'] &&
    redirPort === configs.value['redir-port'] &&
    tproxyPort === configs.value['tproxy-port'] &&
    mixedPort === configs.value['mixed-port']
  ) {
    return
  }

  patchConfig({
    port,
    'socks-port': socksPort,
    'redir-port': redirPort,
    'tproxy-port': tproxyPort,
    'mixed-port': mixedPort
  })
}

const saveTun = () => {
  patchConfig({ tun: configs.value.tun })
}

// 内核进程管理
const handleStartCore = async () => {
  coreStatus.value.loading = true
  try {
    const resp = await apiFetch('/core/start', { method: 'POST' })
    const data = await resp.json()
    if (resp.ok && data.status === 'ok') {
      globalStore.showToast(t('config.core_start_success'), 'success')
      fetchCoreStatus()
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
  const ok = await globalStore.showConfirm(t('config.confirm_stop_core'))
  if (!ok) return
  coreStatus.value.loading = true
  try {
    const resp = await apiFetch('/core/stop', { method: 'POST' })
    const data = await resp.json()
    if (resp.ok && data.status === 'ok') {
      globalStore.showToast(t('config.core_stopped_success'), 'success')
      fetchCoreStatus()
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
  const ok = await globalStore.showConfirm(t('config.confirm_restart'))
  if (!ok) return
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
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  }
}

const handleReloadConfig = async () => {
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
  }
}

const handleFlushFakeIP = async () => {
  try {
    const resp = await apiFetch('/cache/fakeip/flush', { method: 'POST' })
    if (resp.ok) globalStore.showToast(t('config.flush_fakeip_success'), 'success')
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  }
}

const handleFlushDNS = async () => {
  try {
    const resp = await apiFetch('/cache/dns/flush', { method: 'POST' })
    if (resp.ok) globalStore.showToast(t('config.flush_dns_success'), 'success')
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  }
}

const handleUpdateGeo = async () => {
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

const handleDNSQuery = async () => {
  if (!dnsQuery.value.name.trim()) return
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

const changeLang = () => {
  localStorage.setItem('lang', locale.value)
}

onMounted(() => {
  fetchCoreStatus()
  fetchConfigs()
  fetchInterfaces()
  statusTimer.value = setInterval(fetchCoreStatus, 5000)
})

onUnmounted(() => {
  if (statusTimer.value) clearInterval(statusTimer.value)
})
</script>

<template>
  <div class="space-y-6 w-full">
    <!-- 1. 配置参数面板区（内核启动时才显示） -->
    <div v-if="coreStatus.running" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 relative">
      <!-- 同步配置遮罩屏 -->
      <div v-if="configsLoading"
        class="absolute inset-0 bg-white/40 dark:bg-[#1e293b]/45 backdrop-blur-[1.5px] z-30 flex flex-col items-center justify-center rounded-2xl gap-2 select-none border border-slate-200/40 dark:border-slate-800/40 shadow-sm animate-pulse">
        <span class="text-xs font-bold text-slate-500 dark:text-slate-400 tracking-wider">同步内核配置中...</span>
      </div>

      <div
        class="bg-white dark:bg-[#1e293b] p-6 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm space-y-5 h-full transition-all flex flex-col">
        <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
          <OptionsOutline class="w-4 h-4 text-accent" />
          {{ t('config.general_settings') }}
        </h4>

        <div class="flex items-center justify-between">
          <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.allow_lan') }}</label>
          <button @click="configs['allow-lan'] = !configs['allow-lan']; toggleAllowLan()"
            class="w-10 h-6 flex items-center rounded-full p-0.5 transition-all"
            :class="configs['allow-lan'] ? 'bg-accent justify-end' : 'bg-slate-200 dark:bg-slate-700 justify-start'">
            <span class="w-5 h-5 rounded-full bg-white shadow-md"></span>
          </button>
        </div>

        <div class="flex items-center justify-between">
          <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.ipv6_toggle') }}</label>
          <button @click="configs.ipv6 = !configs.ipv6; toggleIPv6()"
            class="w-10 h-6 flex items-center rounded-full p-0.5 transition-all"
            :class="configs.ipv6 ? 'bg-accent justify-end' : 'bg-slate-200 dark:bg-slate-700 justify-start'">
            <span class="w-5 h-5 rounded-full bg-white shadow-md"></span>
          </button>
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

      <div
        class="bg-white dark:bg-[#1e293b] p-6 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm space-y-5 h-full transition-all flex flex-col">
        <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
          <HardwareChipOutline class="w-4 h-4 text-accent" />
          {{ t('config.port_settings') }}
        </h4>

        <div class="grid grid-cols-2 gap-4">
          <div class="flex flex-col gap-1">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.mixed_port') }}</label>
            <input type="number" v-model="mixedPortInput" @blur="savePorts" @keyup.enter="savePorts" :placeholder="t('config.port_disabled_hint')"
              class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full" />
          </div>
          <div class="flex flex-col gap-1">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.http_port') }}</label>
            <input type="number" v-model="portInput" @blur="savePorts" @keyup.enter="savePorts" :placeholder="t('config.port_disabled_hint')"
              class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full" />
          </div>
          <div class="flex flex-col gap-1">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.socks_port') }}</label>
            <input type="number" v-model="socksPortInput" @blur="savePorts" @keyup.enter="savePorts" :placeholder="t('config.port_disabled_hint')"
              class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full" />
          </div>
          <div class="flex flex-col gap-1">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.redir_port') }}</label>
            <input type="number" v-model="redirPortInput" @blur="savePorts" @keyup.enter="savePorts" :placeholder="t('config.port_disabled_hint')"
              class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full" />
          </div>
          <div class="flex flex-col gap-1 col-span-2">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.tproxy_port')
              }}</label>
            <input type="number" v-model="tproxyPortInput" @blur="savePorts" @keyup.enter="savePorts" :placeholder="t('config.port_disabled_hint')"
              class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full" />
          </div>
        </div>
      </div>

      <div
        class="bg-white dark:bg-[#1e293b] p-6 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm space-y-5 h-full transition-all flex flex-col">
        <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
          <ShieldCheckmarkOutline class="w-4 h-4 text-accent" />
          {{ t('config.tun') }}
        </h4>

        <div class="flex items-center justify-between">
          <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.tun_enable') }}</label>
          <button @click="configs.tun.enable = !configs.tun.enable; saveTun()"
            class="w-10 h-6 flex items-center rounded-full p-0.5 transition-all"
            :class="configs.tun.enable ? 'bg-accent justify-end' : 'bg-slate-200 dark:bg-slate-700 justify-start'">
            <span class="w-5 h-5 rounded-full bg-white shadow-md"></span>
          </button>
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

        <!-- 分割线 -->
        <div class="h-px bg-slate-100 dark:bg-slate-800"></div>

        <div class="flex flex-col gap-1">
          <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.interface_name') }}</label>
          <select v-model="configs['interface-name']" @change="saveInterface"
            class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full">
            <option value="">{{ t('config.interface_name_auto') }}</option>
            <option v-for="iface in interfaces" :key="iface" :value="iface">{{ iface }}</option>
          </select>
        </div>
      </div>
    </div>

    <!-- 始终展示的控制与维护区 -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      <!-- 运维控制 -->
      <div
        class="bg-white dark:bg-[#1e293b] p-6 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm space-y-5 h-full transition-all flex flex-col">
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-slate-100 dark:border-slate-800 pb-4">
          <h4 class="font-bold text-sm flex items-center gap-2">
            <BuildOutline class="w-4 h-4 text-accent" />
            {{ t('config.advanced_maintenance') }}
          </h4>
          
          <div class="flex items-center gap-3 text-xs">
            <span class="w-2.5 h-2.5 rounded-full flex shrink-0"
              :class="coreStatus.loading ? 'bg-slate-400 animate-pulse' : (coreStatus.running ? 'bg-success shadow-lg shadow-success/30' : 'bg-red-500 shadow-lg shadow-red-500/30')"></span>
            <span class="font-bold text-slate-700 dark:text-slate-300">
              {{ coreStatus.running ? t('config.core_running') : t('config.core_stopped') }}
            </span>
            <span v-if="coreStatus.running && stats.coreVersion !== '未知' && stats.coreVersion !== '加载中...'"
              class="px-2 py-0.5 font-semibold bg-slate-100 dark:bg-slate-800 text-slate-500 rounded-md">
              {{ coreVersion }}
            </span>
          </div>
        </div>

        <div class="space-y-4 flex-1 flex flex-col justify-between">
          <!-- 内核核心控制 -->
          <div class="flex flex-wrap gap-3">
            <button v-if="!coreStatus.running" @click="handleStartCore" :disabled="coreStatus.loading"
              class="px-4 py-2 bg-success hover:bg-success-hover text-white text-xs font-semibold rounded-xl shadow-md shadow-success/15 hover:shadow-success/25 transition-all flex items-center gap-1.5">
              <RefreshOutline v-if="coreStatus.loading" class="w-3.5 h-3.5 animate-spin" />
              {{ coreStatus.loading ? t('config.core_starting') : t('config.start_core') }}
            </button>
            <template v-else>
              <button @click="handleStopCore" :disabled="coreStatus.loading"
                class="px-4 py-2 bg-red-500 hover:bg-red-600 text-white text-xs font-semibold rounded-xl shadow-md shadow-red-500/15 hover:shadow-red-500/25 transition-all flex items-center gap-1.5">
                <RefreshOutline v-if="coreStatus.loading" class="w-3.5 h-3.5 animate-spin" />
                {{ coreStatus.loading ? t('config.core_stopping') : t('config.stop_core') }}
              </button>
              <button @click="handleRestartCore" :disabled="coreStatus.loading"
                class="px-4 py-2 bg-amber-500 hover:bg-amber-600 text-white text-xs font-semibold rounded-xl shadow-md shadow-amber-500/15 hover:shadow-amber-500/25 transition-all">
                {{ t('config.restart') }}
              </button>
            </template>
            <button @click="handleUpgradeCore" :disabled="isUpgrading"
              class="px-4 py-2 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-xl shadow-md shadow-accent/15 hover:shadow-accent/25 transition-all flex items-center gap-1">
              <CloudDownloadOutline class="w-3.5 h-3.5" :class="{ 'animate-spin': isUpgrading }" />
              {{ t('config.upgrade_core') }}
            </button>
          </div>

          <!-- 分割线 -->
          <div class="h-px bg-slate-100 dark:bg-slate-800"></div>

          <!-- 常规运维 -->
          <div class="grid grid-cols-2 gap-3">
            <button @click="handleReloadConfig" :disabled="!coreStatus.running"
              class="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-xl text-slate-700 dark:text-slate-200 transition-all border border-slate-200/20 disabled:opacity-50">
              {{ t('config.reload') }}
            </button>
            <button @click="handleFlushFakeIP" :disabled="!coreStatus.running"
              class="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-xl text-slate-700 dark:text-slate-200 transition-all border border-slate-200/20 disabled:opacity-50">
              {{ t('config.flush_fakeip') }}
            </button>
            <button @click="handleFlushDNS" :disabled="!coreStatus.running"
              class="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-xl text-slate-700 dark:text-slate-200 transition-all border border-slate-200/20 disabled:opacity-50">
              {{ t('config.flush_dns') }}
            </button>
            <button @click="handleUpdateGeo" :disabled="!coreStatus.running"
              class="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-xl text-slate-700 dark:text-slate-200 transition-all border border-slate-200/20 disabled:opacity-50">
              {{ t('config.update_geo') }}
            </button>
          </div>
        </div>
      </div>

      <!-- 界面设置 -->
      <div
        class="bg-white dark:bg-[#1e293b] p-6 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm space-y-5 h-full transition-all flex flex-col">
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
              <option value="system">{{ t('config.theme_system') }}</option>
            </select>
          </div>
        </div>
      </div>

      <!-- DNS 查询卡片，在内核启动时单独显示 -->
      <div v-if="coreStatus.running"
        class="bg-white dark:bg-[#1e293b] p-6 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm space-y-4 h-full transition-all flex flex-col">
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
            class="p-4 bg-slate-950 font-mono text-xs rounded-xl overflow-x-auto h-28 border border-slate-800 transition-all flex-1"
            :class="dnsQuery.result ? 'text-emerald-400' : 'text-slate-500 italic flex items-center justify-center select-none'">{{ dnsQuery.result || t('config.dns_result_default') }}</pre>
        </div>
      </div>
    </div>
  </div>
</template>
