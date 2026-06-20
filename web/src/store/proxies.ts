import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch } from '../utils/api'

export interface ProxyGroup {
  name: string
  type: string
  now: string
  all: string[]
}

export const useProxyStore = defineStore('proxies', () => {
  const proxyGroups = ref<ProxyGroup[]>([])
  const delays = ref<Record<string, number>>({})
  const allProxiesRaw = ref<Record<string, any>>({})
  const isLoading = ref(false)
  const expandedState = ref<Record<string, boolean>>({})

  // 获取所有代理组并解析历史测速延迟
  const fetchProxies = async (silent = false) => {
    if (!silent) isLoading.value = true
    try {
      const resp = await apiFetch('/proxies')
      if (resp.ok) {
        const data = await resp.json()
        allProxiesRaw.value = data.proxies || {}
        const groups = Object.values(data.proxies || {}).filter((p: any) => 
          p.type === 'Selector' || p.type === 'URLTest' || p.type === 'Fallback' || p.type === 'LoadBalance'
        ) as ProxyGroup[]

        // 排序逻辑
        groups.sort((a, b) => {
          const getPriority = (name: string) => {
            if (name.includes('节点选择')) return 0
            if (name.includes('手动选择')) return 1
            if (name.includes('自动选择')) return 2
            return 3
          }
          const aPriority = getPriority(a.name)
          const bPriority = getPriority(b.name)
          if (aPriority !== bPriority) return aPriority - bPriority
          return a.name.localeCompare(b.name)
        })

        proxyGroups.value = groups

        // 解析代理节点历史延迟
        groups.forEach(g => {
          g.all.forEach(name => {
            if (data.proxies[name] && data.proxies[name].history) {
              const hist = data.proxies[name].history
              if (hist.length > 0) {
                const last = hist[hist.length - 1]
                delays.value[name] = last.delay > 0 ? last.delay : -1
              }
            }
          })
        })
      }
    } catch (e) {
      console.error('获取代理失败', e)
      proxyGroups.value = []
      delays.value = {}
    } finally {
      if (!silent) isLoading.value = false
    }
  }

  // 测速单个节点
  const testDelay = async (proxyName: string) => {
    delays.value[proxyName] = 0 // 测速中标记
    try {
      const encoded = encodeURIComponent(proxyName)
      const url = 'http://www.gstatic.com/generate_204'
      const resp = await apiFetch(`/proxies/${encoded}/delay?timeout=5000&url=${encodeURIComponent(url)}`)
      if (resp.ok) {
        const data = await resp.json()
        delays.value[proxyName] = data.delay
      } else {
        delays.value[proxyName] = -1
      }
    } catch (e) {
      delays.value[proxyName] = -1
    }
  }

  // 限制并发的批量节点测速
  const testProxiesWithConcurrency = async (proxyNames: string[], concurrency = 10) => {
    const queue = [...proxyNames]
    const workers = Array.from({ length: Math.min(concurrency, queue.length) }, async () => {
      while (queue.length > 0) {
        const next = queue.shift()
        if (next) {
          await testDelay(next)
        }
      }
    })
    await Promise.all(workers)
  }

  return {
    proxyGroups,
    delays,
    allProxiesRaw,
    isLoading,
    expandedState,
    fetchProxies,
    testDelay,
    testProxiesWithConcurrency
  }
})
