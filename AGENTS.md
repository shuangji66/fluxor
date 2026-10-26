# Fluxor 项目 AI 代理指南 (AGENTS.md)

本文件为后续接手的 AI 编码助手或开发者提供项目的系统架构、核心逻辑、通信接口及开发规约，以便于快速理解项目并进行无缝维护与扩展。

---

## 1. 项目定位与架构概述

`Fluxor` 是一个轻量级、无冗余的 Mihomo 内核管理面板与订阅生成系统。它采用**前后端不分离**的架构设计：
- **后端 (Go)**：位于 `backend/` 目录，以 Go 标准库为主，仅引入两个外部依赖：`gorilla/websocket`（通信）与 `gopkg.in/yaml.v3`（订阅/配置文件字段级校验）。新增依赖需有明确理由。后端托管在 Unix Socket (`/var/apps/Fluxor/target/app.sock`) 上，对外通过前端反向代理暴露，内嵌了前端的所有静态资源。
- **前端 (Vue 3 TypeScript 版)**：位于 `frontend/` 目录，由 Vue 3 (Composition API / Setup) + Vite + TailwindCSS + Pinia + TypeScript 构成，是项目唯一且主维护的前端实现。
  - **前端构建与内嵌**：`frontend/` 使用 Vite 构建，产物输出到 `frontend/dist/`（含 `index.html` 模板与 `assets/` 静态资源）。直接执行 `make` 会清理旧 dist、构建前端、同步到 `backend/dist/`，再由后端在 `backend/main.go` 中以 `//go:embed dist` 内嵌该产物。
  - **版本号**：不在前端注入。编译时经 `-ldflags -X fluxor/internal/buildinfo.Version=...` 写入后端（`make V=1.0.0`），前端经 `GET /app-version` 读取。
  - **开发环境编译**：任何新功能或漏洞修复请在 Vue 3 版本中维护，修改前端代码后在项目根目录执行 `make` 完成整体构建。

### 核心功能职责

1. **内核进程生命周期管理**：负责本地 Mihomo 二进制文件的启动、停止、状态查询及配置热重载（不中断长连接）。
2. **配置文件订阅与生成**：读取用户的订阅链接及自定义规则集，生成内核可运行的 `config.yaml`。
3. **通信中转代理 (Bridge)**：由于 Mihomo 运行在本地 Unix Socket 上，Fluxor 后端作为前端与本地内核之间的“双向桥梁”，代理所有的 HTTP API 请求与 WebSocket 数据流（流量、内存、连接、日志等）。**该链路不携带任何认证头**——内核对 Unix Socket 来源默认信任（见 3.10）；`panel_secret` 只对内核的 TCP 外部控制端口生效。

---

## 2. 目录结构与索引

```text
fluxor/
├── Makefile               # 统一构建入口：make（清 dist → 前端 → 同步 → 后端）、make clean、make V=x.y.z
├── backend/               # Go 后端源码目录（独立 Go module，root 位于 backend/go.mod）
│   ├── main.go            # 程序入口：参数/环境变量解析、路由注册、监听启动、优雅退出，
│   │                      #   并以 //go:embed dist 内嵌前端构建产物
│   ├── internal/          # 按功能域拆分的后端实现（详见「2.1 后端包结构」）
│   ├── go.mod / go.sum    # Go module 定义与依赖
│   └── dist/              # 构建产物目录（由 make 从 frontend/dist 同步，已 gitignore）
└── frontend/              # 主维护 Vue 3 前端源码目录
    ├── package.json       # Vue 3.5 + Pinia 4 + vue-i18n 11 + Vite 8 (Rolldown) + Tailwind CSS 4 + TypeScript 5.9 + @vicons/ionicons5
    ├── vite.config.js     # 构建输出到 dist/（index.html + assets/），集成 @tailwindcss/vite 插件
    ├── index.html         # HTML 入口（Go 模板：{{.BaseHref}} / {{.RawBase}}）
    └── src/
        ├── main.ts        # 挂载 Pinia + vue-i18n (Composition API, legacy:false)
        ├── App.vue        # 根组件：响应式侧边栏/移动端底部 Tab、亮暗/跟随系统主题、中英切换、Toast 队列、Promise 确认框
        ├── env.d.ts       # .vue 类型声明 & Window.BASE_URL 接口扩展
        ├── i18n.ts        # 全站国际化（zh/en），从 localStorage 读取语言偏好，禁止硬编码中文
        ├── index.css      # Tailwind v4 入口（@import/@source/@custom-variant/@theme/@utility，取代原 tailwind.config.js）
        │                  #   + CSS 变量亮暗主题（data-theme 选择器）+ 自定义滚动条
        ├── components/    # 公共及细粒度组件 (ProxyGroupCard, FormSwitch, CustomRulesDialog,
        │                  #   CustomNodeDialog（基础/传输层/TLS/高级四个分区 + 按 visible_when 条件显示）
        │                  #   + NodeFieldInput（含 map 与嵌套 group 递归渲染）+ FieldLabel（中文界面下给
        │                  #     纯中文配置标题补英文小字，仅该弹窗使用）：自定义模式的动态协议表单,
        │                  #   CustomNodeCard：自定义模式的节点卡片)
        ├── composables/   # 全局解耦组合式函数 (useTheme, useLanguage)
        ├── utils/
        │   ├── api.ts     # withBase()、apiFetch() HTTP 封装、wsConnect() WebSocket 封装、sseConnect() SSE 封装
        │   └── mock.ts    # 前端离线开发模拟器：拦截 HTTP/WS 请求提供 mock 数据，支持脱离后端独立测试
        ├── store/
        │   ├── global.ts   # 标签页激活状态、侧边栏折叠、亮暗/跟随系统主题、Toast 队列（3s 自动消失）、Promise 驱动确认框
        │   ├── config.ts   # 内核常规配置参数（allow-lan/ipv6/mode/log-level/tun/端口等，由 app.vue 统一订阅，与订阅解耦）
        │   ├── subscription.ts # 订阅管理 Pinia Store：订阅/节点配置 CRUD、解析及状态更新，由 config.ts 中拆分解耦而来
        │   │                   #   + 自定义模式：custom_nodes 列表、节点协议字段表（/subscribe/node-protocols 懒加载缓存）、
        │   │                   #     独立规则作用域（/subscribe/custom-mode-rules/custom）
        │   ├── overview.ts # 仪表盘实时统计（速度/流量/内存/连接数/版本/当前节点）、60 点流量历史、3 路 WS + 1 路 SSE
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
            └── Subscription.vue # 订阅中心：三模式（融合/切换/自定义）分段控件、代理/面板/TProxy 端口、密钥显隐切换、
                                 #   规则集档位（仅融合模式渲染）、UI 面板选择、「保存并应用」。
                                 #   融合/切换模式 = 订阅 CRUD 模态框 + 流量/健康度/有效期卡片 + 卡片选中；
                                 #   自定义模式 = 「节点列表」+「添加节点」弹窗（协议下拉 → 基础/传输层/TLS/高级四段动态渲染，
                                 #     默认值预填、按 network 条件显示传输选项块、嵌套块递归渲染），
                                 #     节点改动先落在本地列表，保存并应用时才落库并重生成配置。
                                 #   「自定义规则」弹窗（components/CustomRulesDialog.vue，多作用域 + 页签）：
                                 #   切换模式由订阅卡片按钮打开（作用域=该订阅，单页签）；
                                 #   融合模式与自定义模式由标题行「添加订阅/添加节点」左侧按钮打开
                                 #   （融合=base/full 两页签；自定义=独立作用域 custom 的单页签，规则与融合模式分开存放），
                                 #   并传入 hint 文案说明作用域与切换模式的入口（文案由父组件按模式给定，组件内不判模式）。
                                 #   同一个弹窗兼管「流量隧道」：类型下拉末尾的 tunnels 选项把表单切成隧道字段
                                 #   （监听类型下拉 / 两个地址输入框 / proxy 选择框），隧道列表独立成段排在规则列表下方，
                                 #   每条带启停开关与编辑/换序/删除；两个前端接口（endpoint / tunnelEndpoint）由父组件按模式传入
```

> **构建流程**：`make` → ① 清理旧 `frontend/dist` 与 `backend/dist`；② `npm run build` 输出到 `frontend/dist/`；③ 拷贝至 `backend/dist/`；④ 在 `backend/` 内 `go build -ldflags` （依赖 `//go:embed dist`，同时注入版本号）输出到项目根目录 `./fluxor`。版本号用 `make V=1.0.0` 指定，缺省 `1.0.0`（`make V=dev` 可产出不参与更新判断的调试版本）。

> **页面路由机制**：未使用 vue-router，通过 `globalStore.activeTab` 与 `<component :is="..." />` 动态组件切换视图。在此基础上，外层包裹了 `<KeepAlive :max="7">`（与视图总数一致，避免 LRU 淘汰引发的重挂）进行视图缓存，以长效留存页面各交互状态（如滚动进度与折叠状态）并规避切换页面时的重复连接请求。

### 2.1 后端包结构 (`backend/internal/`)

后端按**功能域**拆分为多个包，每个包内再按细分功能拆分为小文件。拆分原则：单一职责、依赖单向、禁止循环依赖。

