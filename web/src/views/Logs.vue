<script setup lang="ts">
<<<<<<< HEAD
import { ref, onMounted, onUnmounted, computed, nextTick, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { useLogStore } from '../store/logs'
import {
  TrashOutline,
  PauseOutline,
  PlayOutline,
  DocumentTextOutline,
  SearchOutline,
  ArrowDownOutline
} from '@vicons/ionicons5'
=======
import { ref, onMounted, onUnmounted, computed, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { wsConnect } from '../utils/api'
import { storeToRefs } from 'pinia'
import { useLogStore, type LogItem } from '../store/logs'
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69

const { t } = useI18n()
const logStore = useLogStore()
const { logs, autoScroll, isPaused } = storeToRefs(logStore)

<<<<<<< HEAD
const LEVELS = ['debug', 'info', 'warning', 'error']
const currentLevel = ref('info')
const searchText = ref('')
const terminalRef = ref<HTMLDivElement | null>(null)

=======
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

>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
const scrollToBottom = () => {
  nextTick(() => {
    if (terminalRef.value) {
      terminalRef.value.scrollTop = terminalRef.value.scrollHeight
    }
  })
}

<<<<<<< HEAD
// 监听日志数量增加自动滚动底部
watch(() => logs.value.length, () => {
  if (autoScroll.value) {
    scrollToBottom()
  }
})

// 监听滚动事件，支持手动/自动滚动模式切换
const handleScroll = () => {
  if (!terminalRef.value) return
  const { scrollTop, scrollHeight, clientHeight } = terminalRef.value
  // 滚动条距离底部小于 30px 时判定为触底并开启自动滚动
  const isAtBottom = scrollHeight - scrollTop - clientHeight < 30
  autoScroll.value = isAtBottom
}

const filteredLogs = computed(() => {
  const curIdx = LEVELS.indexOf(currentLevel.value)
  const list = logs.value.filter(log => {
    const logIdx = LEVELS.indexOf(log.type.toLowerCase())
    const itemIdx = logIdx === -1 ? 1 : logIdx
    return itemIdx >= curIdx
  })
  if (!searchText.value.trim()) return list
  const query = searchText.value.toLowerCase()
  return list.filter(log => log.payload.toLowerCase().includes(query))
=======
const filteredLogs = computed(() => {
  if (!searchText.value.trim()) return logs.value
  const query = searchText.value.toLowerCase()
  return logs.value.filter(log => log.payload.toLowerCase().includes(query))
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
})

const handleClear = () => {
  logStore.clearLogs()
}

onMounted(() => {
<<<<<<< HEAD
  logStore.subscribe()
=======
  connectWS()
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
  if (logs.value.length > 0 && autoScroll.value) {
    scrollToBottom()
  }
})

onUnmounted(() => {
<<<<<<< HEAD
  logStore.unsubscribe()
=======
  if (ws) {
    ws.close()
  }
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
})
</script>

<template>
<<<<<<< HEAD
  <div class="h-[calc(100vh-160px)] md:h-[calc(100vh-140px)] flex flex-col gap-4">
    <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm flex flex-wrap gap-4 items-center justify-between transition-all">
      <h3 class="text-base font-semibold flex items-center gap-2">
        <DocumentTextOutline class="w-5 h-5 text-accent" />
        {{ t('logs.title') }}
        <span class="w-2 h-2 rounded-full bg-success animate-pulse ml-1"></span>
      </h3>

      <div class="flex flex-wrap items-center gap-3 flex-1 sm:flex-initial">
        <!-- 日志级别过滤 -->
        <div class="flex rounded-lg bg-slate-100 dark:bg-slate-800 p-0.5 border border-slate-200 dark:border-slate-700/50">
          <button
            v-for="level in LEVELS"
            :key="level"
            @click="currentLevel = level"
            class="px-3 py-1 text-xs font-semibold rounded-md transition-all uppercase"
            :class="currentLevel === level
              ? 'bg-white dark:bg-slate-700 text-accent shadow-sm'
              : 'text-slate-500 dark:text-slate-400 hover:text-slate-800 dark:hover:text-slate-200'"
          >
            {{ level }}
          </button>
        </div>

        <!-- 搜索输入框 -->
        <div class="relative w-full sm:w-64">
          <SearchOutline class="w-4 h-4 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            v-model="searchText"
            :placeholder="t('logs.search')"
            class="w-full pl-9 pr-3 py-1.5 text-sm rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none"
          />
        </div>
      </div>

      <div class="flex gap-2">
        <button
          @click="isPaused = !isPaused"
          class="px-4 py-1.5 text-xs font-semibold rounded-lg border transition-all flex items-center gap-1.5"
          :class="isPaused
            ? 'bg-amber-500/10 text-amber-500 border-amber-500/20 hover:bg-amber-500/20'
            : 'bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 border-transparent text-slate-700 dark:text-slate-300'"
        >
          <PlayOutline v-if="isPaused" class="w-3.5 h-3.5" />
          <PauseOutline v-else class="w-3.5 h-3.5" />
          {{ isPaused ? t('logs.resume') : t('logs.pause') }}
        </button>
        <button
          @click="handleClear"
          class="px-4 py-1.5 text-xs font-semibold rounded-lg bg-red-500/10 hover:bg-red-500/20 text-red-500 transition-all border border-red-500/10 flex items-center gap-1.5"
        >
          <TrashOutline class="w-3.5 h-3.5" />
=======
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
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
          {{ t('logs.clear') }}
        </button>
      </div>
    </div>

<<<<<<< HEAD
    <!-- 日志终端界面 -->
    <div
      ref="terminalRef"
      @scroll.passive="handleScroll"
      class="flex-1 bg-slate-950 text-slate-300 font-mono text-xs p-5 rounded-2xl overflow-y-auto leading-relaxed border border-slate-800 shadow-2xl relative select-text"
    >
=======
    <div ref="terminalRef" class="flex-1 bg-slate-950 text-slate-300 font-mono text-xs p-5 rounded-2xl overflow-y-auto leading-relaxed border border-slate-800 shadow-2xl relative select-text">
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
      <div v-if="filteredLogs.length === 0" class="text-slate-600 flex items-center justify-center h-full">
        {{ t('logs.waiting') }}
      </div>
      <div v-else class="space-y-1">
<<<<<<< HEAD
        <div
          v-for="log in filteredLogs"
          :key="log.id"
          class="flex items-start gap-2 break-all hover:bg-slate-900/60 py-0.5 px-1 rounded transition-colors"
        >
          <span class="text-slate-500 shrink-0 select-none">[{{ log.time }}]</span>
          <span
            class="shrink-0 font-bold uppercase text-[10px] px-1.5 py-0.5 rounded tracking-wider text-center min-w-[56px] select-none"
            :class="{
              'bg-blue-500/20 text-blue-400': log.type === 'info' || log.type === 'debug',
              'bg-amber-500/20 text-amber-400': log.type === 'warning',
              'bg-red-500/20 text-red-400': log.type === 'error'
            }"
          >
            {{ log.type }}
          </span>
          <span
            :class="{
              'text-slate-300': log.type === 'info' || log.type === 'debug',
              'text-amber-300': log.type === 'warning',
              'text-red-400 font-medium': log.type === 'error'
            }"
          >{{ log.payload }}</span>
        </div>
      </div>
      
      <!-- 智能自动滚动控制悬浮钮 -->
      <button
        @click="autoScroll = !autoScroll"
        class="absolute bottom-4 right-4 bg-slate-900/90 hover:bg-slate-800 border border-slate-700 hover:border-slate-500 text-[10px] px-2 py-1.5 rounded-lg text-slate-400 flex items-center gap-1.5 transition-all shadow-lg"
      >
        <ArrowDownOutline class="w-3 h-3 transition-transform duration-200" :class="{ 'translate-y-0.5 animate-bounce text-success': autoScroll }" />
=======
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
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
        {{ t('logs.auto_scroll') }}
      </button>
    </div>
  </div>
</template>
<<<<<<< HEAD

=======
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
