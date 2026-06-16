package main

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

//go:embed static/*
var staticFS embed.FS

const (
	socketPath   = "/var/apps/Fluxor/target/app.sock"
	baseURL      = "/app/Fluxor"
	settingsFile = "/var/apps/Fluxor/var/settings.json"

	corePidFile = "/var/apps/Fluxor/var/core.pid"
	coreBin     = "/var/apps/Fluxor/target/bin/mihomo"
	coreSocket  = "/var/apps/Fluxor/target/core.sock"

	metaDir = "/var/apps/Fluxor/target/meta"
	zashDir = "/var/apps/Fluxor/target/zash"

	// 订阅模块常量也保留在此
	subscribeConfigFile = "/var/apps/Fluxor/var/subscribe.json"
	configTarget        = "/var/apps/Fluxor/var/config.yaml"
	configTemplateDir   = "/var/apps/Fluxor/target/templates"

	// 日志与工作目录
	infoLogFile = "/var/apps/Fluxor/var/info.log"
	coreWorkDir = "/var/apps/Fluxor/var"
	metaConfigFile = "config.js" 
)

var (
	indexTmpl *template.Template
)

func main() {
	// 加载持久化配置
	loadSettings()
	loadSubscribeConfig()
	initCoreLogger()

	// 启动时自动生成基本配置文件（无订阅时）
	if _, err := os.Stat(configTarget); os.IsNotExist(err) {
		subscribeMu.RLock()
		subsEmpty := len(subscribeConfig.Subscriptions) == 0
		subscribeMu.RUnlock()
		if subsEmpty {
			basic := `mixed-port: 7790
allow-lan: true
mode: rule
log-level: silent
external-controller-unix: '/var/apps/Fluxor/target/core.sock'
external-controller: '0.0.0.0:9090'
`
			os.MkdirAll(filepath.Dir(configTarget), 0755)
			if err := os.WriteFile(configTarget, []byte(basic), 0644); err != nil {
				fmt.Printf("生成基本配置文件失败: %v\n", err)
			} else {
				fmt.Println("已生成基本配置文件 (config.yaml) 因为订阅列表为空")
			}
		}
	}

	// 加载内嵌模板
	var err error
	indexTmpl, err = template.ParseFS(staticFS, "static/html/index.html")
	if err != nil {
		fmt.Printf("加载主页模板失败: %v\n", err)
		os.Exit(1)
	}

	// 准备 Unix socket
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

	// 外部静态面板（meta / zash）
	mux.Handle(baseURL+"/meta/", http.StripPrefix(baseURL+"/meta/", http.FileServer(http.Dir(metaDir))))
	mux.Handle(baseURL+"/zash/", http.StripPrefix(baseURL+"/zash/", http.FileServer(http.Dir(zashDir))))

	// 内嵌静态文件服务
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle(baseURL+"/static/", http.StripPrefix(baseURL+"/static/", http.FileServer(http.FS(staticSub))))

	// 页面路由
	mux.HandleFunc(baseURL+"/", handleIndex)

	// 内核控制
	mux.HandleFunc(baseURL+"/core/status", handleCoreStatus)
	mux.HandleFunc(baseURL+"/core/start", handleCoreStart)
	mux.HandleFunc(baseURL+"/core/stop", handleCoreStop)
	mux.HandleFunc(baseURL+"/core/restart", handleCoreRestart)

	// 面板更新 API
	mux.HandleFunc(baseURL+"/update/meta", handleUpdateMeta)
	mux.HandleFunc(baseURL+"/update/zash", handleUpdateZash)

	// 代理设置 API
	mux.HandleFunc(baseURL+"/settings", handleSettings)

	// 订阅中心 API
	mux.HandleFunc(baseURL+"/subscribe/config", handleSubscribeConfigAPI)
	mux.HandleFunc(baseURL+"/subscribe/generate", handleGenerateConfig)

	// 仪表盘数据代理 API
	mux.HandleFunc(baseURL+"/version", handleVersion)
	mux.HandleFunc(baseURL+"/traffic", handleTraffic)
	mux.HandleFunc(baseURL+"/memory", handleMemory)
	mux.HandleFunc(baseURL+"/connections", handleConnections)
	mux.HandleFunc(baseURL+"/configs", handleConfigsAPI)
	mux.HandleFunc(baseURL+"/configs/geo", handleConfigsGeo)
	mux.HandleFunc(baseURL+"/providers/geo", handleProvidersGeo)
	mux.HandleFunc(baseURL+"/cache/fakeip/flush", handleFlushFakeIP)
	mux.HandleFunc(baseURL+"/cache/dns/flush", handleFlushDNS)
	mux.HandleFunc(baseURL+"/dns/query", handleDNSQuery)
	mux.HandleFunc(baseURL+"/restart", handleRestart)

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
		if err := http.Serve(listener, mux); err != nil && err != http.ErrServerClosed {
			fmt.Printf("HTTP 服务错误: %v\n", err)
		}
	}()

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\n收到退出信号，正在关闭 Fluxor...")
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