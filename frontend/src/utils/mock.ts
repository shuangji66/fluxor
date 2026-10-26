import type { CustomRule, CustomRulesPayload, TunnelView } from '../store/subscription'

// 模拟后端数据库，维持状态更改
let coreRunning = true
const mockConfigs = {
  'allow-lan': true,
  ipv6: false,
  mode: 'Rule',
  'log-level': 'info',
  'interface-name': 'eth0',
  tun: { enable: false, stack: 'System', device: '' },
  port: 0,
  'socks-port': 0,
  'redir-port': 0,
  'tproxy-port': 0,
  'mixed-port': 7890
}

const mockSubConfig = {
  proxy_port: 7890,
  panel_port: 9090,
  panel_secret: 'secret123',
  rule_group: 'base',
  ipv6: true,
  dns_ipv6: true,
  ui_panel: 'metacubexd',
  meta_backend_url: '',
  tproxy_port: 7893,
  mode: 'merge',
  // 自定义模式的手工节点（后端按「只存与协议默认值不同的字段」持久化，
  // mock 里直接给出补齐后的取值即可，前端表单按此预填）
  custom_nodes: [],
  subscriptions: [
    {
      name: 'Sub-Mock-01',
      url: 'https://example.com/subscribe',
      update_interval: 3600,
      health_interval: 300,
      prefix: '',
      // 与真机一致：元数据（机场流量/到期）不随订阅注册表返回，而是由后端从
      // subscription-meta.json 合并进 GET 响应；前端再把它组装成展示用的 info
      subscription_info: {
        upload: 1204850123,
        download: 58941094120,
        total: 107374182400,
        expire: Math.floor(Date.now() / 1000) + 864000
      },
      updated_at: new Date().toISOString()
    }
  ]
}

// 后端「设置」类字段：/subscribe/config 与 /subscribe/generate 只承载这些内容
// （settings.json）。规则与隧道分别由 /subscribe/*-custom-rules 与 /subscribe/*-tunnels
// 维护，机场元数据由更新流程写入，mock 里各自独立存放——因此保存请求即使带着过期的
// 规则/隧道/元数据回来也不会覆盖它们，与后端「忽略而非合并」的语义一致。
const MOCK_SETTINGS_KEYS = [
  'proxy_port', 'tproxy_port', 'panel_port', 'panel_secret', 'rule_group',
  'ui_panel', 'meta_backend_url', 'mode', 'active_subscription', 'custom_nodes',
  'ipv6', 'dns_ipv6',
] as const

// 订阅注册表项（settings.json 里的 subscriptions[]）：规则/隧道/元数据都不在其中
const MOCK_SUBSCRIPTION_REGISTRY_KEYS = ['name', 'url', 'update_interval', 'health_interval', 'prefix'] as const

// applyMockSettings 模拟后端 SaveSettings：只取设置类字段，
// 订阅注册表按「注册表字段 + 服务端保留的元数据」重建，并顺带做改名搬迁与孤儿回收。
const applyMockSettings = (payload: any) => {
  for (const key of MOCK_SETTINGS_KEYS) {
    if (key in payload) (mockSubConfig as any)[key] = payload[key]
  }
  if (!Array.isArray(payload.subscriptions)) return

  const prev = new Map<string, any>((mockSubConfig.subscriptions as any[]).map(sub => [sub.name, sub]))
  const renamedFrom = new Map<string, string>() // 新名 → 旧名
  for (const sub of payload.subscriptions) {
    if (prev.has(sub.name)) continue
    // 旧表独有、且 URL 相同 → 视为改名（与后端 detectRenames 的判据一致）
    const candidates = [...prev.values()].filter(prevSub =>
      !payload.subscriptions.some((next: any) => next.name === prevSub.name) && prevSub.url === sub.url)
    if (candidates.length === 1) renamedFrom.set(sub.name, candidates[0].name)
  }

  mockSubConfig.subscriptions = payload.subscriptions.map((sub: any) => {
    const kept: any = {}
    for (const key of MOCK_SUBSCRIPTION_REGISTRY_KEYS) kept[key] = sub[key]
    // 元数据由服务端保留（mock 用同一对键名，等价于 subscription-meta.json）；
    // 改名时从旧名字那一项取，与后端「搬家」的行为一致
    const old = prev.get(renamedFrom.get(sub.name) ?? sub.name)
    if (old?.subscription_info) {
      kept.subscription_info = old.subscription_info
      kept.updated_at = old.updated_at
    }
    return kept
  })

  // 改名：规则与隧道跟着搬家（键都是订阅名）
  for (const [to, from] of renamedFrom) {
    if (mockCustomRulesBySub[from]) {
      mockCustomRulesBySub[to] = mockCustomRulesBySub[from]
      delete mockCustomRulesBySub[from]
    }
    if (mockTunnelsByScope[`sub:${from}`]) {
      mockTunnelsByScope[`sub:${to}`] = mockTunnelsByScope[`sub:${from}`]
      delete mockTunnelsByScope[`sub:${from}`]
    }
  }

  // 孤儿回收：注册表里已不存在的订阅，其规则与隧道一并清掉（后端由 GCResources 负责）
  const names = new Set((mockSubConfig.subscriptions as any[]).map(sub => sub.name))
  for (const name of Object.keys(mockCustomRulesBySub)) {
    if (!names.has(name)) delete mockCustomRulesBySub[name]
  }
  for (const scopeKey of Object.keys(mockTunnelsByScope)) {
    if (scopeKey.startsWith('sub:') && !names.has(scopeKey.slice('sub:'.length))) {
      delete mockTunnelsByScope[scopeKey]
    }
  }
}

