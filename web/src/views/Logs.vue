<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { wsConnect } from '../utils/api'
import { storeToRefs } from 'pinia'
import { useLogStore, type LogItem } from '../store/logs'

const { t } = useI18n()
const logStore = useLogStore()
const { logs, autoScroll, isPaused } = storeToRefs(logStore)

const searchText = ref('')
const terminalRef = ref<HTMLDivElement | null>(null)

let ws: WebSocket | null = null

const connectWS = () => {
  if (ws) ws.close()
  ws = wsConnect('/logs', (e: MessageEvent) => {
    if (isPaused.value) return
    let item: LogItem
    try {
      const data = JSON.parse(e.data)
      item = {
        id: Date.now() + Math.random(),
        type: data.type || 'info',
        payload: data.payload || data,
        time: new Date().toLocaleTimeString()
      }
    } catch (err) {
      item = {
        id: Date.now() + Math.random(),
        type: 'info',
        payload: e.data,
        time: new Date().toLocaleTimeString()
      }
    }

    logStore.addLog(item)

    if (autoScroll.value) {
      scrollToBottom()
    }
  })
}

const scrollToBottom = () => {
  nextTick(() => {
    if (terminalRef.value) {
      terminalRef.value.scrollTop = terminalRef.value.scrollHeight
    }
  })
}

const filteredLogs = computed(() => {
  if (!searchText.value.trim()) return logs.value
  const query = searchText.value.toLowerCase()
  return logs.value.filter(log => log.payload.toLowerCase().includes(query))
})

const handleClear = () => {
  logStore.clearLogs()
}

onMounted(() => {
  connectWS()
  if (logs.value.length > 0 && autoScroll.value) {
    scrollToBottom()
  }
})

onUnmounted(() => {
  if (ws) {
    ws.close()
  }
})
</script>

<template>
  <div class="h-[calc(100vh-140px)] flex flex-col gap-4">
    <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm flex flex-wrap gap-3 items-center justify-between transition-all">
      <h3 class="text-base font-semibold flex items-center gap-2">
        <span class="w-2.5 h-2.5 rounded-full bg-success animate-pulse"></span>
        {{ t('logs.title') }}
      </h3>

      <div class="flex gap-2 items-center flex-1 sm:flex-initial min-w-[260px] sm:min-w-0">
        <input type="text" v-model="searchText" :placeholder="t('logs.search')" class="w-full sm:w-64 px-3 py-1.5 text-sm rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none" />
      </div>

      <div class="flex gap-2">
        <button @click="isPaused = !isPaused" class="px-4 py-1.5 text-xs font-semibold rounded-lg border transition-all" :class="isPaused ? 'bg-amber-500/10 text-amber-500 border-amber-500/20' : 'bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 border-transparent'">
          {{ isPaused ? t('logs.resume') : t('logs.pause') }}
        </button>
        <button @click="handleClear" class="px-4 py-1.5 text-xs font-semibold rounded-lg bg-red-500/10 hover:bg-red-500/20 text-red-500 transition-all border border-red-500/10">
          {{ t('logs.clear') }}
        </button>
      </div>
    </div>

    <div ref="terminalRef" class="flex-1 bg-slate-950 text-slate-300 font-mono text-xs p-5 rounded-2xl overflow-y-auto leading-relaxed border border-slate-800 shadow-2xl relative select-text">
      <div v-if="filteredLogs.length === 0" class="text-slate-600 flex items-center justify-center h-full">
        {{ t('logs.waiting') }}
      </div>
      <div v-else class="space-y-1">
        <div v-for="log in filteredLogs" :key="log.id" class="flex items-start gap-2 break-all hover:bg-slate-900/60 py-0.5 px-1 rounded transition-colors">
          <span class="text-slate-500 shrink-0">[{{ log.time }}]</span>
          <span class="shrink-0 font-bold uppercase text-[10px] px-1 rounded tracking-wider" :class="{
            'bg-blue-500/20 text-blue-400': log.type === 'info' || log.type === 'debug',
            'bg-amber-500/20 text-amber-400': log.type === 'warning',
            'bg-red-500/20 text-red-400': log.type === 'error'
          }">
            {{ log.type }}
          </span>
          <span :class="{
            'text-slate-300': log.type === 'info' || log.type === 'debug',
            'text-amber-300': log.type === 'warning',
            'text-red-400 font-medium': log.type === 'error'
          }">{{ log.payload }}</span>
        </div>
      </div>
      
      <button @click="autoScroll = !autoScroll" class="absolute bottom-4 right-4 bg-slate-900/90 border border-slate-700 hover:border-slate-500 text-[10px] px-2 py-1 rounded text-slate-400 flex items-center gap-1 transition-all">
        <span class="w-1.5 h-1.5 rounded-full" :class="autoScroll ? 'bg-success' : 'bg-slate-600'"></span>
        {{ t('logs.auto_scroll') }}
      </button>
    </div>
  </div>
</template>
