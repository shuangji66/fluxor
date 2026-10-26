package configgen

import (
	"fluxor/internal/config"
	"fluxor/internal/configcheck"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// GenerateConfig 根据订阅配置生成 config.yaml（融合模式）。
//
// 生成策略：模板负责提供基础骨架与规则集，随后用 YAML 解析改写需要按用户配置
// 覆盖的字段。不采用字符串拼接的原因：订阅名等用户输入可能含 `:`, `,`, `]`
// 等 YAML 特殊字符，插值会产出内核无法加载的配置。
func GenerateConfig(cfg config.SubscribeConfig) error {
	// 无订阅时退化为基础配置。
	//
	// 不能继续走「模板 + 规则集」：full 规则集的地区组以 include-all-providers 引用
	// 全部订阅 provider（groups_full.go），无订阅时这样的组既无 use 也无 proxies，内核
	// 直接拒绝加载（proxy group：`use` or `proxies` missing），使「删除最后一个订阅并
	// 保存」后重载必然失败。此时也不应再下发规则集——没有节点可供规则分流。
	if len(cfg.Subscriptions) == 0 {
		return GenerateBaseConfig(cfg)
	}

	providers, err := buildProviders(cfg)
	if err != nil {
		return err
	}

	doc, err := buildBaseDocument(cfg, providers)
	if err != nil {
		return err
	}

	// 按规则集追加动态块（rule-providers -> proxy-groups -> rules）
	if err := appendRuleSet(doc, cfg, cfg.RuleGroup); err != nil {
		return err
	}

	// 融合模式的自定义规则：必须在该档位的代理组就位后注入，否则目标校验看不到组。
	// 只取当前档位那一份——另一档保持惰性，等切档后再生效。
	if err := applyCustomRules(doc, cfg.RuleGroup, cfg.MergeCustomRulesFor(cfg.RuleGroup)); err != nil {
		return err
	}

	// 流量隧道：同样只取当前档位那一份；必须在代理组/代理就位后注入（proxy 要能解析）。
	// 位置由 ApplyTunnels 保证写在 rule-providers / rules 之前。
	if err := applyTunnels(doc, cfg.RuleGroup, cfg.MergeTunnelsFor(cfg.RuleGroup)); err != nil {
		return err
	}

	// 替换 DNS 块
	if err := applyDNSBlock(doc, cfg.DNSIPv6); err != nil {
		return err
	}

	return writeConfigTarget(doc)
}

// GenerateBaseConfig 生成基础配置（用于无订阅时）。
//
// 字段集合与 patch.go 给订阅文件注入的「Fluxor 必需字段」保持一致：
// 其中 tproxy-port 必须保留——TProxy 规则会把流量重定向到该端口，若基础配置
// 不声明该端口，内核不再监听，已启用的 TProxy 将把流量导入黑洞。
func GenerateBaseConfig(cfg config.SubscribeConfig) error {
	doc := configcheck.NewDoc()

	fields := []struct {
		key   string
		value any
	}{
		{"mixed-port", cfg.ProxyPort},
		{"tproxy-port", cfg.TproxyPort},
		{"allow-lan", true},
		{"mode", "rule"},
		{"log-level", "silent"},
		{"ipv6", cfg.IPv6},
		{"external-controller-unix", config.CoreSocket},
		{"external-controller", fmt.Sprintf("0.0.0.0:%d", cfg.PanelPort)},
		{"external-ui", uiPath(cfg.UIPanel)},
	}
	if cfg.PanelSecret != "" {
		fields = append(fields, struct {
			key   string
			value any
		}{"secret", cfg.PanelSecret})
	}
	if cfg.UIPanel == "zashboard" {
		fields = append(fields, struct {
			key   string
			value any
		}{"external-ui-url", zashboardUIURL})
	}

	for _, f := range fields {
		if err := doc.Set(f.key, f.value); err != nil {
			return fmt.Errorf("写入字段 %s 失败: %w", f.key, err)
		}
	}

	// TUN 基线：无订阅的兜底配置同样要带上。缺了它内核会退回 DefaultRawConfig 的
	// tun 默认值（含 fdfe:dcba:9876::1/126），从第三方面板开 TUN 会直接踩回
	// 「disable_ipv6 宿主 + EACCES」那个坑。
	tunNode, err := ParseTunBlock()
	if err != nil {
		return err
	}
	doc.SetNode("tun", tunNode)

	// 流量隧道：无订阅的兜底配置同样要下发（融合模式删掉全部订阅后，隧道不该无声消失）。
	// 只取融合模式当前档位那一份——切换模式下隧道挂在订阅上，没有订阅就没有作用域。
	// 基础配置里没有代理组，因此引用代理组的隧道会被 ApplyTunnels 依据本文件的实际
	// 目标判为失效并跳过：少一条隧道远优于内核拒绝加载整份配置。
	if cfg.Mode == config.ModeMerge {
		if err := applyTunnels(doc, cfg.RuleGroup, cfg.MergeTunnelsFor(cfg.RuleGroup)); err != nil {
			return err
		}
	}
	return writeConfigTarget(doc)
}

// buildBaseDocument 以模板为骨架，注入 Fluxor 的监听参数与 proxy-providers。
func buildBaseDocument(cfg config.SubscribeConfig, providers *yaml.Node) (*configcheck.Doc, error) {
	doc, err := configcheck.ParseDoc([]byte(configTemplate))
	if err != nil {
		return nil, fmt.Errorf("解析配置模板失败: %w", err)
	}

	if err := applyFluxorFields(doc, cfg); err != nil {
		return nil, err
	}

	if providers != nil {
		doc.SetNode("proxy-providers", providers)
	}
	return doc, nil
}

// applyFluxorFields 把 Fluxor 的监听与运行参数写进文档（融合模式与自定义模式共用）。
func applyFluxorFields(doc *configcheck.Doc, cfg config.SubscribeConfig) error {
	fields := []struct {
		key   string
		value any
	}{
		{"mixed-port", cfg.ProxyPort},
		{"tproxy-port", cfg.TproxyPort},
		{"external-controller", fmt.Sprintf("0.0.0.0:%d", cfg.PanelPort)},
		// 内核的 Unix Socket 是 Fluxor 与内核之间的唯一 HTTP 通道（见 config.CoreSocket），
		// 必须按运行时实际路径下发：模板里的字面量只是 fnos 默认值，openwrt 模式或
		// 用环境变量改过路径时，照抄模板会让内核把 socket 建在别处，面板与内核彻底失联。
		{"external-controller-unix", config.CoreSocket},
		{"secret", cfg.PanelSecret},
		{"external-ui", uiPath(cfg.UIPanel)},
		// IPv6 总开关：生成期设置（订阅中心），决定内核自身解析/出站是否使用 IPv6。
		// 模板里不写这个键，避免与设置两处真相。
		{"ipv6", cfg.IPv6},
	}
	if cfg.UIPanel == "zashboard" {
		fields = append(fields, struct {
			key   string
			value any
		}{"external-ui-url", zashboardUIURL})
	}

	for _, f := range fields {
		if err := doc.Set(f.key, f.value); err != nil {
			return fmt.Errorf("写入字段 %s 失败: %w", f.key, err)
		}
	}
	return nil
}

// buildProviders 构建 proxy-providers 映射节点。无订阅时返回 nil。
//
// 订阅名作为 YAML 键由节点承载，避免含 `:` 时破坏映射语法。
func buildProviders(cfg config.SubscribeConfig) (*yaml.Node, error) {
	if len(cfg.Subscriptions) == 0 {
		return nil, nil
	}

	providers := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, sub := range cfg.Subscriptions {
		interval := sub.UpdateInterval
		if interval <= 0 {
			interval = 86400
		}
		health := sub.HealthInterval
		if health <= 0 {
			health = 300
		}

		provider := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		appendMap(provider, "type", "http")
		appendMap(provider, "url", sub.URL)
		appendMap(provider, "interval", interval)
		appendMap(provider, "path", "proxies/"+config.SanitizeSubscriptionFileName(sub.Name))

		healthCheck := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		appendMap(healthCheck, "enable", true)
		appendMap(healthCheck, "url", "https://www.gstatic.com/generate_204")
		appendMap(healthCheck, "interval", health)
		appendMapNode(provider, "health-check", healthCheck)

		if sub.Prefix != "" {
			override := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			appendMap(override, "additional-prefix", sub.Prefix)
			appendMapNode(provider, "override", override)
		}

		nameNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: sub.Name}
		providers.Content = append(providers.Content, nameNode, provider)
	}
	return providers, nil
}

