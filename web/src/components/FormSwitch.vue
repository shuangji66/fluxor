<script setup lang="ts">
// 使用 Vue 3.4+ defineModel 实现极致简洁的双向绑定
const model = defineModel<boolean>({ default: false })
</script>

<template>
  <button
    type="button"
    @click="model = !model"
    class="w-10 h-6 flex items-center rounded-full p-0.5 transition-all outline-none duration-200"
    :class="model ? 'bg-accent justify-end' : 'bg-slate-200 dark:bg-slate-700 justify-start'"
  >
    <span class="w-5 h-5 rounded-full bg-white shadow-md transition-transform duration-200"></span>
  </button>
</template>