// 自定义模式可添加的协议与字段表。
//
// 权威来源是后端 backend/internal/nodespec（新增协议只改后端），此处只保留覆盖
// 各种字段形态（文本/整数/布尔/下拉/列表/多行文本/高级项/必填/二选一）的一小撮协议，
// 供前端离线联调表单渲染用。
const mockNodeProtocols = [
  {
    type: 'http',
    name: 'HTTP',
    fields: [
      { key: 'server', label: 'Server', kind: 'string', required: true },
      { key: 'port', label: 'Port', kind: 'int', required: true },
      { key: 'username', label: 'Username', kind: 'string' },
      { key: 'password', label: 'Password', kind: 'string', secret: true },
      { key: 'tls', label: 'TLS', kind: 'bool' },
      { key: 'headers', label: 'Headers', kind: 'map', section: 'transport' },
      { key: 'sni', label: 'SNI', kind: 'string', section: 'tls' },
      { key: 'skip-cert-verify', label: 'Skip Cert Verify', kind: 'bool', section: 'tls' },
      { key: 'certificate', label: 'Certificate', kind: 'text', section: 'tls' },
      { key: 'ip-version', label: 'IP Version', kind: 'select', default: 'dual', options: ['dual', 'ipv4', 'ipv6'], section: 'advanced' }
    ]
  },
  {
    type: 'ss',
    name: 'Shadowsocks',
    fields: [
      { key: 'server', label: 'Server', kind: 'string', required: true },
      { key: 'port', label: 'Port', kind: 'int', required: true },
      { key: 'cipher', label: 'Cipher', kind: 'select', default: 'aes-256-gcm', options: ['aes-256-gcm', 'chacha20-ietf-poly1305'], required: true },
      { key: 'password', label: 'Password', kind: 'string', secret: true, required: true },
      { key: 'udp', label: 'UDP Relay', kind: 'bool' },
      { key: 'plugin', label: 'Plugin', kind: 'select', options: ['obfs', 'v2ray-plugin', 'shadow-tls'], section: 'transport' },
      { key: 'plugin-opts', label: 'Plugin Options', kind: 'map', section: 'transport' },
      { key: 'client-fingerprint', label: 'Client Fingerprint', kind: 'select', options: ['chrome', 'firefox'], section: 'tls' }
    ]
  },
  {
    // 过时协议在 mock 里也保留一条：离线联调时能验证「仅提示、不阻止保存」
    type: 'ssr',
    name: 'ShadowsocksR',
    deprecated: true,
    fields: [
      { key: 'server', label: 'Server', kind: 'string', required: true },
      { key: 'port', label: 'Port', kind: 'int', required: true },
      { key: 'cipher', label: 'Cipher', kind: 'select', default: 'chacha20-ietf', options: ['chacha20-ietf', 'aes-256-cfb'], required: true },
      { key: 'password', label: 'Password', kind: 'string', secret: true, required: true },
      { key: 'obfs', label: 'Obfs', kind: 'select', default: 'plain', options: ['plain', 'http_simple', 'tls1.2_ticket_auth'], required: true },
      { key: 'protocol', label: 'Protocol', kind: 'select', default: 'origin', options: ['origin', 'auth_sha1_v4', 'auth_aes128_md5'], required: true },
      { key: 'udp', label: 'UDP Relay', kind: 'bool' }
    ]
  },
  {
    type: 'vmess',
    name: 'VMess',
    fields: [
      { key: 'server', label: 'Server', kind: 'string', required: true },
      { key: 'port', label: 'Port', kind: 'int', required: true },
      { key: 'uuid', label: 'UUID', kind: 'string', required: true },
      { key: 'cipher', label: 'Cipher', kind: 'select', default: 'auto', options: ['auto', 'none', 'aes-128-gcm'], required: true },
      { key: 'tls', label: 'TLS', kind: 'bool' },
      { key: 'udp', label: 'UDP Relay', kind: 'bool' },
      { key: 'network', label: 'Network', kind: 'select', options: ['tcp', 'ws', 'h2', 'grpc'], section: 'transport' },
      {
        key: 'ws-opts', label: 'WebSocket', kind: 'group', section: 'transport',
        visible_when: { key: 'network', values: ['ws'] },
        children: [
          { key: 'path', label: 'Path', kind: 'string', default: '/' },
          { key: 'headers', label: 'Headers', kind: 'map' },
          { key: 'max-early-data', label: 'Max Early Data', kind: 'int' },
          { key: 'v2ray-http-upgrade', label: 'V2Ray HTTP Upgrade', kind: 'bool' }
        ]
      },
      {
        key: 'grpc-opts', label: 'gRPC', kind: 'group', section: 'transport',
        visible_when: { key: 'network', values: ['grpc'] },
        children: [
          { key: 'grpc-service-name', label: 'Service Name', kind: 'string' },
          { key: 'max-connections', label: 'Max Connections', kind: 'int' }
        ]
      },
      { key: 'servername', label: 'Server Name', kind: 'string', section: 'tls' },
      { key: 'alpn', label: 'ALPN', kind: 'list', section: 'tls' },
      { key: 'skip-cert-verify', label: 'Skip Cert Verify', kind: 'bool', section: 'tls' },
      { key: 'reality-opts', label: 'REALITY', kind: 'group', section: 'tls', children: [
          { key: 'public-key', label: 'Public Key', kind: 'string', required: true },
          { key: 'short-id', label: 'Short ID', kind: 'string' }
      ] },
      { key: 'alterId', label: 'Alter ID', kind: 'int', section: 'advanced', always: true },
      { key: 'ip-version', label: 'IP Version', kind: 'select', default: 'dual', options: ['dual', 'ipv4', 'ipv6'], section: 'advanced' }
    ]
  },
  {
    type: 'hysteria2',
    name: 'Hysteria2',
    fields: [
      { key: 'server', label: 'Server', kind: 'string', required: true },
      { key: 'port', label: 'Port', kind: 'int' },
      { key: 'ports', label: 'Ports', kind: 'string' },
      { key: 'password', label: 'Password', kind: 'string', secret: true },
      { key: 'obfs', label: 'Obfs', kind: 'select', options: ['salamander', 'gecko'] },
      { key: 'sni', label: 'SNI', kind: 'string', section: 'tls' },
      { key: 'alpn', label: 'ALPN', kind: 'list', section: 'tls' },
      { key: 'cwnd', label: 'CWND', kind: 'int', section: 'advanced' }
    ],
    require_any: [['port'], ['ports']]
  },
  {
    type: 'openvpn',
    tunnel: true,
    name: 'OpenVPN',
    fields: [
      { key: 'server', label: 'Server', kind: 'string', required: true },
      { key: 'port', label: 'Port', kind: 'int', required: true },
      { key: 'proto', label: 'Proto', kind: 'select', default: 'udp', options: ['udp', 'tcp'] },
      { key: 'ca', label: 'CA Certificate', kind: 'text', required: true },
      { key: 'username', label: 'Username', kind: 'string' },
      { key: 'cert', label: 'Client Certificate', kind: 'text', section: 'tls' },
      { key: 'key', label: 'Client Key', kind: 'text', secret: true, section: 'tls' }
    ]
  }
]

const mockProxies: any = {
  GLOBAL: {
    name: 'GLOBAL',
    type: 'Selector',
    now: '节点选择',
    all: ['节点选择', '自动选择', 'DIRECT', 'REJECT']
  },
  '节点选择': {
    name: '节点选择',
    type: 'Selector',
    now: '香港 01 (XUDP)',
    all: ['自动选择', '香港 01 (XUDP)', '日本 01 [IPLc]', 'DIRECT']
  },
  '自动选择': {
    name: '自动选择',
    type: 'URLTest',
    now: '香港 01 (XUDP)',
    all: ['香港 01 (XUDP)', '日本 01 [IPLc]']
  },
  '香港 01 (XUDP)': {
    name: '香港 01 (XUDP)',
    type: 'Shadowsocks',
    udp: true,
    history: [{ time: new Date().toISOString(), delay: 42 }]
  },
  '日本 01 [IPLc]': {
    name: '日本 01 [IPLc]',
    type: 'Vmess',
    udp: true,
    history: [{ time: new Date().toISOString(), delay: 110 }]
  },
  'DIRECT': { name: 'DIRECT', type: 'Direct', history: [] },
  'REJECT': { name: 'REJECT', type: 'Reject', history: [] }
}

