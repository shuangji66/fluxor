package netinfo

import (
	"encoding/json"
	"fluxor/internal/httpx"
	"fmt"
	"net/http"
)

// 经代理出口查询公网 IP 的地址清单（v4 查 IPv4，v6 查 IPv6）。
//
// 这些域名必须在 configgen 的 full 规则集模板（configgen/rules_full.go）里显式指向
// 代理组：它们会命中 networktest 规则集，而该组默认选中「🎯 全球直连」，查询于是
// 落到本机出口，概览卡片显示的会是本机归属地而非节点归属地。
// handlers_test.go 会校验两处保持一致——新增查询源时同步补模板规则。
var (
	proxyIPv4LookupURLs = []string{
		"https://api.ipify.org?format=json",
		"https://ipv4.icanhazip.com",
		"https://v4.ident.me",
	}
	proxyIPv6LookupURLs = []string{
		"https://api6.ipify.org?format=json",
		"https://ipv6.icanhazip.com",
		"https://v6.ident.me",
	}
)

// HandleLocalIPv4 获取本机 IPv4 及地理信息
func HandleLocalIPv4(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	urls := []string{
		"https://ipv4.ddnspod.com/",
		"https://myip.ipip.net",
		"https://ip.3322.net",
	}
	ip, err := fetchPublicIPWithFallback(r.Context(), urls, "")
	if err != nil {
		// 回退到网卡获取的本机局域网 IP
		if localIP, localErr := getLocalIPFromInterfaces(false); localErr == nil {
			ip = localIP
		} else {
			httpx.WriteJSONError(w, http.StatusServiceUnavailable, "获取本机 IPv4 失败: "+err.Error())
			return
		}
	}
	// 获取地理信息
	country, region, isp := fetchGeoInfo(r.Context(), ip)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"ip":      ip,
		"country": country,
		"region":  region,
		"isp":     isp,
	})
}

// HandleLocalIPv6 获取本机 IPv6
func HandleLocalIPv6(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	urls := []string{
		"https://ipv6.ddnspod.com",
		"https://v6.myip.la",
		"https://speed.neu6.edu.cn/getIP.php",
		"https://api6.ipify.org?format=json",
	}
	ip, err := fetchPublicIPWithFallback(r.Context(), urls, "")
	if err != nil {
		// 回退到网卡获取的本机全球单播 IPv6
		if localIP, localErr := getLocalIPFromInterfaces(true); localErr == nil {
			ip = localIP
		} else {
			httpx.WriteJSONError(w, http.StatusServiceUnavailable, "获取本机 IPv6 失败: "+err.Error())
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ip": ip})
}

// HandleProxyIPv4 获取代理 IPv4 及地理信息
func HandleProxyIPv4(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	proxyPort := GetProxyPortFromConfig()
	if proxyPort == 0 {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "无可用代理端口")
		return
	}
	proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", proxyPort)
	ip, err := fetchPublicIPWithFallback(r.Context(), proxyIPv4LookupURLs, proxyAddr)
	if err != nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "获取代理 IPv4 失败: "+err.Error())
		return
	}
	// 获取地理信息
	country, region, isp := fetchGeoInfo(r.Context(), ip)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"ip":      ip,
		"country": country,
		"region":  region,
		"isp":     isp,
	})
}

// HandleProxyIPv6 获取代理 IPv6
func HandleProxyIPv6(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	proxyPort := GetProxyPortFromConfig()
	if proxyPort == 0 {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "无可用代理端口")
		return
	}
	proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", proxyPort)
	ip, err := fetchPublicIPWithFallback(r.Context(), proxyIPv6LookupURLs, proxyAddr)
	if err != nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "获取代理 IPv6 失败: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ip": ip})
}
