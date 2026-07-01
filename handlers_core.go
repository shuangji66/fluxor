package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// 内核操作日志记录器
var coreLogger *log.Logger

// initCoreLogger 初始化内核日志记录器
func initCoreLogger() {
	dir := filepath.Dir(infoLogFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("无法创建日志目录 %s: %v\n", dir, err)
		return
	}
	file, err := os.OpenFile(infoLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("无法打开日志文件 %s: %v\n", infoLogFile, err)
		return
	}
	coreLogger = log.New(file, "", log.Ldate|log.Ltime|log.Lmicroseconds)
}

// 与内核 Unix socket 通信的 HTTP 客户端
var coreHTTPClient = &http.Client{
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var conn net.Conn
			var lastErr error
			for attempt := 0; attempt < 3; attempt++ {
				conn, lastErr = net.Dial("unix", coreSocket)
				if lastErr == nil {
					return conn, nil
				}
				if attempt < 2 {
					time.Sleep(time.Millisecond * 100)
				}
			}
			return nil, lastErr
		},
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     60 * time.Second,
	},
	Timeout: 90 * time.Second,
}

// cancelableReadCloser 在 Close 时释放 Context
type cancelableReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelableReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

// coreRequest 向内核发送 HTTP 请求，自动添加 Authorization 头
func coreRequest(method, path string, body io.Reader) (*http.Response, error) {
	subscribeMu.RLock()
	secret := subscribeConfig.PanelSecret
	subscribeMu.RUnlock()

	// 动态超时：测速与提供商拉取为 90s，其余普通请求 10s
	timeout := 10 * time.Second
	if strings.Contains(path, "/healthcheck") || strings.Contains(path, "/providers/") {
		timeout = 90 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	url := "http://localhost" + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	resp, err := coreHTTPClient.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}

	// 包装 Body 保证读取完毕后再释放 Context，防止流截断
	resp.Body = &cancelableReadCloser{
		ReadCloser: resp.Body,
		cancel:     cancel,
	}
	return resp, nil
}


