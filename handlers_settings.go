package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// handleSettings 提供代理设置的获取 (GET) 和更新 (POST) 接口
func handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		proxyMutex.RLock()
		defer proxyMutex.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settings)

	case http.MethodPost:
		var newSettings Settings
		if err := json.NewDecoder(r.Body).Decode(&newSettings); err != nil {
			http.Error(w, "无效的请求格式", http.StatusBadRequest)
			return
		}
		// 校验代理地址格式
		if newSettings.Enabled && newSettings.URL != "" && !backendURLRegex.MatchString(newSettings.URL) {
			http.Error(w, "代理地址格式不正确，需为 http(s)://host:port", http.StatusBadRequest)
			return
		}
		// 校验 MetaCubeXD 后端地址格式
		if newSettings.ModifyConfig && newSettings.ConfigBackendURL != "" && !backendURLRegex.MatchString(newSettings.ConfigBackendURL) {
			http.Error(w, "后端地址格式不正确，需为 http(s)://host:port", http.StatusBadRequest)
			return
		}

		proxyMutex.Lock()
		settings = newSettings
		proxyMutex.Unlock()

		// 持久化设置到文件
		if err := saveSettingsToFile(); err != nil {
			http.Error(w, "保存设置失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		msg := "设置已保存"
		// 如果开启了修改 MetaCubeXD 后端地址，则立即修改 config.js
		if newSettings.ModifyConfig && newSettings.ConfigBackendURL != "" {
			updateMutex.Lock()
			err := modifyMetaConfig(newSettings.ConfigBackendURL)
			updateMutex.Unlock()
			if err != nil {
				msg += "，但 config.js 更新失败: " + err.Error()
			} else {
				msg += "，config.js 已更新"
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"message": msg,
		})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// loadSettings 从文件加载代理设置到全局变量
func loadSettings() {
	proxyMutex.Lock()
	defer proxyMutex.Unlock()
	data, err := os.ReadFile(settingsFile)
	if err != nil {
		// 文件不存在是正常情况，使用默认空设置
		return
	}
	json.Unmarshal(data, &settings)
}

// saveSettingsToFile 将当前代理设置持久化到文件
func saveSettingsToFile() error {
	proxyMutex.RLock()
	defer proxyMutex.RUnlock()
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(settingsFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(settingsFile, data, 0644)
}