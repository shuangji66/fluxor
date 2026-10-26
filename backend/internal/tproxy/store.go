package tproxy

import (
	"fluxor/internal/config"
	"fluxor/internal/logx"
	"slices"
)

// tproxy.json 是本包独占的持久化文件（config.TproxyFile）。
//
// 拆分前这些字段与订阅配置共用 fluxor.json，写入必须借道 config.UpdateConfigFile 并
// 共用那把全局文件锁（否则会把订阅字段抹掉）。现在本包自己持有一个 config.Store：
// 只有本包写它，锁按文件隔离，读取也不再需要「读整份配置」。
//
// 默认值不落盘：两条绕过列表为 nil 表示「用代码里的预填模板」（defaultDstExceptions /
// defaultSrcExceptions），因此预填的 20+ 行中文注释不会写进用户文件。旧单文件时代
// 已经写进去的默认值，会在 LoadTproxyState 的归一化步骤中被移除。
//
// 三个开关（Enabled / ProxyLocal / IPv6）**全部默认关闭**，即零值即可，故没有 Init：
// 它们都会改宿主网络（nft 表、策略路由、本机出站劫持），新建部署必须从「什么都没接管」
// 开始，由用户逐项显式开启。旧实现里「本机流量接管默认开」是沿用更早的语义，现已收敛。
var tproxyStore = config.NewStore[config.TproxyFile]("tproxy.json", &config.FluxorTproxyFile, nil)

// effectiveExceptions 把「nil = 未自定义」还原成生效列表。
func effectiveExceptions(stored []string, fallback func() []string) []string {
	if stored == nil {
		return fallback()
	}
	return stored
}

// normalizeExceptions 把与预填模板完全一致的列表收敛为 nil（不落盘）。
//
// 刻意用「与默认值相等」而不是「用户是否点过恢复默认」作为判据：两者结果一致，
// 但不依赖任何额外状态，迁移过来的历史文件也能自动瘦身。
func normalizeExceptions(list []string, def func() []string) []string {
	if slices.Equal(list, def()) {
		return nil
	}
	return list
}

// LoadTproxyState 载入 tproxy.json：读盘 → 归一化（默认值不落盘）→ 填充内存缓存。
//
// 取代了此前四个各读一次整份配置的 LoadTproxyXxx 函数：那时每读一个字段都要解析
// 整个 fluxor.json（含订阅元数据与全部规则），现在只读这一份小文件。
func LoadTproxyState() {
	if err := tproxyStore.Load(); err != nil {
		logx.Error(logx.ModuleTproxy, "failed to load %s: %v", tproxyStore.Name, err)
	}

	// 归一化：先把判断所需的值取出，确有需要再另起一次 Update——不得在 store 的
	// View 回调里再调 Update（同一把非可重入锁会自死锁）。
	var needNormalize bool
	tproxyStore.View(func(v *config.TproxyFile) {
		needNormalize = slices.Equal(v.DstExceptions, defaultDstExceptions()) ||
			slices.Equal(v.SrcExceptions, defaultSrcExceptions())
	})
	if needNormalize {
		if err := tproxyStore.Update(func(v *config.TproxyFile) error {
			v.DstExceptions = normalizeExceptions(v.DstExceptions, defaultDstExceptions)
			v.SrcExceptions = normalizeExceptions(v.SrcExceptions, defaultSrcExceptions)
			return nil
		}); err != nil {
			logx.Warn(logx.ModuleTproxy, "failed to drop default bypass template from %s: %v", tproxyStore.Name, err)
		} else {
			logx.Info(logx.ModuleTproxy, "default bypass template removed from %s (defaults now come from code)", tproxyStore.Name)
		}
	}

	var proxyLocal, ipv6 bool
	tproxyStore.View(func(v *config.TproxyFile) {
		proxyLocal, ipv6 = v.ProxyLocal, v.IPv6
		dst := effectiveExceptions(v.DstExceptions, defaultDstExceptions)
		src := effectiveExceptions(v.SrcExceptions, defaultSrcExceptions)

		exceptionsMu.Lock()
		tproxyDstExceptionsCache = dst
		tproxySrcExceptionsCache = src
		tproxyProxyLocal = v.ProxyLocal
		tproxyIPv6 = v.IPv6
		exceptionsMu.Unlock()
	})
	logx.Debug(logx.ModuleTproxy, "tproxy state loaded: file=%s proxy_local=%v ipv6=%v",
		tproxyStore.FilePath(), proxyLocal, ipv6)
}

