package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"context"
    "time"
	"net/url"
	"io"
    "encoding/base64"
    "strconv"
)

// SubscribeConfig 订阅配置结构体
type SubscribeConfig struct {
	ProxyPort      int            `json:"proxy_port"`
	TproxyPort     int            `json:"tproxy_port"`
	PanelPort      int            `json:"panel_port"`
	PanelSecret    string         `json:"panel_secret"`
	RuleGroup      string         `json:"rule_group"`
	UIPanel        string         `json:"ui_panel"`
	MetaBackendURL string         `json:"meta_backend_url"`
	Mode           string         `json:"mode"`
	ActiveSubscription string     `json:"active_subscription"`
	Subscriptions  []Subscription `json:"subscriptions"`
	DeletePhysical []string       `json:"delete_physical,omitempty"`

}

type Subscription struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	UpdateInterval int    `json:"update_interval"`
	HealthInterval int    `json:"health_interval"`
	Prefix         string `json:"prefix"`
	UpdatedAt      string `json:"updated_at,omitempty"`
    SubscriptionInfo map[string]interface{} `json:"subscription_info,omitempty"`
}

var (
	subscribeConfig SubscribeConfig
	subscribeMu     sync.RWMutex
	timerCancel map[string]context.CancelFunc // 订阅名称 -> 取消函数
    timerMu     sync.RWMutex
)

func init() {
    timerCancel = make(map[string]context.CancelFunc)
}

// loadSubscribeConfig 从文件加载订阅配置（启动时调用），若失败则设置默认值
func loadSubscribeConfig() {
    subscribeMu.Lock()
    defer subscribeMu.Unlock()

    defaultCfg := SubscribeConfig{
        ProxyPort:      7890,
        PanelPort:      9090,
        PanelSecret:    "",
        RuleGroup:      "base",
        UIPanel:        "metacubexd",
        MetaBackendURL: "",
        Subscriptions:  []Subscription{},
        TproxyPort:     7898,
    }

    data, err := os.ReadFile(fluxorConfigFile)
    if err != nil {
        if !os.IsNotExist(err) {
            log.Printf("读取订阅配置失败: %v", err)
        }
        subscribeConfig = defaultCfg
        return
    }

    var tmp SubscribeConfig
    if err := json.Unmarshal(data, &tmp); err != nil {
        log.Printf("解析订阅配置失败: %v，使用默认配置", err)
        subscribeConfig = defaultCfg
        return
    }

    // 检查 JSON 中哪些键实际存在
    var raw map[string]json.RawMessage
    if err := json.Unmarshal(data, &raw); err != nil {
        subscribeConfig = defaultCfg
        return
    }

    // 仅当键不存在时，才使用默认值（避免覆盖用户设置的 0）
    if _, ok := raw["proxy_port"]; !ok {
        tmp.ProxyPort = defaultCfg.ProxyPort
    }
    if _, ok := raw["panel_port"]; !ok {
        tmp.PanelPort = defaultCfg.PanelPort
    }
    if _, ok := raw["tproxy_port"]; !ok {
        tmp.TproxyPort = defaultCfg.TproxyPort
    }

    // 字符串类型字段：若为空则设为默认值（合理）
    if tmp.UIPanel == "" {
        tmp.UIPanel = defaultCfg.UIPanel
    }
    if tmp.Mode == "" {
        tmp.Mode = "merge"
    }
    if tmp.Subscriptions == nil {
        tmp.Subscriptions = []Subscription{}
    }

    subscribeConfig = tmp
    log.Printf("成功加载订阅配置：%d 个订阅", len(subscribeConfig.Subscriptions))
}

// saveSubscribeConfig 保存订阅配置到文件
func saveSubscribeConfig() error {
    subscribeMu.Lock()
    defer subscribeMu.Unlock()
    data, err := json.MarshalIndent(subscribeConfig, "", "  ")
    if err != nil {
        return err
    }
    dir := filepath.Dir(fluxorConfigFile)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return err
    }
    return os.WriteFile(fluxorConfigFile, data, 0644)
}
// ---------- 订阅配置 API ----------

