package tproxy

import (
	"bytes"
	"fluxor/internal/logx"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

// FWMARK / TABLE_ID 与各家族 enable 中写入策略路由时使用的取值一致。
//
// v4 与 v6 共用同一组取值是安全的：`ip rule` / `ip route` 的家族由 -4 / -6 决定，
// 两个家族的 FIB 相互独立，互不干扰。
const (
	tproxyFwmark  = "1"
	tproxyTableID = "100"
)

// tproxyFamily 描述「一套」TProxy 规则所需的家族差异。
//
// IPv4 与 IPv6 的规则结构完全同构，只有家族、表名、地址关键字、集合元素类型、
// 策略路由的默认路由与保留网段不同。把它们收敛到本结构，让启用与清理各自只写
// 一遍逻辑，避免两套规则在后续维护中漂移（改一处、忘一处是最容易出的事故）。
type tproxyFamily struct {
	label      string   // 日志/错误信息中的可读名
	nftFamily  string   // nftables 家族：ip / ip6
	nftTable   string   // nftables 表名
	addrKw     string   // nft 地址关键字：ip / ip6
	setType    string   // 集合元素类型：ipv4_addr / ipv6_addr
	ipFlag     string   // ip 命令的家族参数：-4 / -6
	defaultRt  string   // 策略路由默认路由：0.0.0.0/0 / ::/0
	bypassNets []string // 需要绕过的保留网段
	isV6       bool
}

var (
	// IPv4：表名沿用历史值 fluxor_tproxy，升级后面板仍能清理旧残留。
	tproxyFamilyV4 = tproxyFamily{
		label:     "IPv4",
		nftFamily: "ip",
		nftTable:  "fluxor_tproxy",
		addrKw:    "ip",
		setType:   "ipv4_addr",
		ipFlag:    "-4",
		defaultRt: "0.0.0.0/0",
		bypassNets: []string{
			"10.0.0.0/8", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
			"192.168.0.0/16", "224.0.0.0/4", "240.0.0.0/4",
		},
	}

	// IPv6：表名与 IPv4 区分（便于 `nft list tables` 与排障直读），仅由
	// 「接管 IPv6 流量」开关控制下发。
	//
	// ff00::/8（组播）必须绕过：DHCPv6 的 UDP 547 目的地址是 ff02::1:2，
	// 不绕过会被 TPROXY 劫持，导致 DHCPv6 直接失效。
	tproxyFamilyV6 = tproxyFamily{
		label:     "IPv6",
		nftFamily: "ip6",
		nftTable:  "fluxor_tproxy6",
		addrKw:    "ip6",
		setType:   "ipv6_addr",
		ipFlag:    "-6",
		defaultRt: "::/0",
		bypassNets: []string{
			"::1/128", "fc00::/7", "fe80::/10", "ff00::/8",
		},
		isV6: true,
	}
)

// tproxyFamilies 清理时使用的家族全集。
//
// 清理必须覆盖两个家族，且与开关状态无关：IPv6 规则可能由上一次「已开启」的
// 运行遗留（此后用户关掉开关或非优雅退出），只按当前开关清理会漏删残留。
var tproxyFamilies = []tproxyFamily{tproxyFamilyV4, tproxyFamilyV6}

// hasFwmarkRule 检测策略路由规则（fwmark 1 -> table 100）是否已存在。
//
// 先探测再删除，避免对不存在的规则执行 del 而徒增错误。
func (f tproxyFamily) hasFwmarkRule() bool {
	out, err := exec.Command("ip", f.ipFlag, "rule", "show").Output()
	if err != nil {
		return false
	}
	return matchFwmarkRule(string(out))
}

// matchFwmarkRule 解析 `ip rule show` 的输出，判断是否含目标策略路由。
// 抽成纯函数以便单测覆盖，无需真实改动系统防火墙。
func matchFwmarkRule(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "fwmark") {
			continue
		}
		if strings.Contains(line, "lookup "+tproxyTableID) || strings.Contains(line, "table "+tproxyTableID) {
			return true
		}
	}
	return false
}

