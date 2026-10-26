# 配置文件布局

Fluxor 的设计核心之一是不引入复杂的数据库系统：所有状态都持久化在本地的 JSON 文件中，可以直接查看、备份与手工修复。

**这些文件按类别分开存放，而不是挤在一个大文件里**——不同数据的写入者、写入频率与体积差异很大（全局设置很少改、机场元数据每次更新都写、TProxy 预填模板上万字节），混在一起会带来三个后果：改一条规则要重写整份文件、定时更新与界面操作互相争锁、任何一次写坏都要靠肉眼在一大坨 JSON 里定位。

---

## 文件一览

全部位于运行数据目录 `FLUXOR_DATA_DIR`（飞牛 OS 即 `/var/apps/Fluxor/var`，见[目录与路径配置](file-structure.md)）：

| 文件 | 内容 | 主要写入者 | 丢失后果 |
|------|------|-----------|----------|
| `settings.json` | 全局参数 + 订阅注册表（名称 / 链接 / 间隔 / 前缀）+ 自定义模式的手工节点 | 订阅中心的「保存并应用」、订阅增删改、手工节点编辑 | 需重新配置 |
| `rules.json` | 三作用域自定义规则：`merge.<base\|full>`（融合档位）、`custom_mode`（自定义模式）、`subscriptions.<订阅名>`（切换模式） | 自定义规则弹窗的增 / 改 / 排序 / 删 | 需重新添加规则 |
| `tunnels.json` | 三作用域流量隧道，结构与 `rules.json` 完全对应 | 自定义规则弹窗里的隧道列表 | 需重新添加隧道 |
| `subscription-meta.json` | 每个订阅的 `updated_at` 与机场元数据（流量 / 到期 / 节点数） | 手动更新、定时更新、启动时的订阅检查 | **可自动重抓** |
| `tproxy.json` | TProxy 三个开关（Enabled / ProxyLocal / IPv6）+ 两条绕过列表 | 透明代理卡片里的开关与高级设置 | 开关回默认、列表回预填模板 |
| `tun.json` | TUN 运行参数（协议栈 / 设备名 / DNS 处理方式 / IPv6 接管），**稀疏**且**不含 enable** | TUN 高级设置弹窗 | 参数回默认（不影响开关，TUN 开关本就不落盘） |

共同约定：

| 事项 | 说明 |
|------|------|
| 写入时机 | 只有对应功能被真正修改时才写；未修改的文件不会被触碰（例如更新订阅元数据只重写 `subscription-meta.json`） |
| 写入方式 | 每个文件由**唯一的写入者**在文件锁内「读—改—写」，落盘走「临时文件 + rename」，不会出现写到一半的半截文件 |
| 默认值不落盘 | 未改动的内容不写入文件（TProxy 绕过列表的预填模板、空规则列表等都不落盘），因此文件里只有「你真正配置过的东西」 |
| 内容损坏 | 面板会把损坏内容备份为 `<文件名>.corrupt-<时间戳>`，以默认值启动并**停止写入该文件**（避免覆盖现场），修好或删除后重启即可恢复 |
| 建议 | **面板运行期间不要手工编辑这些文件**，避免覆盖掉正在写入的内容 |

> 旧版本使用单个 `fluxor.json` 保存全部内容。升级后首次启动会自动把它拆分成上述 6 个文件中的前 5 份（`tun.json` 是后加的参数文件，没有历史数据、不参与迁移），并把原文件改名为 `fluxor.json.migrated-<时间戳>` 保留（确认无误后可自行删除）。想回滚：停掉面板，把归档文件改回 `fluxor.json` 并删除新生成的文件即可。

---

## 配置字段格式参考

以下是**全部字段的结构示例**。为便于对照，它们写在同一个 JSON 块里；实际落盘时按上表分散在 6 个文件中（`settings.json` / `rules.json` / `tunnels.json` / `subscription-meta.json` / `tproxy.json` / `tun.json`）：

