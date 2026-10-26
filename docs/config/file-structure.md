# 目录与路径配置

本页面为您介绍系统的默认目录结构以及在运行/启动服务时可供调配的系统环境变量。

---

## 默认目录结构

如果您是通过安装包安装或处于飞牛 OS 默认环境下运行，底层的程序、库文件、订阅以及日志的默认路径分布如下：

```text
/var/apps/Fluxor/
├── target -> /vol1/@appcenter/Fluxor/      # 程序目录（软链）
│   ├── app.sock              # Fluxor 自身监听的 UNIX Socket
│   ├── core.sock             # mi鸿蒙 内核监听的 UNIX Socket
│   └── bin/
│       ├── mihomo            # mi鸿蒙 内核二进制执行程序
│       └── fluxor            # Fluxor 后端二进制管理程序
├── var -> /vol1/@appdata/Fluxor/           # 运行数据目录 FLUXOR_DATA_DIR（软链）
│   ├── settings.json         # 全局设置 + 订阅注册表（名称/链接/间隔/前缀）+ 手工节点
│   ├── rules.json            # 三作用域自定义规则（融合档位 / 自定义模式 / 各订阅）
│   ├── tunnels.json          # 三作用域流量隧道（结构同 rules.json）
│   ├── subscription-meta.json# 各订阅的更新时间与机场元数据（丢失可自动重抓）
│   ├── tproxy.json           # TProxy 三个开关与绕过列表（默认全关，仅用户改过才落盘）
│   ├── tun.json              # TUN 运行参数（协议栈/设备名/DNS 处理/IPv6 接管；不含 enable）
│   ├── core.pid              # 内核进程的 PID 运行记录文件
│   ├── fluxor.pid            # 面板自身的 PID 运行记录文件
│   └── fluxor.log            # 面板自身的运行日志（由后端独占写入，见下）
├── etc -> /vol1/@appconf/Fluxor/           # 配置目录（软链）
└── shares/                                 # 数据共享目录
    ├── Fluxor -> /vol1/@appshare/Fluxor/   # 内核工作目录
    │   ├── config.yaml       # 当前在内核中实际生效的配置文件
    │   ├── proxies/          # 各订阅下载的节点源文件存储目录
    │   └── ruleset/          # 规则集模板目录
    └── ui -> /vol1/@appshare/Fluxor/ui/    # 外部面板静态文件
        ├── meta/             # 预置 MetaCubeXD 面板的静态文件目录
        └── zash/             # 预置 Zashboard 面板的静态文件目录
```

> 飞牛 OS 下 `/var/apps/Fluxor/` 内的 `target`、`var`、`etc`、`shares/*` 均为指向 `/vol1/@app*` 的软链接，上表已标注真实落点；两套路径等价，访问哪个都可以。
>
> **面板自身的数据集中在运行数据目录**：`settings.json` / `rules.json` / `tunnels.json` / `subscription-meta.json` / `tproxy.json` / `tun.json`（配置，见「[配置文件布局](/config/fluxor-json)」）、`fluxor.log`（运行日志），以及默认情况下的 `fluxor.pid`、`core.pid`，都生成在 `FLUXOR_DATA_DIR` 下（飞牛 OS 即上表的 `var` 目录，由应用框架注入的 `TRIM_PKGVAR` 指定）。这些文件不再支持逐个指定路径，排查问题只需先确认这一个目录。
>
> 例外：**OpenWrt 模式下 `fluxor.pid` 与 `core.pid` 固定在 `/var/run/`**（tmpfs），配置文件与 `fluxor.log` 位于 `/etc/fluxor/`。PID 属于纯运行时状态，不该写到 flash 上，因此这两个文件不随 `FLUXOR_DATA_DIR` 变化。
>
`fluxor.log` 是**面板自身的运行日志**（不是内核的运行日志，内核的运行日志请在面板的「日志」页面查看）。它由后端独占写入——启动脚本**不应**再把面板的 stdout/stderr 重定向到该文件，否则同一份日志会被拆成两份、位置与内容都不保证一致。每行形如：

```text
2026-01-02 15:04:05.000 INFO  [CORE] mihomo started: pid=1234 bin=/etc/fluxor/mihomo
```

即「本地时间戳（毫秒）+ 等级（DEBUG/INFO/WARN/ERROR）+ 大功能模块标记 + 英文正文」。等级由 `FLUXOR_LOG_LEVEL` 控制（缺省 `info`）。

> 内核二进制按惯例命名为 `mihomo`。若您自行重命名或放在其它位置，请通过下方环境变量覆盖。

---

## 启动与运行环境变量参考

若您是在fnOS环境中部署，或者需要调整后台的工作目录，请在启动二进制文件前，配置并导出以下环境变量：