// appendRuleSet 按规则集档位追加 rule-providers / proxy-groups / rules 三个顶层块。
//
// 档位由参数传入而不是读 cfg.RuleGroup：自定义模式的规则集固定用标准档位，
// 但它的自定义规则存在另一个作用域（见 GenerateCustomConfig）。
func appendRuleSet(doc *configcheck.Doc, cfg config.SubscribeConfig, ruleGroup string) error {
	switch ruleGroup {
	case "base":
		if err := mergeBlock(doc, proxyGroupsBase); err != nil {
			return err
		}
		return mergeBlock(doc, rulesBase)

	case "full":
		// full 档位的地区组以 include-all-providers 引用全部订阅 provider（见
		// groups_full.go），模板里没有订阅名，因此无需按订阅做任何改写。
		//
		// 无订阅时例外：那样的组既没有 use 也没有 proxies，内核实测直接拒绝加载
		// （proxy group[N]: `use` or `proxies` missing）。GenerateConfig 已在无订阅时
		// 改走基础配置，这里再兜一次，避免将来重构让该分支重新变得可达。
		if len(cfg.Subscriptions) == 0 {
			return fmt.Errorf("full 规则集需要至少一个订阅")
		}
		// 追加顺序即产物中三个块的顺序：rule-providers -> proxy-groups -> rules
		if err := mergeBlock(doc, ruleProvidersFull); err != nil {
			return err
		}
		if err := mergeBlock(doc, proxyGroupsFullTemplate); err != nil {
			return err
		}
		return mergeBlock(doc, rulesFull)

	default:
		return fmt.Errorf("未知规则集: %s", cfg.RuleGroup)
	}
}