// hasLocalRoute 检测 table 100 中是否存在本地路由（local default dev lo）。
//
// table 不存在时 `ip route show table 100` 会以非零码退出，据此判定为无残留，
// 不执行任何删除。
func (f tproxyFamily) hasLocalRoute() bool {
	out, err := exec.Command("ip", f.ipFlag, "route", "show", "table", tproxyTableID).Output()
	if err != nil {
		return false
	}
	return matchLocalRoute(string(out))
}

// matchLocalRoute 解析 `ip route show table 100` 的输出，判断是否含本地路由。
func matchLocalRoute(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "local ") {
			return true
		}
	}
	return false
}

// hasNftTable 检测本家族的 nftables 表是否已存在。
//
// `nft list table` 在表不存在时返回非零码；无权限时同样返回非零码，
// 两种情况都会被视为「无残留」，此时删除也必然失败，跳过是正确行为。
func (f tproxyFamily) hasNftTable() bool {
	// nft 会把报错写到 stderr，这里丢弃以免污染日志
	cmd := exec.Command("nft", "list", "table", f.nftFamily, f.nftTable)
	cmd.Stderr = &bytes.Buffer{}
	return cmd.Run() == nil
}

// parseTproxyException 解析单条绕过规则，返回 (ruleType, ipNet, proto, port)
// 支持：
//   - IP/CIDR: 192.168.1.0/24、2001:db8::/32
//   - 单个IP: 192.168.1.1、2001:db8::1（IPv6 按 /128 处理）
//   - 端口(所有协议): 53
//   - 协议:端口: tcp:80 或 udp:443
//
// ruleType 为 "ip" 时 ipNet 有效；为 "port" 时 proto/port 有效。
func parseTproxyException(rule string) (typ string, ipNet *net.IPNet, proto string, port int, err error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return "", nil, "", 0, fmt.Errorf("空规则")
	}
	// 尝试 IP/CIDR
	if _, n, err := net.ParseCIDR(rule); err == nil {
		// v4-mapped（::ffff:a.b.c.d/x）的 To4() 非 nil，Go 会把它显示成点分形式，
		// 下发时得到形如 1.2.3.4/128 的非法 CIDR（家族与掩码长度不匹配）。
		// 这类输入显式拒绝，而不是下发出错后只留一行日志。
		if n.IP.To4() != nil && strings.Contains(rule, ":") {
			return "", nil, "", 0, fmt.Errorf("不支持 IPv4-mapped IPv6 网段，请使用 IPv4 或 IPv6 写法")
		}
		return "ip", n, "", 0, nil
	}
	// 尝试单个 IP
	if ip := net.ParseIP(rule); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return "ip", &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}, "", 0, nil
		}
		// IPv6 单地址必须落到 /128。历史上这里对所有单地址一律拼 "/32"，
		// 而 net.ParseCIDR("2001:db8::1/32") 是合法的并返回 2001:db8::/32——
		// 于是「一个 IPv6 地址」被静默放大成整个 /32 网段。
		return "ip", &net.IPNet{IP: ip.To16(), Mask: net.CIDRMask(128, 128)}, "", 0, nil
	}
	// 尝试 协议:端口
	if strings.Contains(rule, ":") {
		parts := strings.SplitN(rule, ":", 2)
		proto := strings.ToLower(parts[0])
		if proto != "tcp" && proto != "udp" {
			return "", nil, "", 0, fmt.Errorf("协议仅支持 tcp/udp")
		}
		p, err := strconv.Atoi(parts[1])
		if err != nil || p < 1 || p > 65535 {
			return "", nil, "", 0, fmt.Errorf("端口无效")
		}
		return "port", nil, proto, p, nil
	}
	// 尝试纯数字端口
	if p, err := strconv.Atoi(rule); err == nil && p > 0 && p <= 65535 {
		return "port", nil, "", p, nil
	}
	return "", nil, "", 0, fmt.Errorf("不支持的格式")
}