```text
backend/
├── main.go                       # 唯一入口：参数/环境变量解析、路由注册、监听、信号处理、go:embed
└── internal/
    ├── config/                   # 【叶子】全局配置与持久化状态
    │   ├── doc.go                #   包说明
    │   ├── model.go              #   SubscribeConfig / Subscription / CustomRule（订阅级自定义规则）
    │   │                         #     + CustomNode（自定义模式的手工节点）+ CustomModeRules（自定义模式的自定义规则）
    │   │                         #     + 三种模式常量 ModeMerge/ModeSwitch/ModeCustom + 作用域常量 RuleScopeCustom
    │   ├── state.go              #   Current（当前配置快照）+ Mu（读写锁）
    │   ├── paths.go              #   全部运行路径（FluxorDataDir 统一运行数据目录 / Socket / 内核 / 面板等）
    │   ├── modes.go              #   fnos / openwrt 两套默认路径；SetDataDir 由 FluxorDataDir 派生
    │   │                         #     fluxor.log 与 6 个配置文件（旧 fluxor.json 仅供迁移读取；
    │   │                         #     PID 文件默认同址，openwrt 经 pidDirPinned 固定到 /var/run）；
    │   │                         #     fnos 默认取 TRIM_PKGVAR，未注入回退 /var/apps/Fluxor/var
    │   ├── name.go               #   订阅名校验与节点文件名清洗（防路径穿越）
    │   ├── nodes.go              #   CustomNode 名校验（非空/长度/无控制字符/重名）
    │   ├── rules.go              #   CustomRule 列表操作：同组内上/下移动、按生效顺序重排
    │   ├── tunnels.go            #   Tunnel 模型：网络类型归一化、同地址判重、列表内上/下移动、按作用域取列表
    │   ├── store.go              #   泛型 Store[T]：一个 JSON 文件 = 一把独立锁 + 一份内存态 + 一个写入者
    │   │                         #     （原子写盘 writeFileAtomic、损坏则备份并拒写）
    │   ├── storemodels.go        #   各文件的磁盘结构：Settings / SubscriptionRef / RulesFile /
    │   │                         #     TunnelsFile / MetaFile / SubscriptionMeta / TproxyFile / TunFile
    │   ├── stores.go             #   各文件唯一的读写入口（SaveSettings / UpdateSubscriptionRules /
    │   │                         #     UpdateTemplateRules / SaveSubscriptionMeta …）、视图组装
    │   │                         #     assembleCurrent、孤儿回收 GCResources、改名搬迁 renameResources
    │   └── migrate.go            #   旧单文件 fluxor.json → 多文件的一次性拆分（幂等、可回滚、损坏先备份）
    ├── configgen/                # 【基础层】config.yaml 模板 + YAML 结构化改写
    │   ├── generator.go          #   GenerateConfig / GenerateBaseConfig（写盘前归一化顶层键序：标量键在前、块在后，块序 dns→proxy-providers→proxy-groups→proxies→tunnels→rule-providers→rules）
    │   ├── template_base.go      #   基础字段骨架
    │   ├── template_dns.go       #   统一注入的 DNS 块
    │   ├── groups_lite.go        #   base 规则集的代理组
    │   ├── rules_lite.go         #   base 规则集的规则
    │   ├── groups_full.go        #   full 规则集的代理组（地区组用 include-all-providers 引用全部订阅 provider，模板内不含订阅名）
    │   ├── providers_full.go     #   full 规则集的 rule-providers
    │   ├── rules_full.go         #   full 规则集的规则（开头 6 条 DOMAIN 把概览页「代理出口 IP / 归属地」的查询域名指向 🚀 节点选择，避免被 networktest 规则集分流到直连）
    │   ├── customrules.go        #   自定义规则：校验 + 幂等注入 rules 序列（before/after 双锚点）
    │   ├── mergerules.go         #   规则上下文：融合档位（MergeRuleSetContext）与自定义模式（CustomModeRuleContext，目标额外含手工节点）
    │   ├── tunnels.go            #   流量隧道：校验（监听地址 host:port、目标支持域名、proxy 必须存在）
    │   │                         #     + 幂等注入 tunnels 块（写在 rule-providers/rules 之前；无效项跳过）
    │   ├── customconfig.go       #   GenerateCustomConfig：自定义模式产物 = 模板骨架 + dns + proxies 块 + 标准规则集
    │   └── doc.go                #   包说明
    ├── nodespec/                 # 【叶子】出站代理协议字段表（自定义模式的「默认模板」唯一来源）
    │   ├── spec.go               #   字段类型（string/text/int/bool/select/list/map/group）+ 分区与条件显示 + 默认值补齐、归一化（类型转换/必填/组合要求/未知键/剔除默认值）
    │   ├── fields.go             #   字段构造函数（str/secret/num/flag/sel/list/pairs/group/on/req/always）与分区包装（basicSection/transportSection/tlsSection/advancedSection）
    │   ├── blocks.go             #   可复用配置块：network + ws-opts/h2-opts/grpc-opts/http-opts/xhttp-opts/mkcp-opts，TLS 标量与 reality-opts/ech-opts/shadow-tls-opts/restls-opts/jls-opts
    │   ├── protocol_core.go      #   HTTP / SOCKS5 / Shadowsocks / SSR / Snell
    │   ├── protocol_v2ray.go     #   VMess / VLESS / Trojan / AnyTLS / SSH
    │   ├── protocol_quic.go      #   Mieru / Sudoku / Hysteria / Hysteria2 / TUIC / ShadowQUIC
    │   ├── protocol_tunnel.go    #   WireGuard / MASQUE / TrustTunnel / Tailscale / ZeroTier / EasyTier / OpenVPN
    │   └── doc.go                #   包说明：字段来源（mihomo v1.19.31 struct tag + wiki 的通用字段/TLS配置/传输层配置三页）与刻意不下的部分（tlsmirror/mekya、xhttp 调优项、smux/ip-stack/dialer-proxy 等）
    ├── httpx/                    # 【叶子】HTTP 通用工具
    │   ├── response.go           #   WriteJSONError / RespondJSON
    │   └── regex.go              #   外部面板后端地址校验正则
    ├── configcheck/              # 【叶子】Clash 配置校验与 YAML 文档操作（依赖 yaml.v3）
    │   ├── doc.go                #   包说明：provider 契约 vs 主配置契约
    │   ├── check.go              #   ValidateClashConfig：YAML 映射 + 顶层字段及类型校验
    │   ├── rulespec.go           #   规则类型白名单 + 载荷/目标校验 + 规则行组装 + RuleEnv
    │   └── document.go           #   Doc：保留键序/注释的顶层字段读写与序列化（含 NodeNames/MappingKeys/OrderTopLevel：顶层键序归一）
    ├── core/                     # 内核进程生命周期
    │   ├── client.go             #   CoreRequest + cancelableReadCloser（Context 回收）
    │   ├── lifecycle.go          #   启动/停止/热重载（含内核 PID 身份校验）
    │   ├── tmpcore.go            #   DownloadWithTempCore：临时内核下载订阅节点文件
    │   │                         #     + CleanupStaleTempCores：启动清理残留临时内核
    │   └── handlers.go           #   /core/* HTTP 接口
    ├── tproxy/                   # TProxy 防火墙与策略路由（IPv4 恒接管，IPv6 由开关控制）
    │   ├── state.go              #   启用状态、IPv6 接管开关、绕过列表缓存、读写锁
    │   │                         #     （读一律走 GetTproxyState / ipv6Enabled / proxyLocalEnabled）
    │   ├── store.go              #   开关状态/绕过列表（含预填模板）/本机流量接管/IPv6 接管开关的持久化
    │   │                         #     + ResetOnStartup：冷启动归零开关并清理残留规则
    │   ├── rules.go              #   规则解析 + nftables 应用/清理（IPv4/IPv6 同构：tproxyFamily）
    │   ├── rules_test.go         #   绕过规则解析/策略路由探测/家族定义的纯函数单测
    │   └── handlers.go           #   /config/tproxy* HTTP 接口
    ├── tun/                      # TUN 运行参数与启停（见 3.15）
    │   ├── settings.go           #   tun.json 稀疏持久化（只存与模板默认值不同的键；**不含 enable**）
    │   │                         #     + 取值归一化与白名单校验（stack / dns-mode / ipv6 / device）
    │   ├── host.go               #   HostIPv6Available：读 sysctl 判断新建 tun 接口能否挂 IPv6 地址
    │   └── handlers.go           #   /config/tun（参数）与 /config/tun/state（启停）
    ├── subscription/             # 订阅中心
    │   ├── doc.go                #   包说明
    │   ├── active.go             #   校验 active_subscription 是否为列表真实成员
    │   ├── api.go                #   /subscribe/config 读写
    │   ├── api_generate.go       #   /subscribe/generate（按 mode 分派到切换 / 融合 / 自定义三条生成链路）
    │   ├── api_generate_custom.go#   自定义模式的「保存并应用」：归一化节点 → 落库 → GenerateCustomConfig → 重载内核
    │   ├── api_update.go         #   /subscribe/update/{name}
    │   ├── nodesapi.go           #   /subscribe/node-protocols：下发协议字段表（前端动态表单的唯一来源）
    │   ├── customnodes.go        #   自定义节点归一化：名称/重名/与模板组名与内置目标冲突校验 + 补 ID
    │   ├── customrules.go        #   /subscribe/custom-rules/{name}：切换模式 订阅级自定义规则 读/增/改/排序/删
    │   ├── templaterules.go      #   模板级自定义规则的公共实现：ruleScope（作用域）抽象 + 增/改/排序/删/响应/生效同步
    │   ├── mergecustomrules.go   #   /subscribe/merge-custom-rules/{base|full}：融合模式档位级自定义规则（仅融合模式）
    │   ├── custommoderules.go    #   /subscribe/custom-mode-rules/custom：自定义模式自定义规则（独立存储，仅自定义模式）
    │   ├── tunnels.go            #   流量隧道三作用域入口（custom-tunnels / merge-custom-tunnels /
    │   │                         #     custom-mode-tunnels）+ tunnelScope 抽象 + 增/改/换序/删/启停
    │   ├── runtimeconfig.go      #   writeRuntimeConfig：订阅文件 → config.yaml 副本 + 自定义规则与隧道一趟叠加
    │   ├── patch.go              #   向节点文件注入端口/密钥/DNS（YAML 结构化改写；并归一化顶层键序）
    │   ├── ensure.go             #   切换模式下确保订阅文件就绪
    │   ├── update.go             #   切换模式下的单个订阅更新（锁内取快照 → 锁外下载 → 锁内写回）
    │   ├── metadata.go           #   元数据抓取与键规范化
    │   ├── timers.go             #   定时更新/健康检查定时器生命周期
    │   ├── healthcheck.go        #   switch 模式下激活订阅的周期测速
    │   ├── panel.go              #   MetaCubeXD config.js 后端地址改写
    │   ├── fileutil.go           #   文件复制工具
    │   └── download/             #   订阅下载层（注意：非叶子包，会调用 core.DownloadWithTempCore）
    │       ├── download.go       #     直连优先、临时内核回退（两阶段各 15s 上限，不重试）
    │       ├── direct.go         #     直接 HTTP 下载（仅收 Clash 明文 YAML）+ userinfo 头解析
    │       ├── validate.go       #     直连内容校验（委托 configcheck 做字段级校验）
    │       └── keys.go           #     map 键小写规范化
    ├── dashapi/                  # 内核 HTTP API 反向代理
    │   ├── dashboard.go          #   /version 版本接口
    │   ├── configs.go            #   配置读写、重载、GEO、缓存、DNS 查询
    │   ├── proxies.go            #   代理组、单节点测速、策略组测速
    │   ├── providers.go          #   订阅代理信息
    │   ├── rules.go              #   规则与规则提供商
    │   ├── connections.go        #   连接断开
    │   ├── pathparam.go          #   拼入内核路径的片段校验（防穿越）
    │   └── upgrade.go            #   内核升级
    ├── wsproxy/                  # WebSocket 双向桥接
    │   ├── handler.go            #   WsProxyHandler
    │   └── upgrader.go           #   全局 upgrader 配置
    ├── netinfo/                  # 网络信息查询
    │   ├── interfaces.go         #   网卡枚举 + 本地出口 IP 回退
    │   ├── iplookup.go           #   公网 IP 查询（JSON/纯文本兼容）+ newLookupClient
    │   ├── geo.go                #   IP 归属地查询（10 分钟缓存，条目数有上限）
    │   ├── proxyport.go          #   从内核配置读取实际代理端口
    │   ├── addr.go               #   TCP 监听地址校验
    │   └── handlers.go           #   /ipinfo/* 与 /interfaces
    ├── delaytest/                # 连通延迟测试
    │   ├── probe.go              #   HEAD 探测实现
    │   └── handlers.go           #   /delaytest/* HTTP 接口
    ├── quality/                  # 节点质量评分
    │   ├── model.go              #   评分模型与权重常量
    │   ├── calculator.go         #   延迟/稳定性/成功率三项算法
    │   └── handler.go            #   /proxies/quality
    ├── appupdate/                # 版本检查与自更新
    │   ├── github.go             #   Release/Tag 查询与版本比较
    │   ├── cache.go              #   版本信息缓存（TTL 10 分钟）
    │   ├── coreversion.go        #   内核本地/远程版本比对
    │   ├── selfupdate.go         #   Fluxor 自更新（优先 fluxor-<arch>.tar.gz 解压提取，回退裸二进制；
    │   │                         #     仅「自身代理 / 直连」两条链路各重试 2 次，失败提示检查代理或网络；
    │   │                         #     当前版本取自 buildinfo）
    │   └── handler.go            #   /check-update（当前版本取自 buildinfo，无需 ?current=；
    │                             #     ?force=1 = 弹窗手动检查，无视 10 分钟缓存冷却并续期）
    ├── logx/                     # 【叶子】统一日志出口（唯一日志文件 fluxor.log；见 3.13）
    │   ├── doc.go                #   包说明：唯一出口/记录格式/模块与等级语义/正文英文等约定
    │   └── logx.go               #   等级与模块常量、Setup/Close、Debug/Info/Warn/Error
    ├── buildinfo/                # 【叶子】构建期注入的版本号（-ldflags -X）
    │   ├── doc.go                #   包说明
    │   └── version.go            #   Version / Name() / IsKnown()
    └── web/                      # 前端入口渲染
        ├── index.go              #   index.html 模板渲染
        ├── whoami.go             #   /whoami
        └── version.go            #   /app-version（暴露 buildinfo 版本给前端）
```

**依赖方向（严格单向，不得出现环）**：实际图如下，可用 `go list -f '{{.ImportPath}} {{.Imports}}' ./...` 复核。

```text
main ──> 所有 internal 包

【叶子层】 config     （无 internal 依赖）
           logx       （无 internal 依赖；任何包都可依赖它输出日志）
           httpx      （无 internal 依赖）
           configcheck（无 internal 依赖，仅依赖 yaml.v3）
           buildinfo  （无 internal 依赖，版本号由 -ldflags 注入）
           nodespec   ──> config（协议字段表与取值归一化，不依赖其它 internal 包）

【基础层】 configgen  ──> config, configcheck, nodespec
           tproxy     ──> config, httpx
           web        ──> buildinfo, config
           wsproxy    ──> config

【核心层】 core       ──> config, configcheck, configgen, httpx, tproxy
           netinfo    ──> core, httpx

【业务层】 dashapi    ──> config, core, httpx, tproxy
           quality    ──> config, core, httpx
           delaytest  ──> netinfo, httpx
           appupdate  ──> buildinfo, config, httpx, netinfo
           subscription        ──> config, configcheck, configgen, core, dashapi, httpx, nodespec, subscription/download
           subscription/download ──> config, configcheck, core
```

> **循环依赖规避要点（拆分时实际采用的三个手段）**：
> 1. **下沉叶子包**：`core` 需要生成配置文件，而 `subscription` 依赖 `core`。若把生成逻辑留在 `subscription` 就会与 `core` 形成环，故将配置生成与改写逻辑抽成包 `configgen`（只依赖 `config` 与叶子包 `configcheck`），由 `core` 与 `subscription` 共同依赖。
> 2. **同层共用工具下沉**：`subscription` 与其子包 `subscription/download` 都需要 map 键规范化，若留在父包则子包反向依赖父包，故下沉为 `subscription/download/keys.go`（`NormalizeMapKeys`）；同理，`configgen`、`subscription`、`core` 三方都需要 YAML 校验与改写能力，一并下沉为叶子包 `configcheck`。
> 3. **依赖方向统一为单向链**：订阅相关的内核调用统一为 `subscription → core`；`core.DownloadWithTempCore` 只接收 `config.Subscription` 数据结构，不反向引用订阅包，因此不产生环。