// mergeBlock 解析一个 YAML 块并合并进文档：同名顶层键以块内容覆盖。
func mergeBlock(doc *configcheck.Doc, block string) error {
	var parsed yaml.Node
	if err := yaml.Unmarshal([]byte(block), &parsed); err != nil {
		return fmt.Errorf("解析配置块失败: %w", err)
	}
	if len(parsed.Content) == 0 {
		return nil
	}
	blockTop := parsed.Content[0]
	if blockTop.Kind != yaml.MappingNode {
		return fmt.Errorf("解析配置块失败: 顶层应为映射")
	}
	for i := 0; i+1 < len(blockTop.Content); i += 2 {
		doc.SetNode(blockTop.Content[i].Value, blockTop.Content[i+1])
	}
	return nil
}

// applyDNSBlock 把 Fluxor 的统一 DNS 块写入文档，并按设置覆盖 dns.ipv6。
func applyDNSBlock(doc *configcheck.Doc, dnsIPv6 bool) error {
	node, err := ParseDNSBlock(dnsIPv6)
	if err != nil {
		return err
	}
	doc.SetNode("dns", node)
	return nil
}

// ParseDNSBlock 解析统一 DNS 块，并把 dns.ipv6 改成给定取值。
//
// 单独导出是给切换模式用的：那边的 config.yaml 是订阅文件副本，DNS 块整块替换，
// 但同样必须尊重「DNS 是否返回 AAAA」这个设置（订阅里自带的 dns.ipv6 不作数）。
func ParseDNSBlock(dnsIPv6 bool) (*yaml.Node, error) {
	var wrapper struct {
		DNS yaml.Node `yaml:"dns"`
	}
	if err := yaml.Unmarshal([]byte(DnsBlock), &wrapper); err != nil {
		return nil, fmt.Errorf("解析 DNS 块失败: %w", err)
	}
	if wrapper.DNS.Kind == 0 {
		return nil, fmt.Errorf("解析 DNS 块失败: 未得到 dns 节点")
	}
	setMappingValue(&wrapper.DNS, "ipv6", dnsIPv6)
	return &wrapper.DNS, nil
}

// setMappingValue 就地替换映射节点中某个键的值；键不存在时追加。
func setMappingValue(m *yaml.Node, key string, value any) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			var v yaml.Node
			_ = v.Encode(value)
			m.Content[i+1] = &v
			return
		}
	}
	appendMap(m, key, value)
}

// writeConfigTarget 序列化文档并写入内核配置路径。
func writeConfigTarget(doc *configcheck.Doc) error {
	// 顶层键序归一：标量键在前、块在后，块按固定顺序排（dns 在 proxy-providers 之前，
	// 末尾依次 proxy-providers / proxy-groups / proxies / tunnels / rule-providers / rules）。
	// 模板自身的键序不保证这一点（geodata-* 三个键就写在 tun 之后），按订阅追加的块
	// 也只会落在末尾。
	doc.OrderTopLevel()

	out, err := doc.Bytes()
	if err != nil {
		return err
	}
	dir := filepath.Dir(config.ConfigTarget)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	return os.WriteFile(config.ConfigTarget, out, 0644)
}

// uiPath 返回外部面板的静态文件目录。
func uiPath(uiPanel string) string {
	if uiPanel == "zashboard" {
		return "ui/zash"
	}
	return "ui/meta"
}

const zashboardUIURL = "https://github.com/Zephyruso/zashboard/releases/latest/download/dist-cdn-fonts.zip"

// appendMap 向映射节点追加一个标量键值对。
func appendMap(m *yaml.Node, key string, value any) {
	var v yaml.Node
	_ = v.Encode(value)
	appendMapNode(m, key, &v)
}

// appendMapNode 向映射节点追加一个键值对。
func appendMapNode(m *yaml.Node, key string, value *yaml.Node) {
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}
