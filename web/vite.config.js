import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import fs from 'fs'
import path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

function fluxorBuildPlugin() {
  return {
    name: 'fluxor-build-plugin',
    buildStart() {
      const assetsDir = path.resolve(__dirname, 'dist/static/assets')
      if (fs.existsSync(assetsDir)) {
        const files = fs.readdirSync(assetsDir)
        for (const file of files) {
          const filePath = path.join(assetsDir, file)
          if (fs.statSync(filePath).isFile()) {
            fs.unlinkSync(filePath)
          }
        }
        console.log('[Fluxor] Cleared old assets in dist/static/assets')
      }
    },
    closeBundle() {
      const distDir = path.resolve(__dirname, 'dist')
      const indexHtml = path.resolve(distDir, 'index.html')
      const targetDir = path.resolve(distDir, 'static/html')
      const targetHtml = path.resolve(targetDir, 'index.html')

      if (fs.existsSync(indexHtml)) {
        if (!fs.existsSync(targetDir)) {
          fs.mkdirSync(targetDir, { recursive: true })
        }
        fs.renameSync(indexHtml, targetHtml)
        console.log('[Fluxor] Successfully relocated index.html to dist/static/html/index.html')
      }
    }
  }
}

const pkg = JSON.parse(fs.readFileSync(path.resolve(__dirname, 'package.json'), 'utf-8'))

export default defineConfig({
  plugins: [vue(), fluxorBuildPlugin()],
  base: '/app/Fluxor/',
  define: {
    __APP_VERSION__: JSON.stringify(pkg.version)
  },
  build: {
    outDir: 'dist',
    assetsDir: 'static/assets',
    emptyOutDir: false // 避免清空 dist 导致占位符被删除或并发问题
  }
})
