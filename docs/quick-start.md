# 快速开始

本指南将帮助您快速了解如何安装并启动 Fluxor 面板，以及如何完成首次代理配置。

---

## 前提条件

在使用 Fluxor 之前，请确认您的系统满足以下条件：
* **操作系统**：推荐使用 Linux 环境（如 Ubuntu/Debian 等，并已安装 nftables 以支持透明代理）。
* **内核准备**：已下载对应系统架构的 Mihomo (Clash.Meta) 二进制文件（推荐 v1.18.0 或更高版本）。

---

## 启动与运行环境变量（参考）

Fluxor 后端不需要复杂的数据库依赖，只运行一个单一二进制文件，其路径与端口完全由环境变量进行配置。在运行后端二进制程序前，您可以根据需要导出以下环境变量（通常情况下已由系统安装包自动处理）：

```bash
# 设置管理面板访问的前缀（适用于飞牛 OS 等应用网关反代环境）
export BASE_URL="/app/Fluxor"

# 设置面板监听的本地 Unix Socket 路径
export SOCKET_PATH="/var/apps/Fluxor/target/app.sock"

# 设置底层内核使用的 Unix Socket 路径
export CORE_SOCKET="/var/apps/Fluxor/target/core.sock"

# 设置内核二进制主程序路径
export CORE_BIN="/var/apps/Fluxor/target/bin/mihomo"

# 设置内核运行时的 PID 文件路径
export CORE_PID_FILE="/var/apps/Fluxor/var/core.pid"

# 设置持久化保存您的订阅与参数配置的文件路径
export FLUXOR_CONFIG_FILE="/var/apps/Fluxor/var/fluxor.json"

# 设置内核工作目录（存放 Geo 数据库、测速缓存、节点文件等）
export CORE_WORK_DIR="/var/apps/Fluxor/shares/Fluxor"

# 设置最终渲染生成的内核配置文件路径
export CONFIG_TARGET="/var/apps/Fluxor/shares/Fluxor/config.yaml"

# 设置日志存储文件路径
export INFO_LOG_FILE="/var/apps/Fluxor/shares/Fluxor/info.log"

# 可选：如果希望启用传统的 TCP 监听端口（直接通过 IP 访问面板），可设置此项
# export FLUXOR_ADDR="0.0.0.0:8080"
```

配置好路径环境变量并赋予执行权限后，在后台启动服务即可：
```bash
chmod +x fluxor
chmod +x bin/mihomo
./fluxor
```

---

## 首次配置三步上手

后端运行起来并进入 Web 界面后，请按以下步骤快速激活代理：

### 第一步：添加订阅并保存应用
1. 点击侧边栏的「订阅配置」页面。
2. 找到「订阅列表」，点击「添加订阅」。输入您的订阅名称（别名）和机场订阅 URL。
3. 可选配置检测间隔（默认 86400 秒即 24 小时自动拉取）及前缀过滤。
4. 在基础参数区：
   * 设定混合代理端口（默认 7890）和面板控制端口（默认 9090）。
   * 选择您想要的规则集（Base 基础版 或 Full 完整版）。
   * 切换订阅模式：融合模式（Merge，混合使用全部订阅的节点）或切换模式（Switch，点击卡片高亮选中单个订阅）。
5. 点击页面最下方的「保存并应用」按钮。系统会自动合并规则并启动内核。

### 第二步：节点测速与切换
1. 点击侧边栏进入「代理管理」页面。
2. 找到生成的各个分流策略组，点击可以展开节点列表。
3. 点击顶部的「测试全部延迟」，或者点击策略组卡片右上角的小闪电进行并发测速（系统强制并发最大为 10）。
4. 测速完成后根据红黄绿延迟颜色，点击选定您满意的节点即完成切换。

### 第三步：全局流量劫持 (TUN/TProxy)
如果需要您的软路由或服务器系统自动代理所有进出站流量，请前往「内核配置」页面：
* **TUN 模式**：打开 TUN 模式开关，由虚拟网卡接管系统层流量。引擎可以选择 System 或 gVisor。
* **TProxy 模式**：确认 TProxy 监听端口不为 0 后，打开透明代理开关，并配置您需要的例外 IP 列表。
* **防冲突提示**：TUN 模式与 TProxy 透明代理处于**互斥状态**。开启 TUN 会自动重置并关闭 TProxy；同样，开启 TProxy 会自动清空并关闭 TUN 虚拟网卡。
