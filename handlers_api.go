package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"fmt"
	"time"
	"strconv"
)

// ---------- 仪表盘数据 API ----------

// handleVersion 返回内核版本信息（代理 /version）
func handleVersion(w http.ResponseWriter, r *http.Request) {
	resp, err := coreRequest("GET", "/version", nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "无法获取内核版本: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

// handleTraffic 返回实时流量信息（代理 /traffic）
func handleTraffic(w http.ResponseWriter, r *http.Request) {
	resp, err := coreRequest("GET", "/traffic", nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "无法获取流量信息: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

// handleMemory 返回内存使用信息（代理 /memory）
func handleMemory(w http.ResponseWriter, r *http.Request) {
	resp, err := coreRequest("GET", "/memory", nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "无法获取内存信息: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

// handleConnections 返回连接信息（代理 /connections）
func handleConnections(w http.ResponseWriter, r *http.Request) {
	resp, err := coreRequest("GET", "/connections", nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "无法获取连接信息: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

// ---------- 配置管理 ----------

// handleConfigsAPI 处理配置的获取、修改和重载
func handleConfigsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		resp, err := coreRequest("GET", "/configs", nil)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, "获取配置失败: "+err.Error())
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		io.Copy(w, resp.Body)

	case http.MethodPatch:
		resp, err := coreRequest("PATCH", "/configs", r.Body)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, "修改配置失败: "+err.Error())
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)

	case http.MethodPut:
		if err := reloadCore(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "重载配置失败: "+err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// handleRestart 重启内核（POST /restart）
func handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	resp, err := coreRequest("POST", "/restart", strings.NewReader(`{"path": "", "payload": ""}`))
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "重启内核失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// handleConfigsGeo 更新 GEO 数据库（POST /configs/geo）
func handleConfigsGeo(w http.ResponseWriter, r *http.Request) {
	resp, err := coreRequest("POST", "/configs/geo", nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "更新 GEO 失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
}

// handleProvidersGeo 更新 GEO 数据库（回退接口，POST /providers/geo）
func handleProvidersGeo(w http.ResponseWriter, r *http.Request) {
	resp, err := coreRequest("POST", "/providers/geo", nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "更新 GEO 失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
}

// handleFlushFakeIP 清空 FakeIP 缓存（POST /cache/fakeip/flush）
func handleFlushFakeIP(w http.ResponseWriter, r *http.Request) {
	resp, err := coreRequest("POST", "/cache/fakeip/flush", nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "清空 FakeIP 失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(http.StatusOK)
}

// handleFlushDNS 清空 DNS 缓存（POST /cache/dns/flush）
func handleFlushDNS(w http.ResponseWriter, r *http.Request) {
	resp, err := coreRequest("POST", "/cache/dns/flush", nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "清空 DNS 缓存失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(http.StatusOK)
}

// handleDNSQuery 执行 DNS 查询（代理 /dns/query）
func handleDNSQuery(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	qtype := r.URL.Query().Get("type")
	path := "/dns/query?name=" + name + "&type=" + qtype
	resp, err := coreRequest("GET", path, nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "DNS 查询失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

// handleConnectionsClose 关闭单个连接或所有连接（DELETE /connections 或 /connections/{id}）
func handleConnectionsClose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// 从路径中提取 ID（支持 /connections 和 /connections/xxx）
	path := strings.TrimPrefix(r.URL.Path, baseURL+"/connections")
	var id string
	if path != "" && path != "/" {
		id = strings.TrimPrefix(path, "/")
	}

	var targetPath string
	if id != "" {
		targetPath = "/connections/" + id
	} else {
		targetPath = "/connections"
	}

	resp, err := coreRequest("DELETE", targetPath, nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "关闭连接失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
}

// handleProxies 获取所有代理组信息（代理 GET /proxies）
func handleProxies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	resp, err := coreRequest("GET", "/proxies", nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "获取代理列表失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

// handleProxyDelay 测速（GET /proxies/{name}/delay）
func handleProxyDelay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.EscapedPath()
	trimmed := strings.TrimPrefix(path, baseURL+"/proxies/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 || parts[1] != "delay" {
		writeJSONError(w, http.StatusBadRequest, "无效的请求路径")
		return
	}
	proxyName := parts[0]
	targetPath := "/proxies/" + proxyName + "/delay?" + r.URL.RawQuery
	resp, err := coreRequest("GET", targetPath, nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "测速失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

// handleProxySwitch 切换代理选择（PUT /proxies/{name}）
func handleProxySwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.EscapedPath()
	trimmed := strings.TrimPrefix(path, baseURL+"/proxies/")
	if trimmed == "" || strings.Contains(trimmed, "/") {
		writeJSONError(w, http.StatusBadRequest, "无效的代理名称")
		return
	}
	proxyName := trimmed
	resp, err := coreRequest("PUT", "/proxies/"+proxyName, r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "切换代理失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// ---------- 规则相关 API ----------

// handleRules 获取所有规则（代理 GET /rules）
func handleRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	resp, err := coreRequest("GET", "/rules", nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "获取规则列表失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

// handleRuleProviders 获取规则提供商（代理 GET /providers/rules）
func handleRuleProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	resp, err := coreRequest("GET", "/providers/rules", nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "获取规则提供商失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

// handleUpdateRuleProvider 更新规则提供商（PUT /providers/rules/{name}）
func handleUpdateRuleProvider(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.EscapedPath()
	// 使用 baseURL + "/providers/rules/" 作为前缀
	trimmed := strings.TrimPrefix(path, baseURL+"/providers/rules/")
	if trimmed == "" || strings.Contains(trimmed, "/") {
		writeJSONError(w, http.StatusBadRequest, "无效的提供商名称")
		return
	}
	providerName := trimmed
	targetPath := "/providers/rules/" + providerName
	resp, err := coreRequest("PUT", targetPath, r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "更新规则提供商失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// handleRulesDisable 禁用/启用规则（代理 PATCH /rules/disable）
func handleRulesDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	resp, err := coreRequest("PATCH", "/rules/disable", r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "规则禁用/启用失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
// handleProviderProxies 获取指定订阅的代理信息（含流量/有效期）
// GET /providers/proxies/{encodedName}
// handleProviderProxies 处理订阅信息获取（GET）和更新（PUT）
func handleProviderProxies(w http.ResponseWriter, r *http.Request) {
	// 提取路径 /providers/proxies/{name}
	path := r.URL.EscapedPath()
	trimmed := strings.TrimPrefix(path, baseURL+"/providers/proxies/")
	if trimmed == "" {
		writeJSONError(w, http.StatusBadRequest, "缺少代理名称")
		return
	}
	targetPath := "/providers/proxies/" + trimmed

	var resp *http.Response
	var err error

	switch r.Method {
	case http.MethodGet:
		resp, err = coreRequest("GET", targetPath, nil)
	case http.MethodPut:
		resp, err = coreRequest("PUT", targetPath, r.Body)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "请求失败: "+err.Error())
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
// handleUpgrade 更新内核（POST /upgrade）
func handleUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}

	resp, err := coreRequest("POST", "/upgrade", nil)
	if err != nil {
		// 连接内核失败（如内核未运行或 socket 不可达）
		writeJSONError(w, http.StatusBadGateway, "请求内核失败: "+err.Error())
		return
	}
	defer resp.Body.Close()

	// 原样透传状态码和响应体（包括 500 以及 JSON 消息）
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// handleInterfaces 返回系统所有的物理网络接口名称（GET /interfaces）
func handleInterfaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "获取网络接口失败: "+err.Error())
		return
	}

	var names []string
	for _, iface := range ifaces {
		// 过滤回环接口和未启用的接口
		if (iface.Flags & net.FlagLoopback) != 0 {
			continue
		}
		if (iface.Flags & net.FlagUp) == 0 {
			continue
		}
		names = append(names, iface.Name)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(names)
}

// handleLocalIPv4 获取本机 IPv4
func handleLocalIPv4(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	ip, err := fetchPublicIPWithFallback("http://ip-api.com/json?fields=query", "https://api.ipify.org?format=json", "")
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "获取本机 IPv4 失败: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ip": ip})
}

// handleLocalIPv6 获取本机 IPv6
func handleLocalIPv6(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	ip, err := fetchPublicIP("https://api6.ipify.org?format=json", "")
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "获取本机 IPv6 失败: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ip": ip})
}

// handleProxyIPv4 获取代理 IPv4
func handleProxyIPv4(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	proxyPort := getProxyPortFromConfig()
	if proxyPort == 0 {
		writeJSONError(w, http.StatusServiceUnavailable, "无可用代理端口")
		return
	}
	proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", proxyPort)
	ip, err := fetchPublicIP("https://api.ipify.org?format=json", proxyAddr)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "获取代理 IPv4 失败: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ip": ip})
}

// handleProxyIPv6 获取代理 IPv6
func handleProxyIPv6(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	proxyPort := getProxyPortFromConfig()
	if proxyPort == 0 {
		writeJSONError(w, http.StatusServiceUnavailable, "无可用代理端口")
		return
	}
	proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", proxyPort)
	ip, err := fetchPublicIP("https://api6.ipify.org?format=json", proxyAddr)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "获取代理 IPv6 失败: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ip": ip})
}

// getProxyPortFromConfig 从内核配置中获取代理端口
func getProxyPortFromConfig() int {
	resp, err := coreRequest("GET", "/configs", nil)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0
	}
	var cfg map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return 0
	}
	// 优先 mixed-port
	if p, ok := cfg["mixed-port"]; ok {
		if port, ok := p.(float64); ok && port > 0 {
			return int(port)
		}
	}
	if p, ok := cfg["port"]; ok {
		if port, ok := p.(float64); ok && port > 0 {
			return int(port)
		}
	}
	if p, ok := cfg["socks-port"]; ok {
		if port, ok := p.(float64); ok && port > 0 {
			return int(port)
		}
	}
	return 0
}

// fetchPublicIP 支持通过代理获取 IP
func fetchPublicIP(apiURL, proxyAddr string) (string, error) {
    client := &http.Client{Timeout: 5 * time.Second}
    if proxyAddr != "" {
        proxyURL, err := url.Parse(proxyAddr)
        if err == nil {
            client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
        }
    }
    resp, err := client.Get(apiURL)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("HTTP %d", resp.StatusCode)
    }
    var data struct {
        IP    string `json:"ip"`
        Query string `json:"query"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
        return "", err
    }
    if data.IP != "" {
        return data.IP, nil
    }
    if data.Query != "" {
        return data.Query, nil
    }
    return "", fmt.Errorf("no ip field in response")
}
// fetchPublicIPWithFallback 尝试主 URL，失败则尝试备用 URL（无代理）
func fetchPublicIPWithFallback(primaryURL, fallbackURL, proxyAddr string) (string, error) {
	ip, err := fetchPublicIP(primaryURL, proxyAddr)
	if err == nil && ip != "" {
		return ip, nil
	}
	// 尝试备用 URL（无代理）
	ip, err = fetchPublicIP(fallbackURL, "")
	if err == nil && ip != "" {
		return ip, nil
	}
	return "", fmt.Errorf("所有尝试均失败: %v", err)
}
// testDelayThroughProxy 通过代理测试目标URL的延迟（HEAD请求），返回毫秒
func testDelayThroughProxy(targetURL string, timeout time.Duration) (int, error) {
    proxyPort := getProxyPortFromConfig()
    if proxyPort == 0 {
        return 0, fmt.Errorf("no proxy port available")
    }
    proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", proxyPort)
    proxyURL, err := url.Parse(proxyAddr)
    if err != nil {
        return 0, err
    }
    transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
    client := &http.Client{
        Transport: transport,
        Timeout:   timeout,
        // 禁止重定向，因为HEAD请求不需要
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            return http.ErrUseLastResponse
        },
    }
    start := time.Now()
    resp, err := client.Head(targetURL)
    if err != nil {
        return 0, err
    }
    defer resp.Body.Close()
    // 即使状态码不是200，也认为连接成功，只要建立连接即可
    elapsed := time.Since(start).Milliseconds()
    return int(elapsed), nil
}
// handleDelayTestGoogle 测试 Google 延迟
func handleDelayTestGoogle(w http.ResponseWriter, r *http.Request) {
    handleDelayTestCommon(w, r, "https://www.gstatic.com/generate_204")
}

// handleDelayTestMicrosoft 测试 Microsoft 延迟
func handleDelayTestMicrosoft(w http.ResponseWriter, r *http.Request) {
    handleDelayTestCommon(w, r, "https://www.microsoft.com")
}

// handleDelayTestApple 测试 Apple 延迟
func handleDelayTestApple(w http.ResponseWriter, r *http.Request) {
    handleDelayTestCommon(w, r, "https://www.apple.com")
}

// handleDelayTestYouTube 测试 YouTube 延迟
func handleDelayTestYouTube(w http.ResponseWriter, r *http.Request) {
    handleDelayTestCommon(w, r, "https://www.youtube.com")
}

// 公共处理函数
func handleDelayTestCommon(w http.ResponseWriter, r *http.Request, targetURL string) {
    if r.Method != http.MethodGet {
        writeJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
        return
    }
    // 读取超时参数，默认5000ms
    timeoutMs := 5000
    if t := r.URL.Query().Get("timeout"); t != "" {
        if val, err := strconv.Atoi(t); err == nil && val > 0 {
            timeoutMs = val
        }
    }
    timeout := time.Duration(timeoutMs) * time.Millisecond
    delay, err := testDelayThroughProxy(targetURL, timeout)
    if err != nil {
        // 超时或错误，返回 delay=null 或 -1，前端显示超时
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{"delay": nil, "error": err.Error()})
        return
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]int{"delay": delay})
}