/** provider 里承载的节点名（与 mockProxies 中的节点保持一致） */
const mockProviderNodeNames = ['香港 01 (XUDP)', '日本 01 [IPLc]']

/**
 * 组装 GET /providers/proxies 的响应，形状与内核下发的完全一致：
 *
 *   { providers: { <订阅名>: { ...元数据, proxies: [节点...] }, default: { proxies: [代理组...] } } }
 *
 * 前端的融合模式是这样消费它的：遍历 providers 收集所有节点（排除 default），
 * 再从 providers.default 里挑出 type 为 Selector/URLTest/... 的代理组。形状不对
 * （例如把 `{proxies: {...}}` 当成 provider 表）就会静默得到空列表。
 *
 * 节点的 `provider-name` 尤其重要：前端据此决定测速打 /proxies/{name}/delay
 * 还是 /providers/proxies/{provider}/{node}/healthcheck——缺了它，融合模式下的
 * 节点测速会走错接口。
 */
const buildMockProviders = (): Record<string, any> => {
  const providers: Record<string, any> = {}

  const declared = (mockSubConfig.subscriptions || []).map(s => s.name)
  const providerNames = declared.length > 0 ? declared : ['离线模拟订阅']

  providerNames.forEach((name, idx) => {
    providers[name] = {
      name,
      vehicleType: 'HTTP',
      type: 'Proxy',
      updatedAt: new Date().toISOString(),
      subscriptionInfo: mockSubConfig.subscriptions?.[idx]?.subscription_info || {},
      proxies: mockProviderNodeNames.map(nodeName => ({
        ...mockProxies[nodeName],
        'provider-name': name
      }))
    }
  })

  // default 承载代理组（内核把 config.yaml 里静态的 proxy-groups 放在这里）
  providers['default'] = {
    name: 'default',
    vehicleType: 'Compatible',
    type: 'Proxy',
    proxies: ['GLOBAL', '节点选择', '自动选择'].map(n => mockProxies[n])
  }
  return providers
}

const mockRuleProviders = {
  providers: {
    "AdBlock": { name: "AdBlock", type: "Rule", behavior: "classical", ruleCount: 1250, updatedAt: "2026-06-20T10:15:30Z" },
    "GeoIP-CN": { name: "GeoIP-CN", type: "Rule", behavior: "ipcidr", ruleCount: 5400, updatedAt: "2026-06-20T11:00:00Z" }
  }
}

const mockRules = {
  rules: [
    { type: "DomainSuffix", payload: "google.com", proxy: "节点选择" },
    { type: "IPCIDR", payload: "192.168.0.0/16", proxy: "DIRECT" },
    { type: "Match", payload: "", proxy: "GLOBAL" }
  ]
}

let activeMockConns = [
  {
    id: 'c-100',
    metadata: { host: 'github.com', destinationIP: '140.82.113.3', destinationPort: 443, type: 'TLS', network: 'tcp' },
    upload: 15400,
    download: 245000,
    rule: 'DomainKeyword',
    chains: ['GLOBAL', '节点选择', '香港 01 (XUDP)'],
    start: new Date(Date.now() - 60000).toISOString()
  }
]

// TProxy 状态模拟（与后端语义保持一致：三个开关默认全关）
let mockTproxyEnabled = false
let mockTproxyProxyLocal = false
let mockTproxyIPv6 = false
let mockTunSettings = { stack: 'mixed', device: '', dns_mode: 'hijack', ipv6: 'auto' }
let mockTproxyDstExceptions = ['# 公共 DNS 服务器', '223.5.5.5', '1.12.12.12']
let mockTproxySrcExceptions = ['# Docker 默认网段', '172.17.0.0/16']

// 切换模式订阅的自定义规则（按订阅名隔离，模拟后端的即时持久化）
const mockCustomRulesBySub: Record<string, CustomRule[]> = {
  'Sub-Mock-01': [
    { id: 'mock-rule-01', type: 'DOMAIN-SUFFIX', payload: 'ads.example.com', target: 'REJECT', position: 'after', line: 'DOMAIN-SUFFIX,ads.example.com,REJECT', valid: true },
    { id: 'mock-rule-02', type: 'RULE-SET', payload: 'gone', target: '已改名的组', position: 'after', line: 'RULE-SET,gone,已改名的组', valid: false, reason: '规则集 "gone" 不存在于该订阅的 rule-providers 中' }
  ]
}

// 与后端 configcheck.RuleSpecs() 保持一致的类型白名单及载荷示例
const mockCustomRuleTypes = [
  { type: 'DOMAIN', example: 'example.com', no_resolve: false },
  { type: 'DOMAIN-SUFFIX', example: 'example.com', no_resolve: false },
  { type: 'DOMAIN-KEYWORD', example: 'example', no_resolve: false },
  { type: 'DOMAIN-REGEX', example: '^ads\\..*$', no_resolve: false },
  { type: 'GEOSITE', example: 'github', no_resolve: false },
  { type: 'GEOIP', example: 'CN', no_resolve: true },
  { type: 'IP-CIDR', example: '1.1.1.0/24', no_resolve: true },
  { type: 'IP-CIDR6', example: '2001:db8::/32', no_resolve: true },
  { type: 'IP-SUFFIX', example: '1.1.1.0/24', no_resolve: true },
  { type: 'IP-ASN', example: '13335', no_resolve: true },
  { type: 'SRC-IP-CIDR', example: '192.168.1.0/24', no_resolve: true },
  { type: 'SRC-PORT', example: '443', no_resolve: false },
  { type: 'DST-PORT', example: '443', no_resolve: false },
  { type: 'PROCESS-NAME', example: 'curl', no_resolve: false },
  { type: 'NETWORK', example: 'tcp', no_resolve: false },
  { type: 'RULE-SET', example: '从该订阅的规则集中选择', no_resolve: true }
]

// 融合模式的两档规则集：各自的代理组与规则集清单都不同（与后端 configgen 模板一致），
// 因此两档的规则必须分开存放——同一份列表放在两档下必然有一半指向不存在的目标。
// base ≈ 5 个代理组且没有 rule-providers；full ≈ 38 个代理组 + 31 个规则集（此处取代表样子集）。
const mockMergeBaseGroups = ['🚀 节点选择', '👉 手动选择', '♻️ 自动选择', '🐟 漏网之鱼', '🎯 全球直连']
const mockMergeFullGroups = [
  '🚀 节点选择', '👉 手动选择', '♻️ 自动选择', '📈 网络测试', '🕹️ 游戏平台', '🤖 AI 平台',
  '🎬 Prime Video', '📹 油管视频', '🎵 TikTok', '🇨🇳 国内域名', '🀄️ 国内 IP',
  '🐟 漏网之鱼', '🛑 广告域名', '🎯 全球直连', '🇭🇰 香港节点', '🇯🇵 日本节点',
]
// base 档位没有 rule-providers，RULE-SET 在这里无从选择（前端会禁用表单）
const mockMergeBaseProviders: string[] = []
const mockMergeFullProviders = ['ads', 'cn', 'gfw']

