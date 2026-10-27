<script setup lang="ts">
import { ref, onMounted, onUnmounted, onActivated, onDeactivated, computed, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiFetch, readErrorMessage } from '../utils/api'
import { formatBytes, formatSpeed, formatTimestamp, formatDuration } from '../utils/format'
import { useViewActive } from '../composables/useViewActive'
import PageHeader from '../components/PageHeader.vue'
import AppModal from '../components/AppModal.vue'
import {
  ArrowDownOutline,
  ArrowUpOutline,
  LinkOutline,
  SettingsOutline,
  CloseOutline,
  InformationCircleOutline,
  BarChartOutline,
  ServerOutline,
  PeopleOutline,
  DocumentTextOutline,
  GitBranchOutline,
  RocketOutline,
  CloudOutline,
  PauseOutline,
  PlayOutline
} from '@vicons/ionicons5'
import { storeToRefs } from 'pinia'
import { useConnectionsStore } from '../store/connections'
import { useGlobalStore } from '../store/global'

const { t } = useI18n()
const globalStore = useGlobalStore()

const connStore = useConnectionsStore()
const { activeConnections, closedConnections, isPaused, isWsConnected } = storeToRefs(connStore)

const activeTab = ref<'active' | 'closed'>('active')
const searchText = ref('')
const sortBy = ref<'host' | 'rule' | 'chain' | 'uploadSpeed' | 'downloadSpeed' | 'duration'>('duration')
const sortDesc = ref(true)

// ========== 移动端检测 ==========
const isMobile = ref(window.innerWidth < 768)
const handleResize = () => {
  isMobile.value = window.innerWidth < 768
}

// ========== 列配置 ==========
interface ColumnDef {
  key: string
  labelKey: string
  defaultVisible: boolean
  sortable: boolean
  getValue: (conn: any, tab: 'active' | 'closed') => string | number
  specialRender?: 'host' | 'type' | 'upload' | 'download' | 'duration' | 'action' | 'rule'
}

const columnDefs: ColumnDef[] = [
  // ---- 现有列（默认可见） ----
  {
    key: 'host',
    labelKey: 'connections.host',
    defaultVisible: true,
    sortable: true,
    getValue: (c) => c.metadata?.host || c.metadata?.destinationIP || '',
    specialRender: 'host'
  },
  // ---- 进程名（默认可见，移至第二列） ----
  {
    key: 'metadata.process',
    labelKey: 'connections.column.metadata.process',
    defaultVisible: true,
    sortable: false,
    getValue: (c) => c.metadata?.process || ''
  },
  {
    key: 'type',
    labelKey: 'connections.type',
    defaultVisible: true,
    sortable: false,
    getValue: (c) => c.metadata?.network || 'tcp',
    specialRender: 'type'
  },
  {
    key: 'rule',
    labelKey: 'connections.rule',
    defaultVisible: true,
    sortable: true,
    getValue: (c) => c.rule || '',
    specialRender: 'rule'
  },
  {
    key: 'chain',
    labelKey: 'connections.chain',
    defaultVisible: true,
    sortable: true,
    getValue: (c) => [...(c.chains || [])].reverse().join(' → ')
  },
  {
    key: 'uploadSpeed',
    labelKey: 'connections.upload_speed',
    defaultVisible: true,
    sortable: true,
    getValue: (c, tab) => tab === 'active' ? (c.speedUp || 0) : c.upload,
    specialRender: 'upload'
  },
  {
    key: 'downloadSpeed',
    labelKey: 'connections.download_speed',
    defaultVisible: true,
    sortable: true,
    getValue: (c, tab) => tab === 'active' ? (c.speedDown || 0) : c.download,
    specialRender: 'download'
  },
  {
    key: 'duration',
    labelKey: 'connections.duration',
    defaultVisible: true,
    sortable: true,
    getValue: (c, tab) => tab === 'active' ? new Date(c.start).getTime() : (c.closedAtTimestamp || 0),
    specialRender: 'duration'
  },
  // ---- metadata 字段（默认隐藏） ----
  {
    key: 'metadata.sourceIP',
    labelKey: 'connections.column.metadata.sourceIP',
    defaultVisible: false,
    sortable: false,
    getValue: (c) => c.metadata?.sourceIP || ''
  },
  {
    key: 'metadata.sourcePort',
    labelKey: 'connections.column.metadata.sourcePort',
    defaultVisible: false,
    sortable: false,
    getValue: (c) => c.metadata?.sourcePort || ''
  },
  {
    key: 'metadata.destinationIP',
    labelKey: 'connections.column.metadata.destinationIP',
    defaultVisible: false,
    sortable: false,
    getValue: (c) => c.metadata?.destinationIP || ''
  },
  {
    key: 'metadata.destinationPort',
    labelKey: 'connections.column.metadata.destinationPort',
    defaultVisible: false,
    sortable: false,
    getValue: (c) => c.metadata?.destinationPort || ''
  },
  {
    key: 'metadata.remoteDestination',
    labelKey: 'connections.column.metadata.remoteDestination',
    defaultVisible: false,
    sortable: false,
    getValue: (c) => c.metadata?.remoteDestination || ''
  },
  {
    key: 'metadata.sniffHost',
    labelKey: 'connections.column.metadata.sniffHost',
    defaultVisible: false,
    sortable: false,
    getValue: (c) => c.metadata?.sniffHost || ''
  },
  {
    key: 'metadata.inboundIP',
    labelKey: 'connections.column.metadata.inboundIP',
    defaultVisible: false,
    sortable: false,
    getValue: (c) => c.metadata?.inboundIP || ''
  },
  {
    key: 'metadata.inboundPort',
    labelKey: 'connections.column.metadata.inboundPort',
    defaultVisible: false,
    sortable: false,
    getValue: (c) => c.metadata?.inboundPort || ''
  },
  {
    key: 'metadata.uid',
    labelKey: 'connections.column.metadata.uid',
    defaultVisible: false,
    sortable: false,
    getValue: (c) => c.metadata?.uid ?? ''
  },
  {
    key: 'metadata.processPath',
    labelKey: 'connections.column.metadata.processPath',
    defaultVisible: false,
    sortable: false,
    getValue: (c) => c.metadata?.processPath || ''
  }
]

