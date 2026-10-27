import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { apiFetch, apiJson } from '../utils/api'
import { useSubscriptionStore } from './subscription'
import { createStaleFlag } from '../utils/staleFlag'

export interface ProxyGroup {
  name: string
  type: string
  now: string
  all: string[]
  hidden?: boolean
}

/**
 * 默认测速地址。
 *
 * 内核的延迟测试要求 `url` 参数是一个会快速返回 204/2xx 的端点，
 * 该地址是**经代理**访问的，因此不能填内网地址或被墙域名——
 * 那样只会让所有节点一起显示「超时」。
 */
export const DEFAULT_DELAY_TEST_URL = 'https://www.gstatic.com/generate_204'

// 旧版本的默认值（http）。历史 localStorage 里若仍是它，读盘时升级为当前的 https 默认值。
const LEGACY_DEFAULT_DELAY_TEST_URL = 'http://www.gstatic.com/generate_204'

/**
 * 归一化用户输入的测速地址。
 *
 * - 空串/纯空白 → 回落到默认地址（用户只是清空了输入框，不该把测速打坏）；
 * - 非 http(s) 的完整 URL → 返回 null，由调用方拒绝保存并提示。
 *
 * 校验不能省：`/proxies/{name}/delay` 与 `/group/{name}/delay` 的 url 由内核解析，
 * 非法值会让每个节点都直接失败，界面上看起来就是「所有节点全部超时」。
 */
export function normalizeDelayTestUrl(raw: unknown): string | null {
  const trimmed = typeof raw === 'string' ? raw.trim() : ''
  if (!trimmed) return DEFAULT_DELAY_TEST_URL
  try {
    const parsed = new URL(trimmed)
    if (parsed.protocol === 'http:' || parsed.protocol === 'https:') return trimmed
  } catch {
    // 落到下方 return null
  }
  return null
}

