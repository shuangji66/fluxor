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
    "path/filepath"
    "os"
    "os/exec"
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
            return net.Dial("unix", coreSocket)
        },
    },
    Timeout: 3 * time.Second,
}

// coreRequest 向内核发送 HTTP 请求并返回响应
func coreRequest(method, path string, body io.Reader) (*http.Response, error) {
    req, err := http.NewRequestWithContext(context.Background(), method, "http://unix"+path, body)
    if err != nil {
        return nil, err
    }
    req.Header.Set("Content-Type", "application/json")
    return coreHTTPClient.Do(req)
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

    // 确保配置文件存在，若不存在则生成基本配置
    if _, err := os.Stat(configTarget); os.IsNotExist(err) {
        basic := `mixed-port: 7790
allow-lan: true
mode: rule
log-level: silent
external-controller-unix: '/var/apps/Fluxor/target/core.sock'
external-controller: '0.0.0.0:9090'
`
        os.MkdirAll(filepath.Dir(configTarget), 0755)
        if err := os.WriteFile(configTarget, []byte(basic), 0644); err != nil {
            if coreLogger != nil {
                coreLogger.Println("[START][ERROR] 创建基本配置文件失败:", err)
            }
            return fmt.Errorf("创建基本配置文件失败: %v", err)
        }
        if coreLogger != nil {
            coreLogger.Println("[START] 已生成基本配置文件")
        }
    }

    if coreLogger != nil {
        coreLogger.Println("[START] 尝试启动内核 (工作目录:", coreWorkDir, ")")
    }

    cmd := exec.Command(coreBin, "-d", coreWorkDir)
    var stderr bytes.Buffer
    cmd.Stderr = &stderr

    if err := cmd.Start(); err != nil {
        errMsg := fmt.Sprintf("启动内核失败: %v, stderr: %s", err, stderr.String())
        if coreLogger != nil {
            coreLogger.Println("[START][ERROR]", errMsg)
        }
        return fmt.Errorf(errMsg)
    }

    // 等待 1 秒，检查进程是否存活
    time.Sleep(1 * time.Second)
    err := cmd.Process.Signal(syscall.Signal(0))
    if err != nil {
        stderrContent := stderr.String()
        waitErr := cmd.Wait() // 回收子进程，防止僵尸
        if waitErr != nil {
            stderrContent += " (Wait err: " + waitErr.Error() + ")"
        }
        if stderrContent == "" {
            stderrContent = "进程已退出，无 stderr 输出"
        }
        errMsg := fmt.Sprintf("内核启动后立即退出: %s", stderrContent)
        if coreLogger != nil {
            coreLogger.Println("[START][ERROR]", errMsg)
        }
        return fmt.Errorf(errMsg)
    }

    pid := cmd.Process.Pid
    os.MkdirAll(filepath.Dir(corePidFile), 0755)
    if err := os.WriteFile(corePidFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
        cmd.Process.Kill()
        if coreLogger != nil {
            coreLogger.Println("[START][ERROR] 写入 PID 文件失败:", err)
        }
        cmd.Wait()
        return fmt.Errorf("写入 PID 文件失败: %v", err)
    }

    // 后台等待进程退出，防止僵尸进程
    go func() {
        err := cmd.Wait()
        if coreLogger != nil {
            if err != nil {
                coreLogger.Printf("[START] 内核进程 (PID: %d) 退出: %v\n", pid, err)
            } else {
                coreLogger.Printf("[START] 内核进程 (PID: %d) 正常退出\n", pid)
            }
        }
        os.Remove(corePidFile)
    }()

    if coreLogger != nil {
        coreLogger.Printf("[START] 内核已启动 (PID: %d)\n", pid)
    }
    return nil
}

// stopCore 停止内核进程
func stopCore() error {
    if !isCoreRunning() {
        msg := "内核未运行，停止操作被忽略"
        if coreLogger != nil {
            coreLogger.Println("[STOP] " + msg)
        }
        return fmt.Errorf(msg)
    }
    if coreLogger != nil {
        coreLogger.Println("[STOP] 尝试停止内核...")
    }
    data, _ := os.ReadFile(corePidFile)
    pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
    process, err := os.FindProcess(pid)
    if err != nil {
        errMsg := fmt.Sprintf("查找进程失败: %v", err)
        if coreLogger != nil {
            coreLogger.Println("[STOP][ERROR] " + errMsg)
        }
        return fmt.Errorf(errMsg)
    }
    if err := process.Signal(syscall.SIGTERM); err != nil {
        errMsg := fmt.Sprintf("停止进程失败: %v", err)
        if coreLogger != nil {
            coreLogger.Println("[STOP][ERROR] " + errMsg)
        }
        return fmt.Errorf(errMsg)
    }
    process.Wait()
    os.Remove(corePidFile)
    if coreLogger != nil {
        coreLogger.Printf("[STOP] 内核已停止 (PID: %d)\n", pid)
    }
    return nil
}

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
    if coreLogger != nil {
        coreLogger.Println("[RESTART] 通过重载配置文件热重启内核...")
    }
    if err := reloadCore(); err != nil {
        errMsg := "内核热重启失败: " + err.Error()
        if coreLogger != nil {
            coreLogger.Println("[RESTART][ERROR] " + errMsg)
        }
        writeJSONError(w, http.StatusInternalServerError, errMsg)
        return
    }
    if coreLogger != nil {
        coreLogger.Println("[RESTART] 内核热重启成功")
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "内核已热重启（重载配置）"})
}