> **新增后端代码时的约定**：先判断属于哪个功能域，再放入对应包；新增 HTTP Handler 放在该域的 `handlers.go` / `api*.go` 中；若新代码引入跨层调用，务必先确认不会形成依赖环——若会形成环，应把共用逻辑下沉为新的叶子包，或改为在同一方向上调用，切勿直接互相 import。


---

## 3. 前后端通信与代理机制 (重要规避点)

### 3.1 统一路由前缀 (BASE_URL)
所有的请求均有统一的基本路径前缀：`baseURL = "/app/Fluxor"`。
在 Vue 源码中，所有 `apiFetch` 或 WebSocket 通信必须调用 [api.ts](frontend/src/utils/api.ts)，它会自动且妥善地完成前缀拼接。

### 3.2 HTTP 代理流过早截断修复与 Context 释放
前端向后端发起管理请求时，后端通过 Unix Socket 拨号并发 Do(req) 请求内核。为了防止大 JSON 数据（例如代理组数据、连接历史）在传输中因超时 Context 被提前取消导致流被中断（抛出 `Unterminated string in JSON` 错误），后端在 [backend/internal/core/client.go](backend/internal/core/client.go) 中通过 `cancelableReadCloser` 实现了：
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
| `/` | GET | `web.HandleIndex` | SPA 主页模板渲染 |
| `/whoami` | GET | `web.HandleWhoAmI` | 读取 `X-Trim-Username` 头并原样返回（门户透传的用户名，**仅供显示欢迎语**；请求头由客户端自填，**不得用于鉴权或授权判断**） |
| `/app-version` | GET | `web.HandleAppVersion` | 获取 Fluxor 版本号（编译期注入，供前端展示与更新检查） |
| `/core/status` | GET | `core.HandleCoreStatus` | 内核运行状态（PID 文件检测）；仅供启停操作后确认与 SSE 降级兜底，启动阶段不再调用 |
| `/core/events` | SSE | `core.HandleCoreEvents` | 内核运行状态变更推送（替代前端轮询） |
| `/core/start` | POST | `core.HandleCoreStart` | 启动内核进程 |
| `/core/stop` | POST | `core.HandleCoreStop` | 停止内核进程（SIGTERM） |
| `/core/restart` | POST | `core.HandleCoreRestart` | 热重启（重载配置） |
| `/upgrade` | POST | `dashapi.HandleUpgrade` | 升级内核（透传内核 /upgrade） |
| `/core/check-update` | GET | `appupdate.HandleCoreCheckUpdate` | 检查内核是否有新版本（对比本地内核版本与 GitHub release） |
| `/subscribe/config` | GET/POST | `subscription.HandleSubscribeConfigAPI` | 订阅配置读写（持久化到 `settings.json`；只收设置类字段，规则/隧道/元数据由各自的接口维护） |
| `/subscribe/generate` | POST | `subscription.HandleGenerateConfig` | 保存配置 + 生成 config.yaml + 重载内核 |
| `/subscribe/update/{name}` | POST | `subscription.HandleSubscribeUpdate` | 手动更新指定订阅节点数据（只接受 POST，GET 返回 405） |
| `/subscribe/node-protocols` | GET | `subscription.HandleNodeProtocolsAPI` | 自定义模式可添加的协议与字段表（类型/默认值/可选值/必填/高级），前端据此渲染动态表单；字段表由后端 `nodespec` 单点维护 |
| `/subscribe/custom-rules/{name}` | GET/POST/PUT/PATCH/DELETE | `subscription.HandleCustomRulesAPI` | 切换模式下该订阅的自定义规则：查询 / 新增 / 修改（body 带 `id`）/ 排序（`{id,direction:up\|down}`）/ 删除（`?id=`）；所有写操作即时持久化，激活订阅改动后重写 config.yaml 并重载内核 |
| `/subscribe/merge-custom-rules/{ruleGroup}` | GET/POST/PUT/PATCH/DELETE | `subscription.HandleMergeCustomRulesAPI` | **仅融合模式**：按规则集档位（`base`/`full`）分开存放的模板级自定义规则，方法与语义同上；改动当前生效档位时重新生成 config.yaml 并重载内核 |
| `/subscribe/custom-mode-rules/{scope}` | GET/POST/PUT/PATCH/DELETE | `subscription.HandleCustomModeRulesAPI` | **仅自定义模式**：独立一份自定义规则（`custom_mode_rules`，不按档位分表），方法与语义同上；可选目标额外含手工节点名；规则恒生效，改动即重新生成 config.yaml 并重载内核 |
| `/subscribe/custom-tunnels/{name}` | GET/POST/PUT/PATCH/DELETE | `subscription.HandleSubscriptionTunnelsAPI` | 切换模式下该订阅的流量隧道（写进 config.yaml 的 `tunnels` 块）：查询 / 新增 / 修改（body 带 `id`，**启停开关也走它**）/ 排序（`{id,direction}`）/ 删除（`?id=`）；写操作即时持久化，激活订阅改动后重写 config.yaml 并重载内核 |
| `/subscribe/merge-custom-tunnels/{ruleGroup}` | GET/POST/PUT/PATCH/DELETE | `subscription.HandleMergeTunnelsAPI` | **仅融合模式**：按规则集档位（`base`/`full`）分开存放的流量隧道，方法与语义同上；改动当前生效档位时重新生成 config.yaml 并重载内核 |
| `/subscribe/custom-mode-tunnels/{scope}` | GET/POST/PUT/PATCH/DELETE | `subscription.HandleCustomModeTunnelsAPI` | **仅自定义模式**：独立一份流量隧道（`custom_mode_tunnels`），方法与语义同上；可选 proxy 额外含手工节点名 |
| `/traffic` | WS | `wsproxy.WsProxyHandler("/traffic")` | 实时流量数据 WebSocket 代理 |
| `/memory` | WS | `wsproxy.WsProxyHandler("/memory")` | 实时内存数据 WebSocket 代理 |
| `/logs` | WS | `wsproxy.WsProxyHandler("/logs")` | 实时日志流 WebSocket 代理 |
| `/connections` | WS/DELETE | `wsproxy.WsProxyHandler` / `dashapi.HandleConnectionsClose` | 连接实时流或全部断开 |
| `/connections/{id}` | DELETE | `dashapi.HandleConnectionsClose` | 断开指定连接 |
| `/version` | GET | `dashapi.HandleVersion` | 内核版本信息 |
| `/configs` | GET/PATCH/PUT | `dashapi.HandleConfigsAPI` | 获取/修改/重载内核配置 |
| `/configs/geo` | POST | `dashapi.HandleConfigsGeo` | 更新 GEO 数据库 |
| `/providers/geo` | POST | `dashapi.HandleProvidersGeo` | 回退 GEO 更新接口 |
| `/providers/rules` | GET | `dashapi.HandleRuleProviders` | 获取规则提供商列表 |
| `/providers/rules/{name}` | PUT | `dashapi.HandleUpdateRuleProvider` | 更新单个规则提供商 |
| `/interfaces` | GET | `netinfo.HandleInterfaces` | 返回系统物理接口名称（过滤回环及未启用接口） |
| `/providers/proxies` | GET | `dashapi.HandleProvidersProxiesAll` | 获取全部订阅的代理信息（含节点历史与代理组；融合模式的代理页数据源） |
| `/providers/proxies/{name}` | GET/PUT | `dashapi.HandleProviderProxies` | 获取/更新订阅代理信息；也承接 `/providers/proxies/{provider}/{node}/healthcheck` |
| `/rules` | GET | `dashapi.HandleRules` | 获取所有规则 |
| `/rules/disable` | PATCH | `dashapi.HandleRulesDisable` | 启用/禁用规则 |
| `/proxies` | GET | `dashapi.HandleProxies` | 获取所有代理组 |
| `/proxies/{name}/delay` | GET | `dashapi.HandleProxyDelay` | 测速（需 ?url=&timeout= 参数） |
| `/proxies/{name}` | PUT | `dashapi.HandleProxySwitch` | 切换代理选择 |
| `/proxies/quality` | GET | `quality.HandleQualityScores` | 获取所有代理节点的质量分数评分列表 |
| `/group/{name}/delay` | GET | `dashapi.HandleGroupDelay` | 策略组整体测速（返回 `{节点名: 延迟}`；需 ?url=&timeout=） |
| `/cache/fakeip/flush` | POST | `dashapi.HandleFlushFakeIP` | 清空 FakeIP 缓存 |
| `/cache/dns/flush` | POST | `dashapi.HandleFlushDNS` | 清空 DNS 缓存 |
| `/dns/query` | GET | `dashapi.HandleDNSQuery` | DNS 查询（?name=&type=） |
| `/restart` | POST | `dashapi.HandleRestart` | 内核远端重启 |
| `/check-update` | GET | `appupdate.HandleCheckUpdate` | 检查 Fluxor 自身是否有新版本（`?force=1` 无视 10 分钟冷却，供弹窗手动检查） |
| `/update-self` | POST | `appupdate.HandleSelfUpdate` | 下载并安装新版本（校验 checksums.txt 的 sha256 与 ELF 头，原子替换，失败回滚） |
| `/config/tproxy` | GET/POST | `tproxy.HandleTproxyState` | 获取或切换 TProxy 防火墙状态 |
| `/config/tproxy/exceptions` | GET/POST | `tproxy.HandleTproxyExceptions` | 获取或配置 TProxy 源/目的绕过列表（GET 一并返回 `defaults` 预填模板，供前端「恢复默认」） |
| `/config/tproxy/proxy-local` | GET/POST | `tproxy.HandleTproxyProxyLocal` | 获取或切换本机流量接管开关（界面文案「接管本机流量」） |
| `/config/tproxy/proxy-ipv6` | GET/POST | `tproxy.HandleTproxyProxyIPv6` | 获取或切换「接管 IPv6 流量」开关（默认关闭；启用 TProxy 期间前端置灰） |
| `/config/tun` | GET/POST | `tun.HandleTunSettings` | TUN 运行参数（stack / device / dns-mode / ipv6）：GET 返回生效值 + 模板默认值 + 宿主 IPv6 能力；POST 为部分更新，稀疏落盘 `tun.json`，**不碰内核、不碰 config.yaml**，下次开启 TUN 时生效 |
| `/config/tun/state` | POST | `tun.HandleTunState` | 开启/关闭 TUN（唯一入口）：后端组装完整 tun 对象 PATCH 内核；开关状态不落盘、**不回放** |
| `/ipinfo/local/v4` | GET | `netinfo.HandleLocalIPv4` | 查询本地出站 IPv4 归属信息 |
| `/ipinfo/local/v6` | GET | `netinfo.HandleLocalIPv6` | 查询本地出站 IPv6 归属信息 |
| `/ipinfo/proxy/v4` | GET | `netinfo.HandleProxyIPv4` | 查询代理出站 IPv4 归属信息 |
| `/ipinfo/proxy/v6` | GET | `netinfo.HandleProxyIPv6` | 查询代理出站 IPv6 归属信息 |
| `/delaytest/google` | GET | `delaytest.HandleDelayTestGoogle` | 测试 Google 连通延迟 |
| `/delaytest/youtube` | GET | `delaytest.HandleDelayTestYouTube` | 测试 YouTube 连通延迟 |
| `/delaytest/github` | GET | `delaytest.HandleDelayTestGitHub` | 测试 GitHub 连通延迟 |
| `/delaytest/baidu` | GET | `delaytest.HandleDelayTestBaidu` | 测试百度连通延迟 |
| `/delaytest/bilibili` | GET | `delaytest.HandleDelayTestBilibili` | 测试 Bilibili 连通延迟 |
| `/delaytest/custom` | GET | `delaytest.HandleDelayTestCustom` | 测试用户自定义地址连通延迟 |
| `/meta/` | GET | `http.FileServer` | MetaCubeXD 外部面板静态文件 |
| `/zash/` | GET | `http.FileServer` | Zashboard 外部面板静态文件 |
| `/assets/` | GET | `http.FileServer` | 内嵌前端构建静态资源（Vite assets 目录，文件名带内容哈希 → 一年期 immutable） |
| `/ICON.PNG` | GET | `httpx.ETagFile` | 内嵌 favicon；名字固定而内容随部署变化，故 `no-cache` + 内容摘要 ETag（命中 If-None-Match 回 304，避免每次导航重下 80 KB） |

> 所有路由均挂载在 `baseURL = "/app/Fluxor"` 之下，如 `/app/Fluxor/core/status`。

### 3.4 TProxy 防火墙安全操作与退避规约 (`backend/internal/tproxy/`)