// 绕过列表的预填内容（tproxy.json 里缺该字段时取用，也供前端「恢复默认」按钮取用）。
//
// 只预填可核实的条目：公共 DNS 的 IPv4/IPv6 地址取自厂商官方页面
// （阿里 223.5.5.5 / 223.6.6.6 / 2400:3200::1 / 2400:3200:baba::1，
// 百度 180.76.76.76 / 2400:da00::6666）。微信 / 支付宝 / 抖音等服务的接入 IP 由 CDN
// 动态分配、没有官方公布的固定网段，因此**不预填具体 IP**，只在注释里给出自行查证
// 的方式——填错网段会把本该走代理的流量静默放行，比留空更危险。
//
// 注释（`#` 之后的内容）解析时会被剥离（stripComment），整行注释等价于空行，
// 因此下面的文字不会进入任何 nft 规则；但界面与文档共用这份内容，需保持可读。
//
// 分隔行刻意用 "#" 而不是空字符串：前端保存时会丢弃空行（见 Config.vue 的
// saveTproxyExceptions），只有这样「预填 → 保存 → 再打开」才能逐行一致。
func defaultDstExceptions() []string {
	return []string{
		"# 目的绕过：命中这些目标的流量不进代理，直接在系统侧出站",
		"#",
		"# 公共 DNS（IPv4）：让解析请求直连，避免 DNS 被劫持",
		"223.5.5.5 # 阿里公共 DNS",
		"223.6.6.6 # 阿里公共 DNS",
		"180.76.76.76 # 百度公共 DNS",
		"119.29.29.29 # 腾讯 DNSPod 公共 DNS",
		"1.12.12.12 # 腾讯 DNSPod 公共 DNS，DoH 入口",
		"#",
		"# 公共 DNS（IPv6）：需先开启「接管 IPv6 流量」，否则不会下发",
		"2400:3200::1 # 阿里公共 DNS",
		"2400:3200:baba::1 # 阿里公共 DNS",
		"2400:da00::6666 # 百度公共 DNS",
		"#",
		"# STUN 与内网穿透：穿透通道必须能直连，否则开启后可能连不上中转节点",
		"141.101.90.1 # 示例：请替换为你实际使用的 STUN 或穿透服务地址",
		"#",
		"# 国内常见服务：微信、支付宝、抖音等没有官方公布的固定网段，接入 IP 由 CDN 动态分配，",
		"# 故不预填。确需按服务绕过时先查当前 IP 再填，例如 dig +short short.weixin.qq.com。",
		"# 多数国内目标已由内核的 GEOIP 与 GEOSITE 规则判为直连，这里只用于完全不进内核的场景。",
		"#",
		"# 常用设备的 IPv6 网段：用 IPv6 全局地址访问 NAS、摄像头等设备时，其前缀不在内置保留",
		"# 网段内，需按设备前缀整段放行。前缀随重新拨号变化，按 /64 整段填比填单个地址稳定",
		"# （用 ip -6 addr 查看）。ULA 与链路本地已由内置规则绕过，无需重复填写。例如：",
		"# 240e:3b3:xxxx:xxxx::/64",
	}
}

// defaultSrcExceptions 源绕过的预填内容：命中这些来源的设备、容器完全不经代理。
func defaultSrcExceptions() []string {
	return []string{
		"# 源绕过：命中这些来源的设备、容器完全不经过代理，含 DNS",
		"# 仅支持 IP 与 CIDR，端口写法在这里无效",
		"#",
		"# Docker 默认 bridge 网段：填在这里意味着 Docker 容器的出站流量默认不代理，",
		"# 容器仍按宿主机原有网络直连。要让容器也走代理，删掉本行即可；",
		"# 自定义过 docker 网络的话，用 docker network inspect 查看实际网段后替换本行。",
		"172.17.0.0/16",
		"#",
		"# 常用设备的 IPv6 网段：设备的 IPv6 全局地址由运营商前缀加设备后缀组成，前缀会随",
		"# 重新拨号变化，按 /64 整段填比填单个地址稳定（用 ip -6 addr 查看）。例如：",
		"# 240e:3b3:xxxx:xxxx::/64",
		"# 注意：ULA 与链路本地已由内置保留网段绕过，无需重复填写；未开启「接管 IPv6 流量」",
		"# 时这些 IPv6 条目不会下发。",
	}
}