// reloadCore 通过 Unix socket 重载内核配置（热重启）
func reloadCore() error {
    bodyJSON := fmt.Sprintf(`{"path":"%s"}`, configTarget)
    resp, err := coreRequest(http.MethodPut, "/configs?force=true", strings.NewReader(bodyJSON))
    if err != nil {
        return fmt.Errorf("内核重载请求失败: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
        respBody, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("内核返回错误状态 %d: %s", resp.StatusCode, string(respBody))
    }

    // 重载成功后，异步更新 nftables TProxy 规则
    go func() {
        time.Sleep(500 * time.Millisecond)
        resp2, err2 := coreRequest("GET", "/configs", nil)
        if err2 == nil {
            defer resp2.Body.Close()
            var info map[string]interface{}
            if err2 := json.NewDecoder(resp2.Body).Decode(&info); err2 == nil {
                if tp, ok := info["tproxy-port"]; ok {
                    if tpf, ok := tp.(float64); ok {
                        if tpf > 0 && GetTproxyState() {
                            disableTProxyRules()
                            enableTProxyRules(int(tpf))
                        } else {
                            disableTProxyRules()
                        }
                    }
                }
            }
        }
    }()

    return nil
}

// isCoreRunning 检查内核是否在运行（通过 PID 文件）
func isCoreRunning() bool {
	data, err := os.ReadFile(corePidFile)
	if err != nil {
		return false
	}
	pidStr := strings.TrimSpace(string(data))
	if pidStr == "" {
		return false
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

// startCore 启动内核进程
func startCore() error {
	if isCoreRunning() {
		return fmt.Errorf("内核已在运行")
	}

	// 确保配置文件存在，若不存在则使用 subscribeConfig 生成；若存在则强制补齐网关属性
	if _, err := os.Stat(configTarget); os.IsNotExist(err) {
		if err := generateConfig(subscribeConfig); err != nil {
			if coreLogger != nil {
				coreLogger.Printf("[START][ERROR] 生成配置文件失败: %v\n", err)
			}
			return fmt.Errorf("生成配置文件失败: %w", err)
		}
	}

	cmd := exec.Command(coreBin, "-d", coreWorkDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		if coreLogger != nil {
			coreLogger.Printf("[START][ERROR] 启动内核失败: %v, stderr: %s\n", err, stderr.String())
		}
		return fmt.Errorf("启动内核失败: %v, stderr: %s", err, stderr.String())
	}

	// 等待 1 秒，检查进程是否存活
	time.Sleep(1 * time.Second)
	err := cmd.Process.Signal(syscall.Signal(0))
	if err != nil {
		stderrContent := stderr.String()
		waitErr := cmd.Wait()
		if waitErr != nil {
			stderrContent += " (Wait err: " + waitErr.Error() + ")"
		}
		if stderrContent == "" {
			stderrContent = "进程已退出，无 stderr 输出"
		}
		if coreLogger != nil {
			coreLogger.Printf("[START][ERROR] 内核启动后立即退出: %s\n", stderrContent)
		}
		return fmt.Errorf("内核启动后立即退出: %s", stderrContent)
	}

	pid := cmd.Process.Pid
	os.MkdirAll(filepath.Dir(corePidFile), 0755)
	if err := os.WriteFile(corePidFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		if coreLogger != nil {
			coreLogger.Printf("[START][ERROR] 写入 PID 文件失败: %v\n", err)
		}
		return fmt.Errorf("写入 PID 文件失败: %v", err)
	}

	// 后台等待进程退出
	go func() {
		cmd.Wait()
		os.Remove(corePidFile)
	}()

	return nil
}

// stopCore 停止内核进程
func stopCore() error {
	disableTProxyRules() // 进程停掉前，立即释放系统 nft 规则
	_ = os.Remove(coreSocket)

	if !isCoreRunning() {
		return fmt.Errorf("内核未运行，停止操作被忽略")
	}
	data, _ := os.ReadFile(corePidFile)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	process, err := os.FindProcess(pid)
	if err != nil {
		if coreLogger != nil {
			coreLogger.Printf("[STOP][ERROR] 查找进程失败: %v\n", err)
		}
		return fmt.Errorf("查找进程失败: %v", err)
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		if coreLogger != nil {
			coreLogger.Printf("[STOP][ERROR] 停止进程失败: %v\n", err)
		}
		return fmt.Errorf("停止进程失败: %v", err)
	}

	// 轮询检查进程是否退出（最大 5 秒超时，每 100ms 一次）
	killed := false
	for i := 0; i < 50; i++ {
		err := process.Signal(syscall.Signal(0))
		if err != nil {
			killed = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !killed {
		// 超时则发送 SIGKILL 强杀
		process.Signal(syscall.SIGKILL)
		time.Sleep(200 * time.Millisecond)
	}

	os.Remove(corePidFile)
	_ = os.Remove(coreSocket)
	return nil
}

// ---------- HTTP Handlers ----------

// handleCoreStatus 返回内核运行状态
func handleCoreStatus(w http.ResponseWriter, r *http.Request) {
	running := isCoreRunning()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"running": running})
}

// handleCoreStart 启动内核
func handleCoreStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := startCore(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "内核已启动"})
}

// handleCoreStop 停止内核
func handleCoreStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := stopCore(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "内核已停止"})
}

