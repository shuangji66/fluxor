package tun

import (
	"os"
	"strings"
)

// HostIPv6Available 判断「新建 tun 接口能否接受 IPv6 地址」。
//
// 只看宿主 sysctl，**不看宿主有没有全局 IPv6**：TUN 是否承运 IPv6 与宿主自身的
// v6 连通性无关——代理节点可能提供 v6 出口，宿主没有 v6 一样可以接管。
//
// 判据针对的是内核的实际行为：宿主 net.ipv6.conf.default.disable_ipv6=1 时，
// 新建接口默认禁用 IPv6，sing-tun 下发地址会被内核以 EACCES 拒绝（addrconf_add_dev），
// 整只 TUN 起不来并报 "configure tun interface: permission denied"（看起来像权限问题，
// 实际是 IPv6 被禁用）。这也是 TUN 基线不下发 IPv6 地址的原因。
//
// 三个文件任一为 1 即视为不可用；一个都读不到（未挂载 procfs / 无 ipv6 栈）同样
// 视为不可用——这是保守方向：宁可不下发地址，也不要让 TUN 起不来。
func HostIPv6Available() bool {
	found := false
	for _, path := range []string{
		"/proc/sys/net/ipv6/conf/default/disable_ipv6",
		"/proc/sys/net/ipv6/conf/all/disable_ipv6",
		"/sys/module/ipv6/parameters/disable",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		found = true
		if strings.TrimSpace(string(data)) == "1" {
			return false
		}
	}
	return found
}
