<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ColorPaletteOutline } from '@vicons/ionicons5'
import { useGlobalStore } from '../../store/global'
import { useLanguage } from '../../composables/useLanguage'

/**
 * 界面设置卡（语言 / 主题 / 启动页）。
 *
 * 纯前端偏好，不触碰内核，也不参与「保存并应用」——三项都是即时生效并各自落
 * localStorage（主题与启动页的持久化由 globalStore 的 watch 负责，语言走 useLanguage）。
 * 从配置页迁出后，配置页少一段与其余卡片无关的表单。
 */
const { t } = useI18n()
const globalStore = useGlobalStore()
const { locale, setLanguage } = useLanguage()
</script>

<template>
  <div
    class="live-card bg-slate-50/50 dark:bg-slate-900/30 p-6 rounded-xl border border-slate-200/40 dark:border-slate-800/40 hover:border-slate-300/80 dark:hover:border-slate-700/80 hover:-translate-y-[3px] hover:shadow-md hover:bg-slate-100/80 dark:hover:bg-slate-900/80 duration-300 space-y-5 h-full transition-all flex flex-col relative"
  >
    <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
      <ColorPaletteOutline class="w-4 h-4 text-accent" />
      {{ t('config.interface_settings') }}
    </h4>

    <div class="space-y-4 flex-1">
      <div class="flex flex-col gap-1.5">
        <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.language') }}</label>
        <select
          v-model="locale"
          @change="setLanguage(locale)"
          class="px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full"
        >
          <option value="zh">{{ t('config.lang_zh') }}</option>
          <option value="en">{{ t('config.lang_en') }}</option>
        </select>
      </div>

      <div class="flex flex-col gap-1.5">
        <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.theme') }}</label>
        <select
          v-model="globalStore.theme"
          class="px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full"
        >
          <option value="light">{{ t('config.theme_light') }}</option>
          <option value="dark">{{ t('config.theme_dark') }}</option>
          <option value="purple">{{ t('config.theme_purple') }}</option>
          <option value="pink">{{ t('config.theme_pink') }}</option>
          <option value="green">{{ t('config.theme_green') }}</option>
          <option value="blue">{{ t('config.theme_blue') }}</option>
          <option value="system">{{ t('config.theme_system') }}</option>
        </select>
      </div>

      <div class="flex flex-col gap-1.5">
        <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.start_page') }}</label>
        <select
          v-model="globalStore.startPage"
          class="px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full"
        >
          <option value="last">{{ t('config.start_page_last') }}</option>
          <option value="overview">{{ t('nav.overview') }}</option>
          <option value="proxies">{{ t('nav.proxies') }}</option>
          <option value="subscription">{{ t('nav.subscription') }}</option>
          <option value="rules">{{ t('nav.rules') }}</option>
          <option value="connections">{{ t('nav.connections') }}</option>
          <option value="logs">{{ t('nav.logs') }}</option>
          <option value="config">{{ t('nav.config') }}</option>
        </select>
      </div>
    </div>
  </div>
</template>
