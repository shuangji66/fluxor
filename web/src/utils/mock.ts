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
    all: ['节点选择', 'DIRECT', 'REJECT']
  },
  '节点选择': {
    name: '节点选择',
    type: 'Selector',
    now: '香港 01 [IPLc]',
    all: ['香港 01 [IPLc]', '日本 01 [IPLc]', '美国 01 [IPLc]', 'DIRECT']
  },
  '香港 01 [IPLc]': {
    name: '香港 01 [IPLc]',
    type: 'Shadowsocks',
    udp: true,
    history: [{ time: new Date().toISOString(), delay: 45 }]
  },
  '日本 01 [IPLc]': {
    name: '日本 01 [IPLc]',
    type: 'Vmess',
    udp: true,
    history: [{ time: new Date().toISOString(), delay: 120 }]
  },
  '美国 01 [IPLc]': {
    name: '美国 01 [IPLc]',
    type: 'Trojan',
    udp: false,
    history: []
  }
}

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
    return new Response(JSON.stringify({ providers: {} }), { status: 200 })
  }
  if (cleanPath.endsWith('/rules')) {
    return new Response(JSON.stringify({ rules: [] }), { status: 200 })
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
        const up = Math.floor(Math.random() * 2000000)
        const down = Math.floor(Math.random() * 8000000)
        dataStr = JSON.stringify({ up, down })
      } else if (path.includes('/memory')) {
        const inuse = Math.floor(32000000 + Math.random() * 10000000)
        dataStr = JSON.stringify({ inuse })
      } else if (path.includes('/logs')) {
        const logs = [
          '{"type":"info","payload":"[Proxy] switch Hong Kong to US"}',
          '{"type":"debug","payload":"[TCP] dial google.com:443 direct"}',
          '{"type":"warning","payload":"[DNS] fallback trigger for test.com"}',
          '{"type":"info","payload":"[Metadata] outbound connection processed successfully"}'
        ]
        dataStr = logs[Math.floor(Math.random() * logs.length)]
      } else if (path.includes('/connections')) {
        dataStr = JSON.stringify({
          connections: [
            {
              id: 'c-100',
              metadata: { host: 'github.com', port: 443, type: 'TLS', network: 'tcp' },
              upload: Math.floor(Math.random() * 200000),
              download: Math.floor(Math.random() * 5000000),
              start: new Date(Date.now() - 120000).toISOString()
            },
            {
              id: 'c-200',
              metadata: { host: 'gemini.google.com', port: 443, type: 'HTTP', network: 'tcp' },
              upload: Math.floor(Math.random() * 5000000),
              download: Math.floor(Math.random() * 100000000),
              start: new Date(Date.now() - 45000).toISOString()
            }
          ]
        })
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
