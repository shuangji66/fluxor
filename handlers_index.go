package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
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

var (
	latestVersionCache     string
	latestVersionCacheTime time.Time
	cacheMutex             sync.RWMutex
	cacheTTL               = 10 * time.Minute
	latestReleaseCache     *githubRelease
	latestReleaseCacheTime time.Time
	releaseCacheMutex      sync.RWMutex
)

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// stripVersionSuffix 去除版本号中的后缀（如 ~670ab34），仅保留主版本号
func stripVersionSuffix(v string) string {
	parts := strings.Split(v, "~")
	if len(parts) > 0 {
		return parts[0]
	}
	return v
}

// handleCheckUpdate 检查 Fluxor 自身是否有新版本
func handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	current := r.URL.Query().Get("current")
	if current == "" {
		writeJSONError(w, http.StatusBadRequest, "missing current version")
		return
	}
	current = stripVersionSuffix(current)

	latest, err := getLatestVersion()
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "failed to check update: "+err.Error())
		return
	}

	hasUpdate := compareVersions(latest, current) > 0
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hasUpdate": hasUpdate,
		"latest":    latest,
		"current":   current,
	})
}

// getLatestVersion 从 GitHub API 获取最新 release 版本号（带缓存）
func getLatestVersion() (string, error) {
	cacheMutex.RLock()
	if latestVersionCache != "" && time.Since(latestVersionCacheTime) < cacheTTL {
		cacheMutex.RUnlock()
		return latestVersionCache, nil
	}
	cacheMutex.RUnlock()

	url := "https://api.github.com/repos/shuangji66/fluxor/releases/latest"
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", err // 可进一步处理非200状态
	}

	var result struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	// 去除前缀 'v'
	version := strings.TrimPrefix(result.TagName, "v")
	if version == "" {
		version = result.TagName
	}

	cacheMutex.Lock()
	latestVersionCache = version
	latestVersionCacheTime = time.Now()
	cacheMutex.Unlock()

	return version, nil
}

// compareVersions 比较两个语义化版本号，返回 1（v1 > v2）、-1（v1 < v2）、0（相等）
// 格式如 "1.2.3" 或 "1.2"，无前缀 'v'
func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			n1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			n2, _ = strconv.Atoi(parts2[i])
		}
		if n1 > n2 {
			return 1
		}
		if n1 < n2 {
			return -1
		}
	}
	return 0
}

// getLatestReleaseInfo 获取完整 release 信息（带缓存）
func getLatestReleaseInfo() (*githubRelease, error) {
	releaseCacheMutex.RLock()
	if latestReleaseCache != nil && time.Since(latestReleaseCacheTime) < cacheTTL {
		releaseCacheMutex.RUnlock()
		return latestReleaseCache, nil
	}
	releaseCacheMutex.RUnlock()

	url := "https://api.github.com/repos/shuangji66/fluxor/releases/latest"
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	// 去除 tag 前缀 v
	rel.TagName = strings.TrimPrefix(rel.TagName, "v")

	releaseCacheMutex.Lock()
	latestReleaseCache = &rel
	latestReleaseCacheTime = time.Now()
	releaseCacheMutex.Unlock()

	return &rel, nil
}

// handleSelfUpdate 更新自身
func handleSelfUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	rel, err := getLatestReleaseInfo()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "获取版本信息失败: "+err.Error())
		return
	}

	current := r.URL.Query().Get("current")
	if current == "" {
		current = "0.0.0"
	}
	if compareVersions(rel.TagName, current) <= 0 {
		writeJSONError(w, http.StatusBadRequest, "当前已是最新版本，无需更新")
		return
	}
	current = stripVersionSuffix(current)


	// 确定目标路径
	targetPath := filepath.Join(fluxorBinDir, "fluxor")
	// 备份目录
	backupDir := filepath.Join(fluxorBinDir, "fluxor-backup")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "创建备份目录失败: "+err.Error())
		return
	}

	// 架构匹配
	archMap := map[string]string{
		"amd64":  "fluxor-amd64",
		"arm64":  "fluxor-arm64",
		"arm":    "fluxor-arm64",
		"x86":    "fluxor-amd64",
		"x86_64": "fluxor-amd64",
	}
	expectedName := archMap[runtime.GOARCH]
	if expectedName == "" {
		writeJSONError(w, http.StatusBadRequest, "不支持的架构: "+runtime.GOARCH)
		return
	}

	var downloadURL string
	for _, asset := range rel.Assets {
		if asset.Name == expectedName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		writeJSONError(w, http.StatusNotFound, "未找到对应架构的发布文件: "+expectedName)
		return
	}

	// 1. 下载到临时文件
	tmpFile, err := os.CreateTemp("", "fluxor-update-*")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "创建临时文件失败: "+err.Error())
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	resp, err := http.Get(downloadURL)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "下载失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeJSONError(w, http.StatusInternalServerError, "下载失败，状态码: "+strconv.Itoa(resp.StatusCode))
		return
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "写入临时文件失败: "+err.Error())
		return
	}
	tmpFile.Close()
	if err := os.Chmod(tmpPath, 0755); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "设置临时文件权限失败: "+err.Error())
		return
	}

	// 2. 备份旧文件（如果存在）
	if _, err := os.Stat(targetPath); err == nil {
        backupName := filepath.Join(backupDir, "fluxor")
        if err := os.Rename(targetPath, backupName); err != nil {
            writeJSONError(w, http.StatusInternalServerError, "备份旧文件失败: "+err.Error())
            return
        }
    }

	// 3. 复制新文件到目标路径
	srcFile, err := os.Open(tmpPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "打开临时文件失败: "+err.Error())
		return
	}
	defer srcFile.Close()

	dstFile, err := os.Create(targetPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "创建目标文件失败: "+err.Error())
		return
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "复制文件失败: "+err.Error())
		return
	}
	if err := os.Chmod(targetPath, 0755); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "设置目标文件权限失败: "+err.Error())
		return
	}

	// 4. 响应成功
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "更新成功，即将重启"})

	// 5. 启动新进程并退出
	go func() {
		time.Sleep(200 * time.Millisecond)
		cmd := exec.Command(targetPath, os.Args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = os.Environ()
		if err := cmd.Start(); err != nil {
			fmt.Printf("重启失败: %v\n", err)
			return
		}
		os.Exit(0)
	}()
}