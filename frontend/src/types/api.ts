/**
 * 内核（Mihomo）HTTP API 的响应形状。
 *
 * 前端此前把这些响应当作 `any` 处理，字段名拼错、类型错（`delay` 可能是字符串/负数、
 * `chains` 可能缺失）都只能靠运行时才发现。这里只收拢**前端确实读到**的字段，
 * 其余一律可选、按需放宽——目的是让 store 的解析代码有契约可依，
 * 而不是完整复刻内核 API。
 */
export interface ConnectionMetadata {
  host?: string
  destinationIP?: string
  destinationPort?: string
  sourceIP?: string
  sourcePort?: string
  network?: string
  type?: string
  process?: string
  processPath?: string
  dscp?: number
  sniffHost?: string
  remoteDestination?: string
  inboundName?: string
  inboundIP?: string
  inboundUser?: string
}

export interface ConnectionWire {
  id: string
  metadata?: ConnectionMetadata
  upload: number
  download: number
  start: string
  chains?: string[]
  rule?: string
  rulePayload?: string
}

/** `/connections` WebSocket 的每帧负载 */
export interface ConnectionsFrame {
  connections?: ConnectionWire[]
  uploadTotal?: number
  downloadTotal?: number
  memory?: number
}

export interface ProxyHistoryPoint {
  time: string
  delay: number
}

export interface ProxyWire {
  name?: string
  type?: string
  now?: string
  all?: string[]
  history?: ProxyHistoryPoint[]
  udp?: boolean
  /** 融合模式下来自 provider 的节点会带这个字段（用于单节点健康检查路由） */
  'provider-name'?: string
  [key: string]: unknown
}

export interface ProxiesFrame {
  proxies?: Record<string, ProxyWire>
}

export interface ProvidersFrame {
  providers?: Record<string, { name?: string; proxies?: ProxyWire[]; [key: string]: unknown }>
}

/** `/group/{name}/delay` 与单节点测速接口的返回：节点名 → 毫秒 */
export type DelayMap = Record<string, number>

export interface RuleWire {
  type: string
  payload: string
  proxy: string
  size?: number
  extraPayload?: string
}

export interface RulesFrame {
  rules?: RuleWire[]
}

export interface RulesProvidersFrame {
  providers?: Record<string, { name?: string; type?: string; updatedAt?: string; [key: string]: unknown }>
}