// handleSubscribeConfigAPI 处理 GET /subscribe/config 和 POST /subscribe/config
func handleSubscribeConfigAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		subscribeMu.RLock()
		defer subscribeMu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(subscribeConfig); err != nil {
			log.Printf("编码订阅配置失败: %v", err)
		}

	case http.MethodPost:
		var newConfig SubscribeConfig
		if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
			writeJSONError(w, http.StatusBadRequest, "无效的请求格式: "+err.Error())
			return
		}
		if newConfig.MetaBackendURL != "" && !backendURLRegex.MatchString(newConfig.MetaBackendURL) {
			writeJSONError(w, http.StatusBadRequest, "外部面板后端地址格式不正确")
			return
		}
		subscribeMu.Lock()
		subscribeConfig = newConfig
		subscribeMu.Unlock()

		if err := saveSubscribeConfig(); err != nil {
			log.Printf("保存订阅配置失败: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "保存配置失败: "+err.Error())
			return
		}

		// 重置定时器（先停止再启动）
		stopAllTimers()
		startAllTimers()

		respondJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"message": "配置已保存",
		})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func handleGenerateConfig(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
        return
    }
    var cfg SubscribeConfig
    if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
        writeJSONError(w, http.StatusBadRequest, "无效的请求格式: "+err.Error())
        return
    }

    if cfg.MetaBackendURL != "" && !backendURLRegex.MatchString(cfg.MetaBackendURL) {
        writeJSONError(w, http.StatusBadRequest, "外部面板后端地址格式不正确")
        return
    }

    // 物理清理标记删除的配置文件，使用 filepath.Base 防范路径穿越
    if len(cfg.DeletePhysical) > 0 {
        for _, name := range cfg.DeletePhysical {
            cleanName := filepath.Base(name)
            if cleanName == "." || cleanName == "/" || cleanName == "\\" {
                continue
            }
            targetFile := filepath.Join(coreWorkDir, "proxies", cleanName+".yaml")
            if _, err := os.Stat(targetFile); err == nil {
                if err := os.Remove(targetFile); err != nil {
                    log.Printf("[DELETE] 物理删除配置文件失败 %s: %v", targetFile, err)
                } else {
                    log.Printf("[DELETE] 成功物理删除配置文件: %s", targetFile)
                }
            }
        }
    }
    cfg.DeletePhysical = nil // 清空临时字段避免持久化

    // 切换模式
    if cfg.Mode == "switch" {
        // 如果订阅列表为空，生成基础配置，清除选中状态，保存并重载
        if len(cfg.Subscriptions) == 0 {
            // 生成基础配置文件
            if err := generateBaseConfig(cfg); err != nil {
                writeJSONError(w, http.StatusInternalServerError, "生成基础配置失败: "+err.Error())
                return
            }
            // 清除选中的订阅
            cfg.ActiveSubscription = ""
            // 保存配置到全局并持久化
            subscribeMu.Lock()
            subscribeConfig = cfg
            subscribeMu.Unlock()
            if err := saveSubscribeConfig(); err != nil {
                log.Printf("保存订阅配置失败: %v", err)
            }
            // 重置定时器（无订阅时需停止所有定时器）
            stopAllTimers()
            startAllTimers() // 会检查模式，切换模式且无订阅时会跳过启动
            // 重载内核
            if err := reloadCore(); err != nil {
                respondJSON(w, http.StatusOK, map[string]string{
                    "status":  "warning",
                    "message": "基础配置已生成，但重载内核失败: " + err.Error(),
                })
                return
            }
            respondJSON(w, http.StatusOK, map[string]string{
                "status":  "ok",
                "message": "已清除订阅，切换到基础配置",
            })
            return
        }

        // 有订阅时，确保所有订阅文件已下载
        if err := ensureSubscriptionFiles(&cfg); err != nil {
            writeJSONError(w, http.StatusInternalServerError, "下载订阅文件失败: "+err.Error())
            return
        }
        // 检查是否选中了订阅
        if cfg.ActiveSubscription == "" {
            writeJSONError(w, http.StatusBadRequest, "切换模式下请先选择一个订阅")
            return
        }
        // 构建源文件路径
        srcFile := filepath.Join(coreWorkDir, "proxies", cfg.ActiveSubscription+".yaml")
        if _, err := os.Stat(srcFile); err != nil {
            writeJSONError(w, http.StatusInternalServerError, "选中的订阅文件不存在: "+err.Error())
            return
        }
        // 复制文件到 configTarget
        if err := copyFile(srcFile, configTarget); err != nil {
            writeJSONError(w, http.StatusInternalServerError, "复制配置文件失败: "+err.Error())
            return
        }
        // 保存配置到 subscribe.json
        subscribeMu.Lock()
        subscribeConfig = cfg
        subscribeMu.Unlock()
        if err := saveSubscribeConfig(); err != nil {
            log.Printf("保存订阅配置失败: %v", err)
        }
        // 重置定时器
        stopAllTimers()
        startAllTimers()
        // 重载内核
        if err := reloadCore(); err != nil {
            respondJSON(w, http.StatusOK, map[string]string{
                "status":  "warning",
                "message": "配置文件已复制，但重载内核失败: " + err.Error(),
            })
            return
        }
        respondJSON(w, http.StatusOK, map[string]string{
            "status":  "ok",
            "message": "已切换到订阅 " + cfg.ActiveSubscription + " 的配置",
        })
        return
    }

    // ---------- 融合模式（原有逻辑） ----------
    // 删除旧配置
    if _, err := os.Stat(configTarget); err == nil {
        if err := os.Remove(configTarget); err != nil {
            writeJSONError(w, http.StatusInternalServerError, "删除旧配置文件失败: "+err.Error())
            return
        }
    }

    subscribeMu.Lock()
    subscribeConfig = cfg
    subscribeMu.Unlock()
    if err := saveSubscribeConfig(); err != nil {
        writeJSONError(w, http.StatusInternalServerError, "保存配置失败: "+err.Error())
        return
    }
	// 重置定时器
    stopAllTimers()
    startAllTimers()

    if err := generateConfig(cfg); err != nil {
        writeJSONError(w, http.StatusInternalServerError, "生成配置文件失败: "+err.Error())
        return
    }

    if cfg.MetaBackendURL != "" {
        if err := modifyMetaConfig(cfg.MetaBackendURL); err != nil {
            log.Printf("[WARN] 修改 MetaCubeXD 后端地址失败: %v", err)
        }
    }

    if err := reloadCore(); err != nil {
        respondJSON(w, http.StatusOK, map[string]string{
            "status":  "warning",
            "message": "配置文件已生成，但重载内核失败: " + err.Error(),
        })
        return
    }

	updateAllSubscriptionsMetadata(&cfg)
	// 将更新后的 cfg 保存到全局并持久化
	subscribeMu.Lock()
	subscribeConfig = cfg
	subscribeMu.Unlock()
	if err := saveSubscribeConfig(); err != nil {
    	log.Printf("保存订阅配置失败: %v", err)
	}

    respondJSON(w, http.StatusOK, map[string]string{
        "status":  "ok",
        "message": "配置文件已生成并成功重载内核",
    })
}
// patchSubscriptionFile 修改订阅文件，添加/更新必要字段
func patchSubscriptionFile(filePath string, cfg SubscribeConfig) error {
    content, err := os.ReadFile(filePath)
    if err != nil {
        return err
    }
    text := string(content)

    // 清理可能冲突的顶层单端口定义，防止重复端口绑定导致内核崩溃
    conflictKeys := []string{"port", "socks-port", "redir-port"}
    for _, k := range conflictKeys {
        reConflict := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(k) + `:\s*\d+\s*$`)
        text = reConflict.ReplaceAllString(text, "# "+k+" removed by Fluxor")
    }

    // 定义待替换/添加的字段
    rules := []struct {
        key   string
        value string
    }{
        {"mixed-port", fmt.Sprintf("%d", cfg.ProxyPort)},
        {"tproxy-port", fmt.Sprintf("%d", cfg.TproxyPort)},
        {"external-controller", fmt.Sprintf("'0.0.0.0:%d'", cfg.PanelPort)},
        {"external-controller-unix", fmt.Sprintf("'%s'", coreSocket)},
        {"secret", fmt.Sprintf("'%s'", cfg.PanelSecret)},
		{"allow-lan", "true"},
        {"ipv6", "true"},
		{"unified-delay", "true"},
        {"geodata-mode", "false"},
        {"routing-mark", "255"},
    }

    // 根据面板选择 external-ui
    uiPath := "ui/meta"
    if cfg.UIPanel == "zashboard" {
        uiPath = "ui/zash"
    }
    rules = append(rules, struct{ key, value string }{"external-ui", uiPath})

	// 仅当面板为 zashboard 时添加 external-ui-url
	if cfg.UIPanel == "zashboard" {
		rules = append(rules, struct{ key, value string }{
			"external-ui-url",
			`"https://github.com/Zephyruso/zashboard/releases/latest/download/dist-cdn-fonts.zip"`,
		})
	}

    // 遍历规则，替换或追加
    for _, r := range rules {
        // 匹配行首的键，后跟冒号和任意空白
        re := regexp.MustCompile(`(?m)^(` + regexp.QuoteMeta(r.key) + `):\s*.*$`)
        if re.MatchString(text) {
            // 替换已有行
            text = re.ReplaceAllString(text, "$1: "+r.value)
        } else {
            // 不存在则追加到文件末尾
            text += "\n" + r.key + ": " + r.value
        }
    }

    // ----- DNS 监听注入，替换/注入完整 DNS 配置 -----
    reDns := regexp.MustCompile(`(?m)^dns:\s*\r?\n((?:[ \t]+.*\r?\n?)*)`)
    text = reDns.ReplaceAllString(text, "")
    text += "\n" + dnsBlock + "\n"
    
    return os.WriteFile(filePath, []byte(text), 0644)
}
// copyFile 复制文件
func copyFile(src, dst string) error {
    srcData, err := os.ReadFile(src)
    if err != nil {
        return err
    }
    return os.WriteFile(dst, srcData, 0644)
}