// 融合模式的自定义规则（按档位 base/full 隔离，互不影响）
const mockMergeCustomRules: Record<string, CustomRule[]> = {
  base: [],
  full: [
    { id: 'mock-merge-rule-01', type: 'RULE-SET', payload: 'ads', target: '🛑 广告域名', position: 'before', line: 'RULE-SET,ads,🛑 广告域名', valid: true },
  ],
}

// 自定义模式的自定义规则：独立一份，与上面的融合档位互不影响（与后端 custom_mode_rules 对应）
const mockCustomModeRules: CustomRule[] = []

const mockCustomRuleGroups = ['节点选择', '自动选择', '广告拦截']
const mockCustomRuleBuiltins = ['DIRECT', 'REJECT', 'PASS']
const mockCustomRuleProviders = ['ads', 'private']
// 支持 no-resolve 的类型（IP 类与 RULE-SET），用于组装规则行
const mockCustomRuleNoResolveTypes = mockCustomRuleTypes.filter(t => t.no_resolve).map(t => t.type)

// 流量隧道（按作用域隔离，模拟后端的即时持久化）。
//
// key 与后端三种作用域一一对应：融合=merge:<档位>、自定义=custom、切换=sub:<订阅名>。
// 离线开发时即可完整走通「新增 / 排序 / 开关 / 编辑 / 删除」与产物预览。
const mockTunnelsByScope: Record<string, TunnelView[]> = {
  'merge:base': [],
  'merge:full': [
    { id: 'mock-tunnel-01', network: ['tcp', 'udp'], address: '127.0.0.1:6553', target: '8.8.8.8:53', proxy: '🚀 节点选择', enabled: true, line: 'tcp/udp,127.0.0.1:6553,8.8.8.8:53,🚀 节点选择', valid: true },
  ],
  custom: [],
}

let mockTunnelSeq = 100

// 取（并按需初始化）某个作用域的隧道列表。
const mockTunnelStore = (scopeKey: string): TunnelView[] => {
  if (!mockTunnelsByScope[scopeKey]) mockTunnelsByScope[scopeKey] = []
  return mockTunnelsByScope[scopeKey]
}

// 组装隧道视图：单行展示形式、目标归一化与合法性判定都与后端 buildTunnelViews 对齐。
//
// 与后端同样把网络类型归一化为小写（界面显示大写、落盘小写），并把目标定形成 host:port：
// 只写域名时补默认端口 `:80`，裸 IP 则判为无效（不猜 IP 的端口）。内核解析不了裸主机，
// 而失败只在起监听时打一行日志并跳过该条（隧道静默失效），所以必须在写入前定形。
const buildMockTunnelView = (body: any, id: string, knownProxies: string[]): TunnelView => {
  const network: string[] = (Array.isArray(body.network) ? body.network : ['tcp', 'udp'])
    .map((item: unknown) => String(item).trim().toLowerCase())
  const address = String(body.address || '').trim()
  const proxy = String(body.proxy || '').trim()

  let target = String(body.target || '').trim()
  // 裸 IP（纯数字点分，或含冒号的 IPv6）不补端口；纯域名按 http 默认端口补 :80
  const bareIp = /^[\d.]+$/.test(target) || target.includes(':')
  if (target && !target.includes(':') && !bareIp) target = `${target}:80`

  const view: TunnelView = {
    id,
    network,
    address,
    target,
    proxy,
    enabled: body.enabled === undefined ? true : !!body.enabled,
    line: [network.join('/'), address, target, proxy].filter((part, i) => part !== '' || i === 2).join(','),
    valid: true,
  }
  if (!address.includes(':')) {
    view.valid = false
    view.reason = '本地监听地址应为 host:port（如 127.0.0.1:8888、0.0.0.0:8888）'
  } else if (!target.includes(':')) {
    view.valid = false
    view.reason = '目标转发地址缺少端口，请写成 host:port（如 8.8.8.8:8888）；只写域名时按 :80 处理'
  } else if (proxy && !knownProxies.includes(proxy)) {
    view.valid = false
    view.reason = `proxy "${proxy}" 不在当前可选目标里（代理组/节点可能已改名，请重新选择）`
  }
  return view
}

// 处理隧道接口（三个作用域共用）：方法语义与后端一致，写操作直接改内存数组。
//
// 返回 null 表示当前方法不是本模块处理的（调用方继续向下匹配）。
// 返回 null 表示「这个方法不是本模块处理的」，"" 表示成功，其它字符串是**给用户看的原因**。
//
// 失败原因直接取自视图的 reason（与后端 ValidateTunnel 的措辞对齐），这样离线开发时看到的
// 提示与真实后端一致——否则 mock 只会甩一句笼统的「保存失败」，让人分不清是哪一项不合法。
const handleMockTunnelRequest = (
  tunnels: TunnelView[],
  knownProxies: string[],
  rawPath: string,
  method: string,
  bodyRaw: string | undefined,
): string | null => {
  if (method === 'POST') {
    const body = JSON.parse(bodyRaw || '{}')
    mockTunnelSeq += 1
    const view = buildMockTunnelView(body, `mock-tunnel-${mockTunnelSeq}`, knownProxies)
    if (!view.valid) return view.reason || '隧道配置无效'
    // 新增追加到列表末尾（与后端一致）
    tunnels.push(view)
    return ''
  }

  if (method === 'PUT') {
    const body = JSON.parse(bodyRaw || '{}')
    const idx = tunnels.findIndex(t => t.id === body.id)
    if (idx < 0) return '隧道不存在: ' + body.id
    // 就地替换：位置不变；开关（关闭时不做校验）与字段一起更新
    const view = buildMockTunnelView(body, tunnels[idx].id, knownProxies)
    view.valid = body.enabled === false ? true : view.valid
    if (!view.valid) return view.reason || '隧道配置无效'
    tunnels[idx] = view
    return ''
  }

  if (method === 'PATCH') {
    const body = JSON.parse(bodyRaw || '{}')
    const idx = tunnels.findIndex(t => t.id === body.id)
    if (idx < 0) return '隧道不存在: ' + body.id
    const swapIdx = body.direction === 'up' ? idx - 1 : idx + 1
    if (swapIdx < 0 || swapIdx >= tunnels.length) return '隧道已在列表的最前/最后，无法继续移动'
    const moved = tunnels[idx]
    tunnels[idx] = tunnels[swapIdx]
    tunnels[swapIdx] = moved
    return ''
  }

  if (method === 'DELETE') {
    const id = decodeURIComponent(new URLSearchParams(rawPath.split('?')[1] || '').get('id') || '')
    const idx = tunnels.findIndex(t => t.id === id)
    if (idx < 0) return '隧道不存在: ' + id
    tunnels.splice(idx, 1)
    return ''
  }

  return null
}

