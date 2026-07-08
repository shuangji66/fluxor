package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var (
	tproxyEnableState        bool
	tproxyMu                 sync.RWMutex
	exceptionsMu             sync.RWMutex
	tproxyProxyLocal         bool
	tproxyDstExceptionsCache []string
    tproxySrcExceptionsCache []string
)

// loadTproxyDstExceptions 加载目的例外
func loadTproxyDstExceptions() []string {
    exceptionsMu.Lock()
    defer exceptionsMu.Unlock()
    data, err := os.ReadFile(fluxorConfigFile)
    if err != nil {
        return []string{"# 公共 DNS 服务器","223.5.5.5 #注释可单独一行也可写在规则后", "1.12.12.12", "# stun服务器","141.101.90.1"} // 默认值
    }
    var raw map[string]json.RawMessage
    if err := json.Unmarshal(data, &raw); err != nil {
        return []string{"# 公共 DNS 服务器","223.5.5.5 #注释可单独一行也可写在规则后", "1.12.12.12", "# stun服务器","141.101.90.1"}
    }
    // 尝试读取新字段 tproxy_dst_exceptions
    if dstRaw, ok := raw["tproxy_dst_exceptions"]; ok {
        var dst []string
        if err := json.Unmarshal(dstRaw, &dst); err == nil {
            tproxyDstExceptionsCache = dst
            return dst
        }
    }
    // 回退到旧字段 tproxy_exceptions
    if oldRaw, ok := raw["tproxy_exceptions"]; ok {
        var old []string
        if err := json.Unmarshal(oldRaw, &old); err == nil {
            // 迁移到新字段
            tproxyDstExceptionsCache = old
            saveTproxyDstExceptionsLocked(old)
            // 删除旧字段（可选）
            delete(raw, "tproxy_exceptions")
            return old
        }
    }
    // 默认值
    defaultDst := []string{"# 公共 DNS 服务器","223.5.5.5 #注释可单独一行也可写在规则后", "1.12.12.12", "# stun服务器","141.101.90.1"}
    tproxyDstExceptionsCache = defaultDst
    saveTproxyDstExceptionsLocked(defaultDst)
    return defaultDst
}

// saveTproxyDstExceptionsLocked 假定已持有锁
func saveTproxyDstExceptionsLocked(dst []string) error {
    data, err := os.ReadFile(fluxorConfigFile)
    var full map[string]interface{}
    if err == nil && len(data) > 0 {
        json.Unmarshal(data, &full)
    } else {
        full = make(map[string]interface{})
    }
    full["tproxy_dst_exceptions"] = dst
    // 删除旧字段（可选）
    delete(full, "tproxy_exceptions")
    newData, _ := json.MarshalIndent(full, "", "  ")
    return os.WriteFile(fluxorConfigFile, newData, 0644)
}

// SaveTproxyDstExceptions 供外部调用（加锁）
func SaveTproxyDstExceptions(dst []string) error {
    exceptionsMu.Lock()
    defer exceptionsMu.Unlock()
    tproxyDstExceptionsCache = dst
    return saveTproxyDstExceptionsLocked(dst)
}

// loadTproxyDstExceptions 加载源例外
func loadTproxySrcExceptions() []string {
    exceptionsMu.Lock()
    defer exceptionsMu.Unlock()
    data, err := os.ReadFile(fluxorConfigFile)
    if err != nil {
        // 文件不存在，创建默认
        defaultSrc := []string{"# Docker 默认网段","172.17.0.0/16"}
        tproxySrcExceptionsCache = defaultSrc
        saveTproxySrcExceptionsLocked(defaultSrc)
        return defaultSrc
    }
    var raw map[string]json.RawMessage
    if err := json.Unmarshal(data, &raw); err != nil {
        defaultSrc := []string{"# Docker 默认网段","172.17.0.0/16"}
        tproxySrcExceptionsCache = defaultSrc
        saveTproxySrcExceptionsLocked(defaultSrc)
        return defaultSrc
    }
    if srcRaw, ok := raw["tproxy_src_exceptions"]; ok {
        var src []string
        if err := json.Unmarshal(srcRaw, &src); err == nil {
            tproxySrcExceptionsCache = src
            return src
        }
    }
    // 字段不存在，初始化默认
    defaultSrc := []string{"# Docker 默认网段","172.17.0.0/16"}
    tproxySrcExceptionsCache = defaultSrc
    saveTproxySrcExceptionsLocked(defaultSrc)
    return defaultSrc
}

