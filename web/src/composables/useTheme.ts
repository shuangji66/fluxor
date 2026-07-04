import { watch, onUnmounted } from 'vue'
import { useGlobalStore } from '../store/global'

export function useTheme() {
  const globalStore = useGlobalStore()

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

    systemThemeListener = () => {
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
      pink: 'green',
      green: 'blue',
      blue: 'system',
      system: 'light'
    }
    globalStore.theme = cycle[current] || 'system'
  }

  onUnmounted(() => {
    if (systemThemeListener) {
      window.matchMedia('(prefers-color-scheme: dark)').removeEventListener('change', systemThemeListener)
    }
  })

  return {
    initTheme,
    switchThemeCycle
  }
}
