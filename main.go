package main

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/gorilla/websocket"
)

var staticFS fs.FS

var (
    socketPath          string
    baseURL             string
    corePidFile         string
    coreBin             string
    coreSocket          string
    metaDir             string
    zashDir             string
    subscribeConfigFile string
    configTarget        string
    configTemplateDir   string
    infoLogFile         string
    coreWorkDir         string
)

func init() {
    socketPath = getEnv("SOCKET_PATH", "/var/apps/Fluxor/target/app.sock")
    baseURL = getEnv("BASE_URL", "/app/Fluxor")
    corePidFile = getEnv("CORE_PID_FILE", "/var/apps/Fluxor/var/core.pid")
    coreBin = getEnv("CORE_BIN", "/var/apps/Fluxor/target/bin/mihomo")
    coreSocket = getEnv("CORE_SOCKET", "/var/apps/Fluxor/target/core.sock")
    metaDir = getEnv("META_DIR", "/var/apps/Fluxor/shares/ui/meta")
    zashDir = getEnv("ZASH_DIR", "/var/apps/Fluxor/shares/ui/zash")
    subscribeConfigFile = getEnv("SUBSCRIBE_CONFIG_FILE", "/var/apps/Fluxor/var/subscribe.json")
    configTarget = getEnv("CONFIG_TARGET", "/var/apps/Fluxor/shares/Fluxor/config.yaml")
    configTemplateDir = getEnv("CONFIG_TEMPLATE_DIR", "/var/apps/Fluxor/target/templates")
    infoLogFile = getEnv("INFO_LOG_FILE", "/var/apps/Fluxor/shares/Fluxor/info.log")
    coreWorkDir = getEnv("CORE_WORK_DIR", "/var/apps/Fluxor/shares/Fluxor")
}

func getEnv(key, defaultVal string) string {
    if val := os.Getenv(key); val != "" {
        return val
    }
    return defaultVal
}

const (
	metaConfigFile = "config.js"
)