func saveTproxySrcExceptionsLocked(src []string) error {
    data, _ := os.ReadFile(fluxorConfigFile)
    var full map[string]interface{}
    if len(data) > 0 { json.Unmarshal(data, &full) } else { full = make(map[string]interface{}) }
    full["tproxy_src_exceptions"] = src
    newData, _ := json.MarshalIndent(full, "", "  ")
    return os.WriteFile(fluxorConfigFile, newData, 0644)
}

func SaveTproxySrcExceptions(src []string) error {
    exceptionsMu.Lock()
    defer exceptionsMu.Unlock()
    tproxySrcExceptionsCache = src
    return saveTproxySrcExceptionsLocked(src)
}

// ---------- TProxy 规则管理 ----------

// parseTproxyException 解析单条规则，返回 (ruleType, value, proto, port)
// 支持：
//   - IP/CIDR: 192.168.1.0/24
//   - 单个IP: 192.168.1.1
//   - 端口(所有协议): 53
//   - 协议:端口: tcp:80 或 udp:443
func parseTproxyException(rule string) (typ string, ipNet *net.IPNet, proto string, port int, err error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return "", nil, "", 0, fmt.Errorf("空规则")
	}
	// 尝试 IP/CIDR
	if _, ipNet, err := net.ParseCIDR(rule); err == nil {
		return "ip", ipNet, "", 0, nil
	}
	// 尝试单个 IP
	if ip := net.ParseIP(rule); ip != nil {
		_, ipNet, _ := net.ParseCIDR(rule + "/32")
		return "ip", ipNet, "", 0, nil
	}
	// 尝试 协议:端口
	if strings.Contains(rule, ":") {
		parts := strings.SplitN(rule, ":", 2)
		proto := strings.ToLower(parts[0])
		if proto != "tcp" && proto != "udp" {
			return "", nil, "", 0, fmt.Errorf("协议仅支持 tcp/udp")
		}
		p, err := strconv.Atoi(parts[1])
		if err != nil || p < 1 || p > 65535 {
			return "", nil, "", 0, fmt.Errorf("端口无效")
		}
		return "port", nil, proto, p, nil
	}
	// 尝试纯数字端口
	if p, err := strconv.Atoi(rule); err == nil && p > 0 && p <= 65535 {
		return "port", nil, "", p, nil
	}
	return "", nil, "", 0, fmt.Errorf("不支持的格式")
}

// loadTproxyProxyLocal 从 fluxor.json 读取 tproxy_proxy_local 字段，默认 true
func loadTproxyProxyLocal() bool {
	exceptionsMu.Lock()
	defer exceptionsMu.Unlock()

	data, err := os.ReadFile(fluxorConfigFile)
	if err != nil {
		// 文件不存在，默认开启并保存
		tproxyProxyLocal = true
		saveTproxyProxyLocalLocked(true)
		return true
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		tproxyProxyLocal = true
		saveTproxyProxyLocalLocked(true)
		return true
	}
	var enabled bool
	if val, ok := raw["tproxy_proxy_local"]; ok {
		if err := json.Unmarshal(val, &enabled); err != nil {
			tproxyProxyLocal = true
			saveTproxyProxyLocalLocked(true)
			return true
		}
		tproxyProxyLocal = enabled
		return enabled
	}
	// 字段不存在，默认 true，写入文件
	tproxyProxyLocal = true
	saveTproxyProxyLocalLocked(true)
	return true
}

