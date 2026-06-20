import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch } from '../utils/api'

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

  // 获取所有规则，支持静默刷新
  const fetchRules = async (silent = false) => {
    if (!silent) isLoadingRules.value = true
    try {
      const resp = await apiFetch('/rules')
      if (resp.ok) {
        const data = await resp.json()
        const list = data.rules || []
        list.forEach((r: any, idx: number) => {
          r.index = idx
          r.enabled = !(r.extra?.disabled === true)
        })
        rules.value = list
      }
    } catch (e) {
      console.error('获取规则失败', e)
    } finally {
      if (!silent) isLoadingRules.value = false
    }
  }

  // 获取规则提供商
  const fetchProviders = async (silent = false) => {
    if (!silent) isLoadingProviders.value = true
    try {
      const resp = await apiFetch('/providers/rules')
      if (resp.ok) {
        const data = await resp.json()
        const providersObj = data.providers || {}
        providers.value = Object.keys(providersObj).map(name => ({
          name,
          ...providersObj[name]
        })) as ProviderItem[]
      }
    } catch (e) {
      console.error('获取规则提供商失败', e)
    } finally {
      if (!silent) isLoadingProviders.value = false
    }
  }

  return {
    rules,
    providers,
    isLoadingRules,
    isLoadingProviders,
    fetchRules,
    fetchProviders
  }
})