// isV6Net 判断解析出的网段属于 IPv6 家族。
//
// 判据取「下发时使用的文本是否含 ':'」。v4-mapped 形式在 parse 阶段已被显式
// 拒绝，因此这里不会出现「To4() 非 nil 但文本含 ':'」的歧义输入。
func isV6Net(n *net.IPNet) bool {
	return n != nil && strings.Contains(n.String(), ":")
}

// warnSkippedV6Exceptions 在未开启 IPv6 接管时，逐条说明哪些 IPv6 绕过未下发。
//
// 历史上这些规则会被当成 IPv4 规则塞进 ip 家族的表里、必然失败，而错误只留一行
// 通用日志，用户根本看不出「绕过没生效」——静默失效正是必须消除的行为。
func warnSkippedV6Exceptions(dst, src []string) {
	logSkipped := func(kind, rule string) {
		logx.Warn(logx.ModuleTproxy, "ipv6 %s bypass entry not applied: rule=%q (IPv6 tproxy is not applied)", kind, rule)
	}
	for _, rule := range dst {
		rule = stripComment(rule)
		if rule == "" {
			continue
		}
		if typ, n, _, _, err := parseTproxyException(rule); err == nil && typ == "ip" && isV6Net(n) {
			logSkipped("destination", rule)
		}
	}
	for _, rule := range src {
		rule = stripComment(rule)
		if rule == "" {
			continue
		}
		if typ, n, _, _, err := parseTproxyException(rule); err == nil && typ == "ip" && isV6Net(n) {
			logSkipped("source", rule)
		}
	}
}

// runCmd 执行单条规则命令，失败时记录日志并返回错误。
//
// 单条规则失败不中断整套下发（与历史行为一致），但关键产物（nft 表、策略路由）
// 会在 enable 末尾各自复核。
func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Run(); err != nil {
		logx.Error(logx.ModuleTproxy, "command execution failed: name=%s args=%v err=%v", name, args, err)
		return err
	}
	return nil
}

// effectiveIPv6Takeover 决定本次是否下发 IPv6 家族规则。
//
//   - preference 是用户「接管 IPv6 流量」开关（想不想）；
//   - global 是订阅中心「IPv6 支持」总开关（能不能）。
//
// 两者必须同时为真。总开关关闭时只跳过规则、不写回 preference，因此用户不必在
// 关掉总开关后重新勾一遍；反过来也不允许子开关把总开关带开。
func effectiveIPv6Takeover(preference, global bool) bool {
	return preference && global
}

// EnableTProxyRules 配置策略路由与 nftables 规则。
//
// IPv4 恒下发；IPv6 仅在「接管 IPv6 流量」开关打开时下发（默认关闭）：节点多数
// 没有 IPv6 出口，无条件接管会把原本可直连的 IPv6 目标变成必走代理而失败。
//
// 写任何规则之前先执行注入的启用前校验（见 SetEnableGuard）：TUN 与 TProxy 都会
// 改宿主路由/防火墙，同时生效会互相破坏对方的流量路径。校验放在这里而不是只放在
// HTTP handler，是因为端口变更（/configs PATCH）与重载后的重建也会走到本函数——
// 只在开关路径拦截，就漏掉了「TUN 开着时改 TProxy 端口」这条。
func EnableTProxyRules(port int) error {
	if port <= 0 {
		return nil
	}

	if err := guardRulesApply(); err != nil {
		return err
	}

	dst := dstExceptions()
	src := srcExceptions()

	// IPv6 家族：偏好与「IPv6 支持」总开关必须同时为真。
	// 总开关关闭时只跳过、不修改偏好（tproxy.json 原样保留），恢复后下次重建自动生效。
	families := []tproxyFamily{tproxyFamilyV4}
	if effectiveIPv6Takeover(ipv6Enabled(), globalIPv6Enabled()) {
		families = append(families, tproxyFamilyV6)
	} else {
		if ipv6Enabled() {
			logx.Warn(logx.ModuleTproxy, "ipv6 tproxy takeover skipped: IPv6 support is disabled in subscriptions")
		}
		warnSkippedV6Exceptions(dst, src)
	}

	applied := make([]string, 0, len(families))
	errs := make([]string, 0, len(families))
	for _, f := range families {
		if err := f.enable(port, dst, src); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		applied = append(applied, f.label)
	}
	if len(errs) > 0 {
		// 任一已启用家族没装成功都必须如实上报：由调用方决定是否回滚开关，
		// 绝不能在规则未生效时回报 enabled=true。
		return fmt.Errorf("%s", strings.Join(errs, "；"))
	}

	logx.Info(logx.ModuleTproxy, "tproxy rules applied (including dst/src bypass and proxy-local): families=%s", strings.Join(applied, " + "))
	return nil
}

