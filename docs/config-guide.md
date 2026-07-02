# 配置指南

本指南将深入介绍 Fluxor 的持久化配置机制、订阅工作模式、分流规则模板以及透明网关例外的高级配置。

---

## 配置文件结构参考 (`fluxor.json`)

Fluxor 面板所有的设置、端口、密钥以及添加的订阅信息，都会在点击「保存并应用」时，在后台以 JSON 格式持久化存入磁盘中的 `fluxor.json` 数据库文件中（默认路径为 `/var/apps/Fluxor/var/fluxor.json`）。该文件由程序自动管理，无需手动编辑。

为了让您了解其后台数据流，下面是 `fluxor.json` 文件的结构参考：

```json
{
  "proxy_port": 7890,
  "tproxy_port": 7898,
  "panel_port": 9090,
  "panel_secret": "your-secret-key",
  "rule_group": "base",
  "ui_panel": "metacubexd",
  "meta_backend_url": "",
  "mode": "merge",
  "active_subscription": "",
  "subscriptions": [
    {
      "name": "airport_name",
      "url": "https://example.com/sub/link",
      "update_interval": 86400,
      "health_interval": 300,
      "prefix": "香港",
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

## 订阅模式详解

在「订阅配置」页面，您可以选择以下两种节点加载模式：

### 融合模式 (Merge)
* **逻辑**：当您有多个机场订阅时，融合模式会将这多个订阅中的所有节点混合并打散，全部提取出来填充进同一个配置模板。
* **适用场景**：适合购买了多个低价机场且希望通过不同策略组自动选优、互补备用的用户。

### 切换模式 (Switch)
* **逻辑**：每次只启用其中一个被激活的订阅（Active Subscription）。在网页上，选中的订阅卡片会被亮起蓝色边框以示区别。其它未被选中的订阅节点将被隐藏。
* **适用场景**：适合不同机场用途划分明确（例如一个用于工作查资料，一个用于游戏加速），需要在不同订阅间快速手动切选的用户。

---

## 规则集模板区别

Fluxor 提供了两种分流规则集，供生成 `config.yaml` 配置文件时使用：

* **基础版 (Base)**：标准分流规则，包含常用的 GeoIP / GeoSite 地理位置以及基本的国内外分流规则。它的规则条目较少，内核解析和匹配速度极快，适合轻度代理用户。
* **完整版 (Full)**：精细分流规则，集成了更多的 Rule Providers 规则集。分类非常详尽，包括流媒体、广告拦截、谷歌服务、苹果服务、学术研究、游戏代理等。适合对各种网络服务有精确分流需求的用户。

---

## TProxy 透明网关例外配置

透明网关（TProxy）能够在无需客户端做任何配置的情况下，直接在网关层劫持网络流量并进行代理。这需要您的 Linux 系统支持 nftables 和策略路由。

开启透明代理后，Fluxor 将在系统后台自动注册名为 `ip fluxor_tproxy` 的 nftables 链，并绑定 `fwmark 1` 进行策略路由转发（策略路由表为 100）。为了让特定的局域网设备或公网流量绕过代理直连，您可以在「配置」页配置例外列表：

* **目的例外**：用于配置不希望被劫持的目标地址或端口。
  * 支持输入具体的 IP、CIDR 子网网段或特定端口（例如：`53` 代表拦截 DNS 请求但直接放行，`tcp:80` 代表不对 80 端口进行代理劫持）。
* **源例外**：用于配置不希望被劫持的局域网客户端 IP 地址（仅入站）。
  * 支持输入具体的客户端 IP 地址或 CIDR 网段（例如：`192.168.1.100`，表示该局域网主机的流量将直接直连，绝对不会经过 Mihomo 内核代理）。
* **本机出站代理**：
  * 面板默认只代理局域网内其他设备的入站流量。如果您希望运行 Fluxor 面板的主机自身发出的外部流量也通过代理，请开启“本机代理”开关（后台将自动在系统防火墙中下发针对本机出站流量的 nftables 规则）。

---

## 飞牛 OS 默认路径结构参考

如果您使用的是飞牛 OS 或者是基于默认目录路径部署的应用，您可以参考以下系统路径结构，以便在后台直接定位文件：

```text
/var/apps/Fluxor/
├── target/
│   ├── app.sock              # Fluxor 自身监听的 UNIX Socket
│   ├── core.sock             # Mihomo 内核监听的 UNIX Socket
│   └── bin/
│       ├── mihomo            # Mihomo 内核二进制执行程序
│       └── fluxor            # Fluxor 后端二进制管理程序
├── var/
│   ├── core.pid              # 内核进程的 PID 运行记录文件
│   └── fluxor.json           # 面板设置与订阅持久化 JSON 数据库
└── shares/
    ├── Fluxor/
    │   ├── config.yaml       # 当前在内核中实际生效的配置文件
    │   ├── info.log          # 内核运行产生的日志文件
    │   └── proxies/          # 订阅模式下下载的节点源文件存储目录
    └── ui/
        ├── meta/             # 预置 MetaCubeXD 面板的静态文件目录
        └── zash/             # 预置 Zashboard 面板的静态文件目录
```
