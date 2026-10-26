package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"fluxor/internal/logx"
)

// 本文件负责「旧版单文件 → 新版分文件」的一次性迁移。
//
// 旧布局：全部状态挤在 fluxor.json（订阅配置 + 规则 + 隧道 + 元数据 + TProxy 字段）。
// 新布局：settings.json / rules.json / tunnels.json / subscription-meta.json / tproxy.json。
//
// 迁移原则：
//   - **幂等**：迁移标记不存在且 settings.json 已存在 → 新布局在用，直接跳过；
//   - **可回滚**：旧文件不删除，改名为 fluxor.json.migrated-<时间戳>；
//   - **要么全成功、要么续做**：写盘前先落一个 `fluxor.json.migrating` 标记，五份新文件
//     全部写成才归档旧文件并删标记。中途失败下次启动会读到标记，只补写缺失的那几份
//     （已写成的保持原样，不覆盖用户其后的改动），避免出现「旧文件还在、新文件只写了一半」
//     以及「未迁移的类别被永久跳过」两种状态；
//   - **损坏不阻断启动**：旧文件解析失败时备份为 .corrupt-<时间戳> 后按全新安装处理。

// legacyConfig 旧 fluxor.json 的完整形状：订阅配置 + TProxy 旁路字段。
//
// 这里刻意内嵌 SubscribeConfig（而不是新布局的类型）：迁移读的是**旧格式**，
// 其字段划分（规则/隧道/元数据都在订阅里）与新布局不同。
type legacyConfig struct {
	SubscribeConfig
	TproxyEnabled       *bool    `json:"tproxy_enabled"`
	TproxyProxyLocal    *bool    `json:"tproxy_proxy_local"`
	TproxyIPv6          *bool    `json:"tproxy_ipv6"`
	TproxyDstExceptions []string `json:"tproxy_dst_exceptions"`
	TproxySrcExceptions []string `json:"tproxy_src_exceptions"`
	// TproxyExceptionsOld 更早版本的合并字段（只有目的绕过），读取时迁移。
	TproxyExceptionsOld []string `json:"tproxy_exceptions"`
}

