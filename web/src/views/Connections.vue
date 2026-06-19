<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { wsConnect, apiFetch } from '../utils/api'
import { ArrowDownOutline, ArrowUpOutline } from '@vicons/ionicons5'

const { t } = useI18n()

export interface ConnectionMetadata {
  host: string
  destinationIP: string
  destinationPort: number
  sourcePort: number
  type: string
}

export interface ConnectionItem {
  id: string
  metadata: ConnectionMetadata
  rule: string
  chains: string[]
  upload: number
  download: number
  start: string
}

export interface ConnectionItem {
  id: string
  metadata: ConnectionMetadata
  rule: string
  chains: string[]
  upload: number
  download: number
  start: string
  speedUp?: number
  speedDown?: number
  closedAt?: string
}

const activeConnections = ref<ConnectionItem[]>([])
const closedConnections = ref<ConnectionItem[]>([])
const activeTab = ref<'active' | 'closed'>('active')
const isPaused = ref(false)
const searchText = ref('')
const sortBy = ref<'id' | 'uploadSpeed' | 'downloadSpeed' | 'duration'>('id')
const sortDesc = ref(true)

let ws: WebSocket | null = null
const prevSnapshot = new Map<string, { upload: number, download: number, timestamp: number }>()

const connectWS = () => {
  if (ws) ws.close()
  ws = wsConnect('/connections', (e: MessageEvent) => {
    if (isPaused.value) return
    try {
      const data = JSON.parse(e.data)
      const newActiveList: any[] = data.connections || []
      const now = performance.now()

      // 计算速度与构建新活跃 Map
      const newActiveMap = new Map<string, ConnectionItem>()
      newActiveList.forEach(conn => {
        const prev = prevSnapshot.get(conn.id)
        let speedUp = 0
        let speedDown = 0
        if (prev) {
          const timeDiff = (now - prev.timestamp) / 1000
          if (timeDiff > 0.05) {
            speedUp = Math.max(0, (conn.upload - prev.upload) / timeDiff)
            speedDown = Math.max(0, (conn.download - prev.download) / timeDiff)
          }
        }
        newActiveMap.set(conn.id, {
          ...conn,
          speedUp,
          speedDown
        })
      })

      // 找出在此次快照中消失（已关闭）的连接
      activeConnections.value.forEach(prevConn => {
        if (!newActiveMap.has(prevConn.id)) {
          closedConnections.value.unshift({
            ...prevConn,
            speedUp: 0,
            speedDown: 0,
            closedAt: new Date().toLocaleTimeString()
          })
        }
      })

      // 限制已关闭列表最大 100 条
      if (closedConnections.value.length > 100) {
        closedConnections.value = closedConnections.value.slice(0, 100)
      }

      // 缓存快照用于下次计算速度
      prevSnapshot.clear()
      newActiveList.forEach(conn => {
        prevSnapshot.set(conn.id, {
          upload: conn.upload,
          download: conn.download,
          timestamp: now
        })
      })

      activeConnections.value = Array.from(newActiveMap.values())
    } catch (err) {
      console.warn('解析连接数据失败', err)
    }
  })
}

// 模糊匹配搜索和排序
const filteredConnections = computed(() => {
  const query = searchText.value.trim().toLowerCase()
  let list = activeTab.value === 'active' 
    ? [...activeConnections.value] 
    : [...closedConnections.value]

  if (query) {
    list = list.filter(c => 
      c.metadata.host.toLowerCase().includes(query) || 
      c.metadata.destinationIP.toLowerCase().includes(query) || 
      c.metadata.sourcePort.toString().includes(query) || 
      c.rule.toLowerCase().includes(query) || 
      c.metadata.type.toLowerCase().includes(query)
    )
  }

  list.sort((a, b) => {
    let valA = 0
    let valB = 0

    if (sortBy.value === 'uploadSpeed') {
      valA = activeTab.value === 'active' ? (a.speedUp || 0) : a.upload
      valB = activeTab.value === 'active' ? (b.speedUp || 0) : b.upload
    } else if (sortBy.value === 'downloadSpeed') {
      valA = activeTab.value === 'active' ? (a.speedDown || 0) : a.download
      valB = activeTab.value === 'active' ? (b.speedDown || 0) : b.download
    } else if (sortBy.value === 'duration') {
      valA = new Date(a.start).getTime()
      valB = new Date(b.start).getTime()
      return sortDesc.value ? valA - valB : valB - valA
    } else {
      return sortDesc.value 
        ? b.id.localeCompare(a.id) 
        : a.id.localeCompare(b.id)
    }

    if (valA === valB) return 0
    const res = valA > valB ? 1 : -1
    return sortDesc.value ? -res : res
  })

  return list
})

const handleSort = (field: 'id' | 'uploadSpeed' | 'downloadSpeed' | 'duration') => {
  if (sortBy.value === field) {
    sortDesc.value = !sortDesc.value
  } else {
    sortBy.value = field
    sortDesc.value = true
  }
}