// generateConfig 根据订阅配置生成 config.yaml
func generateConfig(cfg SubscribeConfig) error {
	// 无订阅时生成基本配置（使用配置中的端口和密钥）
	if len(cfg.Subscriptions) == 0 {
		basic := fmt.Sprintf(`mixed-port: %d
tproxy-port: %d
allow-lan: true
mode: rule
log-level: silent
external-controller-unix: '%s'
external-controller: '0.0.0.0:%d'
`, cfg.ProxyPort, cfg.TproxyPort, coreSocket, cfg.PanelPort)
		if cfg.PanelSecret != "" {
			basic += fmt.Sprintf("secret: '%s'\n", cfg.PanelSecret)
		}
		uiSelect := "ui/meta"
		uiURL := ""
		if cfg.UIPanel == "zashboard" {
			uiSelect = "ui/zash"
			uiURL = `external-ui-url: "https://github.com/Zephyruso/zashboard/releases/latest/download/dist-cdn-fonts.zip"`
		}
		basic += fmt.Sprintf("external-ui: %s\n", uiSelect)
		if uiURL != "" {
			basic += uiURL + "\n"
		}
		dir := filepath.Dir(configTarget)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录失败: %w", err)
		}
		return os.WriteFile(configTarget, []byte(basic), 0644)
	}

	// 生成 proxy-providers 子项（不包含头部，每项缩进2个空格）
	var providersBuf strings.Builder
	for i, sub := range cfg.Subscriptions {
		interval := sub.UpdateInterval
		if interval <= 0 {
			interval = 86400
		}
		health := sub.HealthInterval
		if health <= 0 {
			health = 300
		}
		prefix := sub.Prefix // 直接使用，不再依赖开关

		providersBuf.WriteString(fmt.Sprintf(`  %s:
    type: http
    url: "%s"
    interval: %d
    path: proxies/%s.yaml
    health-check:
      enable: true
      url: "https://www.gstatic.com/generate_204"
      interval: %d
`, sub.Name, sub.URL, interval, sub.Name, health))

		// 如果前缀非空，添加 override.additional-prefix
		if prefix != "" {
			providersBuf.WriteString(fmt.Sprintf(`    override:
      additional-prefix: "%s"
`, prefix))
		}

		if i < len(cfg.Subscriptions)-1 {
			providersBuf.WriteString("\n")
		}
	}

    configContent := configTemplate

    // 定义待替换/添加的字段
    rules := []struct {
        key   string
        value string
    }{
        {"mixed-port", fmt.Sprintf("%d", cfg.ProxyPort)},
        {"tproxy-port", fmt.Sprintf("%d", cfg.TproxyPort)},
        {"external-controller", fmt.Sprintf("'0.0.0.0:%d'", cfg.PanelPort)},
        {"secret", fmt.Sprintf("'%s'", cfg.PanelSecret)},
    }

    // 添加 external-ui
	uiPath := "ui/meta"
	if cfg.UIPanel == "zashboard" {
		uiPath = "ui/zash"
	}
	rules = append(rules, struct{ key, value string }{"external-ui", uiPath})

	// 仅当面板为 zashboard 时添加 external-ui-url
	if cfg.UIPanel == "zashboard" {
		rules = append(rules, struct{ key, value string }{
			"external-ui-url",
			`"https://github.com/Zephyruso/zashboard/releases/latest/download/dist-cdn-fonts.zip"`,
		})
	}

    // 应用规则到 configContent（替换或追加）
	for _, r := range rules {
		re := regexp.MustCompile(`(?m)^(` + regexp.QuoteMeta(r.key) + `):\s*.*$`)
		if re.MatchString(configContent) {
			configContent = re.ReplaceAllString(configContent, "$1: "+r.value)
		} else {
			configContent += "\n" + r.key + ": " + r.value
		}
	}

	// ----- 确保 proxy-providers 字段存在并注入内容 -----
	if providersBuf.Len() > 0 {
  	  reProxy := regexp.MustCompile(`(?m)^proxy-providers:\s*$`)
   	 if reProxy.MatchString(configContent) {
    	    // 替换主键行，并在其后插入 providers 内容（providersBuf 已含缩进）
    	    configContent = reProxy.ReplaceAllString(configContent, "proxy-providers:\n"+providersBuf.String())
    	} else {
    	    // 不存在则追加
            configContent += "\nproxy-providers:\n" + providersBuf.String()
        }
    }

    // 根据规则集生成动态块
    var groupsBlock, providersBlock, rulesBlock string
    switch cfg.RuleGroup {
    case "base":
        groupsBlock = proxyGroupsBase
        providersBlock = "" // base 不生成 rule-providers
        rulesBlock = rulesBase
    case "full":
        // 替换 __SUB_NAMES__ 为订阅名称列表（逗号分隔）
        subList := strings.Join(subNames(cfg.Subscriptions), ",")
        groupsBlock = strings.ReplaceAll(proxyGroupsFullTemplate, "__SUB_NAMES__", subList)
        providersBlock = ruleProvidersFull
        rulesBlock = rulesFull
    default:
        return fmt.Errorf("未知规则集: %s", cfg.RuleGroup)
    }

    // 追加到配置末尾（顺序：rule-providers -> proxy-groups -> rules）
    if providersBlock != "" {
        configContent += "\n" + providersBlock
    }
    configContent += "\n" + groupsBlock + "\n" + rulesBlock

    // ----- DNS 监听注入 -----
	reDns := regexp.MustCompile(`(?m)^dns:\s*\r?\n((?:[ \t]+.*\r?\n)*?)`)
    configContent = reDns.ReplaceAllString(configContent, "")
    // 追加新的 DNS 块（确保前后有空行）
    configContent += "\n" + dnsBlock + "\n"

	// 清理多余空行
	configContent = regexp.MustCompile(`\n{3,}`).ReplaceAllString(configContent, "\n\n")

	// 写入文件
	dir := filepath.Dir(configTarget)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	return os.WriteFile(configTarget, []byte(configContent), 0644)
}

// ensureSubscriptionFiles 在切换模式下，确保所有订阅的本地文件已下载
// 若模式为 "merge" 或无订阅，则直接返回 nil
func ensureSubscriptionFiles(cfg *SubscribeConfig) error {
    if cfg.Mode != "switch" {
        return nil
    }
    if len(cfg.Subscriptions) == 0 {
        return nil
    }

    proxiesDir := filepath.Join(coreWorkDir, "proxies")
    if err := os.MkdirAll(proxiesDir, 0755); err != nil {
        return fmt.Errorf("创建 proxies 目录失败: %w", err)
    }

    var wg sync.WaitGroup
    errCh := make(chan error, len(cfg.Subscriptions))
    sem := make(chan struct{}, 5)

    for i := range cfg.Subscriptions {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            sem <- struct{}{}
            defer func() { <-sem }()

            s := cfg.Subscriptions[idx]
            targetFile := filepath.Join(proxiesDir, s.Name+".yaml")

            // 检查文件是否存在以及是否有元数据
            needDownload := false
            if info, err := os.Stat(targetFile); err != nil || info.Size() == 0 {
                needDownload = true
            } else {
                // 文件存在且非空，检查元数据是否缺失
                if len(cfg.Subscriptions[idx].SubscriptionInfo) == 0 {
                    needDownload = true
                    // 为保险，先删除旧文件，确保重新下载
                    if err := os.Remove(targetFile); err != nil && !os.IsNotExist(err) {
                        errCh <- fmt.Errorf("订阅 %s 删除旧文件失败: %w", s.Name, err)
                        return
                    }
                }
            }

            if needDownload {
                // 下载文件并获取元数据
                updatedAt, subInfo, err := DownloadSubscriptionFile(s, idx, targetFile)
                if err != nil {
                    errCh <- fmt.Errorf("订阅 %s 下载失败: %w", s.Name, err)
                    return
                }
                cfg.Subscriptions[idx].UpdatedAt = updatedAt
                cfg.Subscriptions[idx].SubscriptionInfo = subInfo
                log.Printf("[ensure] 已下载并更新订阅 %s 元数据", s.Name)
                // 下载成功后打补丁
                if err := patchSubscriptionFile(targetFile, *cfg); err != nil {
                    errCh <- fmt.Errorf("订阅 %s 打补丁失败: %w", s.Name, err)
                }
            } else {
                // 文件已存在且元数据完整，只打补丁
                if err := patchSubscriptionFile(targetFile, *cfg); err != nil {
                    errCh <- fmt.Errorf("订阅 %s 打补丁失败: %w", s.Name, err)
                }
            }
        }(i)
    }

    wg.Wait()
    close(errCh)

    var errs []error
    for err := range errCh {
        errs = append(errs, err)
    }
    if len(errs) > 0 {
        return fmt.Errorf("部分操作失败: %v", errs)
    }
    return nil
}
// subNames 提取所有订阅名称
func subNames(subs []Subscription) []string {
	names := make([]string, len(subs))
	for i, s := range subs {
		names[i] = s.Name
	}
	return names
}