var (
	indexTmpl *template.Template
	upgrader  = websocket.Upgrader{
		CheckOrigin:     func(r *http.Request) bool { return true },
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

func main() {
	loadSubscribeConfig()
	initCoreLogger()
	startAllTimers()

	if _, err := os.Stat(configTarget); os.IsNotExist(err) {
		if err := generateConfig(subscribeConfig); err != nil {
			fmt.Printf("生成基本配置文件失败: %v\n", err)
		} else {
			fmt.Println("已生成基本配置文件 (config.yaml)")
		}
	}

	var err error
	indexTmpl, err = template.ParseFS(staticFS, "static/html/index.html")
	if err != nil {
		fmt.Printf("加载主页模板失败: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		fmt.Printf("无法创建 socket 目录: %v\n", err)
		os.Exit(1)
	}
	os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		fmt.Printf("监听 Unix socket 失败: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	if err := os.Chmod(socketPath, 0666); err != nil {
		fmt.Printf("设置 socket 权限失败: %v\n", err)
	}

	mux := http.NewServeMux()

	// 外部静态面板
	mux.Handle(baseURL+"/meta/", http.StripPrefix(baseURL+"/meta/", http.FileServer(http.Dir(metaDir))))
	mux.Handle(baseURL+"/zash/", http.StripPrefix(baseURL+"/zash/", http.FileServer(http.Dir(zashDir))))

	// 内嵌静态文件
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle(baseURL+"/static/", http.StripPrefix(baseURL+"/static/", http.FileServer(http.FS(staticSub))))

	// 页面路由
	mux.HandleFunc(baseURL+"/", handleIndex)

	// 内核控制
	mux.HandleFunc(baseURL+"/core/status", handleCoreStatus)
	mux.HandleFunc(baseURL+"/core/start", handleCoreStart)
	mux.HandleFunc(baseURL+"/core/stop", handleCoreStop)
	mux.HandleFunc(baseURL+"/core/restart", handleCoreRestart)
	// 内核升级
    mux.HandleFunc(baseURL+"/upgrade", handleUpgrade)

	// 订阅中心 API
	mux.HandleFunc(baseURL+"/subscribe/config", handleSubscribeConfigAPI)
	mux.HandleFunc(baseURL+"/subscribe/generate", handleGenerateConfig)
	mux.HandleFunc(baseURL+"/subscribe/update/", handleSubscribeUpdate)
	mux.HandleFunc(baseURL+"/subscribe/update-info/", handleUpdateSubscriptionInfo)

	
	// 订阅信息（含流量、有效期）
    mux.HandleFunc(baseURL+"/providers/proxies/", handleProviderProxies)

	// ===== WebSocket 代理（实时数据） =====
	mux.HandleFunc(baseURL+"/traffic", wsProxyHandler("/traffic"))
	mux.HandleFunc(baseURL+"/memory", wsProxyHandler("/memory"))

	// ===== HTTP 代理（版本、配置、DNS、其他） =====
	mux.HandleFunc(baseURL+"/version", handleVersion)
	mux.HandleFunc(baseURL+"/configs", handleConfigsAPI)
	mux.HandleFunc(baseURL+"/interfaces", handleInterfaces)
	mux.HandleFunc(baseURL+"/configs/geo", handleConfigsGeo)
	mux.HandleFunc(baseURL+"/providers/geo", handleProvidersGeo)
	mux.HandleFunc(baseURL+"/cache/fakeip/flush", handleFlushFakeIP)
	mux.HandleFunc(baseURL+"/cache/dns/flush", handleFlushDNS)
	mux.HandleFunc(baseURL+"/dns/query", handleDNSQuery)
	mux.HandleFunc(baseURL+"/restart", handleRestart)

	mux.HandleFunc(baseURL+"/ipinfo/local/v4", handleLocalIPv4)
    mux.HandleFunc(baseURL+"/ipinfo/local/v6", handleLocalIPv6)
    mux.HandleFunc(baseURL+"/ipinfo/proxy/v4", handleProxyIPv4)
    mux.HandleFunc(baseURL+"/ipinfo/proxy/v6", handleProxyIPv6)

    mux.HandleFunc(baseURL+"/delaytest/google", handleDelayTestGoogle)
    mux.HandleFunc(baseURL+"/delaytest/youtube", handleDelayTestYouTube)
	mux.HandleFunc(baseURL+"/delaytest/github", handleDelayTestGitHub)
    mux.HandleFunc(baseURL+"/delaytest/custom", handleDelayTestCustom)

	// 代理 API
	mux.HandleFunc(baseURL+"/proxies", handleProxies)
	mux.HandleFunc(baseURL+"/proxies/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/delay") || strings.Contains(r.URL.Path, "/delay?") {
			handleProxyDelay(w, r)
		} else {
			handleProxySwitch(w, r)
		}
	})

	// 日志 WebSocket
	mux.HandleFunc(baseURL+"/logs", wsProxyHandler("/logs"))

	// 规则 API
	mux.HandleFunc(baseURL+"/rules", handleRules)
	mux.HandleFunc(baseURL+"/rules/disable", handleRulesDisable)
	mux.HandleFunc(baseURL+"/providers/rules", handleRuleProviders)
	mux.HandleFunc(baseURL+"/providers/rules/", handleUpdateRuleProvider)

	// 连接管理
	mux.HandleFunc(baseURL+"/connections", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			handleConnectionsClose(w, r)
		} else {
			wsProxyHandler("/connections")(w, r)
		}
	})
	mux.HandleFunc(baseURL+"/connections/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			handleConnectionsClose(w, r)
		} else {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})

	// 自动启动内核
	if !isCoreRunning() {
		fmt.Println("内核未运行，尝试自动启动...")
		if err := startCore(); err != nil {
			fmt.Printf("自动启动内核失败: %v\n", err)
		} else {
			fmt.Println("内核已自动启动")
		}
	} else {
		fmt.Println("内核已在运行，跳过自动启动")
	}

	// 启动 HTTP 服务
	go func() {
		fmt.Printf("Fluxor 已启动，监听 Unix socket: %s\n", socketPath)
		err := http.Serve(listener, mux)
		if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
			fmt.Printf("HTTP 服务错误: %v\n", err)
		}
	}()

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\n收到退出信号，正在关闭 Fluxor...")
	stopAllTimers()
	if isCoreRunning() {
		fmt.Println("正在停止内核...")
		if err := stopCore(); err != nil {
			fmt.Printf("停止内核失败: %v\n", err)
		} else {
			fmt.Println("内核已停止")
		}
	} else {
		fmt.Println("内核未运行，无需停止")
	}
	fmt.Println("Fluxor 已安全退出")
}

// wsProxyHandler 返回处理 WebSocket 代理的 HandlerFunc
func wsProxyHandler(targetPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("[WS] 升级失败 (路径 %s): %v", targetPath, err)
			return
		}
		defer conn.Close()

		subscribeMu.RLock()
		secret := subscribeConfig.PanelSecret
		subscribeMu.RUnlock()

		dialer := &websocket.Dialer{
			NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", coreSocket)
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		}
		header := http.Header{}
		if secret != "" {
			header.Set("Authorization", "Bearer "+secret)
		}
		path := targetPath
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		coreConn, _, err := dialer.Dial("ws://localhost"+path, header)
		if err != nil {
			// 内核未运行或连接失败是预期情况，不记录日志
			return
		}
		defer coreConn.Close()

		errChan := make(chan error, 2)

		go func() {
			for {
				msgType, msg, err := coreConn.ReadMessage()
				if err != nil {
					errChan <- err
					return
				}
				if err := conn.WriteMessage(msgType, msg); err != nil {
					errChan <- err
					return
				}
			}
		}()

		go func() {
			for {
				msgType, msg, err := conn.ReadMessage()
				if err != nil {
					errChan <- err
					return
				}
				if err := coreConn.WriteMessage(msgType, msg); err != nil {
					errChan <- err
					return
				}
			}
		}()

		<-errChan
	}
}