// 断开单个连接
const handleCloseConnection = async (id: string) => {
  try {
    const resp = await apiFetch(`/connections/${id}`, { method: 'DELETE' })
    if (resp.ok) {
      // 乐观更新：将此连接移到 closed 列表中
      const conn = activeConnections.value.find(c => c.id === id)
      if (conn) {
        closedConnections.value.unshift({
          ...conn,
          speedUp: 0,
          speedDown: 0,
          closedAt: new Date().toLocaleTimeString()
        })
      }
      activeConnections.value = activeConnections.value.filter(c => c.id !== id)
    }
  } catch (e) {
    console.error('断开连接失败', e)
  }
}

// 全部断开连接
const handleCloseAll = async () => {
  if (confirm(t('connections.confirm_close_all'))) {
    try {
      const resp = await apiFetch('/connections', { method: 'DELETE' })
      if (resp.ok) {
        // 移入 closed
        activeConnections.value.forEach(conn => {
          closedConnections.value.unshift({
            ...conn,
            speedUp: 0,
            speedDown: 0,
            closedAt: new Date().toLocaleTimeString()
          })
        })
        activeConnections.value = []
      }
    } catch (e) {
      console.error('批量断开连接失败', e)
    }
  }
}

// 清除单个已关闭记录
const handleClearClosedItem = (id: string) => {
  closedConnections.value = closedConnections.value.filter(c => c.id !== id)
}

// 清除全部已关闭记录
const handleClearAllClosed = () => {
  if (confirm(t('connections.confirm_clear_closed'))) {
    closedConnections.value = []
  }
}

// 格式转换辅助
const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(Math.abs(bytes) || 1) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

const formatSpeed = (bytes: number) => {
  return formatBytes(bytes) + '/s'
}