// respondJSON 辅助函数：返回统一的 JSON 响应
func respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
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
	re := regexp.MustCompile(`defaultBackendURL:\s*['"][^'"]*['"]`)
	if !re.MatchString(string(content)) {
		return fmt.Errorf("未找到 defaultBackendURL 配置项或格式不匹配")
	}
	newContent := re.ReplaceAllString(string(content), fmt.Sprintf("defaultBackendURL: '%s'", backendURL))
	if string(content) == newContent {
		return nil // 无需重复写入
	}
	return os.WriteFile(configPath, []byte(newContent), 0644)
}
// updateSubscriptionInSwitchMode 切换模式下的订阅更新逻辑，返回 needsReload 表示是否需要重载内核
func updateSubscriptionInSwitchMode(cfg *SubscribeConfig, subName string) (needsReload bool, err error) {
    var idx int = -1
    for i, s := range cfg.Subscriptions {
        if s.Name == subName {
            idx = i
            break
        }
    }
    if idx == -1 {
        return false, fmt.Errorf("订阅 %s 不存在", subName)
    }

    proxiesDir := filepath.Join(coreWorkDir, "proxies")
    targetFile := filepath.Join(proxiesDir, subName+".yaml")

    // 强制删除已有文件（确保重新下载）
    if err := os.Remove(targetFile); err != nil && !os.IsNotExist(err) {
        return false, fmt.Errorf("删除旧文件失败: %w", err)
    }

    log.Printf("[UPDATE] 开始下载订阅 %s，目标文件: %s", subName, targetFile)

    updatedAt, subInfo, err := DownloadSubscriptionFile(cfg.Subscriptions[idx], idx, targetFile)
    if err != nil {
        log.Printf("[UPDATE] 下载订阅 %s 失败: %v", subName, err)
        return false, fmt.Errorf("下载失败: %w", err)
    }
    cfg.Subscriptions[idx].UpdatedAt = updatedAt
    cfg.Subscriptions[idx].SubscriptionInfo = subInfo
    log.Printf("[UPDATE] 元数据已更新: updatedAt=%s", updatedAt)

    // 打补丁
    log.Printf("[UPDATE] 开始打补丁: %s", targetFile)
    if err := patchSubscriptionFile(targetFile, *cfg); err != nil {
        log.Printf("[UPDATE] 打补丁失败: %v", err)
        return false, fmt.Errorf("打补丁失败: %w", err)
    }
    log.Printf("[UPDATE] 补丁完成")

    // 如果该订阅是当前激活的订阅，则复制到 configTarget，并标记需要重载
    if cfg.Mode == "switch" && cfg.ActiveSubscription == subName {
        log.Printf("[UPDATE] 当前订阅为激活订阅，开始复制配置文件到 %s", configTarget)
        if err := copyFile(targetFile, configTarget); err != nil {
            log.Printf("[UPDATE] 复制失败: %v", err)
            return false, fmt.Errorf("复制配置文件失败: %w", err)
        }
        log.Printf("[UPDATE] 复制完成")
        return true, nil // 需要重载
    }

    log.Printf("[UPDATE] 当前订阅非激活订阅，跳过复制和重载")
    return false, nil
}
// handleSubscribeUpdate 处理 POST /subscribe/update/{name}
func handleSubscribeUpdate(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
        return
    }

    path := strings.TrimPrefix(r.URL.Path, baseURL+"/subscribe/update/")
    if path == "" {
        writeJSONError(w, http.StatusBadRequest, "缺少订阅名称")
        return
    }
    name, err := url.QueryUnescape(path)
    if err != nil {
        writeJSONError(w, http.StatusBadRequest, "无效的订阅名称")
        return
    }

    log.Printf("[UPDATE] 收到更新请求: %s", name)

    subscribeMu.RLock()
    mode := subscribeConfig.Mode
    subscribeMu.RUnlock()

    var targetSub Subscription
    var found bool

    if mode == "merge" {
        // 先在主线程快速判断订阅是否存在，保证基本参数合法
        subscribeMu.RLock()
        for _, s := range subscribeConfig.Subscriptions {
            if s.Name == name {
                found = true
                break
            }
        }
        subscribeMu.RUnlock()

        if !found {
            writeJSONError(w, http.StatusNotFound, "未找到该订阅")
            return
        }

        // 启动后台协程异步调用内核更新并拉取元数据，避免阻塞 HTTP 主线程导致 504
        go func(subName string) {
            log.Printf("[ASYNC-UPDATE] 后台启动更新订阅: %s", subName)
            encoded := url.QueryEscape(subName)
            resp, err := coreRequest("PUT", "/providers/proxies/"+encoded, nil)
            if err != nil {
                log.Printf("[ASYNC-UPDATE][ERROR] 调用内核更新失败 %s: %v", subName, err)
                return
            }
            resp.Body.Close()

            // 从主内核拉取最新的元数据
            updatedAt, subInfo, err := fetchSubscriptionMetadataFromCore(subName)
            if err != nil {
                log.Printf("[ASYNC-UPDATE][ERROR] 获取订阅元数据失败 %s: %v", subName, err)
                return
            }

            // 更新内存配置数据
            subscribeMu.Lock()
            for i := range subscribeConfig.Subscriptions {
                if subscribeConfig.Subscriptions[i].Name == subName {
                    subscribeConfig.Subscriptions[i].UpdatedAt = updatedAt
                    subscribeConfig.Subscriptions[i].SubscriptionInfo = subInfo
                    break
                }
            }
            subscribeMu.Unlock()

            // 持久化保存到 subscribe.json
            if err := saveSubscribeConfig(); err != nil {
                log.Printf("[ASYNC-UPDATE][ERROR] 保存订阅配置失败 %s: %v", subName, err)
            } else {
                log.Printf("[ASYNC-UPDATE] 订阅 %s 后台更新并保存元数据成功", subName)
            }
        }(name)

        // 立即向前端回传 processing 状态
        respondJSON(w, http.StatusOK, map[string]string{
            "status":  "processing",
            "message": "订阅更新已在后台启动",
        })
        return
    } else {
        // 切换模式：启动HTTP下载或临时内核下载
        var needsReload bool
        var err2 error
        subscribeMu.Lock()
        needsReload, err2 = updateSubscriptionInSwitchMode(&subscribeConfig, name)
        for _, s := range subscribeConfig.Subscriptions {
            if s.Name == name {
                targetSub = s
                found = true
                break
            }
        }
        subscribeMu.Unlock()

        if err2 != nil {
            writeJSONError(w, http.StatusInternalServerError, "更新失败: "+err2.Error())
            return
        }
        if !found {
            writeJSONError(w, http.StatusNotFound, "未找到该订阅")
            return
        }

        // 如果需要重载，在锁外调用
        if needsReload {
            log.Printf("[UPDATE] 开始重载内核")
            if err := reloadCore(); err != nil {
                log.Printf("[UPDATE] 重载内核失败: %v", err)
            }
        }

        // 重置定时器
        stopAllTimers()
        startAllTimers()

        // 立即持久化（避免统一保存被绕过或失败时前端未知）
        if err := saveSubscribeConfig(); err != nil {
            log.Printf("[UPDATE] 保存订阅配置失败: %v", err)
            writeJSONError(w, http.StatusInternalServerError, "保存配置失败: "+err.Error())
            return
        }
    }

    var info interface{}
    if targetSub.SubscriptionInfo != nil {
        info = map[string]interface{}{
            "upload":    targetSub.SubscriptionInfo["upload"],
            "download":  targetSub.SubscriptionInfo["download"],
            "total":     targetSub.SubscriptionInfo["total"],
            "expire":    targetSub.SubscriptionInfo["expire"],
            "updatedAt": targetSub.UpdatedAt,
        }
    }

    respondJSON(w, http.StatusOK, map[string]interface{}{
        "status":  "ok",
        "message": "订阅 " + name + " 更新成功",
        "info":    info,
    })
}
// startSubscriptionTimer 为指定订阅启动定时更新（仅切换模式）
func startSubscriptionTimer(cfg *SubscribeConfig, idx int) {
    if cfg.Mode != "switch" {
        return
    }
    sub := cfg.Subscriptions[idx]
    if sub.UpdateInterval <= 0 {
        return
    }

    timerMu.Lock()
    defer timerMu.Unlock()

    // 取消旧定时器
    if cancel, ok := timerCancel[sub.Name]; ok {
        cancel()
        delete(timerCancel, sub.Name)
    }

    ctx, cancel := context.WithCancel(context.Background())
    timerCancel[sub.Name] = cancel

    go func(name string, interval int) {
        ticker := time.NewTicker(time.Duration(interval) * time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                // 执行更新（需要获取锁）
				var needsReload bool
				var err error
                subscribeMu.Lock()
                // 检查模式是否仍然为 switch 且订阅仍存在
                if subscribeConfig.Mode != "switch" {
                    subscribeMu.Unlock()
                    return
                }
                // 查找订阅索引
                var idxFound int = -1
                for i, s := range subscribeConfig.Subscriptions {
                    if s.Name == name {
                        idxFound = i
                        break
                    }
                }
                if idxFound == -1 {
                    subscribeMu.Unlock()
                    return
                }
				needsReload, err = updateSubscriptionInSwitchMode(&subscribeConfig, name)
                subscribeMu.Unlock()
                // 执行更新
                if err != nil {
                    log.Printf("定时更新订阅 %s 失败: %v", name, err)
                } else {
                    if err := saveSubscribeConfig(); err != nil {
                        log.Printf("保存订阅配置失败: %v", err)
                    }
                    if needsReload {
                        if err := reloadCore(); err != nil {
                            log.Printf("定时更新后重载内核失败: %v", err)
                        }
                    }
                }
            }
        }
    }(sub.Name, sub.UpdateInterval)
}
// fetchSubscriptionMetadataFromCore 从主内核获取订阅元数据
func fetchSubscriptionMetadataFromCore(subName string) (updatedAt string, subInfo map[string]interface{}, err error) {
	encoded := url.QueryEscape(subName)
	resp, err := coreRequest("GET", "/providers/proxies/"+encoded, nil)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("状态码: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", nil, err
	}
	updatedAtVal, _ := data["updatedAt"].(string)
	subInfoVal, _ := data["subscriptionInfo"].(map[string]interface{})
	// 规范化键为小写
	subInfoVal = normalizeMapKeys(subInfoVal)
	return updatedAtVal, subInfoVal, nil
}