// SaveTproxyDstExceptions 保存目的绕过列表（nil 表示恢复为代码里的预填模板）。
func SaveTproxyDstExceptions(dst []string) error {
	stored := normalizeExceptions(dst, defaultDstExceptions)
	if err := tproxyStore.Update(func(v *config.TproxyFile) error {
		v.DstExceptions = stored
		return nil
	}); err != nil {
		return err
	}
	exceptionsMu.Lock()
	tproxyDstExceptionsCache = effectiveExceptions(stored, defaultDstExceptions)
	exceptionsMu.Unlock()
	return nil
}

// SaveTproxySrcExceptions 保存源绕过列表（nil 表示恢复为代码里的预填模板）。
func SaveTproxySrcExceptions(src []string) error {
	stored := normalizeExceptions(src, defaultSrcExceptions)
	if err := tproxyStore.Update(func(v *config.TproxyFile) error {
		v.SrcExceptions = stored
		return nil
	}); err != nil {
		return err
	}
	exceptionsMu.Lock()
	tproxySrcExceptionsCache = effectiveExceptions(stored, defaultSrcExceptions)
	exceptionsMu.Unlock()
	return nil
}

// SaveTproxyProxyLocal 保存「接管本机流量」开关。
func SaveTproxyProxyLocal(enabled bool) error {
	if err := tproxyStore.Update(func(v *config.TproxyFile) error {
		v.ProxyLocal = enabled
		return nil
	}); err != nil {
		return err
	}
	exceptionsMu.Lock()
	tproxyProxyLocal = enabled
	exceptionsMu.Unlock()
	return nil
}

// SaveTproxyIPv6 保存「接管 IPv6 流量」开关。
func SaveTproxyIPv6(enabled bool) error {
	if err := tproxyStore.Update(func(v *config.TproxyFile) error {
		v.IPv6 = enabled
		return nil
	}); err != nil {
		return err
	}
	exceptionsMu.Lock()
	tproxyIPv6 = enabled
	exceptionsMu.Unlock()
	return nil
}

// LoadTproxyEnabled 读取持久化的 TProxy 开关状态。
//
// 仅供诊断/日志使用：实际生效状态由 ResetOnStartup 在冷启动时统一归零，
// 因此本函数不会用于恢复运行状态。
func LoadTproxyEnabled() bool {
	enabled := false
	tproxyStore.View(func(v *config.TproxyFile) { enabled = v.Enabled })
	return enabled
}

// persistTproxyEnabled 持久化开关状态。
func persistTproxyEnabled(enabled bool) error {
	return tproxyStore.Update(func(v *config.TproxyFile) error {
		v.Enabled = enabled
		return nil
	})
}

// SetTproxyEnabled 设置内存中的开关状态并持久化，供 HTTP 层复用。
func SetTproxyEnabled(enabled bool) {
	tproxyMu.Lock()
	tproxyEnableState = enabled
	tproxyMu.Unlock()

	if err := persistTproxyEnabled(enabled); err != nil {
		logx.Error(logx.ModuleTproxy, "failed to persist tproxy state: %v", err)
	}
}

// ResetOnStartup 冷启动时的状态收敛。
//
// nftables 规则不跨重启存活，而开关状态是持久化的：若上一次是非优雅退出
// （kill -9 / 崩溃），nft 规则可能仍然存在，此时磁盘上的开关却可能为「已关闭」，
// 出现「面板显示关闭、流量实际仍被劫持」的静默错配。
//
// 因此在冷启动时无条件把状态归零，并清除任何残留规则，让内存态、磁盘态与
// 内核态三者重新一致。用户需要 TProxy 时再手动开启。
func ResetOnStartup() {
	// 先取「上一次是否处于启用态」再归零：顺序反了就只能读到自己刚写的 false，
	// 归零这件事在日志里永远静默（上一次非优雅退出留下的残留规则也就无从察觉）。
	wasEnabled := LoadTproxyEnabled()

	// 先清残留规则（不依赖当前布尔值，确保任何残留都被移除）
	DisableTProxyRules()

	SetTproxyEnabled(false)

	if wasEnabled {
		logx.Info(logx.ModuleTproxy, "previous state was enabled: reset to disabled on cold start and stale rules were cleaned")
	}
}