// ========== 列可见性状态 ==========
const STORAGE_KEY = 'fluxor-connections-columns'
const loadColumnVisibility = (): Record<string, boolean> => {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored) {
      const parsed = JSON.parse(stored)
      const result: Record<string, boolean> = {}
      columnDefs.forEach(col => {
        result[col.key] = parsed[col.key] !== undefined ? parsed[col.key] : col.defaultVisible
      })
      return result
    }
  } catch (_) {}
  const def: Record<string, boolean> = {}
  columnDefs.forEach(col => { def[col.key] = col.defaultVisible })
  return def
}

const columnVisibility = reactive(loadColumnVisibility())

const saveColumnVisibility = () => {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(columnVisibility))
}

const visibleColumns = computed(() => {
  return columnDefs.filter(col => columnVisibility[col.key])
})

// ========== 设置弹窗 ==========
const showSettings = ref(false)
const isActive = useViewActive()
const tempVisibility = ref<Record<string, boolean>>({})

const openSettings = () => {
  tempVisibility.value = { ...columnVisibility }
  showSettings.value = true
}

const saveSettings = () => {
  Object.assign(columnVisibility, tempVisibility.value)
  saveColumnVisibility()
  showSettings.value = false
}

const closeSettings = () => {
  showSettings.value = false
}

const resetSettings = () => {
  const defaults: Record<string, boolean> = {}
  columnDefs.forEach(col => {
    defaults[col.key] = col.defaultVisible
  })
  tempVisibility.value = defaults
}

// ========== 连接详情弹窗 ==========
const showDetailDialog = ref(false)
const selectedConnection = ref<any>(null)

const openDetail = (conn: any) => {
  selectedConnection.value = conn
  showDetailDialog.value = true
}

const closeDetail = () => {
  showDetailDialog.value = false
  selectedConnection.value = null
}

// 辅助格式化
// 获取连接详情中的各字段
const detailFields = computed(() => {
  const c = selectedConnection.value
  if (!c) return null
  const meta = c.metadata || {}
  const isActive = activeTab.value === 'active'
  return {
    id: c.id,
    start: c.start,
    rule: c.rule,
    rulePayload: c.rulePayload,
    upload: c.upload,
    download: c.download,
    speedUp: isActive ? c.speedUp : undefined,
    speedDown: isActive ? c.speedDown : undefined,
    network: meta.network,
    type: meta.type,
    host: meta.host,
    sniffHost: meta.sniffHost,
    dnsMode: meta.dnsMode,
    sourceIP: meta.sourceIP,
    sourcePort: meta.sourcePort,
    destinationIP: meta.destinationIP,
    destinationPort: meta.destinationPort,
    remoteDestination: meta.remoteDestination,
    inboundName: meta.inboundName,
    inboundIP: meta.inboundIP,
    inboundUser: meta.inboundUser,
    process: meta.process,
    processPath: meta.processPath,
    uid: meta.uid,
    chains: c.chains ? [...c.chains].reverse() : []
  }
})

// 详情弹窗「目标地址」展示值：优先 IP:port，回退 IP，再回退 host（与弹窗内展示保持同源）
const detailTarget = computed(() => {
  const f = detailFields.value
  if (!f) return '-'
  return f.destinationIP && f.destinationPort
    ? `${f.destinationIP}:${f.destinationPort}`
    : (f.destinationIP || f.host || '-')
})

// 复制文本到剪贴板并提示（列表内已禁用选中，弹窗里的 host/目标靠它取走）
const copyText = (text: string | null | undefined, label: string) => {
  if (!text || text === '-') return
  navigator.clipboard.writeText(text).then(() => {
    globalStore.showToast(`${label} ${t('common.copied')}`, 'success')
  }).catch(() => {
    globalStore.showToast(t('common.operation_failed'), 'error')
  })
}