// normalizeMapKeys 将 map 中所有字符串键转换为小写
func normalizeMapKeys(m map[string]interface{}) map[string]interface{} {
    if m == nil {
        return nil
    }
    result := make(map[string]interface{})
    for k, v := range m {
        result[strings.ToLower(k)] = v
    }
    return result
}

// updateAllSubscriptionsMetadata 更新所有订阅的元数据（仅用于融合模式）
func updateAllSubscriptionsMetadata(cfg *SubscribeConfig) {
    // 1. 备份当前全局配置中的旧元数据（用于失败时保留）
    subscribeMu.RLock()
    oldSubs := make(map[string]Subscription)
    for _, s := range subscribeConfig.Subscriptions {
        oldSubs[s.Name] = s
    }
    subscribeMu.RUnlock()

    // 2. 遍历每个订阅，尝试获取元数据
    for i := range cfg.Subscriptions {
        name := cfg.Subscriptions[i].Name
        var updatedAt string
        var subInfo map[string]interface{}
        var err error

        // 3. 重试机制：最多尝试 3 次，每次间隔 500ms
        for attempt := 0; attempt < 3; attempt++ {
            if attempt > 0 {
                time.Sleep(500 * time.Millisecond)
            }
            updatedAt, subInfo, err = fetchSubscriptionMetadataFromCore(name)
            if err == nil {
                break
            }
            log.Printf("获取订阅 %s 元数据失败 (尝试 %d/%d): %v", name, attempt+1, 3, err)
        }

        if err != nil {
            // 获取失败：尝试保留旧数据
            if old, ok := oldSubs[name]; ok {
                cfg.Subscriptions[i].UpdatedAt = old.UpdatedAt
                cfg.Subscriptions[i].SubscriptionInfo = old.SubscriptionInfo
                log.Printf("保留订阅 %s 的旧元数据（获取失败）", name)
            } else {
                cfg.Subscriptions[i].UpdatedAt = ""
                cfg.Subscriptions[i].SubscriptionInfo = nil
                log.Printf("订阅 %s 无历史元数据，保留为空", name)
            }
            continue
        }

        // 获取成功，更新
        cfg.Subscriptions[i].UpdatedAt = updatedAt
        cfg.Subscriptions[i].SubscriptionInfo = subInfo
        log.Printf("更新订阅 %s 元数据成功", name)
    }
}

