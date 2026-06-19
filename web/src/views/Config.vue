<script setup lang="ts">
<<<<<<< HEAD
import { ref, computed, onMounted, onUnmounted } from 'vue'
=======
import { ref, onMounted, onUnmounted } from 'vue'
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
import { useI18n } from 'vue-i18n'
import { apiFetch } from '../utils/api'
import { CloudDownloadOutline, OptionsOutline, HardwareChipOutline, ShieldCheckmarkOutline, BuildOutline, SearchOutline } from '@vicons/ionicons5'
import { useGlobalStore } from '../store/global'
import { storeToRefs } from 'pinia'
import { useConfigStore, type ConfigData } from '../store/config'
<<<<<<< HEAD
import { useOverviewStore } from '../store/overview'
=======
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69

const { t } = useI18n()
const globalStore = useGlobalStore()
const configStore = useConfigStore()
<<<<<<< HEAD
const overviewStore = useOverviewStore()
const { coreStatus, configs, configsLoading } = storeToRefs(configStore)
const { stats } = storeToRefs(overviewStore)
=======
const { coreStatus, configs } = storeToRefs(configStore)
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69

const fetchCoreStatus = configStore.fetchCoreStatus
const fetchConfigs = configStore.fetchConfigs

<<<<<<< HEAD
const coreVersion = computed(() => {
  if (stats.value.coreVersion === '加载中...') return t('common.loading')
  if (stats.value.coreVersion === '未知') return ''
  return 'v' + stats.value.coreVersion
})

