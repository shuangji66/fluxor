package config

// 本文件定义「分文件持久化」后的磁盘结构体（settings / rules / tunnels / meta / tproxy / tun）。
//
// 设计原则：**一个文件只有一个写入者**（见 store.go 的类型注释），因此磁盘结构体按
// 「谁来写」切分，而不是按字段类型切分：
//
//	settings.json          ← 订阅配置接口（保存并应用 / 订阅增删改 / 手工节点编辑）
//	rules.json             ← 自定义规则接口（三作用域）
//	tunnels.json           ← 流量隧道接口（三作用域）
//	subscription-meta.json ← 订阅更新与元数据流程（手动更新 / 定时更新 / 启动 ensure）
//	tproxy.json            ← TProxy 开关与绕过列表
//	tun.json               ← TUN 运行参数（tun 包自持；不含 enable，见 TunFile）
//
// SubscribeConfig 仍是**对外视图**（HTTP 请求体与响应体、配置生成链路的输入）：
// 它由各 store 组装而来，规则/隧道/元数据字段都是派生数据，任何写接口都不再信它。

// Settings 是 settings.json 的内容：全局设置 + 订阅注册表 + 自定义模式的手工节点。
//
// 这里刻意只保留「订阅的身份与拉取参数」，不带规则/隧道/元数据：那三类数据的写入者
// 完全不同（规则接口、隧道接口、更新流程），放在一起就会重新出现「改一处要写全部」。
type Settings struct {
	ProxyPort          int    `json:"proxy_port"`
	TproxyPort         int    `json:"tproxy_port"`
	PanelPort          int    `json:"panel_port"`
	PanelSecret        string `json:"panel_secret"`
	RuleGroup          string `json:"rule_group"`
	UIPanel            string `json:"ui_panel"`
	MetaBackendURL     string `json:"meta_backend_url"`
	Mode               string `json:"mode"`
	ActiveSubscription string `json:"active_subscription"`
	// IPv6 与 DNSIPv6 是生成期字段（写入 config.yaml 的 ipv6 / dns.ipv6）。
	// 默认值 true 由 newSettingsStore 的 Init 填充（bool 零值为 false，不能靠零值）。
	IPv6          bool              `json:"ipv6"`
	DNSIPv6       bool              `json:"dns_ipv6"`
	Subscriptions []SubscriptionRef `json:"subscriptions"`
	CustomNodes   []CustomNode      `json:"custom_nodes,omitempty"`
}

// TunFile 是 tun.json 的内容：TUN 的运行参数（**不含 enable**）。
//
// 两条刻意的设计：
//  1. **稀疏**：只保存与模板默认值（configgen.TunBlock）不同的键，nil 表示「用默认值」。
//     用户把某项改回默认时该键被删除，从此跟随模板升级；默认值不落盘。
//  2. **没有 enable**：config.yaml 里的 tun.enable 恒为 false，任何启动都不得自动接管
//     宿主网络（NAS 上一旦配错就会失联）。开启只通过 /config/tun/state 的运行时 PATCH
//     完成，内核/面板重启即归零，且**不做回放**。
type TunFile struct {
	Stack   *string `json:"stack,omitempty"`
	Device  *string `json:"device,omitempty"`
	DNSMode *string `json:"dns_mode,omitempty"`
	IPv6    *string `json:"ipv6,omitempty"`
}

// SubscriptionRef 是订阅注册表里的一项：身份（名字）与拉取参数。
//
// 与 Subscription（视图）的区别：不含自定义规则、流量隧道与更新元数据——那三者
// 分别由 rules.json / tunnels.json / subscription-meta.json 按订阅名承载。
type SubscriptionRef struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	UpdateInterval int    `json:"update_interval"`
	HealthInterval int    `json:"health_interval"`
	Prefix         string `json:"prefix"`
}

