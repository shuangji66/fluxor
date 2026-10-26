package configgen

import (
	"fluxor/internal/config"
	"fluxor/internal/configcheck"
	"fluxor/internal/nodespec"
	"fmt"

	"gopkg.in/yaml.v3"
)

// GenerateCustomConfig 生成自定义模式的 config.yaml。
//
// 产物 = 基础模板骨架（含 dns 块）+ 手工节点拼成的 proxies 块 + 标准规则集。
// 与融合模式的关键差异：
//   - 不写 proxy-providers：节点全部来自 proxies，与订阅无关（订阅列表只是历史数据）；
//   - 规则集固定 standard（RuleGroupBase）：界面在自定义模式下隐藏档位选择，
//     生成时也无视 cfg.RuleGroup 里残留的档位，避免「界面不显示、实际按 full 生成」；
//   - 自定义规则取自定义模式自己的那一份（config.RuleScopeCustom）——与融合模式的
//     规则完全独立，两种模式各改各的；模板层面的代理组与内置规则仍与标准档位一致。
func GenerateCustomConfig(cfg config.SubscribeConfig) error {
	doc, err := configcheck.ParseDoc([]byte(configTemplate))
	if err != nil {
		return fmt.Errorf("解析配置模板失败: %w", err)
	}

	if err := applyFluxorFields(doc, cfg); err != nil {
		return err
	}

	proxies, err := buildProxiesNode(cfg.CustomNodes)
	if err != nil {
		return err
	}
	if proxies != nil {
		doc.SetNode("proxies", proxies)
	}

	// 模板固定标准档位；自定义规则只取自定义模式的那一份（与融合模式互不影响）
	if err := appendRuleSet(doc, cfg, config.RuleGroupBase); err != nil {
		return err
	}
	if err := applyCustomRules(doc, config.RuleScopeCustom, cfg.CustomModeRules); err != nil {
		return err
	}
	// 流量隧道：在 proxies 之后注入，proxy 才能解析到手工节点名
	if err := applyTunnels(doc, config.RuleScopeCustom, cfg.CustomModeTunnels); err != nil {
		return err
	}

	if err := applyDNSBlock(doc, cfg.DNSIPv6); err != nil {
		return err
	}

	return writeConfigTarget(doc)
}

// buildProxiesNode 把手工节点拼成 proxies 序列节点。无节点时返回 nil（不下发空块）。
//
// 每个节点按协议字段声明顺序落键（name / type 在前），值由 nodespec 补齐默认值后
// 逐项编码：整数保持整数、布尔保持布尔、列表保持序列、键值对与嵌套块（ws-opts /
// reality-opts …）保持映射——若统一按字符串拼接，内核会因类型不符拒绝加载
// （端口 443 写成 "443" 尚可被弱类型解码接受，但 alpn / reserved 这类序列、
// 嵌套选项块写成字符串会直接解析失败）。
func buildProxiesNode(nodes []config.CustomNode) (*yaml.Node, error) {
	if len(nodes) == 0 {
		return nil, nil
	}

	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for i, node := range nodes {
		proto, ok := nodespec.Lookup(node.Type)
		if !ok {
			return nil, fmt.Errorf("第 %d 个节点 %s: 不支持的协议类型 %s", i+1, node.Name, node.Type)
		}
		// WritableOrdered：补齐默认值后剔除空项（含全空的嵌套块），
		// 只把内核真正需要的键写进配置
		kvs, err := nodespec.WritableOrdered(node)
		if err != nil {
			return nil, fmt.Errorf("第 %d 个节点 %s: %w", i+1, node.Name, err)
		}

		item := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		appendMap(item, "name", node.Name)
		appendMap(item, "type", proto.Type)
		appendKVs(item, kvs)
		seq.Content = append(seq.Content, item)
	}
	return seq, nil
}

// appendKVs 把有序字段写进映射节点（嵌套块与键值对递归展开）。
func appendKVs(parent *yaml.Node, kvs []nodespec.KV) {
	for _, kv := range kvs {
		switch kv.Kind {
		case nodespec.KindGroup:
			nested := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			appendKVs(nested, kv.Children)
			appendMapNode(parent, kv.Key, nested)

		case nodespec.KindMap:
			mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			pairs, _ := kv.Value.(map[string]string)
			// 键值对按字典序落键：map 的遍历顺序随机，不排序会让同一份配置
			// 每次写出不同的字节序，破坏「产物可逐字节比对」这一约定
			for _, key := range nodespec.SortedKeys(pairs) {
				appendMap(mapping, key, pairs[key])
			}
			appendMapNode(parent, kv.Key, mapping)

		default:
			appendMap(parent, kv.Key, kv.Value)
		}
	}
}