// ========== 搜索与排序 ==========
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
    const sortField = sortBy.value
    const colDef = columnDefs.find(col => col.key === sortField)
    if (!colDef || !colDef.sortable) return 0

    const valA = colDef.getValue(a, activeTab.value)
    const valB = colDef.getValue(b, activeTab.value)
    if (typeof valA === 'string' && typeof valB === 'string') {
      return sortDesc.value ? valB.localeCompare(valA) : valA.localeCompare(valB)
    } else {
      const numA = Number(valA)
      const numB = Number(valB)
      if (isNaN(numA) || isNaN(numB)) return 0
      return sortDesc.value ? numB - numA : numA - numB
    }
  })

  return list
})

const handleSort = (field: string) => {
  const colDef = columnDefs.find(col => col.key === field)
  if (!colDef || !colDef.sortable) return
  if (sortBy.value === field) {
    sortDesc.value = !sortDesc.value
  } else {
    sortBy.value = field as any
    sortDesc.value = true
  }
}

// ========== 其他函数 ==========
const handleCloseConnection = async (id: string) => {
  try {
    const resp = await apiFetch(`/connections/${id}`, { method: 'DELETE' })
    if (resp.ok) {
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
    } else {
      // 失败必须让用户看见：此前两个分支都不提示，点「断开」没反应时用户只会反复点，
      // 而连接其实一条都没断。内核的直接原因（连接已消失 / 内核未运行）在响应体里。
      globalStore.showToast(
        `${t('common.operation_failed')}: ${await readErrorMessage(resp)}`,
        'error'
      )
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  }
}

const handleCloseAll = async () => {
  const ok = await globalStore.showConfirm({
    message: t('connections.confirm_close_all'),
    type: 'danger'
  })
  if (ok) {
    try {
      const resp = await apiFetch('/connections', { method: 'DELETE' })
      if (resp.ok) {
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
      } else {
        globalStore.showToast(
          `${t('common.operation_failed')}: ${await readErrorMessage(resp)}`,
          'error'
        )
      }
    } catch (e) {
      globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
    }
  }
}

const handleClearClosedItem = (id: string) => {
  connStore.removeClosedConnection(id)
}

const handleClearAllClosed = async () => {
  const ok = await globalStore.showConfirm({
    message: t('connections.confirm_clear_closed'),
    type: 'warning'
  })
  if (ok) {
    connStore.clearClosedConnections()
  }
}

// 固定列（在移动端卡片中已单独显示）
const fixedColumns = new Set(['host', 'type', 'rule', 'chain', 'uploadSpeed', 'downloadSpeed', 'duration','metadata.process'])

// 额外列（移动端卡片中动态显示）
const extraColumns = computed(() => {
  return visibleColumns.value.filter(col => !fixedColumns.has(col.key))
})

onMounted(() => {
  window.addEventListener('resize', handleResize)
})

// 连接流跟随**视图激活态**而非挂载态：本视图在 <KeepAlive :max="7"> 内，切走只触发
// onDeactivated，onUnmounted 要等缓存淘汰/应用销毁才触发。写在 onMounted/onUnmounted
// 会让连接 WebSocket 在整个会话里一直开着（概览页也订阅了同一条流），持续推送全量
// 连接快照。store 侧是引用计数 + 3s 去抖，天然适配页面级开关。
onActivated(() => {
  connStore.subscribe()
})

onDeactivated(() => {
  connStore.unsubscribe()
})

onUnmounted(() => {
  // 应用直接销毁时兜底释放（退订是引用计数、可重复调用）
  connStore.unsubscribe()
  window.removeEventListener('resize', handleResize)
})
</script>

<template>
  <div class="flex flex-col flex-1 min-h-0 gap-4 h-full">
    <!-- 顶部栏 -->
    <PageHeader
      :icon="LinkOutline"
      left-class="justify-between md:justify-start gap-4 flex-1 md:flex-initial"
    >
      <template #title>
        {{ t('connections.title') }}
        <button @click="openSettings" class="p-1.5 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all" :title="t('connections.column_settings')">
          <SettingsOutline class="w-4 h-4" />
        </button>
      </template>
      <template #lead>
        <div class="flex bg-slate-100 dark:bg-slate-800 rounded-lg p-0.5 transition-all shrink-0">
          <button @click="activeTab = 'active'" class="px-4 py-1.5 text-xs font-semibold rounded-md transition-all duration-200" :class="activeTab === 'active' ? 'bg-accent text-white shadow-sm' : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'">
            {{ t('connections.active') }} ({{ activeConnections.length }})
          </button>
          <button @click="activeTab = 'closed'" class="px-4 py-1.5 text-xs font-semibold rounded-md transition-all duration-200" :class="activeTab === 'closed' ? 'bg-accent text-white shadow-sm' : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'">
            {{ t('connections.closed') }} ({{ closedConnections.length }})
          </button>
        </div>
      </template>

      <template #actions>
        <div class="flex items-center gap-3 flex-1 justify-end min-w-[280px] sm:min-w-0 flex-nowrap">
        <input type="text" v-model="searchText" :placeholder="t('connections.search_placeholder')" class="w-full sm:w-60 px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none" />
        <div class="flex gap-2 shrink-0">
          <button v-if="activeTab === 'active'" @click="isPaused = !isPaused" class="px-4 py-1.5 text-xs font-semibold rounded-lg border transition-all flex items-center gap-1.5 whitespace-nowrap" :class="isPaused ? 'bg-amber-500/10 text-amber-500 border-amber-500/20' : 'bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 border-transparent'">
            <PlayOutline v-if="isPaused" class="w-3.5 h-3.5" />
            <PauseOutline v-else class="w-3.5 h-3.5" />
            {{ isPaused ? t('connections.resume') : t('connections.pause') }}
          </button>
          <button v-if="activeTab === 'active'" @click="handleCloseAll" class="px-4 py-1.5 text-xs font-semibold rounded-lg bg-red-500/10 hover:bg-red-500/20 text-red-500 transition-all border border-red-500/10 flex items-center gap-1.5 whitespace-nowrap">
            <CloseOutline class="w-3.5 h-3.5" />
            {{ t('connections.close_all') }}
          </button>
          <button v-if="activeTab === 'closed'" @click="handleClearAllClosed" class="px-4 py-1.5 text-xs font-semibold rounded-lg bg-red-500/10 hover:bg-red-500/20 text-red-500 transition-all border border-red-500/10 whitespace-nowrap">
            {{ t('connections.clear_all_closed') }}
          </button>
        </div>
        </div>
      </template>
    </PageHeader>

    <!-- 表格容器 -->
    <div class="flex-1 min-h-0 overflow-y-auto glass-medium shadow-none rounded-xl border border-slate-200/50 dark:border-slate-800/50 transition-all pr-1">
      <!-- 桌面端表格 -->
      <div class="hidden md:block overflow-x-auto">
        <table class="w-full text-left text-xs border-collapse">
          <thead>
            <tr class="bg-slate-50 dark:bg-slate-800/50 border-b border-slate-100 dark:border-slate-800 text-slate-500 dark:text-slate-400 font-semibold select-none">
              <th v-for="col in visibleColumns" :key="col.key"
                  class="py-3 px-4 cursor-pointer hover:bg-slate-100 dark:hover:bg-slate-800/80 transition-colors"
                  :class="{
                    'shrink-0 w-20 text-center': col.key === 'type',
                    'hidden lg:table-cell': col.key === 'chain' || col.key === 'rule',
                    'md:table-cell': col.key === 'rule',
                    'max-w-[250px] min-w-[120px] whitespace-normal break-words': col.key === 'chain'
                  }"
                  @click="handleSort(col.key)">
                {{ t(col.labelKey) }}
                <span v-if="sortBy === col.key && col.sortable" class="inline-flex align-middle ml-0.5">
                  <ArrowDownOutline v-if="sortDesc" class="w-3.5 h-3.5" />
                  <ArrowUpOutline v-else class="w-3.5 h-3.5" />
                </span>
              </th>
              <th class="py-3 px-4 shrink-0 w-20 text-center whitespace-nowrap">{{ t('connections.action') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100 dark:divide-slate-800">
            <template v-if="!isWsConnected && activeConnections.length === 0">
              <tr v-for="i in 3" :key="i" class="animate-pulse select-none">
                <td v-for="col in visibleColumns" :key="col.key" class="py-4.5 px-4">
                  <div class="h-3.5 bg-slate-200 dark:bg-slate-800 rounded"></div>
                </td>
                <td class="py-4.5 px-4 text-center">
                  <div class="w-10 h-6 bg-slate-200 dark:bg-slate-800 rounded-full mx-auto"></div>
                </td>
              </tr>
            </template>
            <tr v-else-if="filteredConnections.length === 0" class="text-slate-400 dark:text-slate-600 text-center">
              <td :colspan="visibleColumns.length + 1" class="py-8 text-sm">{{ t('connections.empty') }}</td>
            </tr>
            <tr v-else v-for="c in filteredConnections.slice(0, 150)" :key="c.id"
                class="hover:bg-slate-50/50 dark:hover:bg-slate-900/10 transition-colors cursor-pointer"
                @click="openDetail(c)">
              <td v-for="col in visibleColumns" :key="col.key" class="py-3 px-4" :class="{ 'max-w-[250px] min-w-[120px] whitespace-normal break-words': col.key === 'chain' }">
                <template v-if="col.specialRender === 'host'">
                  <div class="flex items-center gap-1.5 flex-wrap">
                    <span v-if="c.metadata?.type" class="px-1 py-0.5 text-[9px] font-semibold bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 rounded tracking-wide uppercase">
                      {{ c.metadata.type }}
                    </span>
                    <div class="font-medium text-slate-800 dark:text-slate-200 select-none max-w-[150px] sm:max-w-[250px] truncate break-all" :title="c.metadata?.host || c.metadata?.destinationIP">
                      {{ c.metadata?.host || c.metadata?.destinationIP }}
                    </div>
                  </div>
                  <div v-if="c.metadata?.destinationIP" class="text-[10px] text-slate-400 dark:text-slate-500 select-none">{{ c.metadata.destinationIP }}{{ c.metadata.destinationPort ? ':' + c.metadata.destinationPort : '' }}</div>
                </template>

                <template v-else-if="col.specialRender === 'type'">
                  <span class="px-1.5 py-0.5 text-[10px] font-bold rounded tracking-wide uppercase" :class="(c.metadata?.network || 'tcp').toUpperCase() === 'UDP' ? 'bg-amber-500/10 text-amber-500' : 'bg-blue-500/10 text-blue-500'">
                    {{ c.metadata?.network || 'tcp' }}
                  </span>
                </template>

                <template v-else-if="col.specialRender === 'upload'">
                  <span class="font-mono font-medium text-blue-500">
                    {{ activeTab === 'active' ? formatSpeed(c.speedUp || 0) : formatBytes(c.upload) }}
                  </span>
                </template>

                <template v-else-if="col.specialRender === 'download'">
                  <span class="font-mono font-medium text-success">
                    {{ activeTab === 'active' ? formatSpeed(c.speedDown || 0) : formatBytes(c.download) }}
                  </span>
                </template>

                <template v-else-if="col.specialRender === 'duration'">
                  <span class="font-mono text-slate-500">
                    {{ activeTab === 'active' ? formatDuration(c.start) : c.closedAt }}
                  </span>
                </template>

                <template v-else-if="col.specialRender === 'rule'">
                  <span class="font-medium text-slate-600 dark:text-slate-300">
                    {{ c.rule }}{{ c.rulePayload ? ': ' + c.rulePayload : '' }}
                  </span>
                </template>

                <template v-else>
                  <span class="text-slate-600 dark:text-slate-300 font-medium" v-if="typeof col.getValue(c, activeTab) === 'string'">
                    {{ col.getValue(c, activeTab) }}
                  </span>
                  <span v-else class="font-mono">
                    {{ col.getValue(c, activeTab) }}
                  </span>
                </template>
              </td>
              <td class="py-3 px-4 text-center shrink-0 whitespace-nowrap" @click.stop>
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
      <div v-if="filteredConnections.length > 150" class="px-5 py-3 bg-slate-50 dark:bg-slate-800/10 text-center text-xs text-slate-400 dark:text-slate-500 border-t border-slate-100 dark:border-slate-800/50">
        {{ t('connections.limit_hint') }}
      </div>

      <!-- ====== 移动端卡片 ====== -->
      <div class="md:hidden divide-y divide-slate-100 dark:divide-slate-800">
        <template v-if="!isWsConnected && activeConnections.length === 0">
          <div v-for="i in 3" :key="i" class="p-4 space-y-3.5 animate-pulse select-none">
            <!-- 骨架屏内容 -->
          </div>
        </template>
        <template v-else>
          <div v-if="filteredConnections.length === 0" class="py-8 text-center text-slate-400 dark:text-slate-600 text-sm">
            {{ t('connections.empty') }}
          </div>
          <div
            v-else
            v-for="c in filteredConnections.slice(0, 150)"
            :key="c.id"
            class="p-4 space-y-3 transition-colors hover:bg-slate-50/50 dark:hover:bg-slate-900/10 cursor-pointer"
            @click="openDetail(c)"
          >
            <!-- 第一行：主机 + 类型标签 -->
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0 flex-1">
                <div class="font-bold text-slate-800 dark:text-slate-200 select-none break-all text-xs leading-snug">
                  {{ c.metadata?.host || c.metadata?.destinationIP }}
                </div>
                <div v-if="c.metadata?.destinationIP" class="text-[10px] text-slate-400 dark:text-slate-500 mt-0.5 select-none">
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

            <!-- 第二行：规则 + 链路 -->
            <div class="text-[10px] space-y-1.5">
              <div class="flex items-center gap-1">
                <span class="text-slate-400">{{ t('connections.rule') }}:</span>
                <span class="font-medium text-slate-600 dark:text-slate-300 bg-slate-100/50 dark:bg-slate-800/50 px-1.5 py-0.5 rounded">{{ c.rule }}{{ c.rulePayload ? ': ' + c.rulePayload : '' }}</span>
              </div>
              <div v-if="c.chains && c.chains.length > 0" class="flex items-start gap-1 flex-wrap">
                <span class="text-slate-400 shrink-0">{{ t('connections.chain') }}:</span>
                <span class="inline-flex items-center gap-0.5 flex-wrap">
                  <template v-for="(item, idx) in [...c.chains].reverse()" :key="idx">
                    <span class="font-mono text-slate-600 dark:text-slate-300 bg-slate-100/50 dark:bg-slate-800/50 px-1.5 py-0.5 rounded text-[10px]">
                      {{ item }}
                    </span>
                    <span v-if="idx < [...c.chains].reverse().length - 1" class="text-slate-400 text-[10px]">→</span>
                  </template>
                </span>
              </div>
            </div>

            <!-- 进程名（移动端固定显示，在链路行下方） -->
            <div v-if="c.metadata?.process" class="text-[10px] flex items-start gap-1">
              <span class="text-slate-400 shrink-0">{{ t('connections.column.metadata.process') }}:</span>
              <span class="font-mono text-slate-600 dark:text-slate-300 bg-slate-100/50 dark:bg-slate-800/50 px-1.5 py-0.5 rounded break-all">{{ c.metadata.process }}</span>
            </div>

            <!-- 额外信息（根据列设置动态显示） -->
            <div v-if="extraColumns.length > 0" class="text-[10px] space-y-1 border-t border-slate-100 dark:border-slate-800/40 pt-1.5 mt-1.5">
              <div v-for="col in extraColumns" :key="col.key" class="flex items-start gap-1">
                <span class="text-slate-400 shrink-0">{{ t(col.labelKey) }}:</span>
                <span class="text-slate-600 dark:text-slate-300 break-all">
                  {{ col.getValue(c, activeTab) || '-' }}
                </span>
              </div>
            </div>

            <!-- 第三行：流量 + 时间 + 操作（固定在底部） -->
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
              <div class="shrink-0 select-none" @click.stop>
                <button v-if="activeTab === 'active'" @click="handleCloseConnection(c.id)" class="px-2.5 py-1 bg-red-500/10 hover:bg-red-500/20 text-red-500 rounded-md font-semibold text-[10px] transition-all">
                  {{ t('connections.close') }}
                </button>
                <button v-else @click="handleClearClosedItem(c.id)" class="px-2.5 py-1 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-500 dark:text-slate-400 rounded-md font-semibold text-[10px] transition-all">
                  {{ t('connections.clear') }}
                </button>
              </div>
            </div>
          </div>
          <div v-if="filteredConnections.length > 150" class="p-4 bg-slate-50 dark:bg-slate-800/10 text-center text-xs text-slate-400 dark:text-slate-500 border-t border-slate-100 dark:border-slate-800/50">
            {{ t('connections.limit_hint') }}
          </div>
        </template>
      </div>
    </div>
  </div>

  <!-- ====== 列设置弹窗 ====== -->
  <AppModal
    :open="isActive && showSettings"
    :label="t('connections.column_settings_title')"
    panel-class="w-full max-w-lg max-h-[80vh] rounded-2xl p-6 flex flex-col gap-4"
    @close="closeSettings"
  >
    <div class="flex items-center justify-between border-b border-slate-100 dark:border-slate-800/60 pb-3">
      <h3 class="text-base font-semibold flex items-center gap-2">
        <SettingsOutline class="w-5 h-5 text-accent" />
        {{ t('connections.column_settings_title') }}
      </h3>
      <button @click="closeSettings" class="p-1.5 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all">
        <CloseOutline class="w-4 h-4 text-slate-400" />
      </button>
    </div>
    <div class="flex-1 overflow-y-auto pr-1">
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
        <div v-for="col in columnDefs" :key="col.key"
             class="flex items-center justify-between py-1.5 px-2 rounded-lg"
             :class="{
               'hover:bg-slate-50/50 dark:hover:bg-slate-800/30': !(isMobile && col.defaultVisible),
               'opacity-50 cursor-not-allowed': isMobile && col.defaultVisible
             }">
          <label class="text-xs font-medium text-slate-700 dark:text-slate-200 select-none"
                 :class="{ 'cursor-not-allowed': isMobile && col.defaultVisible, 'cursor-pointer': !(isMobile && col.defaultVisible) }"
                 @click="isMobile && col.defaultVisible ? null : (tempVisibility[col.key] = !tempVisibility[col.key])">
            {{ t(col.labelKey) }}
          </label>
          <div class="relative inline-flex items-center"
               :class="{ 'cursor-not-allowed': isMobile && col.defaultVisible, 'cursor-pointer': !(isMobile && col.defaultVisible) }"
               @click="isMobile && col.defaultVisible ? null : (tempVisibility[col.key] = !tempVisibility[col.key])">
            <div class="w-9 h-5 rounded-full transition-colors duration-200 ease-in-out"
                 :class="tempVisibility[col.key] ? 'bg-accent' : 'bg-slate-300 dark:bg-slate-600'"></div>
            <div class="absolute left-0.5 top-0.5 w-4 h-4 rounded-full bg-white shadow transform transition-transform duration-200 ease-in-out"
                 :class="tempVisibility[col.key] ? 'translate-x-4' : 'translate-x-0'"></div>
          </div>
        </div>
      </div>
    </div>
    <div class="border-t border-slate-100 dark:border-slate-800/60 pt-3 flex justify-between gap-2">
      <button @click="resetSettings" class="px-4 py-2 text-xs font-semibold rounded-xl bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-600 dark:text-slate-300 transition-all active:scale-95">
        {{ t('common.reset') }}
      </button>
      <div class="flex gap-2">
        <button @click="closeSettings" class="px-4 py-2 text-xs font-semibold rounded-xl bg-white border border-slate-200/80 hover:bg-slate-50 dark:bg-slate-800 dark:border-slate-700 dark:hover:bg-slate-700/60 text-slate-600 dark:text-slate-300 transition-all active:scale-95">
          {{ t('common.cancel') }}
        </button>
        <button @click="saveSettings" class="px-4 py-2 text-xs font-semibold rounded-xl bg-accent hover:bg-accent-hover text-white transition-all active:scale-95 shadow-sm">
          {{ t('common.save') }}
        </button>
      </div>
    </div>

  </AppModal>

  <!-- ====== 连接详情弹窗 ====== -->
  <AppModal
    :open="isActive && showDetailDialog && selectedConnection"
    :label="t('connections.detail.title')"
    panel-class="w-full max-w-3xl max-h-[90vh] rounded-2xl p-6 flex flex-col gap-4"
    @close="closeDetail"
  >
    <!-- 头部 -->
    <div class="flex items-center justify-between border-b border-slate-100 dark:border-slate-800/60 pb-3 shrink-0">
      <h3 class="text-base font-semibold flex items-center gap-2">
        <InformationCircleOutline class="w-5 h-5 text-accent" />
        {{ t('connections.detail.title') }}
      </h3>
      <button @click="closeDetail" class="p-1.5 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all">
        <CloseOutline class="w-4 h-4 text-slate-400" />
      </button>
    </div>

    <!-- 内容 -->
    <div class="flex-1 overflow-y-auto pr-1 space-y-4">
      <!-- 基本信息 -->
      <div class="bg-slate-50/50 dark:bg-slate-900/20 rounded-xl p-3">
        <h4 class="text-xs font-bold text-slate-500 dark:text-slate-400 flex items-center gap-1.5 mb-2">
          <DocumentTextOutline class="w-3.5 h-3.5" />
          {{ t('connections.detail.basic') }}
        </h4>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-x-4 gap-y-1.5 text-xs">
          <div><span class="text-slate-400">{{ t('connections.detail.id') }}:</span> <span class="font-mono text-slate-700 dark:text-slate-300 break-all">{{ detailFields?.id || '-' }}</span></div>
          <div><span class="text-slate-400">{{ t('connections.detail.start') }}:</span> <span class="font-mono text-slate-700 dark:text-slate-300">{{ detailFields?.start ? formatTimestamp(detailFields.start) : '-' }}</span></div>
          <div><span class="text-slate-400">{{ t('connections.rule') }}:</span> <span class="font-medium text-slate-700 dark:text-slate-300">{{ detailFields?.rule || '-' }}</span></div>
          <div><span class="text-slate-400">{{ t('connections.detail.rulePayload') }}:</span> <span class="font-mono text-slate-700 dark:text-slate-300">{{ detailFields?.rulePayload || '-' }}</span></div>
        </div>
      </div>

      <!-- 流量 -->
      <div class="bg-slate-50/50 dark:bg-slate-900/20 rounded-xl p-3">
        <h4 class="text-xs font-bold text-slate-500 dark:text-slate-400 flex items-center gap-1.5 mb-2">
          <BarChartOutline class="w-3.5 h-3.5" />
          {{ t('connections.detail.traffic') }}
        </h4>
        <div class="grid grid-cols-2 sm:grid-cols-4 gap-x-4 gap-y-1.5 text-xs">
          <div><span class="text-slate-400">{{ t('connections.detail.upload') }}:</span> <span class="font-mono text-blue-500 font-medium">{{ formatBytes(detailFields?.upload || 0) }}</span></div>
          <div><span class="text-slate-400">{{ t('connections.detail.download') }}:</span> <span class="font-mono text-success font-medium">{{ formatBytes(detailFields?.download || 0) }}</span></div>
          <div><span class="text-slate-400">{{ t('connections.detail.uploadSpeed') }}:</span> <span class="font-mono text-blue-500">{{ detailFields?.speedUp !== undefined ? formatSpeed(detailFields.speedUp) : '-' }}</span></div>
          <div><span class="text-slate-400">{{ t('connections.detail.downloadSpeed') }}:</span> <span class="font-mono text-success">{{ detailFields?.speedDown !== undefined ? formatSpeed(detailFields.speedDown) : '-' }}</span></div>
        </div>
      </div>

      <!-- 元数据 -->
      <div class="bg-slate-50/50 dark:bg-slate-900/20 rounded-xl p-3">
        <h4 class="text-xs font-bold text-slate-500 dark:text-slate-400 flex items-center gap-1.5 mb-2">
          <ServerOutline class="w-3.5 h-3.5" />
          {{ t('connections.detail.metadata') }}
        </h4>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-x-4 gap-y-1.5 text-xs">
          <div><span class="text-slate-400">{{ t('connections.detail.network') }}:</span> <span class="font-medium text-slate-700 dark:text-slate-300">{{ detailFields?.network || '-' }}</span></div>
          <div><span class="text-slate-400">{{ t('connections.detail.type') }}:</span> <span class="font-medium text-slate-700 dark:text-slate-300">{{ detailFields?.type || '-' }}</span></div>
          <div><span class="text-slate-400">{{ t('connections.detail.host') }}:</span> <span
              class="font-mono text-slate-700 dark:text-slate-300 cursor-pointer hover:text-accent transition-colors"
              :title="detailFields?.host ? t('connections.detail.click_copy_host') : ''"
              @click="copyText(detailFields?.host, t('connections.detail.copy_label_host'))">{{ detailFields?.host || '-' }}</span></div>
          <div><span class="text-slate-400">{{ t('connections.detail.sniffHost') }}:</span> <span class="font-mono text-slate-700 dark:text-slate-300">{{ detailFields?.sniffHost || '-' }}</span></div>
          <div><span class="text-slate-400">{{ t('connections.detail.dnsMode') }}:</span> <span class="font-medium text-slate-700 dark:text-slate-300">{{ detailFields?.dnsMode || '-' }}</span></div>
        </div>
      </div>

      <!-- 源与目标 -->
      <div class="bg-slate-50/50 dark:bg-slate-900/20 rounded-xl p-3">
        <h4 class="text-xs font-bold text-slate-500 dark:text-slate-400 flex items-center gap-1.5 mb-2">
          <PeopleOutline class="w-3.5 h-3.5" />
          {{ t('connections.detail.source_target') }}
        </h4>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-x-4 gap-y-1.5 text-xs">
          <div><span class="text-slate-400">{{ t('connections.detail.source') }}:</span> <span class="font-mono text-slate-700 dark:text-slate-300">{{ detailFields?.sourceIP && detailFields?.sourcePort ? detailFields.sourceIP + ':' + detailFields.sourcePort : detailFields?.sourceIP || '-' }}</span></div>
          <div><span class="text-slate-400">{{ t('connections.detail.target') }}:</span> <span
              class="font-mono text-slate-700 dark:text-slate-300 cursor-pointer hover:text-accent transition-colors"
              :title="detailTarget !== '-' ? t('connections.detail.click_copy_target') : ''"
              @click="copyText(detailTarget, t('connections.detail.copy_label_target'))">{{ detailTarget }}</span></div>
          <div class="sm:col-span-2"><span class="text-slate-400">{{ t('connections.detail.remoteDestination') }}:</span> <span class="font-mono text-slate-700 dark:text-slate-300">{{ detailFields?.remoteDestination || '-' }}</span></div>
        </div>
      </div>

      <!-- 入站 -->
      <div class="bg-slate-50/50 dark:bg-slate-900/20 rounded-xl p-3">
        <h4 class="text-xs font-bold text-slate-500 dark:text-slate-400 flex items-center gap-1.5 mb-2">
          <CloudOutline class="w-3.5 h-3.5" />
          {{ t('connections.detail.inbound') }}
        </h4>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-x-4 gap-y-1.5 text-xs">
          <div><span class="text-slate-400">{{ t('connections.detail.inboundName') }}:</span> <span class="font-medium text-slate-700 dark:text-slate-300">{{ detailFields?.inboundName || '-' }}</span></div>
          <div><span class="text-slate-400">{{ t('connections.detail.inboundIP') }}:</span> <span class="font-mono text-slate-700 dark:text-slate-300">{{ detailFields?.inboundIP || '-' }}</span></div>
          <div><span class="text-slate-400">{{ t('connections.detail.inboundUser') }}:</span> <span class="font-medium text-slate-700 dark:text-slate-300">{{ detailFields?.inboundUser || '-' }}</span></div>
        </div>
      </div>

      <!-- 进程 -->
      <div class="bg-slate-50/50 dark:bg-slate-900/20 rounded-xl p-3">
        <h4 class="text-xs font-bold text-slate-500 dark:text-slate-400 flex items-center gap-1.5 mb-2">
          <RocketOutline class="w-3.5 h-3.5" />
          {{ t('connections.detail.process') }}
        </h4>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-x-4 gap-y-1.5 text-xs">
          <div><span class="text-slate-400">{{ t('connections.detail.processName') }}:</span> <span class="font-mono text-slate-700 dark:text-slate-300">{{ detailFields?.process || '-' }}</span></div>
          <div><span class="text-slate-400">{{ t('connections.detail.processPath') }}:</span> <span class="font-mono text-slate-700 dark:text-slate-300 break-all">{{ detailFields?.processPath || '-' }}</span></div>
          <div><span class="text-slate-400">{{ t('connections.detail.uid') }}:</span> <span class="font-mono text-slate-700 dark:text-slate-300">{{ detailFields?.uid ?? '-' }}</span></div>
        </div>
      </div>

      <!-- 链路 -->
      <div class="bg-slate-50/50 dark:bg-slate-900/20 rounded-xl p-3">
        <h4 class="text-xs font-bold text-slate-500 dark:text-slate-400 flex items-center gap-1.5 mb-2">
          <GitBranchOutline class="w-3.5 h-3.5" />
          {{ t('connections.detail.chain') }}
        </h4>
        <div class="text-xs font-mono text-slate-700 dark:text-slate-300 break-all leading-relaxed">
          <span v-if="detailFields?.chains && detailFields.chains.length > 0" class="inline-flex flex-wrap gap-1">
            <span v-for="(item, idx) in detailFields.chains" :key="idx" class="bg-slate-200/60 dark:bg-slate-800/50 px-1.5 py-0.5 rounded">
              {{ item }}
            </span>
          </span>
          <span v-else>-</span>
        </div>
      </div>
    </div>

    <!-- 底部关闭 -->
    <div class="border-t border-slate-100 dark:border-slate-800/60 pt-3 flex justify-end shrink-0">
      <button @click="closeDetail" class="px-4 py-2 text-xs font-semibold rounded-xl bg-white border border-slate-200/80 hover:bg-slate-50 dark:bg-slate-800 dark:border-slate-700 dark:hover:bg-slate-700/60 text-slate-600 dark:text-slate-300 transition-all active:scale-95">
        {{ t('common.close') }}
      </button>
    </div>

  </AppModal>
</template>