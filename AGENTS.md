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
├── main.go                 # 程序入口，路由注册，Unix Socket 监听，WebSocket 双向代理 (wsProxyHandler)
├── assets_vanilla.go       # 嵌入旧版静态资源 (go:build !vue)
├── assets_vue.go           # 嵌入 Vue 版编译产物 (go:build vue)
├── handlers_core.go        # 内核进程生命周期（start/stop/restart/coreRequest/cancelableReadCloser）
├── handlers_api.go         # 代理内核 HTTP API（流量、内存、连接、代理、规则、配置、DNS、GEO、升级等）
├── handlers_index.go       # 主页入口 index.html 模板渲染
├── handlers_utils.go       # JSON 错误响应工具 (writeJSONError/respondJSON) 及后端地址正则校验
├── subscribe.go            # 订阅配置 CRUD、config.yaml 生成、模板替换、MetaCubeXD config.js 修改
├── build/                  # 跨平台自动化编译与打包工具链（含 config_lite/base/full.yaml 模板）
├── static/                 # 旧版原生 Vanilla JS 前端目录
└── web/                    # 主维护 Vue 3 前端源码目录
    ├── package.json        # Vue 3.4 + Pinia + vue-i18n 9 + Vite 5 + Tailwind CSS 3 + TypeScript 5 + @vicons/ionicons5
    ├── vite.config.js      # 自定义 fluxorBuildPlugin：构建后将 index.html 移至 static/html/ 适配 embed.FS
    ├── tailwind.config.js  # data-theme 暗黑模式 + 扩展 accent/success/danger/warning 颜色
    ├── postcss.config.js   # Tailwind + Autoprefixer
    ├── index.html          # HTML 入口
    └── src/
        ├── main.ts         # 挂载 Pinia + vue-i18n (Composition API, legacy:false)
        ├── App.vue         # 根组件：响应式侧边栏/移动端底部 Tab、亮暗/跟随系统主题、中英切换、Toast 队列、Promise 确认框
        ├── env.d.ts        # .vue 类型声明 & Window.BASE_URL 接口扩展
        ├── i18n.ts         # 全站国际化（zh/en），从 localStorage 读取语言偏好，禁止硬编码中文
        ├── index.css       # Tailwind 基础指令 + CSS 变量亮暗主题（data-theme 选择器）+ 自定义滚动条
        ├── components/     # 公共及细粒度组件 (ProxyGroupCard, FormSwitch)
        ├── composables/    # 全局解耦组合式函数 (useTheme, useLanguage)
        ├── utils/
        │   ├── api.ts      # withBase() 拼接 BASE_URL、apiFetch() HTTP 封装、wsConnect() WebSocket 封装（自动 ws/wss 协议选择）
        │   └── mock.ts     # 前端离线开发模拟器：拦截 HTTP/WS 请求提供 mock 数据，支持脱离后端独立测试
        ├── store/
        │   ├── global.ts   # 标签页激活状态、侧边栏折叠、亮暗/跟随系统主题、Toast 队列（3s 自动消失）、Promise 驱动确认框
        │   ├── config.ts   # 内核运行状态、内核配置参数（allow-lan/ipv6/mode/log-level/tun/端口等）、订阅配置 CRUD
        │   ├── overview.ts # 仪表盘实时统计（速度/流量/内存/连接数/版本/当前节点）、60 点流量历史、3 路 WS + 1 路 HTTP 轮询
        │   ├── proxies.ts  # 代理组列表、节点延迟字典、手风琴展开状态、并发受限（10）批量测速
        │   ├── connections.ts # 活跃/已关闭连接列表、汇总统计、排序/搜索、WS 瞬时速率计算（快照差分）
        │   ├── rules.ts    # 规则列表、规则提供商列表、fetch/refresh 方法
        │   └── logs.ts     # 日志缓冲区（上限 2000 条）、自动滚动、暂停/继续、指数退避重连（1s~30s）
        └── views/
            ├── Overview.vue     # 概览：4 指标卡（上传/下载速度+总量）+ 4 信息卡（内存/连接数/版本/外部面板）+ Canvas 自绘折线图（max 60 点）
            ├── Proxies.vue      # 代理：手风琴展开/折叠、点击切换选择、单节点/组/全部测速、延迟着色（绿≤150 / 黄≤300 / 红>300ms/超时）
            ├── Rules.vue        # 规则：搜索过滤、启用/禁用开关（乐观更新+回滚）、规则提供商单个/全部更新
            ├── Connections.vue  # 连接：活跃/已关闭双标签、多列排序、搜索过滤、单条/全部断开（乐观更新移入 closed）、清空已关闭
            ├── Logs.vue         # 日志：暗色终端风格、级别过滤（Debug/Info/Warning/Error）、搜索、暂停/继续、智能自动滚动
            ├── Config.vue       # 配置：内核状态卡（启动/停止/升级）、常规参数、端口校验（1025-65535+重复检测）、TUN（gVisor/System/Mixed）、高级运维（重载/清缓存/GEO）、内置 DNS 查询
            └── Subscription.vue # 订阅：代理/面板端口、密钥显隐切换、规则集（lite/base/full）、UI 面板选择、订阅 CRUD 模态框（zoomIn 动画，支持订阅名称、链接、检测间隔、节点前缀）、流量/健康度/有效期卡片、「保存并应用」