// 与后端一致：响应中的 rules 已按生效顺序排列（before 组整体在前、after 组整体在后），前端原样渲染
const orderMockCustomRules = (rules: any[]) => [
  ...rules.filter(r => r.position === 'before'),
  ...rules.filter(r => r.position !== 'before')
]

// 递增序号：id 是列表 key 也是编辑/排序的定位依据，同一毫秒内连续写入不能重复
let mockRuleSeq = 0
const nextMockRuleId = () => `mock-rule-${Date.now()}-${++mockRuleSeq}`

// 由请求体组装一条规则：规则行由结构化字段拼装，no-resolve 仅对支持的类型生效，position 缺省为最前
const buildMockCustomRule = (body: any, id: string) => {
  const type = String(body.type || '').toUpperCase()
  const payload = String(body.payload || '').trim()
  const target = String(body.target || '').trim()
  const noResolve = !!body.no_resolve && mockCustomRuleNoResolveTypes.includes(type)
  return {
    id,
    type,
    payload,
    target,
    position: body.position === 'after' ? 'after' : 'before',
    no_resolve: noResolve,
    line: [type, payload, target].join(',') + (noResolve ? ',no-resolve' : ''),
    valid: true
  }
}

// 快捷响应封装（模块级：合并规则的处理函数同样需要）
const reply = (data: any, status = 200) => new Response(JSON.stringify(data), { status })

// 组装自定义规则接口的统一响应体（切换/融合两种模式同构，仅 groups/providers 不同；
// status/message 仅写操作时携带）
const buildMockCustomRulesPayload = (
  rules: CustomRule[],
  groups: string[],
  providers: string[],
  status?: 'ok' | 'warning',
  message?: string,
  nodes: string[] = [],
  tunnels: TunnelView[] = [],
): CustomRulesPayload => ({
  file_ready: true,
  rules: orderMockCustomRules(rules),
  // 规则接口与隧道接口的响应必须同构：前端拿到任一响应都会整份刷新弹窗
  tunnels,
  groups,
  // 节点作为目标只在自定义模式下给出（与后端 buildMergeRulesPayload 一致）
  nodes,
  builtins: mockCustomRuleBuiltins,
  providers,
  rule_types: mockCustomRuleTypes,
  status,
  message
})

// 处理 /subscribe/custom-rules 与 /subscribe/merge-custom-rules 的共用逻辑：
// 两者的方法语义完全一致，只有「作用域 → 规则数组/groups/providers」的映射不同。
//
// 返回 null 表示当前方法不是本模块写的（调用方继续向下匹配），
// rawQuery 需传入含查询串的原始 path——DELETE 的 id 只能从这里取。
const handleMockCustomRulesRequest = (
  rules: CustomRule[],
  groups: string[],
  providers: string[],
  rawPath: string,
  method: string,
  bodyRaw: string | undefined,
  nodes: string[] = [],
  tunnels: TunnelView[] = [],
): Response | null => {
  if (method === 'POST') {
    const body = JSON.parse(bodyRaw || '{}')
    rules.push(buildMockCustomRule(body, nextMockRuleId()))
    return reply(buildMockCustomRulesPayload(rules, groups, providers, 'ok', undefined, nodes, tunnels))
  }

  if (method === 'PUT') {
    const body = JSON.parse(bodyRaw || '{}')
    const idx = rules.findIndex(r => r.id === body.id)
    if (idx < 0) return reply({ status: 'error', message: '规则不存在: ' + body.id }, 404)
    // 就地替换：索引与 id 都保持原样，列表位置不变（与后端 PUT 语义一致）
    rules[idx] = buildMockCustomRule(body, rules[idx].id)
    return reply(buildMockCustomRulesPayload(rules, groups, providers, 'ok', undefined, nodes, tunnels))
  }

  if (method === 'PATCH') {
    const body = JSON.parse(bodyRaw || '{}')
    const direction = body.direction === 'up' ? 'up' : 'down'
    // 交换在生效顺序上进行，且只与同 position 分组内的相邻规则交换
    const ordered = orderMockCustomRules(rules)
    const idx = ordered.findIndex(r => r.id === body.id)
    if (idx < 0) return reply({ status: 'error', message: '规则不存在: ' + body.id }, 404)
    const swapIdx = direction === 'up' ? idx - 1 : idx + 1
    const neighbour = ordered[swapIdx]
    // 边界（已是该分组首/末条）回 400，而不是假装成功
    if (!neighbour || neighbour.position !== ordered[idx].position) {
      return reply({ status: 'error', message: direction === 'up' ? '规则已在该分组的最前，无法继续移动' : '规则已在该分组的最后，无法继续移动' }, 400)
    }
    const moved = ordered[idx]
    ordered[idx] = neighbour
    ordered[swapIdx] = moved
    // 交换结果按生效顺序写回原数组
    ordered.forEach((r, i) => { rules[i] = r })
    return reply(buildMockCustomRulesPayload(rules, groups, providers, undefined, undefined, nodes, tunnels))
  }

  if (method === 'DELETE') {
    // 原始 path 的查询串已随 cleanPath 一起剥离，id 需从这里取
    const id = decodeURIComponent(new URLSearchParams(rawPath.split('?')[1] || '').get('id') || '')
    const idx = rules.findIndex(r => r.id === id)
    if (idx < 0) return reply({ status: 'error', message: '规则不存在: ' + id }, 404)
    rules.splice(idx, 1)
    return reply(buildMockCustomRulesPayload(rules, groups, providers, 'ok', undefined, nodes, tunnels))
  }

  if (method === 'GET') return reply(buildMockCustomRulesPayload(rules, groups, providers, undefined, undefined, nodes, tunnels))

  return null
}

