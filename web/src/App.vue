<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useGlobalStore } from './store/global'
import { useConfigStore } from './store/config'
import { useOverviewStore } from './store/overview'

// 视图组件导入
import Overview from './views/Overview.vue'
import Proxies from './views/Proxies.vue'
import Rules from './views/Rules.vue'
import Connections from './views/Connections.vue'
import Logs from './views/Logs.vue'
import Config from './views/Config.vue'
import Subscription from './views/Subscription.vue'

const { t, locale } = useI18n()
const globalStore = useGlobalStore()
const configStore = useConfigStore()
const overviewStore = useOverviewStore()

const components: Record<string, any> = {
  overview: Overview,
  proxies: Proxies,
  rules: Rules,
  connections: Connections,
  logs: Logs,
  config: Config,
  subscription: Subscription
}

const activeComponent = computed(() => {
  return components[globalStore.activeTab] || Overview
})

// === 侧边栏折叠/抽屉控制 ===
const toggleSidebar = () => {
  globalStore.isSidebarCollapsed = !globalStore.isSidebarCollapsed
}

// === 主题管理 ===
const applyTheme = (themeName: string) => {
  let effectiveTheme = themeName
  if (themeName === 'system') {
    effectiveTheme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  }
  document.documentElement.setAttribute('data-theme', effectiveTheme)
  if (effectiveTheme === 'dark') {
    document.documentElement.classList.add('dark')
  } else {
    document.documentElement.classList.remove('dark')
  }
}

watch(() => globalStore.theme, (newTheme: string) => {
  localStorage.setItem('fluxor-theme', newTheme)
  applyTheme(newTheme)
})

let systemThemeListener: ((e: MediaQueryListEvent) => void) | null = null

const initTheme = () => {
  const saved = globalStore.theme
  applyTheme(saved)

  systemThemeListener = (e: MediaQueryListEvent) => {
    if (globalStore.theme === 'system') {
      applyTheme('system')
    }
  }
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', systemThemeListener)
}

const switchThemeCycle = () => {
  const current = globalStore.theme
  const cycle: Record<string, string> = { light: 'dark', dark: 'system', system: 'light' }
  globalStore.theme = cycle[current]
}

// === 语言管理 ===
const currentLangDisplay = computed(() => {
  return locale.value === 'zh' ? '简' : 'EN'
})

const toggleLanguage = () => {
  const target = locale.value === 'zh' ? 'en' : 'zh'
  locale.value = target
  localStorage.setItem('lang', target)
  // 更新页面标题
  document.title = 'Fluxor - ' + t('nav.' + globalStore.activeTab)
}

watch(() => globalStore.activeTab, (newTab: string) => {
  document.title = 'Fluxor - ' + t('nav.' + newTab)
})

// 切换导航 Tab，在移动端自动收起侧边栏
const selectTab = (tabName: string) => {
  globalStore.activeTab = tabName
  if (window.innerWidth < 768) {
    globalStore.isSidebarCollapsed = true
  }
}

// 记录上次宽度
let lastWidth = window.innerWidth

const handleResize = () => {
  const currentWidth = window.innerWidth
  // 跨越 768px 断点时处理状态适配
  if ((lastWidth >= 768 && currentWidth < 768) || (lastWidth < 768 && currentWidth >= 768)) {
    if (currentWidth < 768) {
      // 切换到移动端：侧边栏默认强制折叠收起
      globalStore.isSidebarCollapsed = true
    } else {
      // 切换到桌面端：恢复用户的折叠历史设置
      globalStore.isSidebarCollapsed = localStorage.getItem('fluxor-sidebar-collapsed') === 'true'
    }
  }
  lastWidth = currentWidth
}

onMounted(() => {
  initTheme()
  document.title = 'Fluxor - ' + t('nav.' + globalStore.activeTab)
  
  // 初始化检测：若直接在手机小屏载入，默认收起
  if (window.innerWidth < 768) {
    globalStore.isSidebarCollapsed = true
  }
  
  window.addEventListener('resize', handleResize)
  
  // 预加载配置与订阅状态
  configStore.fetchCoreStatus()
  configStore.fetchConfigs()
  configStore.loadConfig()
})

