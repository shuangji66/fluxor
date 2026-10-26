package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMigrateLegacyFiles 旧单文件被拆成 5 个文件，原文件归档保留，且迁移幂等。
func TestMigrateLegacyFiles(t *testing.T) {
	dir := t.TempDir()
	SetDataDir(dir)
	defer capturePaths()()

	legacy := map[string]any{
		"proxy_port":          7890,
		"tproxy_port":         7898,
		"panel_port":          9090,
		"panel_secret":        "s3cret",
		"rule_group":          "full",
		"ui_panel":            "zashboard",
		"mode":                "switch",
		"active_subscription": "机场A",
		"subscriptions": []map[string]any{{
			"name":            "机场A",
			"url":             "https://example.com/a",
			"update_interval": 720,
			"prefix":          "A",
			"updated_at":      "2026-01-01T00:00:00Z",
			"subscription_info": map[string]any{
				"total": 100,
			},
			"custom_rules": []map[string]any{{
				"id": "sr1", "type": "DOMAIN", "payload": "a.test", "target": "DIRECT", "position": "before",
			}},
			"tunnels": []map[string]any{{
				"id": "st1", "network": []string{"tcp"}, "address": "127.0.0.1:1080", "target": "a.test:80",
			}},
		}},
		"custom_nodes": []map[string]any{{
			"id": "n1", "name": "手工节点", "type": "ss", "config": map[string]any{"server": "1.2.3.4"},
		}},
		"merge_custom_rules": map[string]any{
			"base": []map[string]any{{"id": "br1", "type": "DOMAIN", "payload": "b.test", "target": "DIRECT"}},
		},
		"custom_mode_rules": []map[string]any{{"id": "cr1", "type": "DOMAIN", "payload": "c.test", "target": "DIRECT"}},
		"merge_tunnels": map[string]any{
			"full": []map[string]any{{"id": "bt1", "network": []string{"udp"}, "address": "127.0.0.1:1081", "target": "b.test:80"}},
		},
		"custom_mode_tunnels": []map[string]any{{"id": "ct1", "network": []string{"tcp"}, "address": "127.0.0.1:1082", "target": "c.test:80"}},
		"tproxy_enabled":      true,
		"tproxy_proxy_local":  true, // 显式写过 → 迁移必须原样保留（缺省才是关闭）
		"tproxy_ipv6":         true,
		"tproxy_exceptions":   []string{"1.1.1.1"}, // 更早版本的合并字段 → 目的绕过
	}
	writeJSON(t, FluxorConfigFile, legacy)

	LoadAll()

	// 5 个新文件都生成，且内容按类别分开
	settings := readJSON[Settings](t, FluxorSettingsFile)
	if settings.PanelSecret != "s3cret" || settings.Mode != "switch" || settings.RuleGroup != "full" ||
		settings.ActiveSubscription != "机场A" || settings.UIPanel != "zashboard" {
		t.Fatalf("全局设置迁移不完整: %+v", settings)
	}
	// 旧文件里没有 IPv6 字段，迁移必须补成默认开启：升级前模板里 ipv6 恒为 true，
	// 写成显式 false 会让 Init 的默认值失效（键存在即被覆盖）。
	if !settings.IPv6 || !settings.DNSIPv6 {
		t.Fatalf("迁移后 IPv6 两个开关应默认开启: ipv6=%v dns_ipv6=%v", settings.IPv6, settings.DNSIPv6)
	}
	if len(settings.Subscriptions) != 1 || settings.Subscriptions[0].Prefix != "A" ||
		settings.Subscriptions[0].UpdateInterval != 720 {
		t.Fatalf("订阅注册表迁移不完整: %+v", settings.Subscriptions)
	}
	if len(settings.CustomNodes) != 1 || settings.CustomNodes[0].Name != "手工节点" {
		t.Fatalf("手工节点迁移不完整: %+v", settings.CustomNodes)
	}

	rules := readJSON[RulesFile](t, FluxorRulesFile)
	if len(rules.Merge[RuleGroupBase]) != 1 || len(rules.CustomMode) != 1 || len(rules.BySub["机场A"]) != 1 {
		t.Fatalf("规则迁移不完整: %+v", rules)
	}
	tunnels := readJSON[TunnelsFile](t, FluxorTunnelsFile)
	if len(tunnels.Merge[RuleGroupFull]) != 1 || len(tunnels.CustomMode) != 1 || len(tunnels.BySub["机场A"]) != 1 {
		t.Fatalf("隧道迁移不完整: %+v", tunnels)
	}
	meta := readJSON[MetaFile](t, FluxorMetaFile)
	if meta.Subscriptions["机场A"].UpdatedAt == "" || meta.Subscriptions["机场A"].Info["total"] != float64(100) {
		t.Fatalf("元数据迁移不完整: %+v", meta.Subscriptions)
	}
	tproxyState := readJSON[TproxyFile](t, FluxorTproxyFile)
	if !tproxyState.Enabled || !tproxyState.IPv6 || !tproxyState.ProxyLocal {
		t.Fatalf("TProxy 开关迁移不完整: %+v", tproxyState)
	}
	if len(tproxyState.DstExceptions) != 1 || tproxyState.DstExceptions[0] != "1.1.1.1" {
		t.Fatalf("旧合并字段应迁移为目的绕过: %+v", tproxyState.DstExceptions)
	}

	// 旧文件被归档（不删除），且不再参与后续启动
	archives, err := filepath.Glob(FluxorConfigFile + ".migrated-*")
	if err != nil || len(archives) != 1 {
		t.Fatalf("旧文件应归档一份，实际 %v (err=%v)", archives, err)
	}
	if _, err := os.Stat(FluxorConfigFile); !os.IsNotExist(err) {
		t.Fatalf("迁移后旧的 fluxor.json 不应继续存在: %v", err)
	}

	// 幂等：再跑一次不会改动任何文件，也不会新产生归档
	before := map[string]string{}
	for _, f := range []string{FluxorSettingsFile, FluxorRulesFile, FluxorTunnelsFile, FluxorMetaFile, FluxorTproxyFile} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", f, err)
		}
		before[f] = string(data)
	}
	MigrateLegacyFiles()
	for f, want := range before {
		data, err := os.ReadFile(f)
		if err != nil || string(data) != want {
			t.Fatalf("重复迁移改动了 %s", f)
		}
	}
}

