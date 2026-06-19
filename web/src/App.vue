<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useGlobalStore } from './store/global'
import { useConfigStore } from './store/config'
import { 
  ChevronBackOutline, 
  ChevronForwardOutline, 
  SpeedometerOutline, 
  GlobeOutline, 
  MailOutline, 
  ListOutline, 
  SwapHorizontalOutline, 
  TerminalOutline, 
  SettingsOutline, 
  MoonOutline, 
  SunnyOutline, 
  ContrastOutline, 
  MenuOutline 
} from '@vicons/ionicons5'

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

onMounted(() => {
  initTheme()
  document.title = 'Fluxor - ' + t('nav.' + globalStore.activeTab)
  
  // 预加载配置与订阅状态
  configStore.fetchCoreStatus()
  configStore.fetchConfigs()
  configStore.loadConfig()
})

onUnmounted(() => {
  if (systemThemeListener) {
    window.matchMedia('(prefers-color-scheme: dark)').removeEventListener('change', systemThemeListener)
  }
})
</script>

<template>
  <div class="flex h-screen w-screen overflow-hidden bg-[#f1f5f9] dark:bg-[#0f172a] transition-colors duration-200">
    
    <!-- 移动端侧边栏遮罩 -->
    <div v-if="!globalStore.isSidebarCollapsed" @click="globalStore.isSidebarCollapsed = true" class="fixed inset-0 bg-black/40 backdrop-blur-sm z-40 md:hidden"></div>

    <!-- 侧边栏 aside -->
    <aside class="fixed md:static inset-y-0 left-0 bg-white dark:bg-[#1e293b] border-r border-slate-200 dark:border-slate-800/80 z-50 flex flex-col justify-between transition-all duration-200 overflow-y-auto overflow-x-hidden md:translate-x-0"
      :class="[
        globalStore.isSidebarCollapsed ? '-translate-x-full w-0 md:w-16 md:translate-x-0' : 'translate-x-0 w-60'
      ]">
      <div class="p-4 flex items-center justify-between border-b border-slate-100 dark:border-slate-800/60">
        <span v-if="!globalStore.isSidebarCollapsed" class="font-bold text-accent tracking-wider text-base select-none">FLUXOR</span>
        <button @click="toggleSidebar" class="p-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 rounded transition-all">
          <ChevronBackOutline v-if="!globalStore.isSidebarCollapsed" class="w-5 h-5" />
          <ChevronForwardOutline v-else class="w-5 h-5" />
        </button>
      </div>

      <!-- 导航项目 -->
      <nav class="flex-1 px-3 py-4 space-y-1">
        <!-- 概览 -->
        <button @click="globalStore.activeTab = 'overview'" class="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl font-medium text-sm transition-all"
          :class="globalStore.activeTab === 'overview' ? 'bg-accent/10 text-accent' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'">
          <SpeedometerOutline class="w-5 h-5 shrink-0" />
          <span v-if="!globalStore.isSidebarCollapsed" class="transition-opacity">{{ t('nav.overview') }}</span>
        </button>

        <!-- 代理 -->
        <button @click="globalStore.activeTab = 'proxies'" class="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl font-medium text-sm transition-all"
          :class="globalStore.activeTab === 'proxies' ? 'bg-accent/10 text-accent' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'">
          <GlobeOutline class="w-5 h-5 shrink-0" />
          <span v-if="!globalStore.isSidebarCollapsed">{{ t('nav.proxies') }}</span>
        </button>

        <!-- 订阅 -->
        <button @click="globalStore.activeTab = 'subscription'" class="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl font-medium text-sm transition-all"
          :class="globalStore.activeTab === 'subscription' ? 'bg-accent/10 text-accent' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'">
          <MailOutline class="w-5 h-5 shrink-0" />
          <span v-if="!globalStore.isSidebarCollapsed">{{ t('nav.subscription') }}</span>
        </button>

        <!-- 规则 -->
        <button @click="globalStore.activeTab = 'rules'" class="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl font-medium text-sm transition-all"
          :class="globalStore.activeTab === 'rules' ? 'bg-accent/10 text-accent' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'">
          <ListOutline class="w-5 h-5 shrink-0" />
          <span v-if="!globalStore.isSidebarCollapsed">{{ t('nav.rules') }}</span>
        </button>

        <!-- 连接 -->
        <button @click="globalStore.activeTab = 'connections'" class="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl font-medium text-sm transition-all"
          :class="globalStore.activeTab === 'connections' ? 'bg-accent/10 text-accent' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'">
          <SwapHorizontalOutline class="w-5 h-5 shrink-0" />
          <span v-if="!globalStore.isSidebarCollapsed">{{ t('nav.connections') }}</span>
        </button>

        <!-- 日志 -->
        <button @click="globalStore.activeTab = 'logs'" class="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl font-medium text-sm transition-all"
          :class="globalStore.activeTab === 'logs' ? 'bg-accent/10 text-accent' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'">
          <TerminalOutline class="w-5 h-5 shrink-0" />
          <span v-if="!globalStore.isSidebarCollapsed">{{ t('nav.logs') }}</span>
        </button>

        <!-- 配置 -->
        <button @click="globalStore.activeTab = 'config'" class="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl font-medium text-sm transition-all"
          :class="globalStore.activeTab === 'config' ? 'bg-accent/10 text-accent' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'">
          <SettingsOutline class="w-5 h-5 shrink-0" />
          <span v-if="!globalStore.isSidebarCollapsed">{{ t('nav.config') }}</span>
        </button>
      </nav>

      <!-- 底部操作：中英文与亮暗主题 -->
      <div class="p-3 border-t border-slate-100 dark:border-slate-800/60 flex flex-col gap-2">
        <!-- 切换语言 -->
        <button @click="toggleLanguage" class="w-full flex items-center justify-center py-2 text-xs font-semibold rounded-lg bg-slate-50 hover:bg-slate-100 dark:bg-slate-800/40 dark:hover:bg-slate-800 transition-all text-slate-500 dark:text-slate-400">
          <span>{{ currentLangDisplay }}</span>
        </button>
        <!-- 切换主题 -->
        <button @click="switchThemeCycle" class="w-full flex items-center justify-center py-2 text-xs font-semibold rounded-lg bg-slate-50 hover:bg-slate-100 dark:bg-slate-800/40 dark:hover:bg-slate-800 transition-all text-slate-500 dark:text-slate-400">
          <MoonOutline v-if="globalStore.theme === 'dark'" class="w-5 h-5" />
          <SunnyOutline v-else-if="globalStore.theme === 'light'" class="w-5 h-5" />
          <ContrastOutline v-else class="w-5 h-5" />
        </button>
      </div>
    </aside>

    <!-- 移动端顶部标题栏 -->
    <div class="flex-1 flex flex-col min-w-0">
      <header class="md:hidden flex h-14 bg-white dark:bg-[#1e293b] border-b border-slate-200 dark:border-slate-800/80 px-4 justify-between items-center z-30 shadow-sm shrink-0">
        <div class="flex items-center gap-3">
          <!-- 汉堡按钮 -->
          <button @click="globalStore.isSidebarCollapsed = false" class="p-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 flex items-center justify-center">
            <MenuOutline class="w-6 h-6" />
          </button>
          <span class="font-bold text-slate-800 dark:text-slate-100 text-sm select-none">Fluxor</span>
        </div>

        <!-- 移动端快捷设置 -->
        <div class="flex gap-2">
          <!-- 语言 -->
          <button @click="toggleLanguage" class="p-1.5 text-xs font-semibold rounded bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400">
            {{ currentLangDisplay }}
          </button>
          <!-- 主题 -->
          <button @click="switchThemeCycle" class="p-1.5 text-xs rounded bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400">
            <span class="text-[10px] font-bold tracking-wider uppercase">{{ globalStore.theme }}</span>
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
        <button @click="globalStore.activeTab = 'overview'" class="flex flex-col items-center gap-0.5 text-slate-500" :class="{ 'text-accent': globalStore.activeTab === 'overview' }">
          <SpeedometerOutline class="w-5 h-5" />
          <span class="text-[9px] font-medium">{{ t('nav.overview') }}</span>
        </button>
        <!-- 代理 -->
        <button @click="globalStore.activeTab = 'proxies'" class="flex flex-col items-center gap-0.5 text-slate-500" :class="{ 'text-accent': globalStore.activeTab === 'proxies' }">
          <GlobeOutline class="w-5 h-5" />
          <span class="text-[9px] font-medium">{{ t('nav.proxies') }}</span>
        </button>
        <!-- 订阅 -->
        <button @click="globalStore.activeTab = 'subscription'" class="flex flex-col items-center gap-0.5 text-slate-500" :class="{ 'text-accent': globalStore.activeTab === 'subscription' }">
          <MailOutline class="w-5 h-5" />
          <span class="text-[9px] font-medium">{{ t('nav.subscription') }}</span>
        </button>
        <!-- 规则 -->
        <button @click="globalStore.activeTab = 'rules'" class="flex flex-col items-center gap-0.5 text-slate-500" :class="{ 'text-accent': globalStore.activeTab === 'rules' }">
          <ListOutline class="w-5 h-5" />
          <span class="text-[9px] font-medium">{{ t('nav.rules') }}</span>
        </button>
        <!-- 连接 -->
        <button @click="globalStore.activeTab = 'connections'" class="flex flex-col items-center gap-0.5 text-slate-500" :class="{ 'text-accent': globalStore.activeTab === 'connections' }">
          <SwapHorizontalOutline class="w-5 h-5" />
          <span class="text-[9px] font-medium">{{ t('nav.connections') }}</span>
        </button>
        <!-- 日志 -->
        <button @click="globalStore.activeTab = 'logs'" class="flex flex-col items-center gap-0.5 text-slate-500" :class="{ 'text-accent': globalStore.activeTab === 'logs' }">
          <TerminalOutline class="w-5 h-5" />
          <span class="text-[9px] font-medium">{{ t('nav.logs') }}</span>
        </button>
        <!-- 配置 -->
        <button @click="globalStore.activeTab = 'config'" class="flex flex-col items-center gap-0.5 text-slate-500" :class="{ 'text-accent': globalStore.activeTab === 'config' }">
          <SettingsOutline class="w-5 h-5" />
          <span class="text-[9px] font-medium">{{ t('nav.config') }}</span>
        </button>
      </nav>
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
</style>