// MigrateLegacyFiles 在启动载入前完成迁移（幂等，可反复调用）。
//
// 幂等判据不能只看「settings.json 是否存在」。迁移是逐个文件写盘的，任一步失败都会
// 保留旧文件等待重试；若判据只看第一个文件，那么从第 2 个起失败时，下次启动会整段
// 跳过迁移——未迁移的类别既不落新文件、也不会被读取（旧文件只是被记一条 WARN 忽略），
// 用户看到的就是「规则/隧道/元数据凭空消失」。
//
// 因此引入一个显式的「迁移进行中」标记，把两种情形区分开：
//   - 标记存在 → 上一次迁移写到一半（本次是续做），只补写**缺失**的那几份，不覆盖已有文件；
//   - 标记不存在且 settings.json 已存在 → 新布局确实在用，旧文件只是残留，保持忽略。
func MigrateLegacyFiles() {
	legacyPath := FluxorConfigFile
	markerPath := legacyPath + ".migrating"

	resuming := false
	if _, err := os.Stat(markerPath); err == nil {
		resuming = true
	} else if _, err := os.Stat(FluxorSettingsFile); err == nil {
		// 新布局已在用：旧文件若还在，说明是上一次迁移后的残留（或用户手工放回的），
		// 只能忽略——反过来读它会与新文件产生两套真相。
		if _, err := os.Stat(legacyPath); err == nil {
			logx.Warn(logx.ModuleConfig, "legacy config %s is ignored because %s already exists",
				legacyPath, FluxorSettingsFile)
		}
		return
	}

	data, err := os.ReadFile(legacyPath)
	if err != nil {
		if resuming {
			// 续做时旧文件却不在了（被手工删除或归档）：无事可做，清掉标记即可
			logx.Warn(logx.ModuleConfig, "migration marker %s exists but %s is gone, clearing marker", markerPath, legacyPath)
			_ = os.Remove(markerPath)
		}
		// 没有旧文件：全新安装，各 store 首次写入时自然生成
		return
	}

	var legacy legacyConfig
	if err := json.Unmarshal(data, &legacy); err != nil {
		backup := fmt.Sprintf("%s.corrupt-%s", legacyPath, time.Now().Format("20060102-150405"))
		if werr := os.WriteFile(backup, data, 0644); werr != nil {
			logx.Error(logx.ModuleConfig, "failed to back up corrupt legacy config to %s: %v", backup, werr)
		}
		logx.Error(logx.ModuleConfig, "legacy config %s is not valid JSON, starting from defaults (backup: %s): %v",
			legacyPath, backup, err)
		_ = os.Remove(markerPath)
		return
	}

	settings, rules, tunnels, meta, tproxyState := splitLegacy(legacy)

	files := []struct {
		name string
		path string
		v    any
	}{
		{settingsStore.Name, FluxorSettingsFile, settings},
		{rulesStore.Name, FluxorRulesFile, rules},
		{tunnelsStore.Name, FluxorTunnelsFile, tunnels},
		{metaStore.Name, FluxorMetaFile, meta},
		{"tproxy.json", FluxorTproxyFile, tproxyState},
	}

	// 先落标记再写文件：标记存在即代表「这五份文件可能只写了一半」，与写入顺序无关。
	if err := os.WriteFile(markerPath, []byte(time.Now().Format(time.RFC3339)+"\n"), 0644); err != nil {
		logx.Error(logx.ModuleConfig, "failed to write migration marker %s, aborting migration: %v", markerPath, err)
		return
	}

	for _, f := range files {
		if resuming {
			// 续做：已写成的文件保持原样（它们可能已被用户改过），只补缺失的
			if info, err := os.Stat(f.path); err == nil && info.Size() > 0 {
				logx.Info(logx.ModuleConfig, "resuming migration: %s already present, skipping", f.name)
				continue
			}
		}
		out, err := json.MarshalIndent(f.v, "", "  ")
		if err != nil {
			logx.Error(logx.ModuleConfig, "failed to encode %s during migration: %v", f.name, err)
			return // 保留标记与旧文件，下次启动续做
		}
		if err := writeFileAtomic(f.path, append(out, '\n')); err != nil {
			logx.Error(logx.ModuleConfig, "failed to write %s during migration, will resume on next start: %v",
				f.name, err)
			return // 保留标记与旧文件，下次启动续做
		}
	}

	archived := fmt.Sprintf("%s.migrated-%s", legacyPath, time.Now().Format("20060102-150405"))
	if err := os.Rename(legacyPath, archived); err != nil {
		// 五份新文件都已就绪，仅归档失败：清掉标记即可（新布局已完整）。
		logx.Warn(logx.ModuleConfig, "config split done but archiving %s failed: %v", legacyPath, err)
	} else {
		logx.Info(logx.ModuleConfig, "config split done: %s archived as %s (subscriptions=%d)",
			legacyPath, archived, len(settings.Subscriptions))
	}
	if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
		logx.Warn(logx.ModuleConfig, "failed to remove migration marker %s: %v", markerPath, err)
	}
}

