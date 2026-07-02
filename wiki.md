Welcome to the fluxor wiki!
# Fluxor Wiki

欢迎来到 Fluxor 的文档仓库。Fluxor 是一个基于 [Mihomo](https://github.com/MetaCubeX/mihomo) 内核的图形化代理管理面板，提供简洁直观的 Web 界面，支持订阅管理、节点测速、规则控制、实时流量监控等核心功能。本文档由deepseek根据源代码生成，经人工初步校对，有错误请包容，使用问题以实际为主。

---

## 📖 目录

- [项目简介](#项目简介)
- [功能特性](#功能特性)
- [快速开始](#快速开始)
- [安装与部署](#安装与部署)
- [配置指南](#配置指南)
- [使用指南](#使用指南)
- [API 参考](#api-参考)
- [开发指南](#开发指南)
- [常见问题](#常见问题)

---

## 项目简介

Fluxor 是一个轻量级的代理面板，旨在为 Mihomo 内核提供便捷的配置管理和运行控制。它通过 Unix Socket 与内核通信，支持：

- 订阅管理（融合/切换两种模式）
- 节点测速与质量评分
- 规则集（Rule Providers）管理
- 实时连接查看与断开
- 日志流式输出
- 系统配置（端口、TUN、TProxy 等）
- 多语言（中文/英文）和主题（亮色/暗色/紫色/粉色）

项目采用前后端分离架构：
- **前端**：Vue 3 + TypeScript + Pinia + Vite + TailwindCSS
- **后端**：Go (1.26) + 标准库，通过 Unix Socket 与 Mihomo 交互

---

## 功能特性

### 核心功能
- **订阅管理**：支持 HTTP 订阅链接，自动更新节点列表，可切换“融合模式”（所有订阅节点合并）或“切换模式”（每次只激活一个订阅）。
- **代理组与节点**：直观展示所有代理组，支持手动选择节点、一键测速（全部或单个），并显示延迟历史和质量评分。
- **规则管理**：显示当前规则列表，支持启用/禁用单条规则，并管理规则提供商（Rule Providers）的手动更新。
- **连接管理**：实时显示活动连接，可查看详细信息（来源、目标、规则链、速率等），支持断开单个或全部连接。已关闭连接保留历史记录。
- **日志查看**：实时流式输出内核日志，可按级别过滤（debug/info/warning/error），支持搜索和暂停滚动。
- **概览仪表盘**：展示实时上下行速度、总流量、内存占用、活动连接数、内核版本、当前节点等信息，并配有流量趋势图（可悬停查看详情）。
- **IP 信息**：分别显示本机 IPv4/IPv6 和代理 IPv4/IPv6 地址及归属地（国家、地区、ISP），支持复制和密文显示。
- **延迟测速**：对预设网站（Baidu、Bilibili、Google、GitHub、YouTube）及自定义 URL 通过代理进行延迟测试，结果以颜色徽章呈现。
- **TProxy 透明代理**：支持 TCP/UDP 透明代理，可配置目的/源例外列表，以及是否代理本机出站流量（需 nftables）。
- **面板控制**：一键启动/停止/重启内核，热重载配置，升级内核，清空 FakeIP/DNS 缓存，更新 Geo 数据库。

### 界面与体验
- 侧边栏可折叠，适应桌面/移动端。
- 深色/浅色/紫色/粉色多种主题。
- 中文/英文界面切换。
- 全局 Toast 提示和确认对话框。
- 响应式布局，移动端底部导航。

---

## 快速开始

### 前提条件
- Linux 系统（推荐 Ubuntu/Debian，需支持 nftables 和 ip 命令）
- 已安装 Mihomo 内核（建议版本 v1.18.0+）
- 可选：启用 TProxy 时需 nftables

### 一键安装（假设已编译好二进制）
```bash
# 将编译获得的fluxor与官方下载的mihomo内核放在合适路径并赋予执行权限
chmod +x fluxor
chmod +x mihomo

# 设置环境变量
export BASE_URL="/app/Fluxor"                    # 启用baseurl路径访问（适用于飞牛os等nas网关环境）
# export FLUXOR_ADDR="0.0.0.0:8080"              # 启用 TCP 监听（否则只监听 Unix Socket）
export SOCKET_PATH="path-to-app.sock"            # fluxor 监听的Unix Socket
export SOCKET_PATH="path-to-core.sock"           # mihomo 内核监听的Unix Socket
export CORE_BIN="path-to-mihomo"                 # mihomo 内核路径
export CORE_PID_FILE="path-to-core.pid"          # mihomo 内核运行pid
export FLUXOR_CONFIG_FILE="path-to-fluxor.json"  # fluxor 运行配置
export CORE_WORK_DIR="path-to-workdir"           # mihomo 内核工作目录（包含config.yaml、ui、proxies、geoip、db等）
export CONFIG_TARGET="path-to-config.yaml"       # mihomo 运行配置文件
export INFO_LOG_FILE="path-to-info.log"          # fluxor 运行log
# export META_DIR="path-to-metadir"              # metacubexd 面板路径(非必需)
# export ZASH_DIR="path-to-zashdir"              # Zashboard 面板路径(非必需)


# 运行
./fluxor
```
此时访问 `http://your-server:8080/app/Fluxor` 或nginx反代的fluxor Unix Socket地址，即可打开面板（如果设置了 `BASE_URL`，路径对应）。

### 首次配置
1. 打开面板后，进入“订阅”页面，添加您的订阅链接。
2. 设置代理端口（默认 7890）、面板端口（默认 9090）和面板密钥（可选）。
3. 选择“规则组” (base 或 full) 和 UI 面板 (MetaCubeXD 或 Zashboard)。
4. 点击“保存并应用”，面板会自动生成配置文件并启动内核。

---

## 安装与部署

### 目录结构
Fluxor 飞牛os默认目录结构（可通过环境变量修改）：
```
/var/apps/Fluxor/
├── target/
│   ├── app.sock          # Fluxor 自身 Unix Socket
│   ├── core.sock         # Mihomo 内核 Unix Socket
│   ├── bin/
│       │── mihomo        # 内核二进制文件
│       └── fluxor        # fluxor二进制文件
├── var/
│   ├── app.pid           # fluxor PID 文件
│   ├── core.pid          # 内核 PID 文件
│   └── fluxor.json       # 订阅配置持久化
├── shares/
│   ├── Fluxor/
│   │   ├── config.yaml   # 当前生效的配置文件
│   │   ├── info.log      # 内核日志文件
│   │   └── proxies/      # 订阅文件存储（切换模式）
│   └── ui/
│       ├── meta/         # MetaCubeXD 静态资源
│       └── zash/         # Zashboard 静态资源
```

### 环境变量（飞牛os默认设置）
| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `SOCKET_PATH` | `/var/apps/Fluxor/target/app.sock` | Fluxor 监听的 Unix Socket 路径 |
| `BASE_URL` | `/app/Fluxor` | 面板访问的 URL 前缀 |
| `CORE_PID_FILE` | `/var/apps/Fluxor/var/core.pid` | 内核 PID 文件 |
| `CORE_BIN` | `/var/apps/Fluxor/target/bin/mihomo` | 内核二进制路径 |
| `CORE_SOCKET` | `/var/apps/Fluxor/target/core.sock` | 内核 Unix Socket 路径 |
| `META_DIR` | `/var/apps/Fluxor/shares/ui/meta` | MetaCubeXD 静态文件目录 |
| `ZASH_DIR` | `/var/apps/Fluxor/shares/ui/zash` | Zashboard 静态文件目录 |
| `FLUXOR_CONFIG_FILE` | `/var/apps/Fluxor/var/fluxor.json` | 订阅配置持久化文件 |
| `CONFIG_TARGET` | `/var/apps/Fluxor/shares/Fluxor/config.yaml` | 生成的配置文件路径 |
| `CONFIG_TEMPLATE_DIR` | `/var/apps/Fluxor/target/templates` | （已废弃）模板目录 |
| `INFO_LOG_FILE` | `/var/apps/Fluxor/shares/Fluxor/info.log` | 日志文件路径 |
| `CORE_WORK_DIR` | `/var/apps/Fluxor/shares/Fluxor` | 内核工作目录 |
| `FLUXOR_ADDR` | 空 | 若设置，则启用 TCP 监听（如 `0.0.0.0:8080`） |

### 构建
#### 前端
```bash
git clone https://github.com/shuangji66/fluxor
cd fluxor/web
npm ci
npm run build
# 将 dist 目录内容复制到后端的 static 目录（内嵌）
```

#### 后端
```bash
cd ..
go build -tags vue -o fluxor
```

### 运行服务(未实现，请忽略)
直接运行二进制即可，也可以使用 systemd 管理：
```ini
# /etc/systemd/system/fluxor.service
[Unit]
Description=Fluxor Proxy Panel
After=network.target

[Service]
ExecStart=/usr/local/bin/fluxor
WorkingDirectory=/var/apps/Fluxor
Restart=always
User=root

[Install]
WantedBy=multi-user.target
```

---

## 配置指南

### 订阅配置 (`fluxor.json`)
该文件由面板自动管理，无需手动编辑。结构如下：
```json
{
  "proxy_port": 7890,
  "tproxy_port": 7898,
  "panel_port": 9090,
  "panel_secret": "your-secret",
  "rule_group": "base",
  "ui_panel": "metacubexd",
  "meta_backend_url": "",
  "mode": "merge",
  "active_subscription": "",
  "subscriptions": [
    {
      "name": "订阅1",
      "url": "https://example.com/sub",
      "update_interval": 86400,
      "health_interval": 600,
      "prefix": "",
      "updated_at": "2025-01-01T00:00:00Z",
      "subscription_info": {
        "upload": 123456789,
        "download": 987654321,
        "total": 1073741824,
        "expire": 1735689600
      }
    }
  ]
}
```

### 模式选择
- **融合模式 (merge)**：所有订阅的节点合并到代理组中，并使用预置规则组，适用于多个订阅互补。
- **切换模式 (switch)**：只激活一个订阅，使用订阅自带代理组和规则，适合按需切换。

### 规则组
- **base**：标准规则，包含少量常见分流（GEOIP/GEOSITE）。
- **full**：详细规则，包含大量规则提供者（如广告拦截、流媒体、游戏等），适用于需精细分流的场景。

### TProxy 配置
TProxy 透明代理需系统支持 nftables。在“配置”页面可开启 TProxy，并设置例外列表：
- **目的例外**：IP/CIDR 或 端口（如 `53`、`tcp:80`），这些流量将不被劫持。
- **源例外**：源 IP/CIDR 不被劫持（仅入站）。
- **本机代理**：是否代理本机发出的流量。

开启后，Fluxor 会自动添加 nftables 规则（表 `ip fluxor_tproxy`）和策略路由（fwmark 1 table 100）。

---

## 使用指南

### 概览页
- **顶部统计卡片**：显示上下行速率、总量、内存、连接数、内核版本。
- **当前节点信息**：显示当前订阅、代理组、选中节点。
- **流量趋势图**：展示最近 65 个点的上下行速率，可悬停查看具体数值。
- **IP 信息**：显示本机和代理的 IPv4/IPv6 地址及归属地，支持复制和密文切换。
- **延迟测试**：对预设网址或自定义 URL 进行测速，结果以颜色标识（绿<200ms，黄<500ms，红≥500ms）。

### 代理页
- 显示所有代理组，每个组可展开查看节点列表。
- 支持选择节点（点击节点名称即可切换）。
- 每个组有测速按钮，可测试该组所有节点的延迟。
- 支持按名称、延迟、质量排序，并可设置延迟阈值和历史记录条数。
- 支持正则表达式过滤节点（在设置弹窗中配置）。

### 订阅页
- 添加/编辑/删除订阅。
- 手动更新单个订阅，更新进度实时反馈。
- 设置代理端口、面板端口、密钥、规则组、UI 面板等全局参数。
- “保存并应用”生成配置文件并热重载内核。

### 规则页
- 显示当前所有规则，支持启用/禁用。
- 规则提供商列表，可手动更新每个提供商。
- 支持搜索过滤。

### 连接页
- 实时显示活动连接（包括来源 IP、目标、规则链、上下行速率、持续时间）。
- 可断开单个或全部连接。
- 已关闭连接会保留在“已关闭”标签页中，可手动清除。
- 支持排序（按主机、规则、链路、速率等）和搜索。

### 日志页
- 流式输出内核日志，可按级别过滤。
- 支持暂停/恢复实时滚动，清空日志。
- 日志内容可选中复制。

### 配置页
- 通用设置：允许局域网、IPv6、运行模式（Rule/Global/Direct）、日志级别。
- 端口设置：HTTP/Socks/Redir/TProxy/Mixed 端口。
- TUN 设置：开启 TUN 模式，选择堆栈（gVisor/System/Mixed）和设备名。
- 网络接口选择。
- TProxy 透明代理开关及例外列表管理。
- 高级维护：启动/停止/重启内核，升级内核，重载配置，清空 FakeIP/DNS 缓存，更新 Geo 数据库。
- 界面设置：语言、主题、起始页。

---

## API 参考

Fluxor 后端提供 RESTful API 和 WebSocket 接口，所有路径均以 `BASE_URL` 为前缀。

### 内核控制
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/core/status` | 获取内核运行状态 (`{"running": bool}`) |
| POST | `/core/start` | 启动内核 |
| POST | `/core/stop` | 停止内核 |
| POST | `/core/restart` | 热重载内核（通过 PUT /configs） |
| POST | `/upgrade` | 升级内核（调用内核 `/upgrade`） |

### 配置管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/configs` | 获取当前内核配置 |
| PATCH | `/configs` | 修改部分配置（同时处理 TProxy 端口变化） |
| PUT | `/configs` | 重载配置文件 |

### 订阅管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/subscribe/config` | 获取订阅配置 |
| POST | `/subscribe/config` | 保存订阅配置 |
| POST | `/subscribe/generate` | 生成并应用配置文件（含模式切换） |
| POST | `/subscribe/update/{name}` | 更新指定订阅（融合模式异步，切换模式同步） |
| POST | `/subscribe/update-info/{name}` | 手动更新单个订阅的元数据（流量/有效期） |

### 代理与测速
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/proxies` | 获取所有代理组 |
| GET | `/proxies/{name}/delay?url=&timeout=` | 测速单个节点 |
| PUT | `/proxies/{name}` | 切换代理选择（body 为 `{"name":"新节点"}`） |
| GET | `/providers/proxies/{name}` | 获取订阅代理列表及元数据 |
| PUT | `/providers/proxies/{name}` | 更新订阅代理（内核） |
| GET | `/proxies/quality` | 获取所有节点的质量评分（基于历史延迟） |

### 规则管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/rules` | 获取规则列表 |
| PATCH | `/rules/disable` | 禁用/启用规则（body 为 `{"index": enabled}`） |
| GET | `/providers/rules` | 获取规则提供商列表 |
| PUT | `/providers/rules/{name}` | 更新指定规则提供商 |

### 连接管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/connections` | WebSocket 实时连接流 |
| DELETE | `/connections` | 断开所有连接 |
| DELETE | `/connections/{id}` | 断开指定连接 |

### 日志
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/logs` | WebSocket 日志流 |

### 实时数据
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/traffic` | WebSocket 流量数据 |
| GET | `/memory` | WebSocket 内存数据 |

### 系统工具
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/interfaces` | 获取可用网络接口列表 |
| POST | `/cache/fakeip/flush` | 清空 FakeIP 缓存 |
| POST | `/cache/dns/flush` | 清空 DNS 缓存 |
| POST | `/configs/geo` | 更新 Geo 数据库（回退 /providers/geo） |
| GET | `/dns/query?name=&type=` | DNS 查询（代理至内核） |
| GET | `/version` | 获取内核版本 |
| GET | `/whoami` | 返回当前用户（从 X-Trim-Username 头） |

### IP 信息
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/ipinfo/local/v4` | 本机 IPv4 及归属地 |
| GET | `/ipinfo/local/v6` | 本机 IPv6 |
| GET | `/ipinfo/proxy/v4` | 代理 IPv4 及归属地 |
| GET | `/ipinfo/proxy/v6` | 代理 IPv6 |

### 延迟测试
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/delaytest/google` | 测试 Google 延迟 |
| GET | `/delaytest/youtube` | 测试 YouTube 延迟 |
| GET | `/delaytest/github` | 测试 GitHub 延迟 |
| GET | `/delaytest/baidu` | 测试 Baidu 延迟 |
| GET | `/delaytest/bilibili` | 测试 Bilibili 延迟 |
| GET | `/delaytest/custom?url=&timeout=` | 测试自定义 URL |

### TProxy 管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/config/tproxy` | 获取 TProxy 启用状态 |
| POST | `/config/tproxy` | 设置 TProxy 状态 (body: `{"enable": bool}`) |
| GET | `/config/tproxy/exceptions` | 获取例外列表 (`{"dst":[], "src":[]}`) |
| POST | `/config/tproxy/exceptions` | 保存例外列表 |
| GET | `/config/tproxy/proxy-local` | 获取本机代理开关 |
| POST | `/config/tproxy/proxy-local` | 设置本机代理开关 |

### WebSocket 代理
- `/traffic`、`/memory`、`/logs`、`/connections` 均支持 WebSocket 升级，后端会透传至内核 Unix Socket。

---

## 开发指南

### 前端开发
#### 技术栈
- Vue 3 (Composition API)
- TypeScript
- Pinia (状态管理)
- Vue Router (虽未显式使用，但通过动态组件切换)
- Vue I18n (国际化)
- TailwindCSS + 自定义样式
- Vite

#### 目录结构
```
web/
├── src/
│   ├── components/        # 可复用组件（如 FormSwitch, ProxyGroupCard）
│   ├── composables/       # 组合式函数（useTheme, useLanguage）
│   ├── store/             # Pinia stores（global, config, overview, connections, proxies, rules, logs）
│   ├── utils/             # 工具函数（api.ts 封装 fetch）
│   ├── views/             # 页面组件（Overview, Proxies, Rules, Connections, Logs, Config, Subscription）
│   ├── App.vue            # 主布局组件
│   └── main.ts            # 入口
├── public/
├── index.html
└── package.json
```

#### 本地开发
```bash
cd web
npm ci
npm run dev   # 默认监听 5173，可通过 proxy 配置转发 API 请求
```

#### 构建
```bash
npm run build
# 产物在 dist/，需将其嵌入后端的 static 目录（通过 go:embed）
```

### 后端开发
#### 技术栈
- Go 1.21+
- 标准库 net/http
- gorilla/websocket (WebSocket 支持)
- 无外部依赖

#### 目录结构
```
fluxor/
├── main.go               # 入口，路由注册，服务启动
├── handlers_api.go       # 通用 API（版本、配置、连接、规则等）
├── handlers_core.go      # 内核控制（启动/停止/重载）
├── handlers_index.go     # 主页和 whoami
├── handlers_tproxy.go    # TProxy 规则管理
├── handlers_utils.go     # 辅助函数（IP 查询、测速、质量评分等）
├── subscribe.go          # 订阅配置管理、生成配置文件、定时更新
├── static/               # 内嵌静态文件（前端构建产物）
│   ├── html/
│   └── ...
├── go.mod
└── go.sum
```

#### 关键模块
- **coreRequest**：封装向内核 Unix Socket 的 HTTP 请求，自动添加认证头。
- **订阅管理**：`subscribeConfig` 全局变量，通过 `fluxor.json` 持久化。
- **配置生成**：`generateConfig` 根据订阅和规则组生成完整的 `config.yaml`，支持模板替换。
- **TProxy**：使用 nftables 和 ip 命令动态添加/删除规则。
- **定时器**：切换模式下为每个订阅启动定时更新；并启动健康检查定时器（每 10 秒检查一次，对当前激活订阅的节点进行测速）。
- **WebSocket 代理**：透传连接至内核，支持日志、流量、连接等实时数据。

#### 构建与运行
```bash
cd fluxor
go mod tidy
go build -tags vue -o fluxor
./fluxor
```

#### 嵌入静态文件
使用 `//go:embed static` 将前端构建产物嵌入二进制，无需额外部署静态资源。

#### 自定义扩展
- 新增 API 路由：在 `main.go` 的 `mux.HandleFunc` 中注册。
- 新增前端页面：在 `views/` 创建组件，并在 `App.vue` 的 `components` 映射中添加。
- 新增 store：在 `store/` 下创建，使用 Pinia 的 `defineStore`。

---

## 常见问题

### 1. 面板无法启动内核
- 检查 `CORE_BIN` 路径是否正确，且二进制有执行权限。
- 检查 `CORE_WORK_DIR` 目录是否可写。
- 查看 `info.log` 日志文件获取详细错误。

### 2. 订阅更新失败
- 确认订阅 URL 可访问，且返回内容包含 `proxies:` 或 `proxy-providers:`。
- 切换模式下，确保 `proxies/` 目录可写。

### 3. TProxy 不生效
- 确认系统支持 nftables（`nft --version`）。
- 检查 `tproxy_port` 是否设置且大于 0。
- 查看系统日志（`dmesg`）是否有 nftables 错误。
- 确认 TProxy 开关已开启，并且例外列表未拦截所有流量。

### 4. WebSocket 连接失败
- 确认内核已运行且 Unix Socket (`coreSocket`) 存在。
- 检查面板密钥是否与内核配置中的 `secret` 一致（若启用）。
- 查看浏览器开发者工具网络面板，确认 WebSocket 握手状态。

### 5. 流量趋势图无数据
- 确保 `/traffic` WebSocket 连接正常。
- 检查内核是否启用 `external-controller` 和 `external-controller-unix`。
- 刷新页面或重新启动内核。

### 6. 如何自定义规则组？
- 可在 `subscribe.go` 中修改 `proxyGroupsBase` 或 `proxyGroupsFullTemplate` 常量，或添加新的规则组选项。
- 注意修改后需重新编译后端。

### 7. 如何切换 UI 面板？
- 在订阅页面的“UI 面板”下拉框选择 `metacubexd` 或 `zashboard`，保存并应用后，概览中外部控制面板链接会指向对应的子路径。

### 8. 数据库/持久化
- Fluxor 仅使用 JSON 文件持久化（`fluxor.json`），无需外部数据库。配置文件 (`config.yaml`) 由程序生成。

### 9. 日志级别
- 支持 `silent`、`info`、`warning`、`error`、`debug`，可在配置页修改。

### 10. 如何升级内核？
- 使用配置页的“升级内核”按钮，会调用内核自身的 `/upgrade` 接口。

---

## 贡献指南

欢迎提交 Issue 和 Pull Request。请确保：
- 前端代码通过 ESLint 检查（`npm run lint`）。
- 后端代码通过 `go vet` 和 `go fmt`。
- 提交信息清晰描述改动内容。
- 新增功能需补充对应文档。

---

## 许可证

本项目采用 Apache-2.0 许可证，详情请见 [LICENSE](LICENSE) 文件。

---

感谢使用 Fluxor！如有任何疑问，请通过 GitHub Issues 与我们联系。