```json
{
  "proxy_port": 7890,
  "tproxy_port": 7898,
  "panel_port": 9090,
  "panel_secret": "your-panel-controller-secret",
  "rule_group": "base",
  "ui_panel": "metacubexd",
  "meta_backend_url": "",
  "mode": "merge",
  "active_subscription": "",
  "ipv6": true,
  "dns_ipv6": true,
  "merge_custom_rules": {
    "base": [
      {
        "id": "9f1c0b7a2d3e4f58",
        "type": "DOMAIN-SUFFIX",
        "payload": "ads.example.com",
        "target": "🎯 全球直连",
        "position": "before"
      }
    ],
    "full": [
      {
        "id": "1a2b3c4d5e6f7788",
        "type": "RULE-SET",
        "payload": "ads",
        "target": "🛑 广告域名",
        "position": "before"
      }
    ]
  },
  "merge_tunnels": {
    "base": [
      {
        "id": "7c9d1e2f3a4b5c6d",
        "network": ["tcp", "udp"],
        "address": "127.0.0.1:6553",
        "target": "8.8.8.8:53",
        "proxy": "🚀 节点选择",
        "enabled": true
      }
    ],
    "full": []
  },
  "custom_mode_tunnels": [],
  "tproxy_enabled": false,
  "tproxy_dst_exceptions": [],
  "tproxy_src_exceptions": [],
  "tproxy_proxy_local": false,
  "tun": {
    "stack": "system",
    "ipv6": "auto"
  },
  "subscriptions": [
    {
      "name": "my_airport",
      "url": "https://example.com/sub/link",
      "update_interval": 86400,
      "health_interval": 300,
      "prefix": "香港",
      "custom_rules": [
        {
          "id": "a345c008a8f85685",
          "type": "DOMAIN-SUFFIX",
          "payload": "ads.example.com",
          "target": "REJECT",
          "position": "after"
        },
        {
          "id": "602a1b0615fb591a",
          "type": "IP-CIDR",
          "payload": "1.1.1.0/24",
          "target": "🚀 节点选择",
          "position": "before",
          "no_resolve": true
        }
      ],
      "tunnels": [
        {
          "id": "b1c2d3e4f5a60718",
          "network": ["tcp"],
          "address": "127.0.0.1:7777",
          "target": "dns.google:53",
          "enabled": true
        }
      ],
      "updated_at": "2026-07-02T15:00:00Z",
      "subscription_info": {
        "upload": 10737418240,
        "download": 53687091200,
        "total": 536870912000,
        "expire": 1782979200
      }
    }
  ]
}
```

---

## 核心字段详解

### 全局参数（`settings.json`）

* **`proxy_port`**：混合富强端口（Mixed Port），该端口同时支持 HTTP 和 SOCKS5 富强协议，默认 `7890`。取值为 `0` 表示禁用。
* **`tproxy_port`**：透明富强网关端口（TProxy Port），专门供 nftables 防火墙重定向劫持流量使用，默认 `7898`。
* **`panel_port`**：外部控制器监听端口，外置面板（如 MetaCubeXD）会通过该端口发送 HTTP API/WS 请求，默认 `9090`。
* **`panel_secret`**：保护内核 **TCP 外部控制端口**（`panel_port`）的密钥，外置面板（MetaCubeXD / Zashboard）或其它客户端直连该端口时需要提供。密钥不会下发给浏览器；Fluxor 面板自身与内核之间的通信走 UNIX Socket，内核对 unix 来源默认信任、不校验密钥，因此该链路不携带认证头。
* **`rule_group`**：当前选用的规则分流集模板名称，取值为 `base`（标准）或 `full`（详细）。
* **`ui_panel`**：关联的外部控制面板界面类型，取值为 `metacubexd` 或 `zashboard`。
* **`meta_backend_url`**：外部面板回连面板后端所用的地址，留空表示使用默认推导值。
* **`mode`**：订阅加载方式，取值为 `merge`（融合模式）或 `switch`（切换模式）。详见 [订阅工作模式](./subscription-modes)。
* **`active_subscription`**：当处于切换模式（`switch`）时，当前正在生效并激活的订阅名。
* **`ipv6`**：**生成期**的 IPv6 总开关，写入 `config.yaml` 顶层 `ipv6`，决定内核自身解析与出站是否使用 IPv6。默认 `true`；改动需点「保存并应用」重载生效。
* **`dns_ipv6`**：**生成期**设置，写入 `config.yaml` 的 `dns.ipv6`，决定内核 DNS 是否向客户端返回 AAAA。默认 `true`；`ipv6` 为 `false` 时它必然为 `false`（后端强制归一化，避免「内核不用 IPv6、却把 AAAA 交给客户端」）。
* **`merge_custom_rules`**：**融合模式**的自定义规则，**按规则集档位分开存放**（`base` 标准 / `full` 详细）。两个档位的代理组与规则集完全不同，因此各存一份、各自生效——只有当前 `rule_group` 那一份会写进 `config.yaml`，另一份保持惰性。字段结构与订阅级 `custom_rules` 完全一致（见下方「订阅数组」），由「订阅配置」页的自定义规则弹窗按档位页面维护。