onUnmounted(() => {
  if (systemThemeListener) {
    window.matchMedia('(prefers-color-scheme: dark)').removeEventListener('change', systemThemeListener)
  }
  window.removeEventListener('resize', handleResize)
})
</script>

<template>
  <div class="flex h-screen w-screen overflow-hidden bg-[#f1f5f9] dark:bg-[#0f172a] transition-colors duration-200">
    
    <!-- 侧边栏 aside -->
    <aside class="hidden md:flex md:static inset-y-0 left-0 bg-white dark:bg-[#1e293b] border-r border-slate-200 dark:border-slate-800/80 z-50 flex-col justify-between transition-all duration-200 overflow-y-auto overflow-x-hidden"
      :class="[
        globalStore.isSidebarCollapsed ? 'md:w-16' : 'md:w-60'
      ]">
      <div class="p-4 flex items-center border-b border-slate-100 dark:border-slate-800/60 transition-all duration-200"
        :class="globalStore.isSidebarCollapsed ? 'justify-center' : 'justify-between'">
        <span class="font-bold text-accent tracking-wider text-base select-none transition-all duration-200 whitespace-nowrap overflow-hidden"
          :class="globalStore.isSidebarCollapsed ? 'opacity-0 w-0' : 'opacity-100 w-auto'">Fluxor</span>
        <button @click="toggleSidebar" class="p-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 rounded transition-all flex items-center justify-center">
          <svg v-if="!globalStore.isSidebarCollapsed" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="w-5 h-5"><polyline points="11 17 6 12 11 7"></polyline><polyline points="18 17 13 12 18 7"></polyline></svg>
          <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="w-5 h-5"><polyline points="13 17 18 12 13 7"></polyline><polyline points="6 17 11 12 6 7"></polyline></svg>
        </button>
      </div>

      <!-- 导航项目 -->
      <nav class="flex-1 px-3 py-4 space-y-1">
        <!-- 概览 -->
        <button @click="selectTab('overview')" class="w-full flex items-center py-2.5 rounded-xl font-medium text-sm transition-all duration-200"
          :class="[
            globalStore.activeTab === 'overview' ? 'bg-accent/10 text-accent' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed ? 'justify-center px-0' : 'px-3 gap-3'
          ]">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-5 h-5 shrink-0"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/></svg>
          <span class="transition-all duration-200 whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 w-0' : 'opacity-100 w-auto'">
            {{ t('nav.overview') }}
          </span>
        </button>

        <!-- 代理 -->
        <button @click="selectTab('proxies')" class="w-full flex items-center py-2.5 rounded-xl font-medium text-sm transition-all duration-200"
          :class="[
            globalStore.activeTab === 'proxies' ? 'bg-accent/10 text-accent' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed ? 'justify-center px-0' : 'px-3 gap-3'
          ]">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-5 h-5 shrink-0"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>
          <span class="transition-all duration-200 whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 w-0' : 'opacity-100 w-auto'">
            {{ t('nav.proxies') }}
          </span>
        </button>

        <!-- 订阅 -->
        <button @click="selectTab('subscription')" class="w-full flex items-center py-2.5 rounded-xl font-medium text-sm transition-all duration-200"
          :class="[
            globalStore.activeTab === 'subscription' ? 'bg-accent/10 text-accent' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed ? 'justify-center px-0' : 'px-3 gap-3'
          ]">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-5 h-5 shrink-0"><rect x="2" y="4" width="20" height="16" rx="2"/><polyline points="2 8 12 14 22 8"/></svg>
          <span class="transition-all duration-200 whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 w-0' : 'opacity-100 w-auto'">
            {{ t('nav.subscription') }}
          </span>
        </button>

        <!-- 规则 -->
        <button @click="selectTab('rules')" class="w-full flex items-center py-2.5 rounded-xl font-medium text-sm transition-all duration-200"
          :class="[
            globalStore.activeTab === 'rules' ? 'bg-accent/10 text-accent' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed ? 'justify-center px-0' : 'px-3 gap-3'
          ]">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-5 h-5 shrink-0"><polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/></svg>
          <span class="transition-all duration-200 whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 w-0' : 'opacity-100 w-auto'">
            {{ t('nav.rules') }}
          </span>
        </button>

        <!-- 连接 -->
        <button @click="selectTab('connections')" class="w-full flex items-center py-2.5 rounded-xl font-medium text-sm transition-all duration-200"
          :class="[
            globalStore.activeTab === 'connections' ? 'bg-accent/10 text-accent' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed ? 'justify-center px-0' : 'px-3 gap-3'
          ]">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-5 h-5 shrink-0"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>
          <span class="transition-all duration-200 whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 w-0' : 'opacity-100 w-auto'">
            {{ t('nav.connections') }}
          </span>
        </button>

        <!-- 日志 -->
        <button @click="selectTab('logs')" class="w-full flex items-center py-2.5 rounded-xl font-medium text-sm transition-all duration-200"
          :class="[
            globalStore.activeTab === 'logs' ? 'bg-accent/10 text-accent' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed ? 'justify-center px-0' : 'px-3 gap-3'
          ]">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-5 h-5 shrink-0"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>
          <span class="transition-all duration-200 whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 w-0' : 'opacity-100 w-auto'">
            {{ t('nav.logs') }}
          </span>
        </button>

        <!-- 配置 -->
        <button @click="selectTab('config')" class="w-full flex items-center py-2.5 rounded-xl font-medium text-sm transition-all duration-200"
          :class="[
            globalStore.activeTab === 'config' ? 'bg-accent/10 text-accent' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed ? 'justify-center px-0' : 'px-3 gap-3'
          ]">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-5 h-5 shrink-0"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
          <span class="transition-all duration-200 whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 w-0' : 'opacity-100 w-auto'">
            {{ t('nav.config') }}
          </span>
        </button>
      </nav>

      <!-- 底部操作：中英文与亮暗主题 -->
      <div class="p-3 border-t border-slate-100 dark:border-slate-800/60 flex gap-2 transition-all duration-200"
        :class="globalStore.isSidebarCollapsed ? 'flex-col' : 'flex-col md:flex-row'">
        <!-- 切换语言 -->
        <button @click="toggleLanguage" class="flex-1 flex items-center justify-center py-2 text-xs font-semibold rounded-lg bg-slate-50 hover:bg-slate-100 dark:bg-slate-800/40 dark:hover:bg-slate-800 transition-all text-slate-500 dark:text-slate-400">
          <span>{{ currentLangDisplay }}</span>
        </button>
        <!-- 切换主题 -->
        <button @click="switchThemeCycle" class="flex-1 flex items-center justify-center py-2 text-xs font-semibold rounded-lg bg-slate-50 hover:bg-slate-100 dark:bg-slate-800/40 dark:hover:bg-slate-800 transition-all text-slate-500 dark:text-slate-400" aria-label="Toggle Theme">
          <svg v-if="globalStore.theme === 'dark'" class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
          </svg>
          <svg v-else-if="globalStore.theme === 'light'" class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="5"/>
            <line x1="12" y1="1" x2="12" y2="3"/>
            <line x1="12" y1="21" x2="12" y2="23"/>
            <line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/>
            <line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/>
            <line x1="1" y1="12" x2="3" y2="12"/>
            <line x1="21" y1="12" x2="23" y2="12"/>
            <line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/>
            <line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/>
          </svg>
          <svg v-else class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="10"/>
            <text x="12" y="16" font-size="14" text-anchor="middle" fill="currentColor" stroke="none">A</text>
          </svg>
        </button>
      </div>
    </aside>

    <!-- 移动端顶部标题栏 -->
    <div class="flex-1 flex flex-col min-w-0">
      <header class="md:hidden flex h-14 bg-white dark:bg-[#1e293b] border-b border-slate-200 dark:border-slate-800/80 px-4 justify-between items-center z-30 shadow-sm shrink-0">
        <div class="flex items-center gap-3">
          <span class="font-bold text-slate-800 dark:text-slate-100 text-base select-none">Fluxor</span>
        </div>

        <!-- 移动端快捷设置 -->
        <div class="flex gap-2 items-center">
          <!-- 语言 -->
          <button @click="toggleLanguage" class="px-2 py-1.5 text-xs font-semibold rounded bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 min-w-8">
            {{ currentLangDisplay }}
          </button>
          <!-- 主题 -->
          <button @click="switchThemeCycle" class="p-1.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 flex items-center justify-center" aria-label="Toggle Theme">
            <svg v-if="globalStore.theme === 'dark'" class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
            </svg>
            <svg v-else-if="globalStore.theme === 'light'" class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="5"/>
              <line x1="12" y1="1" x2="12" y2="3"/>
              <line x1="12" y1="21" x2="12" y2="23"/>
              <line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/>
              <line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/>
              <line x1="1" y1="12" x2="3" y2="12"/>
              <line x1="21" y1="12" x2="23" y2="12"/>
              <line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/>
              <line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/>
            </svg>
            <svg v-else class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="10"/>
              <text x="12" y="16" font-size="14" text-anchor="middle" fill="currentColor" stroke="none">A</text>
            </svg>
          </button>
        </div>
      </header>

      <!-- 主工作区容器 -->
      <main class="flex-1 overflow-y-auto p-4 pb-20 md:pb-4 select-none">
        <component :is="activeComponent" />
      </main>

      <!-- 移动端底部选项卡 Bar -->
      <nav class="md:hidden fixed bottom-0 inset-x-0 h-14 bg-white/95 dark:bg-[#1e293b]/95 backdrop-blur-md border-t border-slate-200 dark:border-slate-800/80 flex items-center justify-around z-40 shadow-lg">
        <!-- 概览 -->
        <button @click="selectTab('overview')" class="flex flex-col items-center gap-0.5 transition-all duration-200 active:scale-95" :class="globalStore.activeTab === 'overview' ? 'text-accent font-semibold scale-105' : 'text-slate-500 dark:text-slate-400'">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-5 h-5"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/></svg>
          <span class="text-[9px] font-medium">{{ t('nav.overview') }}</span>
        </button>
        <!-- 代理 -->
        <button @click="selectTab('proxies')" class="flex flex-col items-center gap-0.5 transition-all duration-200 active:scale-95" :class="globalStore.activeTab === 'proxies' ? 'text-accent font-semibold scale-105' : 'text-slate-500 dark:text-slate-400'">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-5 h-5"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>
          <span class="text-[9px] font-medium">{{ t('nav.proxies') }}</span>
        </button>
        <!-- 订阅 -->
        <button @click="selectTab('subscription')" class="flex flex-col items-center gap-0.5 transition-all duration-200 active:scale-95" :class="globalStore.activeTab === 'subscription' ? 'text-accent font-semibold scale-105' : 'text-slate-500 dark:text-slate-400'">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-5 h-5"><rect x="2" y="4" width="20" height="16" rx="2"/><polyline points="2 8 12 14 22 8"/></svg>
          <span class="text-[9px] font-medium">{{ t('nav.subscription') }}</span>
        </button>
        <!-- 规则 -->
        <button @click="selectTab('rules')" class="flex flex-col items-center gap-0.5 transition-all duration-200 active:scale-95" :class="globalStore.activeTab === 'rules' ? 'text-accent font-semibold scale-105' : 'text-slate-500 dark:text-slate-400'">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-5 h-5"><polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/></svg>
          <span class="text-[9px] font-medium">{{ t('nav.rules') }}</span>
        </button>
        <!-- 连接 -->
        <button @click="selectTab('connections')" class="flex flex-col items-center gap-0.5 transition-all duration-200 active:scale-95" :class="globalStore.activeTab === 'connections' ? 'text-accent font-semibold scale-105' : 'text-slate-500 dark:text-slate-400'">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-5 h-5"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>
          <span class="text-[9px] font-medium">{{ t('nav.connections') }}</span>
        </button>
        <!-- 日志 -->
        <button @click="selectTab('logs')" class="flex flex-col items-center gap-0.5 transition-all duration-200 active:scale-95" :class="globalStore.activeTab === 'logs' ? 'text-accent font-semibold scale-105' : 'text-slate-500 dark:text-slate-400'">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-5 h-5"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>
          <span class="text-[9px] font-medium">{{ t('nav.logs') }}</span>
        </button>
        <!-- 配置 -->
        <button @click="selectTab('config')" class="flex flex-col items-center gap-0.5 transition-all duration-200 active:scale-95" :class="globalStore.activeTab === 'config' ? 'text-accent font-semibold scale-105' : 'text-slate-500 dark:text-slate-400'">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-5 h-5"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
          <span class="text-[9px] font-medium">{{ t('nav.config') }}</span>
        </button>
      </nav>
    </div>

    <!-- 全局 Confirm 确认框 -->
    <div v-if="globalStore.confirmDialog && globalStore.confirmDialog.visible" class="fixed inset-0 bg-slate-900/60 dark:bg-slate-950/80 backdrop-blur-sm z-[9999] flex items-center justify-center p-4">
      <div class="bg-white dark:bg-[#1e293b] w-full max-w-[92vw] sm:max-w-md rounded-2xl shadow-xl border border-slate-200 dark:border-slate-800/80 p-6 flex flex-col gap-5 animate-[zoomIn_0.15s_ease-out]">
        <div class="flex flex-col gap-3">
          <h3 v-if="globalStore.confirmDialog.title" class="text-base font-bold text-slate-800 dark:text-slate-100">
            {{ globalStore.confirmDialog.title }}
          </h3>
          <p class="text-sm text-slate-600 dark:text-slate-300 leading-relaxed break-words whitespace-pre-line">
            {{ globalStore.confirmDialog.message }}
          </p>
        </div>
        <div class="flex justify-end gap-2.5 pt-2">
          <button @click="globalStore.handleConfirmResult(false)" class="px-5 py-2.5 text-sm font-semibold rounded-xl bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-600 dark:text-slate-300 transition-all duration-200 active:scale-95 select-none">
            {{ globalStore.confirmDialog.cancelText || t('common.cancel') }}
          </button>
          <button @click="globalStore.handleConfirmResult(true)" class="px-5 py-2.5 text-sm font-semibold rounded-xl bg-accent hover:bg-accent-hover text-white transition-all duration-200 active:scale-95 shadow-md shadow-accent/15 select-none">
            {{ globalStore.confirmDialog.okText || t('common.confirm') }}
          </button>
        </div>
      </div>
    </div>

    <!-- 全局 Toast 提示容器 -->
    <div class="fixed top-4 right-4 z-[9999] flex flex-col gap-2 pointer-events-none max-w-sm w-full px-4">
      <div v-for="toast in globalStore.toasts" :key="toast.id" class="p-3.5 rounded-xl shadow-lg border text-xs font-semibold flex items-center justify-between gap-3 animate-[slideIn_0.2s_ease-out] pointer-events-auto backdrop-blur-md"
        :class="{
          'bg-success/10 border-success/30 text-success': toast.type === 'success',
          'bg-red-500/10 border-red-500/30 text-red-500': toast.type === 'error',
          'bg-amber-500/10 border-amber-500/30 text-amber-500': toast.type === 'warning',
          'bg-slate-500/10 border-slate-500/30 text-slate-500 dark:text-slate-300': toast.type === 'info'
        }">
        <span>{{ toast.text }}</span>
      </div>
    </div>
  </div>
</template>

<style>
@keyframes slideIn {
  from { opacity: 0; transform: translateY(-20px) scale(0.95); }
  to { opacity: 1; transform: translateY(0) scale(1); }
}
@keyframes zoomIn {
  from { opacity: 0; transform: scale(0.95); }
  to { opacity: 1; transform: scale(1); }
}
</style>
