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
)

// SubscribeConfig 订阅配置结构体（与前端 JSON 完全对应）
type SubscribeConfig struct {
	ProxyPort      int            `json:"proxy_port"`
	PanelPort      int            `json:"panel_port"`
	PanelSecret    string         `json:"panel_secret"`
	RuleGroup      string         `json:"rule_group"`
	UIPanel        string         `json:"ui_panel"`          // "metacubexd" 或 "zashboard"
	MetaBackendURL string         `json:"meta_backend_url"`  // MetaCubeXD 后端地址，空表示不修改
	Subscriptions  []Subscription `json:"subscriptions"`
}

type Subscription struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	UpdateInterval int    `json:"update_interval"`
	HealthInterval int    `json:"health_interval"`
	Prefix         string `json:"prefix"`
}

var (
	subscribeConfig SubscribeConfig
	subscribeMu     sync.RWMutex
)

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

		respondJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"message": "配置已保存",
		})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// handleGenerateConfig 处理 POST /subscribe/generate （保存并应用）
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

	if err := generateConfig(cfg); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "生成配置文件失败: "+err.Error())
		return
	}

	// 如果设置了 MetaCubeXD 后端地址，则修改 config.js
	if cfg.MetaBackendURL != "" {
		if err := modifyMetaConfig(cfg.MetaBackendURL); err != nil {
			log.Printf("[WARN] 修改 MetaCubeXD 后端地址失败: %v", err)
			// 仅记录警告，不中断流程
		}
	}

	if err := reloadCore(); err != nil {
		respondJSON(w, http.StatusOK, map[string]string{
			"status":  "warning",
			"message": "配置文件已生成，但重载内核失败: " + err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "配置文件已生成并成功重载内核",
	})
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
    proxy: 节点选择
    health-check:
      enable: true
      url: "https://www.gstatic.com/generate_204"
      interval: %d
`, sub.Name, sub.URL, interval, health))

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