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
    <div class="glass-medium shadow-none px-6 py-3 md:py-0 rounded-xl border border-slate-200/50 dark:border-slate-800/50 flex flex-wrap gap-4 items-center justify-between transition-all shrink-0 h-auto min-h-[56px] md:h-[56px]">
      <!-- 标题和状态 -->
      <h3 class="text-base font-semibold flex items-center gap-2 shrink-0 order-1">
        <DocumentTextOutline class="w-5 h-5 text-accent" />
        {{ t('logs.title') }}
      </h3>

      <!-- 搜索、级别过滤区域 -->
      <div class="flex items-center gap-3 flex-1 justify-end min-w-[280px] sm:min-w-0 order-3 md:order-2">
        <!-- 桌面端日志级别过滤（4按钮并排） -->
        <div class="hidden sm:flex rounded-lg bg-slate-100 dark:bg-slate-800 p-0.5 border border-slate-200 dark:border-slate-700/50 shrink-0 transition-all">
          <button
            v-for="level in LEVELS"
            :key="level"
            @click="currentLevel = level"
            class="px-4 py-1.5 text-xs font-semibold rounded-md transition-all duration-200 uppercase"
            :class="currentLevel === level
              ? 'bg-accent text-white shadow-sm'
              : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'"
          >
            {{ level }}
          </button>
        </div>

        <!-- 移动端日志级别过滤（下拉 select，极致精简空间） -->
        <select
          v-model="currentLevel"
          class="sm:hidden px-2 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none shrink-0"
        >
          <option v-for="level in LEVELS" :key="level" :value="level">{{ level.toUpperCase() }}</option>
        </select>

        <!-- 搜索输入框 -->
        <div class="relative flex-1 sm:flex-initial sm:w-60">
          <SearchOutline class="w-3.5 h-3.5 text-slate-400 absolute left-2.5 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            v-model="searchText"
            :placeholder="t('logs.search')"
            class="w-full pl-8 pr-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none"
          />
        </div>
      </div>

      <!-- 操作按钮（暂停、清空） -->
      <div class="flex gap-2 shrink-0 order-2 md:order-3">
        <button
          @click="isPaused = !isPaused"
          class="px-3 py-1.5 text-xs font-semibold rounded-lg border transition-all flex items-center gap-1.5 whitespace-nowrap"
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
          class="px-3 py-1.5 text-xs font-semibold rounded-lg bg-red-500/10 hover:bg-red-500/20 text-red-500 transition-all border border-red-500/10 flex items-center gap-1.5 whitespace-nowrap"
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
      class="flex-1 bg-slate-950 text-slate-300 font-mono text-xs p-3 sm:p-5 rounded-xl overflow-y-auto leading-relaxed border border-slate-800/50 relative select-text"
    >
      <div v-if="filteredLogs.length === 0" class="text-slate-600 flex items-center justify-center h-full">
        {{ t('logs.waiting') }}
      </div>
      <div v-else class="space-y-1">
        <div
          v-for="log in filteredLogs"
          :key="log.id"
          class="break-all hover:bg-slate-900/60 py-0.5 px-1 rounded transition-colors text-slate-300"
        >
          <!-- 桌面端显示完整毫秒 -->
          <span class="text-slate-500 select-none hidden sm:inline mr-1.5 sm:mr-2">[{{ log.time }}]</span>
          <!-- 移动端隐藏毫秒以节省极窄的空间 -->
          <span class="text-slate-500 select-none sm:hidden mr-1.5 sm:mr-2">[{{ log.time.split('.')[0] }}]</span>
          
          <span
            class="inline-block align-middle font-bold uppercase text-[9px] sm:text-[10px] px-1 py-0.5 rounded tracking-wider text-center min-w-[45px] sm:min-w-[56px] select-none mr-1.5 sm:mr-2"
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
    </div>

    <!-- 智能自动滚动控制悬浮钮（提升至滚动区外层以实现固定定位） -->
    <button
      v-if="filteredLogs.length > 0"
      @click="autoScroll = !autoScroll"
      class="absolute bottom-4 right-4 bg-slate-900/90 hover:bg-slate-800 border border-slate-700 hover:border-slate-500 text-[10px] px-2.5 py-1.5 rounded-lg text-slate-400 flex items-center gap-1.5 transition-all shadow-lg z-40 select-none"
    >
      <ArrowDownOutline class="w-3 h-3 transition-transform duration-200" :class="{ 'translate-y-0.5 animate-bounce text-success': autoScroll }" />
      {{ t('logs.auto_scroll') }}
    </button>
  </div>
</template>