```bash
# 设置管理面板访问的前缀路径（适用于飞牛 OS 等 NAS 网关环境）
export BASE_URL="/app/Fluxor"

# 设置面板自身监听的本地 Unix Socket 路径
export SOCKET_PATH="/var/apps/Fluxor/target/app.sock"

# 设置底层内核运行绑定的 Unix Socket 路径
export CORE_SOCKET="/var/apps/Fluxor/target/core.sock"

# 设置内核二进制程序的主路径
export CORE_BIN="/var/apps/Fluxor/target/bin/mihomo"

# 设置fluxor二进制程序的路径
export FLUXOR_BIN_DIR="/var/apps/Fluxor/target/bin/"

# 设置统一的运行数据目录：配置文件（settings/rules/tunnels/subscription-meta/tproxy/tun）
# 与 fluxor.log 都生成在此
# 飞牛 OS 下默认取应用框架注入的 TRIM_PKGVAR（即 /var/apps/Fluxor/var）
export FLUXOR_DATA_DIR="/var/apps/Fluxor/var"

# 可选：日志等级（debug / info / warn / error），缺省 info
# export FLUXOR_LOG_LEVEL="info"

# 设置内核的主工作目录（用于存放 Geo 数据库、测速缓存、临时文件等）
export CORE_WORK_DIR="/var/apps/Fluxor/shares/Fluxor"

# 设置最终渲染生成的 mi鸿蒙 运行配置文件的目标路径
export CONFIG_TARGET="/var/apps/Fluxor/shares/Fluxor/config.yaml"

# 可选：metacubexd 外置面板路径
# export META_DIR="/var/apps/Fluxor/shares/ui/meta"

# 可选：zashboard 外置面板路径
# export ZASH_DIR="/var/apps/Fluxor/shares/ui/zash"              

# 可选：如果需要通过传统的端口号（如 http://IP:8080）访问面板，可设置此项
# export FLUXOR_ADDR="0.0.0.0:8080"
```
若您是在openwrt等嵌入式环境中部署，可创建`/etc/fluxor`目录，将 fluxor 与 mi鸿蒙内核放置在此目录，以`./fluxor -w`启动以下预设的openwrt运行设置或自行修改相应环境变量：

```bash
# 通过传统的端口号（如 http://IP:8080）访问面板
# 也可通过`-a 0.0.0.0:18080`运行参数传入
export FLUXOR_ADDR="0.0.0.0:18080"

# 设置管理面板访问的前缀路径
export BASE_URL="/"

# 可选：设置面板自身监听的本地 Unix Socket 路径
# export SOCKET_PATH=""

# 设置底层内核运行绑定的 Unix Socket 路径
export CORE_SOCKET="/etc/fluxor/core.sock"

# 设置内核二进制程序的主路径
export CORE_BIN="/etc/fluxor/mihomo"

# 设置fluxor二进制程序的路径
export FLUXOR_BIN_DIR="/etc/fluxor/"

# 设置统一的运行数据目录：配置文件（settings/rules/tunnels/subscription-meta/tproxy/tun）
# 与 fluxor.log 都生成在此
# 注意：OpenWrt 下 fluxor.pid 与 core.pid 固定在 /var/run/，不随本目录变化
export FLUXOR_DATA_DIR="/etc/fluxor"

# 可选：日志等级（debug / info / warn / error），缺省 info
# export FLUXOR_LOG_LEVEL="info"

# 设置内核的主工作目录（用于存放 Geo 数据库、测速缓存、临时文件等）
export CORE_WORK_DIR="/etc/fluxor"

# 设置最终渲染生成的 mi鸿蒙 运行配置文件的目标路径
export CONFIG_TARGET="/etc/fluxor/config.yaml"

# 可选：metacubexd 外置面板路径
# export META_DIR="/etc/fluxor/ui/meta"

# 可选：zashboard 外置面板路径
# export ZASH_DIR="/etc/fluxor/ui/zash"              
```

---

## 关于环境变量

* **优先级**：环境变量 > 运行模式（`-f` / `-w`）的预设默认值。启动参数 `-a` 指定的监听地址优先级最高。
* **路径不必全部手动设置**：只要选择了运行模式，未设置的路径会自动套用该模式的默认值。上表之所以逐个列出，是为了便于您按需覆盖。
* **`FLUXOR_DATA_DIR`（运行数据目录）**：配置文件（`settings.json` 等 6 份，见[配置文件布局](/config/fluxor-json)）与 `fluxor.log` 生成在该目录下；`fluxor.pid` 与 `core.pid` 默认也在该目录下，**但 OpenWrt 模式固定在 `/var/run/`**（PID 属运行时状态，不该写到 flash 上），不受本变量影响。默认值：飞牛 OS 取应用框架注入的 `TRIM_PKGVAR`（未注入时回退 `/var/apps/Fluxor/var`），OpenWrt 为 `/etc/fluxor/`。原用于单独指定这些路径的环境变量（`FLUXOR_PID_FILE`、`CORE_PID_FILE`、`FLUXOR_CONFIG_FILE`、`INFO_LOG_FILE`）已移除。
* **监听入口至少需要一个**：面板可以通过 Unix Socket（`SOCKET_PATH`）或 TCP 端口（`FLUXOR_ADDR`）对外提供服务，两者至少有一个非空，否则面板无法访问。
  * 飞牛 OS 默认模式：仅监听 Unix Socket，由系统反向代理暴露到 `/app/Fluxor`。
  * OpenWrt 默认模式：监听 `0.0.0.0:18080`，直接以端口访问。
* **`FLUXOR_LOG_LEVEL`（日志等级）**：`debug` / `info` / `warn` / `error`，大小写不敏感，缺省 `info`。日志固定写入 `FLUXOR_DATA_DIR/fluxor.log`，无需（也无法）单独指定日志文件路径。
* **`BASE_URL`**：面板对外访问的统一路径前缀。设为 `/` 会被归一化为空串（即根路径部署）。
* **修改 `FLUXOR_DATA_DIR` 前请先停止面板**：其中的配置文件在运行时被面板读写（`settings.json` 承载订阅与全局设置、`tproxy.json` 承载 TProxy 状态等）。