// handleUpdateSubscriptionInfo 用于前端融合模式手动更新后持久化单个订阅元数据
func handleUpdateSubscriptionInfo(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
        return
    }
    path := strings.TrimPrefix(r.URL.Path, baseURL+"/subscribe/update-info/")
    if path == "" {
        writeJSONError(w, http.StatusBadRequest, "缺少订阅名称")
        return
    }
    name, err := url.QueryUnescape(path)
    if err != nil {
        writeJSONError(w, http.StatusBadRequest, "无效的订阅名称")
        return
    }

    var payload struct {
        Upload    int64  `json:"Upload"`
        Download  int64  `json:"Download"`
        Total     int64  `json:"Total"`
        Expire    int64  `json:"Expire"`
        UpdatedAt string `json:"updatedAt"`
    }
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        writeJSONError(w, http.StatusBadRequest, "无效的请求体: "+err.Error())
        return
    }

    subscribeMu.Lock()
    found := false
    for i := range subscribeConfig.Subscriptions {
        if subscribeConfig.Subscriptions[i].Name == name {
            subInfo := map[string]interface{}{
                "upload":   payload.Upload,
                "download": payload.Download,
                "total":    payload.Total,
                "expire":   payload.Expire,
            }
            subscribeConfig.Subscriptions[i].UpdatedAt = payload.UpdatedAt
            subscribeConfig.Subscriptions[i].SubscriptionInfo = subInfo
            found = true
            break
        }
    }
    subscribeMu.Unlock()

    if !found {
        writeJSONError(w, http.StatusNotFound, "订阅不存在")
        return
    }

    if err := saveSubscribeConfig(); err != nil {
        log.Printf("保存配置失败: %v", err)
        writeJSONError(w, http.StatusInternalServerError, "保存失败")
        return
    }
    respondJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "订阅信息已更新"})
}
// generateBaseConfig 生成基础配置文件（用于无订阅时）
func generateBaseConfig(cfg SubscribeConfig) error {
    basic := fmt.Sprintf(`mixed-port: %d
allow-lan: true
mode: rule
log-level: silent
external-controller-unix: '%s'
external-controller: '0.0.0.0:%d'
`, cfg.ProxyPort, coreSocket, cfg.PanelPort)

    // 如果面板密钥不为空，追加 secret 字段
    if cfg.PanelSecret != "" {
        basic += fmt.Sprintf("secret: '%s'\n", cfg.PanelSecret)
    }

    dir := filepath.Dir(configTarget)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return fmt.Errorf("创建目录失败: %w", err)
    }
    return os.WriteFile(configTarget, []byte(basic), 0644)
}

func startAllTimers() {
    subscribeMu.RLock()
    defer subscribeMu.RUnlock()
    if subscribeConfig.Mode != "switch" {
        return
    }
    for i := range subscribeConfig.Subscriptions {
        startSubscriptionTimer(&subscribeConfig, i)
    }
    startHealthCheckTimer()
}
// stopAllTimers 停止所有定时器
func stopAllTimers() {
    timerMu.Lock()
    defer timerMu.Unlock()
    for name, cancel := range timerCancel {
        cancel()
        delete(timerCancel, name)
    }
    stopHealthCheckTimer()
}

// DownloadSubscriptionFile 下载单个订阅的节点文件，返回元数据
// 优先尝试直接 HTTP 下载，失败则回退到临时内核方式
func DownloadSubscriptionFile(sub Subscription, index int, targetFile string) (updatedAt string, subInfo map[string]interface{}, err error) {
    // 1. 尝试直接下载
    directUpdatedAt, directSubInfo, directErr := tryDirectDownload(sub, targetFile)
    if directErr == nil {
        return directUpdatedAt, normalizeMapKeys(directSubInfo), nil
    }
    // 直接下载失败，记录日志并回退到临时内核
    if coreLogger != nil {
        coreLogger.Printf("[DOWNLOAD] 订阅 %s 直连下载失败: %v，回退到临时内核", sub.Name, directErr)
    } else {
        log.Printf("[DOWNLOAD] 订阅 %s 直连下载失败: %v，回退到临时内核", sub.Name, directErr)
    }

    // 2. 回退到原有临时内核流程
    updatedAt, subInfo, err = downloadWithTempCore(sub, index, targetFile)
    if err != nil {
        return "", nil, err
    }
    return updatedAt, normalizeMapKeys(subInfo), nil
}

// tryDirectDownload 尝试直接 HTTP 下载订阅
func tryDirectDownload(sub Subscription, targetFile string) (updatedAt string, subInfo map[string]interface{}, err error) {
    client := &http.Client{
        Timeout: 30 * time.Second,
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            if len(via) >= 10 {
                return fmt.Errorf("too many redirects")
            }
            return nil
        },
    }

    req, err := http.NewRequest("GET", sub.URL, nil)
    if err != nil {
        return "", nil, err
    }
    req.Header.Set("User-Agent", "clash.meta")
    req.Header.Set("Accept", "text/plain, application/json, */*")

    resp, err := client.Do(req)
    if err != nil {
        return "", nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return "", nil, fmt.Errorf("HTTP 状态码: %d", resp.StatusCode)
    }

    // 读取响应体
    bodyBytes, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", nil, err
    }

    // 检测是否为 Base64 编码
    content := string(bodyBytes)
    decodedContent, isBase64 := tryBase64Decode(bodyBytes)
    if isBase64 && decodedContent != "" {
        content = decodedContent
    }

    // 检查是否为有效订阅配置
    if !isValidSubscription(content) {
        return "", nil, fmt.Errorf("无效的订阅配置")
    }

    // 写入文件
    if err := os.WriteFile(targetFile, []byte(content), 0644); err != nil {
        return "", nil, err
    }

    // 解析 subscription-userinfo 头
    subInfo = parseSubscriptionUserinfo(resp.Header.Get("subscription-userinfo"))
    updatedAt = time.Now().Format(time.RFC3339)

    return updatedAt, subInfo, nil
}

// tryBase64Decode 尝试解码 Base64，返回解码后的字符串和是否成功
func tryBase64Decode(data []byte) (string, bool) {
    // 去除可能的空白字符
    raw := strings.TrimSpace(string(data))
    // 尝试标准 Base64 解码
    decoded, err := base64.StdEncoding.DecodeString(raw)
    if err == nil {
        return string(decoded), true
    }
    // 尝试 URL 编码 Base64 (替换 - _ 等)
    raw = strings.ReplaceAll(raw, "-", "+")
    raw = strings.ReplaceAll(raw, "_", "/")
    decoded, err = base64.StdEncoding.DecodeString(raw)
    if err == nil {
        return string(decoded), true
    }
    return "", false
}

// isValidSubscription 检查内容是否包含有效订阅标志
func isValidSubscription(content string) bool {
    // 检查是否包含 proxies: 或 proxy-providers: 或 proxy-groups:
    // 简单匹配，忽略大小写
    lower := strings.ToLower(content)
    return strings.Contains(lower, "proxies:") ||
        strings.Contains(lower, "proxy-providers:") ||
        strings.Contains(lower, "proxy-groups:")
}

// parseSubscriptionUserinfo 解析 subscription-userinfo 头
func parseSubscriptionUserinfo(header string) map[string]interface{} {
	result := make(map[string]interface{})
	if header == "" {
		return result
	}
	parts := strings.Split(header, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		switch key {
		case "upload", "download", "total":
			if v, err := strconv.ParseInt(val, 10, 64); err == nil {
				result[key] = v
			}
		case "expire":
			if v, err := strconv.ParseInt(val, 10, 64); err == nil {
				result[key] = v
			}
		default:
			result[key] = val
		}
	}
	return result
}

const configTemplate = `mixed-port:
tproxy-port:
allow-lan: true
ipv6: true
bind-address: '*'
mode: rule
log-level: silent
unified-delay: true
external-controller: ''
external-controller-unix: '/var/apps/Fluxor/target/core.sock'
external-ui:
external-ui-url:
secret: ''

routing-mark: 255
find-process-mode: strict
client-fingerprint: chrome

profile:
  store-selected: true
  store-fake-ip: true

sniffer:
  enable: true
  sniff:
    HTTP:
      ports: [80, 8080-8880]
      override-destination: true
    TLS:
      ports: [443, 8443]
    QUIC:
      ports: [443, 8443]
  skip-domain:
    - "+.push.apple.com"

tun:
  enable: false
  stack: mixed
  dns-hijack:
    - "any:53"
    - "tcp://any:53"
  auto-route: true
  auto-redirect: true
  auto-detect-interface: true

geodata-mode: false
geo-auto-update: true
geo-update-interval: 24
`

