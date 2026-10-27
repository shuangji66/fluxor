import { computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useGlobalStore } from '../store/global'

export function useLanguage() {
  const { t, locale } = useI18n()
  const globalStore = useGlobalStore()

  const currentLangDisplay = computed(() => {
    return locale.value === 'zh' ? '简' : 'EN'
  })

  const updateTitle = (tabName: string) => {
    document.title = 'Fluxor - ' + t('nav.' + tabName)
  }

  const toggleLanguage = () => {
    setLanguage(locale.value === 'zh' ? 'en' : 'zh')
  }

  // 直接切到指定语言：落 localStorage 并同步窗口标题（语言下拉用）
  const setLanguage = (lang: string) => {
    locale.value = lang
    localStorage.setItem('lang', lang)
    updateTitle(globalStore.activeTab)
  }

  watch(() => globalStore.activeTab, (newTab: string) => {
    updateTitle(newTab)
  })

  return {
    locale,
    currentLangDisplay,
    setLanguage,
    toggleLanguage,
    updateTitle
  }
}
