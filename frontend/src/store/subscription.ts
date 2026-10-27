// stores/subscription.ts
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch, apiJson } from '../utils/api'

export interface SubscriptionInfo {
  upload: number
  download: number
  total: number
  expire: number
  updatedAt: string | null
}

/**
 * 切换模式下挂在某个订阅上的单条自定义规则。
 *
 * 与后端 config.CustomRule 一一对应：存的是结构化字段而非拼好的规则文本，
 * 规则行由后端按内核语法组装（用户输入的逗号/空格无法破坏规则行结构）。
 */
export interface CustomRule {
  id: string
  type: string          // 规则类型，如 'DOMAIN-SUFFIX'
  payload: string       // 规则载荷，如 'example.com'
  target: string        // 该订阅的代理组/代理节点名，或 'DIRECT' | 'REJECT' | 'PASS'
  position: string      // 'before'（默认，插在最前）| 'after'（插在最后一条 MATCH 之前）
  no_resolve?: boolean
  // 以下两个字段仅由后端在响应中返回，请求体不需要
  line: string          // 后端组装好的规则行，用于列表展示
  valid: boolean
  reason?: string       // valid=false 时的原因
}

/** 内核支持的规则类型白名单及其载荷示例（示例用作输入框 placeholder）。 */
export interface RuleTypeSpec {
  type: string          // 'DOMAIN' | 'DOMAIN-SUFFIX' | ... | 'RULE-SET'
  example: string       // 载荷示例
  no_resolve: boolean   // 该类型是否支持 no-resolve 选项
}

/**
 * 单条流量隧道（config.yaml 顶层 tunnels 块的一项）。
 *
 * 与后端 config.Tunnel 一一对应：network 是 tcp/udp 的列表，address/target 为 host:port，
 * proxy 留空表示不指定（内核按正常规则匹配选择出口，因此「关闭」不等于「直连」）。
 */
export interface Tunnel {
  id: string
  network: string[]     // ['tcp','udp'] | ['tcp'] | ['udp']
  address: string       // 本地监听地址，如 127.0.0.1:6553
  target: string        // 转发目标地址，如 8.8.8.8:53
  proxy?: string        // 可选的代理组/代理节点名，必须在当前配置里存在
  enabled: boolean      // 启停开关：关闭的隧道不写进 config.yaml
}

/** 隧道视图：后端附带单行展示形式与合法性判定（前端不复刻校验逻辑）。 */
export interface TunnelView extends Tunnel {
  line: string          // 单行展示形式：network,address,target[,proxy]
  valid: boolean
  reason?: string       // valid=false 时的原因（proxy 不存在、地址非法、与另一条冲突…）
}

/** 规则与隧道接口的统一响应体：两者服务同一个作用域，响应必须同构。 */
export interface CustomRulesPayload {
  file_ready: boolean     // 订阅原始文件是否已下载（false 时无法校验规则与隧道的 proxy）
  rules: CustomRule[]     // 已按生效顺序返回（before 组在前、after 组在后），前端原样渲染，勿再排序/分组
  tunnels: TunnelView[]   // 该作用域的流量隧道，顺序即写入 config.yaml 的顺序
  groups: string[]        // 可选目标：代理组（订阅自带或档位模板），不含代理节点
  nodes: string[]         // 可选目标：自定义模式下的手工节点名（其余模式为空数组）
  builtins: string[]      // ['DIRECT','REJECT','PASS']
  providers: string[]     // 该订阅 rule-providers 的键，供 RULE-SET 选择
  rule_types: RuleTypeSpec[]
  status?: 'ok' | 'warning'
  message?: string
}

export interface SubscriptionItem {
  name: string
  url: string
  update_interval: number
  health_interval: number
  prefix: string
  info?: SubscriptionInfo | null
  // 订阅级自定义规则（切换模式）与流量隧道。
  //
  // 这两个字段**由后端持有**（rules.json / tunnels.json），只出现在 GET 响应里供展示：
  // 它们的增删改走各自的专用接口（写一条保存一条），「保存并应用」不会提交它们
  // （见 SettingsPayload）。因此前端不要把它们的正确性当成自己的责任。
  custom_rules?: CustomRule[]
  tunnels?: Tunnel[]
}