export const useProxyStore = defineStore('proxies', () => {
  const proxyGroups = ref<ProxyGroup[]>([])
  const delays = ref<Record<string, number>>({})
  const allProxiesRaw = ref<Record<string, any>>({})
  const isLoading = ref(false)
  const expandedState = ref<Record<string, boolean>>({})
  const providerInfos = ref<Record<string, any>>({})

  // 自定义测速地址：单节点 / 单组 / 全部测速三个入口共用这一处设置。
  // 任何一路单独硬编码 URL 都会造成「有的入口走自定义、有的走默认」的错位。
  const testUrl = ref(DEFAULT_DELAY_TEST_URL)
  const getDelayTestUrl = () => testUrl.value

  // needsRefresh：代理数据（代理组、节点、各组的当前选择）可能已过期。
  // 来源是配置页对内核运行态的任何一次重建——重载 config.yaml、启动 / 重启 / 升级内核，
  // 内核都会按磁盘上的配置重新加载，代理组与节点集合随之变化，
  // 而本 store 里还留着那次操作之前的快照。
  //
  // 与规则页同构：重建方只登记标记，由代理页在 KeepAlive 的 onActivated 里消费并静默补拉，
  // 用户不切到代理页就绝不产生请求（标记只存在于内存，刷新页面即清除）。
  // 消费方在补拉失败时会把标记放回去（内核可能尚未就绪），下次切入即自动重试。
  const { markNeedsRefresh, consumeNeedsRefresh } = createStaleFlag()

  // === 策略组链式解析（AGENTS §4.9：解析下沉至 Store 并缓存） ===
  //
  // 节点列表里可能包含策略组（Selector / URLTest）。它们的延迟与元数据并不属于
  // 自身，而要沿 `now` 指针逐级下钻，直到承载流量的真实节点。
  //
  // 该遍历只依赖代理图结构（allProxiesRaw），与 delays 无关；因此在 Store 中以
  // computed 全局缓存一次，模板侧退化为 O(1) 查表。任何 `now` 变更——包括
  // ProxyGroupCard.handleSelectProxy 的乐观更新与失败回滚——都会自动使其失效
  // 重算，无需调用方手工触发，也就不存在"忘记失效"导致解析陈旧的窗口。
  //
  // 深度上限 10 用于防御环状引用（A→B→A）。
  const resolvedNodeName = computed<Record<string, string>>(() => {
    const raw = allProxiesRaw.value
    const map: Record<string, string> = {}
    for (const start of Object.keys(raw)) {
      let current = start
      let node = raw[current]
      let depth = 0
      while (node && (node.type === 'Selector' || node.type === 'URLTest') && node.now && depth < 10) {
        const next = node.now
        if (next === current) break
        current = next
        node = raw[current]
        if (!node) break
        depth++
      }
      map[start] = current
    }
    return map
  })

  // 解析后的真实节点名；图中不存在该节点时回退为原名（与原行为一致）
  const effectiveName = (name: string): string => resolvedNodeName.value[name] ?? name

  // 有效延迟：解析后节点的延迟。读 delays 保持响应性，供模板 O(1) 取用
  const getEffectiveDelay = (name: string): number | undefined => delays.value[effectiveName(name)]

  // 有效节点对象：解析后节点本身（type / udp / xudp / recentColors 均取自它）
  const getEffectiveNode = (name: string): any => allProxiesRaw.value[effectiveName(name)]

  // 获取所有代理组并解析历史测速延迟（实际执行体，外部请走 fetchProxies）
  const runFetchProxies = async (silent = false): Promise<boolean> => {
    if (!silent) isLoading.value = true
    try {
      // 动态获取当前模式
      const subscriptionStore = useSubscriptionStore()
      const mode = subscriptionStore.currentConfig.mode

      let allProxies: Record<string, any> = {}
      let groups: ProxyGroup[] = []   // 改用不同名，避免遮蔽响应式 proxyGroups

      if (mode === 'merge') {
        // 融合模式：从 /providers/proxies 获取数据
        const resp = await apiFetch('/providers/proxies')
        if (!resp.ok) {
          // 非 2xx 不得继续走到下面的赋值，否则会用空数据覆盖上一份快照
          console.error('获取代理失败：HTTP', resp.status)
          return false
        }
        {
          const data = await resp.json()
          const providers = data.providers || {}
          providerInfos.value = {}

          // 1. 收集所有节点（含历史）—— 遍历所有订阅的 proxies
          for (const [providerName, provider] of Object.entries(providers) as any[]) {
            // 提取 provider 的元数据（排除 proxies 字段）
            const { proxies, ...meta } = provider
            providerInfos.value[providerName] = meta

            if (provider.proxies) {
              for (const node of provider.proxies) {
                allProxies[node.name] = node
              }
            }
          }

          // 2. 从 default 中提取代理组（Selector / URLTest / Fallback / LoadBalance）
          const defaultProvider = providers['default']
          if (defaultProvider && defaultProvider.proxies) {
            for (const item of defaultProvider.proxies) {
              if (['Selector', 'URLTest', 'Fallback', 'LoadBalance'].includes(item.type)) {
                if (item.hidden === undefined) item.hidden = false
                groups.push(item)
              }
            }
          }
        }
      } else {
        // 切换模式：保持使用 /proxies
        const resp = await apiFetch('/proxies')
        if (!resp.ok) {
          console.error('获取代理失败：HTTP', resp.status)
          return false
        }
        providerInfos.value = {}
        {
          const data = await resp.json()
          allProxies = data.proxies || {}
          for (const name of Object.keys(allProxies)) {
            const item = allProxies[name]
            if (['Selector', 'URLTest', 'Fallback', 'LoadBalance'].includes(item.type)) {
              if (item.hidden === undefined) item.hidden = false
              groups.push(item)
            }
          }
        }
      }

      // 代理组排序（原有逻辑）
      groups.sort((a, b) => {
        const getPriority = (name: string) => {
          if (name.includes('节点选择')) return 0
          if (name.includes('手动选择')) return 1
          if (name.includes('自动选择')) return 2
          if (name.includes('香港')) return 3
          if (name.includes('新加坡')) return 4
          if (name.includes('日本')) return 5
          if (name.includes('美国')) return 6
          if (name.includes('台湾')) return 7
          return 8
        }
        const aPriority = getPriority(a.name)
        const bPriority = getPriority(b.name)
        if (aPriority !== bPriority) return aPriority - bPriority
        return a.name.localeCompare(b.name)
      })

      // 更新响应式状态
      proxyGroups.value = groups
      allProxiesRaw.value = allProxies

      // 解析延迟和颜色（原有逻辑，遍历 allProxies）
      Object.keys(allProxies).forEach(name => {
        const node = allProxies[name]
        if (node) {
          let latest: number | null = null
          let sortedHist: any[] = []
          if (node.history && node.history.length > 0) {
            sortedHist = [...node.history].sort((a: any, b: any) => new Date(a.time).getTime() - new Date(b.time).getTime())
            const last = sortedHist[sortedHist.length - 1]
            latest = last.delay > 0 ? last.delay : -1
          }
          node.latestDelay = latest

          const { low, mid } = delayThresholds.value
          const histCount = historyCount.value
          node.recentColors = sortedHist.slice(-histCount).map((h: any) => {
            const d = h.delay
            let colorClass = 'bg-slate-200 dark:bg-slate-800'
            if (d === -1) colorClass = 'bg-red-500'
            else if (d === 0) colorClass = 'bg-slate-300 dark:bg-slate-700 animate-pulse'
            else if (d > 0 && d <= low) colorClass = 'bg-success'
            else if (d > low && d <= mid) colorClass = 'bg-amber-500'
            else if (d > mid) colorClass = 'bg-red-400'
            return {
              colorClass,
              title: `${h.time}: ${d > 0 ? d + 'ms' : d === 0 ? 'Testing...' : 'Timeout'}`
            }
          })

          if (latest !== null) {
            delays.value[name] = latest
          }
        }
      })
    } catch (e) {
      // 保留上一份成功快照：网络抖动不应把已渲染的代理列表清空（§4.1 快速加载）。
      // 返回值交给调用方决定是否提示，避免 Store 私自吞掉失败。
      console.error('获取代理失败', e)
      return false
    } finally {
      if (!silent) isLoading.value = false
    }
    return true
  }

  // === 单飞 + 尾随合并（coalescing） ===
  //
  // 批量测速会在每完成一组后触发一次刷新（见 Proxies.vue 的循环），组与组之间
  // 可能相互重叠。若不去重，融合模式下会并发多个 /providers/proxies 全量请求：
  // 单次响应包含所有订阅的全部节点与 history，并发叠加会同时压迫浏览器连接
  // 队列与后端 Unix Socket，且多个响应会互相覆盖解析结果。
  //
  // 策略：
  //  1. 单飞——任一时刻至多一个请求在飞；
  //  2. 尾随合并——飞行期间到来的调用不重复发起，只登记一次「待刷新」，
  //     当前请求落地后补跑一次（若补跑期间又有调用，则继续补跑）。
  //
  // 关键语义：重复调用返回的是**同一个** in-flight promise，而该 promise 会等到
  // 尾随补跑结束才 resolve。因此「await 之后数据一定不早于本次调用」这一保证
  // 依然成立（如 handleSelectProxy 切换后必须看到新选择）。
  let inFlight: Promise<boolean> | null = null
  let pendingRefresh = false

  const fetchProxies = (silent = false): Promise<boolean> => {
    if (inFlight) {
      // 已有请求在飞：不重复发起，仅登记一次尾随刷新
      pendingRefresh = true
      return inFlight
    }
    inFlight = (async () => {
      try {
        let result = await runFetchProxies(silent)
        // 飞行期间若有新的刷新请求，补跑；静默执行避免 loading 反复闪烁
        while (pendingRefresh) {
          pendingRefresh = false
          result = await runFetchProxies(true)
        }
        return result
      } finally {
        inFlight = null
        pendingRefresh = false
      }
    })()
    return inFlight
  }

  /**
   * 把内核测速接口返回的 { 节点名: 延迟 } 写进 delays。
   *
   * 负值/非数字一律记为 -1（超时），0 视为「测速中」——这套映射原先在
   * 「全部测速」（Proxies.vue）与「单组测速」（ProxyGroupCard.vue）里各写了一份，
   * 迟早会有一边漏改。放这里让两个入口共用同一套判定。
   */
  const applyDelayMap = (data: Record<string, unknown>) => {
    for (const [name, raw] of Object.entries(data)) {
      if (typeof raw === 'number' && raw > 0) {
        delays.value[name] = raw
      } else if (raw === 0) {
        delays.value[name] = 0
      } else {
        delays.value[name] = -1
      }
    }
  }

  // 测速单个节点（自动解析策略组）
  const testDelay = async (proxyName: string) => {
    // 动态获取当前模式
    const subscriptionStore = useSubscriptionStore()
    const mode = subscriptionStore.currentConfig.mode
    // 复用 Store 的链式解析结果，避免此处再维护一份相同遍历
    const realName = effectiveName(proxyName)
  
    // 如果实际节点正在测速，跳过重复请求
    if (delays.value[realName] === 0) return
  
    // 标记实际节点为测速中
    delays.value[realName] = 0
  
    try {
      const encoded = encodeURIComponent(realName)
      const url = testUrl.value
      let endpoint: string
  
      if (mode === 'merge') {
        const nodeData = allProxiesRaw.value[realName]
        const providerName = nodeData?.['provider-name'] || ''
        if (!providerName) {
          endpoint = `/proxies/${encoded}/delay?timeout=5000&url=${encodeURIComponent(url)}`
        } else {
          const encodedProvider = encodeURIComponent(providerName)
          endpoint = `/providers/proxies/${encodedProvider}/${encoded}/healthcheck?url=${encodeURIComponent(url)}&timeout=5000`
        }
      } else {
        endpoint = `/proxies/${encoded}/delay?timeout=5000&url=${encodeURIComponent(url)}`
      }
  
      const resp = await apiFetch(endpoint)
      if (resp.ok) {
        const data = await resp.json()
        delays.value[realName] = data.delay ?? data.Delay ?? -1
      } else {
        delays.value[realName] = -1
      }
    } catch (e) {
      delays.value[realName] = -1
    }
  }

  // ===== 持久化设置 =====
  const delayThresholds = ref({
    low: 200,
    mid: 500
  })
  const historyCount = ref(5)
  const sortOrder = ref<'default' | 'name' | 'delay' | 'quality'>('default')
  const qualityScores = ref<Record<string, number>>({})
  const filterRegex = ref('(套餐|过期|流量|剩余|订阅|网站|教程)')
  const autoCloseConnections = ref(true)

  // 加载本地设置
  const loadSettings = () => {
    const stored = localStorage.getItem('proxySettings')
    if (stored) {
      try {
        const parsed = JSON.parse(stored)
        if (parsed.delayThresholds) {
          delayThresholds.value = parsed.delayThresholds
        }
        if (parsed.historyCount) {
          historyCount.value = Math.min(10, Math.max(5, parsed.historyCount))
        }
        if (parsed.sortOrder) {
          sortOrder.value = parsed.sortOrder
        }
        if (parsed.filterRegex !== undefined) {
          filterRegex.value = parsed.filterRegex
        }
        if (parsed.autoCloseConnections !== undefined) {
          autoCloseConnections.value = parsed.autoCloseConnections
        }
        // 旧版本没有该字段；历史数据也可能被手工改坏，一律归一化后再用
        if (parsed.testUrl !== undefined) {
          const stored = normalizeDelayTestUrl(parsed.testUrl) ?? DEFAULT_DELAY_TEST_URL
          testUrl.value = stored === LEGACY_DEFAULT_DELAY_TEST_URL ? DEFAULT_DELAY_TEST_URL : stored
        }
      } catch (e) {}
    }
  }
  loadSettings()

  // 保存所有设置到 localStorage
  function saveAllSettings() {
    localStorage.setItem('proxySettings', JSON.stringify({
      delayThresholds: delayThresholds.value,
      historyCount: historyCount.value,
      sortOrder: sortOrder.value,
      filterRegex: filterRegex.value,
      autoCloseConnections: autoCloseConnections.value,
      testUrl: testUrl.value,
    }))
  }

  // 设置测速地址（调用方负责先过 normalizeDelayTestUrl，非法值不会走到这里）
  function setTestUrl(url: string) {
    testUrl.value = normalizeDelayTestUrl(url) ?? DEFAULT_DELAY_TEST_URL
    saveAllSettings()
  }

  // 新增：单独设置自动断开连接开关
  function setAutoCloseConnections(value: boolean) {
    autoCloseConnections.value = value
    saveAllSettings()
  }

  // 设置过滤正则
  function setFilterRegex(regex: string) {
    filterRegex.value = regex
    saveAllSettings()
  }

  // 更新阈值和历史条数
  function updateSettings(thresholds: { low: number, mid: number }, count: number) {
    delayThresholds.value = { low: thresholds.low, mid: thresholds.mid }
    historyCount.value = Math.min(10, Math.max(5, count))
    saveAllSettings()
    fetchProxies(true)
  }

  // 设置排序方式
  function setSortOrder(order: 'default' | 'name' | 'delay' | 'quality') {
    sortOrder.value = order
    saveAllSettings()
    // 如果排序为 quality，主动获取分数；否则清空
    if (order === 'quality') {
      fetchQualityScores()
    } else {
      qualityScores.value = {}
    }
  }

  // 获取质量分数
  const fetchQualityScores = async () => {
    try {
      // 非 2xx 由 apiJson 抛出（含后端 message），下面统一记日志
      const data = await apiJson<Record<string, { score?: number }>>('/proxies/quality')
      const scores: Record<string, number> = {}
      for (const [name, val] of Object.entries(data)) {
        scores[name] = val?.score || 0
      }
      qualityScores.value = scores
    } catch (e) {
      console.warn('获取质量分数失败', e)
    }
  }

  // 按需补齐质量分数。
  // sortOrder 持久化在 localStorage，而质量分数是内核测速历史的派生结果、不随之持久化：
  // 重进前端后若仍按质量排序，必须重新拉取一次，否则分数全空——列表会退化为按名称
  // 兜底排序（见 ProxyGroupCard 中 sb - sa === 0 的分支），节点上的分数徽标也一并消失。
  // 已有分数时不重复请求，避免每次切回本页都打一次内核；强制重算请直接用 fetchQualityScores。
  const ensureQualityScores = async () => {
    if (sortOrder.value !== 'quality') return
    if (Object.keys(qualityScores.value).length > 0) return
    await fetchQualityScores()
  }

  return {
    proxyGroups,
    delays,
    allProxiesRaw,
    isLoading,
    expandedState,
    fetchProxies,
    testDelay,
    applyDelayMap,
    // 策略组链式解析（解析结果由 Store 统一缓存，组件只读）
    getEffectiveDelay,
    getEffectiveNode,
    sortOrder,
    setSortOrder,
    delayThresholds,
    historyCount,
    updateSettings,
    qualityScores,
    fetchQualityScores,
    ensureQualityScores,
    filterRegex,
    setFilterRegex,
    autoCloseConnections,
    setAutoCloseConnections,
    // 测速地址：单节点（本 Store）/ 单组（ProxyGroupCard）/ 全部（Proxies.vue）共用
    testUrl,
    getDelayTestUrl,
    setTestUrl,
    providerInfos,
    // 跨页「代理数据已过期」标记：登记方为配置页的内核运行态操作，消费方为代理页 onActivated
    markNeedsRefresh,
    consumeNeedsRefresh,
  }
})