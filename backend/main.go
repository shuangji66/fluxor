package main

import (
	"embed"
	"fluxor/internal/appupdate"
	"fluxor/internal/buildinfo"
	"fluxor/internal/config"
	"fluxor/internal/configgen"
	"fluxor/internal/core"
	"fluxor/internal/dashapi"
	"fluxor/internal/delaytest"
	"fluxor/internal/httpx"
	"fluxor/internal/logx"
	"fluxor/internal/netinfo"
	"fluxor/internal/quality"
	"fluxor/internal/subscription"
	"fluxor/internal/tproxy"
	"fluxor/internal/tun"
	"fluxor/internal/web"
	"fluxor/internal/wsproxy"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// staticFS 内嵌前端构建产物 (frontend/dist) 的文件系统根
var staticFS fs.FS

//go:embed dist
var distFS embed.FS

func init() {
	// dist 由 Makefile 在构建前端后同步到 backend/dist，并在此内嵌
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	staticFS = sub
}

// HTTP 服务器的超时与限额。
//
// 面板没有任何认证，因此不能假设调用方都是善意的：裸 http.Serve 既不限制握手/读取
// 时长，也不限制请求体大小，一个「只连不发」或「发超大 body」的客户端就能长期占用
// 内存与连接。
//
// 取值取舍：
//   - ReadHeaderTimeout 5s：只覆盖握手阶段，正常浏览器毫秒级完成，慢速攻击封顶；
//   - ReadTimeout 60s：覆盖含请求体在内的整个请求读取。最慢的正常场景是弱网下提交
//     「保存并应用」那几 KB 的 JSON，60s 余量充足；
//   - WriteTimeout 不设：本服务同时承载 SSE（/core/events）与升级后的 WebSocket
//     长连接，任何写超时都会把它们掐断。这两条链路的生命周期由各自的协议管理
//     （SSE 有 25s 心跳，WS 由连接两端的读写错误退出），不需要服务器兜底；
//   - IdleTimeout 60s：回收空闲的 keep-alive 连接。
const (
	serverReadHeaderTimeout = 5 * time.Second
	serverReadTimeout       = 60 * time.Second
	serverIdleTimeout       = 60 * time.Second
	// maxRequestBody 请求体上限。所有写接口的载荷都是几 KB 的 JSON（订阅链接、自定义
	// 规则/隧道、内核配置片段），1 MiB 留出两个数量级的余量；再大的请求不可能是合法
	// 操作，直接由 MaxBytesReader 拒掉，不必先读进内存。
	maxRequestBody int64 = 1 << 20
	// maxRequestHeaderBytes 请求头上限（默认 1 MiB 过于宽松，本项目没有任何长头需求）。
	maxRequestHeaderBytes = 1 << 16
)

// newHTTPServer 构造带超时的服务器。两个监听器（unix socket 与可选 TCP）各用一个。
func newHTTPServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		IdleTimeout:       serverIdleTimeout,
		MaxHeaderBytes:    maxRequestHeaderBytes,
	}
}

// newServerHandler 给全部路由套一层请求体上限。
//
// 放在最外层而不是逐个 handler 里加，是为了不依赖「以后新增的接口记得自己加」：
// 超限时 net/http 会向客户端回 413 并关闭连接，handler 侧读到的是一个明确的
// "http: request body too large" 错误。
func newServerHandler(mux http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
		}
		mux.ServeHTTP(w, r)
	})
}