/** 自定义协议字段的取值类型（与后端 nodespec.Kind 一一对应）。 */
export type NodeFieldKind = 'string' | 'text' | 'int' | 'bool' | 'select' | 'list' | 'map' | 'group'

/** 字段所属分区（与后端 nodespec.Section* 一一对应），决定渲染在哪个折叠区。 */
export type NodeFieldSection = '' | 'transport' | 'tls' | 'advanced'

/** 条件显示：同层字段 key 的取值命中 values 时才渲染（如 ws-opts 只在 network=ws 时出现）。 */
export interface NodeVisibleWhen {
  key: string
  values: string[]
}

/**
 * 单个协议字段的声明，由后端 /subscribe/node-protocols 下发。
 *
 * 前端的动态表单完全按它渲染：字段集合、默认值、可选值、是否必填、分区与条件显示
 * 都不在前端复刻，后端为某协议增补默认模板后前端自动跟随（label 是英文兜底文案，
 * 中文走 i18n，必要时可用 subscription.node_field.<协议>.<键> 覆盖）。
 */
export interface NodeFieldSpec {
  key: string
  label: string
  kind: NodeFieldKind
  default?: unknown
  options?: string[]
  required?: boolean
  secret?: boolean
  /** 所属折叠区（空 = 基础项） */
  section?: NodeFieldSection
  /** kind=group 时的子字段（递归同构） */
  children?: NodeFieldSpec[]
  /** 条件显示（仅同层字段可引用） */
  visible_when?: NodeVisibleWhen
  /** 内核解码器要求必须存在的键：即使取零值也要写进 config.yaml。 */
  always?: boolean
}

/** 协议声明：type 为内核 type 取值；require_any 是「至少满足一组」的字段组合（组内需同时填写）。 */
export interface NodeProtocolSpec {
  type: string
  name: string
  fields: NodeFieldSpec[]
  require_any?: string[][]
  /** 后端标记的过时协议（如 ShadowsocksR）：界面给提示，但不阻止保存 */
  deprecated?: boolean
  /** 后端标记的隧道类协议（WireGuard / EasyTier / OpenVPN…）：提示建议搭配自定义规则 */
  tunnel?: boolean
}

/**
 * 自定义模式下手动添加的节点。
 *
 * config 只保存「与协议默认值不同的字段」（后端归一化时剔除默认值），
 * 读取时由后端补齐默认值，因此这里的取值是最新默认值生效后的结果。
 */
export interface CustomNode {
  id: string
  name: string
  type: string
  config?: Record<string, unknown>
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
  // IPv6 总开关与 DNS 返回 AAAA：生成期字段，写入 config.yaml 的 ipv6 / dns.ipv6。
  // 后端会归一化「IPv6 关闭 ⇒ AAAA 关闭」，前端同时做联动以保持界面一致。
  ipv6: boolean
  dns_ipv6: boolean
  // 融合模式按规则集档位存放的自定义规则。
  //
  // 与订阅级规则一样由后端持有（rules.json），只用于展示；保存请求不再提交它
  // （后端即使收到也会忽略，见 SettingsPayload）。
  merge_custom_rules?: Record<string, CustomRule[]>
  subscriptions: SubscriptionItem[]
  // 自定义模式的手工节点列表：由本页面整体覆盖提交，「保存并应用」时才落库生效
  custom_nodes: CustomNode[]
  tproxy_port: number
}

