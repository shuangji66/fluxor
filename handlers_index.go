package main

import (
	"net/http"
	"encoding/json"
)

// handleIndex 渲染主页单页应用
func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != baseURL && r.URL.Path != baseURL+"/" {
		http.NotFound(w, r)
		return
	}
	if indexTmpl == nil {
		http.Error(w, "主页模板未加载", http.StatusInternalServerError)
		return
	}
	indexTmpl.Execute(w, map[string]string{"BaseURL": baseURL})
}

// handleWhoAmI 返回当前用户信息（从 X-Trim-Username 请求头读取）
func handleWhoAmI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	username := r.Header.Get("X-Trim-Username")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"username": username})
}