// 模拟 HTTP API
export function handleMockFetch(path: string, options: RequestInit = {}): Response {
  const method = (options.method || 'GET').toUpperCase()
  const cleanPath = path.split('?')[0].replace(/\/$/, '')

  if (cleanPath.endsWith('/core/status')) return reply({ running: coreRunning })
  // 后端编译期注入的版本号（dev 模式下用固定值模拟）
  if (cleanPath.endsWith('/app-version')) return reply({ version: '1.0.0' })
  if (cleanPath.endsWith('/check-update')) return reply({ hasUpdate: false, current: '1.0.0' })
  if (cleanPath.endsWith('/core/start')) { coreRunning = true; return reply({ status: 'ok' }) }
  if (cleanPath.endsWith('/core/stop')) { coreRunning = false; return reply({ status: 'ok' }) }
  if (cleanPath.endsWith('/restart') || cleanPath.endsWith('/core/restart')) return reply({ status: 'ok' })
  if (cleanPath.endsWith('/version')) return reply({ version: 'v1.18.8-meta' })

  // TProxy：/config/tproxy/exceptions 与 /config/tproxy/proxy-local 路径更长，
  // 必须先匹配，否则会被 /config/tproxy 的前缀判断吞掉
  if (cleanPath.endsWith('/config/tproxy/exceptions')) {
    if (method === 'POST') {
      const body = JSON.parse(options.body as string || '{}')
      mockTproxyDstExceptions = body.dst || []
      mockTproxySrcExceptions = body.src || []
    }
    // defaults 仅用于让离线开发也能走通「恢复默认」按钮，取的是缩略示例；
    // 真正的预填清单由后端维护（backend/internal/tproxy/store.go 的 defaultDst/SrcExceptions）
    return reply({
      dst: mockTproxyDstExceptions,
      src: mockTproxySrcExceptions,
      defaults: {
        dst: ['# 公共 DNS（IPv4）：让解析请求直连，避免 DNS 被劫持', '223.5.5.5', '# 公共 DNS（IPv6）：需先开启「接管 IPv6 流量」，否则不会下发', '2400:3200::1'],
        src: ['# Docker 默认 bridge 网段：填在这里意味着 Docker 容器的出站流量默认不代理', '172.17.0.0/16'],
      },
    })
  }
  if (cleanPath.endsWith('/config/tproxy/proxy-local')) {
    if (method === 'POST') {
      const body = JSON.parse(options.body as string || '{}')
      mockTproxyProxyLocal = !!body.enabled
    }
    return reply({ enabled: mockTproxyProxyLocal })
  }
  if (cleanPath.endsWith('/config/tproxy/proxy-ipv6')) {
    if (method === 'POST') {
      const body = JSON.parse(options.body as string || '{}')
      mockTproxyIPv6 = !!body.enabled
    }
    // global_ipv6：订阅中心的 IPv6 总开关，关闭时接管偏好仍保留但不生效
    return reply({ enabled: mockTproxyIPv6, global_ipv6: (mockSubConfig as any).ipv6 !== false })
  }
  if (cleanPath.endsWith('/config/tproxy')) {
    if (method === 'POST') {
      const body = JSON.parse(options.body as string || '{}')
      const enable = !!body.enable
      // 与后端一致：端口为 0 时拒绝启用（后端返回 400）
      if (enable && (mockConfigs['tproxy-port'] || 0) === 0) {
        return reply({ status: 'error', message: 'TProxy 端口为 0，请先配置端口' }, 400)
      }
      mockTproxyEnabled = enable
    }
    return reply({ enabled: mockTproxyEnabled })
  }

  // TUN 参数与启停（与后端语义一致：参数只落 tun.json，开关只 PATCH 内核）
  if (cleanPath.endsWith('/config/tun/state')) {
    const body = JSON.parse(options.body as string || '{}')
    mockConfigs.tun.enable = !!body.enable
    return reply({ status: 'ok', enable: !!body.enable })
  }
  if (cleanPath.endsWith('/config/tun')) {
    if (method === 'POST') {
      const body = JSON.parse(options.body as string || '{}')
      mockTunSettings = { ...mockTunSettings, ...body }
    }
    return reply({
      effective: mockTunSettings,
      defaults: { stack: 'mixed', device: '', dns_mode: 'hijack', ipv6: 'auto' },
      host_ipv6: true,
      global_ipv6: (mockSubConfig as any).ipv6 !== false,
    })
  }

  if (cleanPath.endsWith('/configs')) {
    if (method === 'PATCH' || method === 'PUT') {
      Object.assign(mockConfigs, JSON.parse(options.body as string || '{}'))
      return reply({ status: 'ok' })
    }
    return reply(mockConfigs)
  }

  if (cleanPath.endsWith('/subscribe/node-protocols')) {
    return reply({ protocols: mockNodeProtocols })
  }
  // 手动更新单个订阅：/subscribe/update/{name}
  //
  // 与后端一致：切换模式同步返回更新后的元数据（前端直接刷新卡片），融合模式先回
  // processing、后台更新完成后再让元数据的 updated_at 变化（前端每 2s 轮询
  // /subscribe/config 直到看到变化）。元数据在 mock 里与订阅注册表同处一项，
  // 但由更新流程单独写入——对应后端 subscription-meta.json 由更新流程维护。
  if (cleanPath.includes('/subscribe/update/')) {
    const subName = decodeURIComponent(cleanPath.split('/subscribe/update/')[1] || '')
    const sub = (mockSubConfig.subscriptions as any[]).find(item => item.name === subName)
    if (!sub) {
      return reply({ status: 'error', message: '未找到该订阅' }, 404)
    }
    const refreshMeta = () => {
      const prev = sub.subscription_info || {}
      sub.subscription_info = {
        upload: prev.upload || 0,
        download: (prev.download || 0) + 1024 * 1024,
        total: prev.total || 0,
        expire: prev.expire || 0,
      }
      sub.updated_at = new Date().toISOString()
    }
    if (mockSubConfig.mode === 'switch') {
      refreshMeta()
      return reply({
        status: 'ok',
        message: '订阅更新成功',
        info: { ...sub.subscription_info, updatedAt: sub.updated_at },
      })
    }
    // 融合模式：异步，2 秒后元数据才变化（前端轮询期间会看到 updatedAt 更新）
    setTimeout(refreshMeta, 2000)
    return reply({ status: 'processing', message: '订阅更新已在后台启动' })
  }
  if (cleanPath.endsWith('/subscribe/config')) {
    if (method === 'POST') {
      applyMockSettings(JSON.parse(options.body as string || '{}'))
      return reply({ status: 'ok' })
    }
    return reply(mockSubConfig)
  }
  if (cleanPath.endsWith('/subscribe/generate')) {
    // 与后端一致：只落「设置」类字段（全局参数 + 订阅注册表 + 手工节点），
    // 请求体里的规则/隧道/元数据一律忽略；delete_physical 是仅本次有效的临时字段。
    if (method === 'POST') {
      const payload = JSON.parse(options.body as string || '{}')
      applyMockSettings(payload)
      return reply({ status: 'ok', message: payload.mode === 'custom' ? '自定义节点配置已生成并成功重载内核' : '配置文件已生成并成功重载内核' })
    }
    return reply({ status: 'ok' })
  }

  // ===== provider 相关必须在 /proxies 之前判定 =====
  //
  // 后端的 /providers/proxies 与 /proxies 是两个接口，而这里原先只有一条
  // `cleanPath.endsWith('/proxies')`——它同时匹配 '/proxies' 与 '/providers/proxies'，
  // 于是融合模式（离线模拟的默认模式）拿到的是 `{proxies: {...}}` 而不是
  // `{providers: {<订阅名>: {...}}}`，前端遍历 providers 得到空对象，代理页永远空白；
  // /providers/proxies/{订阅}/{节点}/healthcheck 也因为同样的顺序问题落进了 delay 分支。
  // 模拟器与真实后端契约不一致最危险的地方就在于：开发期看不出问题，上线才炸。
  if (cleanPath.endsWith('/providers/proxies')) {
    return reply({ providers: buildMockProviders() })
  }
  if (cleanPath.includes('/providers/proxies/')) {
    const parts = cleanPath.split('/providers/proxies/')[1]?.split('/') || []
    const providerName = decodeURIComponent(parts[0] || '')
    // /providers/proxies/{订阅}/{节点}/healthcheck → 内核返回 {delay}
    if (parts.length >= 3 && parts[2] === 'healthcheck') {
      const nodeName = decodeURIComponent(parts[1] || '')
      const delay = Math.floor(30 + Math.random() * 200)
      if (mockProxies[nodeName]) {
        mockProxies[nodeName].history = [{ time: new Date().toISOString(), delay }]
      }
      return reply({ delay })
    }
    const providers = buildMockProviders()
    if (providers[providerName]) return reply(providers[providerName])
    return reply({ message: `provider ${providerName} not found` }, 404)
  }

  if (cleanPath.endsWith('/proxies')) return reply({ proxies: mockProxies })
  if (cleanPath.includes('/proxies/') && cleanPath.endsWith('/delay')) {
    const parts = cleanPath.split('/')
    const proxyName = decodeURIComponent(parts[parts.length - 2])
    const delay = Math.floor(30 + Math.random() * 200)
    if (mockProxies[proxyName]) {
      mockProxies[proxyName].history = [{ time: new Date().toISOString(), delay }]
    }
    return reply({ delay })
  }
  if (cleanPath.includes('/proxies/') && method === 'PUT') {
    const parts = cleanPath.split('/')
    const groupName = decodeURIComponent(parts[parts.length - 1])
    const body = JSON.parse(options.body as string || '{}')
    if (mockProxies[groupName]) {
      mockProxies[groupName].now = body.name
      return reply({ status: 'ok' })
    }
  }

  if (cleanPath.endsWith('/providers/rules')) return reply(mockRuleProviders)
  if (cleanPath.endsWith('/rules')) return reply(mockRules)
  if (cleanPath.includes('/cache/')) return reply({ status: 'ok' })

  if (cleanPath.endsWith('/dns/query')) {
    return reply({ Status: 0, Answer: [{ data: '192.168.1.100' }] })
  }
  if (cleanPath.endsWith('/interfaces')) return reply(['eth0', 'wlan0', 'meta'])

  // IP 信息模拟
  if (cleanPath.endsWith('/ipinfo/local/v4')) {
    return reply({ ip: '116.228.111.222', country: '中国', region: '上海', isp: '电信' })
  }
  if (cleanPath.endsWith('/ipinfo/local/v6')) return reply({ ip: '240e:3b3:3000:200::100' })
  if (cleanPath.endsWith('/ipinfo/proxy/v4')) {
    return reply({ ip: '104.244.42.1', country: '美国', region: '加利福尼亚', isp: 'Twitter Inc.' })
  }
  if (cleanPath.endsWith('/ipinfo/proxy/v6')) return reply({ ip: '2606:4700:3030::ac43:8ad7' })

  // 延迟测试模拟
  if (cleanPath.includes('/delaytest/')) {
    return reply({ delay: Math.floor(40 + Math.random() * 100) })
  }

  // 自定义模式自定义规则（/subscribe/custom-mode-rules/custom）：独立一份存储，
  // 目标里额外含手工节点名，与融合模式的规则互不影响
  if (cleanPath.includes('/subscribe/custom-mode-rules/')) {
    const scope = decodeURIComponent(cleanPath.split('/subscribe/custom-mode-rules/')[1] || '')
    if (scope !== 'custom') {
      return reply({ status: 'error', message: '未知作用域: ' + scope }, 400)
    }
    const customNodes = (mockSubConfig.custom_nodes || []) as { name: string }[]
    const nodes = mockSubConfig.mode === 'custom' ? customNodes.map(node => node.name) : []
    const resp = handleMockCustomRulesRequest(
      mockCustomModeRules,
      mockMergeBaseGroups,
      mockMergeBaseProviders,
      path,
      method,
      options.body as string | undefined,
      nodes,
      mockTunnelStore('custom'),
    )
    if (resp) return resp
  }

  // 融合模式自定义规则（/subscribe/merge-custom-rules/{base|full}）：方法语义与切换模式一致，
  // 区别只在于作用域是规则集档位、两档各有自己的代理组与规则集清单（互不影响）。
  // 该前缀必须在切换模式那条更短的前缀之前匹配（两者字符串不同，此处仅作可读性排序）。
  if (cleanPath.includes('/subscribe/merge-custom-rules/')) {
    const ruleGroup = decodeURIComponent(cleanPath.split('/subscribe/merge-custom-rules/')[1] || '')
    if (ruleGroup !== 'base' && ruleGroup !== 'full') {
      return reply({ status: 'error', message: '未知规则集: ' + ruleGroup }, 400)
    }
    if (!mockMergeCustomRules[ruleGroup]) mockMergeCustomRules[ruleGroup] = []
    const groups = ruleGroup === 'full' ? mockMergeFullGroups : mockMergeBaseGroups
    const providers = ruleGroup === 'full' ? mockMergeFullProviders : mockMergeBaseProviders
    // 融合模式的档位不含节点目标（节点只在自定义模式下可选）
    const resp = handleMockCustomRulesRequest(
      mockMergeCustomRules[ruleGroup],
      groups,
      providers,
      path,
      method,
      options.body as string | undefined,
      [],
      mockTunnelStore(`merge:${ruleGroup}`),
    )
    if (resp) return resp
  }

  // 流量隧道（三个作用域各一条前缀）：作用域语义与上面三条规则入口一一对应。
  // 响应体与规则接口同构（同时含 rules 与 tunnels），前端用一个 payload 刷新整个弹窗。
  const tunnelRoutes: { prefix: string, scopeKey: string, groups: string[], nodes: string[] }[] = [
    { prefix: '/subscribe/merge-custom-tunnels/', scopeKey: 'merge:', groups: mockMergeBaseGroups, nodes: [] },
    { prefix: '/subscribe/custom-mode-tunnels/', scopeKey: 'custom', groups: mockMergeBaseGroups, nodes: [] },
    { prefix: '/subscribe/custom-tunnels/', scopeKey: 'sub:', groups: mockCustomRuleGroups, nodes: [] },
  ]
  for (const route of tunnelRoutes) {
    if (!cleanPath.includes(route.prefix)) continue
    const scope = decodeURIComponent(cleanPath.split(route.prefix)[1] || '')
    let scopeKey = route.scopeKey + scope
    let groups = route.groups
    let nodes = route.nodes
    let rules = mockCustomModeRules
    let providers = mockMergeBaseProviders
    if (route.prefix === '/subscribe/merge-custom-tunnels/') {
      if (scope !== 'base' && scope !== 'full') return reply({ status: 'error', message: '未知规则集: ' + scope }, 400)
      groups = scope === 'full' ? mockMergeFullGroups : mockMergeBaseGroups
      providers = scope === 'full' ? mockMergeFullProviders : mockMergeBaseProviders
      rules = mockMergeCustomRules[scope] || (mockMergeCustomRules[scope] = [])
    } else if (route.prefix === '/subscribe/custom-mode-tunnels/') {
      if (scope !== 'custom') return reply({ status: 'error', message: '未知作用域: ' + scope }, 400)
      const customNodes = (mockSubConfig.custom_nodes || []) as { name: string }[]
      nodes = mockSubConfig.mode === 'custom' ? customNodes.map(node => node.name) : []
    } else {
      if (!mockCustomRulesBySub[scope]) mockCustomRulesBySub[scope] = []
      rules = mockCustomRulesBySub[scope]
      providers = mockCustomRuleProviders
    }
    const tunnels = mockTunnelStore(scopeKey)
    // 已知可选 proxy = 代理组 + 自定义模式的手工节点（与后端一致：其余模式不下发节点）
    const result = handleMockTunnelRequest(tunnels, [...groups, ...nodes], path, method, options.body as string | undefined)
    if (result === null) {
      return reply(buildMockCustomRulesPayload(rules, groups, providers, undefined, undefined, nodes, tunnels))
    }
    if (result !== '') {
      return reply({ status: 'error', message: result }, 400)
    }
    return reply(buildMockCustomRulesPayload(rules, groups, providers, 'ok', undefined, nodes, tunnels))
  }

  // 订阅自定义规则（切换模式）：GET 查询 / POST 新增 / PUT 修改 / PATCH 排序 / DELETE 删除（?id=）
  // 规则按订阅名隔离，写操作立即改内存数组，与后端「即时持久化」语义一致
  if (cleanPath.includes('/subscribe/custom-rules/')) {
    const subName = decodeURIComponent(cleanPath.split('/subscribe/custom-rules/')[1] || '')
    if (!mockCustomRulesBySub[subName]) mockCustomRulesBySub[subName] = []
    const resp = handleMockCustomRulesRequest(
      mockCustomRulesBySub[subName],
      mockCustomRuleGroups,
      mockCustomRuleProviders,
      path,
      method,
      options.body as string | undefined,
      [],
      mockTunnelStore(`sub:${subName}`),
    )
    if (resp) return resp
  }

  return reply({ error: 'Not Found' }, 404)
}