* **`merge_tunnels`**：**融合模式**的流量隧道，**按规则集档位分开存放**（`base` 标准 / `full` 详细），理由与 `merge_custom_rules` 相同——两档的代理组不同，隧道引用的 `proxy` 也各不相同。字段结构见下方「订阅数组」的 `tunnels`；只有当前 `rule_group` 那一份会写进 `config.yaml` 的 `tunnels` 块。
* **`custom_mode_tunnels`**：**自定义模式**的流量隧道（独立一份，与融合模式互不影响）。该模式的节点是手工添加的静态 `proxies`，因此隧道的 `proxy` 里额外允许引用手工节点名。

### TProxy 状态字段（`tproxy.json`）

这几个字段由 TProxy 功能单独维护，通常不需要手工编辑。文件里的键名与下表的字段名对应（去掉 `tproxy_` 前缀，因为整个文件都属于 TProxy）：

| 文件中的键 | 说明 |
|-----------|------|
| `enabled` | 对应 `tproxy_enabled` |
| `proxy_local` | 对应 `tproxy_proxy_local` |
| `ipv6` | 对应 `tproxy_ipv6` |
| `dst_exceptions` / `src_exceptions` | 对应 `tproxy_dst_exceptions` / `tproxy_src_exceptions`；**与预填模板一致时不会写进文件** |

* **`tproxy_enabled`**：透明富强开关的持久化值。**注意**：nftables 规则不跨重启存活，因此面板每次启动都会把该值**归零**并清理残留规则，确保「面板显示的状态」与「系统实际生效的规则」一致。
* **默认值**：`enabled` / `proxy_local` / `ipv6` 三个开关**默认全关**；只有用户显式开启才会落盘为 `true`。
* **`ipv6`**：是否随 TProxy 一起下发 IPv6 家族规则。它还受订阅中心「IPv6 支持」总开关约束：总开关关闭时只跳过规则、不改这里的偏好。
* **`tproxy_dst_exceptions`** / **`tproxy_src_exceptions`**：目的 / 源绕过列表（字段名沿用历史命名，界面统一叫「绕过」）。
* **`tproxy_proxy_local`**：是否接管面板主机自身的出站流量（界面上的「接管本机流量」开关）。

> 旧版本曾使用 `tproxy_exceptions` 单一字段，现在会自动迁移到分离的源/目的两个字段。

### TUN 运行参数（`tun.json`）

TUN 的**参数**与**开关**分开存放：参数在这里，开关只活在运行时（不落盘、不回放）。

| 文件中的键 | 默认值 | 说明 |
|-----------|--------|------|
| `stack` | `mixed` | TUN 协议栈：`gvisor` / `system` / `mixed` / `mips` |
| `device` | 空（自动命名） | TUN 设备名，最多 15 个字符 |
| `dns_mode` | `hijack` | DNS 处理方式：`hijack` / `native` / `disabled` |
| `ipv6` | `off` | 是否给 TUN 下发 IPv6 地址：`off` / `auto`（按宿主能力，宿主允许给新接口挂 IPv6 才下发） |

**稀疏存储**：只写与默认值不同的键，改回默认即删键；文件里没有 `enable`。`config.yaml` 里的 `tun.enable` 恒为 `false`，TUN 只能由用户在「配置」页手动开启，内核启动/重启或配置重载后会自动关闭。示例中上方 `tun` 块只展示两个非默认键，实际文件里也只有这类键。

### 订阅数组（`settings.json` 的 `subscriptions` + `subscription-meta.json`）

订阅的**身份与拉取参数**在 `settings.json` 里，**更新元数据**（`updated_at` / `subscription_info`）在 `subscription-meta.json` 里，而**该订阅的自定义规则与隧道**分别在 `rules.json` / `tunnels.json` 的 `subscriptions.<订阅名>` 下。

