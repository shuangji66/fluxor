<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiFetch } from '../utils/api'
import { ArrowDownOutline, ArrowUpOutline } from '@vicons/ionicons5'
import { storeToRefs } from 'pinia'
import { useConnectionsStore, type ConnectionItem } from '../store/connections'
import { useGlobalStore } from '../store/global'

const { t } = useI18n()
const globalStore = useGlobalStore()

const connStore = useConnectionsStore()
const { activeConnections, closedConnections, isPaused } = storeToRefs(connStore)

const activeTab = ref<'active' | 'closed'>('active')
const searchText = ref('')
const sortBy = ref<'host' | 'rule' | 'chain' | 'uploadSpeed' | 'downloadSpeed' | 'duration'>('duration')
const sortDesc = ref(true)

// 模糊匹配搜索和排序
const filteredConnections = computed(() => {
  const query = searchText.value.trim().toLowerCase()
  let list = activeTab.value === 'active' 
    ? [...activeConnections.value] 
    : [...closedConnections.value]

  if (query) {
    list = list.filter(c => {
      const host = (c.metadata?.host || '').toLowerCase()
      const ip = (c.metadata?.destinationIP || '').toLowerCase()
      const port = (c.metadata?.destinationPort || '').toString()
      const rule = (c.rule || '').toLowerCase()
      const type = (c.metadata?.type || '').toLowerCase()
      const network = (c.metadata?.network || '').toLowerCase()
      const chainStr = (c.chains || []).join(' ').toLowerCase()
      return host.includes(query) || 
             ip.includes(query) || 
             port.includes(query) || 
             rule.includes(query) || 
             type.includes(query) || 
             network.includes(query) || 
             chainStr.includes(query)
    })
  }

  list.sort((a, b) => {
    if (sortBy.value === 'uploadSpeed') {
      const valA = activeTab.value === 'active' ? (a.speedUp || 0) : a.upload
      const valB = activeTab.value === 'active' ? (b.speedUp || 0) : b.upload
      if (valA === valB) return 0
      const res = valA > valB ? 1 : -1
      return sortDesc.value ? -res : res
    } else if (sortBy.value === 'downloadSpeed') {
      const valA = activeTab.value === 'active' ? (a.speedDown || 0) : a.download
      const valB = activeTab.value === 'active' ? (b.speedDown || 0) : b.download
      if (valA === valB) return 0
      const res = valA > valB ? 1 : -1
      return sortDesc.value ? -res : res
    } else if (sortBy.value === 'duration') {
      if (activeTab.value === 'active') {
        const valA = new Date(a.start).getTime()
        const valB = new Date(b.start).getTime()
        return sortDesc.value ? valA - valB : valB - valA
      } else {
        const valA = a.closedAtTimestamp || 0
        const valB = b.closedAtTimestamp || 0
        return sortDesc.value ? valB - valA : valA - valB
      }
    } else if (sortBy.value === 'host') {
      const valA = a.metadata?.host || a.metadata?.destinationIP || ''
      const valB = b.metadata?.host || b.metadata?.destinationIP || ''
      return sortDesc.value ? valB.localeCompare(valA) : valA.localeCompare(valB)
    } else if (sortBy.value === 'rule') {
      const valA = a.rule || ''
      const valB = b.rule || ''
      return sortDesc.value ? valB.localeCompare(valA) : valA.localeCompare(valB)
    } else if (sortBy.value === 'chain') {
      const valA = (a.chains || []).join(' → ')
      const valB = (b.chains || []).join(' → ')
      return sortDesc.value ? valB.localeCompare(valA) : valA.localeCompare(valB)
    }
    return 0
  })

  return list
})

