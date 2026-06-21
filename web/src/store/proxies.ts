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
            if (name.includes('GLOBAL')) return 1
            if (name.includes('手动选择')) return 2
            if (name.includes('电报')) return 3
            if (name.includes('自动选择')) return 4
            if (name.includes('谷歌')) return 5
            if (name.includes('香港')) return 6
            if (name.includes('苹果')) return 7
            if (name.includes('新加坡')) return 8
            if (name.includes('哔哩')) return 9
            if (name.includes('日本')) return 10
            if (name.includes('微软')) return 11
            if (name.includes('美国')) return 12
            if (name.includes('游戏服务')) return 13
            if (name.includes('台湾')) return 14
            return 15
          }
          const aPriority = getPriority(a.name)
          const bPriority = getPriority(b.name)
          if (aPriority !== bPriority) return aPriority - bPriority
          return a.name.localeCompare(b.name)
        })

        proxyGroups.value = groups

        // 解析代理节点最新延迟与近5次历史色块，避免在模板中高频调用 sort 导致性能开销
        Object.keys(data.proxies || {}).forEach(name => {
          const node = data.proxies[name]
          if (node) {
            let latest: number | null = null
            let sortedHist: any[] = []
            if (node.history && node.history.length > 0) {
              sortedHist = [...node.history].sort((a: any, b: any) => new Date(a.time).getTime() - new Date(b.time).getTime())
              const last = sortedHist[sortedHist.length - 1]
              latest = last.delay > 0 ? last.delay : -1
            }
            node.latestDelay = latest
            
            node.recentColors = sortedHist.slice(-5).map((h: any) => {
              const d = h.delay
              let colorClass = 'bg-slate-200 dark:bg-slate-800'
              if (d === 0) colorClass = 'bg-[#1a1a1a]'
              else if (d === -1) colorClass = 'bg-red-500'
              else if (d <= 150) colorClass = 'bg-success'
              else if (d <= 300) colorClass = 'bg-amber-500'
              else colorClass = 'bg-red-400'
              return {
                colorClass,
                title: `${h.time}: ${d > 0 ? d + 'ms' : 'Timeout'}`
              }
            })

            if (latest !== null) {
              delays.value[name] = latest
            }
          }
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
