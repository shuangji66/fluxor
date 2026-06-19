# Fluxor 工程技术手册

本文件面向技术维护人员，主要用于指导 Fluxor 项目的工程结构图谱、开发环境联调、跨平台构建打包与部署运维。

关于具体的编码避坑指南、双向中转 Context 回收实现细节及 UI 开发设计限制规约，请参阅 [AGENTS.md](AGENTS.md)。

---

## 1. 目录结构与源文件说明

```text
fluxor/
├── main.go                 # 路由及服务入口，默认监听并绑定 Unix Socket
├── assets_vanilla.go       # 条件编译：Vanilla JS 前端静态资源 (go:build !vue)
├── assets_vue.go           # 条件编译：Vue 3 前端静态资源 (go:build vue)
├── handlers_core.go        # Mihomo 本地进程生命周期控制与基础中转代理
├── handlers_api.go         # 代理转发 Mihomo HTTP API 接口
├── handlers_index.go       # 网页主入口 index.html 模板渲染及路由拦截
├── handlers_utils.go       # 统一 JSON 错误与成功格式响应工具
├── subscribe.go            # 订阅链接拉取与配置合并生成逻辑
├── static/                 # Vanilla JS 版本前端源码（原生零构建版）
├── build/                  # 构建产物与打包封包工具链
│   ├── app/
│   │   └── templates/      # 内置配置模板（config_base/lite/full.yaml）
│   ├── build.bat           # Windows 构建脚本（编译后端为 linux 并调用 fnpack）
│   └── build.sh            # Linux 构建脚本（调用 fnpack-1.2.1-linux-amd64）
└── web/                    # Vue 3 前端源码（当前主维护版）
```

---

## 2. 编译构建与分流控制

### 2.1 编译标签（Build Tags）控制
系统通过 Go 条件标签实现前端双版本并存嵌入：
- **默认构建（Vanilla）**：
  直接运行 `go build`（使用 `assets_vanilla.go`），打包的原生静态网页无需任何额外构建步骤。
- **现代化构建（Vue 3）**：
  必须先在 `web/` 下完成 `npm run build`，随后在根目录下添加 Tags 编译：
  ```bash
  go build -tags vue
  ```

### 2.2 跨平台自动化打包
- **Windows 构建**：运行 `build/build.bat`。将编译前端并输出 Linux amd64 后端程序，最终利用 `fnpack.exe` 进行本地打包校验。
- **Linux 构建**：运行 `build/build.sh`。执行相同的打包机制，但会自动寻址并使用 Linux 原生的 `fnpack-1.2.1-linux-amd64` 封包工具。

---

## 3. 本地开发与联调环境

### 3.1 跨域及反向代理说明
- **后端运行**：Go 后端默认在本地 Unix Socket (`/var/apps/Fluxor/target/app.sock`) 上提供服务。
- **前端开发热更新**：
  在 `web/` 目录下执行 `npm run dev`，Vite 默认起在 `5173` 端口上。Vite 在 `vite.config.ts` 中配置了对 `/app/Fluxor` 请求的反向代理，直接中转至后端的开发监听端口，从而避免本地联调跨域。
