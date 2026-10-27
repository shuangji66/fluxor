import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  // 使用相对路径构建，配合后端渲染的 <base href> 标签适配任意子路径部署
  base: './',
  build: {
    outDir: 'dist',
    assetsDir: 'assets',
    emptyOutDir: true,
    // vendor 单独成块：vue / vue-i18n / pinia 等内容随版本固定，
    // 与「改一行业务代码」解耦，避免每次改前端都让整包缓存失效
    // （后端对 /assets/ 的不可变缓存策略见 AGENTS.md 3.3）。
    rolldownOptions: {
      output: {
        advancedChunks: {
          groups: [{ name: 'vendor', test: /node_modules/ }]
        }
      }
    }
  }
})