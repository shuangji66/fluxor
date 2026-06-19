# Fluxor 项目 AI 代理指南 (AGENTS.md)

本文件为后续接手的 AI 编码助手或开发者提供项目的系统架构、核心逻辑、通信接口及开发规约，以便于快速理解项目并进行无缝维护与扩展。

---

## 1. 项目定位与架构概述

`Fluxor` 是一个轻量级、零臃肿的 Mihomo 内核管理面板与订阅生成系统。它采用**前后端不分离**的架构设计：
- **后端 (Go)**：使用 Go 1.26 标准库（仅引入 `gorilla/websocket` 作为唯一外部依赖）。后端托管在 Unix Socket (`/var/apps/Fluxor/target/app.sock`) 上，对外通过前端反向代理暴露，内嵌了前端的所有静态资源。
- **前端双版本并存与条件编译**：
  为了维护旧版的零构建独立性，同时满足现代客户端的体验，项目支持**双版本前端分流编译**：
  1. **Vanilla JS 版（旧版，默认）**：无需任何构建步骤，直接嵌入 `static/` 目录的静态原生 HTML/JS 源码。
  2. **Vue 3 TypeScript 版（新版，主维护）**：源码位于 `web/` 目录，由 Vue 3 (Composition API / Setup) + Vite + TailwindCSS + Pinia + TypeScript 构成。
  - **分流机制**：后端利用 Go 条件编译标签进行控制：
    - `assets_vanilla.go` (go:build !vue)：默认构建 Vanilla JS 前端。
    - `assets_vue.go` (go:build vue)：使用 `-tags vue` 参数时，自动嵌入 `web/dist` 中的 Vue 3 前端。
  - **开发环境编译**：任何新功能或漏洞修复请**首选在 Vue 3 版本中维护**，修改 Vue 代码后，需在 `web` 目录下执行 `npm run build`。

### 核心功能职责

1. **内核进程生命周期管理**：负责本地 Mihomo 二进制文件的启动、停止、状态查询及配置热重载（不中断长连接）。
2. **配置文件订阅与生成**：读取用户的订阅链接及自定义规则集，生成内核可运行的 `config.yaml`。
3. **通信中转代理 (Bridge)**：由于 Mihomo 运行在本地 Unix Socket 上，Fluxor 后端作为前端与本地内核之间的“双向桥梁”，代理所有的 HTTP API 请求与 WebSocket 数据流（流量、内存、连接、日志等），并自动附加 `Bearer Token` 认证。

---

## 2. 目录结构与索引

```text
fluxor/
├── main.go                 # 程序入口，初始化路由，绑定 Unix Socket
├── assets_vanilla.go       # 嵌入旧版静态资源 (tags: !vue)
├── assets_vue.go           # 嵌入 Vue 版编译产物 (tags: vue)
├── handlers_core.go        # 内核进程控制、核心请求代理与 Context 释放控制
├── handlers_api.go         # 代理内核 HTTP API 接口
├── handlers_index.go       # 主页入口 index.html 模板渲染
├── handlers_utils.go       # JSON 错误响应等辅助工具
├── subscribe.go            # 订阅生成逻辑
├── build/                  # 跨平台自动化编译与打包工具链（含配置文件模板）
├── static/                 # 旧版原生 Vanilla JS 前端目录
└── web/                    # 主维护 Vue 3 前端源码目录
    ├── src/
    │   ├── App.vue         # 主框架组件，负责侧边栏切换、亮暗主题、语言及全局配置状态预加载
    │   ├── main.ts         # 入口文件，挂载 Pinia 和 i18n
    │   ├── i18n.ts         # 全站国际化翻译资源文件，禁止在页面写入硬编码中文
    │   ├── store/
    │   │   ├── global.ts   # 侧边栏折叠、当前 Tab 激活状态及全局 Toast 队列控制
    │   │   ├── config.ts   # 托管内核状态、常规配置及订阅管理列表，提供全局初始化 actions
    │   │   ├── proxies.ts  # 托管代理组数据与节点测速延迟记录，支持静默刷新
    │   │   ├── overview.ts # 托管仪表盘实时性能指标与 60 点流量图表历史快照
    │   │   └── logs.ts     # 托管系统终端历史日志缓存快照，切页自动回显
    │   ├── utils/
    │   │   └── api.ts      # 强类型网络类库（HTTP fetch / WebSocket 连接）
    │   └── views/          # 业务视图组件
    │       ├── Overview.vue     # 概览：Canvas 高清折线图自绘
    │       ├── Proxies.vue      # 代理：手风琴组折叠与测速，根据延迟动态着色（红/黄/绿）
    │       ├── Subscription.vue # 订阅：订阅增删改查、手动健康度测试
    │       ├── Config.vue       # 配置：内核常规、网卡、TUN及高级运维微调
    │       ├── Connections.vue  # 连接：实时 TCP/UDP 树形监控、主动切断与历史已关闭连接审计
    │       ├── Rules.vue        # 规则：规则匹配列表规则过滤与启用/禁用控制
    │       └── Logs.vue         # 日志：着色过滤与终端自滚
    └── vite.config.ts      # 配置自定义编译重定向插件，确保产物目录兼容 embed.FS
```

