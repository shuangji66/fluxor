<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, nextTick, watch, onActivated, onDeactivated } from 'vue'
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

const { t } = useI18n()
const logStore = useLogStore()
const { logs, autoScroll, isPaused } = storeToRefs(logStore)

const LEVELS = ['debug', 'info', 'warning', 'error']
const currentLevel = ref('info')
const searchText = ref('')
const terminalRef = ref<HTMLDivElement | null>(null)

const scrollToBottom = () => {
  nextTick(() => {
    if (terminalRef.value) {
      terminalRef.value.scrollTop = terminalRef.value.scrollHeight
    }
  })
}

const isActive = ref(true)
onActivated(() => {
  isActive.value = true
  if (logs.value.length > 0 && autoScroll.value) {
    scrollToBottom()
  }
})
onDeactivated(() => {
  isActive.value = false
})

// 监听日志数量增加自动滚动底部，当组件处于后台时，静默冻结滚动计算
watch(() => logs.value.length, () => {
  if (isActive.value && autoScroll.value) {
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
})

const parseLogPayload = (payload: string) => {
  if (!payload) return []
  const segments: { text: string; colorClass?: string }[] = []
  const regex = /(\[[^\]]+\])|(\b(?:DIRECT|REJECT|PROXY|MATCH)\b)|(\d+\.?\d*ms)|(\b\d{1,3}(?:\.\d{1,3}){3}(?::\d+)?\b)|(\b(?:[a-zA-Z0-9-]+\.)+[a-zA-Z]{2,}(?::\d+)?\b)/g

  let lastIndex = 0
  let match

  while ((match = regex.exec(payload)) !== null) {
    if (match.index > lastIndex) {
      segments.push({ text: payload.slice(lastIndex, match.index) })
    }

    const text = match[0]
    if (match[1]) {
      let color = 'text-purple-400 font-semibold'
      const lower = text.toLowerCase()
      if (lower.includes('dns')) color = 'text-sky-400 font-semibold'
      else if (lower.includes('tcp') || lower.includes('udp')) color = 'text-emerald-400 font-semibold'
      else if (lower.includes('rule')) color = 'text-indigo-400 font-semibold'
      else if (lower.includes('proxy') || lower.includes('policy')) color = 'text-blue-400 font-semibold'
      segments.push({ text, colorClass: color })
    } else if (match[2]) {
      let color = 'text-emerald-400 font-bold'
      if (text === 'REJECT') color = 'text-red-400 font-bold'
      else if (text === 'PROXY') color = 'text-sky-400 font-bold'
      segments.push({ text, colorClass: color })
    } else if (match[3]) {
      segments.push({ text, colorClass: 'text-amber-400 font-semibold' })
    } else if (match[4]) {
      segments.push({ text, colorClass: 'text-orange-400 font-mono select-all' })
    } else if (match[5]) {
      segments.push({ text, colorClass: 'text-orange-300 font-mono select-all' })
    }

    lastIndex = regex.lastIndex
  }

  if (lastIndex < payload.length) {
    segments.push({ text: payload.slice(lastIndex) })
  }

  return segments
}

const handleClear = () => {
  logStore.clearLogs()
}

onMounted(() => {
  logStore.subscribe()
  if (logs.value.length > 0 && autoScroll.value) {
    scrollToBottom()
  }
})

onUnmounted(() => {
  logStore.unsubscribe()
})
</script>

