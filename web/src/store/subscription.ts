// stores/subscription.ts
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch } from '../utils/api'

export interface SubscriptionInfo {
  upload: number
  download: number
  total: number
  expire: number
  updatedAt: string | null
  aliveCount?: number
  totalCount?: number
  avgDelay?: number
}

export interface SubscriptionItem {
  name: string
  url: string
  update_interval: number
  health_interval: number
  prefix: string
  info?: SubscriptionInfo | null
}

export interface SubscriptionConfigData {
  proxy_port: number
  panel_port: number
  panel_secret: string
  rule_group: string
  ui_panel: string
  meta_backend_url: string
  mode: string
  active_subscription: string
  subscriptions: SubscriptionItem[]
  tproxy_port: number
}

export const useSubscriptionStore = defineStore('subscription', () => {
  // 订阅配置参数
  const currentConfig = ref<SubscriptionConfigData>({
    proxy_port: 7890,
    panel_port: 9090,
    panel_secret: '',
    rule_group: 'base',
    ui_panel: 'metacubexd',
    meta_backend_url: '',
    mode: 'merge',
    active_subscription: '', 
    subscriptions: [],
    tproxy_port: 7898
  })

  // 已保存应用的订阅名称白名单
  const savedSubNames = ref<Set<string>>(new Set())

  // 获取订阅中心配置
  const loadConfig = async () => {
    try {
      const resp = await apiFetch('/subscribe/config')
      if (resp.ok) {
        const cfg = await resp.json()
        const subs = cfg.subscriptions || []
        savedSubNames.value = new Set(subs.map((s: any) => s.name))
        currentConfig.value = {
          proxy_port: cfg.proxy_port || 7890,
          panel_port: cfg.panel_port || 9090,
          panel_secret: cfg.panel_secret || '',
          rule_group: cfg.rule_group || 'base',
          ui_panel: cfg.ui_panel || 'metacubexd',
          meta_backend_url: cfg.meta_backend_url || '',
          mode: cfg.mode || 'merge',
          active_subscription: cfg.active_subscription || '',
          tproxy_port: cfg.tproxy_port ?? 7898,
          subscriptions: subs.map((s: any) => {
            // 将后端存储的 subscription_info 映射为前端的 info 对象
            const info = s.subscription_info ? {
              upload: s.subscription_info.upload || 0,
              download: s.subscription_info.download || 0,
              total: s.subscription_info.total || 0,
              expire: s.subscription_info.expire || 0,
              updatedAt: s.updated_at || null,
            } : null
            return {
              ...s,
              info
            }
          })
        }
      }
    } catch (e) {
      console.error('加载订阅配置失败', e)
    }
  }

  // 获取单个订阅详情
  const fetchSubscriptionInfo = async (name: string): Promise<SubscriptionInfo | null> => {
    try {
      const encoded = encodeURIComponent(name)
      const resp = await apiFetch(`/providers/proxies/${encoded}`)
      if (resp.ok) {
        const data = await resp.json()
        const updatedAt = data.updatedAt || null
        const proxiesList = data.proxies || []
        
        let aliveCount = 0
        let totalDelay = 0
        let delayCount = 0
        
        proxiesList.forEach((p: any) => {
          const hasHistory = p.history && p.history.length > 0
          const lastDelay = hasHistory ? p.history[p.history.length - 1].delay : 0
          const isAlive = p.alive === true || lastDelay > 0
          if (isAlive) {
            aliveCount++
            if (lastDelay > 0) {
              totalDelay += lastDelay
              delayCount++
            }
          }
        })

        const totalCount = proxiesList.length
        const avgDelay = delayCount > 0 ? totalDelay / delayCount : undefined

        let info = data.subscriptionInfo || data
        if (info && typeof info === 'object') {
          return {
            upload: info.Upload || 0,
            download: info.Download || 0,
            total: info.Total || 0,
            expire: info.Expire || 0,
            updatedAt,
            aliveCount,
            totalCount,
            avgDelay
          }
        }
      }
    } catch (e) {
      console.warn('获取订阅信息失败', name, e)
    }
    return null
  }

  // 完善订阅项的额外信息（健康度/平均延迟）
  const enrichSubscriptions = async () => {
    const subs = currentConfig.value.subscriptions
    if (!subs) return
    const promises = subs.map(async (sub) => {
      if (savedSubNames.value.has(sub.name)) {
        sub.info = await fetchSubscriptionInfo(sub.name)
      } else {
        sub.info = null
      }
    })
    await Promise.all(promises)
  }

  return {
    currentConfig,
    savedSubNames,
    loadConfig,
    fetchSubscriptionInfo,
    enrichSubscriptions,
  }
})