package tproxy

import "testing"

// TestSwitchesDefaultOff 钉住三个开关的默认值：全新部署（没有 tproxy.json）时
// TProxy 开关、本机流量接管、IPv6 接管都必须关闭——开启任何一个都会改宿主网络
// （nft 表 / 策略路由 / 本机出站劫持），缺省值必须是安全方向。
func TestSwitchesDefaultOff(t *testing.T) {
	useTempTproxyFile(t)
	LoadTproxyState()

	if LoadTproxyEnabled() {
		t.Error("TProxy 开关应默认关闭")
	}
	if proxyLocalEnabled() {
		t.Error("接管本机流量应默认关闭")
	}
	if ipv6Enabled() {
		t.Error("接管 IPv6 流量应默认关闭")
	}
}
