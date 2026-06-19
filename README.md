# Fluxor 工程技术手册

本文件面向技术维护人员，主要用于指导 Fluxor 项目的工程结构图谱、开发环境联调、跨平台构建打包与部署运维。

关于具体的编码避坑指南、双向中转 Context 回收实现细节及 UI 开发设计限制规约，请参阅 [AGENTS.md](AGENTS.md)。

---

## 1. 目录结构（概览）

```text
fluxor/
├── main.go, assets_vanilla.go, assets_vue.go   # 入口 + 前端条件编译嵌入
├── handlers_core.go, handlers_api.go            # 内核管理 & API 代理
├── handlers_index.go, handlers_utils.go         # 页面渲染 & 响应工具
├── subscribe.go                                 # 订阅生成逻辑
├── static/                                      # Vanilla JS 旧版前端
├── build/                                       # 打包脚本 & 配置模板
└── web/                                         # Vue 3 新版前端（主维护）
    ├── package.json, vite.config.js, tailwind.config.js, ...
    └── src/
        ├── main.ts, App.vue, i18n.ts
        ├── utils/api.ts                         # HTTP/WS 网络工具
        ├── store/  (global / config / overview / proxies / connections / rules / logs)
        └── views/  (Overview / Proxies / Rules / Connections / Logs / Config / Subscription)
```

> 详细文件级描述请参阅 [AGENTS.md §2](AGENTS.md#2-目录结构与索引)。前端使用 `globalStore.activeTab` + 动态组件 `<component :is="..." />` 切换页面，未使用 vue-router。**所有新功能请在 Vue 3 版本中维护**。

---

## 2. 编译构建与分流控制

### 2.1 前端运行脚本
| 命令 | 说明 |
|------|------|
| `npm run dev` | 启动 Vite 开发服务器，默认端口 5173，热更新 |
| `npm run build` | 生产构建，产物输出到 `web/dist/`，自动将 index.html 移至 `static/html/` |
| `npm run preview` | 预览生产构建产物 |

### 2.2 编译标签（Build Tags）控制
系统通过 Go 条件标签实现前端双版本并存嵌入：
- **默认构建（Vanilla）**：
  直接运行 `go build`（使用 `assets_vanilla.go`），打包的原生静态网页无需任何额外构建步骤。
- **现代化构建（Vue 3）**：
  必须先在 `web/` 下完成 `npm run build`，随后在根目录下添加 Tags 编译：
  ```bash
  go build -tags vue
  ```

### 2.3 跨平台自动化打包
- **Windows 构建**：运行 `build/build.bat`。自动执行前端构建 → 交叉编译 Linux amd64 后端 → 调用 `fnpack.exe` 封包校验。
- **Linux 构建**：运行 `build/build.sh`。执行相同的打包机制，自动寻址 `fnpack-1.2.1-linux-amd64` 封包工具。

---

## 3. 本地开发与联调环境

### 3.1 跨域及反向代理说明
- **后端运行**：Go 后端默认在 Unix Socket (`/var/apps/Fluxor/target/app.sock`) 上提供服务。
- **前端开发热更新**：
  在 `web/` 目录下执行 `npm run dev`，Vite 默认起在 `5173` 端口。Vite 在 `vite.config.js` 中配置了对 `/app/Fluxor` 路径的反向代理，直接中转至后端的 Unix Socket 或开发监听端口，避免跨域问题。

### 3.2 技术栈汇总
| 层级 | 技术选型 |
|------|----------|
| 框架 | Vue 3 (Composition API + `<script setup>`) |
| 状态管理 | Pinia |
| 构建工具 | Vite 5 |
| 样式方案 | Tailwind CSS 3 + CSS 变量主题系统 |
| 国际化 | vue-i18n 9 (Composition API 模式) |
| 图标库 | @vicons/ionicons5 |
| 网络通信 | fetch (HTTP) + WebSocket (实时数据流) |
| 开发语言 | TypeScript |
| 后端基路径 | `/app/Fluxor/` |
