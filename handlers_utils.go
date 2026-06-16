package main

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sync"
)

// Settings 全局设置结构
type Settings struct {
	Enabled          bool   `json:"enabled"`
	URL              string `json:"url"`
	TokenEnabled     bool   `json:"token_enabled"`
	Token            string `json:"token"`
	ModifyConfig     bool   `json:"modify_config"`
	ConfigBackendURL string `json:"config_backend_url"`
}

var (
	settings    Settings
	proxyMutex  sync.RWMutex
	updateMutex sync.Mutex
)

// 地址格式验证正则（代理和后端地址共用）
var backendURLRegex = regexp.MustCompile(`^https?://(([0-9]{1,3}\.){3}[0-9]{1,3}|([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}):([1-9][0-9]{0,3}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])$`)

// writeJSONError 统一返回 JSON 格式的错误响应
func writeJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": message})
}