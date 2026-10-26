package configgen

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// TunBlock 是 config.yaml 里 TUN 的**静态安全基线**。
//
// 它不承载任何用户偏好——用户的 TUN 设置存在 tun.json 里，只在开启 TUN 时由
// /config/tun/state 组装成完整对象 PATCH 给内核。这里固定下来的原因有两条：
//
//  1. enable 恒为 false：NAS 上开机自动接管宿主网络，一旦配置有误就会失联。
//     因此任何启动（面板启动、内核启动、配置重载）都不得让 TUN 处于开启状态；
//     开启动作只能由用户手动触发，且不做回放。
//  2. inet6-address 恒为空：宿主 net.ipv6.conf.default.disable_ipv6=1 时，新建
//     tun 接口会禁用 IPv6，sing-tun 下发地址会被内核以 EACCES 拒绝，整只 TUN 起不来
//     （"configure tun interface: permission denied"）。基线不接管 IPv6 是最安全的
//     失败方向；需要接管时由用户在 TUN 设置里显式开启，并在开启时校验宿主能力。
//
// 这份基线还有一层兜底意义：Fluxor 对外暴露了 MetaCubeXD / Zashboard，用户可能在
// 第三方面板里直接打开 TUN——那时只有这份基线生效，它必须是安全的。
//
// 三条生成链路共用它（模板 / 切换模式的订阅文件副本 / 无订阅兜底），不得各写一份。
const TunBlock = `
tun:
  enable: false
  stack: mixed
  dns-mode: hijack
  inet6-address: []
  dns-hijack:
    - "any:53"
    - "tcp://any:53"
  auto-route: true
  auto-redirect: true
  auto-detect-interface: true
`

// TUN 运行参数的默认值（与 TunBlock 一一对应，供 tun 包做稀疏落盘与「恢复默认」判断）。
//
// 刻意只在这两个地方出现：TunBlock 是产物，这里是代码可比较的常量；两者一致性由
// tun_test.go 的断言保证，避免以后改了一边忘了另一边。
const (
	TunDefaultStack    = "mixed"
	TunDefaultDNSMode  = "hijack"
	TunDefaultIPv6Mode = TunIPv6Auto
)

// TUN 是否承运 IPv6 的两种取值（tun.json 的 ipv6 字段）。
//
// 刻意不提供「强制开启」：宿主支持时 auto 与强制等价，宿主不支持时强制只会让
// TUN 起不来（内核拒绝给禁用 IPv6 的新接口挂地址），因此强制档没有可用场景。
// 旧版本写入的 "on" 会在 tun.Normalize 里收敛为 auto，不做报错。
const (
	// TunIPv6Auto 跟随宿主能力：新建接口允许 IPv6 才下发地址（修复 disable_ipv6 宿主）。
	TunIPv6Auto = "auto"
	// TunIPv6Off 不下发地址（IPv6 不走 TUN，可能直连）。
	TunIPv6Off = "off"
)

// TUN 的 dns-mode 取值（透传给内核，见 mihomo 的 tun.dns-mode）。
const (
	TunDNSModeHijack   = "hijack"
	TunDNSModeNative   = "native"
	TunDNSModeDisabled = "disabled"
)

// ParseTunBlock 把 TunBlock 解析成顶层键 tun 的值节点。
//
// 切换模式的 config.yaml 是订阅文件副本，机场自带的 tun 块与宿主无关（device、
// auto-route、甚至 enable: true 都可能出现），必须整块替换成这份基线。
func ParseTunBlock() (*yaml.Node, error) {
	var wrapper struct {
		Tun yaml.Node `yaml:"tun"`
	}
	if err := yaml.Unmarshal([]byte(TunBlock), &wrapper); err != nil {
		return nil, fmt.Errorf("解析 TUN 块失败: %w", err)
	}
	if wrapper.Tun.Kind == 0 {
		return nil, fmt.Errorf("解析 TUN 块失败: 未得到 tun 节点")
	}
	return &wrapper.Tun, nil
}