func main() {
	// === 解析命令行参数 ===
	openwrtMode := false
	fnosMode := false
	customAddr := ""
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-w", "--openwrt":
			openwrtMode = true
		case "-f", "--fnos":
			fnosMode = true
		case "-a", "--addr":
			if i+1 < len(args) {
				customAddr = args[i+1]
				i++
			} else {
				// 此时日志系统尚未初始化（日志路径由运行模式决定，晚于参数解析）
				fmt.Fprintln(os.Stderr, "usage error: -a/--addr requires an address")
				os.Exit(1)
			}
		default:
			// 忽略未知参数
		}
	}

	// 确定运行模式：-w 优先，否则若指定 -f 或无参数则 fnos
	mode := "fnos"
	if openwrtMode {
		mode = "openwrt"
	} else if !fnosMode && len(args) > 0 {
		// 若指定了其他参数（如 -a 等）但没有 -f/-w，仍视作 fnos
		// 因此保持 mode = "fnos"
	}

	// 设置默认值（根据模式）
	config.SetDefaults(mode)

	// 环境变量覆盖（所有配置均可通过环境变量修改）
	if v := os.Getenv("FLUXOR_DATA_DIR"); v != "" {
		// 统一运行数据目录：fluxor.json / fluxor.log 由此派生（PID 文件见 config.SetDataDir）
		config.SetDataDir(v)
	}
	if v := os.Getenv("SOCKET_PATH"); v != "" {
		config.SocketPath = v
	}
	if v := os.Getenv("BASE_URL"); v != "" {
		config.BaseURL = v
	}
	if v := os.Getenv("FLUXOR_ADDR"); v != "" {
		config.TcpAddr = v
	}
	if v := os.Getenv("FLUXOR_BIN_DIR"); v != "" {
		config.FluxorBinDir = v
	}
	if v := os.Getenv("CORE_BIN"); v != "" {
		config.CoreBin = v
	}
	if v := os.Getenv("CORE_SOCKET"); v != "" {
		config.CoreSocket = v
	}
	if v := os.Getenv("META_DIR"); v != "" {
		config.MetaDir = v
	}
	if v := os.Getenv("ZASH_DIR"); v != "" {
		config.ZashDir = v
	}
	if v := os.Getenv("CONFIG_TARGET"); v != "" {
		config.ConfigTarget = v
	}
	if v := os.Getenv("CORE_WORK_DIR"); v != "" {
		config.CoreWorkDir = v
	}

	// 命令行 -a 覆盖 TCP 地址（最高优先级）
	if customAddr != "" {
		config.TcpAddr = customAddr
	}

	// 更新 originalBaseURL（可能被环境变量修改）
	config.OriginalBaseURL = config.BaseURL

	// === 初始化日志 ===
	// 全部日志由后端独占写入运行数据目录下的 fluxor.log（并镜像到 stderr），
	// 不依赖启动脚本的 stdout/stderr 重定向——否则同一份日志会因启动方式不同而
	// 落在不同文件里。等级由 FLUXOR_LOG_LEVEL 控制（debug / info / warn / error）。
	logLevel := logx.ParseLevel(os.Getenv("FLUXOR_LOG_LEVEL"))
	logx.SetLevel(logLevel)
	if err := logx.Setup(config.FluxorLogFile); err != nil {
		// 文件打不开不阻断启动：logx 退回「只写 stderr」，这里如实报一条
		logx.Error(logx.ModuleMain, "failed to open log file %s, logging to stderr only: %v", config.FluxorLogFile, err)
	}
	defer logx.Close()
	logx.Info(logx.ModuleMain, "Fluxor starting: version=%s mode=%s log_file=%s log_level=%s",
		buildinfo.Name(), mode, config.FluxorLogFile, logx.LevelName(logLevel))

	// === 检查和准备 ===
	// 运行数据目录与 PID 目录先建好（openwrt 下 PID 目录固定在 /var/run，与数据
	// 目录不同址）：其下的 fluxor.json / fluxor.log / fluxor.pid / core.pid 都在
	// 启动早期被读取或写入，目录缺失时各处的报错会分散且难定位。
	for _, dir := range []string{config.FluxorDataDir, filepath.Dir(config.FluxorPidFile)} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			logx.Error(logx.ModuleMain, "failed to create runtime directory %s: %v", dir, err)
		}
	}
	// 载入全部配置文件（必要时先把旧的单文件 fluxor.json 拆分成多文件）
	config.LoadAll()
	subscription.StartAllTimers()
	tproxy.LoadTproxyState()
	// TUN 运行参数（tun.json）。注意这里**只载入用户偏好**，不会开启 TUN：
	// config.yaml 里 tun.enable 恒为 false，TUN 只能由用户手动开启且不做回放。
	tun.Load()
	// TUN 与 TProxy 互斥：TProxy 下发规则前必须确认 TUN 没在接管。
	//
	// 校验用内核的 /configs 实时查询，而不是任何本地标志：TUN 的开关不落盘且第三方
	// 面板也能打开它，缓存会在面板重启/内核重载/第三方操作后失真。tproxy 不能反向
	// import tun（依赖方向是 tun → tproxy），故由 main 以回调注入。
	tproxy.SetEnableGuard(func() error {
		on, err := tun.CoreTunEnabled()
		if err != nil {
			// 查询失败不阻断：内核没运行时 TUN 必然没开；运行中但读不到时，
			// 宁可保留现有的 TProxy 能力也不让它永久不可用——但必须留痕。
			logx.Warn(logx.ModuleTproxy, "cannot query TUN state before applying tproxy rules: %v", err)
			return nil
		}
		if on {
			return fmt.Errorf("TUN 正在接管流量，请先关闭 TUN 再启用 TPROXY（两者互斥）")
		}
		return nil
	})
	// 冷启动收敛：把开关状态归零并清除可能残留的 nft/策略路由规则，
	// 避免上次非优雅退出后出现「面板显示关闭、流量仍被劫持」的错配。
	tproxy.ResetOnStartup()
	// 清理上次非优雅退出遗留的临时内核进程与临时文件
	core.CleanupStaleTempCores()
	// 清理下载中途被杀留下的 *.downloading.yaml（不参与任何流程，只占空间）
	subscription.CleanupStaleDownloads()

	if _, err := os.Stat(config.ConfigTarget); os.IsNotExist(err) {
		// 按当前模式生成首份配置：
		//   - 自定义模式：模板 + 手工节点 + 标准规则集（否则首启会得到一份没有节点的
		//     基础配置，用户保存过的节点在重启后不生效）；
		//   - 其余模式沿用融合式生成（无订阅时其内部会退化为基础配置）。
		// 切换模式的首启仍走这条：此时订阅文件可能尚未下载，只能先生成骨架。
		// 加锁取快照：启动阶段虽然还没开始服务，但定时更新 goroutine（StartAllTimers）
		// 已经可能触发 assembleCurrent 写入 Current，无锁读同样是竞争。
		snap := config.CurrentSnapshot()
		generate := configgen.GenerateConfig
		if snap.Mode == config.ModeCustom {
			generate = configgen.GenerateCustomConfig
		}
		if err := generate(snap); err != nil {
			logx.Error(logx.ModuleGen, "failed to generate initial config.yaml: %v", err)
		} else {
			logx.Info(logx.ModuleGen, "initial config.yaml generated: %s", config.ConfigTarget)
		}
	}

	// 写入面板自身的 PID 文件（运行数据目录已在启动早期创建）
	pidData := []byte(fmt.Sprintf("%d", os.Getpid()))
	if err := os.WriteFile(config.FluxorPidFile, pidData, 0644); err != nil {
		logx.Error(logx.ModuleMain, "failed to write pid file %s: %v", config.FluxorPidFile, err)
	} else {
		logx.Debug(logx.ModuleMain, "pid file written: %s (pid=%d)", config.FluxorPidFile, os.Getpid())
		defer func() {
			if err := os.Remove(config.FluxorPidFile); err != nil {
				logx.Error(logx.ModuleMain, "failed to remove pid file %s: %v", config.FluxorPidFile, err)
			}
		}()
	}

	var err error
	web.IndexTmpl, err = template.ParseFS(staticFS, "index.html")
	if err != nil {
		logx.Error(logx.ModuleWeb, "failed to parse index.html template: %v", err)
		os.Exit(1)
	}

	// === 检查监听方式 ===
	if config.SocketPath == "" && config.TcpAddr == "" {
		logx.Error(logx.ModuleMain, "no listen address configured: both SOCKET_PATH and FLUXOR_ADDR are empty")
		os.Exit(1)
	}

	// === 创建 Unix socket 监听器（若启用）===
	var listener net.Listener
	if config.SocketPath != "" {
		if err := os.MkdirAll(filepath.Dir(config.SocketPath), 0755); err != nil {
			logx.Error(logx.ModuleMain, "failed to create socket directory for %s: %v", config.SocketPath, err)
			os.Exit(1)
		}
		os.Remove(config.SocketPath)

		listener, err = net.Listen("unix", config.SocketPath)
		if err != nil {
			logx.Error(logx.ModuleMain, "failed to listen on unix socket %s: %v", config.SocketPath, err)
			os.Exit(1)
		}
		defer listener.Close()

		// 0660：仅属主与所属组可读写，收窄此前 0666（任意本地用户均可
		// 通过该 socket 全权操作内核）。
		if err := os.Chmod(config.SocketPath, 0660); err != nil {
			logx.Warn(logx.ModuleMain, "failed to chmod unix socket %s to 0660: %v", config.SocketPath, err)
		}
		logx.Info(logx.ModuleMain, "listening on unix socket: %s", config.SocketPath)
	} else {
		logx.Info(logx.ModuleMain, "unix socket disabled")
	}

	// === 创建 TCP 监听器（若启用）===
	var tcpListener net.Listener
	if config.TcpAddr != "" {
		if err := netinfo.ValidateTCPAddr(config.TcpAddr); err != nil {
			logx.Warn(logx.ModuleMain, "invalid FLUXOR_ADDR %q (%v), tcp listener disabled", config.TcpAddr, err)
			config.TcpAddr = ""
		}
		if config.TcpAddr != "" {
			tcpListener, err = net.Listen("tcp", config.TcpAddr)
			if err != nil {
				logx.Error(logx.ModuleMain, "failed to listen on tcp address %s: %v", config.TcpAddr, err)
			} else {
				defer tcpListener.Close()
				logx.Info(logx.ModuleMain, "listening on tcp address: %s", config.TcpAddr)
			}
		}
	}

	if config.BaseURL == "/" {
		config.BaseURL = ""
	} else {
		config.BaseURL = strings.TrimSuffix(config.BaseURL, "/")
	}

	// === 创建路由 ===
	mux := http.NewServeMux()

	// 外部静态面板
	mux.Handle(config.BaseURL+"/meta/", http.StripPrefix(config.BaseURL+"/meta/", http.FileServer(http.Dir(config.MetaDir))))
	mux.Handle(config.BaseURL+"/zash/", http.StripPrefix(config.BaseURL+"/zash/", http.FileServer(http.Dir(config.ZashDir))))

	// 内嵌静态文件（Vue 构建产物 assets/ 目录，直接挂载在 baseURL 下）
	// assets/ 下的文件名都带内容哈希（index-<hash>.js），内容一改名字即变，故给一年期强缓存：
	// 重复访问不再重下（此前无 Cache-Control，且内嵌文件无 ModTime/ETag，连 304 都无法协商）；
	// 而每次部署后 index.html 引用的都是新哈希名，不会取到旧副本。
	staticFileServer := http.FileServer(http.FS(staticFS))
	mux.Handle(config.BaseURL+"/assets/", http.StripPrefix(config.BaseURL, httpx.CacheImmutable(staticFileServer)))
	// 内嵌静态根文件（index.html 之外的静态资源，如 favicon ICON.PNG）。
	// 名字固定、内容可能随部署变化，因此必须每次回源校验；但内嵌文件系统没有
	// ModTime/ETag，单靠 no-cache 会让浏览器每次完整重下（ICON.PNG 有 80 KB）。
	// ETagFile 在启动时算出内容摘要，命中 If-None-Match 时回 304（无响应体）。
	if iconHandler, err := httpx.ETagFile(staticFS, "ICON.PNG", "image/png"); err != nil {
		logx.Warn(logx.ModuleMain, "failed to prepare ETag handler for ICON.PNG, falling back to plain file server: %v", err)
		mux.Handle(config.BaseURL+"/ICON.PNG", http.StripPrefix(config.BaseURL, httpx.CacheRevalidate(staticFileServer)))
	} else {
		mux.Handle(config.BaseURL+"/ICON.PNG", iconHandler)
	}

	// 页面路由
	if config.BaseURL == "" {
		// 根路径直接渲染首页
		mux.HandleFunc("/", web.HandleIndex)
	} else {
		// 根路径重定向到实际前缀
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				redirectTo := config.BaseURL
				if !strings.HasSuffix(redirectTo, "/") {
					redirectTo += "/"
				}
				http.Redirect(w, r, redirectTo, http.StatusFound)
				return
			}
			http.NotFound(w, r)
		})
		// 实际页面路由
		mux.HandleFunc(config.BaseURL+"/", web.HandleIndex)
	}
	mux.HandleFunc(config.BaseURL+"/whoami", web.HandleWhoAmI)
	mux.HandleFunc(config.BaseURL+"/app-version", web.HandleAppVersion)

	// 内核控制
	mux.HandleFunc(config.BaseURL+"/core/status", core.HandleCoreStatus)
	// 内核状态变更的 SSE 推送（前端据此替代轮询）
	mux.HandleFunc(config.BaseURL+"/core/events", core.HandleCoreEvents)
	mux.HandleFunc(config.BaseURL+"/core/start", core.HandleCoreStart)
	mux.HandleFunc(config.BaseURL+"/core/stop", core.HandleCoreStop)
	mux.HandleFunc(config.BaseURL+"/core/restart", core.HandleCoreRestart)
	mux.HandleFunc(config.BaseURL+"/upgrade", dashapi.HandleUpgrade)
	mux.HandleFunc(config.BaseURL+"/core/check-update", appupdate.HandleCoreCheckUpdate)

	// 订阅中心 API
	mux.HandleFunc(config.BaseURL+"/subscribe/config", subscription.HandleSubscribeConfigAPI)
	mux.HandleFunc(config.BaseURL+"/subscribe/generate", subscription.HandleGenerateConfig)
	mux.HandleFunc(config.BaseURL+"/subscribe/update/", subscription.HandleSubscribeUpdate)
	// 自定义模式：可添加的协议与字段表（前端按此渲染动态表单，默认模板由后端单点维护）
	mux.HandleFunc(config.BaseURL+"/subscribe/node-protocols", subscription.HandleNodeProtocolsAPI)
	// 切换模式：订阅级自定义规则（查询 / 新增 / 修改 / 排序 / 删除，即时持久化并同步运行配置）
	mux.HandleFunc(config.BaseURL+"/subscribe/custom-rules/", subscription.HandleCustomRulesAPI)
	// 融合模式：按规则集档位（base / full）分开存放的自定义规则，接口语义与切换模式一致
	mux.HandleFunc(config.BaseURL+"/subscribe/merge-custom-rules/", subscription.HandleMergeCustomRulesAPI)
	// 自定义模式：独立一份规则（与融合模式互不影响），可选目标含手工节点名
	mux.HandleFunc(config.BaseURL+"/subscribe/custom-mode-rules/", subscription.HandleCustomModeRulesAPI)
	// 流量隧道（config.yaml 的 tunnels 顶层块）三个作用域入口，与上面三条自定义规则入口
	// 一一对应：切换模式按订阅、融合模式按规则集档位、自定义模式独立一份。
	// 均为查询 / 新增 / 修改（含启停开关）/ 排序 / 删除，即时持久化并在生效作用域同步运行配置。
	mux.HandleFunc(config.BaseURL+"/subscribe/custom-tunnels/", subscription.HandleSubscriptionTunnelsAPI)
	mux.HandleFunc(config.BaseURL+"/subscribe/merge-custom-tunnels/", subscription.HandleMergeTunnelsAPI)
	mux.HandleFunc(config.BaseURL+"/subscribe/custom-mode-tunnels/", subscription.HandleCustomModeTunnelsAPI)

	// 获取所有订阅的代理信息（融合模式使用）
	mux.HandleFunc(config.BaseURL+"/providers/proxies", dashapi.HandleProvidersProxiesAll)
	mux.HandleFunc(config.BaseURL+"/providers/proxies/", dashapi.HandleProviderProxies)

	// 策略组测速（组内所有节点/子策略组）
	mux.HandleFunc(config.BaseURL+"/group/", dashapi.HandleGroupDelay)

	// WebSocket 代理
	mux.HandleFunc(config.BaseURL+"/traffic", wsproxy.WsProxyHandler("/traffic"))
	mux.HandleFunc(config.BaseURL+"/memory", wsproxy.WsProxyHandler("/memory"))

	// HTTP 代理
	mux.HandleFunc(config.BaseURL+"/version", dashapi.HandleVersion)
	mux.HandleFunc(config.BaseURL+"/configs", dashapi.HandleConfigsAPI)
	mux.HandleFunc(config.BaseURL+"/interfaces", netinfo.HandleInterfaces)
	mux.HandleFunc(config.BaseURL+"/configs/geo", dashapi.HandleConfigsGeo)
	mux.HandleFunc(config.BaseURL+"/providers/geo", dashapi.HandleProvidersGeo)
	mux.HandleFunc(config.BaseURL+"/cache/fakeip/flush", dashapi.HandleFlushFakeIP)
	mux.HandleFunc(config.BaseURL+"/cache/dns/flush", dashapi.HandleFlushDNS)
	mux.HandleFunc(config.BaseURL+"/dns/query", dashapi.HandleDNSQuery)
	mux.HandleFunc(config.BaseURL+"/restart", dashapi.HandleRestart)
	mux.HandleFunc(config.BaseURL+"/config/tproxy", tproxy.HandleTproxyState)
	mux.HandleFunc(config.BaseURL+"/config/tproxy/exceptions", tproxy.HandleTproxyExceptions)
	mux.HandleFunc(config.BaseURL+"/config/tproxy/proxy-local", tproxy.HandleTproxyProxyLocal)
	mux.HandleFunc(config.BaseURL+"/config/tproxy/proxy-ipv6", tproxy.HandleTproxyProxyIPv6)
	// TUN 运行参数与启停：参数只落 tun.json（稀疏），开启时才组装完整对象 PATCH 内核。
	mux.HandleFunc(config.BaseURL+"/config/tun", tun.HandleTunSettings)
	mux.HandleFunc(config.BaseURL+"/config/tun/state", tun.HandleTunState)

	mux.HandleFunc(config.BaseURL+"/ipinfo/local/v4", netinfo.HandleLocalIPv4)
	mux.HandleFunc(config.BaseURL+"/ipinfo/local/v6", netinfo.HandleLocalIPv6)
	mux.HandleFunc(config.BaseURL+"/ipinfo/proxy/v4", netinfo.HandleProxyIPv4)
	mux.HandleFunc(config.BaseURL+"/ipinfo/proxy/v6", netinfo.HandleProxyIPv6)

	mux.HandleFunc(config.BaseURL+"/delaytest/google", delaytest.HandleDelayTestGoogle)
	mux.HandleFunc(config.BaseURL+"/delaytest/youtube", delaytest.HandleDelayTestYouTube)
	mux.HandleFunc(config.BaseURL+"/delaytest/github", delaytest.HandleDelayTestGitHub)
	mux.HandleFunc(config.BaseURL+"/delaytest/baidu", delaytest.HandleDelayTestBaidu)
	mux.HandleFunc(config.BaseURL+"/delaytest/bilibili", delaytest.HandleDelayTestBilibili)
	mux.HandleFunc(config.BaseURL+"/delaytest/custom", delaytest.HandleDelayTestCustom)

	// 代理 API
	mux.HandleFunc(config.BaseURL+"/proxies", dashapi.HandleProxies)
	mux.HandleFunc(config.BaseURL+"/proxies/", func(w http.ResponseWriter, r *http.Request) {
		// 只按路径后缀分派：r.URL.Path **不含查询串**（参数在 r.URL.RawQuery 里），
		// 因此曾经的 `strings.Contains(r.URL.Path, "/delay?")` 恒为假——看起来像在
		// 处理 "/proxies/x/delay?timeout=..." 这种形态，实际由上面的后缀判断覆盖。
		//
		// 已知限制（未修）：名叫 delay 的代理同样以 "/delay" 结尾，会被误判成测速请求，
		// 于是 PUT /proxies/delay 回 405、该代理无法被选中。要修需改按路径段分派。
		if strings.HasSuffix(r.URL.Path, "/delay") {
			dashapi.HandleProxyDelay(w, r)
		} else {
			dashapi.HandleProxySwitch(w, r)
		}
	})

	// fluxor 版本更新
	mux.HandleFunc(config.BaseURL+"/check-update", appupdate.HandleCheckUpdate)
	mux.HandleFunc(config.BaseURL+"/update-self", appupdate.HandleSelfUpdate)

	// 质量分数
	mux.HandleFunc(config.BaseURL+"/proxies/quality", quality.HandleQualityScores)

	// 日志 WebSocket
	mux.HandleFunc(config.BaseURL+"/logs", wsproxy.WsProxyHandler("/logs"))

	// 规则 API
	mux.HandleFunc(config.BaseURL+"/rules", dashapi.HandleRules)
	mux.HandleFunc(config.BaseURL+"/rules/disable", dashapi.HandleRulesDisable)
	mux.HandleFunc(config.BaseURL+"/providers/rules", dashapi.HandleRuleProviders)
	mux.HandleFunc(config.BaseURL+"/providers/rules/", dashapi.HandleUpdateRuleProvider)

	// 连接管理
	mux.HandleFunc(config.BaseURL+"/connections", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			dashapi.HandleConnectionsClose(w, r)
		} else {
			wsproxy.WsProxyHandler("/connections")(w, r)
		}
	})
	mux.HandleFunc(config.BaseURL+"/connections/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			dashapi.HandleConnectionsClose(w, r)
		} else {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})

	// 自动启动内核
	if !core.IsCoreRunning() {
		if err := core.StartCore(); err != nil {
			logx.Error(logx.ModuleCore, "auto start of mihomo failed: %v", err)
		}
	} else {
		logx.Info(logx.ModuleCore, "mihomo already running, auto start skipped")
	}

	// 无条件发布一次初态，确保 SSE hub 的状态「已确定」。
	//
	// 这一步是必需的：hub 的「接入即快照」依赖它已知道当前状态，否则
	// 新订阅者接入后收不到任何事件——而 /core/events 是前端启动时获取
	// 内核状态的唯一来源（启动阶段不再请求 /core/status）。
	// 内核确实在跑时（StartCore 内部已发布一次）此调用因状态未变化而成为空操作。
	core.PublishCoreState(core.IsCoreRunning())

	// === 启动服务 ===
	//
	// 用显式构造的 http.Server 取代裸 http.Serve：裸 Serve 没有读超时，一个只连不发的
	// 客户端就能长期占住连接与 goroutine（面板无认证，默认可达范围取决于监听地址，
	// 不能假设调用方都是善意的）。handler 外面再套一层请求体上限，见 newServerHandler。
	serve := func(l net.Listener, label string) {
		srv := newHTTPServer(newServerHandler(mux))
		go func() {
			err := srv.Serve(l)
			if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
				logx.Error(logx.ModuleMain, "%s http server stopped unexpectedly: %v", label, err)
			}
		}()
	}
	if listener != nil {
		serve(listener, "unix socket")
	}
	if tcpListener != nil {
		logx.Info(logx.ModuleMain, "http server started on tcp address: %s", config.TcpAddr)
		serve(tcpListener, "tcp")
	}

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logx.Info(logx.ModuleMain, "shutdown signal received, stopping Fluxor")
	subscription.StopAllTimers()
	tproxy.DisableTProxyRules()
	if core.IsCoreRunning() {
		if err := core.StopCore(); err != nil {
			logx.Error(logx.ModuleCore, "failed to stop mihomo: %v", err)
		}
	} else {
		logx.Debug(logx.ModuleCore, "mihomo is not running, nothing to stop")
	}
	logx.Info(logx.ModuleMain, "Fluxor stopped")
}
