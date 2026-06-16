package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// handleUpdateMeta 更新 MetaCubeXD 面板
func handleUpdateMeta(w http.ResponseWriter, r *http.Request) {
	updateMutex.Lock()
	defer updateMutex.Unlock()

	err := updateMeta()
	success := err == nil
	msg := "MetaCubeXD 更新成功"
	if !success {
		msg = "MetaCubeXD 更新失败: " + err.Error()
	} else {
		proxyMutex.RLock()
		mod := settings.ModifyConfig
		backend := settings.ConfigBackendURL
		proxyMutex.RUnlock()
		if mod && backend != "" && backendURLRegex.MatchString(backend) {
			if e := modifyMetaConfig(backend); e != nil {
				msg += "，config.js 更新失败: " + e.Error()
			} else {
				msg += "，config.js 已更新"
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": success, "message": msg})
}

// handleUpdateZash 更新 Zashboard 面板
func handleUpdateZash(w http.ResponseWriter, r *http.Request) {
	updateMutex.Lock()
	defer updateMutex.Unlock()

	err := updateZash()
	success := err == nil
	msg := "Zashboard 更新成功"
	if !success {
		msg = "Zashboard 更新失败: " + err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": success, "message": msg})
}

// updateMeta 执行 MetaCubeXD 更新逻辑
func updateMeta() error {
	return downloadAndUpdate("MetaCubeX", "metacubexd", "compressed-dist.tgz", metaDir, true)
}

// updateZash 执行 Zashboard 更新逻辑
func updateZash() error {
	return downloadAndUpdate("Zephyruso", "zashboard", "dist-cdn-fonts.zip", zashDir, false)
}

// downloadAndUpdate 从 GitHub 下载指定资源并更新到目标目录
func downloadAndUpdate(owner, repo, asset, destDir string, isTgz bool) error {
	downloadURL, err := getLatestAssetURL(owner, repo, asset)
	if err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp("", "update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	if isTgz {
		if err = downloadAndExtractTgz(downloadURL, tmpDir); err != nil {
			return err
		}
	} else {
		if err = downloadAndExtractZip(downloadURL, tmpDir); err != nil {
			return err
		}
		src := filepath.Join(tmpDir, "dist")
		if _, err := os.Stat(src); os.IsNotExist(err) {
			return fmt.Errorf("压缩包内未找到 dist 目录")
		}
		tmpDir = src
	}
	return replaceDir(tmpDir, destDir)
}

// modifyMetaConfig 修改 MetaCubeXD 的 config.js 文件中的后端地址
func modifyMetaConfig(backendURL string) error {
	configPath := filepath.Join(metaDir, metaConfigFile)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("config.js 不存在")
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`defaultBackendURL:\s*''`)
	newContent := re.ReplaceAllString(string(content), fmt.Sprintf("defaultBackendURL: '%s'", backendURL))
	if string(content) == newContent {
		return fmt.Errorf("未找到 defaultBackendURL 配置项或格式不匹配")
	}
	return os.WriteFile(configPath, []byte(newContent), 0644)
}

// newHTTPClient 根据代理设置创建 HTTP 客户端
func newHTTPClient() *http.Client {
	proxyMutex.RLock()
	defer proxyMutex.RUnlock()
	transport := &http.Transport{}
	if settings.Enabled && settings.URL != "" {
		if proxyURL, err := url.Parse(settings.URL); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	return &http.Client{Transport: transport}
}

// getLatestAssetURL 获取指定仓库最新 release 中指定资源的下载 URL
func getLatestAssetURL(owner, repo, asset string) (string, error) {
	api := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	req, _ := http.NewRequest("GET", api, nil)
	req.Header.Set("User-Agent", "Fluxor-Updater")
	req.Header.Set("Accept", "application/vnd.github+json")
	proxyMutex.RLock()
	if settings.TokenEnabled && settings.Token != "" {
		req.Header.Set("Authorization", "Bearer "+settings.Token)
	}
	proxyMutex.RUnlock()
	resp, err := newHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API 状态码 %d", resp.StatusCode)
	}
	var release struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	for _, a := range release.Assets {
		if a.Name == asset {
			return a.URL, nil
		}
	}
	return "", fmt.Errorf("未找到资源 %s", asset)
}

// downloadAndExtractTgz 下载 tgz 文件并解压到目标目录
func downloadAndExtractTgz(downloadURL, dest string) error {
	resp, err := newHTTPClient().Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败，状态码 %d", resp.StatusCode)
	}
	gz, _ := gzip.NewReader(resp.Body)
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, hdr.Name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			continue
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			f, _ := os.Create(target)
			io.Copy(f, tr)
			f.Close()
		}
	}
	return nil
}

// downloadAndExtractZip 下载 zip 文件并解压到目标目录
func downloadAndExtractZip(downloadURL, dest string) error {
	resp, err := newHTTPClient().Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败，状态码 %d", resp.StatusCode)
	}
	tmp, _ := os.CreateTemp("", "fluxor-zip")
	defer os.Remove(tmp.Name())
	io.Copy(tmp, resp.Body)
	tmp.Close()
	r, _ := zip.OpenReader(tmp.Name())
	defer r.Close()
	for _, f := range r.File {
		target := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			continue
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(target), 0755)
		rc, _ := f.Open()
		out, _ := os.Create(target)
		io.Copy(out, rc)
		out.Close()
		rc.Close()
	}
	return nil
}

// replaceDir 删除目标目录，然后将源目录内容复制过去
func replaceDir(src, dest string) error {
	os.RemoveAll(dest)
	os.MkdirAll(filepath.Dir(dest), 0755)
	return copyDir(src, dest)
}

// copyDir 递归复制目录
func copyDir(src, dest string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dest, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, _ := os.ReadFile(path)
		return os.WriteFile(target, data, info.Mode())
	})
}