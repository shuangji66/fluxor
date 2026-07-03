<script setup lang="ts">
// 使用 Vue 3.4+ defineModel 实现极致简洁的双向绑定
const model = defineModel<boolean>({ default: false })
</script>

<template>
  <button
    type="button"
    @click="model = !model"
    class="w-10 h-6 flex items-center rounded-full p-0.5 transition-all outline-none duration-200 border"
    :class="model 
      ? 'bg-accent border-transparent justify-end' 
      : 'switch-bg-off border-apple-border/60 justify-start'"
  >
    <span class="w-5 h-5 rounded-full bg-white shadow-[0_1.5px_3px_rgba(0,0,0,0.2),0_1px_1px_rgba(0,0,0,0.05)] border border-slate-200/30 transition-transform duration-200"></span>
  </button>
</template>
