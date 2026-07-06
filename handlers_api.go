package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// ---------- 仪表盘数据 API ----------

// handleVersion 返回内核版本信息（代理 /version）
func handleVersion(w http.ResponseWriter, _ *http.Request) {
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
func handleTraffic(w http.ResponseWriter, _ *http.Request) {
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
func handleMemory(w http.ResponseWriter, _ *http.Request) {
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
func handleConnections(w http.ResponseWriter, _ *http.Request) {
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
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "读取请求体失败: "+err.Error())
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		var fields map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &fields); err == nil {
			if tpVal, ok := fields["tproxy-port"]; ok {
				if tpPort, ok := tpVal.(float64); ok {
					if tpPort > 0 {
						disableTProxyRules()
						enableTProxyRules(int(tpPort))
					} else {
						disableTProxyRules()
					}
				}
			}
		}

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
	// 构建目标路径，包含查询参数
	targetPath := "/upgrade"
	if r.URL.RawQuery != "" {
		targetPath += "?" + r.URL.RawQuery
	}
	resp, err := coreRequest("POST", targetPath, nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "请求内核失败: "+err.Error())
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}