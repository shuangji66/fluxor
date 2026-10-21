package netinfo

import (
	"net/url"
	"strings"
	"testing"

	"fluxor/internal/configgen"
)

// TestProxyLookupDomainsCoveredByFullRuleset 校验「经代理出口查询公网 IP」的地址
// 在 full（详细）规则集模板里都有显式 DOMAIN 规则，且排在 networktest 规则集之前。
//
// 背景：这些域名会命中 networktest 规则集，该组默认选中「🎯 全球直连」。一旦模板漏写，
// 概览页的节点归属地会静默变成本机归属地——没有报错，只是数值悄悄错了。因此两处清单
// 必须同步：netinfo 的 proxyIPv4/IPv6LookupURLs 与 configgen/rules_full.go。
func TestProxyLookupDomainsCoveredByFullRuleset(t *testing.T) {
	lines, err := configgen.MergeRuleSetRuleLines(configgen.RuleGroupFull)
	if err != nil {
		t.Fatalf("读取 full 规则集模板失败: %v", err)
	}

	indexOf := func(prefix string) int {
		for i, line := range lines {
			if strings.HasPrefix(line, prefix) {
				return i
			}
		}
		return -1
	}
	networktestIdx := indexOf("RULE-SET,networktest,")
	if networktestIdx < 0 {
		t.Fatalf("full 规则集模板里找不到 networktest 规则集，测试前提已变: %v", lines)
	}

	seen := make(map[string]bool)
	for _, rawURL := range append(append([]string{}, proxyIPv4LookupURLs...), proxyIPv6LookupURLs...) {
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Host == "" {
			t.Fatalf("查询地址格式非法: %s", rawURL)
		}
		host := parsed.Hostname()
		if seen[host] {
			continue
		}
		seen[host] = true

		want := "DOMAIN," + host + ",🚀 节点选择"
		idx := -1
		for i, line := range lines {
			if line == want {
				idx = i
				break
			}
		}
		if idx < 0 {
			t.Fatalf("full 规则集缺少规则 %q：代理出口 IP 查询会被规则集分流，归属地显示错误", want)
		}
		if idx > networktestIdx {
			t.Fatalf("规则 %q 必须排在 RULE-SET,networktest 之前（实际位置 %d，networktest 在 %d）", want, idx, networktestIdx)
		}
	}
}