// saveTproxyProxyLocalLocked 假定已持有 exceptionsMu 锁
func saveTproxyProxyLocalLocked(enabled bool) error {
	// 读取完整配置
	data, err := os.ReadFile(fluxorConfigFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var full map[string]interface{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &full); err != nil {
			return err
		}
	} else {
		full = make(map[string]interface{})
	}
	full["tproxy_proxy_local"] = enabled
	dir := filepath.Dir(fluxorConfigFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	newData, err := json.MarshalIndent(full, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fluxorConfigFile, newData, 0644)
}

// SaveTproxyProxyLocal 外部调用，加锁并保存
func SaveTproxyProxyLocal(enabled bool) error {
	exceptionsMu.Lock()
	defer exceptionsMu.Unlock()
	tproxyProxyLocal = enabled
	return saveTproxyProxyLocalLocked(enabled)
}

// enableTProxyRules 配置策略路由与 nftables 规则（增强版）
// 支持目的例外、源例外以及本机出站流量控制
func enableTProxyRules(port int) error {
	if port <= 0 {
		return nil
	}

	// 辅助函数：执行命令并忽略错误（保留原行为）
	runCmd := func(name string, args ...string) {
		cmd := exec.Command(name, args...)
		if err := cmd.Run(); err != nil {
			log.Printf("[TProxy] 命令执行失败: %s %v, 错误: %v", name, args, err)
		}
	}

	// 1. 策略路由
	runCmd("ip", "rule", "add", "fwmark", "1", "table", "100")
	runCmd("ip", "route", "add", "local", "0.0.0.0/0", "dev", "lo", "table", "100")

	// 2. 关闭反向路径过滤
	runCmd("sysctl", "-w", "net.ipv4.conf.all.rp_filter=0")
	runCmd("sysctl", "-w", "net.ipv4.conf.default.rp_filter=0")
	runCmd("sysctl", "-w", "net.ipv4.conf.lo.rp_filter=0")

	// 3. 创建 nftables 表
	runCmd("nft", "add", "table", "ip", "fluxor_tproxy")

	// 4. 绕过私有网段
	runCmd("nft", "add", "set", "ip", "fluxor_tproxy", "private_ips", "{ type ipv4_addr; flags interval; }")
	bypassIPs := []string{"10.0.0.0/8", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.168.0.0/16", "224.0.0.0/4", "240.0.0.0/4"}
	for _, ip := range bypassIPs {
		runCmd("nft", "add", "element", "ip", "fluxor_tproxy", "private_ips", "{", ip, "}")
	}

	// 5. 创建链
	runCmd("nft", "add", "chain", "ip", "fluxor_tproxy", "prerouting", "{ type filter hook prerouting priority mangle; policy accept; }")
	runCmd("nft", "add", "chain", "ip", "fluxor_tproxy", "output", "{ type route hook output priority mangle; policy accept; }")
	runCmd("nft", "add", "chain", "ip", "fluxor_tproxy", "dstnat", "{ type nat hook prerouting priority -100; policy accept; }")
	runCmd("nft", "add", "chain", "ip", "fluxor_tproxy", "nat_output", "{ type nat hook output priority -100; policy accept; }")

	// 6. 私有IP绕过
	runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "prerouting", "fib", "daddr", "type", "local", "return")
	runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "prerouting", "ip", "daddr", "@private_ips", "return")
	runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "output", "ip", "daddr", "@private_ips", "return")

	// 7. 加载例外列表
	dstExceptions := loadTproxyDstExceptions()
	srcExceptions := loadTproxySrcExceptions()

	// 7a. 目的例外
	for _, rule := range dstExceptions {
		rule = stripComment(rule)
  	    if rule == "" {
            continue
        }
        typ, ipNet, proto, portVal, err := parseTproxyException(rule)
        if err != nil {
            log.Printf("[TProxy] 跳过无效目的例外规则 %q: %v", rule, err)
            continue
        }

		if typ == "ip" {
			cidr := ipNet.String()
			// TProxy 劫持
			runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "prerouting", "ip", "daddr", cidr, "return")
			runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "output", "ip", "daddr", cidr, "return")
			// DNS 重定向
			runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "dstnat", "ip", "daddr", cidr, "return")
			if tproxyProxyLocal {
				runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "nat_output", "ip", "daddr", cidr, "return")
			}
		} else if typ == "port" {
			// 将集合作为一个完整的字符串参数
			protoExpr := "{tcp, udp}"
			if proto != "" {
				protoExpr = "{" + proto + "}"
			}
			// TProxy 劫持
			runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "prerouting", "meta", "l4proto", protoExpr, "th", "dport", strconv.Itoa(portVal), "return")
			runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "output", "meta", "l4proto", protoExpr, "th", "dport", strconv.Itoa(portVal), "return")
			// DNS 重定向
			runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "dstnat", "meta", "l4proto", protoExpr, "th", "dport", strconv.Itoa(portVal), "return")
			if tproxyProxyLocal {
				runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "nat_output", "meta", "l4proto", protoExpr, "th", "dport", strconv.Itoa(portVal), "return")
			}
		}
	}

	// 7b. 源例外
	for _, rule := range srcExceptions {
		rule = stripComment(rule)
        if rule == "" {
            continue
        }
		if _, ipNet, err := net.ParseCIDR(rule); err == nil {
			cidr := ipNet.String()
			runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "prerouting", "ip", "saddr", cidr, "return")
			runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "dstnat", "ip", "saddr", cidr, "return")
		} else if ip := net.ParseIP(rule); ip != nil {
			cidr := ip.String() + "/32"
			runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "prerouting", "ip", "saddr", cidr, "return")
			runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "dstnat", "ip", "saddr", cidr, "return")
		} else {
			log.Printf("[TProxy] 源例外仅支持 IP/CIDR，忽略无效规则: %s", rule)
		}
	}

	// 8. TProxy 劫持规则
	// 使用集合字符串 "{tcp,udp}"
	runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "prerouting", "meta", "l4proto", "{tcp,udp}", "tproxy", "to", fmt.Sprintf(":%d", port), "meta", "mark", "set", "1", "accept")
	runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "output", "meta", "mark", "0xff", "return")
	runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "output", "socket", "mark", "0xff", "return")
	if tproxyProxyLocal {
		runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "output", "meta", "l4proto", "{tcp,udp}", "meta", "mark", "set", "1", "accept")
	}

	// 9. DNS 重定向
	runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "dstnat", "udp", "dport", "53", "redirect", "to", ":1053")
	runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "dstnat", "tcp", "dport", "53", "redirect", "to", ":1053")
	if tproxyProxyLocal {
		runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "nat_output", "meta", "mark", "0xff", "return")
		runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "nat_output", "socket", "mark", "0xff", "return")
		runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "nat_output", "udp", "dport", "53", "redirect", "to", ":1053")
		runCmd("nft", "add", "rule", "ip", "fluxor_tproxy", "nat_output", "tcp", "dport", "53", "redirect", "to", ":1053")
	}

	log.Printf("[TProxy] 规则应用成功（含目的/源例外及本机代理开关）")
	return nil
}

