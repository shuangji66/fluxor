package tun

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"fluxor/internal/config"
	"fluxor/internal/configgen"
	"fluxor/internal/core"
	"fluxor/internal/httpx"
	"fluxor/internal/logx"
	"fluxor/internal/tproxy"
)

// tunInet6Address 是下发给 TUN 的 IPv6 地址（与 mihomo 内置默认值一致）。
//
// 只在开启 TUN 且判定需要承运 IPv6 时才下发；不出现在 config.yaml 的基线里。
const tunInet6Address = "fdfe:dcba:9876::1/126"

// tunDNSHijack 与基线保持一致：拦截全部 53 端口（v4/v6 同表）。
var tunDNSHijack = []string{"any:53", "tcp://any:53"}

// HandleTunSettings 处理 /config/tun：
//
//	GET  → {effective, defaults, host_ipv6}
//	POST → 部分更新（只传要改的键），归一化 + 校验 + 稀疏落盘
//
// 本接口**不碰内核、不碰 config.yaml**：设置只在下次开启 TUN 时组装成 PATCH 下发。
func HandleTunSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		httpx.RespondJSON(w, http.StatusOK, map[string]any{
			"effective": Current(),
			"defaults":  Defaults(),
			"host_ipv6": HostIPv6Available(),
			// 订阅中心的「IPv6 支持」总开关：关闭时接管偏好照旧保存，但开启 TUN 时
			// 不会下发 IPv6 地址（见 HandleTunState），前端据此把下拉置灰并说明原因。
			"global_ipv6": config.CurrentSnapshot().IPv6,
		})

	case http.MethodPost:
		var payload struct {
			Stack   *string `json:"stack"`
			Device  *string `json:"device"`
			DNSMode *string `json:"dns_mode"`
			IPv6    *string `json:"ipv6"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "无效的请求格式: "+err.Error())
			return
		}

		next := Current()
		if payload.Stack != nil {
			next.Stack = *payload.Stack
		}
		if payload.Device != nil {
			next.Device = *payload.Device
		}
		if payload.DNSMode != nil {
			next.DNSMode = *payload.DNSMode
		}
		if payload.IPv6 != nil {
			next.IPv6 = *payload.IPv6
		}

		saved, err := Save(next)
		if err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		logx.Info(logx.ModuleTun, "tun settings saved: stack=%s device=%q dns-mode=%s ipv6=%s",
			saved.Stack, saved.Device, saved.DNSMode, saved.IPv6)
		httpx.RespondJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"effective": saved,
			"defaults":  Defaults(),
			"host_ipv6": HostIPv6Available(),
		})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// HandleTunState 处理 /config/tun/state：开启/关闭 TUN。
//
// 开启时由后端组装**完整的** tun 对象再 PATCH 内核。只发 {enable:true} 是不行的：
// mihomo 的 patchConfigs 用 listener.LastTunConf（来自 config.yaml 的基线）作为基底，
// 缺省的字段会静默回落到基线，用户的 stack / dns-mode / IPv6 偏好就全丢了。
//
// 关闭时只发 {enable:false}。开关状态不落盘、不回放：重载或重启后 TUN 必然关闭。
func HandleTunState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Enable bool `json:"enable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "无效的请求格式: "+err.Error())
		return
	}

	if !payload.Enable {
		if err := patchCoreTun(map[string]any{"enable": false}); err != nil {
			httpx.WriteJSONError(w, http.StatusBadGateway, "关闭 TUN 失败: "+err.Error())
			return
		}
		logx.Info(logx.ModuleTun, "tun disabled")
		httpx.RespondJSON(w, http.StatusOK, map[string]any{"status": "ok", "enable": false})
		return
	}

	// TUN 与 TProxy 互斥：两者都改宿主路由/防火墙，同时开启会互相破坏对方的流量路径。
	// 前端会先关 TProxy 再调本接口，但边界必须由后端守（第三方面板/curl 都绕不过）。
	if tproxy.GetTproxyState() {
		httpx.WriteJSONError(w, http.StatusBadRequest,
			"TPROXY 正在接管流量，请先关闭 TPROXY 再开启 TUN（两者互斥）")
		return
	}

	settings := Current()
	if settings.IPv6 == configgen.TunIPv6On && !HostIPv6Available() {
		httpx.WriteJSONError(w, http.StatusBadRequest,
			"本机新建接口默认禁用 IPv6，强制接管会让 TUN 启动失败；请先执行 "+
				"sysctl -w net.ipv6.conf.default.disable_ipv6=0 后重试")
		return
	}

	globalIPv6 := config.CurrentSnapshot().IPv6
	inet6 := settings.ipv6Enabled() && globalIPv6
	switch {
	case !globalIPv6 && settings.IPv6 == configgen.TunIPv6On:
		// 显式要求接管、但订阅中心把 IPv6 总开关关了：不静默照做，也不因此拒绝开启
		// TUN（v6 只是少一条能力）——日志留痕，响应里带上实际取值。
		logx.Warn(logx.ModuleTun,
			"tun ipv6 takeover skipped: global ipv6 is disabled (tun ipv6 = on)")
	case settings.IPv6 == configgen.TunIPv6Auto && !inet6:
		logx.Info(logx.ModuleTun,
			"tun ipv6 auto = off (host_ipv6=%v global_ipv6=%v)", HostIPv6Available(), globalIPv6)
	}

	if err := patchCoreTun(runtimeConfig(settings, inet6)); err != nil {
		httpx.WriteJSONError(w, http.StatusBadGateway, "开启 TUN 失败: "+err.Error())
		return
	}

	logx.Info(logx.ModuleTun, "tun enabled: stack=%s device=%q dns-mode=%s ipv6=%s inet6=%v",
		settings.Stack, settings.Device, settings.DNSMode, settings.IPv6, inet6)
	httpx.RespondJSON(w, http.StatusOK, map[string]any{"status": "ok", "enable": true, "ipv6": inet6})
}

// runtimeConfig 组装开启 TUN 时下发给内核的完整 tun 对象。
//
// 字段集合与 configgen.TunBlock 保持一致，另外按设置覆盖 stack / device / dns-mode /
// inet6-address。auto-route / auto-redirect 固定为 true：dns-mode=hijack 在 Linux 上
// 依赖 auto-redirect 的 nft 链，关掉它会让 hijack 静默退化成 native。
func runtimeConfig(s Settings, inet6 bool) map[string]any {
	inet6Addresses := []string{}
	if inet6 {
		inet6Addresses = []string{tunInet6Address}
	}

	cfg := map[string]any{
		"enable":                true,
		"stack":                 s.Stack,
		"dns-mode":              s.DNSMode,
		"inet6-address":         inet6Addresses,
		"dns-hijack":            tunDNSHijack,
		"auto-route":            true,
		"auto-redirect":         true,
		"auto-detect-interface": true,
	}
	if s.Device != "" {
		cfg["device"] = s.Device
	}
	return cfg
}

// patchCoreTun 把 tun 对象 PATCH 给内核，失败时带上内核的错误正文。
func patchCoreTun(tunConfig map[string]any) error {
	body, err := json.Marshal(map[string]any{"tun": tunConfig})
	if err != nil {
		return err
	}

	resp, err := core.CoreRequest(http.MethodPatch, "/configs", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		msg, _ := httpx.ReadAllLimited(resp.Body, httpx.MaxUpstreamBody)
		text := strings.TrimSpace(string(msg))
		if text == "" {
			text = resp.Status
		}
		return fmt.Errorf("内核拒绝：%s", text)
	}
	return nil
}
