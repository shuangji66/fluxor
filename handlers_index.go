package main

import (
	"net/http"
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