// splitLegacy 把旧格式拆成新布局的五份内容。
func splitLegacy(legacy legacyConfig) (Settings, RulesFile, TunnelsFile, MetaFile, TproxyFile) {
	// IPv6 两个开关在旧文件里没有对应字段，必须显式补成默认开启。
	//
	// 不能指望 newSettingsStore 的 Init：迁移是直接写 JSON，Settings 的 bool 没有
	// omitempty，写出的 "ipv6": false 会让 Init 的默认值失效（键存在即被覆盖）。
	// 而升级前的模板里 ipv6 恒为 true——漏掉这两行就是「升级后内核与 DNS 静默失去 IPv6」。
	defaults := DefaultSubscribeConfig()
	settings := Settings{
		IPv6:               defaults.IPv6,
		DNSIPv6:            defaults.DNSIPv6,
		ProxyPort:          legacy.ProxyPort,
		TproxyPort:         legacy.TproxyPort,
		PanelPort:          legacy.PanelPort,
		PanelSecret:        legacy.PanelSecret,
		RuleGroup:          legacy.RuleGroup,
		UIPanel:            legacy.UIPanel,
		MetaBackendURL:     legacy.MetaBackendURL,
		Mode:               legacy.Mode,
		ActiveSubscription: legacy.ActiveSubscription,
		CustomNodes:        copyCustomNodes(legacy.CustomNodes),
	}
	// 旧文件的默认值语义（键缺失即默认值）在迁移时补一次
	if settings.ProxyPort == 0 {
		settings.ProxyPort = defaultProxyPort
	}
	if settings.PanelPort == 0 {
		settings.PanelPort = defaultPanelPort
	}
	if settings.TproxyPort == 0 {
		settings.TproxyPort = defaultTproxyPort
	}
	if settings.Mode == "" {
		settings.Mode = ModeMerge
	}
	if settings.RuleGroup == "" {
		settings.RuleGroup = RuleGroupBase
	}
	if settings.UIPanel == "" {
		settings.UIPanel = "metacubexd"
	}

	rules := NewRulesFile()
	tunnels := NewTunnelsFile()
	meta := NewMetaFile()

	for _, sub := range legacy.Subscriptions {
		settings.Subscriptions = append(settings.Subscriptions, SubscriptionRef{
			Name:           sub.Name,
			URL:            sub.URL,
			UpdateInterval: sub.UpdateInterval,
			HealthInterval: sub.HealthInterval,
			Prefix:         sub.Prefix,
		})
		if len(sub.CustomRules) > 0 {
			rules.BySub[sub.Name] = copyRules(sub.CustomRules)
		}
		if len(sub.Tunnels) > 0 {
			tunnels.BySub[sub.Name] = CopyTunnels(sub.Tunnels)
		}
		if sub.UpdatedAt != "" || len(sub.SubscriptionInfo) > 0 {
			meta.Subscriptions[sub.Name] = SubscriptionMeta{
				UpdatedAt: sub.UpdatedAt,
				Info:      copyAnyMap(sub.SubscriptionInfo),
			}
		}
	}
	for scope, list := range legacy.MergeCustomRules {
		if len(list) > 0 {
			rules.Merge[scope] = copyRules(list)
		}
	}
	if len(legacy.CustomModeRules) > 0 {
		rules.CustomMode = copyRules(legacy.CustomModeRules)
	}
	for scope, list := range legacy.MergeTunnels {
		if len(list) > 0 {
			tunnels.Merge[scope] = CopyTunnels(list)
		}
	}
	if len(legacy.CustomModeTunnels) > 0 {
		tunnels.CustomMode = CopyTunnels(legacy.CustomModeTunnels)
	}

	// TProxy：三个开关与 tproxy 包的默认值一致（全部关闭）。旧文件里显式写过的取值
	// 在下面逐项覆盖；只有「旧文件缺该字段」才落到关闭——开启任何一个都会改宿主网络，
	// 缺省必须是安全方向。
	tproxyState := TproxyFile{}
	if legacy.TproxyEnabled != nil {
		tproxyState.Enabled = *legacy.TproxyEnabled
	}
	if legacy.TproxyProxyLocal != nil {
		tproxyState.ProxyLocal = *legacy.TproxyProxyLocal
	}
	if legacy.TproxyIPv6 != nil {
		tproxyState.IPv6 = *legacy.TproxyIPv6
	}
	tproxyState.DstExceptions = legacy.TproxyDstExceptions
	if len(tproxyState.DstExceptions) == 0 {
		tproxyState.DstExceptions = legacy.TproxyExceptionsOld
	}
	tproxyState.SrcExceptions = legacy.TproxySrcExceptions

	return settings, rules, tunnels, meta, tproxyState
}
