<script setup lang="ts">
// 首屏加载占位：异步视图 chunk 到达前显示。
// 只画骨架、不写文案：这里没有语言上下文（文案走 i18n 的成本高于收益），
// 骨架与页面顶栏 + 内容区结构一致，避免 chunk 到达时的高度跳动。
</script>

<template>
  <div class="flex flex-col flex-1 min-h-0 gap-4 h-full" aria-busy="true">
    <div class="glass-medium shadow-none px-6 py-3 md:py-0 rounded-xl border border-slate-200/50 dark:border-slate-800/50 shrink-0 md:h-[56px] flex items-center">
      <div class="h-4 w-28 rounded bg-slate-200/70 dark:bg-slate-800/70 animate-pulse"></div>
    </div>
    <div class="flex-1 min-h-0 glass-medium shadow-none p-6 rounded-xl border border-slate-200/50 dark:border-slate-800/50">
      <div class="h-4 w-1/3 rounded bg-slate-200/70 dark:bg-slate-800/70 animate-pulse"></div>
      <div class="mt-4 h-4 w-2/3 rounded bg-slate-200/70 dark:bg-slate-800/70 animate-pulse"></div>
      <div class="mt-4 h-4 w-1/2 rounded bg-slate-200/70 dark:bg-slate-800/70 animate-pulse"></div>
    </div>
  </div>
</template>
