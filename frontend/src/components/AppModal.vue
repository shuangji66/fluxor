<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'

/**
 * 通用模态外壳。
 *
 * 收拢两件此前每个弹窗各写一遍、且都没写对的事：
 * 1. **键盘可达**：Esc 关闭、Tab 焦点锁在面板内、打开时聚焦面板、关闭后焦点回到触发元素；
 * 2. **语义**：`role="dialog"` + `aria-modal` + `aria-label`（读屏能识别为对话框）。
 *
 * 遮罩类名沿用项目既有的 `glass-mask` 方案，面板外观由 `panelClass` 决定，
 * 因此各处的圆角 / 宽度 / 内边距不受影响。Teleport 目标固定为 body。
 *
 * 注意：`open` 与视图激活态（`useViewActive`）的相与由调用方负责——本组件只认 `open`，
 * 这样 KeepAlive 缓存页里的弹窗仍可在切回时原样恢复，而不会跨页残留。
 */
const props = withDefaults(
  defineProps<{
    open: boolean
    /** 面板附加类（宽度 / 圆角 / 内边距 / 最大高度等） */
    panelClass?: string
    /** 无障碍名称，通常传弹窗标题文案 */
    label?: string
    /** 点击遮罩是否关闭 */
    closeOnBackdrop?: boolean
    /** 提交中等场景禁止 Esc 关闭 */
    persistent?: boolean
    /** 遮罩层附加类（层级不同时覆盖，如确认框的 z-[10000]） */
    overlayClass?: string
  }>(),
  { panelClass: '', closeOnBackdrop: true, persistent: false, overlayClass: 'z-[9999]' }
)

const emit = defineEmits<{ (e: 'close'): void }>()

const panelRef = ref<HTMLElement | null>(null)
let lastFocused: HTMLElement | null = null

const FOCUSABLE =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])'

const focusables = (root: HTMLElement) =>
  [...root.querySelectorAll<HTMLElement>(FOCUSABLE)].filter(
    (el) => el.offsetParent !== null || el === document.activeElement
  )

const onKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Escape' && !props.persistent) {
    e.stopPropagation()
    emit('close')
    return
  }
  if (e.key !== 'Tab' || !panelRef.value) return
  const nodes = focusables(panelRef.value)
  if (nodes.length === 0) {
    e.preventDefault()
    panelRef.value.focus()
    return
  }
  const first = nodes[0]
  const last = nodes[nodes.length - 1]
  // 焦点环：在首尾之间循环，避免 Tab 跑出面板落到背后的页面上
  if (e.shiftKey && (document.activeElement === first || document.activeElement === panelRef.value)) {
    e.preventDefault()
    last.focus()
  } else if (!e.shiftKey && document.activeElement === last) {
    e.preventDefault()
    first.focus()
  }
}

watch(
  () => props.open,
  async (open) => {
    if (open) {
      lastFocused = (document.activeElement as HTMLElement) ?? null
      document.addEventListener('keydown', onKeydown, true)
      await nextTick()
      panelRef.value?.focus()
    } else {
      document.removeEventListener('keydown', onKeydown, true)
      lastFocused?.focus?.()
      lastFocused = null
    }
  },
  { immediate: true }
)

onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown, true))
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="fixed inset-0 glass-mask flex items-center justify-center p-4"
      :class="overlayClass"
      @click.self="closeOnBackdrop && emit('close')"
    >
      <div
        ref="panelRef"
        role="dialog"
        aria-modal="true"
        :aria-label="label"
        tabindex="-1"
        class="glass-heavy border shadow-2xl animate-[zoomIn_0.15s_ease-out] relative overflow-hidden outline-none"
        :class="panelClass"
      >
        <slot />
      </div>
    </div>
  </Teleport>
</template>
