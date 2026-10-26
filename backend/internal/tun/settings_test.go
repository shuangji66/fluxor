package tun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fluxor/internal/config"
)

// setupStore 把 tun.json 指到临时目录，返回文件路径。
func setupStore(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tun.json")
	prev := config.FluxorTunFile
	config.FluxorTunFile = path
	t.Cleanup(func() { config.FluxorTunFile = prev })
	Load()
	return path
}

// TestCurrentDefaults 保证没有 tun.json 时生效值就是模板默认值。
func TestCurrentDefaults(t *testing.T) {
	setupStore(t)

	got := Current()
	want := Defaults()
	if got != want {
		t.Fatalf("Current() = %+v，期望默认值 %+v", got, want)
	}
}

// TestLoadSanitizesInvalidFile 保证手改坏的 tun.json 不会把非法值带进内核。
//
// 读盘后必须归一化：若不收敛，Current() 会原样返回非法栈名，开启 TUN 时直接下发，
// 用户只会看到内核的 "invalid tun stack"；而 POST 也会被同一条非法值拒绝，等于
// 没有任何入口能改回来。非法内容整体回落默认值，文件本身不被静默改写。
func TestLoadSanitizesInvalidFile(t *testing.T) {
	path := setupStore(t)
	if err := os.WriteFile(path, []byte(`{"stack":"bogus","dns_mode":"native","device":"mihomo0"}`), 0644); err != nil {
		t.Fatalf("写入非法 tun.json: %v", err)
	}
	Load()

	if got, want := Current(), Defaults(); got != want {
		t.Fatalf("Current() = %+v，期望回落默认值 %+v", got, want)
	}
	if tunStore.Damaged() {
		t.Error("非法取值不是文件损坏，不应阻止后续写入")
	}
	if raw, err := os.ReadFile(path); err != nil || !strings.Contains(string(raw), "bogus") {
		t.Fatalf("读盘归一化不应改写文件: raw=%q err=%v", raw, err)
	}
}

// TestSaveSparse 保证只存与默认值不同的键（改回默认即删键）。
func TestSaveSparse(t *testing.T) {
	path := setupStore(t)

	if _, err := Save(Settings{
		Stack:   "system",
		Device:  "",
		DNSMode: "native",
		IPv6:    "auto", // off 是默认值，只有偏离默认的取值才会落盘
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 tun.json: %v", err)
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("tun.json 不是合法 JSON: %v", err)
	}
	if stored["stack"] != "system" || stored["dns_mode"] != "native" || stored["ipv6"] != "auto" {
		t.Fatalf("落盘内容 = %v", stored)
	}
	if _, ok := stored["device"]; ok {
		t.Errorf("device 与默认值相同（空串），不应落盘: %v", stored)
	}

	// 改回默认值：所有键都应从文件里消失
	if _, err := Save(Defaults()); err != nil {
		t.Fatalf("Save(defaults): %v", err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 tun.json: %v", err)
	}
	if strings.TrimSpace(string(raw)) != "{}" {
		t.Errorf("改回默认值后 tun.json = %s，期望 {}（稀疏存储）", strings.TrimSpace(string(raw)))
	}
}

// TestNormalizeRejects 保证非法取值在落库前被拒（mihomo 侧要到建 TUN 时才报错）。
func TestNormalizeRejects(t *testing.T) {
	cases := []struct {
		name string
		in   Settings
		want string
	}{
		{"stack", Settings{Stack: "tun", DNSMode: "hijack", IPv6: "auto"}, "TUN 栈"},
		{"dns-mode", Settings{Stack: "mixed", DNSMode: "on", IPv6: "auto"}, "DNS 处理方式"},
		{"ipv6", Settings{Stack: "mixed", DNSMode: "hijack", IPv6: "yes"}, "IPv6 接管方式"},
		{"device", Settings{Stack: "mixed", DNSMode: "hijack", IPv6: "auto", Device: "bad name/1"}, "设备名"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Normalize(tc.in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Normalize(%+v) err = %v，期望包含 %q", tc.in, err, tc.want)
			}
		})
	}
}

// TestNormalizeLowercases 保证大小写与空白被归一化（界面/curl 都可能传大写）。
func TestNormalizeLowercases(t *testing.T) {
	got, err := Normalize(Settings{
		Stack:   " System ",
		Device:  " mihomo0 ",
		DNSMode: "NATIVE",
		IPv6:    "OFF",
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	want := Settings{Stack: "system", Device: "mihomo0", DNSMode: "native", IPv6: "off"}
	if got != want {
		t.Fatalf("Normalize = %+v，期望 %+v", got, want)
	}
}

// TestRuntimeConfig 保证开启时下发的是完整 tun 对象（缺字段会回落到 config.yaml 基线）。
func TestRuntimeConfig(t *testing.T) {
	cfg := runtimeConfig(Settings{Stack: "mixed", Device: "", DNSMode: "hijack", IPv6: "auto"}, false)

	if cfg["enable"] != true {
		t.Error("enable 必须为 true")
	}
	for _, key := range []string{"stack", "device", "dns-mode", "inet6-address", "dns-hijack", "auto-route", "auto-redirect", "auto-detect-interface"} {
		if _, ok := cfg[key]; !ok {
			t.Errorf("缺少字段 %q（键缺失 = 沿用 LastTunConf，而不是回到默认）", key)
		}
	}
	if got := cfg["inet6-address"].([]string); len(got) != 0 {
		t.Errorf("inet6-address = %v，期望空（不接管 IPv6）", got)
	}
	// 空 device 必须显式下发：patchConfigs 以 LastTunConf 为基底合并，省略该键会
	// 沿用上一次的接口名，用户清空设备名后重新开启 TUN 不会生效。
	if got := cfg["device"]; got != "" {
		t.Errorf("device = %v，期望空串（自动命名也必须显式下发）", got)
	}

	withV6 := runtimeConfig(Settings{Stack: "mixed", DNSMode: "hijack", IPv6: "auto", Device: "mihomo0"}, true)
	if got := withV6["inet6-address"].([]string); len(got) != 1 {
		t.Errorf("接管 IPv6 时 inet6-address = %v，期望一个地址", got)
	}
	if withV6["device"] != "mihomo0" {
		t.Errorf("device = %v，期望 mihomo0", withV6["device"])
	}
}

// TestNormalizeLegacyForceOn 兼容旧值：已取消的 "on"（强制开启）收敛为 auto。
func TestNormalizeLegacyForceOn(t *testing.T) {
	got, err := Normalize(Settings{Stack: "mixed", DNSMode: "hijack", IPv6: "on"})
	if err != nil {
		t.Fatalf("Normalize(on): %v", err)
	}
	if got.IPv6 != "auto" {
		t.Errorf("IPv6 = %q，期望 auto（旧值 on 已取消）", got.IPv6)
	}
}
