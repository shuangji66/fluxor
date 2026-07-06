<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useGlobalStore } from './store/global'
import { useConfigStore } from './store/config'
import { useOverviewStore } from './store/overview'
import { useSubscriptionStore } from './store/subscription'
import { apiFetch } from './utils/api'
import {
  LanguageOutline,
  SunnyOutline,
  MoonOutline,
  ColorPaletteOutline,
  HeartOutline,
  ContrastOutline,
  ChevronBackOutline,
  ChevronForwardOutline,
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
  CloseOutline,
  ApertureOutline
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

import { useTheme } from './composables/useTheme'
import { useLanguage } from './composables/useLanguage'

const { t } = useI18n()
const globalStore = useGlobalStore()
const configStore = useConfigStore()
const overviewStore = useOverviewStore()
const subscriptionStore = useSubscriptionStore()

const { initTheme, switchThemeCycle } = useTheme()
const { locale, currentLangDisplay, toggleLanguage, updateTitle } = useLanguage()

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

// 通用主题与语言管理逻辑已解耦至 composables/useTheme 与 composables/useLanguage

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

// 检查元素或其祖先是否允许选中/复制
const isAllowedElement = (target: HTMLElement | null): boolean => {
  if (!target) return false
  let curr: HTMLElement | null = target
  while (curr) {
    const tag = curr.tagName ? curr.tagName.toLowerCase() : ''
    if (tag === 'input' || tag === 'textarea' || tag === 'select' || tag === 'option') {
      return true
    }
    if (curr.classList && curr.classList.contains('select-text')) {
      return true
    }
    curr = curr.parentElement
  }
  return false
}

// 拦截全局选择事件以防光标与选区
const handleSelectStart = (e: Event) => {
  const target = e.target as HTMLElement
  if (!isAllowedElement(target)) {
    e.preventDefault()
  }
}

// 拦截全局复制事件以防非授权复制
const handleCopy = (e: ClipboardEvent) => {
  const selection = window.getSelection()
  if (selection && selection.rangeCount > 0) {
    const range = selection.getRangeAt(0)
    const commonAncestor = range.commonAncestorContainer
    const container = commonAncestor.nodeType === Node.TEXT_NODE ? commonAncestor.parentElement : commonAncestor as HTMLElement
    if (!isAllowedElement(container)) {
      e.preventDefault()
    }
  } else {
    const target = e.target as HTMLElement
    if (!isAllowedElement(target)) {
      e.preventDefault()
    }
  }
}

// self更新
const isUpdatingSelf = ref(false)

const handleSelfUpdate = async () => {
  if (isUpdatingSelf.value) return
  isUpdatingSelf.value = true
  try {
    const resp = await apiFetch('/update-self', { method: 'POST' })
    if (resp.ok) {
      globalStore.showToast(t('update.update_started'), 'success')
      // 延迟后刷新页面，因为进程即将重启
      setTimeout(() => {
        window.location.reload()
      }, 2000)
    } else {
      let msg = t('update.update_failed')
      try {
        const data = await resp.json()
        if (data.message) msg = data.message
      } catch (_) {}
      globalStore.showToast(msg, 'error')
    }
  } catch (e) {
    globalStore.showToast(t('update.update_failed') + ': ' + (e as Error).message, 'error')
  } finally {
    isUpdatingSelf.value = false
  }
}

onMounted(() => {
  initTheme()
  updateTitle(globalStore.activeTab)
  
  // 初始化检测：若直接在手机小屏载入，默认收起
  if (window.innerWidth < 768) {
    globalStore.isSidebarCollapsed = true
  }
  
  window.addEventListener('resize', handleResize)
  document.addEventListener('selectstart', handleSelectStart)
  document.addEventListener('copy', handleCopy as EventListener)

  // 订阅内核状态（启动轮询）
  configStore.subscribeCoreStatus()
  
  // 预加载配置与订阅状态
  configStore.fetchConfigs()
  subscriptionStore.loadConfig()  // 改用 subscriptionStore（如果 App.vue 中已导入）
  
  // 统一获取版本号（应用启动时立即执行）
  overviewStore.fetchVersionAndStatus()

  // 获取当前用户信息并显示欢迎
  apiFetch('/whoami')
    .then(res => res.ok ? res.json() : null)
    .then(data => {
      if (data && data.username) {
        globalStore.showToast(t('welcome_back', { username: data.username }), 'success')
      }
    })
    .catch(() => {}) // 静默失败，不影响正常使用

  // 检查 Fluxor 自身更新
  apiFetch(`/check-update?current=${appVersion}`)
    .then(res => res.ok ? res.json() : null)
    .then(data => {
      if (data) {
        globalStore.updateInfo = data  // 存储完整信息
        if (data.hasUpdate) {
          globalStore.showToast(
            t('update.available_toast', { latest: data.latest }),
            'info'
          )
        }
      }
    })
    .catch(() => {})
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  document.removeEventListener('selectstart', handleSelectStart)
  document.removeEventListener('copy', handleCopy as EventListener)
  configStore.unsubscribeCoreStatus()
})
</script>

<template>
  <div class="flex h-screen w-screen overflow-hidden bg-[#f1f5f9] dark:bg-[#0f172a] transition-colors duration-200 relative">
    
    <!-- 侧边栏 aside -->
    <aside class="hidden md:flex md:static my-4 ml-4 mr-0 h-[calc(100vh-32px)] glass-medium border border-slate-200/60 dark:border-slate-800/60 rounded-[24px] z-50 flex-col justify-between transition-all duration-300 overflow-y-auto overflow-x-hidden shadow-md"
      :class="[
        globalStore.isSidebarCollapsed ? 'md:w-16 sidebar-collapsed' : 'md:w-60'
      ]">
      <div class="flex items-center p-4 border-b border-slate-100 dark:border-slate-800/60 w-full transition-all duration-300 justify-between">
        <!-- Logo + Title 组合区域 -->
        <div class="flex items-center select-none cursor-pointer group/logo"
             @click="toggleSidebar"
             :title="globalStore.isSidebarCollapsed ? '点击展开侧边栏' : '点击折叠侧边栏'">
          <!-- 应用 Logo 图标 -->
          <div class="w-8 h-8 flex items-center justify-center shrink-0 transition-transform duration-500 text-accent"
               :class="globalStore.isSidebarCollapsed ? 'rotate-180' : 'rotate-0'">
            <ApertureOutline class="w-5.5 h-5.5" />
          </div>
          <!-- 标题，随折叠平滑收缩 -->
          <span class="font-bold text-sm text-slate-700 dark:text-slate-200 tracking-wider transition-all duration-300 ease-in-out whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 max-w-0 ml-0' : 'opacity-100 max-w-32 ml-2.5'">
            Fluxor
          </span>
        </div>
        <!-- 折叠/展开按钮 -->
        <button v-if="!globalStore.isSidebarCollapsed" @click="toggleSidebar" 
          class="p-1.5 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all flex items-center justify-center animate-[fadeIn_0.2s]" aria-label="Toggle Sidebar">
          <ChevronBackOutline class="w-4 h-4 transition-all duration-200 hover:scale-110" />
        </button>
      </div>

      <!-- 导航项目 -->
      <nav class="flex-1 px-3 py-4 space-y-1 transition-all duration-300">
        <!-- 概览 -->
        <button @click="selectTab('overview')" 
          class="w-full flex items-center rounded-xl font-medium text-sm transition-all duration-300 active:scale-95 group relative justify-start"
          :class="[
            globalStore.activeTab === 'overview' ? 'sidebar-active font-bold' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed 
              ? 'px-2.5 py-2 hover:scale-105' 
              : 'px-3.5 py-2.5 hover:translate-x-1'
          ]"
          :title="globalStore.isSidebarCollapsed ? t('nav.overview') : ''">
          <GridOutline class="w-5 h-5 shrink-0" />
          <span class="transition-all duration-300 ease-in-out whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 max-w-0 ml-0' : 'opacity-100 max-w-32 ml-3'">
            {{ t('nav.overview') }}
          </span>
        </button>

        <!-- 代理 -->
        <button @click="selectTab('proxies')" 
          class="w-full flex items-center rounded-xl font-medium text-sm transition-all duration-300 active:scale-95 group relative justify-start"
          :class="[
            globalStore.activeTab === 'proxies' ? 'sidebar-active font-bold' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed 
              ? 'px-2.5 py-2 hover:scale-105' 
              : 'px-3.5 py-2.5 hover:translate-x-1'
          ]"
          :title="globalStore.isSidebarCollapsed ? t('nav.proxies') : ''">
          <GlobeOutline class="w-5 h-5 shrink-0" />
          <span class="transition-all duration-300 ease-in-out whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 max-w-0 ml-0' : 'opacity-100 max-w-32 ml-3'">
            {{ t('nav.proxies') }}
          </span>
        </button>

        <!-- 订阅 -->
        <button @click="selectTab('subscription')" 
          class="w-full flex items-center rounded-xl font-medium text-sm transition-all duration-300 active:scale-95 group relative justify-start"
          :class="[
            globalStore.activeTab === 'subscription' ? 'sidebar-active font-bold' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed 
              ? 'px-2.5 py-2 hover:scale-105' 
              : 'px-3.5 py-2.5 hover:translate-x-1'
          ]"
          :title="globalStore.isSidebarCollapsed ? t('nav.subscription') : ''">
          <MailOutline class="w-5 h-5 shrink-0" />
          <span class="transition-all duration-300 ease-in-out whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 max-w-0 ml-0' : 'opacity-100 max-w-32 ml-3'">
            {{ t('nav.subscription') }}
          </span>
        </button>

        <!-- 规则 -->
        <button @click="selectTab('rules')" 
          class="w-full flex items-center rounded-xl font-medium text-sm transition-all duration-300 active:scale-95 group relative justify-start"
          :class="[
            globalStore.activeTab === 'rules' ? 'sidebar-active font-bold' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed 
              ? 'px-2.5 py-2 hover:scale-105' 
              : 'px-3.5 py-2.5 hover:translate-x-1'
          ]"
          :title="globalStore.isSidebarCollapsed ? t('nav.rules') : ''">
          <LayersOutline class="w-5 h-5 shrink-0" />
          <span class="transition-all duration-300 ease-in-out whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 max-w-0 ml-0' : 'opacity-100 max-w-32 ml-3'">
            {{ t('nav.rules') }}
          </span>
        </button>

        <!-- 连接 -->
        <button @click="selectTab('connections')" 
          class="w-full flex items-center rounded-xl font-medium text-sm transition-all duration-300 active:scale-95 group relative justify-start"
          :class="[
            globalStore.activeTab === 'connections' ? 'sidebar-active font-bold' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed 
              ? 'px-2.5 py-2 hover:scale-105' 
              : 'px-3.5 py-2.5 hover:translate-x-1'
          ]"
          :title="globalStore.isSidebarCollapsed ? t('nav.connections') : ''">
          <LinkOutline class="w-5 h-5 shrink-0" />
          <span class="transition-all duration-300 ease-in-out whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 max-w-0 ml-0' : 'opacity-100 max-w-32 ml-3'">
            {{ t('nav.connections') }}
          </span>
        </button>

        <!-- 日志 -->
        <button @click="selectTab('logs')" 
          class="w-full flex items-center rounded-xl font-medium text-sm transition-all duration-300 active:scale-95 group relative justify-start"
          :class="[
            globalStore.activeTab === 'logs' ? 'sidebar-active font-bold' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed 
              ? 'px-2.5 py-2 hover:scale-105' 
              : 'px-3.5 py-2.5 hover:translate-x-1'
          ]"
          :title="globalStore.isSidebarCollapsed ? t('nav.logs') : ''">
          <DocumentTextOutline class="w-5 h-5 shrink-0" />
          <span class="transition-all duration-300 ease-in-out whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 max-w-0 ml-0' : 'opacity-100 max-w-32 ml-3'">
            {{ t('nav.logs') }}
          </span>
        </button>

        <!-- 配置 -->
        <button @click="selectTab('config')" 
          class="w-full flex items-center rounded-xl font-medium text-sm transition-all duration-300 active:scale-95 group relative justify-start"
          :class="[
            globalStore.activeTab === 'config' ? 'sidebar-active font-bold' : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
            globalStore.isSidebarCollapsed 
              ? 'px-2.5 py-2 hover:scale-105' 
              : 'px-3.5 py-2.5 hover:translate-x-1'
          ]"
          :title="globalStore.isSidebarCollapsed ? t('nav.config') : ''">
          <SettingsOutline class="w-5 h-5 shrink-0" />
          <span class="transition-all duration-300 ease-in-out whitespace-nowrap overflow-hidden"
            :class="globalStore.isSidebarCollapsed ? 'opacity-0 max-w-0 ml-0' : 'opacity-100 max-w-32 ml-3'">
            {{ t('nav.config') }}
          </span>
        </button>
      </nav>

      <!-- 底部操作：中英文与主题 -->
      <div class="border-t border-slate-100 dark:border-slate-800/60 p-3 transition-all duration-300">
        <!-- 底部聚合容器卡片，在展开时提供整合的质感，折叠时平滑淡化过渡 -->
        <div class="flex flex-col transition-all duration-300"
          :class="[
            globalStore.isSidebarCollapsed 
              ? 'items-center gap-1.5' 
              : 'gap-2'
          ]">
          <!-- 语言与主题：展开时左右并排，折叠时上下排布 -->
          <div class="flex w-full transition-all duration-300"
            :class="globalStore.isSidebarCollapsed ? 'flex-col items-center gap-1.5' : 'flex-row gap-2'">
            <!-- 切换语言 -->
            <button @click="toggleLanguage" 
              class="flex items-center justify-center text-xs font-semibold rounded-xl bg-slate-50 hover:bg-slate-100 dark:bg-slate-800/40 dark:hover:bg-slate-800/80 transition-all text-slate-600 dark:text-slate-300 active:scale-95 border border-slate-100/50 dark:border-slate-800/30 group overflow-hidden shrink-0"
              :class="[
                globalStore.isSidebarCollapsed ? 'w-9 h-9 flex-none hover:scale-105 py-0 px-0' : 'flex-1 py-2 px-2.5 hover:scale-[1.02]'
              ]"
              :title="locale === 'zh' ? '切换语言' : 'Switch Language'">
              <LanguageOutline class="w-4 h-4 shrink-0 sidebar-bottom-icon group-hover:scale-110 group-hover:rotate-12" />
              <span class="transition-all duration-300 ease-in-out whitespace-nowrap overflow-hidden"
                :class="globalStore.isSidebarCollapsed ? 'opacity-0 max-w-0 ml-0' : 'opacity-100 max-w-20 ml-1.5'">
                {{ currentLangDisplay }}
              </span>
            </button>
            
            <!-- 切换主题 -->
            <button @click="switchThemeCycle" 
              class="flex items-center justify-center text-xs font-semibold rounded-xl bg-slate-50 hover:bg-slate-100 dark:bg-slate-800/40 dark:hover:bg-slate-800/80 transition-all text-slate-600 dark:text-slate-300 active:scale-95 border border-slate-100/50 dark:border-slate-800/30 group overflow-hidden shrink-0"
              :class="[
                globalStore.isSidebarCollapsed ? 'w-9 h-9 flex-none hover:scale-105 py-0 px-0' : 'flex-1 py-2 px-2.5 hover:scale-[1.02]'
              ]"
              aria-label="Toggle Theme"
              :title="t('config.theme') + ': ' + t('config.theme_' + globalStore.theme)">
              <SunnyOutline v-if="globalStore.theme === 'light'" class="w-4 h-4 shrink-0 sidebar-bottom-icon text-amber-500 group-hover:scale-110 group-hover:rotate-45" />
              <MoonOutline v-else-if="globalStore.theme === 'dark'" class="w-4 h-4 shrink-0 sidebar-bottom-icon text-indigo-400 group-hover:scale-110 group-hover:-rotate-12" />
              <ColorPaletteOutline v-else-if="globalStore.theme === 'purple'" class="w-4 h-4 shrink-0 sidebar-bottom-icon text-purple-500 dark:text-purple-400 group-hover:scale-110 group-hover:-rotate-12" />
              <HeartOutline v-else-if="globalStore.theme === 'pink'" class="w-4 h-4 shrink-0 sidebar-bottom-icon text-rose-500 group-hover:scale-110 group-hover:-rotate-12" />
              <ContrastOutline v-else class="w-4 h-4 shrink-0 sidebar-bottom-icon text-slate-500 dark:text-slate-400 group-hover:scale-110 group-hover:-rotate-12" />
              <span class="transition-all duration-300 ease-in-out whitespace-nowrap overflow-hidden"
                :class="globalStore.isSidebarCollapsed ? 'opacity-0 max-w-0 ml-0' : 'opacity-100 max-w-20 ml-1.5'">
                {{ t('config.theme_' + globalStore.theme) }}
              </span>
            </button>
          </div>
        </div>
      </div>
    </aside>

    <!-- 移动端主工作区容器 -->
    <div class="flex-1 flex flex-col min-w-0">
      <main class="flex-1 my-3 mx-3 md:my-4 md:mx-4 h-[calc(100vh-24px)] md:h-[calc(100vh-32px)] flex flex-col min-h-0 select-none overflow-hidden pb-14 md:pb-0">
        <div class="max-w-7xl mx-auto w-full flex flex-col flex-1 min-h-0">
          <KeepAlive :max="6">
            <component :is="activeComponent" class="flex flex-col flex-1 min-h-0" />
          </KeepAlive>
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
    <Teleport to="body">
      <div v-if="globalStore.showAbout" class="fixed inset-0 glass-mask z-[9999] flex items-center justify-center p-4" @click.self="globalStore.showAbout = false">
        <div class="glass-heavy border w-full max-w-[92vw] sm:max-w-md rounded-[24px] shadow-2xl p-6 flex flex-col gap-6 animate-[zoomIn_0.15s_ease-out] relative overflow-hidden">

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
            <!-- 面板版本行（左侧标签，右侧按钮+版本号） -->
            <div class="flex items-center justify-between text-xs">
              <span class="font-bold text-slate-500 dark:text-slate-400">{{ t('about.version') }}</span>
              <div class="flex items-center gap-2">
                <!-- 更新按钮（有更新时显示） -->
                <button
                  v-if="globalStore.updateInfo?.hasUpdate"
                  @click="handleSelfUpdate"
                  :disabled="isUpdatingSelf"
                  class="px-2 py-0.5 text-xs font-semibold rounded-lg bg-accent hover:bg-accent-hover text-white transition-all disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {{ isUpdatingSelf ? t('update.updating') : t('update.update_button', { latest: globalStore.updateInfo.latest }) }}
                </button>
                <!-- 版本号 -->
                <span class="font-bold px-2 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-200">v{{ appVersion }}</span>
              </div>
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
                <span>Github</span>
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
            <button @click="globalStore.showAbout = false" class="w-full py-2.5 text-xs font-semibold rounded-xl bg-white border border-slate-200 hover:bg-slate-50 dark:bg-slate-800 dark:border-slate-700 dark:hover:bg-slate-700/60 text-slate-700 dark:text-slate-300 transition-all duration-200 active:scale-95 shadow-sm">
              {{ t('common.close') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- 全局 Confirm 确认框 -->
    <Teleport to="body">
      <div v-if="globalStore.confirmDialog && globalStore.confirmDialog.visible" class="fixed inset-0 glass-mask z-[10000] flex items-center justify-center p-4">
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

          <!-- 自定义复选框：左侧复选框，右侧文本联动（左侧 pl-[54px] 与上方标题文字左对齐） -->
          <div v-if="globalStore.confirmDialog.checkboxLabel" class="flex items-center gap-2 select-none pl-[54px] pr-1 py-0.5">
            <label class="relative flex items-center cursor-pointer gap-2">
              <input type="checkbox" v-model="globalStore.confirmDialog.checkboxChecked" class="sr-only" />
              <!-- 复选框图标 -->
              <div class="w-4 h-4 rounded border transition-all duration-200 flex items-center justify-center shadow-sm shrink-0"
                :class="globalStore.confirmDialog.checkboxChecked 
                  ? 'bg-accent border-accent text-white' 
                  : 'bg-slate-50 dark:bg-slate-800/60 border-slate-200 dark:border-slate-700 text-transparent'">
                <svg class="w-2.5 h-2.5 transform transition-all duration-200" 
                  :class="globalStore.confirmDialog.checkboxChecked ? 'scale-100 opacity-100' : 'scale-50 opacity-0'"
                  fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="4">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                </svg>
              </div>
              <!-- 联动文本 -->
              <span class="text-xs font-semibold text-slate-600 dark:text-slate-400 select-none">
                {{ globalStore.confirmDialog.checkboxLabel }}
              </span>
            </label>
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
    </Teleport>

    <!-- 全局 Toast 提示容器 -->
    <Teleport to="body">
      <div class="fixed top-4 right-4 z-[10000] inline-flex flex-col items-end gap-2.5 pointer-events-none max-w-[90%] md:max-w-sm">
        <div v-for="toast in globalStore.toasts" :key="toast.id" 
          @click="globalStore.removeToast(toast.id)"
          class="p-4 rounded-2xl shadow-lg border text-xs font-semibold flex items-center justify-between gap-3 animate-[slideIn_0.25s_cubic-bezier(0.16,1,0.3,1)] pointer-events-auto backdrop-blur-lg cursor-pointer hover:translate-y-[-1px] active:scale-[0.98] transition-all duration-200 w-full"
          :class="{
            'bg-emerald-50/95 dark:bg-[#064e3b]/30 border-emerald-500/20 text-emerald-600 dark:text-emerald-400 shadow-emerald-500/5': toast.type === 'success',
            'bg-red-50/95 dark:bg-[#7f1d1d]/30 border-red-500/20 text-red-600 dark:text-red-400 shadow-red-500/5': toast.type === 'error',
            'bg-amber-50/95 dark:bg-[#78350f]/30 border-amber-500/20 text-amber-600 dark:text-amber-400 shadow-amber-500/5': toast.type === 'warning',
            'glass-heavy text-slate-700 dark:text-slate-300 shadow-slate-500/5': toast.type === 'info'
          }">
          <div class="flex items-center gap-2.5">
            <CheckmarkCircleOutline v-if="toast.type === 'success'" class="w-4 h-4 shrink-0" />
            <CloseCircleOutline v-else-if="toast.type === 'error'" class="w-4 h-4 shrink-0" />
            <AlertCircleOutline v-else-if="toast.type === 'warning'" class="w-4 h-4 shrink-0" />
            <InformationCircleOutline v-else class="w-4 h-4 shrink-0" />
            <span class="leading-normal">{{ toast.text }}</span>
          </div>
          <CloseOutline class="w-3.5 h-3.5 shrink-0 opacity-40 hover:opacity-100 transition-opacity" />
        </div>
      </div>
    </Teleport>
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
