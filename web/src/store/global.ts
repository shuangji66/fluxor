import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

export interface ToastMessage {
  id: number
  text: string
  type: 'success' | 'error' | 'warning' | 'info'
}

export const useGlobalStore = defineStore('global', () => {
  const activeTab = ref<string>(localStorage.getItem('fluxor-active-tab') || 'overview')
  const isSidebarCollapsed = ref<boolean>(localStorage.getItem('fluxor-sidebar-collapsed') === 'true')
  const theme = ref<string>(localStorage.getItem('fluxor-theme') || 'system')
  
  const toasts = ref<ToastMessage[]>([])

  const showToast = (text: string, type: 'success' | 'error' | 'warning' | 'info' = 'info') => {
    const id = Date.now() + Math.random()
    toasts.value.push({ id, text, type })
    setTimeout(() => {
      toasts.value = toasts.value.filter(t => t.id !== id)
    }, 3000)
  }

  watch(activeTab, (newTab) => {
    localStorage.setItem('fluxor-active-tab', newTab)
  })

  watch(isSidebarCollapsed, (val) => {
    localStorage.setItem('fluxor-sidebar-collapsed', val ? 'true' : 'false')
  })

  return { activeTab, isSidebarCollapsed, theme, toasts, showToast }
})
