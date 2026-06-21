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
)

// SubscribeConfig 订阅配置结构体（与前端 JSON 完全对应）
type SubscribeConfig struct {
	ProxyPort      int            `json:"proxy_port"`
	PanelPort      int            `json:"panel_port"`
	PanelSecret    string         `json:"panel_secret"`
	RuleGroup      string         `json:"rule_group"`
	UIPanel        string         `json:"ui_panel"`          // "metacubexd" 或 "zashboard"
	MetaBackendURL string         `json:"meta_backend_url"`  // MetaCubeXD 后端地址，空表示不修改
	Mode           string         `json:"mode"`              // "merge" 或 "switch"
	ActiveSubscription string         `json:"active_subscription"` // 新增：当前选中的订阅名称（切换模式使用）
	Subscriptions  []Subscription `json:"subscriptions"`
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
	}

	data, err := os.ReadFile(subscribeConfigFile)
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

	if tmp.ProxyPort == 0 {
		tmp.ProxyPort = defaultCfg.ProxyPort
	}
	if tmp.PanelPort == 0 {
		tmp.PanelPort = defaultCfg.PanelPort
	}
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
	subscribeMu.RLock()
	defer subscribeMu.RUnlock()
	data, err := json.MarshalIndent(subscribeConfig, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(subscribeConfigFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(subscribeConfigFile, data, 0644)
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

    // 定义待替换/添加的字段
    rules := []struct {
        key   string
        value string
    }{
        {"mixed-port", fmt.Sprintf("%d", cfg.ProxyPort)},
        {"external-controller", fmt.Sprintf("'0.0.0.0:%d'", cfg.PanelPort)},
        {"external-controller-unix", fmt.Sprintf("'%s'", coreSocket)},
        {"secret", fmt.Sprintf("'%s'", cfg.PanelSecret)},
		{"allow-lan", "true"},
        {"ipv6", "true"},
		{"unified-delay", "true"},
    }

    // 根据面板选择 external-ui
    uiPath := "ui/meta"
    if cfg.UIPanel == "zashboard" {
        uiPath = "ui/zash"
    }
    rules = append(rules, struct{ key, value string }{"external-ui", uiPath})

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
allow-lan: true
mode: rule
log-level: silent
external-controller-unix: '%s'
external-controller: '0.0.0.0:%d'
`, cfg.ProxyPort, coreSocket, cfg.PanelPort)
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

	// 选择外部模板文件
	templateFile := ""
	switch cfg.RuleGroup {
	case "lite":
		templateFile = "config_lite.yaml"
	case "base":
		templateFile = "config_base.yaml"
	case "full":
		templateFile = "config_full.yaml"
	default:
		return fmt.Errorf("未知规则集: %s", cfg.RuleGroup)
	}

	templatePath := filepath.Join(configTemplateDir, templateFile)
	tplContent, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("读取模板文件失败: %w", err)
	}
	tplStr := string(tplContent)

	// 根据面板选择设置 external-ui 和 external-ui-url
	uiSelect := "ui/meta"
	uiURL := ""
	if cfg.UIPanel == "zashboard" {
		uiSelect = "ui/zash"
		uiURL = `external-ui-url: "https://github.com/Zephyruso/zashboard/releases/latest/download/dist-cdn-fonts.zip"`
	}

	// 替换所有占位符
	replacer := strings.NewReplacer(
		"${PROXY_PORT}", fmt.Sprintf("%d", cfg.ProxyPort),
		"${UI_PORT}", fmt.Sprintf("%d", cfg.PanelPort),
		"${UI_PASSWORD}", cfg.PanelSecret,
		"${SUB_NAME}", strings.Join(subNames(cfg.Subscriptions), ","),
		"${PROXY_PROVIDERS}", providersBuf.String(),
		"${UI_SELECT}", uiSelect,
		"${UI_URL}", uiURL,
	)
	configContent := replacer.Replace(tplStr)

	// ----- 确保 proxy-providers 字段存在（如果 providersBuf 非空） -----
	if providersBuf.Len() > 0 && !strings.Contains(configContent, "proxy-providers:") {
		configContent += "\nproxy-providers:\n" + providersBuf.String()
	}

	// 清理可能残留的 external-ui 行（如果 uiURL 为空，但模板中可能还有 external-ui-url 行，需清理）
	if uiURL == "" {
		reExternalURL := regexp.MustCompile(`(?m)^\s*external-ui-url:.*\n?`)
		configContent = reExternalURL.ReplaceAllString(configContent, "")
	}

	// 清理多余空行
	configContent = regexp.MustCompile(`\n{3,}`).ReplaceAllString(configContent, "\n\n")

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
                if cfg.Subscriptions[idx].SubscriptionInfo == nil || len(cfg.Subscriptions[idx].SubscriptionInfo) == 0 {
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
// 该函数从 handlers_settings.go 移入此处
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

    var needsReload bool
    var err2 error // 注意：这里不能用 err，因为 err 已被用作解码返回值
    subscribeMu.Lock()
    if subscribeConfig.Mode != "switch" {
        subscribeMu.Unlock()
        writeJSONError(w, http.StatusBadRequest, "当前非切换模式")
        return
    }
    needsReload, err2 = updateSubscriptionInSwitchMode(&subscribeConfig, name)
    subscribeMu.Unlock()
    if err2 != nil {
        writeJSONError(w, http.StatusInternalServerError, "更新失败: "+err2.Error())
        return
    }

    // 保存配置（元数据已更新）
    if err := saveSubscribeConfig(); err != nil {
        log.Printf("保存配置失败: %v", err)
    }

    // 如果需要重载，在锁外调用
    if needsReload {
        log.Printf("[UPDATE] 开始重载内核")
        if err := reloadCore(); err != nil {
            log.Printf("[UPDATE] 重载内核失败: %v", err)
        } else {
            log.Printf("[UPDATE] 重载内核成功")
        }
    }

    // 重置定时器
    stopAllTimers()
    startAllTimers()

    respondJSON(w, http.StatusOK, map[string]string{
        "status":  "ok",
        "message": "订阅 " + name + " 更新成功",
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
	return updatedAtVal, subInfoVal, nil
}

// updateAllSubscriptionsMetadata 更新所有订阅的元数据（仅用于融合模式）
func updateAllSubscriptionsMetadata(cfg *SubscribeConfig) {
	for i := range cfg.Subscriptions {
		updatedAt, subInfo, err := fetchSubscriptionMetadataFromCore(cfg.Subscriptions[i].Name)
		if err != nil {
			log.Printf("获取订阅 %s 元数据失败: %v", cfg.Subscriptions[i].Name, err)
			continue
		}
		cfg.Subscriptions[i].UpdatedAt = updatedAt
		cfg.Subscriptions[i].SubscriptionInfo = subInfo
		log.Printf("更新订阅 %s 元数据成功", cfg.Subscriptions[i].Name)
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
        Upload    int64  `json:"upload"`
        Download  int64  `json:"download"`
        Total     int64  `json:"total"`
        Expire    int64  `json:"expire"`
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
                "Upload":   payload.Upload,
                "Download": payload.Download,
                "Total":    payload.Total,
                "Expire":   payload.Expire,
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
}
// stopAllTimers 停止所有定时器
func stopAllTimers() {
    timerMu.Lock()
    defer timerMu.Unlock()
    for name, cancel := range timerCancel {
        cancel()
        delete(timerCancel, name)
    }
}