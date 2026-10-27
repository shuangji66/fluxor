package dashapi

import (
	"encoding/json"
	"fluxor/internal/config"
	"fluxor/internal/core"
	"fluxor/internal/httpx"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// HandleProxies 获取所有代理组信息（代理 GET /proxies）
func HandleProxies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	resp, err := core.CoreRequest("GET", "/proxies", nil)
	if err != nil {
		httpx.WriteJSONError(w, http.StatusBadGateway, "获取代理列表失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	// 转发内核状态码：恒回 200 会让前端的 resp.ok 形同虚设，
	// 内核的错误响应体会被当成数据解析（例如把 {"message":...} 当规则列表）
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// HandleProxyDelay 测速（GET /proxies/{name}/delay）
func HandleProxyDelay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.EscapedPath()
	trimmed := strings.TrimPrefix(path, config.BaseURL+"/proxies/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 || parts[1] != "delay" {
		httpx.WriteJSONError(w, http.StatusBadRequest, "无效的请求路径")
		return
	}
	proxyName := parts[0]
	if !validateSinglePathSegment(proxyName) {
		httpx.WriteJSONError(w, http.StatusBadRequest, "无效的代理名称")
		return
	}
	targetPath := "/proxies/" + proxyName + "/delay?" + r.URL.RawQuery
	resp, err := core.CoreRequest("GET", targetPath, nil)
	if err != nil {
		httpx.WriteJSONError(w, http.StatusBadGateway, "测速失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	// 转发内核状态码：恒回 200 会让前端的 resp.ok 形同虚设，
	// 内核的错误响应体会被当成数据解析（例如把 {"message":...} 当规则列表）
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// HandleProxySwitch 切换代理选择（PUT /proxies/{name}）
func HandleProxySwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.EscapedPath()
	trimmed := strings.TrimPrefix(path, config.BaseURL+"/proxies/")
	if !validateSinglePathSegment(trimmed) {
		httpx.WriteJSONError(w, http.StatusBadRequest, "无效的代理名称")
		return
	}
	proxyName := trimmed
	resp, err := core.CoreRequest("PUT", "/proxies/"+proxyName, r.Body)
	if err != nil {
		httpx.WriteJSONError(w, http.StatusBadGateway, "切换代理失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// HandleGroupDelay 测试策略组内所有节点的延迟（代理 GET /group/{name}/delay）
// 请求示例：/group/🚀%20节点选择/delay?url=https://www.gstatic.com/generate_204&timeout=5000
// 返回 JSON: {"节点A": 120, "节点B": 350, ...}
func HandleGroupDelay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.EscapedPath()
	// 去掉 baseURL 前缀，提取剩余路径
	trimmed := strings.TrimPrefix(path, config.BaseURL+"/group/")
	if trimmed == "" {
		httpx.WriteJSONError(w, http.StatusBadRequest, "缺少策略组名称")
		return
	}
	// 必须为 /group/{name}/delay 格式
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 || parts[1] != "delay" {
		httpx.WriteJSONError(w, http.StatusBadRequest, "无效的请求路径，应为 /group/{name}/delay")
		return
	}
	groupName := parts[0]
	if !validateSinglePathSegment(groupName) {
		httpx.WriteJSONError(w, http.StatusBadRequest, "无效的策略组名称")
		return
	}
	// 构建目标路径，附带原始查询参数
	targetPath := "/group/" + groupName + "/delay?" + r.URL.RawQuery
	resp, err := core.CoreRequest("GET", targetPath, nil)
	if err != nil {
		httpx.WriteJSONError(w, http.StatusBadGateway, "测速失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// TestGroupDelay 对指定策略组进行测速（不关心返回值，仅触发）
func TestGroupDelay(groupName string) {
	target := fmt.Sprintf("/group/%s/delay?url=%s&timeout=%d",
		url.PathEscape(groupName),
		url.QueryEscape("https://www.gstatic.com/generate_204"),
		5000)
	resp, err := core.CoreRequest("GET", target, nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

// GetAllProxyGroups 从内核获取所有策略组（Selector/URLTest/Fallback/LoadBalance）
func GetAllProxyGroups() ([]string, error) {
	resp, err := core.CoreRequest("GET", "/proxies", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("内核返回状态码 %d", resp.StatusCode)
	}
	var data struct {
		Proxies map[string]struct {
			Type string `json:"type"`
		} `json:"proxies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	var groups []string
	for name, proxy := range data.Proxies {
		if proxy.Type == "Selector" || proxy.Type == "URLTest" || proxy.Type == "Fallback" || proxy.Type == "LoadBalance" {
			groups = append(groups, name)
		}
	}
	return groups, nil
}