// enable 下发单一家族的策略路由与 nftables 规则。
//
// 成功判据不是「命令都没报错」，而是三件关键产物真实存在：nft 表、fwmark 策略
// 路由、策略路由表里的 local 路由。缺任意一项都会造成「面板显示已启用、流量实际
// 被导入黑洞」——策略路由缺失时，被标记的包无法交付给本地 TProxy 套接字。
func (f tproxyFamily) enable(port int, dstExceptions, srcExceptions []string) error {
	proxyLocal := proxyLocalEnabled()

	// 1. 策略路由（v4/v6 家族各自独立的 FIB）
	runCmd("ip", f.ipFlag, "rule", "add", "fwmark", tproxyFwmark, "table", tproxyTableID)
	runCmd("ip", f.ipFlag, "route", "add", "local", f.defaultRt, "dev", "lo", "table", tproxyTableID)

	// 2. 关闭反向路径过滤。rp_filter 只有 IPv4 有，IPv6 无对应项，故不涉及。
	if !f.isV6 {
		runCmd("sysctl", "-w", "net.ipv4.conf.all.rp_filter=0")
		runCmd("sysctl", "-w", "net.ipv4.conf.default.rp_filter=0")
		runCmd("sysctl", "-w", "net.ipv4.conf.lo.rp_filter=0")
	}

	// 3. 创建 nftables 表
	runCmd("nft", "add", "table", f.nftFamily, f.nftTable)

	// 4. 绕过保留网段
	runCmd("nft", "add", "set", f.nftFamily, f.nftTable, "private_ips", "{ type "+f.setType+"; flags interval; }")
	for _, ip := range f.bypassNets {
		runCmd("nft", "add", "element", f.nftFamily, f.nftTable, "private_ips", "{", ip, "}")
	}

	// 5. 创建链
	runCmd("nft", "add", "chain", f.nftFamily, f.nftTable, "prerouting", "{ type filter hook prerouting priority mangle; policy accept; }")
	runCmd("nft", "add", "chain", f.nftFamily, f.nftTable, "output", "{ type route hook output priority mangle; policy accept; }")
	// nat 链单独复核：部分内核/nft 版本不支持 ip6 家族的 NAT（NAT66）。此时只跳过
	// 该家族的 DNS 重定向（TProxy 主链路不受影响）并明确记日志，而不是让整次启用
	// 失败——否则用户会连 IPv6 透明代理本身一起失去。
	dnsRedirect := runCmd("nft", "add", "chain", f.nftFamily, f.nftTable, "dstnat", "{ type nat hook prerouting priority -100; policy accept; }") == nil
	natOutput := runCmd("nft", "add", "chain", f.nftFamily, f.nftTable, "nat_output", "{ type nat hook output priority -100; policy accept; }") == nil
	if !dnsRedirect {
		logx.Warn(logx.ModuleTproxy, "family=%s nat chain not created, dns redirect skipped for this family (kernel/nft may not support %s nat)", f.label, f.label)
	}

	// 6. 本机地址与私有网段绕过
	runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "prerouting", "fib", "daddr", "type", "local", "return")
	runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "prerouting", f.addrKw, "daddr", "@private_ips", "return")
	runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "output", f.addrKw, "daddr", "@private_ips", "return")

	// 7a. 目的绕过
	for _, rule := range dstExceptions {
		rule = stripComment(rule)
		if rule == "" {
			continue
		}
		typ, ipNet, proto, portVal, err := parseTproxyException(rule)
		if err != nil {
			logx.Warn(logx.ModuleTproxy, "invalid destination bypass rule skipped: rule=%q err=%v", rule, err)
			continue
		}

		if typ == "ip" {
			// 网段绕过只作用于同家族：IPv6 绕过进 ip6 表，IPv4 绕过进 ip 表。
			if isV6Net(ipNet) != f.isV6 {
				continue
			}
			cidr := ipNet.String()
			// TProxy 劫持
			runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "prerouting", f.addrKw, "daddr", cidr, "return")
			runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "output", f.addrKw, "daddr", cidr, "return")
			// DNS 重定向
			if dnsRedirect {
				runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "dstnat", f.addrKw, "daddr", cidr, "return")
			}
			if proxyLocal && natOutput {
				runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "nat_output", f.addrKw, "daddr", cidr, "return")
			}
		} else if typ == "port" {
			// 端口绕过与家族无关，两个家族都要下发
			// 将集合作为一个完整的字符串参数
			protoExpr := "{tcp, udp}"
			if proto != "" {
				protoExpr = "{" + proto + "}"
			}
			// TProxy 劫持
			runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "prerouting", "meta", "l4proto", protoExpr, "th", "dport", strconv.Itoa(portVal), "return")
			runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "output", "meta", "l4proto", protoExpr, "th", "dport", strconv.Itoa(portVal), "return")
			// DNS 重定向
			if dnsRedirect {
				runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "dstnat", "meta", "l4proto", protoExpr, "th", "dport", strconv.Itoa(portVal), "return")
			}
			if proxyLocal && natOutput {
				runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "nat_output", "meta", "l4proto", protoExpr, "th", "dport", strconv.Itoa(portVal), "return")
			}
		}
	}

	// 7b. 源绕过（仅支持 IP/CIDR，复用同一套解析以统一 IPv4/IPv6 处理）
	for _, rule := range srcExceptions {
		rule = stripComment(rule)
		if rule == "" {
			continue
		}
		typ, ipNet, _, _, err := parseTproxyException(rule)
		if err != nil || typ != "ip" {
			logx.Warn(logx.ModuleTproxy, "source bypass supports IP/CIDR only, invalid rule ignored: rule=%q", rule)
			continue
		}
		if isV6Net(ipNet) != f.isV6 {
			continue
		}
		cidr := ipNet.String()
		runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "prerouting", f.addrKw, "saddr", cidr, "return")
		if dnsRedirect {
			runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "dstnat", f.addrKw, "saddr", cidr, "return")
		}
	}

	// 8. TProxy 劫持规则
	// 使用集合字符串 "{tcp,udp}"
	runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "prerouting", "meta", "l4proto", "{tcp,udp}", "tproxy", "to", fmt.Sprintf(":%d", port), "meta", "mark", "set", "1", "accept")
	runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "output", "meta", "mark", "0xff", "return")
	runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "output", "socket", "mark", "0xff", "return")
	if proxyLocal {
		runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "output", "meta", "l4proto", "{tcp,udp}", "meta", "mark", "set", "1", "accept")
	}

	// 9. DNS 重定向
	// 内核的 DNS 监听为双栈（dns.listen 写 0.0.0.0:1053 时 mihomo 绑定 [::]:1053），
	// 因此 IPv6 侧 redirect 到 :1053 同样有进程接听。
	if dnsRedirect {
		runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "dstnat", "udp", "dport", "53", "redirect", "to", ":1053")
		runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "dstnat", "tcp", "dport", "53", "redirect", "to", ":1053")
	}
	if proxyLocal && natOutput {
		runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "nat_output", "meta", "mark", "0xff", "return")
		runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "nat_output", "socket", "mark", "0xff", "return")
		runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "nat_output", "udp", "dport", "53", "redirect", "to", ":1053")
		runCmd("nft", "add", "rule", f.nftFamily, f.nftTable, "nat_output", "tcp", "dport", "53", "redirect", "to", ":1053")
	}

	// 10. 结果校验：三条关键产物逐一探测，任一缺失即视为本家族未生效。
	if !f.hasNftTable() {
		return fmt.Errorf("%s：nftables 表 %s/%s 未建成，规则未生效（请确认 nft 可用且权限充足）", f.label, f.nftFamily, f.nftTable)
	}
	if !f.hasFwmarkRule() {
		return fmt.Errorf("%s：策略路由（fwmark %s → table %s）未建成，被标记流量会被导入黑洞，已按失败处理", f.label, tproxyFwmark, tproxyTableID)
	}
	if !f.hasLocalRoute() {
		return fmt.Errorf("%s：策略路由表 %s 缺少 local 路由，TProxy 无法接收被劫持流量，已按失败处理", f.label, tproxyTableID)
	}
	return nil
}

