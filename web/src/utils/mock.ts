// 模拟后端数据库，维持状态更改
let coreRunning = true
const mockConfigs = {
  'allow-lan': true,
  ipv6: false,
  mode: 'Rule',
  'log-level': 'info',
  'interface-name': 'eth0',
  tun: { enable: false, stack: 'System', device: '' },
  port: 7890,
  'socks-port': 7891,
  'redir-port': 0,
  'tproxy-port': 0,
  'mixed-port': 7890
}

const mockSubConfig = {
  proxy_port: 7890,
  panel_port: 9090,
  panel_secret: 'secret123',
  rule_group: 'base',
  ui_panel: 'metacubexd',
  meta_backend_url: '',
  subscriptions: [
    {
      name: 'Sub-Mock-01',
      url: 'https://example.com/subscribe',
      update_interval: 3600,
      health_interval: 300,
      prefix: '',
      info: {
        upload: 1204850123,
        download: 58941094120,
        total: 107374182400,
        expire: Math.floor(Date.now() / 1000) + 864000,
        updatedAt: new Date().toISOString()
      }
    }
  ]
}

const mockProxies: any = {
  GLOBAL: {
    name: 'GLOBAL',
    type: 'Selector',
    now: '节点选择',
    all: ['节点选择', '自动选择', '故障转移', 'DIRECT', 'REJECT']
  },
  '节点选择': {
    name: '节点选择',
    type: 'Selector',
    now: '香港 01 (XUDP)',
    all: ['自动选择', '香港 01 (XUDP)', '日本 01 [IPLc]', '美国 01 [IPLc]', '新加坡 01', 'DIRECT']
  },
  '自动选择': {
    name: '自动选择',
    type: 'URLTest',
    now: '香港 01 (XUDP)',
    all: ['香港 01 (XUDP)', '日本 01 [IPLc]', '美国 01 [IPLc]', '新加坡 01']
  },
  '故障转移': {
    name: '故障转移',
    type: 'Fallback',
    now: '香港 01 (XUDP)',
    all: ['香港 01 (XUDP)', '日本 01 [IPLc]', '美国 01 [IPLc]']
  },
  '香港 01 (XUDP)': {
    name: '香港 01 (XUDP)',
    type: 'Shadowsocks',
    udp: true,
    xudp: true,
    history: [
      { time: new Date(Date.now() - 50000).toISOString(), delay: 42 },
      { time: new Date(Date.now() - 40000).toISOString(), delay: 38 },
      { time: new Date(Date.now() - 30000).toISOString(), delay: 45 },
      { time: new Date(Date.now() - 20000).toISOString(), delay: 52 },
      { time: new Date().toISOString(), delay: 40 }
    ]
  },
  '日本 01 [IPLc]': {
    name: '日本 01 [IPLc]',
    type: 'Vmess',
    udp: true,
    history: [
      { time: new Date(Date.now() - 30000).toISOString(), delay: 110 },
      { time: new Date(Date.now() - 20000).toISOString(), delay: 125 },
      { time: new Date().toISOString(), delay: 118 }
    ]
  },
  '美国 01 [IPLc]': {
    name: '美国 01 [IPLc]',
    type: 'Trojan',
    udp: false,
    history: [
      { time: new Date(Date.now() - 20000).toISOString(), delay: 195 },
      { time: new Date().toISOString(), delay: 210 }
    ]
  },
  '新加坡 01': {
    name: '新加坡 01',
    type: 'Hysteria2',
    udp: true,
    history: [
      { time: new Date(Date.now() - 20000).toISOString(), delay: -1 },
      { time: new Date().toISOString(), delay: 85 }
    ]
  },
  'DIRECT': {
    name: 'DIRECT',
    type: 'Direct',
    history: []
  },
  'REJECT': {
    name: 'REJECT',
    type: 'Reject',
    history: []
  }
}

// 模拟的规则提供商和规则列表数据
const mockRuleProviders = {
  providers: {
    "AdBlock": {
      name: "AdBlock",
      type: "Rule",
      behavior: "classical",
      vehicleType: "HTTP",
      ruleCount: 1250,
      updatedAt: "2026-06-20T10:15:30Z"
    },
    "GeoIP-CN": {
      name: "GeoIP-CN",
      type: "Rule",
      behavior: "ipcidr",
      vehicleType: "File",
      ruleCount: 5400,
      updatedAt: "2026-06-20T11:00:00Z"
    }
  }
}