const proxyGroupsBase = `
proxy-groups:
  - {name: 🚀 节点选择, type: select, proxies: [👉 手动选择,♻️ 自动选择]}
  - {name: 👉 手动选择, type: select, include-all: true}
  - {name: ♻️ 自动选择, type: url-test, include-all: true, tolerance: 100}
  - {name: 🤖 Github, type: select, proxies:  [🚀 节点选择,🎯 全球直连]}
  - {name: 🇨🇳 国内, type: select, proxies:  [🎯 全球直连,🚀 节点选择]}
  - {name: 🧱 国外, type: select, proxies:  [🚀 节点选择,🎯 全球直连]}
  - {name: 🎯 全球直连, type: select, proxies: [DIRECT], hidden: true}
`

const rulesBase = `
rules:
  - GEOIP,lan,🎯 全球直连,no-resolve
  - GEOSITE,github,🤖 Github
  - GEOSITE,google,🚀 节点选择
  - GEOSITE,telegram,🚀 节点选择
  - GEOSITE,CN,🇨🇳 国内
  - GEOSITE,geolocation-!cn,🧱 国外
  - GEOIP,google,🚀 节点选择
  - GEOIP,telegram,🚀 节点选择
  - GEOIP,CN,🇨🇳 国内
  - MATCH,🧱 国外
`

const proxyGroupsFullTemplate = `
proxy-groups:
  - {name: 🚀 节点选择, type: select, proxies: [♻️ 自动选择, 👉 手动选择, 🇭🇰 香港节点, 🇹🇼 台湾节点, 🇯🇵 日本节点, 🇸🇬 新加坡节点, 🇺🇸 美国节点]}
  - {name: 👉 手动选择, type: select, include-all: true}
  - {name: 📈 网络测试, type: select, proxies: [🎯 全球直连, 🚀 节点选择, 🇭🇰 香港节点, 🇹🇼 台湾节点, 🇯🇵 日本节点, 🇸🇬 新加坡节点, 🇺🇸 美国节点]}
  - {name: 🕹️ 游戏平台, type: select, proxies: [🚀 节点选择, 🇭🇰 香港节点, 🇹🇼 台湾节点, 🇯🇵 日本节点, 🇸🇬 新加坡节点, 🇺🇸 美国节点]}
  - {name: 🤖 AI 平台, type: select, proxies: [🚀 节点选择, 🇭🇰 香港节点, 🇹🇼 台湾节点, 🇯🇵 日本节点, 🇸🇬 新加坡节点, 🇺🇸 美国节点]}
  - {name: 🎮 游戏服务, type: select, proxies: [🎯 全球直连, 🚀 节点选择]}
  - {name: 🪟 微软服务, type: select, proxies: [🎯 全球直连, 🚀 节点选择]}
  - {name: 🇬 谷歌服务, type: select, proxies: [🎯 全球直连, 🚀 节点选择]}
  - {name: 🍎 苹果服务, type: select, proxies: [🎯 全球直连, 🚀 节点选择]}
  - {name: 🎥 奈飞视频, type: select, proxies: [🚀 节点选择, 🇭🇰 香港节点, 🇹🇼 台湾节点, 🇯🇵 日本节点, 🇸🇬 新加坡节点, 🇺🇸 美国节点]}
  - {name: 📽️ 迪士尼+, type: select, proxies: [🚀 节点选择, 🇭🇰 香港节点, 🇹🇼 台湾节点, 🇯🇵 日本节点, 🇸🇬 新加坡节点, 🇺🇸 美国节点]}
  - {name: 🎞️ Max, type: select, proxies: [🚀 节点选择, 🇭🇰 香港节点, 🇹🇼 台湾节点, 🇯🇵 日本节点, 🇸🇬 新加坡节点, 🇺🇸 美国节点]}
  - {name: 🎬 Prime Video, type: select, proxies: [🚀 节点选择, 🇭🇰 香港节点, 🇹🇼 台湾节点, 🇯🇵 日本节点, 🇸🇬 新加坡节点, 🇺🇸 美国节点]}
  - {name: 🍎 Apple TV+, type: select, proxies: [🚀 节点选择, 🇭🇰 香港节点, 🇹🇼 台湾节点, 🇯🇵 日本节点, 🇸🇬 新加坡节点, 🇺🇸 美国节点]}
  - {name: 📹 油管视频, type: select, proxies: [🚀 节点选择, 🇭🇰 香港节点, 🇹🇼 台湾节点, 🇯🇵 日本节点, 🇸🇬 新加坡节点, 🇺🇸 美国节点]}
  - {name: 🎵 TikTok, type: select, proxies: [🚀 节点选择, 🇭🇰 香港节点, 🇹🇼 台湾节点, 🇯🇵 日本节点, 🇸🇬 新加坡节点, 🇺🇸 美国节点]}
  - {name: 📺 哔哩哔哩, type: select, proxies: [🎯 全球直连, 🚀 节点选择, 🇭🇰 香港节点, 🇹🇼 台湾节点, 🇯🇵 日本节点, 🇸🇬 新加坡节点, 🇺🇸 美国节点]}
  - {name: 🎶 Spotify, type: select, proxies: [🚀 节点选择, 🇭🇰 香港节点, 🇹🇼 台湾节点, 🇯🇵 日本节点, 🇸🇬 新加坡节点, 🇺🇸 美国节点]}
  - {name: 🌍 国外媒体, type: select, proxies: [🚀 节点选择, 🇭🇰 香港节点, 🇹🇼 台湾节点, 🇯🇵 日本节点, 🇸🇬 新加坡节点, 🇺🇸 美国节点]}
  - {name: 📋 Trackerslist, type: select, proxies: [🎯 全球直连, 🚀 节点选择]}
  - {name: 🇨🇳 国内域名, type: select, proxies: [🎯 全球直连, 🚀 节点选择]}
  - {name: 🀄️ 国内 IP, type: select, proxies: [🎯 全球直连, 🚀 节点选择]}
  - {name: 🌎 国外顶级域名, type: select, proxies: [🚀 节点选择, 🎯 全球直连]}
  - {name: 🌎 国外域名, type: select, proxies: [🚀 节点选择, 🎯 全球直连]}
  - {name: 📲 电报消息, type: select, proxies: [🚀 节点选择, 🇭🇰 香港节点, 🇹🇼 台湾节点, 🇯🇵 日本节点, 🇸🇬 新加坡节点, 🇺🇸 美国节点]}
  - {name: ⬇️ 直连软件, type: select, proxies: [🎯 全球直连], hidden: true}
  - {name: 🔒 私有网络, type: select, proxies: [🎯 全球直连], hidden: true}
  - {name: 🐟 漏网之鱼, type: select, proxies: [🚀 节点选择, 🇭🇰 香港节点, 🇹🇼 台湾节点, 🇯🇵 日本节点, 🇸🇬 新加坡节点, 🇺🇸 美国节点, 🎯 全球直连]}
  - {name: 🛑 广告域名, type: select, proxies: [🔴 全球拦截, 🟢 全球绕过]}
  - {name: 🔴 全球拦截, type: select, proxies: [REJECT], hidden: true}
  - {name: 🟢 全球绕过, type: select, proxies: [PASS], hidden: true}
  - {name: 🎯 全球直连, type: select, proxies: [DIRECT], hidden: true}

  - {name: 🇭🇰 香港节点, type: url-test, tolerance: 50, use: [__SUB_NAMES__], filter: "(?i)(🇭🇰|港|hk|hongkong|hong kong)"}
  - {name: 🇹🇼 台湾节点, type: url-test, tolerance: 50, use: [__SUB_NAMES__], filter: "(?i)(🇹🇼|台|tw|taiwan|tai wan)"}
  - {name: 🇯🇵 日本节点, type: url-test, tolerance: 50, use: [__SUB_NAMES__], filter: "(?i)(🇯🇵|日|jp|japan)"}
  - {name: 🇸🇬 新加坡节点, type: url-test, tolerance: 50, use: [__SUB_NAMES__], filter: "(?i)(🇸🇬|新|sg|singapore)"}
  - {name: 🇺🇸 美国节点, type: url-test, tolerance: 100, use: [__SUB_NAMES__], filter: "(?i)(🇺🇸|美|us|unitedstates|united states)"}
  - {name: ♻️ 自动选择, type: url-test, tolerance: 100, include-all: true}
`

