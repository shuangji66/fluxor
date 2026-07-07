# 目录与路径配置

本页面为您介绍系统的默认目录结构以及在运行/启动服务时可供调配的系统环境变量。

---

## 默认目录结构

如果您是通过安装包安装或处于飞牛 OS 默认环境下运行，底层的程序、库文件、订阅以及日志的默认路径分布如下：

```text
/var/apps/Fluxor/
├── target/
│   ├── app.sock              # Fluxor 自身监听的 UNIX Socket
│   ├── core.sock             # mi鸿蒙 内核监听的 UNIX Socket
│   └── bin/
│       ├── mi鸿蒙            # mi鸿蒙 内核二进制执行程序
│       └── fluxor            # Fluxor 后端二进制管理程序
├── var/
│   ├── core.pid              # 内核进程的 PID 运行记录文件
│   ├── fluxor.pid            # fluxor面板 PID 运行记录文件
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
export CORE_BIN="/var/apps/Fluxor/target/bin/mi鸿蒙"

# 设置fluxor二进制程序的路径
export FLUXOR_BIN_DIR="/var/apps/Fluxor/target/bin/"

# 设置内核运行时的 PID 进程锁定文件路径
export CORE_PID_FILE="/var/apps/Fluxor/var/core.pid"

# 设置fluxor运行时的 PID 进程锁定文件路径
export FLUXOR_PID_FILE="/var/apps/Fluxor/var/fluxor.pid"

# 设置持久化保存您订阅与全局端口密钥等设置的文件路径
export FLUXOR_CONFIG_FILE="/var/apps/Fluxor/var/fluxor.json"

# 设置内核的主工作目录（用于存放 Geo 数据库、测速缓存、临时文件等）
export CORE_WORK_DIR="/var/apps/Fluxor/shares/Fluxor"

# 设置最终渲染生成的 mi鸿蒙 运行配置文件的目标路径
export CONFIG_TARGET="/var/apps/Fluxor/shares/Fluxor/config.yaml"

# 设置系统日志的保存文件路径
export INFO_LOG_FILE="/var/apps/Fluxor/shares/Fluxor/info.log"

# 可选：metacubexd 外置面板路径
# export META_DIR="/var/apps/Fluxor/shares/ui/meta"

# 可选：zashboard 外置面板路径
# export ZASH_DIR="/var/apps/Fluxor/shares/ui/zash"              

# 可选：如果需要通过传统的端口号（如 http://IP:8080）访问面板，可设置此项
# export FLUXOR_ADDR="0.0.0.0:8080"
```
若您是在openwrt等嵌入式环境中部署，可创建`/etc/fluxor`目录，将fluxor和mi鸿蒙内核放置在此目录，以`./fluxor -w`启动以下预设的openwrt运行设置或自行修改相应环境变量：

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
export CORE_BIN="/etc/fluxor/mi鸿蒙"

# 设置fluxor二进制程序的路径
export FLUXOR_BIN_DIR="/etc/fluxor/"

# 设置内核运行时的 PID 进程锁定文件路径
export CORE_PID_FILE="/var/run/core.pid"

# 设置fluxor运行时的 PID 进程锁定文件路径
export FLUXOR_PID_FILE="/var/run/fluxor.pid"

# 设置持久化保存您订阅与全局端口密钥等设置的文件路径
export FLUXOR_CONFIG_FILE="/etc/fluxor/fluxor.json"

# 设置内核的主工作目录（用于存放 Geo 数据库、测速缓存、临时文件等）
export CORE_WORK_DIR="/etc/fluxor"

# 设置最终渲染生成的 mi鸿蒙 运行配置文件的目标路径
export CONFIG_TARGET="/etc/fluxor/config.yaml"

# 设置系统日志的保存文件路径
export INFO_LOG_FILE="/etc/fluxor/info.log"

# 可选：metacubexd 外置面板路径
# export META_DIR="/etc/fluxor/ui/meta"

# 可选：zashboard 外置面板路径
# export ZASH_DIR="/etc/fluxor/ui/zash"              
```