const mockRules = {
  rules: [
    { type: "DomainSuffix", payload: "google.com", proxy: "节点选择" },
    { type: "DomainKeyword", payload: "github", proxy: "节点选择" },
    { type: "IPCIDR", payload: "192.168.0.0/16", proxy: "DIRECT" },
    { type: "GeoIP", payload: "CN", proxy: "DIRECT" },
    { type: "Match", payload: "", proxy: "GLOBAL" }
  ]
}

// 模拟活跃连接列表
let activeMockConns = [
  {
    id: 'c-100',
    metadata: { host: 'github.com', destinationIP: '140.82.113.3', destinationPort: 443, type: 'TLS', network: 'tcp' },
    upload: 15400,
    download: 245000,
    rule: 'DomainKeyword',
    chains: ['GLOBAL', '节点选择', '香港 01 (XUDP)'],
    start: new Date(Date.now() - 120000).toISOString()
  },
  {
    id: 'c-200',
    metadata: { host: 'gemini.google.com', destinationIP: '172.217.160.78', destinationPort: 443, type: 'HTTP', network: 'tcp' },
    upload: 84000,
    download: 1200000,
    rule: 'DomainSuffix',
    chains: ['GLOBAL', '节点选择', '日本 01 [IPLc]'],
    start: new Date(Date.now() - 45000).toISOString()
  },
  {
    id: 'c-300',
    metadata: { host: 'baidu.com', destinationIP: '220.181.38.148', destinationPort: 80, type: 'HTTP', network: 'tcp' },
    upload: 1200,
    download: 4500,
    rule: 'GeoIP',
    chains: ['GLOBAL', 'DIRECT'],
    start: new Date(Date.now() - 15000).toISOString()
  }
]

// 模拟 HTTP API
export function handleMockFetch(path: string, options: RequestInit = {}): Response {
  const method = (options.method || 'GET').toUpperCase()
  const cleanPath = path.split('?')[0].replace(/\/$/, '')

  // 1. 内核状态控制
  if (cleanPath.endsWith('/core/status')) {
    return new Response(JSON.stringify({ running: coreRunning }), { status: 200 })
  }
  if (cleanPath.endsWith('/core/start')) {
    coreRunning = true
    return new Response(JSON.stringify({ status: 'ok', message: 'started' }), { status: 200 })
  }
  if (cleanPath.endsWith('/core/stop')) {
    coreRunning = false
    return new Response(JSON.stringify({ status: 'ok', message: 'stopped' }), { status: 200 })
  }
  if (cleanPath.endsWith('/restart') || cleanPath.endsWith('/core/restart')) {
    return new Response(JSON.stringify({ status: 'ok', message: 'restart sent' }), { status: 200 })
  }
  if (cleanPath.endsWith('/version')) {
    return new Response(JSON.stringify({ version: 'v1.18.8-meta' }), { status: 200 })
  }

  // 2. 内核配置
  if (cleanPath.endsWith('/configs')) {
    if (method === 'PATCH' || method === 'PUT') {
      const body = JSON.parse(options.body as string || '{}')
      Object.assign(mockConfigs, body)
      return new Response(JSON.stringify({ status: 'ok' }), { status: 200 })
    }
    return new Response(JSON.stringify(mockConfigs), { status: 200 })
  }

  // 3. 订阅管理
  if (cleanPath.endsWith('/subscribe/config')) {
    if (method === 'POST') {
      const body = JSON.parse(options.body as string || '{}')
      Object.assign(mockSubConfig, body)
      return new Response(JSON.stringify({ status: 'ok' }), { status: 200 })
    }
    return new Response(JSON.stringify(mockSubConfig), { status: 200 })
  }
  if (cleanPath.endsWith('/subscribe/generate')) {
    return new Response(JSON.stringify({ status: 'ok', message: 'Config generated and reloaded' }), { status: 200 })
  }


  // 4. 代理信息与测速
  if (cleanPath.endsWith('/proxies')) {
    return new Response(JSON.stringify({ proxies: mockProxies }), { status: 200 })
  }
  if (cleanPath.includes('/proxies/') && cleanPath.endsWith('/delay')) {
    const parts = cleanPath.split('/')
    const proxyName = decodeURIComponent(parts[parts.length - 2])
    const delay = Math.floor(30 + Math.random() * 200)
    if (mockProxies[proxyName]) {
      if (!mockProxies[proxyName].history) mockProxies[proxyName].history = []
      mockProxies[proxyName].history.push({ time: new Date().toISOString(), delay })
    }
    return new Response(JSON.stringify({ delay }), { status: 200 })
  }
  if (cleanPath.includes('/proxies/') && method === 'PUT') {
    const parts = cleanPath.split('/')
    const groupName = decodeURIComponent(parts[parts.length - 1])
    const body = JSON.parse(options.body as string || '{}')
    if (mockProxies[groupName]) {
      mockProxies[groupName].now = body.name
      return new Response(JSON.stringify({ status: 'ok' }), { status: 200 })
    }
  }

  // 5. 规则提供者与规则
  if (cleanPath.endsWith('/providers/rules')) {
    return new Response(JSON.stringify(mockRuleProviders), { status: 200 })
  }
  if (cleanPath.endsWith('/rules')) {
    return new Response(JSON.stringify(mockRules), { status: 200 })
  }

  // 6. 其他缓存清理
  if (cleanPath.includes('/cache/')) {
    return new Response(JSON.stringify({ status: 'ok' }), { status: 200 })
  }

  // 7. DNS 查询
  if (cleanPath.endsWith('/dns/query')) {
    return new Response(JSON.stringify({
      Status: 0,
      Answer: [{ data: '192.168.1.100' }, { data: '192.168.1.101' }]
    }), { status: 200 })
  }

  // 8. 系统网卡接口
  if (cleanPath.endsWith('/interfaces')) {
    return new Response(JSON.stringify(['eth0', 'wlan0', 'en0', 'meta', 'WLAN']), { status: 200 })
  }

  return new Response(JSON.stringify({ error: 'Not Found' }), { status: 404 })
}