const ruleProvidersFull = `
rule-providers:
  fakeip-filter:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/fakeip-filter.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/fakeip-filter.mrs"
    interval: 86400

  ads:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/ads.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/ads.mrs"
    interval: 86400

  private:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/private.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/private.mrs"
    interval: 86400

  trackerslist:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/trackerslist.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/trackerslist.mrs"
    interval: 86400

  applications:
    type: http
    behavior: classical
    format: text
    path: ./ruleset/applications.list
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/applications.list"
    interval: 86400

  microsoft-cn:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/microsoft-cn.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/microsoft-cn.mrs"
    interval: 86400

  apple-cn:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/apple-cn.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/apple-cn.mrs"
    interval: 86400

  google-cn:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/google-cn.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/google-cn.mrs"
    interval: 86400

  games-cn:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/games-cn.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/games-cn.mrs"
    interval: 86400

  games:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/games.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/games.mrs"
    interval: 86400

  netflix:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/netflix.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/netflix.mrs"
    interval: 86400

  disney:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/disney.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/disney.mrs"
    interval: 86400

  max:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/max.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/max.mrs"
    interval: 86400

  primevideo:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/primevideo.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/primevideo.mrs"
    interval: 86400

  appletv:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/appletv.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/appletv.mrs"
    interval: 86400

  youtube:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/youtube.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/youtube.mrs"
    interval: 86400

  tiktok:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/tiktok.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/tiktok.mrs"
    interval: 86400

  bilibili:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/bilibili.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/bilibili.mrs"
    interval: 86400

  spotify:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/spotify.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/spotify.mrs"
    interval: 86400

  media:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/media.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/media.mrs"
    interval: 86400

  ai:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/ai.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/ai.mrs"
    interval: 86400

  networktest:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/networktest.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/networktest.mrs"
    interval: 86400

  tld-proxy:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/tld-proxy.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/tld-proxy.mrs"
    interval: 86400

  gfw:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/gfw.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/gfw.mrs"
    interval: 86400

  proxy:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/proxy.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/proxy.mrs"
    interval: 86400

  cn:
    type: http
    behavior: domain
    format: mrs
    path: ./ruleset/cn.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/cn.mrs"
    interval: 86400

  privateip:
    type: http
    behavior: ipcidr
    format: mrs
    path: ./ruleset/privateip.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/privateip.mrs"
    interval: 86400

  cnip:
    type: http
    behavior: ipcidr
    format: mrs
    path: ./ruleset/cnip.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/cnip.mrs"
    interval: 86400

  telegramip:
    type: http
    behavior: ipcidr
    format: mrs
    path: ./ruleset/telegramip.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/telegramip.mrs"
    interval: 86400

  netflixip:
    type: http
    behavior: ipcidr
    format: mrs
    path: ./ruleset/netflixip.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/netflixip.mrs"
    interval: 86400

  mediaip:
    type: http
    behavior: ipcidr
    format: mrs
    path: ./ruleset/mediaip.mrs
    url: "https://github.com/DustinWin/ruleset_geodata/releases/download/mihomo-ruleset/mediaip.mrs"
    interval: 86400
`

const rulesFull = `
rules:
  - RULE-SET,private,🔒 私有网络
  - RULE-SET,ads,🛑 广告域名
  - RULE-SET,trackerslist,📋 Trackerslist
  - RULE-SET,applications,⬇️ 直连软件
  - RULE-SET,microsoft-cn,🪟 微软服务
  - RULE-SET,apple-cn,🍎 苹果服务
  - RULE-SET,google-cn,🇬 谷歌服务
  - RULE-SET,games-cn,🎮 游戏服务
  - RULE-SET,games,🕹️ 游戏平台
  - RULE-SET,netflix,🎥 奈飞视频
  - RULE-SET,disney,📽️ 迪士尼+
  - RULE-SET,max,🎞️ Max
  - RULE-SET,primevideo,🎬 Prime Video
  - RULE-SET,appletv,🍎 Apple TV+
  - RULE-SET,youtube,📹 油管视频
  - RULE-SET,tiktok,🎵 TikTok
  - RULE-SET,bilibili,📺 哔哩哔哩
  - RULE-SET,spotify,🎶 Spotify
  - RULE-SET,media,🌍 国外媒体
  - RULE-SET,ai,🤖 AI 平台
  - RULE-SET,networktest,📈 网络测试
  - RULE-SET,tld-proxy,🌎 国外顶级域名
  - RULE-SET,gfw,🌎 国外域名
  - RULE-SET,proxy,🌎 国外域名
  - RULE-SET,cn,🇨🇳 国内域名
  - RULE-SET,privateip,🔒 私有网络,no-resolve
  - RULE-SET,cnip,🀄️ 国内 IP
  - RULE-SET,telegramip,📲 电报消息,no-resolve
  - RULE-SET,netflixip,🎥 奈飞视频
  - RULE-SET,mediaip,🌍 国外媒体
  - MATCH,🐟 漏网之鱼
`

const dnsBlock = `
dns:
  enable: true
  listen: 0.0.0.0:1053
  prefer-h3: true
  ipv6: true
  use-hosts: true
  respect-rules: true
  default-nameserver:
    - https://223.5.5.5/dns-query
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  fake-ip-filter:
    - '*.lan'
    - '*.local'
    - '*.localhost'
    - localhost.ptlogin2.qq.com
    - '+.stun.*.*'
    - '+.stun.*.*.*'
    - '+.stun.*.*.*.*'
    - lens.l.google.com
    - '*.srv.nintendo.net'
    - +.stun.playstation.net
    - 'xbox.*.*.microsoft.com'
    - '*.*.xboxlive.com'
    - +.msftncsi.com
    - +.msftconnecttest.com
  nameserver:
    - https://120.53.53.53/dns-query
    - https://223.5.5.5/dns-query
  proxy-server-nameserver:
    - https://120.53.53.53/dns-query
    - https://223.5.5.5/dns-query
`