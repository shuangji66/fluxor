<script setup lang="ts">
import type { Component } from 'vue'

/**
 * 页面顶栏（7 个视图共用）。
 *
 * 此前每个视图都复制了同一段容器类名与标题结构，同一个布局问题要改 7 处
 * （订阅页顶栏内容贴顶就是其中一例：复制来的容器漏了让内容行撑满高度的类）。
 * 这里把容器、标题、图标、间距固定下来，各视图只填内容。
 *
 * 插槽分区（按渲染顺序）：
 * - `title`：标题文字（默认取 `title` prop），仍在 `<h3>` 内，可插内联元素；
 * - `lead`：紧跟标题之后、与标题同属左侧分组的控件（如规则页的页签）；
 * - 默认插槽：中间区（如模式分段控件），在桌面端可借 `md:flex-1` 居中；
 * - `actions`：右侧操作区。
 *
 * 插槽内容会成为容器的直接 flex 子项，因此仍可用 `order-*` 控制移动端换行顺序
 * （日志页依赖这一点：标题 / 操作 / 搜索三块在窄屏重排）。
 *
 * 容器桌面端固定 56px 高，内容靠 `items-center` 垂直居中；移动端不固定高度，
 * 由 `py-3` + 换行后的内容高度决定。
 */
withDefaults(
  defineProps<{
    /** 标题文案（也可改用 `title` 插槽以插入图标之外的额外元素） */
    title?: string
    /** 标题前的图标组件（如 @vicons 的 GridOutline） */
    icon?: Component
    /** 子项间距；个别页面在移动端需要更紧凑时覆盖 */
    gap?: string
    /** 左侧分组（标题 + `lead`）的附加类；窄屏需要摊开时由调用方传入 */
    leftClass?: string
  }>(),
  { gap: 'gap-4', leftClass: 'gap-4' }
)
</script>

<template>
  <div
    class="glass-medium shadow-none px-6 py-3 md:py-0 rounded-xl border border-slate-200/50 dark:border-slate-800/50 flex flex-wrap items-center justify-between transition-all shrink-0 h-auto min-h-[56px] md:h-[56px]"
    :class="gap"
  >
    <!-- 左侧分组不设 min-w-0：flex 项默认 min-width:auto 才能让「标题+lead」在窄屏
         保持 min-content 宽度，从而把 actions 挤到下一行；写成 min-w-0 会退化成
         同一行互相压扁（标题竖排、控件重叠）。标题自身的 min-w-0 只影响其内部换行。 -->
    <div class="flex items-center" :class="leftClass">
      <h3 class="text-base font-semibold flex items-center gap-2 min-w-0">
        <component :is="icon" v-if="icon" class="w-5 h-5 text-accent shrink-0" />
        <slot name="title">{{ title }}</slot>
      </h3>
      <slot name="lead" />
    </div>
    <slot />
    <slot name="actions" />
  </div>
</template>
