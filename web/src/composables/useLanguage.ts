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
    const target = locale.value === 'zh' ? 'en' : 'zh'
    locale.value = target
    localStorage.setItem('lang', target)
    updateTitle(globalStore.activeTab)
  }

  watch(() => globalStore.activeTab, (newTab: string) => {
    updateTitle(newTab)
  })

  return {
    locale,
    currentLangDisplay,
    toggleLanguage,
    updateTitle
  }
}
