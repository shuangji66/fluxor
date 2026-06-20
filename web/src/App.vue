<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useGlobalStore } from './store/global'
import { useConfigStore } from './store/config'
import { useOverviewStore } from './store/overview'
import {
  LanguageOutline,
  SunnyOutline,
  MoonOutline,
  ColorPaletteOutline,
  HeartOutline,
  DesktopOutline,
  ChevronBackOutline,
  ChevronForwardOutline,
  InformationCircleOutline,
  LogoGithub,
  GridOutline,
  GlobeOutline,
  MailOutline,
  LayersOutline,
  LinkOutline,
  DocumentTextOutline,
  SettingsOutline,
  CheckmarkCircleOutline,
  CloseCircleOutline,
  AlertCircleOutline,
  CloseOutline
} from '@vicons/ionicons5'

// 视图组件导入
import Overview from './views/Overview.vue'
import Proxies from './views/Proxies.vue'
import Rules from './views/Rules.vue'
import Connections from './views/Connections.vue'
import Logs from './views/Logs.vue'
import Config from './views/Config.vue'
import Subscription from './views/Subscription.vue'

const appVersion = __APP_VERSION__

const { t, locale } = useI18n()
const globalStore = useGlobalStore()
const configStore = useConfigStore()
const overviewStore = useOverviewStore()

