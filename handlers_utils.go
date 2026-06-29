package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"strconv"
	"sync"
	"math"
	"log"
)

var geoCache = struct {
    sync.RWMutex
    data map[string]geoInfo
}{data: make(map[string]geoInfo)}

type geoInfo struct {
    Country string
    Region  string
    Isp     string
    Expire  time.Time
}

// validateTCPAddr 校验 TCP 地址格式（IP:Port 或 :Port）
func validateTCPAddr(addr string) error {
    if addr == "" {
        return nil
    }
    host, port, err := net.SplitHostPort(addr)
    if err != nil {
        return fmt.Errorf("地址格式错误: %w", err)
    }
    if host != "" {
        if ip := net.ParseIP(host); ip == nil {
            return fmt.Errorf("无效的 IP 地址: %s", host)
        }
    }
    p, err := strconv.Atoi(port)
    if err != nil || p < 1 || p > 65535 {
        return fmt.Errorf("端口无效: %s", port)
    }
    return nil
}

// 地址格式验证正则（仅用于后端地址校验）
var backendURLRegex = regexp.MustCompile(`^https?://(([0-9]{1,3}\.){3}[0-9]{1,3}|([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,})(:([1-9][0-9]{0,3}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5]))?$`)

// writeJSONError 统一返回 JSON 格式的错误响应
func writeJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": message})
}

// handleInterfaces 返回系统所有的物理网络接口名称（GET /interfaces）
func handleInterfaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "获取网络接口失败: "+err.Error())
		return
	}

	var names []string
	for _, iface := range ifaces {
		// 过滤回环接口和未启用的接口
		if (iface.Flags & net.FlagLoopback) != 0 {
			continue
		}
		if (iface.Flags & net.FlagUp) == 0 {
			continue
		}
		names = append(names, iface.Name)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(names)
}

// handleLocalIPv4 获取本机 IPv4 及地理信息
func handleLocalIPv4(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	urls := []string{
		"https://ipv4.ddnspod.com/",
		"https://myip.ipip.net",
		"https://ip.3322.net",
	}
	ip, err := fetchPublicIPWithFallback(urls, "")
	if err != nil {
		// 回退到网卡获取的本机局域网 IP
		if localIP, localErr := getLocalIPFromInterfaces(false); localErr == nil {
			ip = localIP
		} else {
			writeJSONError(w, http.StatusServiceUnavailable, "获取本机 IPv4 失败: "+err.Error())
			return
		}
	}
	// 获取地理信息
	country, region, isp := fetchGeoInfo(ip)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"ip":      ip,
		"country": country,
		"region":  region,
		"isp":     isp,
	})
}

// handleLocalIPv6 获取本机 IPv6
func handleLocalIPv6(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	urls := []string{
		"https://ipv6.ddnspod.com",
		"https://v6.myip.la",
		"https://speed.neu6.edu.cn/getIP.php",
		"https://api6.ipify.org?format=json",
	}
	ip, err := fetchPublicIPWithFallback(urls, "")
	if err != nil {
		// 回退到网卡获取的本机全球单播 IPv6
		if localIP, localErr := getLocalIPFromInterfaces(true); localErr == nil {
			ip = localIP
		} else {
			writeJSONError(w, http.StatusServiceUnavailable, "获取本机 IPv6 失败: "+err.Error())
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ip": ip})
}

// handleProxyIPv4 获取代理 IPv4 及地理信息
func handleProxyIPv4(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	proxyPort := getProxyPortFromConfig()
	if proxyPort == 0 {
		writeJSONError(w, http.StatusServiceUnavailable, "无可用代理端口")
		return
	}
	proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", proxyPort)
	urls := []string{
		"https://api.ipify.org?format=json",
		"https://ipv4.icanhazip.com",
		"https://v4.ident.me",
	}
	ip, err := fetchPublicIPWithFallback(urls, proxyAddr)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "获取代理 IPv4 失败: "+err.Error())
		return
	}
	// 获取地理信息
	country, region, isp := fetchGeoInfo(ip)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"ip":      ip,
		"country": country,
		"region":  region,
		"isp":     isp,
	})
}