const formatDuration = (startTime: string) => {
  const diff = Date.now() - new Date(startTime).getTime()
  const sec = Math.floor(diff / 1000)
  if (sec < 60) return `${sec}s`
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min}m`
  const hour = Math.floor(min / 60)
  return `${hour}h ${min % 60}m`
}

onMounted(() => {
  connectWS()
})

onUnmounted(() => {
  if (ws) ws.close()
})

</script>

<template>
  <div class="space-y-4">
    <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm flex flex-wrap gap-4 items-center justify-between transition-all">
      <div class="flex items-center gap-4 flex-wrap">
        <h3 class="text-base font-semibold flex items-center gap-2">
          {{ t('connections.title') }}
        </h3>
        <div class="flex bg-slate-100 dark:bg-slate-800 p-1 rounded-lg">
          <button @click="activeTab = 'active'" class="px-4 py-1 text-xs font-semibold rounded-md transition-all" :class="activeTab === 'active' ? 'bg-white dark:bg-slate-700 shadow-sm text-slate-800 dark:text-slate-100' : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'">
            {{ t('connections.active') }} ({{ activeConnections.length }})
          </button>
          <button @click="activeTab = 'closed'" class="px-4 py-1 text-xs font-semibold rounded-md transition-all" :class="activeTab === 'closed' ? 'bg-white dark:bg-slate-700 shadow-sm text-slate-800 dark:text-slate-100' : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'">
            {{ t('connections.closed') }} ({{ closedConnections.length }})
          </button>
        </div>
      </div>

      <div class="flex gap-2 items-center flex-1 sm:flex-initial min-w-[200px] sm:min-w-0">
        <input type="text" v-model="searchText" :placeholder="t('connections.search_placeholder')" class="w-full sm:w-60 px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none" />
      </div>

      <div class="flex gap-2">
        <button v-if="activeTab === 'active'" @click="isPaused = !isPaused" class="px-4 py-1.5 text-xs font-semibold rounded-lg border transition-all" :class="isPaused ? 'bg-amber-500/10 text-amber-500 border-amber-500/20' : 'bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 border-transparent'">
          {{ isPaused ? t('connections.resume') : t('connections.pause') }}
        </button>
        <button v-if="activeTab === 'active'" @click="handleCloseAll" class="px-4 py-1.5 text-xs font-semibold rounded-lg bg-red-500/10 hover:bg-red-500/20 text-red-500 transition-all border border-red-500/10">
          {{ t('connections.close_all') }}
        </button>
        <button v-if="activeTab === 'closed'" @click="handleClearAllClosed" class="px-4 py-1.5 text-xs font-semibold rounded-lg bg-red-500/10 hover:bg-red-500/20 text-red-500 transition-all border border-red-500/10">
          {{ t('connections.clear_all_closed') }}
        </button>
      </div>
    </div>

    <div class="bg-white dark:bg-[#1e293b] rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm overflow-hidden transition-all">
      <div class="overflow-x-auto">
        <table class="w-full text-left text-xs border-collapse">
          <thead>
            <tr class="bg-slate-50 dark:bg-slate-800/50 border-b border-slate-100 dark:border-slate-800 text-slate-500 dark:text-slate-400 font-semibold select-none">
              <th class="py-3 px-4">{{ t('connections.host') }}</th>
              <th class="py-3 px-4 shrink-0 w-20 text-center">{{ t('connections.type') }}</th>
              <th class="py-3 px-4">{{ t('connections.rule') }}</th>
              <th class="py-3 px-4">{{ t('connections.chain') }}</th>
              <th class="py-3 px-4 cursor-pointer hover:bg-slate-100 dark:hover:bg-slate-800/80 transition-colors" @click="handleSort('uploadSpeed')">
                {{ activeTab === 'active' ? t('connections.upload_speed') : t('connections.total_upload') }}
                <span v-if="sortBy === 'uploadSpeed'" class="inline-flex align-middle ml-0.5">
                  <ArrowDownOutline v-if="sortDesc" class="w-3.5 h-3.5" />
                  <ArrowUpOutline v-else class="w-3.5 h-3.5" />
                </span>
              </th>
              <th class="py-3 px-4 cursor-pointer hover:bg-slate-100 dark:hover:bg-slate-800/80 transition-colors" @click="handleSort('downloadSpeed')">
                {{ activeTab === 'active' ? t('connections.download_speed') : t('connections.total_download') }}
                <span v-if="sortBy === 'downloadSpeed'" class="inline-flex align-middle ml-0.5">
                  <ArrowDownOutline v-if="sortDesc" class="w-3.5 h-3.5" />
                  <ArrowUpOutline v-else class="w-3.5 h-3.5" />
                </span>
              </th>
              <th class="py-3 px-4 cursor-pointer hover:bg-slate-100 dark:hover:bg-slate-800/80 transition-colors" @click="handleSort('duration')">
                {{ activeTab === 'active' ? t('connections.duration') : t('connections.closed_at') }}
                <span v-if="sortBy === 'duration'" class="inline-flex align-middle ml-0.5">
                  <ArrowDownOutline v-if="sortDesc" class="w-3.5 h-3.5" />
                  <ArrowUpOutline v-else class="w-3.5 h-3.5" />
                </span>
              </th>
              <th class="py-3 px-4 shrink-0 w-16 text-center">{{ t('connections.action') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100 dark:divide-slate-800">
            <tr v-if="filteredConnections.length === 0" class="text-slate-400 dark:text-slate-600 text-center">
              <td colspan="8" class="py-8 text-sm">{{ t('connections.empty') }}</td>
            </tr>
            <tr v-else v-for="c in filteredConnections" :key="c.id" class="hover:bg-slate-50/50 dark:hover:bg-slate-900/10 transition-colors">
              <td class="py-3 px-4">
                <div class="font-medium text-slate-800 dark:text-slate-200 select-all max-w-[200px] sm:max-w-[300px] truncate break-all" :title="c.metadata.host || c.metadata.destinationIP">
                  {{ c.metadata.host || c.metadata.destinationIP }}
                </div>
                <div class="text-[10px] text-slate-400 dark:text-slate-500 select-all">{{ c.metadata.destinationIP }}:{{ c.metadata.destinationPort }}</div>
              </td>
              <td class="py-3 px-4 text-center shrink-0">
                <span class="px-1.5 py-0.5 text-[10px] font-bold rounded tracking-wide uppercase" :class="c.metadata.type === 'UDP' ? 'bg-amber-500/10 text-amber-500' : 'bg-blue-500/10 text-blue-500'">
                  {{ c.metadata.type }}
                </span>
              </td>
              <td class="py-3 px-4 text-slate-600 dark:text-slate-300 font-medium">
                {{ c.rule }}
              </td>
              <td class="py-3 px-4 text-slate-400 select-all font-mono text-[10px]">
                {{ c.chains.join(' → ') }}
              </td>
              <td class="py-3 px-4 font-mono font-medium text-blue-500">
                {{ activeTab === 'active' ? formatSpeed(c.speedUp || 0) : formatBytes(c.upload) }}
              </td>
              <td class="py-3 px-4 font-mono font-medium text-success">
                {{ activeTab === 'active' ? formatSpeed(c.speedDown || 0) : formatBytes(c.download) }}
              </td>
              <td class="py-3 px-4 font-mono text-slate-500">
                {{ activeTab === 'active' ? formatDuration(c.start) : c.closedAt }}
              </td>
              <td class="py-3 px-4 text-center shrink-0">
                <button v-if="activeTab === 'active'" @click="handleCloseConnection(c.id)" class="px-2 py-1 bg-red-500/10 hover:bg-red-500/20 text-red-500 rounded font-semibold text-[10px] transition-all">
                  {{ t('connections.close') }}
                </button>
                <button v-else @click="handleClearClosedItem(c.id)" class="px-2 py-1 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-500 dark:text-slate-400 rounded font-semibold text-[10px] transition-all">
                  {{ t('connections.clear') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