---

## 3. 前后端通信与代理机制 (重要规避点)

### 3.1 统一路由前缀 (BASE_URL)
所有的请求均有统一的基本路径前缀：`baseURL = "/app/Fluxor"`。
在 Vue 源码中，所有 `apiFetch` 或 WebSocket 通信必须调用 [api.ts](web/src/utils/api.ts)，它会自动且妥善地完成前缀拼接。

### 3.2 HTTP 代理流过早截断修复与 Context 释放
前端向后端发起管理请求时，后端通过 Unix Socket 拨号并发 Do(req) 请求内核。为了防止大 JSON 数据（例如代理组数据、连接历史）在传输中因超时 Context 被提前取消导致流被中断（抛出 `Unterminated string in JSON` 错误），后端在 [handlers_core.go](handlers_core.go) 实现了：
```go
type cancelableReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}
func (c *cancelableReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.cancel() // 确保数据全部读取拷贝完毕后，在 Close 时才会真正执行 cancel 释放资源
	return err
}
```
编写新的 API 代理 Handler 时，必须使用此包装类接管 Context 的回收。

---

## 4. 前端数据更新与缓存架构 (开发约束)

为了保证页面来回切换时的丝滑体验，同时避免在后台空跑产生资源泄漏：
1. **状态托管与秒开**：
   - 将各个页面的核心业务数据（如配置、代理节点）收归全局 Pinia store 维护。组件重新挂载时无缝呈现历史快照，随后在后台静默发起 fetch 刷新数据。
2. **WebSocket 生命周期控制**：
   - 流量图表（`Overview.vue`）和终端日志（`Logs.vue`）所依赖的 WebSockets 连接，**必须在组件卸载 (onUnmounted) 时主动关闭**，避免用户不在当前页时持续消耗系统资源。
   - 关闭连接前，最新的图表折线点、日志内容会被保存在 `useOverviewStore` 和 `useLogStore` 中。重新切回页面时，这些数据会立刻呈现，并由新开启 of WS 连接继续往下追加，实现无感过渡。
3. **批量测速并发控制规约**：
   - 无论是新版 Vue 3 还是旧版 Vanilla JS 前端，进行批量测速（全部测速或组测速）时，**必须强制实施并发控制（默认并发数限制为 10）**。绝对禁止一次性无限制发起数百个网络测速请求，防止浏览器连接队列拥堵与后端 Unix Socket 重试负载崩溃。
4. **弹窗与交互去原生化**：
   - 全站**绝对禁止使用**浏览器的阻塞式 `alert(...)`。如有提示需要，一律使用 `globalStore.showToast(text, 'success' | 'error' | 'warning' | 'info')` 发送非阻塞高颜值 Toast。
   - 所有在页面上展示的图标**必须使用 `xicons` (@vicons/ionicons5)**，禁止在系统硬编码 SVG、Emoji 或 颜文字符号（包括 `build/app/templates` 的内置配置模板中也绝对禁止在代理组和规则名中硬编码 Emoji），保持视觉绝对统一。
