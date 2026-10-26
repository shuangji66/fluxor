package configgen

import (
	"slices"
	"testing"

	"fluxor/internal/configcheck"

	"gopkg.in/yaml.v3"
)

// TestTunBlockIsSafeBaseline 钉住 config.yaml 里 TUN 基线的几个安全性质。
//
// 这个基线是「用户在第三方面板里直接开 TUN」时的唯一兜底，任何一条被改回去都会
// 变成线上事故：enable: true 会让 NAS 开机就接管网络；inet6-address 非空会在
// disable_ipv6 宿主上让整只 TUN 起不来（EACCES）。
func TestTunBlockIsSafeBaseline(t *testing.T) {
	var wrapper struct {
		Tun struct {
			Enable       bool     `yaml:"enable"`
			Stack        string   `yaml:"stack"`
			DNSMode      string   `yaml:"dns-mode"`
			Inet6Address []string `yaml:"inet6-address"`
			Inet6Legacy  []string `yaml:"inet6_address"`
			DNSHijack    []string `yaml:"dns-hijack"`

			AutoRoute           bool `yaml:"auto-route"`
			AutoRedirect        bool `yaml:"auto-redirect"`
			AutoDetectInterface bool `yaml:"auto-detect-interface"`
		} `yaml:"tun"`
	}
	if err := yaml.Unmarshal([]byte(TunBlock), &wrapper); err != nil {
		t.Fatalf("TunBlock 不是合法 YAML: %v", err)
	}

	tun := wrapper.Tun
	if tun.Enable {
		t.Error("tun.enable 必须为 false：任何启动都不得自动接管宿主网络")
	}
	if tun.Stack != TunDefaultStack {
		t.Errorf("tun.stack = %q，期望默认值 %q", tun.Stack, TunDefaultStack)
	}
	if tun.DNSMode != TunDefaultDNSMode {
		t.Errorf("tun.dns-mode = %q，期望默认值 %q", tun.DNSMode, TunDefaultDNSMode)
	}
	if len(tun.Inet6Address) != 0 {
		t.Errorf("tun.inet6-address = %v，基线不得下发 IPv6 地址", tun.Inet6Address)
	}
	if tun.Inet6Legacy != nil {
		t.Error("TunBlock 里仍有旧的 inet6_address（下划线）键——mihomo 不认它")
	}
	if !tun.AutoRoute || !tun.AutoRedirect || !tun.AutoDetectInterface {
		t.Error("auto-route / auto-redirect / auto-detect-interface 必须为 true")
	}
	if len(tun.DNSHijack) == 0 {
		t.Error("dns-hijack 不得为空")
	}
	// 基线里的 dns-hijack 与开启时下发的 TunDNSHijack 必须是同一份取值：
	// 前者决定「不走 /config/tun/state 就开 TUN」的行为，后者决定面板里开启的行为，
	// 两处漂移就是同一功能两套语义。
	if !slices.Equal(tun.DNSHijack, TunDNSHijack) {
		t.Errorf("tun.dns-hijack = %v，与 TunDNSHijack %v 不一致", tun.DNSHijack, TunDNSHijack)
	}
}

// TestConfigTemplateCarriesTunBaseline 保证模板骨架里带着 TUN 基线，
// 且不写 ipv6（该键由 applyFluxorFields 按设置注入，两处真相会漂移）。
func TestConfigTemplateCarriesTunBaseline(t *testing.T) {
	doc, err := configcheck.ParseDoc([]byte(configTemplate))
	if err != nil {
		t.Fatalf("configTemplate 解析失败: %v", err)
	}
	if doc.Get("tun") == nil {
		t.Error("configTemplate 缺少 tun 基线块")
	}
	if doc.Get("ipv6") != nil {
		t.Error("configTemplate 不应写死 ipv6（由 applyFluxorFields 按设置注入）")
	}
}

// TestParseTunBlock 保证切换模式的整块替换能拿到 tun 节点。
func TestParseTunBlock(t *testing.T) {
	node, err := ParseTunBlock()
	if err != nil {
		t.Fatalf("ParseTunBlock: %v", err)
	}
	if node.Kind != yaml.MappingNode {
		t.Fatalf("tun 节点类型 = %v，期望 mapping", node.Kind)
	}
	if got := mappingValue(node, "enable"); got == nil || got.Value != "false" {
		t.Errorf("tun.enable 节点 = %v，期望 false", got)
	}
}

// TestParseDNSBlockIPv6 保证 dns.ipv6 能被设置覆盖（生成期字段，两档都要对）。
func TestParseDNSBlockIPv6(t *testing.T) {
	for _, want := range []bool{true, false} {
		node, err := ParseDNSBlock(want)
		if err != nil {
			t.Fatalf("ParseDNSBlock(%v): %v", want, err)
		}
		got := mappingValue(node, "ipv6")
		if got == nil {
			t.Fatalf("ParseDNSBlock(%v): 缺少 dns.ipv6", want)
		}
		if (got.Value == "true") != want {
			t.Errorf("dns.ipv6 = %q，期望 %v", got.Value, want)
		}
	}
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}