// handleCoreRestart 热重启内核（通过重载配置文件）
func handleCoreRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	if err := reloadCore(); err != nil {
		if coreLogger != nil {
			coreLogger.Printf("[RESTART][ERROR] 内核热重启失败: %v\n", err)
		}
		writeJSONError(w, http.StatusInternalServerError, "内核热重启失败: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "内核已热重启（重载配置）"})
}
// downloadWithTempCore 使用临时内核下载单个订阅的节点文件，并返回元数据（updatedAt 和 subscriptionInfo）
func downloadWithTempCore(sub Subscription, index int, targetFile string) (updatedAt string, subInfo map[string]interface{}, err error) {
    var lastErr error
    var port int
    var listener net.Listener

    tmpDir := filepath.Dir(corePidFile)
    tmpConfig := filepath.Join(tmpDir, fmt.Sprintf("tmp%d.yaml", index))
    tmpPidFile := filepath.Join(tmpDir, fmt.Sprintf("tmp%d.pid", index))

    for attempt := 0; attempt < 3; attempt++ {
        listener, err = net.Listen("tcp", "127.0.0.1:0")
        if err != nil {
            lastErr = fmt.Errorf("分配端口失败: %w", err)
            time.Sleep(100 * time.Millisecond)
            continue
        }
        port = listener.Addr().(*net.TCPAddr).Port
        listener.Close()

        content := fmt.Sprintf(`mixed-port: 0
log-level: silent
external-controller: '127.0.0.1:%d'
proxy-providers:
  %s:
    type: http
    url: "%s"
    path: "%s"
`, port, sub.Name, sub.URL, targetFile)

        if err = os.WriteFile(tmpConfig, []byte(content), 0644); err != nil {
            lastErr = fmt.Errorf("写入临时配置失败: %w", err)
            continue
        }

        cmd := exec.Command(coreBin, "-f", tmpConfig, "-d", coreWorkDir)
        var stderr bytes.Buffer
        cmd.Stderr = &stderr

        if err = cmd.Start(); err != nil {
            lastErr = fmt.Errorf("启动临时内核失败: %w, stderr: %s", err, stderr.String())
            os.Remove(tmpConfig)
            continue
        }

        // 保存 PID
        _ = os.WriteFile(tmpPidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644)

        // 监测是否因端口冲突在启动瞬间退出
        time.Sleep(100 * time.Millisecond)
        if cmd.Process != nil && cmd.Process.Signal(syscall.Signal(0)) != nil {
            lastErr = fmt.Errorf("临时内核启动后立即退出，可能端口冲突，stderr: %s", stderr.String())
            os.Remove(tmpConfig)
            os.Remove(tmpPidFile)
            continue
        }

        // 成功运行，开始处理下载
        updatedAt, subInfo, err = runDownloadProcess(cmd, targetFile, port, sub.Name, tmpConfig, tmpPidFile)
        if err == nil {
            return updatedAt, subInfo, nil
        }
        lastErr = err
    }

    return "", nil, fmt.Errorf("下载订阅失败，已尝试3次，最后错误: %w", lastErr)
}

func runDownloadProcess(cmd *exec.Cmd, targetFile string, port int, subName string, tmpConfig string, tmpPidFile string) (updatedAt string, subInfo map[string]interface{}, err error) {
    defer func() {
        os.Remove(tmpConfig)
        os.Remove(tmpPidFile)
        if cmd.Process != nil {
            _ = cmd.Process.Signal(syscall.SIGTERM)
            done := make(chan error, 1)
            go func() {
                done <- cmd.Wait()
            }()
            select {
            case <-done:
            case <-time.After(1 * time.Second):
                _ = cmd.Process.Kill()
                <-done
            }
        }
    }()

    // 轮询等待目标文件生成（60秒），同时检查进程存活
    timeout := time.After(60 * time.Second)
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()
    
    fileGenerated := false
    for !fileGenerated {
        select {
        case <-timeout:
            return "", nil, fmt.Errorf("下载超时（60秒），文件未生成")
        case <-ticker.C:
            // 检查进程是否存活
            if cmd.Process == nil || cmd.Process.Signal(syscall.Signal(0)) != nil {
                return "", nil, fmt.Errorf("临时内核进程意外退出")
            }
            info, err := os.Stat(targetFile)
            if err == nil && info.Size() > 0 {
                log.Printf("[DOWNLOAD] 文件 %s 已生成，大小 %d 字节", targetFile, info.Size())
                fileGenerated = true
            }
        }
    }

    client := &http.Client{Timeout: 5 * time.Second}
    urlPath := fmt.Sprintf("http://127.0.0.1:%d/providers/proxies/%s", port, url.QueryEscape(subName))
    var resp *http.Response
    var lastErr error
    
    for attempt := 0; attempt < 30; attempt++ {
        resp, lastErr = client.Get(urlPath)
        if lastErr == nil {
            break
        }
        time.Sleep(500 * time.Millisecond)
    }
    if lastErr != nil {
        return "", nil, fmt.Errorf("获取订阅元数据失败: %w", lastErr)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        bodyBytes, _ := io.ReadAll(resp.Body)
        return "", nil, fmt.Errorf("获取元数据返回非200状态: %d, body: %s", resp.StatusCode, string(bodyBytes))
    }

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", nil, fmt.Errorf("读取响应失败: %w", err)
    }

    var data map[string]interface{}
    if err := json.Unmarshal(body, &data); err != nil {
        return "", nil, fmt.Errorf("解析JSON失败: %w", err)
    }

    updatedAtVal, _ := data["updatedAt"].(string)
    subInfoVal, _ := data["subscriptionInfo"].(map[string]interface{})

    log.Printf("[DOWNLOAD] 成功获取元数据: updatedAt=%s, subInfo=%v", updatedAtVal, subInfoVal)
    return updatedAtVal, subInfoVal, nil
}