/**
 * 「保存并应用」提交的请求体：只含后端 settings.json 承载的字段。
 *
 * 拆分持久化后，`/subscribe/config` 与 `/subscribe/generate` 只负责**设置**——
 * 全局参数、订阅注册表（名称/链接/间隔/前缀）与自定义模式的手工节点。
 * 规则、隧道、机场元数据分别落在后端的 rules.json / tunnels.json /
 * subscription-meta.json，由各自的专用接口维护（写一条保存一条）：
 * 这两个接口即使收到那些字段也会**直接忽略**，因此前端不再把它们塞进请求体——
 * 既避免「以为保存并应用能持久化规则」的误解，也省掉一份可能很大的元数据。
 *
 * 注意：`delete_physical` 是仅此一次请求有效的临时字段（后端处理完即丢弃），
 * 不属于 settings.json 的持久化内容。
 */
export interface SettingsPayload {
  proxy_port: number
  tproxy_port: number
  panel_port: number
  panel_secret: string
  rule_group: string
  ui_panel: string
  meta_backend_url: string
  mode: string
  active_subscription: string
  ipv6: boolean
  dns_ipv6: boolean
  subscriptions: SettingsSubscription[]
  custom_nodes: CustomNode[]
  delete_physical?: string[]
}

/** 订阅注册表项：只有这些字段属于「设置」；规则/隧道/元数据一概不在其中。 */
export interface SettingsSubscription {
  name: string
  url: string
  update_interval: number
  health_interval: number
  prefix: string
}

/**
 * 由本地配置组装「保存并应用」的请求体。
 *
 * 逐字段挑出设置类字段，而不是 `{...cfg}` 整份铺开：后端已按数据类别拆分存储，
 * 请求体里混入规则/隧道/元数据既无意义，也容易让后来者误以为它们会被保存。
 */
