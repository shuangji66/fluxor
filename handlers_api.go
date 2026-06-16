package main

import (
	"io"
	"net/http"
	"strings"
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
		// 代理内核 GET /configs 获取运行时配置
		resp, err := coreRequest("GET", "/configs", nil)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, "获取配置失败: "+err.Error())
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		io.Copy(w, resp.Body)

	case http.MethodPatch:
		// 代理内核 PATCH /configs 更新配置
		resp, err := coreRequest("PATCH", "/configs", r.Body)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, "修改配置失败: "+err.Error())
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)

	case http.MethodPut:
		// 热重载：PUT /configs?force=true
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
	w.WriteHeader(resp.StatusCode)
}

// handleFlushDNS 清空 DNS 缓存（POST /cache/dns/flush）
func handleFlushDNS(w http.ResponseWriter, r *http.Request) {
	resp, err := coreRequest("POST", "/cache/dns/flush", nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "清空 DNS 缓存失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
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