// DisableTProxyRules 清理本模块写入的 nftables 表与策略路由（两个家族都清）。
//
// 关键点在于「各项独立探测、存在才删」：
//
// enable 先写策略路由（ip rule / ip route）再建 nft 表。若建表失败（nft 未安装、
// 权限不足），就会出现「有策略路由、无 nft 表」的中间状态。此前实现把 ip 规则的
// 清理放在「nft 表存在」的判定之后，导致这种情形下策略路由永远清不掉——流量被
// 导入空的 table 100 → 持续断网。
//
// 因此每项资源各自探测：有残留才执行删除，既不会漏删（不再被 nft 表的存在性
// 绑架），也不会对不存在的对象执行 del 而徒增错误。IPv6 家族同样处理，且不按
// 开关状态裁剪——否则关掉开关后就再也清不掉上一次遗留的 IPv6 规则。
//
// 幂等，可重复调用。
func DisableTProxyRules() {
	removed := make([]string, 0, len(tproxyFamilies)*3)

	for _, f := range tproxyFamilies {
		if f.hasNftTable() {
			if err := exec.Command("nft", "delete", "table", f.nftFamily, f.nftTable).Run(); err != nil {
				logx.Error(logx.ModuleTproxy, "failed to delete nftables table: family=%s err=%v", f.label, err)
			} else {
				removed = append(removed, f.label+" nft table")
			}
		}

		if f.hasLocalRoute() {
			if err := exec.Command("ip", f.ipFlag, "route", "del", "local", f.defaultRt, "dev", "lo", "table", tproxyTableID).Run(); err != nil {
				logx.Error(logx.ModuleTproxy, "failed to delete policy route: family=%s err=%v", f.label, err)
			} else {
				removed = append(removed, f.label+" policy route")
			}
		}

		if f.hasFwmarkRule() {
			if err := exec.Command("ip", f.ipFlag, "rule", "del", "fwmark", tproxyFwmark, "table", tproxyTableID).Run(); err != nil {
				logx.Error(logx.ModuleTproxy, "failed to delete routing rule: family=%s err=%v", f.label, err)
			} else {
				removed = append(removed, f.label+" routing rule")
			}
		}
	}

	if len(removed) == 0 {
		logx.Debug(logx.ModuleTproxy, "no leftover rules found, nothing to clean")
		return
	}
	logx.Info(logx.ModuleTproxy, "cleanup done: removed=%s", strings.Join(removed, ", "))
}

// stripComment 去除行尾 # 注释，并 trim 空格，返回纯净的规则部分
func stripComment(line string) string {
	line = strings.TrimSpace(line)
	if idx := strings.Index(line, "#"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	return line
}
