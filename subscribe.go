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
	ProxyPort     int            `json:"proxy_port"`
	PanelPort     int            `json:"panel_port"`
	PanelSecret   string         `json:"panel_secret"`
	RuleGroup     string         `json:"rule_group"`
	PrefixSwitch  bool           `json:"prefix_switch"`
	Subscriptions []Subscription `json:"subscriptions"`
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

// loadSubscribeConfig 从文件加载订阅配置（启动时调用）
func loadSubscribeConfig() {
	subscribeMu.Lock()
	defer subscribeMu.Unlock()
	data, err := os.ReadFile(subscribeConfigFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("读取订阅配置失败: %v", err)
		}
		return
	}
	if err := json.Unmarshal(data, &subscribeConfig); err != nil {
		log.Printf("解析订阅配置失败: %v", err)
	}
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

	// 删除旧配置文件（若存在）
	if _, err := os.Stat(configTarget); err == nil {
		if err := os.Remove(configTarget); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "删除旧配置文件失败: "+err.Error())
			return
		}
	}

	// 持久化新配置
	subscribeMu.Lock()
	subscribeConfig = cfg
	subscribeMu.Unlock()
	if err := saveSubscribeConfig(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "保存配置失败: "+err.Error())
		return
	}

	// 生成配置文件
	if err := generateConfig(cfg); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "生成配置文件失败: "+err.Error())
		return
	}

	// 重载内核（热重启）
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
	// 无订阅时生成基本配置
	if len(cfg.Subscriptions) == 0 {
		basic := `mixed-port: 7790
allow-lan: true
mode: rule
log-level: silent
external-controller-unix: '/var/apps/Fluxor/target/core.sock'
external-controller: '0.0.0.0:9090'
`
		dir := filepath.Dir(configTarget)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录失败: %w", err)
		}
		return os.WriteFile(configTarget, []byte(basic), 0644)
	}

	// 生成 proxy-providers 块
	var providersBuf strings.Builder
	for i, sub := range cfg.Subscriptions {
		interval := sub.UpdateInterval
		if interval <= 0 {
			interval = 3600
		}
		health := sub.HealthInterval
		if health <= 0 {
			health = 300
		}
		prefix := ""
		if cfg.PrefixSwitch {
			prefix = sub.Prefix
		}
		providersBuf.WriteString(fmt.Sprintf(`  %s:
    type: http
    url: "%s"
    interval: %d
    health-check:
      enable: true
      url: "https://www.gstatic.com/generate_204"
      interval: %d
    override:
      additional-prefix: "%s"
`, sub.Name, sub.URL, interval, health, prefix))
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

	// 自动修正模板中的 proxy-providers 占位符（顶格无冒号）
	fixedTpl := regexp.MustCompile(`(?m)^[ \t]*\$\{PROXY_PROVIDERS\}:?[ \t]*`).ReplaceAllString(string(tplContent), "${PROXY_PROVIDERS}")

	replacer := strings.NewReplacer(
		"${PROXY_PORT}", fmt.Sprintf("%d", cfg.ProxyPort),
		"${UI_PORT}", fmt.Sprintf("%d", cfg.PanelPort),
		"${UI_PASSWORD}", cfg.PanelSecret,
		"${SUB_NAME}", strings.Join(subNames(cfg.Subscriptions), ","),
		"${PROXY_PROVIDERS}", providersBuf.String(),
	)
	configContent := replacer.Replace(fixedTpl)

	// 清理可能残留的 external-ui 行
	reExternal := regexp.MustCompile(`(?m)^\s*external-ui:.*\n?`)
	configContent = reExternal.ReplaceAllString(configContent, "")
	reExternalURL := regexp.MustCompile(`(?m)^\s*external-ui-url:.*\n?`)
	configContent = reExternalURL.ReplaceAllString(configContent, "")

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