为了保障系统网络安全，在调用系统防火墙（如 `nft`）时必须遵循以下规则：
1. **防止命令注入**：严禁采用拼接 shell 字符串并使用 `sh -c` 的方式执行。必须使用 `exec.Command` 原生多参数切片传参，并在后台通过 `runCmd` 限制外部参数注入（特别是针对绕过 IP/CIDR 等由用户表单输入的配置项）。
2. **退出彻底清退**：在面板退出时（通过监听 `syscall.SIGINT` 和 `syscall.SIGTERM` 信号），必须在退出前调用 `tproxy.DisableTProxyRules` 以清除所有已应用的网络重定向规则，以避免断网残留。
3. **冷启动收敛**：nft 规则不跨重启存活，而开关状态是持久化的。启动时必须调用 `tproxy.ResetOnStartup()`：把开关无条件归零并清除任何残留规则，让内存态、磁盘态与内核态三者一致。否则上次非优雅退出（kill -9 / 崩溃）会留下「面板显示关闭、流量仍被劫持」的静默错配。
4. **清理：各项独立探测、存在才删**：`DisableTProxyRules` 不得把策略路由的清理绑在「nft 表存在」的判定之后。`EnableTProxyRules` 先写策略路由（`ip rule` / `ip route`）再建 nft 表，若建表失败就会留下「有策略路由、无 nft 表」的状态；此时若因表不存在而提前返回，策略路由将永远清不掉，把流量导入空路由表 → 持续断网。正确做法是对 nft 表、`fwmark` 规则、`local` 路由**各自探测**（`hasNftTable` / `hasFwmarkRule` / `hasLocalRoute`），**有残留才执行对应删除**——既不漏删，也不对不存在的对象执行 del 而徒增错误。
5. **状态读写的单一入口**：读取 TProxy 开关状态一律走 `GetTproxyState()`，写入走 `SetTproxyEnabled()`（内存 + 持久化）。禁止直接访问 `tproxyEnableState`——无锁读会构成数据竞争。同理，「接管 IPv6 流量」与「接管本机流量」两个开关的读取分别走 `ipv6Enabled()` / `proxyLocalEnabled()`，禁止在 `rules.go` 里直接读变量。
6. **失败必须回滚且如实上报**：`EnableTProxyRules` 对每个已启用家族校验三件关键产物是否真实存在——nft 表、`fwmark` 策略路由、策略路由表里的 `local` 路由（缺策略路由会把被标记流量导入黑洞，因此不能只看 nft 表），任一缺失即返回 error；Handler 在失败时回滚开关状态并返回错误，绝不回报 `enabled: true`。同理，改 `tproxy-port` 时只有在开关处于启用态才能重建规则，否则会在开关为「关闭」时被静默装上系统级透明代理规则。
7. **IPv6 家族还要过「IPv6 支持」总开关**：是否下发 ip6 规则由 `effectiveIPv6Takeover(ipv6Enabled(), globalIPv6Enabled())` 决定——用户偏好（`tproxy.json`）与订阅中心的生成期开关必须同时为真。总开关关闭时**只跳过、不改偏好**，恢复后下一次重建规则自动带上 v6；开关开着但总开关关闭时记一条 WARN，避免「界面显示已接管、实际没下发」。
8. **IPv4 / IPv6 是同构的两套规则，必须同生共死**：两套规则由 `tproxyFamily` 参数化（家族/表名/地址关键字/集合类型/`ip` 家族参数/默认路由/绕过网段），启用时按 `ipv6Enabled()` 裁剪，**清理时必须两个家族都探测**（不按开关裁剪，否则关掉开关后再也清不掉上一次遗留的 ip6 规则）。IPv6 绕过网段必须含 `ff00::/8`——DHCPv6 的 UDP 547 目的地址是 `ff02::1:2`，漏掉会把它劫持。绕过条目按家族分流下发（网段绕过进对应家族，端口绕过两个家族都下发），未开启 IPv6 时 IPv6 绕过条目必须逐条记日志说明「未下发」，不得静默丢弃。`ip6` 家族 nat 链（NAT66）在老内核/老 nft 上可能不可用，此时只跳过该家族的 DNS 重定向并记日志，不让整次启用失败。

### 3.5 配置文件布局与写入规约（多文件，取代旧的单文件 `fluxor.json`）

历史上全部持久化状态挤在一个 `fluxor.json` 里，由 `config` 与 `tproxy` 两个包各自「读整文件—改—写整文件」，共用一把全局锁。由此产生四个问题：**写放大**（改一条规则要重写整份，含所有订阅的机场元数据与预填模板）、**全量阻塞**（一把锁串行化两个功能域）、**损坏半径 100%**（解析失败时以空 map 为基底继续写，会把别人的字段一并清空）、**非原子写盘**（裸 `os.WriteFile` 会留下半截文件）。

现在按**写入者**拆成 6 个文件（都在 `FLUXOR_DATA_DIR` 下，路径由 `config.SetDataDir` 派生；`tun.json` 由 `tun` 包自持）：

| 文件 | 内容 | 唯一写入者 | 写入 API |
|------|------|-----------|----------|
| `settings.json` | 全局设置 + 订阅注册表（name/url/interval/prefix）+ 手工节点 | 订阅配置接口（保存并应用、订阅增删改、手工节点编辑） | `config.SaveSettings(view)` |
| `rules.json` | 三作用域自定义规则（融合档位 `merge` / 自定义模式 `custom_mode` / 切换模式按订阅 `subscriptions`） | 自定义规则接口 | `config.UpdateSubscriptionRules` / `config.UpdateTemplateRules` |
| `tunnels.json` | 三作用域流量隧道（结构同 rules.json） | 流量隧道接口 | `config.UpdateSubscriptionTunnels` / `config.UpdateTemplateTunnels` |
| `subscription-meta.json` | 每订阅 `updated_at` + `subscription_info`（机场元数据） | 更新与元数据流程（手动/定时更新、启动 ensure） | `config.SaveSubscriptionMeta` |
| `tproxy.json` | TProxy 三个开关 + 两条绕过列表 | `tproxy` 包（用自己的 `config.Store`） | `tproxy.SaveTproxy*` |
| `tun.json` | TUN 运行参数（stack / device / dns-mode / ipv6），**稀疏**（只存与 `configgen.TunBlock` 不同的键）；**不含 enable** | `tun` 包（用自己的 `config.Store`） | `tun.Save` |

旧的 `fluxor.json` 只作为**一次性迁移的输入**：启动时 `config.MigrateLegacyFiles()` 把它拆成上述五个由旧文件派生的文件（`tun.json` 没有历史数据，不参与迁移）并改名为 `fluxor.json.migrated-<时间戳>`。幂等判据不是「settings.json 是否存在」（它是五份里第一个被写的，从第二个起失败就会让整段迁移被永久跳过、未迁移的类别再也读不到），而是一个显式的 `fluxor.json.migrating` 标记：写文件前落标记，五份全部写成才归档旧文件并删标记；中途失败则下次启动读到标记后**只补写缺失的那几份**（已写成的保持原样，不覆盖用户其后的改动）。新布局已存在且无标记时旧文件被忽略（不会出现两套真相）。

实现载体是 `config.Store[T]`（`internal/config/store.go`）。写代码时必须遵守：

1. **一个文件一个写入者**：新增持久化数据前先问「谁会写它」。同一个字段有第二个写入者，就必须改走该字段所属 store 的 API，而不是「顺手也写一下」。
2. **锁内「读—改—写」**：规则/隧道的修改一律用 `config.UpdateXxx(…, func(current []T) ([]T, error))`——闭包拿到的 `current` 是**此刻**存储里的列表，返回的列表被原样落盘，整段在同一把文件锁内完成。**禁止**「先 `config.SubscriptionRules(name)` 取快照 → 在闭包外算好 → 整份写回」：那是丢失更新的经典形态（同一作用域的并发请求会互相覆盖）。
3. **`config.Current` 是派生视图**：由各 store 组装（`assembleCurrent`），写入口落盘后会自动重组装。**任何代码都不得给 `config.Current` 赋值或就地改它的切片**（`Current` 只在读路径使用）。需要跨数据类别的读，一律走视图或 getter（`TemplateRulesFor` / `SubscriptionTunnelsFor` / `SubscriptionMetaOf` …）。
4. **不得嵌套两把锁**：写入口内部会取 `config.Mu` 重组装，因此**持有 `config.Mu` 时不得调用任何 `Save*/Update*`**；也不得在 `Store.View` 回调里调用该 store 的 `Update`（`sync.Mutex` 不可重入，会自死锁——本项目踩过一次）。
5. **原子落盘与损坏保护**：写盘统一走 `writeFileAtomic`（临时文件 + `fsync(tmp)` + `rename` + **`fsync(父目录)`**，显式 0644）。最后一步不能省：`rename` 改的是父目录的目录项，只 `fsync` 文件不 `fsync` 目录时，断电后 `rename` 可能整个丢失，或在 ext4 这类延迟分配的文件系统上留下「长度正确、内容全零」的文件——对配置来说就是「面板起来是默认值」。目录同步失败只记 WARN 不判失败（部分平台/文件系统不支持对目录 Sync，为此让保存失败得不偿失）。文件内容无法解析时：备份为 `<file>.corrupt-<时间戳>`、内存态回落默认值、**拒绝后续写入**（返回「内容损坏」错误让接口如实回 500），而不是以空值继续——静默继续会覆盖掉用户手工编辑的内容。
6. **默认值不落盘**：只在用户真正改过时才写文件。标量默认值靠 `Store.Init`（读盘前填充，键存在即被覆盖；端口 0 表示禁用，只有 `Init` 能区分「缺失」与「显式 0」）；容器类靠 `Store.Normalize`（读盘后修 `null`）。tproxy 的两条绕过列表用 `nil` 表示「用代码里的预填模板」，`LoadTproxyState` 会把与模板一致的列表从文件里移除——预填的上万字节注释不该进用户文件。
7. **按订阅名存的数据要能自愈**：`rules.json` / `tunnels.json` / `subscription-meta.json` 里的条目以订阅名为键。订阅**改名**时由 `SaveSettings` 识别（判据是「旧表独有 × 新表独有且 URL 相同」）并搬运；订阅**删除**后的孤儿由 `GCResources()` 回收（**只在 `SaveSettings` 之后**——那时 settings 一定刚被成功写入，订阅表是可信的），且只在确有删除时才落盘。**启动阶段只探测不回收**（`probeOrphanResources`）：那时 settings.json 可能解析失败或读不出来，内存态已回落成空订阅表，若照常回收会把 rules / tunnels / meta 三份文件里所有按订阅名存放的条目判成孤儿并删除（这三份没有 `.corrupt` 备份，一次解析失败就能毁掉用户全部规则与隧道）。回收本身只是「让文件干净」而非正确性要求，推迟到下一次保存完全无副作用。取值判据见 `subscriptionNamesForGC()`：settings store 处于损坏状态时一律拒绝回收。孤儿进不了生成链路（生成按订阅名查找），因此回收是清理而非正确性要求——正因如此，拆文件才不需要跨文件事务。前端在「改名的同时修改链接且该订阅确有规则/隧道」时会先弹一次确认（`subscription.rename_url_change_confirm`）——这种组合无法被识别为改名，旧条目会被当作孤儿回收。
8. **不变量留在同一文件内**：唯一需要原子的跨字段约束是「`active_subscription` 必须是 `subscriptions` 的成员」，两者同在 `settings.json`。新增字段时若发现需要跨文件原子性，先重新划分归属，而不是引入跨文件事务。
9. **元数据是唯一允许「写失败即留空」的一类**：`subscription-meta.json` 的内容可从机场重抓，因此更新流程里抓取失败**不落库**（保持 store 里的旧值），只在本地视图沿用旧值以保证本次生成/响应一致。
   同理，**抓到但内容为空**（内核 provider 还没重新拉取、或机场不下发 `subscription-userinfo`）也不算成功：`SaveSubscriptionMeta(name, "", nil/空 map)` 既不写也不删，`updateAllSubscriptionsMetadata` / 异步更新分支据此保留旧值并记一条 WARN。把它当成「清空」会抹掉用户上次已知的流量与到期——切到融合模式后卡片会立刻变成「流量信息不可用」，而这正是最需要旧值兜底的时刻（前端在实时数据缺失时回退显示 `sub.info`，见 `getSubscriptionDisplayInfo`）。

### 3.6 定时器生命周期规约

定时器（尤其 `subscription` 的健康检查/更新定时器）必须遵守：

1. **goroutine 只能捕获局部变量**：绝不能让常驻 goroutine 读取会被置 `nil` 的包级句柄。`select` 每轮都会重新求值 `case` 表达式，一旦读到 `nil *time.Ticker` 就是对 nil 解引用；该 goroutine 没有 `recover`，会**直接终止整个进程**。正确做法是把 `ticker` / `stop` 先落到局部变量再由闭包按值捕获。
2. **停止必须幂等**：重复 stop 不得 `close` 已关闭的 channel。启停整体由专用互斥锁（`healthCheckLifecycleMu`）串行化。
3. **禁止在持锁期间启动定时器**：`StartAllTimers` 必须先取快照再释放 `config.Mu`。Go 的 `RWMutex` 在有写者排队时会阻塞新的读锁，因此「持读锁 → 调用内部会再次取读锁的函数」会形成死锁（内层等写者、写者等外层）。
4. **向可能被置 nil 的 map 写入前要判空**：`performHealthChecks` 执行期间定时器可能已被 stop（`lastHealthCheck` 被置 nil），写入前必须复查。