<template>
  <div class="flex flex-col flex-1 min-h-0 gap-4 h-full relative">
    <div class="glass-medium shadow-none px-6 py-3 rounded-lg border border-apple-border flex flex-wrap gap-4 items-center justify-between transition-all shrink-0 h-auto md:h-[56px]">
      <!-- 标题和状态 -->
      <h3 class="text-base font-semibold flex items-center gap-2 shrink-0 order-1">
        <DocumentTextOutline class="w-5 h-5 text-accent" />
        {{ t('logs.title') }}
      </h3>

      <!-- 搜索、级别过滤区域 -->
      <div class="flex items-center gap-3 flex-1 justify-end min-w-[280px] sm:min-w-0 order-3 md:order-2">
        <!-- 桌面端日志级别过滤（4按钮并排） -->
        <div class="hidden sm:flex rounded-sm bg-apple-input p-0.5 border border-apple-border shrink-0 transition-all">
          <button
            v-for="level in LEVELS"
            :key="level"
            @click="currentLevel = level"
            class="px-4 py-1.5 text-xs font-semibold rounded-sm transition-all duration-200 uppercase active:scale-95"
            :class="currentLevel === level
              ? 'bg-accent text-white shadow-none'
              : 'text-apple-text-muted hover:text-apple-text'"
          >
            {{ level }}
          </button>
        </div>

        <!-- 移动端日志级别过滤（下拉 select，极致精简空间） -->
        <select
          v-model="currentLevel"
          class="sm:hidden px-2 py-1.5 text-xs rounded-sm border border-apple-border bg-apple-input text-apple-text focus:ring-1 focus:ring-accent outline-none shrink-0"
        >
          <option v-for="level in LEVELS" :key="level" :value="level">{{ level.toUpperCase() }}</option>
        </select>

        <!-- 搜索输入框 -->
        <div class="relative flex-1 sm:flex-initial sm:w-60">
          <SearchOutline class="w-3.5 h-3.5 text-apple-text-muted absolute left-2.5 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            v-model="searchText"
            :placeholder="t('logs.search')"
            class="w-full pl-8 pr-3 py-1.5 text-xs rounded-sm border border-apple-border bg-apple-input text-apple-text focus:ring-1 focus:ring-accent outline-none placeholder-apple-text-muted/50"
          />
        </div>
      </div>

      <!-- 操作按钮（暂停、清空） -->
      <div class="flex gap-2 shrink-0 order-2 md:order-3">
        <button
          @click="isPaused = !isPaused"
          class="px-3 py-1.5 text-xs font-semibold rounded-sm border transition-all flex items-center gap-1.5 whitespace-nowrap active:scale-95"
          :class="isPaused
            ? 'bg-warning/10 text-warning border-warning/20'
            : 'bg-apple-bg hover:bg-apple-border/50 text-apple-text-muted border-apple-border'"
        >
          <PlayOutline v-if="isPaused" class="w-3.5 h-3.5" />
          <PauseOutline v-else class="w-3.5 h-3.5" />
          {{ isPaused ? t('logs.resume') : t('logs.pause') }}
        </button>
        <button
          @click="handleClear"
          class="px-3 py-1.5 text-xs font-semibold rounded-sm bg-danger/10 hover:bg-danger/20 text-danger transition-all border border-danger/20 flex items-center gap-1.5 whitespace-nowrap active:scale-95"
        >
          <TrashOutline class="w-3.5 h-3.5" />
          {{ t('logs.clear') }}
        </button>
      </div>
    </div>

    <!-- 日志终端界面 -->
    <div
      ref="terminalRef"
      @scroll.passive="handleScroll"
      class="flex-1 bg-[#121214] text-zinc-300 font-mono text-xs p-3 sm:p-5 rounded-lg overflow-y-auto terminal-scroll leading-relaxed border border-apple-border relative select-text"
    >
      <div v-if="filteredLogs.length === 0" class="text-zinc-500 flex items-center justify-center h-full">
        {{ t('logs.waiting') }}
      </div>
      <div v-else class="space-y-1">
        <div
          v-for="(log, lIdx) in filteredLogs"
          :key="log.id"
          class="break-all py-1.5 px-3 rounded-xs transition-all text-zinc-300 flex items-start flex-wrap gap-x-1.5 leading-relaxed"
          :class="[
            lIdx % 2 === 0 ? 'bg-white/[0.015]' : 'bg-transparent',
            'hover:bg-white/[0.05]'
          ]"
        >
          <!-- 桌面端显示完整毫秒 -->
          <span class="text-zinc-500 font-mono text-[11px] select-none hidden sm:inline shrink-0">[{{ log.time }}]</span>
          <!-- 移动端隐藏毫秒以节省极窄的空间 -->
          <span class="text-zinc-500 font-mono text-[11px] select-none sm:hidden shrink-0">[{{ log.time.split('.')[0] }}]</span>
          
          <span
            class="inline-block align-middle font-bold uppercase text-[9px] sm:text-[10px] px-1.5 py-0.5 rounded-xs tracking-wider text-center min-w-[45px] sm:min-w-[56px] select-none mr-1.5 sm:mr-2"
            :class="{
              'bg-sky-400/20 text-sky-400': log.type === 'info' || log.type === 'debug',
              'bg-amber-400/20 text-amber-400': log.type === 'warning',
              'bg-red-400/20 text-red-400': log.type === 'error'
            }"
          >
            {{ log.type }}
          </span>
          <span
            :class="{
              'text-zinc-100': log.type === 'info' || log.type === 'debug',
              'text-amber-400': log.type === 'warning',
              'text-red-400 font-medium': log.type === 'error'
            }"
          >
            <template v-for="(seg, idx) in parseLogPayload(log.payload)" :key="idx">
              <span v-if="seg.colorClass" :class="seg.colorClass">{{ seg.text }}</span>
              <template v-else>{{ seg.text }}</template>
            </template>
          </span>
        </div>
      </div>
    </div>

    <!-- 智能自动滚动控制悬浮钮（提升至滚动区外层以实现固定定位） -->
    <button
      v-if="filteredLogs.length > 0"
      @click="autoScroll = !autoScroll"
      class="absolute bottom-4 right-4 bg-apple-card border border-apple-border text-[10px] px-3.5 py-1.5 rounded-full text-apple-text-muted flex items-center gap-1.5 transition-all shadow-none z-40 select-none hover:bg-apple-bg active:scale-95"
    >
      <ArrowDownOutline class="w-3 h-3 transition-transform duration-200" :class="{ 'translate-y-0.5 animate-bounce text-success': autoScroll }" />
      {{ t('logs.auto_scroll') }}
    </button>
  </div>
</template>
