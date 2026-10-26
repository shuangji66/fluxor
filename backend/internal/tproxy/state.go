package tproxy

import (
	"fluxor/internal/config"
	"slices"
	"sync"
)

var (
	tproxyEnableState        bool
	tproxyMu                 sync.RWMutex
	exceptionsMu             sync.RWMutex
	tproxyProxyLocal         bool
	tproxyIPv6               bool
	tproxyDstExceptionsCache []string
	tproxySrcExceptionsCache []string
)

// proxyLocalEnabled 读取「同时代理本机出站流量」开关（并发安全）。
//
// 规则装配（rules.go）需要读该开关，必须经本函数，不得直接访问 tproxyProxyLocal。
func proxyLocalEnabled() bool {
	exceptionsMu.RLock()
	defer exceptionsMu.RUnlock()
	return tproxyProxyLocal
}

// globalIPv6Enabled 读取订阅中心的「IPv6 支持」总开关（生成期设置）。
//
// 语义分工：`ipv6Enabled()` 是用户「想不想接管」（偏好，持久化在 tproxy.json），
// 本函数是「能不能接管」（订阅中心的生成期开关）。两者都为真才下发 IPv6 家族规则；
// 总开关关闭时**只跳过、不改偏好**——总开关恢复后下一次重建规则自动带上 v6。
func globalIPv6Enabled() bool {
	config.Mu.RLock()
	defer config.Mu.RUnlock()
	return config.Current.IPv6
}

// ipv6Enabled 读取「接管 IPv6 流量」开关（并发安全）。
//
// 默认关闭：节点普遍没有 IPv6 出口，无条件接管会把原本可直连的 IPv6 目标
// 变成必走代理而失败，故由用户显式开启。读取一律走本函数。
func ipv6Enabled() bool {
	exceptionsMu.RLock()
	defer exceptionsMu.RUnlock()
	return tproxyIPv6
}

// dstExceptions / srcExceptions 读取绕过列表（并发安全，返回副本）。
//
// 规则装配（rules.go）必须走这两个函数：列表的真相是内存缓存（由 LoadTproxyState
// 载入、SaveTproxy* 更新），直接读文件会退化成「每次下发规则都解析一遍配置」。
func dstExceptions() []string {
	exceptionsMu.RLock()
	defer exceptionsMu.RUnlock()
	return slices.Clone(tproxyDstExceptionsCache)
}

func srcExceptions() []string {
	exceptionsMu.RLock()
	defer exceptionsMu.RUnlock()
	return slices.Clone(tproxySrcExceptionsCache)
}

// GetTproxyState 读取 TProxy 开关状态（并发安全）。
//
// 所有读取方都必须经此函数，不要直接访问 tproxyEnableState：
// 该变量由 SetTproxyEnabled / HandleTproxyState 在 tproxyMu 保护下写入，
// 无锁读取会构成数据竞争。
func GetTproxyState() bool {
	tproxyMu.RLock()
	defer tproxyMu.RUnlock()
	return tproxyEnableState
}
