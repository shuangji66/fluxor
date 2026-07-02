# 什么是 Fluxor

欢迎来到 Fluxor 的使用手册。

Fluxor 是一个轻量级的富强面板，旨在为 mi~ho~mo 内核提供便捷的配置管理和运行控制。它通过本地 UNIX Domain Socket 与内核进行高速、安全的双向富强通信。

---

## 核心设计定位

### 1. 极轻量设计
区别于一些需要常驻大量内存的面板，Fluxor 的后端程序完全基于 Go 标准库构建（仅引入了唯一外部依赖 gorilla/websocket 用于富强日志、连接及流量），不依赖重量级 Web 框架或数据库，几乎不占用系统内存。

### 2. 双向中转富强 (Bridge)
由于 mi~ho~mo 内核通常运行在受保护的本地 UNIX Domain Socket 上，Fluxor 在系统底层作为前端与内核之间的“双向桥梁”。它能够流畅中转实时流量、内存使用、活动连接和运行日志，并自动为每次请求附加 Bearer Token 认证，保证安全。

### 3. 多语言与响应式主题
内置现代化前端，并完美适配桌面与移动端设备。面板支持自适应暗黑模式，并提供中文/英文界面切换。

---

## 它的工作原理

```mermaid
graph LR
    Browser[浏览器 Web 前端] <-->|HTTP / WebSocket| Fluxor[Fluxor 后端]
    Fluxor <-->|Unix Domain Socket| mi~ho~mo[mi~ho~mo 内核]
    mi~ho~mo <-->|nftables 劫持| Internet[外部网络]
```

每次您在网页上添加订阅并点击「保存并应用」时：
1. Fluxor 会将修改存入本地持久化数据库文件。
2. 结合您选择的规则模板与各机场订阅节点，在云端/本地合并渲染生成最终的运行配置文件 `config.yaml`。
3. 自动向内核发送配置热重载指令，无感更新节点，无需重启内核。