const handleSort = (field: 'host' | 'rule' | 'chain' | 'uploadSpeed' | 'downloadSpeed' | 'duration') => {
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
          closedAt: new Date().toLocaleTimeString(),
          closedAtTimestamp: Date.now()
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
  const ok = await globalStore.showConfirm(t('connections.confirm_close_all'))
  if (ok) {
    try {
      const resp = await apiFetch('/connections', { method: 'DELETE' })
      if (resp.ok) {
        // 移入 closed
        activeConnections.value.forEach(conn => {
          closedConnections.value.unshift({
            ...conn,
            speedUp: 0,
            speedDown: 0,
            closedAt: new Date().toLocaleTimeString(),
            closedAtTimestamp: Date.now()
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
  connStore.removeClosedConnection(id)
}

// 清除全部已关闭记录
const handleClearAllClosed = async () => {
  const ok = await globalStore.showConfirm(t('connections.confirm_clear_closed'))
  if (ok) {
    connStore.clearClosedConnections()
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
  connStore.subscribe()
})

onUnmounted(() => {
  connStore.unsubscribe()
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

      <!-- 右侧：搜索框与控制按钮合并容器（在大屏下横向右对齐，绝不换行） -->
      <div class="flex items-center gap-3 flex-1 justify-end min-w-[280px] sm:min-w-0 flex-nowrap">
        <input type="text" v-model="searchText" :placeholder="t('connections.search_placeholder')" class="w-full sm:w-60 px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none" />
        
        <div class="flex gap-2 shrink-0">
          <button v-if="activeTab === 'active'" @click="isPaused = !isPaused" class="px-4 py-1.5 text-xs font-semibold rounded-lg border transition-all whitespace-nowrap" :class="isPaused ? 'bg-amber-500/10 text-amber-500 border-amber-500/20' : 'bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 border-transparent'">
            {{ isPaused ? t('connections.resume') : t('connections.pause') }}
          </button>
          <button v-if="activeTab === 'active'" @click="handleCloseAll" class="px-4 py-1.5 text-xs font-semibold rounded-lg bg-red-500/10 hover:bg-red-500/20 text-red-500 transition-all border border-red-500/10 whitespace-nowrap">
            {{ t('connections.close_all') }}
          </button>
          <button v-if="activeTab === 'closed'" @click="handleClearAllClosed" class="px-4 py-1.5 text-xs font-semibold rounded-lg bg-red-500/10 hover:bg-red-500/20 text-red-500 transition-all border border-red-500/10 whitespace-nowrap">
            {{ t('connections.clear_all_closed') }}
          </button>
        </div>
      </div>
    </div>

    <div class="bg-white dark:bg-[#1e293b] rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm overflow-hidden transition-all">
      <!-- 桌面端表格视图 -->
      <div class="hidden md:block overflow-x-auto">
        <table class="w-full text-left text-xs border-collapse">
          <thead>
            <tr class="bg-slate-50 dark:bg-slate-800/50 border-b border-slate-100 dark:border-slate-800 text-slate-500 dark:text-slate-400 font-semibold select-none">
              <th class="py-3 px-4 cursor-pointer hover:bg-slate-100 dark:hover:bg-slate-800/80 transition-colors" @click="handleSort('host')">
                {{ t('connections.host') }}
                <span v-if="sortBy === 'host'" class="inline-flex align-middle ml-0.5">
                  <ArrowDownOutline v-if="sortDesc" class="w-3.5 h-3.5" />
                  <ArrowUpOutline v-else class="w-3.5 h-3.5" />
                </span>
              </th>
              <th class="py-3 px-4 shrink-0 w-20 text-center">{{ t('connections.type') }}</th>
              <th class="hidden md:table-cell py-3 px-4 cursor-pointer hover:bg-slate-100 dark:hover:bg-slate-800/80 transition-colors" @click="handleSort('rule')">
                {{ t('connections.rule') }}
                <span v-if="sortBy === 'rule'" class="inline-flex align-middle ml-0.5">
                  <ArrowDownOutline v-if="sortDesc" class="w-3.5 h-3.5" />
                  <ArrowUpOutline v-else class="w-3.5 h-3.5" />
                </span>
              </th>
              <th class="hidden lg:table-cell py-3 px-4 cursor-pointer hover:bg-slate-100 dark:hover:bg-slate-800/80 transition-colors" @click="handleSort('chain')">
                {{ t('connections.chain') }}
                <span v-if="sortBy === 'chain'" class="inline-flex align-middle ml-0.5">
                  <ArrowDownOutline v-if="sortDesc" class="w-3.5 h-3.5" />
                  <ArrowUpOutline v-else class="w-3.5 h-3.5" />
                </span>
              </th>
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
              <th class="py-3 px-4 shrink-0 w-20 text-center whitespace-nowrap">{{ t('connections.action') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100 dark:divide-slate-800">
            <tr v-if="filteredConnections.length === 0" class="text-slate-400 dark:text-slate-600 text-center">
              <td colspan="8" class="py-8 text-sm">{{ t('connections.empty') }}</td>
            </tr>
            <tr v-else v-for="c in filteredConnections" :key="c.id" class="hover:bg-slate-50/50 dark:hover:bg-slate-900/10 transition-colors">
              <td class="py-3 px-4">
                <div class="flex items-center gap-1.5 flex-wrap">
                  <span v-if="c.metadata?.type" class="px-1 py-0.5 text-[9px] font-semibold bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 rounded tracking-wide uppercase">
                    {{ c.metadata.type }}
                  </span>
                  <div class="font-medium text-slate-800 dark:text-slate-200 select-all max-w-[150px] sm:max-w-[250px] truncate break-all" :title="c.metadata?.host || c.metadata?.destinationIP">
                    {{ c.metadata?.host || c.metadata?.destinationIP }}
                  </div>
                </div>
                <div v-if="c.metadata?.destinationIP" class="text-[10px] text-slate-400 dark:text-slate-500 select-all">{{ c.metadata.destinationIP }}{{ c.metadata.destinationPort ? ':' + c.metadata.destinationPort : '' }}</div>
              </td>
              <td class="py-3 px-4 text-center shrink-0">
                <span class="px-1.5 py-0.5 text-[10px] font-bold rounded tracking-wide uppercase" :class="(c.metadata?.network || 'tcp').toUpperCase() === 'UDP' ? 'bg-amber-500/10 text-amber-500' : 'bg-blue-500/10 text-blue-500'">
                  {{ c.metadata?.network || 'tcp' }}
                </span>
              </td>
              <td class="hidden md:table-cell py-3 px-4 text-slate-600 dark:text-slate-300 font-medium">
                {{ c.rule }}
              </td>
              <td class="hidden lg:table-cell py-3 px-4 text-slate-400 select-all font-mono text-[10px]">
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
              <td class="py-3 px-4 text-center shrink-0 whitespace-nowrap">
                <button v-if="activeTab === 'active'" @click="handleCloseConnection(c.id)" class="px-2 py-1 bg-red-500/10 hover:bg-red-500/20 text-red-500 rounded font-semibold text-[10px] transition-all whitespace-nowrap">
                  {{ t('connections.close') }}
                </button>
                <button v-else @click="handleClearClosedItem(c.id)" class="px-2 py-1 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-500 dark:text-slate-400 rounded font-semibold text-[10px] transition-all whitespace-nowrap">
                  {{ t('connections.clear') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 移动端卡片视图 -->
      <div class="md:hidden divide-y divide-slate-100 dark:divide-slate-800">
        <div v-if="filteredConnections.length === 0" class="py-8 text-center text-slate-400 dark:text-slate-600 text-sm">
          {{ t('connections.empty') }}
        </div>
        <div
          v-else
          v-for="c in filteredConnections"
          :key="c.id"
          class="p-4 space-y-3 transition-colors hover:bg-slate-50/50 dark:hover:bg-slate-900/10"
        >
          <!-- 第一行：名称 + 网络类型 & 协议 -->
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <div class="font-bold text-slate-800 dark:text-slate-200 select-all break-all text-xs leading-snug">
                {{ c.metadata?.host || c.metadata?.destinationIP }}
              </div>
              <div v-if="c.metadata?.destinationIP" class="text-[10px] text-slate-400 dark:text-slate-500 mt-0.5 select-all">
                {{ c.metadata.destinationIP }}{{ c.metadata.destinationPort ? ':' + c.metadata.destinationPort : '' }}
              </div>
            </div>
            <div class="flex items-center gap-1.5 shrink-0 select-none">
              <span v-if="c.metadata?.type" class="px-1.5 py-0.5 text-[9px] font-semibold bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 rounded uppercase">
                {{ c.metadata.type }}
              </span>
              <span class="px-1.5 py-0.5 text-[9px] font-bold rounded uppercase" :class="(c.metadata?.network || 'tcp').toUpperCase() === 'UDP' ? 'bg-amber-500/10 text-amber-500' : 'bg-blue-500/10 text-blue-500'">
                {{ c.metadata?.network || 'tcp' }}
              </span>
            </div>
          </div>

          <!-- 第二行：分流规则 & 链路 -->
          <div class="text-[10px] space-y-1.5">
            <div class="flex items-center gap-1">
              <span class="text-slate-400">{{ t('connections.rule') }}:</span>
              <span class="font-medium text-slate-600 dark:text-slate-300 bg-slate-100/50 dark:bg-slate-800/50 px-1.5 py-0.5 rounded">{{ c.rule }}</span>
            </div>
            <div v-if="c.chains && c.chains.length > 0" class="flex items-start gap-1">
              <span class="text-slate-400 shrink-0">{{ t('connections.chain') }}:</span>
              <span class="font-mono text-slate-500 dark:text-slate-400 leading-tight break-all">{{ c.chains.join(' → ') }}</span>
            </div>
          </div>

          <!-- 第三行：实时流量/总量 + 时间 + 断开操作 -->
          <div class="flex items-center justify-between gap-3 pt-1">
            <div class="flex gap-4 text-[10px]">
              <div class="flex flex-col">
                <span class="text-slate-400">{{ activeTab === 'active' ? t('connections.upload_speed') : t('connections.total_upload') }}</span>
                <span class="font-mono font-bold text-blue-500 mt-0.5">
                  {{ activeTab === 'active' ? formatSpeed(c.speedUp || 0) : formatBytes(c.upload) }}
                </span>
              </div>
              <div class="flex flex-col">
                <span class="text-slate-400">{{ activeTab === 'active' ? t('connections.download_speed') : t('connections.total_download') }}</span>
                <span class="font-mono font-bold text-success mt-0.5">
                  {{ activeTab === 'active' ? formatSpeed(c.speedDown || 0) : formatBytes(c.download) }}
                </span>
              </div>
              <div class="flex flex-col">
                <span class="text-slate-400">{{ activeTab === 'active' ? t('connections.duration') : t('connections.closed_at') }}</span>
                <span class="font-mono text-slate-500 mt-0.5">
                  {{ activeTab === 'active' ? formatDuration(c.start) : c.closedAt }}
                </span>
              </div>
            </div>

            <div class="shrink-0 select-none">
              <button v-if="activeTab === 'active'" @click="handleCloseConnection(c.id)" class="px-2.5 py-1 bg-red-500/10 hover:bg-red-500/20 text-red-500 rounded-md font-semibold text-[10px] transition-all">
                {{ t('connections.close') }}
              </button>
              <button v-else @click="handleClearClosedItem(c.id)" class="px-2.5 py-1 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-500 dark:text-slate-400 rounded-md font-semibold text-[10px] transition-all">
                {{ t('connections.clear') }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

