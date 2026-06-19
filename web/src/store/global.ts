import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

export interface ToastMessage {
  id: number
  text: string
  type: 'success' | 'error' | 'warning' | 'info'
}

<<<<<<< HEAD
export interface ConfirmOptions {
  title?: string
  message: string
  okText?: string
  cancelText?: string
}

export interface ConfirmState {
  visible: boolean
  title: string
  message: string
  okText: string
  cancelText: string
  resolve: ((value: boolean) => void) | null
}

=======
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
export const useGlobalStore = defineStore('global', () => {
  const activeTab = ref<string>(localStorage.getItem('fluxor-active-tab') || 'overview')
  const isSidebarCollapsed = ref<boolean>(localStorage.getItem('fluxor-sidebar-collapsed') === 'true')
  const theme = ref<string>(localStorage.getItem('fluxor-theme') || 'system')
  
  const toasts = ref<ToastMessage[]>([])
<<<<<<< HEAD
  const confirmDialog = ref<ConfirmState | null>(null)
=======
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69

  const showToast = (text: string, type: 'success' | 'error' | 'warning' | 'info' = 'info') => {
    const id = Date.now() + Math.random()
    toasts.value.push({ id, text, type })
    setTimeout(() => {
      toasts.value = toasts.value.filter(t => t.id !== id)
    }, 3000)
  }

<<<<<<< HEAD
  // 触发全局模态确认框
  const showConfirm = (options: ConfirmOptions | string): Promise<boolean> => {
    return new Promise((resolve) => {
      let message = ''
      let title = ''
      let okText = ''
      let cancelText = ''

      if (typeof options === 'string') {
        message = options
      } else {
        message = options.message
        title = options.title || ''
        okText = options.okText || ''
        cancelText = options.cancelText || ''
      }

      confirmDialog.value = {
        visible: true,
        title,
        message,
        okText,
        cancelText,
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

=======
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
  watch(activeTab, (newTab) => {
    localStorage.setItem('fluxor-active-tab', newTab)
  })

  watch(isSidebarCollapsed, (val) => {
    localStorage.setItem('fluxor-sidebar-collapsed', val ? 'true' : 'false')
  })

<<<<<<< HEAD
  return { 
    activeTab, 
    isSidebarCollapsed, 
    theme, 
    toasts, 
    confirmDialog, 
    showToast, 
    showConfirm, 
    handleConfirmResult 
  }
})

=======
  return { activeTab, isSidebarCollapsed, theme, toasts, showToast }
})
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
