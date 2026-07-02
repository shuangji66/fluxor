# 持久化配置 (fluxor.json)

Fluxor 的设计核心之一是不引入复杂的数据库系统。所有的全局参数、机场订阅链接及拉取元数据，都持久化存储在一个 JSON 文件中。

---

## 配置文件路径
默认保存在 `/var/apps/Fluxor/var/fluxor.json` 中。该文件在每次点击网页上的「保存并应用」时由程序在后台自动生成并覆盖写入，禁止在内核运行时手动编辑，防止文件锁冲突或格式损坏。

---

## 配置字段格式参考

以下是 `fluxor.json` 文件的完整 JSON 结构示例，用以帮助您理解底层参数的流动和映射：

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
  "subscriptions": [
    {
      "name": "my_airport",
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

## 核心字段详解

* **`proxy_port`**：混合富强端口（Mixed Port），该端口同时支持 HTTP 和 SOCKS5 富强协议，默认端口为 `7890`。
* **`tproxy_port`**：透明富强网关端口（TProxy Port），专门供 nftables 防火墙重定向劫持流量使用，默认端口为 `7898`。
* **`panel_port`**：外部控制器监听端口，外置面板（如 MetaCubeXD）会通过该端口发送 HTTP API/WS 请求，默认为 `9090`。
* **`panel_secret`**：面板连接的安全验证密钥。
* **`rule_group`**：当前选用的规则分流集模板名称（取值为 `base` 或 `full`）。
* **`ui_panel`**：关联的外部控制面板界面类型（取值为 `metacubexd` 或 `zashboard`）。
* **`mode`**：订阅加载方式模式（取值为 `merge` 融合模式 或 `switch` 切换模式）。
* **`active_subscription`**：当处于切换模式（`switch`）时，当前正在生效并激活的订阅名。
* **`subscriptions`**：机场订阅数组，其中：
  * **`update_interval`**：以秒为单位的自动静默更新间隔（0 表示不自动更新）。
  * **`health_interval`**：以秒为单位的后台健康测速频率。
  * **`prefix`**：节点名称正则过滤关键字。
  * **`subscription_info`**：存储机场返回的流量配额（包含已用上传、已用下载、总配额以及过期时间戳），会在“订阅管理”页作为流量卡片高亮渲染。