### 3.7 订阅下载的超时与重试规约

订阅下载链路为「直连 HTTP → 失败则回退临时内核」两阶段，统一遵守：

1. **各阶段超时均为 15 秒**：`download/direct.go` 的 `directDownloadTimeout` 与 `core/tmpcore.go` 的 `tempCoreTimeout` 都是 15s。
2. **一律不重试**：直连失败只请求一次；临时内核失败也只启动一次。不得在下载路径重新引入重试循环——此前为「60s × 3 次重试」，单次更新最长可拖到 3 分钟。
3. **15 秒是端到端上限**：临时内核路径中，「等待文件产出」与「抓取元数据」共用一个截止时间（`deadline`），元数据请求取剩余预算作为超时，不额外叠加。
4. **进程回收必须有界**：清理临时内核时的 `cmd.Wait()` 等待必须加上限。它的 stdout 拷贝 goroutine 可能因子进程继续持有管道写端而长期不返回，无界等待会突破 15s 上限甚至永久阻塞。

> 两阶段的 15 秒是**串行**的：最坏情况（直连超时后再回退临时内核并超时）单次更新约 30 秒。这是刻意的取舍——回退机制本身要保留，但每一段都不再各自放大 3 倍。

### 3.8 内核状态推送（SSE）与轮询替代

内核运行状态（`/core/status`）**不再轮询**，改由后端经 SSE 主动推送（`GET /core/events`，实现在 `core/events.go`）。要点：

1. **两条变更来源都必须广播**：本进程发起的启停操作，以及内核**自行退出**（崩溃 / 被外部 `kill -9` / OOM）。后者由 `StartCore` 中等待子进程的 goroutine 在 `cmd.Wait()` 返回后调用 `PublishCoreState(false)` 捕获——这是轮询原本承担的主要职责，漏掉会导致前端永远显示「运行中」。
2. **事件模型是「最新状态即真相」**：hub 不排队历史事件，只保存当前状态；新订阅者接入时立即收到一次快照，之后仅在状态**真正变化**时推送（`publish` 内部比对，重复调用不产生重复事件）。
3. **锁的职责**：`pubMu` 串行化发布以保证事件顺序，`mu` 只保护状态字段与订阅者集合。发布路径**不做任何网络 IO**（事件只承载状态，不夹带版本）。
4. **慢订阅者丢弃而非阻塞**：投递用非阻塞 `select`，通道满则丢该条；前端重连或下次状态变化时会重新校正，且 `GET /core/status` 始终可用作兜底。
5. **SSE 必须抗代理缓冲**：响应设置 `X-Accel-Buffering: no`，并每 25 秒发送 `: ping` 注释心跳，避免中间反代因空闲断开或缓冲。
6. **状态走 SSE，版本走 `/version`**：`/core/events` **只推运行状态**，不承载内核版本——版本是内核原生 API `/version` 的职责，由前端在状态变为「运行中」时按需请求。前端启动时**不再**预请求 `/core/status`（接入快照已含状态）。因此 hub 的「已确定状态」是硬性前提——`main.go` 在自动启动内核后**无条件**调用一次 `PublishCoreState(core.IsCoreRunning())`。若漏掉：内核已在运行时不经过 `StartCore`、或内核未运行，`known` 均为 false，新订阅者将收不到任何快照。
7. **前端必须有兜底**：SSE 可能被中间反代缓冲/剥离。`overview.ts` 起一个 5 秒看门狗——超时未收到任何事件（含接入快照）才降级请求一次 `/core/status`；正常收到事件立即清除，**不产生额外请求**。内核版本由 `ensureCoreVersion()` 在「运行中」且尚未取得时请求一次 `/version`（取到即不再重复）。浏览器 `EventSource` 自带重连，前端**不要**再叠加握手超时（与 `wsConnect` 不同）。

> `/core/status` 保留：供内核启停/重启/升级操作后由前端主动确认，以及上述 SSE 降级兜底。启动阶段不调用。

### 3.9 自定义规则注入规约（`configgen` + `subscription`）

自定义规则有三种作用域（切换=订阅、融合=档位、自定义=独立字段），注入链路不同，但共用同一套校验与幂等注入实现。**流量隧道（`tunnels` 顶层块）用的是同一套作用域划分与同一套事务顺序**，差异见 3.12。

| 模式 | 作用域（存放位置） | 生效条件 | 注入点 |
|------|--------------------|----------|--------|
| 切换 | `rules.json` → `subscriptions.<订阅名>` | 该订阅是 `active_subscription` | `writeRuntimeConfig`：订阅文件 → config.yaml 副本 → 叠加 |
| 融合 | `rules.json` → `merge.<base\|full>` | 该档位是 `rule_group` | `GenerateConfig`：`appendRuleSet` 之后、写盘之前注入 |
| 自定义 | `rules.json` → `custom_mode`（不按档位分表） | 恒生效（该模式固定用标准规则集） | `GenerateCustomConfig`：`appendRuleSet(base)` 之后、写盘之前注入 |

接口入口同样按模式分开：切换=`/subscribe/custom-rules/{name}`、融合=`/subscribe/merge-custom-rules/{base\|full}`、自定义=`/subscribe/custom-mode-rules/custom`；每个入口只服务自己那一种模式，跨模式访问统一回 400 并指明去哪儿改。「跨模式」判定针对**接口的全部方法**（含 GET）——只拦写操作会出现「查询能打开、第一次写才被拒」的不一致。错误提示由 `modeMismatchHint` 补上「当前生效模式：X；若刚在界面上改过模式，请先点击「保存并应用」」：界面上的模式选择器允许改动而不保存，用户看到一句「仅在融合模式下可用」时最可能的真实原因是模式还没生效（实测反馈过）。前端另有一道闸门：`store/subscription.ts` 的 `savedMode` 记录已生效模式，模式未生效时点击规则/隧道入口直接给出上述指引，不发请求（`subscription.mode_unsaved_hint`）。

**三种作用域互不影响**：各存各的、各有各的入口与校验集合，写一边绝不会出现在另一边。公共流程（增/改/排序/删 → 持久化 → 按需重新生成 → 热重载）由 `subscription/templaterules.go` 的 `ruleScope` 抽象承载，融合档位与自定义模式只是它的两个实例——两份拷贝迟早会在某次改动后不一致。

切换模式的 `config.yaml` 是「订阅文件副本」，自定义规则只能在**复制之后**叠加，必须遵守：

1. **订阅文件保持原样**：`proxies/<订阅名>.yaml` 始终等于「机场下发内容 + Fluxor 必需字段（端口/密钥/DNS）」，自定义规则只写入 `config.yaml`。因此注入点是 `writeRuntimeConfig`（`subscription/runtimeconfig.go`），而不是 `patchSubscriptionFile`——后者会作用到 `ensure.go` 里**所有订阅**（含未激活的纯备份文件）。
2. **只增不覆盖**：`rules` 是「顺序即语义」的序列，任何 `doc.Set("rules", ...)` 式的整键覆盖都会吃掉机场自带规则。`configgen.ApplyCustomRules` 只做节点级插入：`before`（默认）插到首位、`after` 插到最后一条 `MATCH` 之前；`rules` 缺失或为 null 时新建序列，`rules` 不是序列时报错并放弃改写。
   同一分组内按切片顺序依次插入，因此**切片顺序 = 界面展示顺序 = 生效顺序**：所有写操作（新增/修改/排序）落盘前都会经 `config.SortCustomRulesForDisplay` 归一化为「before 组在前、after 组在后」，避免三者出现分歧。
3. **必须幂等**：订阅每次更新（含定时更新）都会重放一次注入，因此要先按规则文本去重再插入，重复调用产物必须逐字节一致。
4. **目标解析失败必须跳过而不是写进去**：内核遇到无法解析的目标会拒绝加载**整份**配置（实测 `rules[0] [DOMAIN,x.com,G] error: proxy [G] not found`）。机场更新后代理组改名属常态，此时静默跳过该条（日志 + 返回 `ApplyResult.Skipped`，前端在列表中标注原因）远优于让配置整体不可用。同理，`RULE-SET` 的取值必须存在于该订阅的 `rule-providers`。
5. **规则类型走白名单**：`configcheck/rulespec.go` 的清单以实测 `-t` 通过为准（该内核版本不支持 `PROTOCOL`），只收录「单载荷 + 单目标」类型；`AND/OR/NOT/SUB-RULE`（需嵌套语法）与 `MATCH`（会截断其后全部规则）不开放给表单。
6. **合法目标随模式而异，且「可选目标」与「校验目标」不是同一集合**：前端目标下拉列**代理组**——切换模式取自订阅文件的 `proxy-groups`，融合/自定义模式取自**该模板**的代理组（`configgen.MergeRuleSetEnv` 直接解析生成用的同一批模板常量，模板一改、界面与校验自动跟随，不另维护清单）；**节点**只在「配置里有内联 `proxies`」的两种模式下可选（下表按模式区分）。
   - **自定义模式**（`configgen.CustomModeRuleContext`）：手工节点写死在 `config.yaml` 的 `proxies` 里，规则指向节点名内核能解析，因此节点名**既是可选目标也是校验目标**，在目标下拉里单列一个「节点」分组（接口 `nodes` 字段，来自已保存的 `CustomNodes`，新增节点要先「保存并应用」）。节点改名后旧目标会**留在下拉里**（`ruleTargetExtra`：不自动改写历史目标，用户也可能把名字改回去），但表单里选着它时必须在下方给出红字提示（`custom_rule_target_unknown`）——否则用户只会反复撞后端的 400 而不知道该改哪里；
   - **融合模式**（`configgen.MergeRuleSetContext`）：节点来自 `proxy-providers`、运行时才加载，**内核在配置校验阶段尚未展开这些节点**——实测即便 provider 文件已在本地、节点名确实存在，`-t` 仍报 `rules[0] [DOMAIN,x.example,NODE1] error: proxy [NODE1] not found` 并拒绝整份配置。因此节点名既不进下拉也不算合法目标（`Nodes` 恒为空）；用户确实要按节点分流时应改用切换模式或写 `RULE-SET`/代理组。
   - **切换模式**：节点名**既进下拉（单独成组）也参与校验**——订阅文件就是 `config.yaml` 的副本，节点是内联静态 `proxies`，实测 `DOMAIN,x.com,节点A` 可通过内核 `-t`；此前只保留在校验集合里（怕用户选了节点而内核不认），实测证明该顾虑只对 provider 成立，故补上下拉项（`RuleContext.Nodes` 由 `LoadRuleContext` 从订阅文件的 `proxies` 填充）。
7. **融合模式两档必须分开存放与生效**：`base` 与 `full` 的代理组、规则集、内置规则都不同（`base` 没有 `rule-providers`，因此该档位下 `RULE-SET` 不可用），同一份列表放在两档下必然有一半规则指向不存在的目标。生成时只取 `cfg.MergeCustomRulesFor(cfg.RuleGroup)` 那一份——另一档保持惰性，等切档后再生效。判重也要带上该档位的内置模板规则（`MergeRuleSetRuleLines`），否则自定义规则与模板同形时会被幂等注入静默跳过。
8. **排序只在同插入位置分组内进行**：`before` 与 `after` 在 `config.yaml` 中的落点相差甚远（最前 vs MATCH 之前），跨组交换会让「界面顺序」与「生效顺序」不一致，因此 `config.MoveCustomRule` 只在同组内与相邻规则交换，到边界时返回 `moved=false`（接口回 400，而不是假装成功）。
9. **写盘顺序**：`writeRuntimeConfig` 是「复制 → 解析 → 注入 → 写回」，注入失败不落盘，避免留下内核加载不了的半成品 `config.yaml`；无自定义规则时完全跳过读写，保持副本的逐字节一致。

