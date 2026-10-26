// Package tun 管理 TUN 的运行参数与启停。
//
// 与 tproxy 包同构：自己持有一个 config.Store（tun.json），只有本包写它；默认值放在
// configgen.TunBlock 里，文件只存与默认值不同的键（稀疏）。
//
// 两条与安全相关的硬性约定（在 NAS 上 TUN 是危险功能，宁可烦一点）：
//
//  1. tun.json **不含 enable**。config.yaml 里的 tun.enable 恒为 false，任何启动
//     （面板启动、内核启动、配置重载）都不允许 TUN 自动接管宿主网络。
//  2. **不做回放**。TUN 的开关只活在运行时，内核重启或配置重载后必然关闭，用户需要
//     重新手动开启。这样才能保证「面板显示关闭时，网络一定没有被劫持」。
//
// 用户偏好只影响「开启那一刻」下发的完整 tun 对象，不参与 config.yaml 生成。
package tun

import (
	"fmt"
	"regexp"
	"strings"

	"fluxor/internal/config"
	"fluxor/internal/configgen"
	"fluxor/internal/logx"
)

// tunStore 是本包独占的持久化文件（config.TunFile）。
//
// 没有 Init：零值即「全部用模板默认值」，与稀疏语义天然一致。
var tunStore = config.NewStore[config.TunFile]("tun.json", &config.FluxorTunFile, nil)

// Settings 是 TUN 的生效参数（tun.json 覆盖模板默认值后的结果）。
type Settings struct {
	Stack   string `json:"stack"`
	Device  string `json:"device"`
	DNSMode string `json:"dns_mode"`
	IPv6    string `json:"ipv6"`
}

// Defaults 返回模板里的默认值（与 configgen.TunBlock 一一对应）。
func Defaults() Settings {
	return Settings{
		Stack:   configgen.TunDefaultStack,
		Device:  "",
		DNSMode: configgen.TunDefaultDNSMode,
		IPv6:    configgen.TunDefaultIPv6Mode,
	}
}

// Load 载入 tun.json。启动时调用一次。
func Load() {
	if err := tunStore.Load(); err != nil {
		logx.Error(logx.ModuleTun, "failed to load %s: %v", tunStore.Name, err)
	}
}

// Current 返回当前生效的设置（默认值 + tun.json 覆盖）。
func Current() Settings {
	var s Settings
	tunStore.View(func(f *config.TunFile) {
		s = effective(*f)
	})
	return s
}

// Save 归一化、校验并稀疏落盘，返回最终生效的设置。
//
// 稀疏：与默认值相同的键不写文件（指针置 nil），这样用户改回默认后会自动跟随
// 未来模板默认值的变化，而不是被一份历史快照钉死。
func Save(raw Settings) (Settings, error) {
	next, err := Normalize(raw)
	if err != nil {
		return Settings{}, err
	}

	def := Defaults()
	if err := tunStore.Update(func(f *config.TunFile) error {
		f.Stack = sparseString(next.Stack, def.Stack)
		f.Device = sparseString(next.Device, def.Device)
		f.DNSMode = sparseString(next.DNSMode, def.DNSMode)
		f.IPv6 = sparseString(next.IPv6, def.IPv6)
		return nil
	}); err != nil {
		return Settings{}, err
	}
	return next, nil
}

func effective(f config.TunFile) Settings {
	s := Defaults()
	if f.Stack != nil {
		s.Stack = *f.Stack
	}
	if f.Device != nil {
		s.Device = *f.Device
	}
	if f.DNSMode != nil {
		s.DNSMode = *f.DNSMode
	}
	if f.IPv6 != nil {
		s.IPv6 = *f.IPv6
	}
	return s
}

func sparseString(value, def string) *string {
	if value == def {
		return nil
	}
	v := value
	return &v
}

// deviceRe 限制 TUN 设备名的字符集与长度（Linux 接口名上限 15 字节）。
//
// 不引入太严的规则：内核自己会校验并回退到自动命名，这里只挡掉明显非法的输入
// （空格、斜杠、超长），避免把脏值写进 JSON 后再由内核报一个难以定位的错。
var deviceRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,14}$`)

// Normalize 归一化并校验设置。非法取值在落库前拒绝——TUN 参数写错会让内核起不来，
// 而 mihomo 对 dns-mode 的校验发生在创建监听器时（mihomo -t 查不出来）。
func Normalize(raw Settings) (Settings, error) {
	s := raw
	s.Stack = strings.ToLower(strings.TrimSpace(s.Stack))
	s.Device = strings.TrimSpace(s.Device)
	s.DNSMode = strings.ToLower(strings.TrimSpace(s.DNSMode))
	s.IPv6 = strings.ToLower(strings.TrimSpace(s.IPv6))

	switch s.Stack {
	case "gvisor", "system", "mixed", "mips":
	default:
		return Settings{}, fmt.Errorf("无效的 TUN 栈: %q（可选 gvisor / system / mixed / mips）", raw.Stack)
	}

	switch s.DNSMode {
	case configgen.TunDNSModeHijack, configgen.TunDNSModeNative, configgen.TunDNSModeDisabled:
	default:
		return Settings{}, fmt.Errorf("无效的 DNS 处理方式: %q（可选 hijack / native / disabled）", raw.DNSMode)
	}

	switch s.IPv6 {
	case configgen.TunIPv6Auto, configgen.TunIPv6On, configgen.TunIPv6Off:
	default:
		return Settings{}, fmt.Errorf("无效的 IPv6 接管方式: %q（可选 auto / on / off）", raw.IPv6)
	}

	if s.Device != "" && !deviceRe.MatchString(s.Device) {
		return Settings{}, fmt.Errorf("无效的 TUN 设备名: %q（最多 15 个字符，仅限字母、数字、下划线、点、连字符）", s.Device)
	}

	// 强制接管要求宿主真的能挂上 IPv6 地址，否则生成的会是一份起不来的配置。
	// 自动档不在这里拒绝：它按宿主能力自行降级，正是为了容忍这种宿主。
	if s.IPv6 == configgen.TunIPv6On && !HostIPv6Available() {
		return Settings{}, fmt.Errorf("本机新建接口默认禁用 IPv6（net.ipv6.conf.default.disable_ipv6=1），强制接管会让 TUN 启动失败；" +
			"请先执行 sysctl -w net.ipv6.conf.default.disable_ipv6=0 后重试")
	}

	return s, nil
}

// ipv6Enabled 返回本次开启是否应给 TUN 下发 IPv6 地址。
//
// auto 走宿主探测；on 在 Normalize 阶段已校验过宿主能力，这里再兜一次（保存后
// sysctl 可能被改回去）。
func (s Settings) ipv6Enabled() bool {
	switch s.IPv6 {
	case configgen.TunIPv6On:
		return HostIPv6Available()
	case configgen.TunIPv6Off:
		return false
	default:
		return HostIPv6Available()
	}
}
