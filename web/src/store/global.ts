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
  checkboxLabel?: string
  checkboxDefault?: boolean
}

export interface ConfirmState {
  visible: boolean
  title: string
  message: string
  okText: string
  cancelText: string
  type: 'info' | 'warning' | 'danger' | 'success'
  checkboxLabel?: string
  checkboxChecked?: boolean
  resolve: ((value: any) => void) | null
}

export const useGlobalStore = defineStore('global', () => {
  const startPage = ref<string>(localStorage.getItem('fluxor-start-page') || 'last')

  const getInitialTab = () => {
    const sp = startPage.value
    return sp === 'last' ? (localStorage.getItem('fluxor-active-tab') || 'overview') : sp
  }

  const activeTab = ref<string>(getInitialTab())
  const isSidebarCollapsed = ref<boolean>(localStorage.getItem('fluxor-sidebar-collapsed') === 'true')
  const theme = ref<string>(localStorage.getItem('fluxor-theme') || 'pink')
  
  const toasts = ref<ToastMessage[]>([])
  const confirmDialog = ref<ConfirmState | null>(null)
  const showAbout = ref<boolean>(false)

  const showToast = (text: string, type: 'success' | 'error' | 'warning' | 'info' = 'info') => {
    const id = Date.now() + Math.random()
    toasts.value.push({ id, text, type })
    setTimeout(() => {
      toasts.value = toasts.value.filter(t => t.id !== id)
    }, 3000)
  }

  const removeToast = (id: number) => {
    const targetId = id
    toasts.value = toasts.value.filter(t => t.id !== targetId)
  }

  // 触发全局模态确认框
  function showConfirm(options: string): Promise<boolean>;
  function showConfirm(options: ConfirmOptions & { checkboxLabel: string }): Promise<{ confirmed: boolean; checkboxChecked: boolean }>;
  function showConfirm(options: ConfirmOptions): Promise<boolean>;
  function showConfirm(options: ConfirmOptions | string): Promise<any> {
    return new Promise((resolve) => {
      // 防止重复弹窗
      if (confirmDialog.value && confirmDialog.value.visible) {
        resolve(typeof options === 'object' && options.checkboxLabel ? { confirmed: false, checkboxChecked: false } : false);
        return;
      }

      let message = ''
      let title = ''
      let okText = ''
      let cancelText = ''
      let type: 'info' | 'warning' | 'danger' | 'success' = 'warning'
      let checkboxLabel: string | undefined = undefined
      let checkboxChecked = false

      if (typeof options === 'string') {
        message = options
      } else {
        message = options.message
        title = options.title || ''
        okText = options.okText || ''
        cancelText = options.cancelText || ''
        type = options.type || 'warning'
        checkboxLabel = options.checkboxLabel
        checkboxChecked = options.checkboxDefault || false
      }

      const hasCheckbox = !!checkboxLabel

      confirmDialog.value = {
        visible: true,
        title,
        message,
        okText,
        cancelText,
        type,
        checkboxLabel,
        checkboxChecked,
        resolve: (confirmed: boolean) => {
          if (hasCheckbox) {
            resolve({ confirmed, checkboxChecked: confirmDialog.value?.checkboxChecked ?? false })
          } else {
            resolve(confirmed)
          }
        }
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

  const updateInfo = ref<{ hasUpdate: boolean; latest: string; current: string } | null>(null)

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
    showAbout,
    showToast, 
    removeToast,
    showConfirm, 
    updateInfo,
    handleConfirmResult 
  }
})