10. **规则与隧道不在「整份配置覆盖写」的作用域内**：`/subscribe/generate` 与 `/subscribe/config` 的请求体由前端把上次读到的配置铺开拼成（`{...currentConfig}`），因此必然夹带（可能是过期的）规则与隧道字段。拆分之后这两个接口**根本看不到**它们——规则/隧道只存在 `rules.json` / `tunnels.json`，`config.SaveSettings` 只取全局标量、订阅身份与拉取参数、手工节点；视图里的 `custom_rules` / `tunnels` / `updated_at` / `subscription_info` 一律被忽略。

    这是旧 `config.AdoptServerOwnedFields`（「以服务端为准」的深拷贝合并）的**替代**，而不是它的简化版：以前的判据是「键是否出现」，挡得住漏带却挡不住过期（实测过「刚删掉的规则在保存并应用后复活」「刚新增的规则被旧列表覆盖」）；现在「过期快照」与「服务端状态」根本不在同一个存储里，覆盖路径不存在。

    代价是契约收紧：**新建订阅时不能顺带提交规则/隧道**（以前靠「服务端不认识的新订阅名保留请求体内容」兜着）。规则与隧道只能在订阅/作用域存在之后，通过各自的专用接口添加——前端本来就是这么用的（规则弹窗只对已存在的订阅或模板作用域开放）。

    前端对应改成**显式组装请求体**：`store/subscription.ts` 的 `buildSettingsPayload()` 只挑设置类字段（全局参数 + 订阅注册表 + 手工节点），不再用 `{...currentConfig}` 整份铺开。这样契约在两端口径一致，也不会把（可能很大的）机场元数据随每次保存来回搬运。

> 规则写操作（增/改/排序/删）的事务顺序统一为：`config.UpdateSubscriptionRules` / `config.UpdateTemplateRules`（**锁内读—改—写** rules.json + 自动重组装 `Current`）→ 若命中的作用域当前生效（切换：激活订阅；融合：当前档位；自定义：恒为标准档位）则同步运行配置（切换走 `writeRuntimeConfig`，融合走 `configgen.GenerateConfig`，自定义走 `configgen.GenerateCustomConfig`）+ `ReloadCore()`。内核未运行时只更新 `config.yaml`（下次启动生效），并把「已保存但未同步」的情况作为 warning 如实回给前端。
> 「修改」是就地替换（保持列表位置），「排序」是同组内相邻交换（`config/rules.go` 的 `MoveCustomRule`），两者都不改变其他规则的相对顺序。

### 3.10 内核链路不携带认证头（Unix Socket 免密钥）

后端与内核之间的两条链路——`core.CoreRequest`（HTTP）与 `wsproxy.WsProxyHandler`（WebSocket）——**都不得设置 `Authorization` / `Bearer` 头**：

1. **原因**：内核对 `external-controller-unix` 来源默认信任，其鉴权中间件只在**非 unix** 监听上校验 `secret`。实测：同一内核进程，unix socket 上不带密钥、带错误密钥、带正确密钥三者均返回 200；TCP 外部控制端口不带密钥返回 401。因此给 unix 链路加头是纯粹的无效代码——它既不会被校验，也会让「密钥保护了内部链路」的错误印象留在文档里。
2. **`panel_secret` 的真实作用域**：只写进 `config.yaml` 的 `secret` 字段，保护内核 **TCP** 外部控制端口（`external-controller: 0.0.0.0:<panel_port>`，供 MetaCubeXD / Zashboard 等外置面板直连）。密钥不下发给浏览器，前端一律经后端中转。
3. **安全边界依赖文件系统权限**：unix socket 的访问控制由 socket 文件权限（及其父目录）承担，不依赖密钥。新写内核客户端代码时不要「顺手补一个认证头」。

> 历史遗留：`core/client.go` 与 `wsproxy/handler.go` 曾各自读取 `config.Current.PanelSecret` 并附加 `Bearer` 头，已按要求全部移除。

### 3.11 自定义模式（手工节点）生成规约（`nodespec` + `configgen` + `subscription`）

第三种模式：完全不使用订阅，节点由用户在「节点列表」里手工添加，配置由**模板骨架 + dns + 标准规则集 + 手工节点**拼出。涉及四个包：

| 关注点 | 归属 | 说明 |
|--------|------|------|
| 协议字段表与默认值 | `nodespec`（叶子包） | 每个协议的字段（键/类型/默认值/可选值/是否必填/所属分区/条件显示/嵌套子字段）与取值归一化，是「后端给协议加默认模板」的唯一入口 |
| 持久化模型 | `config.CustomNode` | `{id, name, type, config}`；`config` **只存与协议默认值不同的字段** |
| 生成产物 | `configgen.GenerateCustomConfig` | 模板骨架 + `applyFluxorFields` + `proxies` 块 + 标准规则集 + dns，最后 `writeConfigTarget` 归一化键序 |
| 接口与校验 | `subscription`（`customnodes.go` / `nodesapi.go` / `api_generate_custom.go`） | 下发字段表、归一化节点、按模式分派生成并重载 |

关键约定：

1. **默认值只存在于后端**：前端不维护协议清单，`GET /subscribe/node-protocols` 下发字段表，`CustomNodeDialog.vue` 按声明渲染动态表单（第一项选协议，随后按「基础 → 传输层 → TLS → 高级」四段渲染，后三段可折叠）。新增协议或调整默认值**只改 `nodespec`**，不发前端版本。字段中文名走 i18n 的 `subscription.node_field.<key>`；同一键在不同协议下含义不同时用 `subscription.node_field.<协议>.<键>` 覆盖（如 vmess 的 `network` 是「传输方式」，ZeroTier 的是「网络 ID」）；再没有就回落到后端下发的英文 label。

1.05 **协议级提示也由后端声明**：协议表上有两个纯界面标记，都**不参与归一化与生成**——要新增/调整这类提示只改 `nodespec`，前端不硬编码协议名。
   - `Protocol.Deprecated`（目前只有 ShadowsocksR）：红字提示「该协议已过时，不建议使用」。内核仍支持 SSR，用户既有节点与机场仍在用的协议不该因为一句「过时」就存不进去（回归用例 `TestDeprecatedProtocolStillAccepted`）。
   - `Protocol.Tunnel`（`protocol_tunnel.go` 里那 7 个：WireGuard / MASQUE / TrustTunnel / Tailscale / ZeroTier / EasyTier / OpenVPN）：**灰色**中性小字「隧道协议，建议搭配自定义规则使用（按目标网段引流）」，语气必须是建议而非错误（不是红字，也不阻止保存）。原因是隧道节点接入的是对端虚拟网络，而模板规则第一条把私网地址判给了直连，不写 `IP-CIDR` 规则的话内网流量永远进不了隧道（见 3.9 第 6 条；测试见 `TestTunnelProtocolFlag`）。两条提示互斥渲染（`v-else-if`），因为隧道协议不属于过时协议。

1.1 **中文界面下的配置标题附英文小字**：内核配置项以英文为准，中文界面里光看「路径」「跳过证书校验」对不上 `path` / `skip-cert-verify`，因此该弹窗的标题统一走 `components/FieldLabel.vue`（中文 + 灰色小字英文）。英文名来自后端下发的 `label`（字段）与 en 语言包（分区标题），**纯中文标题才追加**——标题里本来就带英文的（「TLS 配置」「ECH 配置」「V2Ray HTTP Upgrade 快速打开」）不再重复，英文界面与专有名词（WebSocket / gRPC / REALITY）也不追加。仅该弹窗使用，其它页面不加。

1.2 **传输层与 TLS 的建模**：`Section` 决定字段落在哪个折叠区；`VisibleWhen` 让传输选项块按 `network` 条件显示（五个块共几十个子字段不可能同时铺开）；`Kind` 里的 `map`（键值对，界面按「键: 值」每行一条）与 `group`（嵌套块，子字段递归渲染）负责表达 `ws-opts.headers`、`reality-opts` 这类结构。三者都由后端声明驱动，前端只做映射。
2. **「只存差异」由后端归一化保证**：保存时 `nodespec.Normalize` 把取值按声明类型转换、剔除与默认值相同的项（空值一律视为未填写）；读取（`GET /subscribe/config`）与写盘（`buildProxiesNode`）时再补齐默认值。因此默认值调整能自动作用到历史数据，`fluxor.json` 也不会被零值塞满。前端的表单既可以发完整取值，也可以发逗号分隔文本（列表字段），归一化在服务端统一完成。
3. **落库前必须过三道校验**（缺失任何一道都会产出内核拒绝加载的配置）：协议与字段取值（`nodespec.Normalize`：未知协议/未知字段/取值类型/下拉选项/必填/`RequireAny` 组合）→ 节点名（非空、长度、无控制字符、列表内不重名，实测 `proxy A is the duplicate name`）→ **名字占用**（不得与标准规则集模板的代理组或内置目标同名，实测 `proxy group X: the duplicate name`）。占用集合直接取 `configgen.MergeRuleSetEnv(base).Targets`，与自定义规则的目标下拉同源，模板一改自动跟随。
4. **嵌套块「要么不出现、要么填齐」**：块内必填项只在「块真的被填写」时校验。判据是 `blockFilled`（块内至少有一个子字段拿到非空取值），**不是「请求里有没有这个块的键」**——读取接口会把块的每个子字段补齐默认值下发，前端原样带回时整块必然非空，按「键存在」判定会让正常节点的保存对着空气报错（曾实测拦下正常保存）。未知子键则无论块是否为空都要拒绝，否则写错键名会被静默忽略。写盘同理分两套判据：顶层字段只按「是否为空」剔除（必填项取默认值也要写出来），嵌套块内的子项额外按「是否等于默认值」剔除——因为**块的存在本身就是内核眼中的「启用该传输」**，把未使用的块 materialize 出来会平白多出 `h2-opts: {path: /}` 这类噪音。

5. **`RequireAny` 是「或 + 与」**：外层数组是「或」、内层是「与」（组内字段需同时填写），用来表达内核的真实约束：hysteria/hysteria2 的 `port` 与 `ports` 二选一、mieru 的 `port` 与 `port-range` 二选一、wireguard/masque/trusttunnel 的本机地址 `ip` 或 `ipv6` 至少一个、openvpn 的「`cert`+`key`」或 `username`、easytier 的 `peers` 或 `listeners`。另有一个 `Always` 标记：内核解码器要求该键必须存在（struct 上没有 omitempty，如 vmess 的 `alterId`），即使取零值也要写进 YAML。
6. **节点名与订阅名规则不同**：节点名不是文件名、不是映射键，只作为 `proxies[].name` 与规则目标，因此允许空格、标点与 emoji（上限 64 字符），仅拒绝空名与控制字符——不要顺手套用 `ValidateSubscriptionName`。
7. **`proxies` 块由 yaml.Node 逐项编码，不拼字符串**：整数保持整数、布尔保持布尔、列表保持序列、键值对与嵌套块保持映射（子块内按字段顺序落键，键值对按字典序）、PEM 多行文本用字面块（`|-`）。字段顺序固定为「name、type、协议字段声明顺序」，便于逐字节比对。
8. **规则集固定标准档位，自定义规则独立存放**：界面上自定义模式隐藏规则集选择，生成时也无视 `rule_group` 里残留的档位（`appendRuleSet(doc, cfg, RuleGroupBase)`），避免「界面不显示、实际按 full 生成」。其自定义规则存在 `custom_mode_rules`（**不按档位分表，也不与融合模式共用**），走独立入口 `/subscribe/custom-mode-rules/custom`；可选目标里额外含手工节点名（该模式的节点是静态 `proxies`、内核加载时就能解析），而融合模式的目标里没有节点。三种作用域的公共流程与差异收敛见 3.9 第一条表后说明。
9. **定时器与订阅无关**：自定义模式不下载订阅、不抓取元数据，`StartAllTimers` 因 `mode != switch` 全部不启动；`/subscribe/update/{name}` 一类的订阅操作在自定义模式下没有意义。
10. **首启按模式生成**：`main.go` 在 `config.yaml` 缺失时按 `config.Current.Mode` 分派（自定义模式走 `GenerateCustomConfig`），否则首启会得到一份没有节点的骨架配置，用户保存过的节点在重启后不生效。
11. **端到端验证手段**：`configgen` 里有一个可选的内核校验用例（`FLUXOR_CORE_BIN=/path/to/mihomo go test ./internal/configgen/ -run TestAllProtocolsAcceptedByCore -v`），它为每个协议生成一个节点并要求真实内核 `-t` 通过——给某协议增补字段/默认值后跑它，能直接发现「字段写错、必填漏填、类型不符」这类只有内核才知道的问题。

---

### 3.12 流量隧道生成规约（`tunnels` 顶层块）