// RulesFile 是 rules.json 的内容：三作用域的自定义规则。
//
// 三个作用域与 AGENTS 3.9 的划分一致：
//   - Merge      融合模式的档位（base / full）；
//   - CustomMode 自定义模式（独立一份，不按档位分表）；
//   - BySub      切换模式的订阅级规则，键为订阅名。
//
// 按订阅名做键意味着订阅改名/删除后可能留下孤儿条目：删除由 GCResources 清理，
// 改名由 SaveSettings 的搬迁逻辑处理（见 renameResources）。
type RulesFile struct {
	Merge      map[string][]CustomRule `json:"merge,omitempty"`
	CustomMode []CustomRule            `json:"custom_mode,omitempty"`
	BySub      map[string][]CustomRule `json:"subscriptions,omitempty"`
}

// TunnelsFile 是 tunnels.json 的内容，结构与 RulesFile 同构（作用域划分相同）。
type TunnelsFile struct {
	Merge      map[string][]Tunnel `json:"merge,omitempty"`
	CustomMode []Tunnel            `json:"custom_mode,omitempty"`
	BySub      map[string][]Tunnel `json:"subscriptions,omitempty"`
}

// MetaFile 是 subscription-meta.json 的内容：各订阅最近一次更新的时间与机场元数据。
//
// 单独成文件的最大好处：定时更新（可配置间隔 × N 个订阅）只重写这一份，不再连带
// 重写订阅注册表与规则/隧道；而这些数据**丢了可以重抓**，是唯一允许「失败即留空」的一类。
type MetaFile struct {
	Subscriptions map[string]SubscriptionMeta `json:"subscriptions,omitempty"`
}

// SubscriptionMeta 单个订阅的更新元数据。
type SubscriptionMeta struct {
	UpdatedAt string         `json:"updated_at,omitempty"`
	Info      map[string]any `json:"subscription_info,omitempty"`
}

// NewRulesFile 返回空白的规则文件（两个 map 都必须非 nil，否则首次写入会 panic）。
func NewRulesFile() RulesFile {
	return RulesFile{Merge: map[string][]CustomRule{}, BySub: map[string][]CustomRule{}}
}

// NewTunnelsFile 返回空白的隧道文件（两个 map 都必须非 nil）。
func NewTunnelsFile() TunnelsFile {
	return TunnelsFile{Merge: map[string][]Tunnel{}, BySub: map[string][]Tunnel{}}
}

// NewMetaFile 返回空白的元数据文件。
func NewMetaFile() MetaFile {
	return MetaFile{Subscriptions: map[string]SubscriptionMeta{}}
}

// TproxyFile 是 tproxy.json 的内容：TProxy 开关与两条绕过列表。
//
// 由 tproxy 包用自己的 Store 读写（该文件的读写者只有它一个），结构体放在这里是为了
// 让迁移能在同一处完成拆分——它只是**持久化布局**，防火墙语义仍归 tproxy 包。
//
// 两条列表**不能**用 omitempty。它们在语义上要区分三种取值，而 omitempty 会把
// 「空列表」和「未设置」都序列化成「键缺失」，把两者合并成同一个状态：
//   - nil        → 写出 `null`      → 读回 nil  → 用代码里的预填模板；
//   - 空列表     → 写出 `[]`        → 读回 `[]`  → 用户显式清空，一条绕过都不要；
//   - 非空列表   → 写出内容本身。
//
// 加上 omitempty 后，「用户把列表清空」在磁盘上等于「键缺失」，重启时预填模板会
// 静默回来——用户明确表达的「不使用任何绕过」被无声撤销，前后两次运行行为不一致。
//
// 预填内容仍然不落盘：与模板完全一致的列表由 tproxy 包的 normalizeExceptions
// 收敛为 nil，因此文件体积不会因为去掉 omitempty 而变大。
type TproxyFile struct {
	Enabled       bool     `json:"enabled"`
	ProxyLocal    bool     `json:"proxy_local"`
	IPv6          bool     `json:"ipv6"`
	DstExceptions []string `json:"dst_exceptions"`
	SrcExceptions []string `json:"src_exceptions"`
}