```

> **页面路由机制**：未使用 vue-router，通过 `globalStore.activeTab` 与 `<component :is="..." />` 动态组件切换视图。在此基础上，外层包裹了 `<KeepAlive :max="6">` 进行视图缓存，以长效留存页面各交互状态（如滚动进度与折叠状态）并规避切页时的网络闪连。

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

### 3.3 后端 API 路由对照表（`main.go` 注册）

| 路由 | 方法 | Handler | 说明 |
|------|------|---------|------|
| `/` | GET | `handleIndex` | SPA 主页模板渲染 |
| `/core/status` | GET | `handleCoreStatus` | 内核运行状态（PID 文件检测） |
| `/core/start` | POST | `handleCoreStart` | 启动内核进程 |
| `/core/stop` | POST | `handleCoreStop` | 停止内核进程（SIGTERM） |
| `/core/restart` | POST | `handleCoreRestart` | 热重启（重载配置） |
| `/upgrade` | POST | `handleUpgrade` | 升级内核（透传内核 /upgrade） |
| `/subscribe/config` | GET/POST | `handleSubscribeConfigAPI` | 订阅配置读写（持久化到 subscribe.json） |
| `/subscribe/generate` | POST | `handleGenerateConfig` | 保存配置 + 生成 config.yaml + 重载内核 |
| `/traffic` | WS | `wsProxyHandler("/traffic")` | 实时流量数据 WebSocket 代理 |
| `/memory` | WS | `wsProxyHandler("/memory")` | 实时内存数据 WebSocket 代理 |
| `/logs` | WS | `wsProxyHandler("/logs")` | 实时日志流 WebSocket 代理 |
| `/connections` | WS/DELETE | `wsProxyHandler` / `handleConnectionsClose` | 连接实时流或全部断开 |
| `/connections/{id}` | DELETE | `handleConnectionsClose` | 断开指定连接 |
| `/version` | GET | `handleVersion` | 内核版本信息 |
| `/configs` | GET/PATCH/PUT | `handleConfigsAPI` | 获取/修改/重载内核配置 |
| `/configs/geo` | POST | `handleConfigsGeo` | 更新 GEO 数据库 |
| `/providers/geo` | POST | `handleProvidersGeo` | 回退 GEO 更新接口 |
| /providers/rules | GET | handleRuleProviders | 获取规则提供商列表 |
| /providers/rules/{name} | PUT | handleUpdateRuleProvider | 更新单个规则提供商 |
| /interfaces | GET | handleInterfaces | 返回系统所有的物理网络接口名称（过滤回环及未启用接口） |
| `/providers/proxies/{name}` | GET/PUT | `handleProviderProxies` | 获取/更新订阅代理信息 |
| `/rules` | GET | `handleRules` | 获取所有规则 |
| `/rules/disable` | PATCH | `handleRulesDisable` | 启用/禁用规则 |
| `/proxies` | GET | `handleProxies` | 获取所有代理组 |
| `/proxies/{name}/delay` | GET | `handleProxyDelay` | 测速（需 ?url=&timeout= 参数） |
| `/proxies/{name}` | PUT | `handleProxySwitch` | 切换代理选择 |
| `/cache/fakeip/flush` | POST | `handleFlushFakeIP` | 清空 FakeIP 缓存 |
| `/cache/dns/flush` | POST | `handleFlushDNS` | 清空 DNS 缓存 |
| `/dns/query` | GET | `handleDNSQuery` | DNS 查询（?name=&type=） |
| `/restart` | POST | `handleRestart` | 内核远端重启 |
| `/meta/` | GET | `http.FileServer` | MetaCubeXD 外部面板静态文件 |
| `/zash/` | GET | `http.FileServer` | Zashboard 外部面板静态文件 |
| `/static/` | GET | `http.FileServer` | 内嵌前端静态资源 |

> 所有路由均挂载在 `baseURL = "/app/Fluxor"` 之下，如 `/app/Fluxor/core/status`。

---

## 4. 前端数据更新与缓存架构 (开发约束)

为了保证页面来回切换时的丝滑体验，同时避免在后台空跑产生资源泄漏：

### 4.1 状态托管与秒开
- 将各个页面的核心业务数据（如配置列表、代理节点、规则、连接快照）收归全局 Pinia store 维护。组件重新挂载时无缝呈现历史快照，随后在后台静默发起 fetch 刷新数据（`silent = true` 参数）。

### 4.2 WebSocket 引用计数生命周期管理
所有实时数据流（流量、内存、连接、日志）均采用 **引用计数 + 防抖断开** 模式，而非在 `onUnmounted` 中直接关闭 WS：

```
subscribe() → subscriberCount++ → 首次订阅时建立连接
unsubscribe() → subscriberCount-- → 归零后延迟 3 秒（防抖）→ 若无新订阅才真正断开
```

**优势**：
- 同一组件可多次安全订阅（如 `Overview.vue` 中的流量、内存、状态三路数据各自独立计数）。
- 组件快速切出/切回时不中断 WS 连接，实现无感过渡。
- 连接断开时，若仍有订阅者，自动尝试重连：
  - 流量/内存/连接 WS：5 秒后重连。
  - 日志 WS：指数退避重连（1s → 2s → 4s → ... → 最大 30s）。

**缓存留存**：
- 流量历史（`overview.ts`）保留最近 60 个数据点。
- 日志缓冲区（`logs.ts`）上限 2000 条。
- 已关闭连接（`connections.ts`）上限 100 条，超限时从旧到新截断。

### 4.3 连接瞬时速率计算
`connections.ts` 通过 **快照差分法** 计算每条连接的瞬时速率：记录每次 WS 消息到时的 `(upload, download, timestamp)` 快照，下一帧按 `(uploadDiff + downloadDiff) / timeDiff` 得出速率。
- 首帧（或暂停恢复后）跳过归档，防止陈旧快照误判。
- 前后端断开瞬间自动归档为已关闭连接。

### 4.4 批量测速并发控制规约
- 无论是新版 Vue 3 还是旧版 Vanilla JS 前端，进行批量测速（全部测速或组测速）时，**必须强制实施并发控制（默认并发数限制为 10）**。绝对禁止一次性无限制发起数百个网络测速请求，防止浏览器连接队列拥堵与后端 Unix Socket 重试负载崩溃。

### 4.5 弹窗与交互去原生化
- 全站**绝对禁止使用**浏览器的阻塞式 `alert(...)`。如有提示需要，一律使用 `globalStore.showToast(text, 'success' | 'error' | 'warning' | 'info')` 发送非阻塞高颜值 Toast。
- 确认操作使用 `globalStore.showConfirm({title, message, confirmText?, cancelText?})` 返回 Promise，在 `App.vue` 中以模态渲染。
- 所有在页面上展示的图标**必须使用 `xicons` (@vicons/ionicons5)**，禁止在系统硬编码 SVG、Emoji 或颜文字符号（包括 `build/app/templates` 的内置配置模板中也绝对禁止在代理组和规则名中硬编码 Emoji），保持视觉绝对统一。

### 4.6 乐观更新与回滚
涉及高频用户操作的接口（如代理选择切换、规则启用/禁用、连接断开）采用**乐观更新**策略：
1. 立即更新本地状态，呈现即时反馈.
2. 并发发起 API 请求。
3. 请求失败时自动回滚至更新前的状态。

### 4.7 前端离线 Mock 联调机制
为了方便前端脱离 Go 后端独立运行与联调，`web/src/utils/mock.ts` 实现了完整的 HTTP API 与 WebSocket 数据流模拟器。
- **启用机制**：在 Vite 开发模式（`import.meta.env.DEV`）下默认开启，拦截网络请求并导入模拟数据。
- **手动控制**：可通过在控制台修改 `localStorage` 的键 `MOCK_BACKEND` 来强制覆盖：
  - 强制启用 Mock 模式：`localStorage.setItem('MOCK_BACKEND', 'true')`
  - 强制关闭 Mock（直接连接真实后端）：`localStorage.setItem('MOCK_BACKEND', 'false')`

### 4.8 缓存视图下的生命周期管理与滚动能效控制
在 KeepAlive 缓存组件后台化时，为了防止后台页面（如 `Logs.vue`）继续对增长的日志流进行无效的 DOM 渲染和滚动条计算，必须绑定 `onActivated` / `onDeactivated` 生命周期钩子来冻结/激活这些滚动计算（如 logs watcher），有效控制挂机闲置能耗。

### 4.9 复杂数据预解析与计算缓存
严禁在组件渲染周期内，或者在模板中高频调用包含重度计算或复制排序的操作（如 `sort` 历史延迟记录）。所有这类解析操作应收归至 Store 中，在拉取到最新数据的第一时间完成一轮预解析，并直接挂载为节点的响应式扁平字段（如 `latestDelay`、`recentColors`），组件仅负责以 O(1) 效率读取。

### 4.10 表单直连与双向绑定规范
表单交互输入（如端口设定）应直接通过 `v-model` 或 `v-model.number` 直连 Pinia Store 托管的数据对象。严禁声明冗余的局部 ref 变量并配合 watch-deep 进行繁重的手动双向映射赋值。当表单输入非法导致校验不通过时，可通过 Store 的 fetch 行为从后端重新拉取真实数据完成本地强制回滚。
