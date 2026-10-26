# Fluxor

`Fluxor` 是一个轻量级、无冗余的 Mihomo (Clash.Meta) 内核管理面板与订阅生成系统。系统采用前后端一体化设计：Go 后端以 `//go:embed` 内嵌 Vue 3 前端构建产物，单体交付，支持自动化配置生成、透明代理（TProxy）管理与实时的内核状态监控。

---

## 核心特性

- **内核管理**：Mihomo 进程的启动、停止、状态查询与配置热重载（不中断长连接）。
- **订阅中心**：订阅链接的 CRUD 管理；支持**融合模式**（多订阅合并为一份配置）与**切换模式**（按订阅切换，含定时更新与健康检查）；自动生成内核可用的 `config.yaml` 并热重载应用。
- **实时监控**：基于 WebSocket 中转上传/下载速率、内存占用、内核日志流与连接历史。
- **透明代理 (TProxy)**：集成 nftables 透明代理与策略路由，支持源/目的例外 IP、端口过滤及本机出站代理开关；IPv6 接管在高级设置中单独控制（默认关闭，受订阅中心「IPv6 支持」总开关约束），退出时自动清退规则。
- **TUN 模式**：虚拟网卡接管系统流量，协议栈 / 设备名 / DNS 处理方式 / IPv6 接管可配（参数存 `tun.json`，开关不落盘、不回放）；与 TProxy 双向互斥，后端兜底校验。
- **IPv6 生成期设置**：订阅中心统一控制内核自身是否使用 IPv6 与 DNS 是否向客户端返回 AAAA，前者关闭时后者强制关闭。
- **节点质量评分**：依据内核记录的历史延迟数据，按「延迟 / 稳定性 / 成功率」三项加权给出 0–100 评分。
- **外部面板集成**：内置 MetaCubeXD 与 Zashboard 静态资源托管，可一键切换。

---

## 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go 1.27（标准库），外部依赖仅 2 个：`gorilla/websocket` v1.5.3、`gopkg.in/yaml.v3` v3.0.1 |
| 前端 | Vue 3.5（Composition API）+ TypeScript 5.9 |
| 状态管理 | Pinia 4 |
| 构建工具 | Vite 8（Rolldown）+ vue-tsc |
| 样式 | Tailwind CSS 4（CSS-first 配置，`@tailwindcss/vite` 插件） |
| 国际化 | vue-i18n 11 |
| 图标 | @vicons/ionicons5 |
| 文档站 | VitePress（`docs/`） |

> 后端按功能域拆分为 `backend/internal/` 下的多个包，包结构与依赖方向详见 [AGENTS.md](AGENTS.md) 的「2.1 后端包结构」章节。

---

## 快速开始

### 环境要求

| 依赖 | 版本要求 | 说明 |
|------|----------|------|
| Go | 1.27+ | `backend/go.mod` 声明 `go 1.27` |
| Node.js | ≥ 22.12.0 | 见 `frontend/package.json` 的 `engines` |
| GNU Make | 4.x | 仅用于统一构建入口 |
| Mihomo | 任意近期版本 | 运行时需要，用于实际代理（构建不需要） |

### 构建

```bash
# 完整构建：安装依赖 → 构建前端 → 同步 dist → 编译后端，输出 ./fluxor
make

# 运行构建产物
./fluxor
```

`make` 会自动按依赖顺序补齐所有前置阶段，**无需手动分步执行**。

### 常用命令

```bash
make            # 清理旧 dist → 构建前端 → 同步 → 编译后端，输出 ./fluxor
make V=1.0.0    # 同上，并把版本号 1.0.0 注入二进制
make clean      # 清理构建产物（frontend/dist、backend/dist、./fluxor）
```

`make` 每次都会先清除旧的 `frontend/dist` 与 `backend/dist` 再重新构建，避免残留旧产物。

### 构建变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `V` | `dev` | 注入二进制的版本号（`-ldflags -X`） |
| `BIN` | `fluxor` | 输出二进制名称 |
| `GO` / `NPM` | `go` / `npm` | 工具链命令 |

交叉编译直接交给 Go，设置 `GOOS` / `GOARCH` 环境变量即可：

```bash
make GOOS=linux GOARCH=arm64
```

