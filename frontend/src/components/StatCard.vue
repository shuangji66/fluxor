<script setup lang="ts">
/**
 * 概览页的指标卡（8 张同构卡片的公共外壳）。
 *
 * 此前每张卡都重复同一串容器类名与「标题 / 数值」两行结构，改一次配色或字号要改 8 处。
 * 这里只固化外观：标题、数值、可选的可点击态；数值的格式化由调用方完成
 * （保留 `formatSpeed/formatBytes` 在不同卡片上的差异）。
 */
withDefaults(
  defineProps<{
    /** 标题文案（已翻译） */
    label?: string
    /** 数值文案（已格式化）；需要富内容时改用默认插槽 */
    value?: string | number
    /** 数值的颜色/截断等附加类 */
    valueClass?: string
    /** 数值的悬停提示（如完整内核版本号） */
    title?: string
    /** 整卡可点击（如跳转连接页） */
    clickable?: boolean
  }>(),
  { valueClass: '', clickable: false }
)
</script>

<template>
  <div
    class="bg-slate-50/50 dark:bg-slate-900/30 p-4 rounded-xl border border-slate-200/40 dark:border-slate-800/40 transition-all"
    :class="clickable ? 'cursor-pointer hover:border-accent/40 active:scale-[0.98]' : ''"
  >
    <div class="text-xs sm:text-sm font-bold text-slate-500 dark:text-slate-400">
      <slot name="label">{{ label }}</slot>
    </div>
    <slot>
      <div class="text-base sm:text-lg font-extrabold mt-1" :class="valueClass" :title="title">{{ value }}</div>
    </slot>
  </div>
</template>