// 模拟推送的局部状态
let lastUp = 100000
let lastDown = 500000
let lastConnId = 100

// 模拟 WebSocket
export class MockWebSocket {
  url: string
  readyState: number = 0
  onopen: (() => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  onmessage: ((e: MessageEvent) => void) | null = null
  private intervalId: any = null

  constructor(url: string, path: string) {
    this.url = url
    setTimeout(() => {
      this.readyState = 1
      if (this.onopen) this.onopen()
      this.startPushData(path)
    }, 50)
  }

  private startPushData(path: string) {
    this.intervalId = setInterval(() => {
      if (this.readyState !== 1) return
      let dataStr = ''

      if (path.includes('/traffic')) {
        lastUp = Math.max(5000, Math.floor(lastUp * (1 + (Math.random() - 0.48) * 0.2)))
        lastDown = Math.max(10000, Math.floor(lastDown * (1 + (Math.random() - 0.48) * 0.2)))
        dataStr = JSON.stringify({ up: lastUp, down: lastDown })
      } else if (path.includes('/memory')) {
        dataStr = JSON.stringify({ inuse: Math.floor(40000000 + Math.random() * 5000000) })
      } else if (path.includes('/logs')) {
        const logs = [
          '{"type":"info","payload":"[Proxy] switch Selector to Hong Kong 01"}',
          '{"type":"debug","payload":"[TCP] dial google.com:443 direct"}',
          '{"type":"info","payload":"[DNS] query baidu.com from 119.29.29.29"}'
        ]
        dataStr = logs[Math.floor(Math.random() * logs.length)]
      } else if (path.includes('/connections')) {
        // 增加流量
        activeMockConns.forEach(c => {
          c.upload += Math.floor(Math.random() * 2000)
          c.download += Math.floor(Math.random() * 10000)
        })
        // 概率关闭连接
        if (activeMockConns.length > 1 && Math.random() < 0.1) {
          activeMockConns.pop()
        }
        // 概率新连接
        if (activeMockConns.length < 4 && Math.random() < 0.15) {
          lastConnId++
          activeMockConns.push({
            id: `c-${lastConnId}`,
            metadata: { host: 'youtube.com', destinationIP: '172.217.160.78', destinationPort: 443, type: 'TLS', network: 'tcp' },
            upload: 100,
            download: 1000,
            rule: 'Match',
            chains: ['GLOBAL', '节点选择', '日本 01 [IPLc]'],
            start: new Date().toISOString()
          })
        }
        dataStr = JSON.stringify({ connections: activeMockConns })
      }

      if (dataStr && this.onmessage) {
        this.onmessage(new MessageEvent('message', { data: dataStr }))
      }
    }, 1000)
  }

  close() {
    this.readyState = 3
    if (this.intervalId) {
      clearInterval(this.intervalId)
      this.intervalId = null
    }
    setTimeout(() => {
      if (this.onclose) this.onclose()
    }, 10)
  }
}

// 模拟后端的内核状态 SSE（/core/events）。
//
// 与真实实现保持同样的语义：连接后先推一次当前快照，之后仅在状态变化时推送。
// 这里通过轮询本地 coreRunning 变量来模拟「后端主动推送」。
export function createMockSse(
  // 与真实 sseConnect 保持同一签名；mock 仅模拟 /core/events 单一事件源，
  // 故路径本身不参与逻辑，用下划线前缀显式标记为有意未使用。
  _path: string,
  onEvent: (data: any) => void,
  handlers: { onOpen?: () => void; onError?: (ev: Event) => void } = {}
) {
  let lastSent: boolean | null = null
  let closed = false
  let pollTimer: any = null

  const openTimer = setTimeout(() => {
    if (closed) return
    if (handlers.onOpen) handlers.onOpen()
    // 首次快照
    lastSent = coreRunning
    onEvent({ running: coreRunning, ts: Date.now() })

    // 每 500ms 检查一次，状态变化时推送（模拟服务端主动推送）
    pollTimer = setInterval(() => {
      if (closed) return
      if (coreRunning !== lastSent) {
        lastSent = coreRunning
        onEvent({ running: coreRunning, ts: Date.now() })
      }
    }, 500)
  }, 50)

  return {
    close: () => {
      closed = true
      clearTimeout(openTimer)
      if (pollTimer) clearInterval(pollTimer)
    }
  }
}