### 版本号

版本号在**编译期由后端注入**（不再是前端 `package.json`）：

```bash
make V=1.2.3
```

注入值写入 `internal/buildinfo.Version`，前端「关于」页与更新检查改为经后端接口
`GET /app-version` 获取，`/check-update`、`/update-self` 也不再需要前端传 `?current=`。
`/check-update` 另支持 `?force=1`：忽略后端 10 分钟缓存冷却直接回源查询，成功后刷新冷却
（「关于」弹窗里的「检查更新」按钮走这一条）。

未指定 `V` 时注入 `dev`；此时后端会判定「版本未知」，更新检查不会误报有新版本。

---

## 运行时配置

### 运行模式

| 模式 | 启用方式 | 监听 | 主要路径 |
|------|----------|------|----------|
| `fnos`（默认） | 无参数或 `-f` / `--fnos` | Unix Socket `/var/apps/Fluxor/target/app.sock` | `/var/apps/Fluxor/…` |
| `openwrt` | `-w` / `--openwrt` | TCP `0.0.0.0:18080` | `/etc/fluxor/…` |

### 命令行参数

```
-w, --openwrt   以 OpenWrt 模式运行
-f, --fnos      以 fnos 模式运行（默认）
-a, --addr      指定 TCP 监听地址（优先级高于模式默认值）
```

### 环境变量

所有路径均可通过环境变量覆盖，优先级高于模式默认值：

`SOCKET_PATH`、`BASE_URL`、`FLUXOR_ADDR`、`FLUXOR_DATA_DIR`、`FLUXOR_BIN_DIR`、`CORE_BIN`、`CORE_SOCKET`、`META_DIR`、`ZASH_DIR`、`CONFIG_TARGET`、`CORE_WORK_DIR`

`FLUXOR_DATA_DIR` 是运行数据目录：配置文件（`settings.json` / `rules.json` / `tunnels.json` / `subscription-meta.json` / `tproxy.json` / `tun.json`）与 `fluxor.log`（后端全部运行日志）都生成在该目录下；`fluxor.pid` 与 `core.pid` 默认同址，但 openwrt 模式固定在 `/var/run/`（PID 属运行时状态，不写 flash），不受该变量影响。默认值：fnos 取环境变量 `TRIM_PKGVAR`（未注入时回退 `/var/apps/Fluxor/var`），openwrt 为 `/etc/fluxor/`。以上文件不再支持逐个指定路径（原 `FLUXOR_PID_FILE` / `CORE_PID_FILE` / `FLUXOR_CONFIG_FILE` / `INFO_LOG_FILE` 已移除）。日志等级由 `FLUXOR_LOG_LEVEL` 控制（`debug` / `info` / `warn` / `error`，缺省 `info`）；日志由后端独占写入 `fluxor.log`，启动脚本不应再重定向面板的 stdout/stderr。

### 默认端口

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `proxy_port` | 7890 | 混合代理端口（mixed-port） |
| `tproxy_port` | 7898 | TProxy 透明代理端口 |
| `panel_port` | 9090 | 外部控制面板端口（external-controller） |

配置按类别分为 6 个文件（fnos 模式下位于 `/var/apps/Fluxor/var/`），布局与字段说明见 [docs/config/fluxor-json.md](docs/config/fluxor-json.md)。

---

## 项目结构

```
fluxor/
├── Makefile          # 统一构建入口
├── backend/          # Go 后端（独立 module）
│   ├── main.go       #   入口：参数解析、路由注册、监听、优雅退出、go:embed
│   └── internal/     #   按功能域拆分的实现（config/configgen/core/tproxy/tun/subscription/…）
├── frontend/         # Vue 3 + Vite 前端源码，构建产物内嵌进后端
└── docs/             # VitePress 文档站
```

---

## 文档

- **[AGENTS.md](AGENTS.md)** — 系统架构、后端包结构与依赖方向、API 路由对照表、前后端通信机制、TProxy 安全规约、前端开发约束。
- **[docs/](docs/index.md)** — 用户文档站：快速开始、功能说明、配置详解与 FAQ。

---

## 许可

本项目采用 [LICENSE](LICENSE) 中声明的许可协议。