// handleProxyIPv6 获取代理 IPv6
func handleProxyIPv6(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	proxyPort := getProxyPortFromConfig()
	if proxyPort == 0 {
		writeJSONError(w, http.StatusServiceUnavailable, "无可用代理端口")
		return
	}
	proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", proxyPort)
	urls := []string{
		"https://api6.ipify.org?format=json",
		"https://ipv6.icanhazip.com",
		"https://v6.ident.me",
	}
	ip, err := fetchPublicIPWithFallback(urls, proxyAddr)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "获取代理 IPv6 失败: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ip": ip})
}

// getProxyPortFromConfig 从内核配置中获取代理端口
func getProxyPortFromConfig() int {
	resp, err := coreRequest("GET", "/configs", nil)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0
	}
	var cfg map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return 0
	}
	// 优先 mixed-port
	if p, ok := cfg["mixed-port"]; ok {
		if port, ok := p.(float64); ok && port > 0 {
			return int(port)
		}
	}
	if p, ok := cfg["port"]; ok {
		if port, ok := p.(float64); ok && port > 0 {
			return int(port)
		}
	}
	if p, ok := cfg["socks-port"]; ok {
		if port, ok := p.(float64); ok && port > 0 {
			return int(port)
		}
	}
	return 0
}

// fetchGeoInfo 查询 IP 地理信息，返回 country, region, isp（带缓存和重试）
func fetchGeoInfo(ip string) (string, string, string) {
    if ip == "" {
        return "", "", ""
    }

    // 1. 检查缓存
    geoCache.RLock()
    if cached, ok := geoCache.data[ip]; ok && time.Now().Before(cached.Expire) {
        geoCache.RUnlock()
        return cached.Country, cached.Region, cached.Isp
    }
    geoCache.RUnlock()

    client := &http.Client{Timeout: 5 * time.Second}
    // 使用 ip-api.com 作为主 API（免费版，无 token，但需遵守使用条款）
    // 返回 JSON: {"country":"...", "regionName":"...", "isp":"..."}
    // 注意：ip-api.com 对非商用有限制，但通常可用
    urls := []string{
        "http://ip-api.com/json/" + ip + "?fields=country,regionName,isp",
        "https://api.ip.sb/geoip/" + ip,
    }

    for attempt := 0; attempt < 3; attempt++ {
        if attempt > 0 {
            time.Sleep(300 * time.Millisecond)
        }
        for _, url := range urls {
            req, err := http.NewRequest("GET", url, nil)
            if err != nil {
                continue
            }
            req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
            resp, err := client.Do(req)
            if err != nil {
                continue
            }
            if resp.StatusCode != http.StatusOK {
                resp.Body.Close()
                continue
            }
            var country, region, isp string
            // 尝试解析 JSON
            var data struct {
                Country    string `json:"country"`
                Region     string `json:"region"`
                RegionName string `json:"regionName"`
                Isp        string `json:"isp"`
            }
            bodyBytes, _ := io.ReadAll(resp.Body)
            resp.Body.Close()
            if err := json.Unmarshal(bodyBytes, &data); err == nil {
                country = data.Country
                if data.RegionName != "" {
                    region = data.RegionName
                } else {
                    region = data.Region
                }
                isp = data.Isp
            } else {
                // 尝试解析 api.ip.sb 格式
                var data2 struct {
                    Country string `json:"country"`
                    Region  string `json:"region"`
                    Isp     string `json:"isp"`
                }
                if err := json.Unmarshal(bodyBytes, &data2); err == nil {
                    country = data2.Country
                    region = data2.Region
                    isp = data2.Isp
                }
            }
            if country != "" || region != "" || isp != "" {
                // 缓存结果，有效期 10 分钟
                geoCache.Lock()
                geoCache.data[ip] = geoInfo{
                    Country: country,
                    Region:  region,
                    Isp:     isp,
                    Expire:  time.Now().Add(10 * time.Minute),
                }
                geoCache.Unlock()
                return country, region, isp
            }
        }
    }
    return "", "", ""
}
// fetchPublicIP 支持通过代理获取 IP，兼容 JSON 与纯文本，带正则表达式提取和校验
func fetchPublicIP(apiURL, proxyAddr string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	if proxyAddr != "" {
		proxyURL, err := url.Parse(proxyAddr)
		if err == nil {
			client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		}
	}
	resp, err := client.Get(apiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	bodyStr := strings.TrimSpace(string(bodyBytes))

	// 1. 尝试解析为 JSON
	var data struct {
		IP    string `json:"ip"`
		Query string `json:"query"`
	}
	if err := json.Unmarshal(bodyBytes, &data); err == nil {
		if data.IP != "" {
			return data.IP, nil
		}
		if data.Query != "" {
			return data.Query, nil
		}
	}

	// 2. 正则从响应内容中搜索首个合法的 IPv4/IPv6 地址并验证
	ipRegex := regexp.MustCompile(`((?:[0-9]{1,3}\.){3}[0-9]{1,3})|((?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4})`)
	matches := ipRegex.FindAllString(bodyStr, -1)
	for _, match := range matches {
		if net.ParseIP(match) != nil {
			return match, nil
		}
	}

	// 3. 直接验证去除空白后的全文
	if net.ParseIP(bodyStr) != nil {
		return bodyStr, nil
	}

	return "", fmt.Errorf("no valid IP address found in response: %s", bodyStr)
}

// fetchPublicIPWithFallback 依次尝试一组 URL，返回首个成功获取到的 IP 地址
func fetchPublicIPWithFallback(urls []string, proxyAddr string) (string, error) {
	var lastErr error
	for _, u := range urls {
		ip, err := fetchPublicIP(u, proxyAddr)
		if err == nil && ip != "" {
			return ip, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("URL 列表为空")
}

// testDelayThroughProxy 通过代理测试目标URL的延迟（HEAD请求），返回毫秒
func testDelayThroughProxy(targetURL string, timeout time.Duration) (int, error) {
	proxyPort := getProxyPortFromConfig()
	if proxyPort == 0 {
		return 0, fmt.Errorf("no proxy port available")
	}
	proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", proxyPort)
	proxyURL, err := url.Parse(proxyAddr)
	if err != nil {
		return 0, err
	}
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		// 禁止重定向，测速只需要首包响应即可
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequest("HEAD", targetURL, nil)
	if err != nil {
		return 0, err
	}
	// 强制通知代理与源站发送完报头后立即关闭连接，防止因无结束标记挂起超时
	req.Close = true
	// 附带标准的浏览器 UA，防止被防火墙或 WAF 丢包防爬拦截导致误报超时
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	// 即使状态码不是200，也认为连接成功，只要建立连接即可
	elapsed := time.Since(start).Milliseconds()
	return int(elapsed), nil
}

// handleDelayTestGoogle 测试 Google 延迟
func handleDelayTestGoogle(w http.ResponseWriter, r *http.Request) {
	handleDelayTestCommon(w, r, "https://www.gstatic.com/generate_204")
}

// handleDelayTestYouTube 测试 YouTube 延迟
func handleDelayTestYouTube(w http.ResponseWriter, r *http.Request) {
	handleDelayTestCommon(w, r, "https://www.youtube.com")
}

// handleDelayTestGitHub 测试 GitHub 延迟
func handleDelayTestGitHub(w http.ResponseWriter, r *http.Request) {
	handleDelayTestCommon(w, r, "https://github.com")
}

// handleDelayTestBaidu 测试 Baidu 延迟
func handleDelayTestBaidu(w http.ResponseWriter, r *http.Request) {
	handleDelayTestCommon(w, r, "https://www.baidu.com")
}

// handleDelayTestBilibili 测试 Bilibili 延迟
func handleDelayTestBilibili(w http.ResponseWriter, r *http.Request) {
	handleDelayTestCommon(w, r, "https://www.bilibili.com")
}

// handleDelayTestCustom 测试自定义 URL 延迟
func handleDelayTestCustom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	customURL := r.URL.Query().Get("url")
	if customURL == "" {
		writeJSONError(w, http.StatusBadRequest, "缺少 url 参数")
		return
	}
	// 简单校验是否以 http:// 或 https:// 开头
	if !strings.HasPrefix(customURL, "http://") && !strings.HasPrefix(customURL, "https://") {
		writeJSONError(w, http.StatusBadRequest, "url 必须以 http:// 或 https:// 开头")
		return
	}
	timeoutMs := 5000
	if t := r.URL.Query().Get("timeout"); t != "" {
		if val, err := strconv.Atoi(t); err == nil && val > 0 {
			timeoutMs = val
		}
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond
	delay, err := testDelayThroughProxy(customURL, timeout)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"delay": nil, "error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"delay": delay})
}

// 公共处理函数
func handleDelayTestCommon(w http.ResponseWriter, r *http.Request, targetURL string) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	// 读取超时参数，默认5000ms
	timeoutMs := 5000
	if t := r.URL.Query().Get("timeout"); t != "" {
		if val, err := strconv.Atoi(t); err == nil && val > 0 {
			timeoutMs = val
		}
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond
	delay, err := testDelayThroughProxy(targetURL, timeout)
	if err != nil {
		// 超时或错误，返回 delay=null 或 -1，前端显示超时
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"delay": nil, "error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"delay": delay})
}

// getLocalIPFromInterfaces 从本地网卡获取有效的 IPv4 或全球单播 IPv6 地址作为回退展示
func getLocalIPFromInterfaces(isIPv6 bool) (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if (iface.Flags & net.FlagUp) == 0 || (iface.Flags & net.FlagLoopback) != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if isIPv6 {
				// 获取非链路本地且合法的全球单播 IPv6 地址 (通常为公网分发 IPv6)
				if ip.To4() == nil && ip.IsGlobalUnicast() && !ip.IsLinkLocalUnicast() {
					return ip.String(), nil
				}
			} else {
				// 获取首个有效的局域网/公网 IPv4
				if ip.To4() != nil {
					return ip.String(), nil
				}
			}
		}
	}
	return "", fmt.Errorf("no active interface IP found")
}

// ===== 节点质量评分系统 =====

// NodeHistoryEntry 单次测速历史记录（仅用于计算）
type NodeHistoryEntry struct {
	Latency int `json:"latency"` // 毫秒，-1 表示超时/失败
}

// NodeQualityScore 节点质量分数
type NodeQualityScore struct {
	Score        int `json:"score"`        // 综合评分 0-100
	LatencyScore int `json:"latencyScore"` // 延迟评分
	Stability    int `json:"stability"`    // 稳定性评分
	SuccessRate  int `json:"successRate"`  // 成功率评分
}

// 评分权重（可配置，暂时固定）
const (
	weightLatency     = 50
	weightStability   = 30
	weightSuccessRate = 20
)

// 计算延迟评分（0-100）
func calcLatencyScore(latencies []int) int {
	var valid []int
	for _, l := range latencies {
		if l > 0 {
			valid = append(valid, l)
		}
	}
	if len(valid) == 0 {
		return 0
	}
	sum := 0
	for _, l := range valid {
		sum += l
	}
	avg := float64(sum) / float64(len(valid))

	// 对数归一化：50ms=100, 500ms=50, 5000ms=0
	if avg <= 50 {
		return 100
	}
	if avg >= 5000 {
		return 0
	}
	minLog := math.Log(50)
	maxLog := math.Log(5000)
	currentLog := math.Log(avg)
	score := 100 * (1 - (currentLog-minLog)/(maxLog-minLog))
	return int(math.Round(score))
}

// 计算稳定性评分（0-100）
func calcStabilityScore(latencies []int) int {
	var valid []int
	for _, l := range latencies {
		if l > 0 {
			valid = append(valid, l)
		}
	}
	if len(valid) == 0 {
		return 0
	}
	if len(valid) < 2 {
		return 50 // 单样本给中性分
	}
	mean := 0.0
	for _, l := range valid {
		mean += float64(l)
	}
	mean /= float64(len(valid))
	variance := 0.0
	for _, l := range valid {
		diff := float64(l) - mean
		variance += diff * diff
	}
	variance /= float64(len(valid))
	stdDev := math.Sqrt(variance)
	cv := stdDev / mean // 变异系数
	if cv <= 0.1 {
		return 100
	}
	if cv >= 0.5 {
		return 0
	}
	return int(math.Round(100 * (1 - (cv-0.1)/0.4)))
}

// 计算成功率评分（0-100）
func calcSuccessRateScore(histories []NodeHistoryEntry) int {
	if len(histories) == 0 {
		return 0
	}
	success := 0
	for _, h := range histories {
		if h.Latency > 0 {
			success++
		}
	}
	return int(math.Round(float64(success) / float64(len(histories)) * 100))
}

// handleQualityScores 返回所有节点的质量分数（从内核获取历史数据计算）
func handleQualityScores(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}

	// 从内核获取所有代理数据
	resp, err := coreRequest("GET", "/proxies", nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "获取代理数据失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeJSONError(w, resp.StatusCode, "内核返回错误")
		return
	}

	var proxiesData struct {
		Proxies map[string]struct {
			History []struct {
				Time  string `json:"time"`
				Delay int    `json:"delay"`
			} `json:"history"`
		} `json:"proxies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&proxiesData); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "解析内核数据失败: "+err.Error())
		return
	}

	// 计算每个节点的质量分数
	scores := make(map[string]NodeQualityScore)
	for name, proxy := range proxiesData.Proxies {
		// 提取历史延迟列表
		latencies := make([]int, 0, len(proxy.History))
		histories := make([]NodeHistoryEntry, 0, len(proxy.History))
		for _, h := range proxy.History {
			latencies = append(latencies, h.Delay)
			histories = append(histories, NodeHistoryEntry{Latency: h.Delay})
		}
		if len(latencies) == 0 {
			scores[name] = NodeQualityScore{Score: 0, LatencyScore: 0, Stability: 0, SuccessRate: 0}
			continue
		}

		latScore := calcLatencyScore(latencies)
		stabScore := calcStabilityScore(latencies)
		succScore := calcSuccessRateScore(histories)

		totalWeight := weightLatency + weightStability + weightSuccessRate
		if totalWeight == 0 {
			scores[name] = NodeQualityScore{Score: 0, LatencyScore: latScore, Stability: stabScore, SuccessRate: succScore}
			continue
		}
		score := float64(latScore)*float64(weightLatency)/float64(totalWeight) +
			float64(stabScore)*float64(weightStability)/float64(totalWeight) +
			float64(succScore)*float64(weightSuccessRate)/float64(totalWeight)

		scores[name] = NodeQualityScore{
			Score:        int(math.Round(score)),
			LatencyScore: latScore,
			Stability:    stabScore,
			SuccessRate:  succScore,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(scores)
}

// ===== 订阅健康检查定时器 =====

var (
	healthCheckTicker  *time.Ticker
	healthCheckStop    chan struct{}
	lastHealthCheck    map[string]time.Time
	healthCheckMu      sync.RWMutex
)

// startHealthCheckTimer 启动健康检查定时器（仅在 switch 模式下生效）
func startHealthCheckTimer() {
	if healthCheckTicker != nil {
		return
	}
	healthCheckStop = make(chan struct{})
	healthCheckMu.Lock()
	lastHealthCheck = make(map[string]time.Time)
	// 将当前激活订阅的 lastHealthCheck 设为当前时间，避免启动后立即测速
 	subscribeMu.RLock()
 	if subscribeConfig.Mode == "switch" && subscribeConfig.ActiveSubscription != "" {
 		lastHealthCheck[subscribeConfig.ActiveSubscription] = time.Now()
 	}
 	subscribeMu.RUnlock()
	healthCheckMu.Unlock()

	healthCheckTicker = time.NewTicker(10 * time.Second) // 每10秒检查一次
	go func() {
		for {
			select {
			case <-healthCheckTicker.C:
				performHealthChecks()
			case <-healthCheckStop:
				return
			}
		}
	}()
}

// stopHealthCheckTimer 停止健康检查定时器
func stopHealthCheckTimer() {
	if healthCheckTicker != nil {
		healthCheckTicker.Stop()
		healthCheckTicker = nil
	}
	if healthCheckStop != nil {
		close(healthCheckStop)
		healthCheckStop = nil
	}
	healthCheckMu.Lock()
	lastHealthCheck = nil
	healthCheckMu.Unlock()
}

// performHealthChecks 执行健康检查（仅在 switch 模式下，对当前激活订阅测速）
func performHealthChecks() {
	subscribeMu.RLock()
	cfg := subscribeConfig
	subscribeMu.RUnlock()

	if cfg.Mode != "switch" || len(cfg.Subscriptions) == 0 || cfg.ActiveSubscription == "" {
		return
	}

	// 获取当前激活的订阅
	var activeSub *Subscription
	for i := range cfg.Subscriptions {
		if cfg.Subscriptions[i].Name == cfg.ActiveSubscription {
			activeSub = &cfg.Subscriptions[i]
			break
		}
	}
	if activeSub == nil {
		return
	}

	interval := activeSub.HealthInterval
	if interval <= 0 {
		interval = 600
	}

	now := time.Now()
	healthCheckMu.RLock()
	last, ok := lastHealthCheck[activeSub.Name]
	healthCheckMu.RUnlock()
	if ok && now.Sub(last) < time.Duration(interval)*time.Second {
		return
	}

	// 从内核获取所有节点名称（包括组和叶子节点）
	nodes, err := getAllProxyNames()
	if err != nil {
		log.Printf("[HealthCheck] 获取节点列表失败: %v", err)
		return
	}
	if len(nodes) == 0 {
		return
	}

	// 并发测速（限制并发数10）
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)
	for _, name := range nodes {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			// 使用默认测试URL和超时
			testDelayForProxy(p, "http://www.gstatic.com/generate_204", 5000)
		}(name)
	}
	wg.Wait()

	healthCheckMu.Lock()
	lastHealthCheck[activeSub.Name] = time.Now()
	healthCheckMu.Unlock()
}

// getAllProxyNames 从内核获取所有叶子节点名称（去重）
func getAllProxyNames() ([]string, error) {
	resp, err := coreRequest("GET", "/proxies", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("内核返回状态码 %d", resp.StatusCode)
	}
	var data struct {
		Proxies map[string]struct {
			All []string `json:"all"`
		} `json:"proxies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	// 收集所有叶子节点（All 中的成员），使用 map 去重
	nameSet := make(map[string]bool)
	for _, proxy := range data.Proxies {
		for _, member := range proxy.All {
			nameSet[member] = true
		}
	}
	result := make([]string, 0, len(nameSet))
	for n := range nameSet {
		result = append(result, n)
	}
	return result, nil
}

// testDelayForProxy 测速单个节点（不关心返回值，只触发内核更新历史）
func testDelayForProxy(name, testURL string, timeoutMs int) {
	target := fmt.Sprintf("/proxies/%s/delay?url=%s&timeout=%d",
		url.PathEscape(name),
		url.QueryEscape(testURL),
		timeoutMs)
	// 忽略错误，仅触发测速
	resp, err := coreRequest("GET", target, nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	// 不需要读取响应体
}