流量隧道（内核的 [tunnels](https://wiki.metacubex.one/config/tunnels/)）把本机某地址收到的 tcp/udp 流量转发到目标地址，可指定经过某个 `proxies` / `proxy-groups`。它是**独立的顶层块**，与规则是两条并行链路，但作用域划分与「写一条存一条」的交互刻意与自定义规则完全对齐（同一个弹窗、同一批作用域）：

| 模式 | 存放位置 | 生效条件 | 注入点 |
|------|----------|----------|--------|
| 切换 | `tunnels.json` → `subscriptions.<订阅名>` | 该订阅是 `active_subscription` | `writeRuntimeConfig`：订阅文件 → config.yaml 副本 → 与规则**一趟**叠加 |
| 融合 | `tunnels.json` → `merge.<base\|full>` | 该档位是 `rule_group` | `GenerateConfig`：`appendRuleSet` 之后、写盘之前注入 |
| 自定义 | `tunnels.json` → `custom_mode` | 恒生效 | `GenerateCustomConfig`：`proxies` 之后、写盘之前注入 |

接口入口同样按模式分开：切换=`/subscribe/custom-tunnels/{name}`、融合=`/subscribe/merge-custom-tunnels/{base\|full}`、自定义=`/subscribe/custom-mode-tunnels/custom`。要点：

1. **`proxy` 必须是配置里真实存在的名字**：内核实测在加载配置时就校验（`tunnel proxy X not found`）并**拒绝整份配置**，因此校验集合直接取该作用域 `RuleContext.Env.Targets`（代理组 + 代理节点 + 内置目标），与规则目标同源。界面上的下拉只列**代理组**；**节点只在自定义模式下**给出（该模式的节点是静态 `proxies`，内核能解析；融合/切换模式的节点来自 provider 或数量庞大的订阅文件，列出来只会淹没下拉）。`DIRECT` / `REJECT` / `PASS` 不进下拉，但校验仍接受（内核确实接受）。
2. **`proxy` 留空 = 不指定代理，不等于「直连」**：内核只在 `SpecialProxy` 非空时才强制走该代理，留空时按正常规则匹配选择出口。界面上该选项写作「关闭（不指定代理）」，文案不要写成「直连」。
3. **目标必须是 `host:port`，但允许省略域名端口**：内核的目标解析是 `socks5.ParseAddr`，它要求 `host:port`；裸主机实测**不会**加载失败，而是启动监听那一刻打一行 `Start tunnel example.com error: invalid target address example.com` 并跳过该条——配置能加载、内核照常运行，隧道却静默失效（界面与日志都看不出；实测加 `:80` 后正常 `listening at`）。因此 `config.NormalizeTunnelTarget` 在写入前把目标定形，供校验、落盘与界面展示三处共用：
   - `host:port`（IP / 域名 / `[IPv6]:port`）→ 原样采用；
   - 纯域名（`example.com`）→ 补**默认端口 80**（`config.DefaultTunnelTargetPort`，http 的约定俗成值）；
   - 裸 IP（`8.8.8.8`、裸 IPv6 `::1`）→ **拒绝**并提示补端口：IP 没有默认端口的约定，猜一个等于替用户把流量转到别处。端口一律不继承监听地址的端口——那会引入一条用户没写在配置里的隐式规则。
4. **非法项跳过而不是写进配置**：监听地址非 `host:port`、端口越界、目标写法无法解析、`proxy` 已不存在、与另一条隧道监听同一地址（内核会 `address already in use`）——这些都会让内核拒绝加载或起不来。`ApplyTunnels` 逐条校验后跳过并记日志，接口再把「有 N 条隧道因配置无效未写入配置」作为 warning 回给前端。判重的依据是 `config.TunnelBindingsOverlap`（**按网络是否相交**，不是比字符串：`[tcp,udp]` 与 `[tcp]` 形态不同却抢同一个端口），保存路径与写入路径共用同一处判断（`configgen.TunnelReasons`）。
5. **启停开关就是「写不写进配置」**：`enabled=false` 的隧道保留在列表里但不写入 `tunnels` 块。开关走整条 PUT（`prepareTunnel` 对关闭的隧道**不做校验**）——否则一条 `proxy` 已被改名的隧道会因为校验失败而关不掉，用户只能删掉重建。
6. **落点恒在 `rule-providers` / `rules` 之前**：生成链路靠 `configcheck.topBlockRank`（`tunnels` 排在 `proxies` 与 `rule-providers` 之间）归一化键序；切换模式的 config.yaml 是订阅文件副本、**不做整体键序归一**（会把机场自带的全部块重排），因此走 `Doc.SetNodeBefore("tunnels", …, "rule-providers", "rules")` 定位插入，两处结论一致。没有任何启用隧道时**完全不触碰文档**：注入的那一块在下次复制订阅文件时自然消失，机场自带的 `tunnels` 块也不该被我们抹掉。
7. **规则接口与隧道接口的响应必须同构**：两者服务同一个作用域（同一个弹窗），响应体都同时含 `rules` 与 `tunnels`。只返回自己那一半，前端整份刷新时就会把另一半抹成空。
8. **与规则的关系是「两条独立链路」**：隧道流量只在 `proxy` 非空时绕过规则匹配；否则仍走规则。因此隧道不参与 `rules` 的排序与幂等注入，也不需要「插入位置」概念——列表顺序只是产物中每条隧道的先后，`config.MoveTunnel` 在同一条列表内相邻交换。
9. **端到端验证手段**：`FLUXOR_CORE_BIN=/path/to/mihomo go test ./internal/configgen/ -run TestTunnelsAcceptedByCore -v` 会把含 `tunnels` 块的配置交给真实内核 `-t`（夹具不含 GEOIP/GEOSITE，因此不需要 geo 数据文件）；给隧道增补字段或改落点时跑它，能直接发现「字段名/类型写错、proxy 解析不到」这类只有内核才知道的问题。

---

### 3.13 日志规约（`logx`，唯一日志出口）

后端**全部**运行日志经叶子包 `logx` 输出，唯一落盘文件是运行数据目录下的 `fluxor.log`（`config.FluxorLogFile`）。这一约定替换了此前三套并存的写法（stdlib `log` → stderr、`fmt.Printf` → stdout、`core.CoreLogger` 直接写文件），原因见 `internal/logx/doc.go`：那三套的落点取决于**启动方式**（fnOS 启动脚本把 stdout/stderr 重定向到日志文件、openwrt 的 procd 进 journal），于是「同一份日志在不同平台落在不同文件、内容还不一样」，甚至出现「面板有日志文件、里面只有几行错误」的误导。

```go
import "fluxor/internal/logx"

logx.Info(logx.ModuleSub, "subscription updated: name=%q proxies=%d", name, n)
logx.Error(logx.ModuleCore, "failed to start mihomo %s: %v", config.CoreBin, err)
```

1. **禁止旁路**：新增代码不得再用 `fmt.Print*` / `log.Print*` / 自建 `*log.Logger`。仅有两类例外——HTTP/WS **响应体**写入（`fmt.Fprintf(w, ...)`、`json.NewEncoder(w).Encode(...)`）与日志系统初始化**之前**的致命用法错误（`fmt.Fprintln(os.Stderr, ...)`）。
2. **禁止外部重定向**：启动脚本不应再把面板的 stdout/stderr 重定向到日志文件，否则日志被拆成两份（后端一份、重定向一份），而两份的位置与内容都不保证一致。脚本自身的 `log_msg`（如 Starting process…）应另择落点或指向同一个 `fluxor.log` 由后端独占写入——两者不可同时写。
3. **记录格式**：`2006-01-02 15:04:05.000 LEVEL [MODULE] message`——本地时间戳（毫秒）、等级（`DEBUG`/`INFO`/`WARN`/`ERROR`）、大功能模块标记、正文。等级由 `FLUXOR_LOG_LEVEL` 控制（debug/info/warn/error，缺省 info）。
4. **正文一律英文**，变量内联（`%q` 名字、`%d` 数量、`%v` 错误，优先 `key=value`），单行输出（format 里**不要**带 `\n`）；注释仍保持中文。新增文案不要写成两行——需要多行时拆成多条记录。
5. **模块标记只用 `logx.Module*` 常量**，粒度对齐后端功能域，不按文件细分：`MAIN`（进程/监听/信号）、`CONFIG`（fluxor.json 持久化）、`CORE`（内核生命周期与状态推送）、`SUB`（订阅下载/更新/元数据/定时器）、`GEN`（config.yaml 生成）、`RULE`（自定义规则注入）、`TUNNEL`（流量隧道注入）、`API`（内核 HTTP 反向代理）、`WS`（WebSocket 桥接）、`TPROXY`（防火墙与策略路由）、`TUN`（TUN 参数与启停）、`NET`、`DELAY`、`QUALITY`、`UPDATE`、`WEB`。新增功能域先在本包登记常量，禁止手写字符串（否则 `grep '\[CORE\]'` 会漏）。
6. **等级语义**（改日志时按此判级，不要一律 Info）：
   - `DEBUG` 内部步骤与正常路径细节：缓存命中、跳过未启用项、定时器启停、条目按家族分流；
   - `INFO` 有意义的状态变化：内核启动/停止、配置生成与热重载、订阅更新完成、规则与隧道落库生效、面板启停；
   - `WARN` 可继续运行但偏离预期：无效项被跳过、直连失败回退临时内核、元数据获取失败但保留旧值、清理失败、开关关闭时忽略操作；
   - `ERROR` 某次操作确实失败：生成/启动/写入/重载失败、持久化失败、nft/ip 命令执行失败。
7. **`Setup` 失败不阻断启动**：日志文件打不开（目录不可写）时 `logx` 退回「只写 stderr」，由 `main.go` 把该错误报到 stderr 后继续运行——日志系统自身不可用不该让面板起不来。
8. **写入口径**：`logx` 用单个 `*log.Logger` 串行写出，每条记录一次 `Write` 且 `O_APPEND`，行不会撕裂；`Setup` 之后才写入的调用方（用 `Enabled()` 自检）无需再加锁。


### 3.14 面板 WebSocket 的来源校验（浏览器 → 面板）

`/traffic`、`/memory`、`/connections`、`/logs` 是**全站唯一走 WebSocket 的接口**，由 `wsproxy` 的 `upgrader` 统一做来源校验。面板自身没有任何认证，而 WebSocket **不受 CORS 约束**（跨站 `fetch` 读不到响应，跨站 WS 却能双向收发），因此这道校验是必要的；但它的实现必须容忍反向代理，否则会一次性打死全部实时功能——四路 WS 同时 403，面板上「流量计、内存占用、连接、日志」全空，而其它走 HTTP 的功能（内核版本、规则、订阅）一切正常，极易误判成后端不通。实测踩过一次，判定见 `wsproxy/upgrader.go`：

1. **显式放行清单** `FLUXOR_WS_ALLOWED_ORIGINS`（逗号分隔主机名；`*` 表示放行全部来源）。逃生口：某些网关把 `Host` 改写成与访问地址无关的值（如上游名 `localhost`）且不透传 `X-Forwarded-Host`，此时自动判定无从得知浏览器用的是哪个主机名，只能显式声明。清单是「额外放行」而非白名单——未命中时仍走后面的自动判定。
2. **`Sec-Fetch-Site`**（首选）：由浏览器按「发起方页面 ↔ 目标 URL」自行推导，**不受反代改写 Host 影响**，是代理部署下唯一可靠的信号。`cross-site` 拒绝，`same-origin` / `same-site` / `none` 放行。
3. **主机名比较**（退化路径，供不发 `Sec-Fetch-Site` 的旧浏览器）：只比**主机名、不比端口**，并可回退到 `X-Forwarded-Host`（多级链取第一个）。

> **「只比主机名、不比端口」是实测教训，不要改回逐字比较**：反代常把 `Host` 的端口剥掉。实测（fnOS 应用网关，面板挂在 `https://<host>:3333` 之后）浏览器发 `Origin: https://fn.1kw.site:3333`，面板收到的却是 `Host: fn.1kw.site`——按「主机:端口」比较会把这种正常部署整体拒掉。跨站攻击来自**不同的站点**（主机名不同），同主机不同端口在浏览器同源策略里属同一个 site；能占用本机其它端口的攻击者已经在本机之内，不是这道校验的目标。
>
> `Origin` 缺失（脚本 / curl 这类非浏览器客户端）一律放行：它们不受跨站请求伪造影响，拦下只会妨碍调试。`Origin: null`（沙箱 iframe / file://）解析出的主机名为空，会被拒绝。
>
> 排查提示：升级失败会记一条 ERROR 并带 `origin=` 与 `host=`（`wsproxy/handler.go`），内核侧拨号失败记 DEBUG（`FLUXOR_LOG_LEVEL=debug` 可见）。这两条日志把「前端没连上 / 来源被拒 / 内核侧连不上」三种情况区分开——此前该路径完全静默，是「WS 没内容」难查的主因。

### 3.15 TUN 运行参数与安全规约（`configgen.TunBlock` + `tun` 包）

在 NAS / 软路由上，TUN 是**会改宿主网络**的高风险功能：一旦开机自动接管且配置有误，面板与机器都可能失联。因此 TUN 的设计围绕「永不自启动、参数与开关分离」展开：

1. **config.yaml 里的 tun 块是静态安全基线**（`configgen.TunBlock`，三条链路共用：模板 / 切换模式的订阅文件副本 / 无订阅兜底 `GenerateBaseConfig`）。它恒为 `enable: false`、`inet6-address: []`，不承载任何用户偏好。
   - 必须保留这份基线而不是省略：mihomo 的 `DefaultRawConfig` 会给 tun 填上 `stack: gvisor`、`dns-hijack: 0.0.0.0:53`、**`inet6-address: fdfe:dcba:9876::1/126`**（`config/config.go`），任何「只发 `{enable:true}`」的 PATCH 都会继承它们；宿主 `net.ipv6.conf.default.disable_ipv6=1` 时下发 IPv6 地址会以 EACCES 失败，整只 TUN 起不来（报成 `configure tun interface: permission denied`）。
   - 切换模式下机场自带的 `tun` 块必须**整块替换**（订阅里的 device / auto-route / enable 与宿主无关）。
2. **用户偏好存 `tun.json`，稀疏**：只写与 `TunBlock` 不同的键（`tun.Save` 用 `sparseString` 对比默认值，相同即删键），**不含 `enable`**。默认值不落盘，因此以后改模板默认值能自动惠及没改过的用户。
3. **开关状态只活在运行时，且不回放**：`/config/tun/state` 是唯一入口。内核启动、配置重载（`PUT /configs` 重新解析 config.yaml）、面板重启之后 TUN 必然关闭——用户需要重新开启；前端必须常驻提示「重载/重启后自动关闭」，这不是错误。
   - 反过来，**任何配置重载（含订阅定时更新）都会拆掉正在运行的 TUN**，这是策略而非缺陷，不要为了"体验"加回放。
4. **开启时必须 PATCH 完整 tun 对象**：`tun.runtimeConfig` 会带上 stack / dns-mode / inet6-address / dns-hijack / auto-route / auto-redirect / auto-detect-interface（以及非空时的 device）。理由同第 1 条——mihomo 的 `patchConfigs` 以 `listener.LastTunConf`（基线）为基底，只发 `enable` 会让用户设置全部静默失效。
5. **改参数不碰 config.yaml、不重载**：`POST /config/tun` 只落 `tun.json`，参数在**下次开启 TUN** 时生效；界面改参数时若 TUN 正在运行，要提示「重新开启后生效」。
6. **IPv6 接管两档**（`tun.json` 的 `ipv6`）：`auto`（默认，按宿主能力：`/proc/sys/net/ipv6/conf/{default,all}/disable_ipv6` 与 `/sys/module/ipv6/parameters/disable` 全为 0 才下发地址）/ `off`。不提供「强制开启」——宿主支持时 auto 与强制等价，宿主不支持时强制只会让 TUN 起不来；旧值 `on` 在 `Normalize` 里收敛为 `auto`，不报错。全局 `ipv6`（订阅中心的生成期开关）关闭时，开启 TUN 一律不下发 IPv6 地址。
7. **`dns-mode` 三档**（透传内核）：`hijack`（默认）/ `native` / `disabled`。两个平台事实：Linux 的 hijack 依赖 `auto-redirect` 的 nft 链（基线里恒为 true）；`native` 只调用 `resolvectl`，无 systemd-resolved 的平台（OpenWrt 等）上等于空操作。该键需要 mihomo ≥ v1.19.33，老内核会忽略它（行为回落到隐式 hijack）。
8. **与 TProxy 互斥**：开启 TUN 时后端校验 `tproxy.GetTproxyState()`，前端只负责先关 TProxy；两者同时生效会互相破坏对方的流量路径。
9. **IPv6 接管受总开关约束（单向门）**：订阅中心的「IPv6 支持」关闭时，TUN 不下载 v6 地址、TProxy 不下发 ip6 规则，但两处的**偏好值原样保留**（`tun.json` / `tproxy.json` 一个字节都不改），总开关恢复后自动重新生效。UI 只做置灰与说明，不替用户改写偏好；`/config/tun` 与 `/config/tproxy/proxy-ipv6` 的 GET 都返回 `global_ipv6` 供前端判断。
10. **两套 IPv6 概念不要混淆**：订阅中心的「IPv6 支持」（`ipv6`）与「DNS 返回 AAAA」（`dns.ipv6`）是**生成期**字段（前者关闭时后者强制关闭，见 `config.NormalizeIPv6Flags`）；TUN 的「IPv6 接管」只决定 `tun.inet6-address`。配置页不再有 IPv6 开关（它原先只 PATCH 内核运行时、重启即失效）。

为了保障页面切换时的流畅交互体验，并避免在后台静置运行时产生资源泄漏：

### 4.1 状态托管与快速加载
- 将各个页面的核心业务数据（如配置列表、代理节点、规则、连接快照）统一由全局 Pinia store 维护。组件重新挂载时直接呈现历史数据快照，随后在后台静默发起 fetch 刷新以同步最新状态（通过 `silent = true` 参数）。

### 4.2 WebSocket 引用计数生命周期管理
所有实时数据流（流量、内存、连接、日志）均采用 **引用计数 + 防抖断开** 模式，而非在 `onUnmounted` 中直接关闭 WS：

```
subscribe() → subscriberCount++ → 首次订阅时建立连接
unsubscribe() → subscriberCount-- → 归零后延迟 3 秒（防抖）→ 若无新订阅才真正断开
```

**优势**：
- 同一组件可多次安全订阅（如 `Overview.vue` 中的流量、内存、状态三路数据各自独立计数）。
- 组件切出与切回时保持 WS 连接的连续性，实现平滑过渡。
- 连接断开时，若仍有订阅者，自动尝试重连：
  - 流量/内存/连接 WS：5 秒后重连。
  - 日志 WS：指数退避重连（1s → 2s → 4s → ... → 最大 30s）。

**缓存留存**：
- 流量历史（`overview.ts`）保留最近 65 个数据点。
- 日志缓冲区（`logs.ts`）上限 2000 条。
- 已关闭连接（`connections.ts`）上限 100 条，超限时从旧到新截断。

### 4.3 连接瞬时速率计算
`connections.ts` 通过 **快照差分法** 计算每条连接的瞬时速率：记录每次 WS 消息到时的 `(upload, download, timestamp)` 快照，下一帧按 `(uploadDiff + downloadDiff) / timeDiff` 得出速率。
- 首帧（或暂停恢复后）跳过归档，防止陈旧快照误判。
- 前后端断开瞬间自动归档为已关闭连接。

### 4.4 批量测速并发控制规约
- 无论是 Vue 3 前端中的批量测速（全部测速或组测速），**必须强制实施并发控制（默认并发数限制为 10）**。绝对禁止一次性无限制发起数百个网络测速请求，防止浏览器连接队列拥堵与后端 Unix Socket 重试负载崩溃。

### 4.5 弹窗与交互去原生化
- 全站**绝对禁止使用**浏览器的阻塞式 `alert(...)`。如有提示需要，一律使用 `globalStore.showToast(text, 'success' | 'error' | 'warning' | 'info')` 发送非阻塞的全局自定义 Toast。
- 确认操作使用 `globalStore.showConfirm({title, message, confirmText?, cancelText?})` 返回 Promise，在 `App.vue` 中以模态渲染。
- 所有在页面上展示的图标**必须使用 `xicons` (@vicons/ionicons5)**，禁止在系统硬编码 SVG、Emoji 或颜文字符号（包括内置配置模板中也绝对禁止在代理组和规则名中硬编码 Emoji），保持视觉绝对统一。

### 4.6 乐观更新与回滚
涉及高频用户操作的接口（如代理选择切换、规则启用/禁用、连接断开）采用**乐观更新**策略：
1. 立即更新本地状态，呈现即时反馈.
2. 并发发起 API 请求。
3. 请求失败时自动回滚至更新前的状态。

### 4.7 前端离线 Mock 联调机制
为了方便前端脱离 Go 后端独立运行与联调，`frontend/src/utils/mock.ts` 实现了完整的 HTTP API 与 WebSocket 数据流模拟器。
- **启用机制**：仅在 Vite 开发模式（`import.meta.env.DEV`）下启用。`api.ts` 通过 `import.meta.env.DEV ? await import('./mock') : null` 动态装载，生产构建中模拟器模块整体被摇树移除，线上无法通过任何 localStorage 值开启。
- **手动控制**（仅 DEV 有效）：可通过在控制台修改 `localStorage` 的键 `MOCK_BACKEND` 来强制覆盖：
  - 强制启用 Mock 模式：`localStorage.setItem('MOCK_BACKEND', 'true')`
  - 强制关闭 Mock（直接连接真实后端）：`localStorage.setItem('MOCK_BACKEND', 'false')`

### 4.8 缓存视图下的生命周期管理与滚动能效控制
在 KeepAlive 缓存组件后台化时，为了防止后台页面（如 `Logs.vue`）继续对增长的日志流进行无效的 DOM 渲染和滚动条计算，必须绑定 `onActivated` / `onDeactivated` 生命周期钩子来冻结/激活这些滚动计算（如 logs watcher），有效控制组件静置状态下的 CPU 负载与系统开销。

### 4.9 复杂数据预解析与计算缓存
严禁在组件渲染周期内，或者在模板中高频调用包含重度计算或复制排序的操作（如 `sort` 历史延迟记录）。所有这类解析操作应收归至 Store 中，在拉取到最新数据的第一时间完成一轮预解析，并直接挂载为节点的响应式扁平字段（如 `latestDelay`、`recentColors`），组件仅负责以 O(1) 效率读取。

### 4.10 表单直连与双向绑定规范
表单交互输入（如端口设定）应直接通过 `v-model` 或 `v-model.number` 直连 Pinia Store 托管的数据对象。严禁声明冗余的局部 ref 变量并配合 watch-deep 进行繁重的手动双向映射赋值。当表单输入非法导致校验不通过时，可通过 Store 的 fetch 行为从后端重新拉取真实数据完成本地强制回滚。

### 4.11 Tailwind CSS v4 迁移要点与样式兼容约束

前端已从 Tailwind CSS 3 迁移至 v4（`@tailwindcss/vite` 插件 + CSS-first 配置）。**原 `tailwind.config.js` 与 `postcss.config.js` 已删除**，全部定制集中在 `src/index.css` 顶部。修改样式前请务必了解以下规则，否则极易引入视觉回归：

1. **配置位置**：`@import "tailwindcss" source(none)` 关闭自动扫描后，由 `@source` 显式声明扫描范围（务必保留 `source(none)`，否则会误扫 `node_modules` 使产物膨胀 30KB+）。
2. **暗色模式**：由 `@custom-variant dark (&:is([data-theme="dark"] *))` 复刻 v3 的 `darkMode: ['class','[data-theme="dark"]']`，选择器形态与 v3 完全一致，**不要**改成 `:where()` 或其他形式。
3. **语义色必须用 `@theme inline`**：`accent/success/danger/warning` 基于 `var(--accent)` 等动态变量，必须写在 `@theme inline` 中。若改用普通 `@theme`，v4 将无法为这些颜色生成透明度修饰符（`bg-accent/10`、`ring-accent/30`、`shadow-accent/15` 等会全部失效）。
4. **尺寸档位覆盖**：v4 的档位整体上移一级（v4 的 `shadow-sm` 等于 v3 的 `shadow`，v4 的 `rounded-sm` 等于 v3 的 `rounded`）。`index.css` 已覆盖 `--shadow-sm` / `--radius-sm` 回到 v3 取值，保证不改类名即维持原效果。**请勿删除这两行覆盖**。
5. **`space-y-*` / `space-x-*` 已用 `@utility` 重写**：v4 内置版本为 `:where(...)`（特异性为 0）且作用于 `margin-block`，会导致子元素自身的 `my-*` 反压 space 间距并叠加外边距（曾使 Overview 卡片矮 2px）。重写后与 v3 行为一致，**不要移除**。
6. **`@layer` 顺序敏感**：项目自定义 CSS 全部位于 `@layer utilities` 之内。v4 把工具类放进 CSS 级联层，若把自定义样式写在层外（无层级），其优先级会反超工具类，导致 `transition-all` 之类属性被意外覆盖。新增全局样式时请同样置于该层内。
7. **v4 默认值差异**：v4 preflight 移除了 `button{cursor:pointer}`（已在 `@layer base` 补回）；`border` 默认色由 `#e5e7eb` 变为 `currentColor`；`hover:` 变体被包进 `@media (hover:hover)`。改动按钮或边框样式时注意这些差异。
8. **调色板为 v4 oklch 配色**：站点主色 slate 系偏差 ≤4/255（基本无感），但 red/amber/rose/emerald/blue 等饱和色最大偏差可达 ~38/255，属有意接受的 v4 新调色板，并非缺陷。
9. **透明度修饰符已生效**：v3 时代因非静态色值而静默失效的 18 处类（如 `bg-accent/10`、`ring-accent/5`、`dark:text-accent/90`、`py-4.5`）在 v4 下已正常生成。这是预期内的修复，若需回退到"失效"状态需另行评估。
10. **回归验证方法**：改动样式后，除 `npm run build` 外建议做一次浏览器计算样式比对：以迁移前的 v3 产物 CSS 为基准，将两版 CSS 分别套在相同 DOM 上 diff `getComputedStyle`（重点看 `display`/`position`/宽高/内外边距/`borderRadius`/`transitionProperty`/`cursor`）。仅比对类名是否存在不足以发现级联层与特异性类回归。

---