* **`name`**：订阅名，作为节点的来源标识与文件名依据。
* **`url`**：机场订阅链接（`http(s)://`）。
* **`update_interval`**：以秒为单位的自动静默更新间隔（`0` 表示不自动更新）。
* **`health_interval`**：以秒为单位的后台健康测速频率。
* **`prefix`**：**节点名称前缀**（不是过滤正则）。填写后生成的节点名会自动带上该前缀，对应内核 `proxy-provider` 的 `override.additional-prefix`，用于多订阅时区分节点来源。
* **`custom_rules`**：**切换模式**下该订阅的自定义规则列表（融合模式不使用，改用顶层的 `merge_custom_rules`）。每条规则由结构化字段组成，写入 `config.yaml` 时由面板组装成内核规则行：

  | 字段 | 说明 |
  |------|------|
  | `id` | 面板生成的稳定标识，用于修改 / 排序 / 删除单条规则 |
  | `type` | 规则类型（如 `DOMAIN-SUFFIX`、`RULE-SET`），取值受面板白名单限制 |
  | `payload` | 规则取值（如 `ads.example.com`、`1.1.1.0/24`、规则集名称） |
  | `target` | 命中目标：该订阅自带的代理组，或 `DIRECT` / `REJECT` / `PASS` |
  | `position` | `before`（默认，插到规则最前，优先级高于订阅自带规则）或 `after`（插在最后一条 `MATCH` 之前） |
  | `no_resolve` | 仅 IP 类与 `RULE-SET` 规则使用，跳过域名解析 |

  数组顺序即生效顺序：`before` 组的规则按数组顺序插在规则最前，`after` 组的规则按数组顺序插在最后一条 `MATCH` 之前；界面上的上/下移动只在同组内交换，并即时写回本文件。

  规则由「订阅配置」页的自定义规则弹窗维护，逐条即时写入本文件；目标已失效的规则会被跳过而不写入 `config.yaml`（内核遇到无法解析的目标会拒绝加载整份配置），并在弹窗中标注原因。详见 [订阅工作模式](./subscription-modes)。
* **`tunnels`**：**切换模式**下该订阅的流量隧道列表（融合模式不使用，改用顶层的 `merge_tunnels`）。每条隧道对应 `config.yaml` 顶层 `tunnels` 块中的一项：

  | 字段 | 说明 |
  |------|------|
  | `id` | 面板生成的稳定标识，用于编辑 / 排序 / 删除 / 开关单条隧道 |
  | `network` | 需要监听的网络类型列表，取值只能是 `tcp` / `udp`（界面上是一个下拉：`tcp+udp` / `tcp` / `udp`） |
  | `address` | 本地监听地址（`host:port`，如 `127.0.0.1:6553`） |
  | `target` | 转发目标地址（`host:port`，如 `8.8.8.8:53`）。只填域名（如 `example.com`）时按 `http` 默认端口补成 `example.com:80`；裸 IP 会被拒绝并要求补端口。内核解析不了裸主机（`socks5.ParseAddr`），而失败只体现为启动时一行日志 + 跳过该条，隧道会静默失效，因此面板在写入前就把目标定形成 `host:port` |
  | `proxy` | 可选的代理组 / 代理节点名；留空表示不指定代理（按正常规则匹配选择出口，**不是**直连） |
  | `enabled` | 启停开关。`false` 的隧道保留在列表里但不写进 `config.yaml`（缺该键的历史数据视为启用） |

  写入 `config.yaml` 时由面板组装成 `tunnels` 块，位置固定在 `rule-providers` / `rules` 之前；地址非法、`proxy` 已不存在或与另一条隧道监听同一地址的条目会被跳过（内核遇到这些情况会拒绝加载配置或起不来监听），并在弹窗中标注原因。详见 [订阅工作模式](./subscription-modes)。
* **`updated_at`**：最近一次成功更新的时间。
* **`subscription_info`**：机场返回的流量配额，包含已用上传（`upload`）、已用下载（`download`）、总配额（`total`）与过期时间戳（`expire`），会在「订阅配置」页作为流量卡片渲染。

---

## 不会保存在这里的配置

以下内容**不在**上述配置文件中，避免产生误解：

| 内容 | 实际存放位置 |
|------|--------------|
| 主题、语言、默认启动页 | 浏览器 `localStorage`（每台设备各自独立） |
| 代理页的排序方式、延迟阈值、节点过滤正则、自动断开开关 | 浏览器 `localStorage` |
| 内核的运行时参数（端口以外的 allow-lan / mode 等） | 内核自身的 `config.yaml`，由面板通过内核 API 修改 |
| TUN 运行参数（协议栈 / 设备名 / DNS 处理 / IPv6 接管） | `tun.json`（见上）；TUN 开关本身**不落盘** |
| 生成期的 IPv6 设置（IPv6 支持 / DNS 返回 AAAA） | `settings.json`，随「保存并应用」写入 `config.yaml` |
| 自定义延迟测试 URL | 浏览器 `localStorage` |
