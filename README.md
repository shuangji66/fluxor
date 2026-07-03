# Fluxor

`Fluxor` 是一个轻量级、无冗余的 Mihomo (Clash.Meta) 内核管理面板与订阅生成系统。该系统采用前后端一体化设计，支持自动化配置生成、透明代理（TProxy）管理以及实时的内核状态监控。

---

## 核心特性

- **内核管理**：支持 Mihomo 二进制进程启动、停止、状态查询及配置热重载。
- **订阅中心**：提供订阅链接的 CRUD 管理，支持解析并生成内核 `config.yaml` 配置文件，并可自动应用。
- **实时监控**：基于 WebSocket 协议，实时中转并呈现上传与下载流量速度、内存占用、内核日志流及连接历史。
- **透明代理 (TProxy)**：集成了 nftables 防火墙透明代理及路由例外控制，支持本机出站代理开关及例外 IP 与端口过滤。
- **双前端支持**：
  - **Vue 3 现代版 (推荐)**：采用 Vue 3 与 TypeScript 构建，提供高性能的客户端交互体验，源码位于 `web/` 目录。
  - **Vanilla JS 原生版**：零构建步骤，直接原生加载，适合轻量级环境，源码位于 `static/` 目录。

---

## 技术栈

### 后端
- **核心语言**：Go 1.26 (标准库)
- **外部依赖**：`gorilla/websocket` (唯一外部依赖)

### 前端 (Vue 3 现代版)
- **核心框架**：Vue 3 (Composition API) + TypeScript
- **状态管理**：Pinia
- **构建工具**：Vite 5
- **样式方案**：Tailwind CSS 3 (支持 data-theme 亮暗切换)
- **国际化**：vue-i18n 9
- **网络通信**：fetch (HTTP) + WebSocket (实时数据流)

---

## 本地开发

### 1. 前端开发
进入 `web/` 目录进行前端热更新开发：
```bash
cd web
npm install
npm run dev
```

### 2. 后端开发
Go 后端开发与运行：
```bash
# 默认构建（嵌入旧版 Vanilla JS 前端）
go build

# 编译 Vue 3 前端现代版
cd web && npm run build && cd ..
go build -tags vue
```

---

## 开发者指南

关于详细的目录结构、路由 API 对照表、前后端通信机制、TProxy 防火墙安全性设计及 Tailwind UI 样式规范等深度技术规约，请阅读 [AGENTS.md](AGENTS.md)。

