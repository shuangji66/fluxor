import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

// Toast 操作按钮接口
export interface ToastAction {
  label: string
  callback: () => void
}

export interface ToastMessage {
  id: number
  text: string
  type: 'success' | 'error' | 'warning' | 'info'
  action?: ToastAction   // 可选操作按钮
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

  // 显示 Toast，支持操作按钮
  const showToast = (
    text: string,
    type: 'success' | 'error' | 'warning' | 'info' = 'info',
    action?: ToastAction
  ) => {
    const id = Date.now() + Math.random()
    // 如果有 action，包装回调，执行后自动移除
    let wrappedAction: ToastAction | undefined
    if (action) {
      wrappedAction = {
        label: action.label,
        callback: () => {
          // 先执行用户定义的回调
          action.callback()
          // 然后移除当前 Toast
          removeToast(id)
        }
      }
    }
    toasts.value.push({ id, text, type, action: wrappedAction })
  
    // 没有 action 时，3秒后自动消失
    if (!action) {
      setTimeout(() => {
        toasts.value = toasts.value.filter(t => t.id !== id)
      }, 3000)
    }
    return id
  }

  const removeToast = (id: number) => {
    toasts.value = toasts.value.filter(t => t.id !== id)
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

  const updateInfo = ref<{ hasUpdate: boolean; latest: string; current: string; releaseNotes?: string } | null>(null)

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