// disableTProxyRules 清理规则（仅当规则存在时才执行删除）
func disableTProxyRules() {
    // 1. 检查 nftables 表 fluxor_tproxy 是否存在
    checkCmd := exec.Command("nft", "list", "table", "ip", "fluxor_tproxy")
    if err := checkCmd.Run(); err != nil {
        // 表不存在，说明规则已清除，直接返回
        return
    }

    // 表存在，执行清理
    exec.Command("nft", "delete", "table", "ip", "fluxor_tproxy").Run()
    exec.Command("ip", "rule", "del", "fwmark", "1", "table", "100").Run()
    exec.Command("ip", "route", "del", "local", "0.0.0.0/0", "dev", "lo", "table", "100").Run()
    log.Printf("[TProxy] nftables防火墙规则清理完成。")
}

// GetTproxyState 导出状态
func GetTproxyState() bool {
	tproxyMu.RLock()
	defer tproxyMu.RUnlock()
	return tproxyEnableState
}

// ---------- HTTP Handlers ----------

// handleTproxyState 处理开关（保持不变，但 POST 时重新应用规则）
func handleTproxyState(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tproxyMu.RLock()
		enabled := tproxyEnableState
		tproxyMu.RUnlock()
		respondJSON(w, http.StatusOK, map[string]bool{"enabled": enabled})
	case http.MethodPost:
		var req struct{ Enable bool }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "无效的请求格式")
			return
		}
		tproxyMu.Lock()
		tproxyEnableState = req.Enable
		tproxyMu.Unlock()

		if req.Enable {
			subscribeMu.RLock()
			port := subscribeConfig.TproxyPort
			subscribeMu.RUnlock()
			if port > 0 {
				disableTProxyRules() // 先清理
				if err := enableTProxyRules(port); err != nil {
					log.Printf("[TProxy] 添加规则失败: %v", err)
				}
			} else {
				log.Printf("[TProxy] 开关开启但端口为0，无法添加规则")
			}
		} else {
			disableTProxyRules()
		}
		respondJSON(w, http.StatusOK, map[string]bool{"enabled": req.Enable})
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// handleTproxyExceptions 处理例外列表的获取和更新
func handleTproxyExceptions(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        exceptionsMu.RLock()
        dst := tproxyDstExceptionsCache
        src := tproxySrcExceptionsCache
        exceptionsMu.RUnlock()
        respondJSON(w, http.StatusOK, map[string]interface{}{
            "dst": dst,
            "src": src,
        })
    case http.MethodPost:
        var req struct {
            Dst []string `json:"dst"`
            Src []string `json:"src"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            writeJSONError(w, http.StatusBadRequest, "无效请求格式")
            return
        }
        // 分别保存
        if err := SaveTproxyDstExceptions(req.Dst); err != nil {
            writeJSONError(w, http.StatusInternalServerError, "保存目的例外失败")
            return
        }
        if err := SaveTproxySrcExceptions(req.Src); err != nil {
            writeJSONError(w, http.StatusInternalServerError, "保存源例外失败")
            return
        }
        // 如果 TProxy 启用则重载
        if tproxyEnableState {
            subscribeMu.RLock()
            port := subscribeConfig.TproxyPort
            subscribeMu.RUnlock()
            if port > 0 {
                disableTProxyRules()
                enableTProxyRules(port)
            }
        }
        respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
    default:
        http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
    }
}

// handleTproxyProxyLocal 处理本机代理开关的获取和设置
func handleTproxyProxyLocal(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		exceptionsMu.RLock()
		enabled := tproxyProxyLocal
		exceptionsMu.RUnlock()
		respondJSON(w, http.StatusOK, map[string]bool{"enabled": enabled})
	case http.MethodPost:
		var req struct{ Enabled bool }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "无效请求格式")
			return
		}
		if err := SaveTproxyProxyLocal(req.Enabled); err != nil {
			log.Printf("[TProxy] 保存本机代理开关失败: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "保存失败")
			return
		}
		// 如果 TProxy 当前启用，立即重新应用规则
		if tproxyEnableState {
			subscribeMu.RLock()
			port := subscribeConfig.TproxyPort
			subscribeMu.RUnlock()
			if port > 0 {
				disableTProxyRules()
				if err := enableTProxyRules(port); err != nil {
					log.Printf("[TProxy] 重新应用规则失败: %v", err)
				}
			}
		}
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// stripComment 去除行尾 # 注释，并 trim 空格，返回纯净的规则部分
func stripComment(line string) string {
	line = strings.TrimSpace(line)
	if idx := strings.Index(line, "#"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	return line
}