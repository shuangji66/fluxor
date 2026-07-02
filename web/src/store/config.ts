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
  // 内核配置参数
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

  // 内核配置同步加载状态
  const configsLoading = ref(true)

  // 独立的 TProxy 开关状态（系统级）
  const tproxyEnabled = ref<boolean>(false)
  const tproxyStateLoaded = ref(false)

  const fetchTproxyState = async (retries = 2) => {
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
  }

  // 获取内核详细配置
  const fetchConfigs = async (forceLoading = false) => {
    const hasData = configs.value.port !== 0 || configs.value['mixed-port'] !== 0
    if (forceLoading || !hasData) {
      configsLoading.value = true
    }
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
        configsLoading.value = false
      }
    } catch (e) {
      console.warn('获取内核详细配置失败，可能内核未运行', e)
    }
  }

  // ===== 新增 coreStatus =====
  const coreStatus = ref({
    running: false,
    loading: true
  })

  // 订阅计数
  const coreStatusSubscribers = ref(0)
  let coreStatusTimer: any = null

  // 获取内核状态（单次请求）
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

  // 订阅：启动轮询
  const subscribeCoreStatus = () => {
    coreStatusSubscribers.value++
    if (coreStatusSubscribers.value === 1) {
      // 首次订阅立即获取
      fetchCoreStatus()
      // 启动定时器（5秒轮询）
      coreStatusTimer = setInterval(fetchCoreStatus, 5000)
    }
  }

  // 取消订阅：停止轮询
  const unsubscribeCoreStatus = () => {
    coreStatusSubscribers.value = Math.max(0, coreStatusSubscribers.value - 1)
    if (coreStatusSubscribers.value === 0 && coreStatusTimer) {
      clearInterval(coreStatusTimer)
      coreStatusTimer = null
    }
  }

  // 手动刷新（供页面操作后调用，如启动/停止内核）
  const refreshCoreStatus = async () => {
    await fetchCoreStatus()
  }

  return {
    configs,
    configsLoading,
    fetchConfigs,
    tproxyEnabled,
    tproxyStateLoaded,
    fetchTproxyState,
    coreStatus,
    subscribeCoreStatus,
    unsubscribeCoreStatus,
    refreshCoreStatus,
  }
})