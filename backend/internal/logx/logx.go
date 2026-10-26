package logx

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// Level 日志等级；数值越大越严重，低于当前等级的记录被丢弃。
type Level int32

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// 大功能模块标记。粒度与后端功能域一致（不按文件细分），新增模块在此登记，
// 调用方一律使用这些常量，避免各处手写字符串导致 grep 失效。
const (
	ModuleMain    = "MAIN"    // 进程启动/退出/监听/信号
	ModuleConfig  = "CONFIG"  // 配置持久化（settings/rules/tunnels 等）
	ModuleCore    = "CORE"    // 内核进程生命周期
	ModuleSub     = "SUB"     // 订阅下载、更新、元数据、定时器
	ModuleGen     = "GEN"     // config.yaml 生成
	ModuleRule    = "RULE"    // 自定义规则注入
	ModuleTunnel  = "TUNNEL"  // 流量隧道注入
	ModuleAPI     = "API"     // 内核 HTTP API 反向代理
	ModuleWS      = "WS"      // WebSocket 桥接
	ModuleTproxy  = "TPROXY"  // 防火墙与策略路由
	ModuleTun     = "TUN"     // TUN 运行参数与启停
	ModuleNet     = "NET"     // 网络信息查询
	ModuleDelay   = "DELAY"   // 连通延迟测试
	ModuleQuality = "QUALITY" // 节点质量评分
	ModuleUpdate  = "UPDATE"  // 版本检查与自更新
	ModuleWeb     = "WEB"     // 前端入口渲染
)

// timestampLayout 时间戳格式：本地时间，毫秒精度（够定位并发顺序，又不冗长）。
const timestampLayout = "2006-01-02 15:04:05.000"

var (
	// level 当前等级，默认 INFO；由 SetLevel / ParseLevel 调整。
	level atomic.Int32
	// sg 串行化写入：log.Logger 自带互斥，且每条记录一次 Write，保证行不撕裂。
	sg = log.New(os.Stderr, "", 0)
	// file 已打开的日志文件；为 nil 表示只写 stderr。
	file *os.File
)

// Setup 打开统一日志文件并与 stderr 双写（追加写，权限 0644）。
//
// 目录不存在时自动创建。失败时保持「只写 stderr」并返回错误——日志系统自身
// 不可用时不应阻止面板启动，调用方负责把这条错误报到 stderr。
func Setup(path string) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	file = f
	sg.SetOutput(io.MultiWriter(os.Stderr, f))
	return nil
}

// Close 同步并关闭日志文件，输出退回 stderr（幂等，可重复调用）。
func Close() {
	if file == nil {
		return
	}
	sg.SetOutput(os.Stderr)
	_ = file.Sync()
	_ = file.Close()
	file = nil
}

// Enabled 报告是否已启用文件双写（仅供自诊断输出）。
func Enabled() bool {
	return file != nil
}

// ParseLevel 解析日志等级取值（debug / info / warn / error，大小写不敏感，
// warning 视作 warn）。空值与无法识别的取值一律回落到 INFO。
func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// SetLevel 设置当前等级。
func SetLevel(l Level) {
	level.Store(int32(l))
}

// LevelName 返回等级的大写名称（用于日志行与自诊断）。
func LevelName(l Level) string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

// Debug 输出调试记录：内部步骤与正常路径的细节。
func Debug(module, format string, args ...any) {
	logf(LevelDebug, module, format, args...)
}

// Info 输出常规记录：有意义的状态变化。
func Info(module, format string, args ...any) {
	logf(LevelInfo, module, format, args...)
}

// Warn 输出告警记录：可继续运行，但出现了偏离预期的状况。
func Warn(module, format string, args ...any) {
	logf(LevelWarn, module, format, args...)
}

// Error 输出错误记录：某次操作确实失败了。
func Error(module, format string, args ...any) {
	logf(LevelError, module, format, args...)
}

// logf 组装并写出一条记录。
//
// 仅在确有参数时才走 fmt.Sprintf：消息里带字面量 % 的调用（如 "100%"）在没有
// 参数时不该被格式化，否则会被拼成 %!(NOVERB)。
func logf(l Level, module, format string, args ...any) {
	if l < Level(level.Load()) {
		return
	}
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	// 去掉调用方可能带上的换行，保证「一条记录 = 一行」
	msg = strings.TrimRight(strings.TrimRight(msg, "\n"), "\r")
	sg.Printf("%s %-5s [%s] %s", time.Now().Format(timestampLayout), LevelName(l), module, msg)
}
