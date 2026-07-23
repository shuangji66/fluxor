// stores/config.ts
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch } from '../utils/api'

export interface TunConfig {
  enable: boolean
  stack: string
  device: string
}

export interface ConfigData {
  'allow-lan': boolean
  ipv6: boolean
  mode: string
  'log-level': string
  'interface-name': string
  tun: TunConfig
  port: number
  'socks-port': number
  'redir-port': number
  'tproxy-port': number
  'mixed-port': number
}

export const useConfigStore = defineStore('config', () => {
  // ---------- 内核配置 ----------
  const configs = ref<ConfigData>({
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
  })

  const configsLoading = ref(true)
  const configsLoaded = ref(false)          // 标记是否已加载过
  let configsFetchPromise: Promise<void> | null = null

  // ---------- TProxy 状态 ----------
  const tproxyEnabled = ref<boolean>(false)
  const tproxyStateLoaded = ref(false)      // 标记 TProxy 状态是否已加载
  let tproxyFetchPromise: Promise<boolean> | null = null

  // ---------- 获取内核配置（缓存 + 防并发） ----------
  const fetchConfigs = async (forceLoading = false) => {
    if (configsLoaded.value && !forceLoading) {
      return
    }
    if (configsFetchPromise) {
      return configsFetchPromise
    }

    const hasData = configs.value.port !== 0 || configs.value['mixed-port'] !== 0
    if (forceLoading || !hasData) {
      configsLoading.value = true
    }

    configsFetchPromise = (async () => {
      try {
        const resp = await apiFetch('/configs')
        if (resp.ok) {
          const data = await resp.json()
          const rawMode = data.mode || 'Rule'
          let normalizedMode = 'Rule'
          if (typeof rawMode === 'string') {
            const m = rawMode.toLowerCase()
            if (m === 'global') normalizedMode = 'Global'
            else if (m === 'direct') normalizedMode = 'Direct'
          }

          const tunData = data.tun || {}
          let normalizedStack = 'System'
          if (tunData.stack) {
            const s = tunData.stack.toLowerCase()
            if (s === 'gvisor') normalizedStack = 'gVisor'
            else if (s === 'mixed') normalizedStack = 'Mixed'
          }

          configs.value = {
            'allow-lan': data['allow-lan'] || false,
            ipv6: data.ipv6 || false,
            mode: normalizedMode,
            'log-level': data['log-level'] || 'silent',
            'interface-name': data['interface-name'] || '',
            tun: {
              enable: tunData.enable || false,
              stack: normalizedStack,
              device: tunData.device || ''
            },
            port: data.port || 0,
            'socks-port': data['socks-port'] || 0,
            'redir-port': data['redir-port'] || 0,
            'tproxy-port': data['tproxy-port'] || 0,
            'mixed-port': data['mixed-port'] || 0
          }
          configsLoaded.value = true
        }
      } catch (e) {
        console.warn('获取内核详细配置失败，可能内核未运行', e)
      } finally {
        configsLoading.value = false
        configsFetchPromise = null
      }
    })()

    return configsFetchPromise
  }

  // ---------- 获取 TProxy 状态（缓存 + 防并发） ----------
  const fetchTproxyState = async (retries = 2) => {
    if (tproxyStateLoaded.value) {
      return true
    }
    if (tproxyFetchPromise) {
      return tproxyFetchPromise
    }

    tproxyFetchPromise = (async () => {
      for (let attempt = 0; attempt <= retries; attempt++) {
        try {
          const resp = await apiFetch('/config/tproxy')
          if (resp.ok) {
            const data = await resp.json()
            tproxyEnabled.value = data.enabled
            tproxyStateLoaded.value = true
            return true
          }
        } catch (e) {
          console.warn(`获取 TProxy 状态失败 (尝试 ${attempt + 1}/${retries + 1})`, e)
          if (attempt < retries) {
            await new Promise(r => setTimeout(r, 300 * (attempt + 1)))
          }
        }
      }
      return false
    })()

    return tproxyFetchPromise
  }

  // ---------- 强制刷新 TProxy 状态（用于操作后） ----------
  const refreshTproxyState = async () => {
    tproxyStateLoaded.value = false
    tproxyFetchPromise = null
    return fetchTproxyState()
  }

  // ---------- 内核运行状态 ----------
  const coreStatus = ref({
    running: false,
    loading: true
  })

  const fetchCoreStatus = async () => {
    try {
      const resp = await apiFetch('/core/status')
      if (resp.ok) {
        const data = await resp.json()
        coreStatus.value.running = data.running
      }
    } catch (e) {
      console.error('获取内核状态失败', e)
    } finally {
      coreStatus.value.loading = false
    }
  }

  const refreshCoreStatus = async () => {
    await fetchCoreStatus()
  }

  return {
    configs,
    configsLoading,
    configsLoaded,
    fetchConfigs,
    tproxyEnabled,
    tproxyStateLoaded,
    fetchTproxyState,
    refreshTproxyState,
    coreStatus,
    refreshCoreStatus,
  }
})