=======
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
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
<<<<<<< HEAD
  const getPortVal = (val: any) => {
    const p = parseInt(val)
    return isNaN(p) ? 0 : p
  }

  const port = getPortVal(configs.value.port)
  const socksPort = getPortVal(configs.value['socks-port'])
  const redirPort = getPortVal(configs.value['redir-port'])
  const tproxyPort = getPortVal(configs.value['tproxy-port'])
  const mixedPort = getPortVal(configs.value['mixed-port'])

  const ports = [port, socksPort, redirPort, tproxyPort, mixedPort]

  for (const p of ports) {
    if (p !== 0 && (p < 1025 || p > 65535)) {
=======
  const ports = [
    configs.value.port,
    configs.value['socks-port'],
    configs.value['redir-port'],
    configs.value['tproxy-port'],
    configs.value['mixed-port']
  ]

  for (const p of ports) {
    if (p !== undefined && p !== null && p !== 0 && (p < 1025 || p > 65535)) {
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
      globalStore.showToast(t('config.port_invalid_hint'), 'error')
      return
    }
  }

<<<<<<< HEAD
  const activePorts = ports.filter(p => p !== 0)
=======
  const activePorts = ports.filter(p => p && p !== 0)
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
  if (new Set(activePorts).size !== activePorts.length) {
    globalStore.showToast(t('config.port_duplicate_hint'), 'error')
    return
  }

  patchConfig({
<<<<<<< HEAD
    port,
    'socks-port': socksPort,
    'redir-port': redirPort,
    'tproxy-port': tproxyPort,
    'mixed-port': mixedPort
=======
    port: configs.value.port,
    'socks-port': configs.value['socks-port'],
    'redir-port': configs.value['redir-port'],
    'tproxy-port': configs.value['tproxy-port'],
    'mixed-port': configs.value['mixed-port']
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
  })
  globalStore.showToast(t('common.success'), 'success')
}

const saveTun = () => {
  patchConfig({ tun: configs.value.tun })
  globalStore.showToast(t('common.success'), 'success')
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
<<<<<<< HEAD
      setTimeout(() => {
        fetchConfigs()
        overviewStore.fetchVersionAndStatus()
      }, 1500)
=======
      setTimeout(fetchConfigs, 1500)
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
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
<<<<<<< HEAD
  const ok = await globalStore.showConfirm(t('config.confirm_stop_core'))
  if (!ok) return
=======
  if (!confirm(t('config.confirm_stop_core'))) return
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
  coreStatus.value.loading = true
  try {
    const resp = await apiFetch('/core/stop', { method: 'POST' })
    const data = await resp.json()
    if (resp.ok && data.status === 'ok') {
      globalStore.showToast(t('config.core_stopped_success'), 'success')
      fetchCoreStatus()
<<<<<<< HEAD
      configsLoading.value = true
=======
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
      configs.value = {
        'allow-lan': false,
        ipv6: false,
        mode: 'Rule',
        'log-level': 'silent',
        'interface-name': '',
<<<<<<< HEAD
        tun: { enable: false, stack: 'System', device: 'utun' },
=======
        tun: { enable: false, stack: 'gvisor', device: 'utun' },
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
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
<<<<<<< HEAD
  const ok = await globalStore.showConfirm(t('config.confirm_restart'))
  if (!ok) return
=======
  if (!confirm(t('config.confirm_restart'))) return
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
  try {
    const resp = await apiFetch('/restart', { method: 'POST' })
    if (resp.ok) {
      globalStore.showToast(t('config.restart_sent'), 'success')
<<<<<<< HEAD
      setTimeout(() => {
        fetchConfigs()
        overviewStore.fetchVersionAndStatus()
      }, 1500)
=======
      setTimeout(fetchConfigs, 1500)
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
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
<<<<<<< HEAD
    let resp = await apiFetch('/providers/geo', { method: 'POST' }).catch(() => null)
    if (!resp || !resp.ok) {
      resp = await apiFetch('/configs/geo', { method: 'POST' })
    }
=======
    const resp = await apiFetch('/configs/geo', { method: 'POST' })
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
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
    const data = await resp.json()
    if (resp.ok) {
      globalStore.showToast(t('config.upgrade_success') + ': ' + (data.version || ''), 'success')
<<<<<<< HEAD
      overviewStore.fetchVersionAndStatus()
=======
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
    } else {
      globalStore.showToast(t('config.upgrade_failed') + ': ' + (data.message || ''), 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
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
<<<<<<< HEAD
      if (data.Status === 0 && data.Answer && data.Answer.length > 0) {
        dnsQuery.value.result = data.Answer.map((a: any) => a.data).join('\n')
      } else {
        dnsQuery.value.result = JSON.stringify(data, null, 2)
      }
=======
      dnsQuery.value.result = JSON.stringify(data, null, 2)
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
    } else {
      dnsQuery.value.result = t('config.dns_query_failed')
    }
  } catch (e) {
    dnsQuery.value.result = `${t('config.dns_query_failed')}: ${(e as Error).message}`
  } finally {
    dnsQuery.value.loading = false
  }
}

onMounted(() => {
  fetchCoreStatus()
  fetchConfigs()
  statusTimer.value = setInterval(fetchCoreStatus, 5000)
})

onUnmounted(() => {
  if (statusTimer.value) clearInterval(statusTimer.value)
})
</script>

<template>
  <div class="space-y-6 max-w-4xl">
    <div class="bg-white dark:bg-[#1e293b] p-6 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm flex flex-col sm:flex-row sm:items-center justify-between gap-6 transition-all">
      <div class="flex items-center gap-4">
        <span class="w-4.5 h-4.5 rounded-full flex shrink-0" :class="coreStatus.loading ? 'bg-slate-400 animate-pulse' : (coreStatus.running ? 'bg-success shadow-lg shadow-success/40' : 'bg-red-500 shadow-lg shadow-red-500/40')"></span>
        <div>
<<<<<<< HEAD
          <div class="flex items-center gap-2">
            <h3 class="font-bold text-base">{{ coreStatus.running ? t('config.core_running') : t('config.core_stopped') }}</h3>
            <span v-if="coreStatus.running && stats.coreVersion !== '未知' && stats.coreVersion !== '加载中...'" class="px-2 py-0.5 text-[10px] font-semibold bg-slate-100 dark:bg-slate-800 text-slate-500 rounded-md">
              {{ coreVersion }}
            </span>
          </div>
=======
          <h3 class="font-bold text-base">{{ coreStatus.running ? t('config.core_running') : t('config.core_stopped') }}</h3>
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
          <p class="text-xs text-slate-400 dark:text-slate-500 mt-1">{{ t('config.core_status_desc') }}</p>
        </div>
      </div>

      <div class="flex gap-2">
        <button v-if="!coreStatus.running" @click="handleStartCore" :disabled="coreStatus.loading" class="px-4 py-2 bg-success text-white text-xs font-semibold rounded-lg shadow-md shadow-success/15 hover:shadow-success/25 transition-all">
          {{ t('config.start_core') }}
        </button>
        <button v-else @click="handleStopCore" :disabled="coreStatus.loading" class="px-4 py-2 bg-red-500 text-white text-xs font-semibold rounded-lg shadow-md shadow-red-500/15 hover:shadow-red-500/25 transition-all">
          {{ t('config.stop_core') }}
        </button>
        <button @click="handleUpgradeCore" :disabled="isUpgrading" class="px-4 py-2 bg-accent text-white text-xs font-semibold rounded-lg shadow-md shadow-accent/15 hover:shadow-accent/25 transition-all flex items-center gap-1">
          <CloudDownloadOutline class="w-3.5 h-3.5" :class="{ 'animate-spin': isUpgrading }" />
          {{ t('config.upgrade_core') }}
        </button>
      </div>
    </div>

<<<<<<< HEAD
    <div v-if="coreStatus.running" class="grid grid-cols-1 md:grid-cols-2 gap-6 relative">
      <!-- 同步配置遮罩屏 -->
      <div v-if="configsLoading" class="absolute inset-0 bg-white/40 dark:bg-[#1e293b]/45 backdrop-blur-[1.5px] z-30 flex flex-col items-center justify-center rounded-2xl gap-2 select-none border border-slate-200/40 dark:border-slate-800/40 shadow-sm animate-pulse">
        <span class="text-xs font-bold text-slate-500 dark:text-slate-400 tracking-wider">同步内核配置中...</span>
      </div>
=======
    <div v-if="coreStatus.running" class="grid grid-cols-1 md:grid-cols-2 gap-6">
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
      <div class="bg-white dark:bg-[#1e293b] p-6 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm space-y-5 transition-all">
        <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
          <OptionsOutline class="w-4 h-4 text-accent" />
          {{ t('config.general_settings') }}
        </h4>

        <div class="flex items-center justify-between">
          <div>
            <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.allow_lan') }}</label>
            <p class="text-[10px] text-slate-400 dark:text-slate-500 mt-0.5">{{ t('config.allow_lan_desc') }}</p>
          </div>
          <button @click="configs['allow-lan'] = !configs['allow-lan']; toggleAllowLan()" class="w-10 h-6 flex items-center rounded-full p-0.5 transition-all" :class="configs['allow-lan'] ? 'bg-accent justify-end' : 'bg-slate-200 dark:bg-slate-700 justify-start'">
            <span class="w-5 h-5 rounded-full bg-white shadow-md"></span>
          </button>
        </div>

        <div class="flex items-center justify-between">
          <div>
            <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.ipv6_toggle') }}</label>
            <p class="text-[10px] text-slate-400 dark:text-slate-500 mt-0.5">{{ t('config.ipv6_toggle_desc') }}</p>
          </div>
          <button @click="configs.ipv6 = !configs.ipv6; toggleIPv6()" class="w-10 h-6 flex items-center rounded-full p-0.5 transition-all" :class="configs.ipv6 ? 'bg-accent justify-end' : 'bg-slate-200 dark:bg-slate-700 justify-start'">
            <span class="w-5 h-5 rounded-full bg-white shadow-md"></span>
          </button>
        </div>

        <div class="flex flex-col gap-1.5">
          <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.mode') }}</label>
          <select v-model="configs.mode" @change="changeMode" class="px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none">
            <option value="Rule">{{ t('config.mode_rule') }}</option>
            <option value="Global">{{ t('config.mode_global') }}</option>
            <option value="Direct">{{ t('config.mode_direct') }}</option>
          </select>
        </div>

        <div class="flex flex-col gap-1.5">
          <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.log_level') }}</label>
          <select v-model="configs['log-level']" @change="changeLogLevel" class="px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none">
            <option value="silent">Silent</option>
            <option value="info">Info</option>
            <option value="warning">Warning</option>
            <option value="error">Error</option>
            <option value="debug">Debug</option>
          </select>
        </div>

        <div class="flex flex-col gap-1.5">
          <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.interface_name') }}</label>
          <div class="flex gap-2">
            <input type="text" v-model="configs['interface-name']" :placeholder="t('config.interface_name_placeholder')" class="flex-1 px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none" />
            <button @click="saveInterface" class="px-3.5 py-1.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-lg transition-all">{{ t('common.save') }}</button>
          </div>
        </div>
      </div>

      <div class="bg-white dark:bg-[#1e293b] p-6 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm space-y-5 transition-all">
        <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
          <HardwareChipOutline class="w-4 h-4 text-accent" />
          {{ t('config.port_settings') }}
        </h4>

        <div class="grid grid-cols-2 gap-4">
          <div class="flex flex-col gap-1">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.mixed_port') }}</label>
            <input type="number" v-model="configs['mixed-port']" class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none" />
          </div>
          <div class="flex flex-col gap-1">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.http_port') }}</label>
            <input type="number" v-model="configs.port" class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none" />
          </div>
          <div class="flex flex-col gap-1">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.socks_port') }}</label>
            <input type="number" v-model="configs['socks-port']" class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none" />
          </div>
          <div class="flex flex-col gap-1">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.redir_port') }}</label>
            <input type="number" v-model="configs['redir-port']" class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none" />
          </div>
          <div class="flex flex-col gap-1 col-span-2">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.tproxy_port') }}</label>
            <input type="number" v-model="configs['tproxy-port']" class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none" />
          </div>
        </div>

        <div class="flex justify-between items-center border-t border-slate-100 dark:border-slate-800 pt-4">
          <span class="text-[10px] text-slate-400 dark:text-slate-500">{{ t('config.port_disabled_hint') }}</span>
          <button @click="savePorts" class="px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all">{{ t('config.save_ports') }}</button>
        </div>
      </div>

      <div class="bg-white dark:bg-[#1e293b] p-6 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm space-y-5 transition-all">
        <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
          <ShieldCheckmarkOutline class="w-4 h-4 text-accent" />
          {{ t('config.tun') }}
        </h4>

        <div class="flex items-center justify-between">
          <div>
            <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.tun_enable') }}</label>
            <p class="text-[10px] text-slate-400 dark:text-slate-500 mt-0.5">{{ t('config.tun_enable_desc') }}</p>
          </div>
<<<<<<< HEAD
          <button @click="configs.tun.enable = !configs.tun.enable; saveTun()" class="w-10 h-6 flex items-center rounded-full p-0.5 transition-all" :class="configs.tun.enable ? 'bg-accent justify-end' : 'bg-slate-200 dark:bg-slate-700 justify-start'">
=======
          <button @click="configs.tun.enable = !configs.tun.enable" class="w-10 h-6 flex items-center rounded-full p-0.5 transition-all" :class="configs.tun.enable ? 'bg-accent justify-end' : 'bg-slate-200 dark:bg-slate-700 justify-start'">
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
            <span class="w-5 h-5 rounded-full bg-white shadow-md"></span>
          </button>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div class="flex flex-col gap-1">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.tun_stack') }}</label>
            <select v-model="configs.tun.stack" class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none">
<<<<<<< HEAD
              <option value="gVisor">gVisor</option>
              <option value="System">System</option>
              <option value="Mixed">Mixed</option>
=======
              <option value="gvisor">gVisor</option>
              <option value="system">System</option>
              <option value="mixed">Mixed</option>
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
            </select>
          </div>
          <div class="flex flex-col gap-1">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.tun_device') }}</label>
            <input type="text" v-model="configs.tun.device" class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none" />
          </div>
        </div>

        <div class="flex justify-end pt-2 border-t border-slate-100 dark:border-slate-800">
          <button @click="saveTun" class="px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all">{{ t('config.save_tun') }}</button>
        </div>
      </div>

      <div class="bg-white dark:bg-[#1e293b] p-6 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm space-y-4 transition-all">
        <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
          <BuildOutline class="w-4 h-4 text-accent" />
          {{ t('config.advanced_maintenance') }}
        </h4>

        <div class="grid grid-cols-2 gap-3">
          <button @click="handleReloadConfig" class="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-xl text-slate-700 dark:text-slate-200 transition-all border border-slate-200/20">
            {{ t('config.reload') }}
          </button>
          <button @click="handleRestartCore" class="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-xl text-slate-700 dark:text-slate-200 transition-all border border-slate-200/20">
            {{ t('config.restart') }}
          </button>
          <button @click="handleFlushFakeIP" class="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-xl text-slate-700 dark:text-slate-200 transition-all border border-slate-200/20">
            {{ t('config.flush_fakeip') }}
          </button>
          <button @click="handleFlushDNS" class="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-xl text-slate-700 dark:text-slate-200 transition-all border border-slate-200/20">
            {{ t('config.flush_dns') }}
          </button>
          <button @click="handleUpdateGeo" class="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-xl text-slate-700 dark:text-slate-200 transition-all border border-slate-200/20 col-span-2">
            {{ t('config.update_geo') }}
          </button>
        </div>
      </div>

      <div class="bg-white dark:bg-[#1e293b] p-6 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm space-y-4 md:col-span-2 transition-all">
        <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
          <SearchOutline class="w-4 h-4 text-accent" />
          {{ t('config.dns_query') }}
        </h4>

        <div class="flex gap-2">
          <input type="text" v-model="dnsQuery.name" :placeholder="t('config.dns_placeholder')" @keyup.enter="handleDNSQuery" class="flex-1 px-4 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none" />
          <select v-model="dnsQuery.type" class="px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none">
            <option value="A">A</option>
            <option value="AAAA">AAAA</option>
            <option value="MX">MX</option>
            <option value="TXT">TXT</option>
          </select>
          <button @click="handleDNSQuery" :disabled="dnsQuery.loading" class="px-5 py-2 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all">
            {{ dnsQuery.loading ? t('config.dns_querying') : t('config.dns_query_btn') }}
          </button>
        </div>

        <pre v-if="dnsQuery.result" class="p-4 bg-slate-950 text-emerald-400 font-mono text-xs rounded-xl overflow-x-auto max-h-40 border border-slate-800">{{ dnsQuery.result }}</pre>
      </div>
    </div>
  </div>
</template>