export const buildSettingsPayload = (cfg: SubscriptionConfigData, deletePhysical: string[] = []): SettingsPayload => ({
  proxy_port: cfg.proxy_port,
  tproxy_port: cfg.tproxy_port,
  panel_port: cfg.panel_port,
  panel_secret: cfg.panel_secret,
  rule_group: cfg.rule_group,
  ui_panel: cfg.ui_panel,
  meta_backend_url: cfg.meta_backend_url,
  mode: cfg.mode,
  active_subscription: cfg.active_subscription,
  ipv6: cfg.ipv6,
  dns_ipv6: cfg.dns_ipv6,
  custom_nodes: cfg.custom_nodes || [],
  subscriptions: (cfg.subscriptions || []).map(sub => ({
    name: sub.name,
    url: sub.url,
    update_interval: sub.update_interval,
    health_interval: sub.health_interval,
    prefix: sub.prefix,
  })),
  delete_physical: deletePhysical,
})

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
    ipv6: true,
    dns_ipv6: true,
    active_subscription: '', 
    subscriptions: [],
    custom_nodes: [],
    tproxy_port: 7898
  })

  // 已保存应用的订阅名称白名单
  const savedSubNames = ref<Set<string>>(new Set())

  // 已保存应用（在当前内核配置里生效）的模式。
  //
  // 与 savedSubNames 同理：界面上的模式选择器可以改动而未保存，而后端按**已保存的
  // 模式**校验「规则/隧道作用域入口」的可用性；若按界面上那个未保存的模式去点按钮，
  // 必然被后端按旧模式拒绝（提示「仅在融合模式下可用」而用户其实已经在自定义模式）。
  // 因此模式相关的入口一律以这个值为准。
  const savedMode = ref('merge')

  // 自定义模式可添加的协议与字段表（后端单点维护，前端只按声明渲染表单）。
  // 懒加载一次并缓存：只在用户打开「添加节点」弹窗时才请求。
  const nodeProtocols = ref<NodeProtocolSpec[]>([])
  const isNodeProtocolsLoaded = ref(false)
  let nodeProtocolsPromise: Promise<void> | null = null

  const isConfigLoaded = ref(false)  // 新增：标记是否已加载
  let loadPromise: Promise<void> | null = null  // 新增：防止并发重复请求

  // 获取订阅中心配置（带缓存）
  //
  // 注意：force=false 且已加载过时是**空操作**（直接 return），不会发起请求。
  // 需要拿到最新数据的调用方（保存后刷新、更新轮询等）必须 force=true
  // 或改用 refreshConfig()，否则前端会一直停留在本地旧快照。
  const loadConfig = async (force = false) => {
    // 如果已加载且非强制刷新，直接返回
    if (isConfigLoaded.value && !force) {
      return
    }
    // 如果正在加载中，复用同一个 Promise
    if (loadPromise) {
      return loadPromise
    }

    loadPromise = (async () => {
      try {
        const resp = await apiFetch('/subscribe/config')
        if (resp.ok) {
          const cfg = await resp.json()
          const subs = cfg.subscriptions || []
          savedSubNames.value = new Set(subs.map((s: any) => s.name))
          savedMode.value = cfg.mode || 'merge' 
          currentConfig.value = {
            // 铺开后端返回的原始字段：本地视图需要它们做展示（订阅卡片、规则入口的
            // 可用性判断等）。但保存请求体不由整份视图拼成——见 buildSettingsPayload，
            // 它只挑设置类字段，因此这里多留几个只读字段不会造成「误保存」。
            ...cfg,
            proxy_port: cfg.proxy_port || 7890,
            panel_port: cfg.panel_port || 9090,
            panel_secret: cfg.panel_secret || '',
            rule_group: cfg.rule_group || 'base',
            ui_panel: cfg.ui_panel || 'metacubexd',
            meta_backend_url: cfg.meta_backend_url || '',
            mode: cfg.mode || 'merge',
            active_subscription: cfg.active_subscription || '',
            // 老版本后端没有这两个字段：缺省按「开启」处理，避免升级瞬间被当成关闭
            ipv6: cfg.ipv6 !== false,
            dns_ipv6: cfg.dns_ipv6 !== false,
            tproxy_port: cfg.tproxy_port ?? 7898,
            custom_nodes: cfg.custom_nodes || [],
            subscriptions: subs.map((s: any) => {
              const info = s.subscription_info ? {
                upload: s.subscription_info.upload || 0,
                download: s.subscription_info.download || 0,
                total: s.subscription_info.total || 0,
                expire: s.subscription_info.expire || 0,
                updatedAt: s.updated_at || null,
              } : null
              // 后端把元数据单独存在 subscription-meta.json，GET 时按订阅名合并进来；
              // 本地只保留展示用的 info，不再保留原始两个字段（它们不参与保存）
              const { subscription_info: _info, updated_at: _updatedAt, ...rest } = s
              return { ...rest, info }
            })
          }
          isConfigLoaded.value = true
        }
      } catch (e) {
        console.error('加载订阅配置失败', e)
        throw e
      } finally {
        loadPromise = null
      }
    })()

    return loadPromise
  }

  // 强制刷新配置（用于订阅更新、保存等操作后）
  const refreshConfig = async () => {
    isConfigLoaded.value = false
    await loadConfig(true)
  }

  // 加载可添加的协议与字段表（带缓存；并发调用复用同一个 Promise）
  const loadNodeProtocols = async (force = false) => {
    if (isNodeProtocolsLoaded.value && !force) {
      return
    }
    if (nodeProtocolsPromise) {
      return nodeProtocolsPromise
    }

    nodeProtocolsPromise = (async () => {
      try {
        const data = await apiJson<{ protocols?: typeof nodeProtocols.value }>('/subscribe/node-protocols')
        nodeProtocols.value = data.protocols || []
        isNodeProtocolsLoaded.value = nodeProtocols.value.length > 0
      } catch (e) {
        console.error('加载节点协议列表失败', e)
        throw e
      } finally {
        nodeProtocolsPromise = null
      }
    })()

    return nodeProtocolsPromise
  }

  // 按 type 取协议声明（用于节点卡片展示协议名等）
  const findNodeProtocol = (type: string): NodeProtocolSpec | undefined =>
    nodeProtocols.value.find(p => p.type === type)

  return {
    currentConfig,
    savedSubNames,
    savedMode,
    loadConfig,
    isConfigLoaded,
    refreshConfig,
    nodeProtocols,
    isNodeProtocolsLoaded,
    loadNodeProtocols,
    findNodeProtocol,
  }
})