// TestMigrateLegacyCorruptFileStartsFresh 旧文件损坏时备份并以默认值启动，不阻断面板。
func TestMigrateLegacyCorruptFileStartsFresh(t *testing.T) {
	dir := t.TempDir()
	SetDataDir(dir)
	defer capturePaths()()

	if err := os.WriteFile(FluxorConfigFile, []byte("{not json"), 0644); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	LoadAll()

	backups, _ := filepath.Glob(FluxorConfigFile + ".corrupt-*")
	if len(backups) != 1 {
		t.Fatalf("损坏的旧文件应被备份，实际 %v", backups)
	}
	Mu.RLock()
	proxyPort := Current.ProxyPort
	Mu.RUnlock()
	if proxyPort != defaultProxyPort {
		t.Fatalf("应以默认值启动，proxy_port = %d", proxyPort)
	}
	// 旧文件保持原位（内容已被备份），不会被当作新布局使用
	if _, err := os.Stat(FluxorConfigFile); err != nil {
		t.Fatalf("损坏的旧文件不应被改名/删除: %v", err)
	}
}

// TestLegacyIgnoredWhenNewLayoutExists 新布局已存在时忽略旧文件（避免两套真相）。
func TestLegacyIgnoredWhenNewLayoutExists(t *testing.T) {
	dir := t.TempDir()
	SetDataDir(dir)
	defer capturePaths()()

	// 新布局：显式写入面板密钥
	writeJSON(t, FluxorSettingsFile, Settings{ProxyPort: 1111, PanelSecret: "new"})
	// 旧文件：内容不同，必须被忽略
	writeJSON(t, FluxorConfigFile, map[string]any{"panel_secret": "legacy", "proxy_port": 2222})

	LoadAll()
	Mu.RLock()
	secret, port := Current.PanelSecret, Current.ProxyPort
	Mu.RUnlock()
	if secret != "new" || port != 1111 {
		t.Fatalf("应以新布局为准，实际 secret=%q port=%d", secret, port)
	}
	if _, err := os.Stat(FluxorConfigFile); err != nil {
		t.Fatalf("旧文件应保持原样（只忽略不改名）: %v", err)
	}
}
