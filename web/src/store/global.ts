import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

export interface ToastMessage {
  id: number
  text: string
  type: 'success' | 'error' | 'warning' | 'info'
}

export interface ConfirmOptions {
  title?: string
  message: string
  okText?: string
  cancelText?: string
  type?: 'info' | 'warning' | 'danger' | 'success'
}

export interface ConfirmState {
  visible: boolean
  title: string
  message: string
  okText: string
  cancelText: string
  type: 'info' | 'warning' | 'danger' | 'success'
  resolve: ((value: boolean) => void) | null
}

export const useGlobalStore = defineStore('global', () => {
  const startPage = ref<string>(localStorage.getItem('fluxor-start-page') || 'last')

  const getInitialTab = () => {
    const sp = startPage.value
    return sp === 'last' ? (localStorage.getItem('fluxor-active-tab') || 'overview') : sp
  }

  const activeTab = ref<string>(getInitialTab())
  const isSidebarCollapsed = ref<boolean>(localStorage.getItem('fluxor-sidebar-collapsed') === 'true')
  const theme = ref<string>(localStorage.getItem('fluxor-theme') || 'system')
  
  const toasts = ref<ToastMessage[]>([])
  const confirmDialog = ref<ConfirmState | null>(null)

  const showToast = (text: string, type: 'success' | 'error' | 'warning' | 'info' = 'info') => {
    const id = Date.now() + Math.random()
    toasts.value.push({ id, text, type })
    setTimeout(() => {
      toasts.value = toasts.value.filter(t => t.id !== id)
    }, 3000)
  }

  const removeToast = (id: number) => {
    toasts.value = toasts.value.filter(t => t.id !== id)
  }

  // 触发全局模态确认框
  const showConfirm = (options: ConfirmOptions | string): Promise<boolean> => {
    return new Promise((resolve) => {
      // 防重入处理，避免 Promise 覆盖泄露
      if (confirmDialog.value && confirmDialog.value.visible) {
        resolve(false);
        return;
      }

      let message = ''
      let title = ''
      let okText = ''
      let cancelText = ''
      let type: 'info' | 'warning' | 'danger' | 'success' = 'warning'

      if (typeof options === 'string') {
        message = options
      } else {
        message = options.message
        title = options.title || ''
        okText = options.okText || ''
        cancelText = options.cancelText || ''
        type = options.type || 'warning'
      }

      confirmDialog.value = {
        visible: true,
        title,
        message,
        okText,
        cancelText,
        type,
        resolve
      }
    })
  }

  // 确认或取消回调
  const handleConfirmResult = (result: boolean) => {
    if (confirmDialog.value?.resolve) {
      confirmDialog.value.resolve(result)
    }
    confirmDialog.value = null
  }

  watch(activeTab, (newTab) => {
    localStorage.setItem('fluxor-active-tab', newTab)
  })

  watch(isSidebarCollapsed, (val) => {
    localStorage.setItem('fluxor-sidebar-collapsed', val ? 'true' : 'false')
  })

  watch(startPage, (newVal) => {
    localStorage.setItem('fluxor-start-page', newVal)
  })

  return { 
    startPage,
    activeTab, 
    isSidebarCollapsed, 
    theme, 
    toasts, 
    confirmDialog, 
    showToast, 
    removeToast,
    showConfirm, 
    handleConfirmResult 
  }
})