const showAbout = ref(false)

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
  if (effectiveTheme === 'dark' || effectiveTheme === 'purple') {
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
  const cycle: Record<string, string> = {
    light: 'dark',
    dark: 'purple',
    purple: 'pink',
    pink: 'system',
    system: 'light'
  }
  globalStore.theme = cycle[current] || 'system'
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

// 切换导航 Tab，在移动端自动收起侧边栏并释放聚焦状态
const selectTab = (tabName: string) => {
  if (document.activeElement instanceof HTMLElement) {
    document.activeElement.blur()
  }
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

// 监听内核运行状态，若已运行且版本号未加载，立即主动拉取一次
watch(() => configStore.coreStatus.running, (running) => {
  if (running && (overviewStore.stats.coreVersion === '加载中...' || overviewStore.stats.coreVersion === '未知')) {
    overviewStore.fetchVersionAndStatus()
  }
}, { immediate: true })

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
        <button @click="toggleSidebar" class="p-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 rounded transition-all flex items-center justify-center" aria-label="Toggle Sidebar">
          <ChevronBackOutline v-if="!globalStore.isSidebarCollapsed" class="w-5 h-5 transition-all duration-200 hover:scale-110" />
          <ChevronForwardOutline v-else class="w-5 h-5 transition-all duration-200 hover:scale-110" />
        </button>
      </div>

      <!-- 导航项目 -->
      <nav class="flex-1 px-3 py-4 space-y-1">
        <!-- 概览 -->
        <button @click="selectTab('overview')" class="w-full flex items-center py-2.5 rounded-xl font-medium text-sm transition-all duration-200 active:scale-95"
          :class="[
            globalStore.activeTab === 'overview' ? 'bg-accent/10 text-accent font-bold' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed ? 'justify-center px-0' : 'px-3 gap-3'
          ]">
          <GridOutline class="w-5 h-5 shrink-0" />
          <span class="transition-all duration-200 whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 w-0' : 'opacity-100 w-auto'">
            {{ t('nav.overview') }}
          </span>
        </button>

        <!-- 代理 -->
        <button @click="selectTab('proxies')" class="w-full flex items-center py-2.5 rounded-xl font-medium text-sm transition-all duration-200 active:scale-95"
          :class="[
            globalStore.activeTab === 'proxies' ? 'bg-accent/10 text-accent font-bold' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed ? 'justify-center px-0' : 'px-3 gap-3'
          ]">
          <GlobeOutline class="w-5 h-5 shrink-0" />
          <span class="transition-all duration-200 whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 w-0' : 'opacity-100 w-auto'">
            {{ t('nav.proxies') }}
          </span>
        </button>

        <!-- 订阅 -->
        <button @click="selectTab('subscription')" class="w-full flex items-center py-2.5 rounded-xl font-medium text-sm transition-all duration-200 active:scale-95"
          :class="[
            globalStore.activeTab === 'subscription' ? 'bg-accent/10 text-accent font-bold' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed ? 'justify-center px-0' : 'px-3 gap-3'
          ]">
          <MailOutline class="w-5 h-5 shrink-0" />
          <span class="transition-all duration-200 whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 w-0' : 'opacity-100 w-auto'">
            {{ t('nav.subscription') }}
          </span>
        </button>

        <!-- 规则 -->
        <button @click="selectTab('rules')" class="w-full flex items-center py-2.5 rounded-xl font-medium text-sm transition-all duration-200 active:scale-95"
          :class="[
            globalStore.activeTab === 'rules' ? 'bg-accent/10 text-accent font-bold' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed ? 'justify-center px-0' : 'px-3 gap-3'
          ]">
          <LayersOutline class="w-5 h-5 shrink-0" />
          <span class="transition-all duration-200 whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 w-0' : 'opacity-100 w-auto'">
            {{ t('nav.rules') }}
          </span>
        </button>

        <!-- 连接 -->
        <button @click="selectTab('connections')" class="w-full flex items-center py-2.5 rounded-xl font-medium text-sm transition-all duration-200 active:scale-95"
          :class="[
            globalStore.activeTab === 'connections' ? 'bg-accent/10 text-accent font-bold' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed ? 'justify-center px-0' : 'px-3 gap-3'
          ]">
          <LinkOutline class="w-5 h-5 shrink-0" />
          <span class="transition-all duration-200 whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 w-0' : 'opacity-100 w-auto'">
            {{ t('nav.connections') }}
          </span>
        </button>

        <!-- 日志 -->
        <button @click="selectTab('logs')" class="w-full flex items-center py-2.5 rounded-xl font-medium text-sm transition-all duration-200 active:scale-95"
          :class="[
            globalStore.activeTab === 'logs' ? 'bg-accent/10 text-accent font-bold' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed ? 'justify-center px-0' : 'px-3 gap-3'
          ]">
          <DocumentTextOutline class="w-5 h-5 shrink-0" />
          <span class="transition-all duration-200 whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 w-0' : 'opacity-100 w-auto'">
            {{ t('nav.logs') }}
          </span>
        </button>

        <!-- 配置 -->
        <button @click="selectTab('config')" class="w-full flex items-center py-2.5 rounded-xl font-medium text-sm transition-all duration-200 active:scale-95"
          :class="[
            globalStore.activeTab === 'config' ? 'bg-accent/10 text-accent font-bold' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed ? 'justify-center px-0' : 'px-3 gap-3'
          ]">
          <SettingsOutline class="w-5 h-5 shrink-0" />
          <span class="transition-all duration-200 whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 w-0' : 'opacity-100 w-auto'">
            {{ t('nav.config') }}
          </span>
        </button>
      </nav>

      <!-- 底部操作：关于、中英文与主题 -->
      <div class="p-3 border-t border-slate-100 dark:border-slate-800/60 flex flex-col gap-2 transition-all duration-200">
        <!-- 语言与主题 -->
        <div class="flex gap-2 w-full transition-all duration-200"
          :class="globalStore.isSidebarCollapsed ? 'flex-col items-center' : 'flex-row'">
          <!-- 切换语言 -->
          <button @click="toggleLanguage" 
            class="flex-1 flex items-center justify-center py-2 px-3 text-xs font-semibold rounded-xl bg-slate-50/80 hover:bg-slate-100 dark:bg-slate-800/40 dark:hover:bg-slate-800/80 transition-all text-slate-600 dark:text-slate-300 hover:scale-105 active:scale-95 border border-slate-100/50 dark:border-slate-800/30 group"
            :class="globalStore.isSidebarCollapsed ? 'w-10 h-10 flex-none rounded-xl' : 'w-full'"
            :title="locale === 'zh' ? '切换语言' : 'Switch Language'">
            <LanguageOutline class="w-4 h-4 shrink-0 transition-transform duration-300 group-hover:rotate-12" />
            <span v-if="!globalStore.isSidebarCollapsed" class="ml-1.5 whitespace-nowrap overflow-hidden transition-all duration-200">
              {{ currentLangDisplay }}
            </span>
          </button>
          <!-- 切换主题 -->
          <button @click="switchThemeCycle" 
            class="flex-1 flex items-center justify-center py-2 px-3 text-xs font-semibold rounded-xl bg-slate-50/80 hover:bg-slate-100 dark:bg-slate-800/40 dark:hover:bg-slate-800/80 transition-all text-slate-600 dark:text-slate-300 hover:scale-105 active:scale-95 border border-slate-100/50 dark:border-slate-800/30 group"
            :class="globalStore.isSidebarCollapsed ? 'w-10 h-10 flex-none rounded-xl' : 'w-full'"
            aria-label="Toggle Theme"
            :title="t('config.theme')">
            <SunnyOutline v-if="globalStore.theme === 'light'" class="w-4 h-4 shrink-0 transition-all text-amber-500 group-hover:rotate-45 duration-300" />
            <MoonOutline v-else-if="globalStore.theme === 'dark'" class="w-4 h-4 shrink-0 transition-all text-indigo-400 group-hover:-rotate-12 duration-300" />
            <ColorPaletteOutline v-else-if="globalStore.theme === 'purple'" class="w-4 h-4 shrink-0 transition-all text-purple-500 dark:text-purple-400 group-hover:scale-110 duration-300" />
            <HeartOutline v-else-if="globalStore.theme === 'pink'" class="w-4 h-4 shrink-0 transition-all text-rose-500 group-hover:scale-110 duration-300" />
            <DesktopOutline v-else class="w-4 h-4 shrink-0 transition-all text-slate-500 dark:text-slate-400 group-hover:scale-110 duration-300" />
            <span v-if="!globalStore.isSidebarCollapsed" class="ml-1.5 whitespace-nowrap overflow-hidden transition-all duration-200">
              {{ t('config.theme_' + globalStore.theme) }}
            </span>
          </button>
        </div>

        <!-- 关于 Fluxor -->
        <button @click="showAbout = true" 
          class="flex items-center justify-center py-2 px-3 text-xs font-semibold rounded-xl bg-slate-50/80 hover:bg-slate-100 dark:bg-slate-800/40 dark:hover:bg-slate-800/80 transition-all text-slate-600 dark:text-slate-300 hover:scale-105 active:scale-95 border border-slate-100/50 dark:border-slate-800/30 group"
          :class="globalStore.isSidebarCollapsed ? 'w-10 h-10 flex-none rounded-xl' : 'w-full'"
          :title="t('about.title')">
          <InformationCircleOutline class="w-4 h-4 shrink-0 transition-transform duration-300 group-hover:scale-110" />
          <span v-if="!globalStore.isSidebarCollapsed" class="ml-1.5 whitespace-nowrap overflow-hidden transition-all duration-200">
            {{ t('about.title') }}
          </span>
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
          <button @click="toggleLanguage" 
            class="flex items-center justify-center px-3 py-1.5 text-xs font-semibold rounded-lg bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700/80 text-slate-600 dark:text-slate-300 transition-all active:scale-95">
            <span>{{ currentLangDisplay }}</span>
          </button>
          <!-- 主题 -->
          <button @click="switchThemeCycle" 
            class="p-1.5 rounded-lg bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700/80 text-slate-600 dark:text-slate-300 flex items-center justify-center transition-all active:scale-95 group" 
            aria-label="Toggle Theme">
            <SunnyOutline v-if="globalStore.theme === 'light'" class="w-4 h-4 text-amber-500 transition-all group-hover:rotate-45 duration-300" />
            <MoonOutline v-else-if="globalStore.theme === 'dark'" class="w-4 h-4 text-indigo-400 transition-all group-hover:-rotate-12 duration-300" />
            <ColorPaletteOutline v-else-if="globalStore.theme === 'purple'" class="w-4 h-4 text-purple-500 dark:text-purple-400 transition-all group-hover:scale-110 duration-300" />
            <HeartOutline v-else-if="globalStore.theme === 'pink'" class="w-4 h-4 text-rose-500 transition-all group-hover:scale-110 duration-300" />
            <DesktopOutline v-else class="w-4 h-4 text-slate-500 dark:text-slate-400 transition-all group-hover:scale-110 duration-300" />
          </button>
          <!-- 关于 -->
          <button @click="showAbout = true" 
            class="p-1.5 rounded-lg bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700/80 text-slate-600 dark:text-slate-300 flex items-center justify-center transition-all active:scale-95 group"
            :title="t('about.title')">
            <InformationCircleOutline class="w-4 h-4 transition-transform duration-300 group-hover:scale-110" />
          </button>
        </div>
      </header>

      <!-- 主工作区容器 -->
      <main class="flex-1 overflow-y-auto p-4 pb-20 md:pb-4 select-none">
        <div class="max-w-7xl mx-auto w-full">
          <component :is="activeComponent" />
        </div>
      </main>

      <!-- 移动端底部选项卡 Bar -->
      <nav class="md:hidden fixed bottom-0 inset-x-0 h-14 glass-heavy border-t flex items-center justify-around z-40 shadow-lg">
        <!-- 概览 -->
        <button @click="selectTab('overview')" class="flex flex-col items-center gap-0.5 transition-all duration-200 active:scale-95" :class="globalStore.activeTab === 'overview' ? 'text-accent font-semibold scale-105' : 'text-slate-500 dark:text-slate-400'">
          <GridOutline class="w-5 h-5" />
          <span class="text-[9px] font-medium">{{ t('nav.overview') }}</span>
        </button>
        <!-- 代理 -->
        <button @click="selectTab('proxies')" class="flex flex-col items-center gap-0.5 transition-all duration-200 active:scale-95" :class="globalStore.activeTab === 'proxies' ? 'text-accent font-semibold scale-105' : 'text-slate-500 dark:text-slate-400'">
          <GlobeOutline class="w-5 h-5" />
          <span class="text-[9px] font-medium">{{ t('nav.proxies') }}</span>
        </button>
        <!-- 订阅 -->
        <button @click="selectTab('subscription')" class="flex flex-col items-center gap-0.5 transition-all duration-200 active:scale-95" :class="globalStore.activeTab === 'subscription' ? 'text-accent font-semibold scale-105' : 'text-slate-500 dark:text-slate-400'">
          <MailOutline class="w-5 h-5" />
          <span class="text-[9px] font-medium">{{ t('nav.subscription') }}</span>
        </button>
        <!-- 规则 -->
        <button @click="selectTab('rules')" class="flex flex-col items-center gap-0.5 transition-all duration-200 active:scale-95" :class="globalStore.activeTab === 'rules' ? 'text-accent font-semibold scale-105' : 'text-slate-500 dark:text-slate-400'">
          <LayersOutline class="w-5 h-5" />
          <span class="text-[9px] font-medium">{{ t('nav.rules') }}</span>
        </button>
        <!-- 连接 -->
        <button @click="selectTab('connections')" class="flex flex-col items-center gap-0.5 transition-all duration-200 active:scale-95" :class="globalStore.activeTab === 'connections' ? 'text-accent font-semibold scale-105' : 'text-slate-500 dark:text-slate-400'">
          <LinkOutline class="w-5 h-5" />
          <span class="text-[9px] font-medium">{{ t('nav.connections') }}</span>
        </button>
        <!-- 日志 -->
        <button @click="selectTab('logs')" class="flex flex-col items-center gap-0.5 transition-all duration-200 active:scale-95" :class="globalStore.activeTab === 'logs' ? 'text-accent font-semibold scale-105' : 'text-slate-500 dark:text-slate-400'">
          <DocumentTextOutline class="w-5 h-5" />
          <span class="text-[9px] font-medium">{{ t('nav.logs') }}</span>
        </button>
        <!-- 配置 -->
        <button @click="selectTab('config')" class="flex flex-col items-center gap-0.5 transition-all duration-200 active:scale-95" :class="globalStore.activeTab === 'config' ? 'text-accent font-semibold scale-105' : 'text-slate-500 dark:text-slate-400'">
          <SettingsOutline class="w-5 h-5" />
          <span class="text-[9px] font-medium">{{ t('nav.config') }}</span>
        </button>
      </nav>
    </div>

    <!-- 关于 Fluxor 模态弹窗 -->
    <div v-if="showAbout" class="fixed inset-0 glass-mask z-[9999] flex items-center justify-center p-4" @click.self="showAbout = false">
      <div class="glass-heavy border w-full max-w-[92vw] sm:max-w-md rounded-[24px] shadow-2xl p-6 flex flex-col gap-6 animate-[zoomIn_0.15s_ease-out] relative overflow-hidden">
        <!-- 装饰渐变光晕背景 -->
        <div class="absolute -top-24 -right-24 w-48 h-48 bg-accent/10 rounded-full blur-3xl pointer-events-none"></div>
        <div class="absolute -bottom-24 -left-24 w-48 h-48 bg-purple-500/10 rounded-full blur-3xl pointer-events-none"></div>

        <!-- 头部信息 -->
        <div class="flex flex-col items-center text-center gap-2 pt-2">
          <h3 class="text-xl font-extrabold bg-gradient-to-r from-accent to-purple-600 dark:from-accent dark:to-purple-400 bg-clip-text text-transparent tracking-wide select-none">
            Fluxor
          </h3>
          <p class="text-xs text-slate-600 dark:text-slate-300/90 font-semibold px-4">
            {{ t('about.description') }}
          </p>
        </div>

        <!-- 详细信息面板 -->
        <div class="bg-slate-50/50 dark:bg-slate-950/30 border border-slate-100 dark:border-slate-800/50 rounded-2xl p-4 flex flex-col gap-3.5">
          <div class="flex items-center justify-between text-xs">
            <span class="font-bold text-slate-500 dark:text-slate-400">{{ t('about.version') }}</span>
            <span class="font-bold px-2 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-200">v{{ appVersion }}</span>
          </div>
          <!-- 内核版本 -->
          <div class="flex items-center justify-between text-xs">
            <span class="font-bold text-slate-500 dark:text-slate-400">{{ t('about.core_version') }}</span>
            <span class="font-bold px-2 py-0.5 rounded bg-accent/10 text-accent dark:text-accent/90">
              Mihomo {{ overviewStore.stats.coreVersion || 'Unknown' }}
            </span>
          </div>
          <!-- 开源仓库 -->
          <div class="flex items-center justify-between text-xs border-t border-slate-100 dark:border-slate-800/40 pt-3">
            <span class="font-bold text-slate-500 dark:text-slate-400">{{ t('about.github') }}</span>
            <a href="https://github.com/shuangji66/fluxor" target="_blank" class="flex items-center gap-1 text-slate-700 dark:text-slate-200 hover:text-accent dark:hover:text-accent font-bold transition-colors">
              <LogoGithub class="w-3.5 h-3.5" />
              <span>shuangji66/fluxor</span>
            </a>
          </div>
        </div>

        <!-- 技术特性 -->
        <div class="flex flex-col gap-2">
          <h4 class="text-xs font-extrabold text-slate-500 dark:text-slate-400 tracking-wider">
            {{ t('about.features') }}
          </h4>
          <ul class="flex flex-col gap-1.5 pl-1">
            <li class="text-[11px] font-semibold text-slate-600 dark:text-slate-300 flex items-start gap-1.5">
              <span class="w-1.5 h-1.5 rounded-full bg-accent mt-1 shrink-0"></span>
              <span>{{ t('about.feature_1') }}</span>
            </li>
            <li class="text-[11px] font-semibold text-slate-600 dark:text-slate-300 flex items-start gap-1.5">
              <span class="w-1.5 h-1.5 rounded-full bg-purple-500 mt-1 shrink-0"></span>
              <span>{{ t('about.feature_2') }}</span>
            </li>
            <li class="text-[11px] font-semibold text-slate-600 dark:text-slate-300 flex items-start gap-1.5">
              <span class="w-1.5 h-1.5 rounded-full bg-pink-500 mt-1 shrink-0"></span>
              <span>{{ t('about.feature_3') }}</span>
            </li>
          </ul>
        </div>

        <!-- 底部关闭按钮 -->
        <div class="flex justify-center pt-2">
          <button @click="showAbout = false" class="w-full py-2.5 text-xs font-semibold rounded-xl bg-white border border-slate-200 hover:bg-slate-50 dark:bg-slate-800 dark:border-slate-700 dark:hover:bg-slate-700/60 text-slate-700 dark:text-slate-300 transition-all duration-200 active:scale-95 shadow-sm">
            {{ t('common.close') }}
          </button>
        </div>
      </div>
    </div>

    <!-- 全局 Confirm 确认框 -->
    <div v-if="globalStore.confirmDialog && globalStore.confirmDialog.visible" class="fixed inset-0 glass-mask z-[9999] flex items-center justify-center p-4">
      <div class="glass-heavy w-full max-w-[92vw] sm:max-w-sm rounded-[20px] shadow-2xl border p-5 flex flex-col gap-4 animate-[zoomIn_0.15s_ease-out]">
        
        <div class="flex items-start gap-3.5">
          <!-- 状态图标 -->
          <div class="w-10 h-10 rounded-full flex items-center justify-center shrink-0 ring-4"
            :class="{
              'bg-red-500/10 text-red-500 ring-red-500/5 dark:bg-red-500/20': globalStore.confirmDialog.type === 'danger',
              'bg-amber-500/10 text-amber-500 ring-amber-500/5 dark:bg-amber-500/20': globalStore.confirmDialog.type === 'warning',
              'bg-emerald-500/10 text-emerald-500 ring-emerald-500/5 dark:bg-emerald-500/20': globalStore.confirmDialog.type === 'success',
              'bg-accent/10 text-accent ring-accent/5 dark:bg-accent/20': globalStore.confirmDialog.type === 'info'
            }">
            <AlertCircleOutline v-if="globalStore.confirmDialog.type === 'danger' || globalStore.confirmDialog.type === 'warning'" class="w-5.5 h-5.5" />
            <CheckmarkCircleOutline v-else-if="globalStore.confirmDialog.type === 'success'" class="w-5.5 h-5.5" />
            <InformationCircleOutline v-else class="w-5.5 h-5.5" />
          </div>

          <!-- 文字区域 -->
          <div class="flex-1 min-w-0">
            <h3 class="text-sm font-bold text-slate-800 dark:text-slate-100">
              {{ globalStore.confirmDialog.title || t('common.confirm') }}
            </h3>
            <p class="text-xs text-slate-500 dark:text-slate-400 mt-1 leading-relaxed break-words whitespace-pre-line">
              {{ globalStore.confirmDialog.message }}
            </p>
          </div>
        </div>

        <!-- 底部操作区 -->
        <div class="flex justify-end gap-2.5 pt-3 border-t border-slate-100 dark:border-slate-800/60">
          <button @click="globalStore.handleConfirmResult(false)" 
            class="px-4 py-2 text-xs font-semibold rounded-xl bg-white border border-slate-200/80 hover:bg-slate-50 dark:bg-slate-800 dark:border-slate-700 dark:hover:bg-slate-700/60 text-slate-600 dark:text-slate-300 transition-all duration-200 active:scale-95 select-none">
            {{ globalStore.confirmDialog.cancelText || t('common.cancel') }}
          </button>
          <button @click="globalStore.handleConfirmResult(true)" 
            class="px-4 py-2 text-xs font-semibold rounded-xl text-white transition-all duration-200 active:scale-95 shadow-sm select-none"
            :class="{
              'bg-red-500 hover:bg-red-600 shadow-red-500/10': globalStore.confirmDialog.type === 'danger',
              'bg-amber-500 hover:bg-amber-600 shadow-amber-500/10': globalStore.confirmDialog.type === 'warning',
              'bg-emerald-500 hover:bg-emerald-600 shadow-emerald-500/10': globalStore.confirmDialog.type === 'success',
              'bg-accent hover:bg-accent-hover shadow-accent/10': globalStore.confirmDialog.type === 'info'
            }">
            {{ globalStore.confirmDialog.okText || t('common.confirm') }}
          </button>
        </div>
      </div>
    </div>

    <!-- 全局 Toast 提示容器 -->
    <div class="fixed top-4 right-4 z-[9999] flex flex-col gap-2.5 pointer-events-none max-w-sm w-full px-4">
      <div v-for="toast in globalStore.toasts" :key="toast.id" 
        @click="globalStore.removeToast(toast.id)"
        class="p-4 rounded-2xl shadow-lg border text-xs font-semibold flex items-center justify-between gap-3 animate-[slideIn_0.25s_cubic-bezier(0.16,1,0.3,1)] pointer-events-auto backdrop-blur-lg cursor-pointer hover:translate-y-[-1px] active:scale-[0.98] transition-all duration-200"
        :class="{
          'bg-emerald-50/95 dark:bg-[#064e3b]/30 border-emerald-500/20 text-emerald-600 dark:text-emerald-400 shadow-emerald-500/5': toast.type === 'success',
          'bg-red-50/95 dark:bg-[#7f1d1d]/30 border-red-500/20 text-red-600 dark:text-red-400 shadow-red-500/5': toast.type === 'error',
          'bg-amber-50/95 dark:bg-[#78350f]/30 border-amber-500/20 text-amber-600 dark:text-amber-400 shadow-amber-500/5': toast.type === 'warning',
          'glass-heavy text-slate-700 dark:text-slate-300 shadow-slate-500/5': toast.type === 'info'
        }">
        <div class="flex items-center gap-2.5">
          <!-- 状态图标 -->
          <CheckmarkCircleOutline v-if="toast.type === 'success'" class="w-4 h-4 shrink-0" />
          <CloseCircleOutline v-else-if="toast.type === 'error'" class="w-4 h-4 shrink-0" />
          <AlertCircleOutline v-else-if="toast.type === 'warning'" class="w-4 h-4 shrink-0" />
          <InformationCircleOutline v-else class="w-4 h-4 shrink-0" />
          
          <span class="leading-normal">{{ toast.text }}</span>
        </div>
        <CloseOutline class="w-3.5 h-3.5 shrink-0 opacity-40 hover:opacity-100 transition-opacity" />
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
