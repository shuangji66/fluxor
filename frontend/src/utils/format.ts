/**
 * 展示层的格式化工具（纯函数，无副作用、无语言依赖）。
 *
 * 收拢此前散落在各视图里的同族实现。两份拷贝最容易出现的分歧是**精度**：
 * 概览页曾用 2 位小数、连接页用 1 位，同一个速度在不同页面显示成不同数字。
 * 这里统一为 2 位（`decimals` 可按需覆盖），订阅流量卡固定 GB 口径单独保留。
 *
 * 约定：本文件只做数值/时间 → 字符串的机械转换；凡是需要 i18n 文案的地方
 * （例如「未知时间」「永久有效」）由调用方在取值失败时套用 `t()`。
 */

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB'] as const

/** 字节数 → 人类可读（1024 进制，默认 2 位小数）。 */
export function formatBytes(bytes: number, decimals = 2): string {
  if (!bytes) return '0 B'
  const k = 1024
  const i = Math.min(Math.floor(Math.log(Math.abs(bytes)) / Math.log(k)), BYTE_UNITS.length - 1)
  const value = bytes / Math.pow(k, i)
  return `${parseFloat(value.toFixed(decimals))} ${BYTE_UNITS[i]}`
}

/** 速率 → 人类可读（自动补 `/s`）。 */
export function formatSpeed(bytesPerSecond: number, decimals = 2): string {
  return `${formatBytes(bytesPerSecond, decimals)}/s`
}

/** 订阅流量卡口径：固定按 GB 展示、1 位小数（避免 GB/TB 随用量跳变）。 */
export function formatGB(bytes: number): string {
  if (!bytes) return '0.0 GB'
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`
}

/**
 * `YYYY-MM-DD HH:mm`（本地时区）。
 * 入参无效或为 Go 零值时间（以 `0001` 开头）时返回 null，由调用方决定回退文案。
 */
export function formatDateTime(dateStr: string | null | undefined): string | null {
  if (!dateStr || dateStr.startsWith('0001')) return null
  const date = new Date(dateStr)
  if (Number.isNaN(date.getTime())) return null
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

/** 本地化完整时间串（如连接详情里的开始时间）；解析失败原样返回。 */
export function formatTimestamp(timeStr: string | null | undefined): string {
  if (!timeStr) return '-'
  const date = new Date(timeStr)
  if (Number.isNaN(date.getTime())) return timeStr
  return date.toLocaleString()
}

/** 从连接开始时间算起的持续时长，粗粒度到 `s` / `m` / `h m`。 */
export function formatDuration(startTime: string | null | undefined): string {
  if (!startTime) return '-'
  const started = new Date(startTime).getTime()
  if (Number.isNaN(started)) return '-'
  const sec = Math.floor((Date.now() - started) / 1000)
  if (sec < 60) return `${sec}s`
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min}m`
  const hour = Math.floor(min / 60)
  return `${hour}h ${min % 60}m`
}
