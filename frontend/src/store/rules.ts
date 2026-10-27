import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiJson } from '../utils/api'
import type { RulesFrame, RulesProvidersFrame } from '../types/api'
import { createStaleFlag } from '../utils/staleFlag'

export interface RuleItem {
  type: string
  payload: string
  proxy: string
  enabled: boolean
  index?: number
}

export interface ProviderItem {
  name: string
  type: string
  ruleCount: number
  updatedAt: string
}

export const useRulesStore = defineStore('rules', () => {
  const rules = ref<RuleItem[]>([])
  const providers = ref<ProviderItem[]>([])
  const isLoadingRules = ref(false)
  const isLoadingProviders = ref(false)

  // needsRefresh：规则页数据（规则列表 + 规则提供商）可能已过期，两种来源：
  //   1. 激活订阅的自定义规则改动——重写 config.yaml 并重载内核，运行中的规则集合随之变化；
  //   2. 配置页对内核运行态的重建——重载 config.yaml、启动 / 重启 / 升级内核，内核会按磁盘上的
  //      配置重新加载，规则集合与 rule-providers 都可能与前端快照不同。
  // 两种来源都只登记标记，由规则页在切入时消费（见 rulesStore.markNeedsRefresh 的调用方）。
  const { needsRefresh, markNeedsRefresh, consumeNeedsRefresh } = createStaleFlag()

  // 获取所有规则，支持静默刷新。返回是否成功，交由调用方决定如何提示。
  const fetchRules = async (silent = false) => {
    if (!silent) isLoadingRules.value = true
    try {
      // 非 2xx 由 apiJson 抛出（含后端 message），统一落到下方 catch
      const data = await apiJson<RulesFrame>('/rules')
      // 内核会附带 extra/index 等前端未建模的字段，这里保留其扩展性
      const list: any[] = data.rules || []
      list.forEach((r: any, idx: number) => {
        // 内核 /rules 已下发权威 index；仅在缺失时才回退到数组下标。
        // 直接写死 idx 会在前端做过任何过滤/重排后与内核错位，
        // 导致 /rules/disable 关掉错误的规则。
        if (typeof r.index !== 'number') r.index = idx
        r.enabled = !(r.extra?.disabled === true)
      })
      rules.value = list
      return true
    } catch (e) {
      // 保留上一份快照，避免网络抖动清空列表
      console.error('获取规则失败', e)
      return false
    } finally {
      if (!silent) isLoadingRules.value = false
    }
  }

  // 获取规则提供商。返回是否成功，交由调用方决定如何提示。
  const fetchProviders = async (silent = false) => {
    if (!silent) isLoadingProviders.value = true
    try {
      // 非 2xx 由 apiJson 抛出（含后端 message），统一落到下方 catch
      const data = await apiJson<RulesProvidersFrame>('/providers/rules')
      const providersObj = data.providers || {}
      providers.value = Object.keys(providersObj).map(name => ({
        name,
        ...providersObj[name]
      })) as ProviderItem[]
      return true
    } catch (e) {
      console.error('获取规则提供商失败', e)
      return false
    } finally {
      if (!silent) isLoadingProviders.value = false
    }
  }

  return {
    rules,
    providers,
    isLoadingRules,
    isLoadingProviders,
    needsRefresh,
    markNeedsRefresh,
    consumeNeedsRefresh,
    fetchRules,
    fetchProviders
  }
})