// 模拟流量与内存状态
let lastUp = 120000
let lastDown = 850000
let lastConnId = 300

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
        // 缓动平滑算法，产生自然的网络波动
        const changeUp = (Math.random() - 0.48) * 0.35
        lastUp = Math.max(8000, Math.min(8000000, Math.floor(lastUp * (1 + changeUp))))
        const changeDown = (Math.random() - 0.48) * 0.35
        lastDown = Math.max(30000, Math.min(25000000, Math.floor(lastDown * (1 + changeDown))))
        dataStr = JSON.stringify({ up: lastUp, down: lastDown })
      } else if (path.includes('/memory')) {
        const inuse = Math.floor(38000000 + Math.random() * 6000000)
        dataStr = JSON.stringify({ inuse })
      } else if (path.includes('/logs')) {
        const logs = [
          '{"type":"info","payload":"[Proxy] switch Hong Kong to US"}',
          '{"type":"debug","payload":"[TCP] dial google.com:443 direct"}',
          '{"type":"warning","payload":"[DNS] fallback trigger for test.com"}',
          '{"type":"info","payload":"[Metadata] outbound connection processed successfully"}',
          '{"type":"info","payload":"[DNS] query apple.com A from 223.5.5.5"}',
          '{"type":"debug","payload":"[TCP] dial github.com:443 proxy via 节点选择"}'
        ]
        dataStr = logs[Math.floor(Math.random() * logs.length)]
      } else if (path.includes('/connections')) {
        // 动态活跃连接吞吐增加
        activeMockConns.forEach(conn => {
          conn.upload += Math.floor(Math.random() * 6000)
          conn.download += Math.floor(Math.random() * 65000)
        })

        // 10% 几率随机断开某个旧连接，以触发前端“已关闭连接列表”收集归档
        if (activeMockConns.length > 2 && Math.random() < 0.1) {
          const closeIdx = Math.floor(Math.random() * activeMockConns.length)
          activeMockConns.splice(closeIdx, 1)
        }

        // 15% 几率新生成一个活跃连接
        if (activeMockConns.length < 6 && Math.random() < 0.15) {
          const hosts = ['netflix.com', 'youtube.com', 'twitter.com', 'microsoft.com', 'steampowered.com']
          const rules = ['DomainSuffix', 'IPCIDR', 'Match']
          const chains = [
            ['GLOBAL', '节点选择', '香港 01 (XUDP)'],
            ['GLOBAL', '节点选择', '美国 01 [IPLc]'],
            ['GLOBAL', '自动选择', '新加坡 01']
          ]
          const randomIps = ['104.16.248.249', '142.250.66.46', '104.244.42.1', '23.59.248.10', '23.59.248.11']
          const targetIp = randomIps[Math.floor(Math.random() * randomIps.length)]
          const targetPort = Math.random() < 0.8 ? 443 : 80
          lastConnId++
          activeMockConns.push({
            id: `c-${lastConnId}`,
            metadata: {
              host: hosts[Math.floor(Math.random() * hosts.length)],
              destinationIP: targetIp,
              destinationPort: targetPort,
              type: Math.random() < 0.9 ? 'TLS' : 'HTTP',
              network: 'tcp'
            },
            upload: Math.floor(Math.random() * 500),
            download: Math.floor(Math.random() * 2000),
            rule: rules[Math.floor(Math.random() * rules.length)],
            chains: chains[Math.floor(Math.random() * chains.length)],
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
