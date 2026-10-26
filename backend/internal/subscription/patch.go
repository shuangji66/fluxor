package subscription

import (
	"fluxor/internal/config"
	"fluxor/internal/configcheck"
	"fluxor/internal/configgen"
	"fmt"
	"os"
)

// patchSubscriptionFile 修改订阅节点文件，注入 Fluxor 必需的端口/密钥/DNS 字段。
//
// 采用 YAML 解析改写而非正则拼接：
//   - 订阅名、路径等值可能含 `:`, `,`, `]` 等 YAML 特殊字符，字符串插值会产出
//     内核无法加载的配置（正则替换同样无法正确转义值）
//   - 解析后按顶层键增删，键序与注释由 yaml.Node 保留
//
// 输入必须是合法的 Clash 配置：若对 Base64 编码的 URI 列表等非 YAML 内容改写，
// 会产出损坏文件，故校验失败时直接报错，由调用方终止流程。
func patchSubscriptionFile(filePath string, cfg config.SubscribeConfig) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	doc, err := configcheck.ParseValidatedDoc(content)
	if err != nil {
		return fmt.Errorf("订阅文件 %s 无法打补丁: %w", filePath, err)
	}

	// 清理可能冲突的顶层单端口定义，防止重复端口绑定导致内核崩溃
	for _, key := range []string{"port", "socks-port", "redir-port"} {
		doc.Delete(key)
	}

	// 注入 Fluxor 的监听与运行参数
	fields := []struct {
		key   string
		value any
	}{
		{"mixed-port", cfg.ProxyPort},
		{"tproxy-port", cfg.TproxyPort},
		{"external-controller", fmt.Sprintf("0.0.0.0:%d", cfg.PanelPort)},
		{"external-controller-unix", config.CoreSocket},
		{"secret", cfg.PanelSecret},
		{"allow-lan", true},
		{"ipv6", cfg.IPv6},
		{"unified-delay", true},
		{"geodata-mode", false},
		{"routing-mark", 255},
		{"external-ui", uiPath(cfg.UIPanel)},
	}
	if cfg.UIPanel == "zashboard" {
		fields = append(fields, struct {
			key   string
			value any
		}{"external-ui-url", zashboardUIRL})
	}

	for _, f := range fields {
		if err := doc.Set(f.key, f.value); err != nil {
			return fmt.Errorf("写入字段 %s 失败: %w", f.key, err)
		}
	}

	// 替换 DNS 块为 Fluxor 的统一配置（dns.ipv6 按设置覆盖：订阅自带的取值不作数）
	dnsNode, err := configgen.ParseDNSBlock(cfg.DNSIPv6)
	if err != nil {
		return err
	}
	doc.SetNode("dns", dnsNode)

	// TUN 块整块替换成 Fluxor 的静态安全基线。
	//
	// 不能只在缺失时补：机场自带的 tun 块与宿主无关（device、auto-route、甚至
	// enable: true 都可能出现），那会把 NAS 拖进一个由订阅决定的网络状态。
	// 用户偏好存在 tun.json，只在开启 TUN 时 PATCH，不进生成产物。
	tunNode, err := configgen.ParseTunBlock()
	if err != nil {
		return err
	}
	doc.SetNode("tun", tunNode)

	// 顶层键序归一（标量键在前、块按固定顺序排）：上面注入的键若订阅里原本没有，Set
	// 只会追加到文件末尾——也就是排在 rules 之后；机场文件自身的块序也各不相同。
	// 切换模式下的 config.yaml 是这份文件的副本，键序随之继承，因此在唯一的注入点收口，
	// 两种模式的最终产物都满足同一套顺序（只动顺序，不动内容）。
	doc.OrderTopLevel()

	out, err := doc.Bytes()
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, out, 0644)
}

// uiPath 返回外部面板的静态文件目录。
func uiPath(uiPanel string) string {
	if uiPanel == "zashboard" {
		return "ui/zash"
	}
	return "ui/meta"
}

const zashboardUIRL = "https://github.com/Zephyruso/zashboard/releases/latest/download/dist-cdn-fonts.zip"
