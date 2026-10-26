package config

import (
	"fmt"
	"sync"

	"fluxor/internal/logx"
)

// 本文件是「分文件持久化」的唯一入口层：
//   - 四个 Store 单例（settings / rules / tunnels / meta；tproxy 的文件由 tproxy 包用
//     同一个 Store 类型自持，因为它的读写者只有那一个包）；
//   - 每个数据类别一对「读 / 写」函数，写函数只碰自己的文件，写完重新组装 Current；
//   - 孤儿资源回收与订阅改名的资源搬迁。
//
// 调用约定（务必遵守，否则会破坏 ACID 之外的那点一致性）：
//  1. 写函数内部完成「改内存 + 落盘 + 重组装」，调用方不要再手动给 Current 赋值；
//  2. 组装视图 Current 是**派生数据**，任何写接口都不应把它整份覆盖回磁盘
//     （这正是旧实现里「删了的规则又回来」那类 bug 的根源）；
//  3. 不得在持有 config.Mu 的情况下调用这些写函数（写函数末尾会取 Mu 重组装）。

var (
	// settingsStore 全局设置 + 订阅注册表 + 手工节点。
	settingsStore = newSettingsStore()
	// rulesStore 三作用域自定义规则。
	rulesStore = NewStore[RulesFile]("rules.json", &FluxorRulesFile, func(f *RulesFile) {
		// 两个 map 都必须非 nil：写入侧直接往 map 里塞键，nil map 会 panic
		if f.Merge == nil {
			f.Merge = map[string][]CustomRule{}
		}
		if f.BySub == nil {
			f.BySub = map[string][]CustomRule{}
		}
	})
	// tunnelsStore 三作用域流量隧道。
	tunnelsStore = NewStore[TunnelsFile]("tunnels.json", &FluxorTunnelsFile, func(f *TunnelsFile) {
		if f.Merge == nil {
			f.Merge = map[string][]Tunnel{}
		}
		if f.BySub == nil {
			f.BySub = map[string][]Tunnel{}
		}
	})
	// metaStore 各订阅的更新元数据（丢失可重抓）。
	metaStore = NewStore[MetaFile]("subscription-meta.json", &FluxorMetaFile, func(f *MetaFile) {
		if f.Subscriptions == nil {
			f.Subscriptions = map[string]SubscriptionMeta{}
		}
	})

	// assembleMu 串行化「读多个 store → 组装 Current」，避免两次组装互相交错。
	// 只保护组装过程本身，不跨 store 持锁（各 store 的锁在 View 内部即取即放）。
	assembleMu sync.Mutex
)

// 全局设置的默认值（旧单文件实现的默认值语义，原样保留）。
const (
	defaultProxyPort  = 7890
	defaultPanelPort  = 9090
	defaultTproxyPort = 7898
)

func newSettingsStore() *Store[Settings] {
	s := NewStore[Settings]("settings.json", &FluxorSettingsFile, func(v *Settings) {
		// 读盘前填默认值：JSON 里存在的键会覆盖它们；端口 0 表示禁用，
		// 因此「缺失 → 默认值」只能在这里做（Normalize 无法区分缺失与显式 0）。
		v.ProxyPort = defaultProxyPort
		v.PanelPort = defaultPanelPort
		v.TproxyPort = defaultTproxyPort
		v.Mode = ModeMerge
		v.RuleGroup = RuleGroupBase
		v.UIPanel = "metacubexd"
		// IPv6 两个开关默认开启（与模板历史行为一致）。bool 的零值是 false，
		// 不能靠 Normalize 区分「缺失」与「显式关闭」，只能在读盘前预置。
		v.IPv6 = true
		v.DNSIPv6 = true
	})
	s.Normalize = func(v *Settings) {
		// 读盘后修容器与非必填字符串（JSON 里可能显式写成 null / ""）
		if v.Subscriptions == nil {
			v.Subscriptions = []SubscriptionRef{}
		}
		if v.CustomNodes == nil {
			v.CustomNodes = []CustomNode{}
		}
		if v.Mode == "" {
			v.Mode = ModeMerge
		}
		if v.RuleGroup == "" {
			v.RuleGroup = RuleGroupBase
		}
		if v.UIPanel == "" {
			v.UIPanel = "metacubexd"
		}
		// 分裂状态（IPv6 关闭却仍返回 AAAA）在写入口已归一化，读盘时再兜一次，
		// 防止手改 settings.json 留下自相矛盾的组合。
		if !v.IPv6 {
			v.DNSIPv6 = false
		}
	}
	return s
}

// LoadAll 载入全部配置文件并在必要时迁移旧布局，最后组装 Current。
//
// 启动时调用一次。单个文件损坏不会阻止面板启动——该文件会退回默认值并拒绝写入，
// 其余文件照常工作（这正是拆分带来的隔离性）。
func LoadAll() {
	MigrateLegacyFiles()

	if err := settingsStore.Load(); err != nil {
		logx.Error(logx.ModuleConfig, "failed to load %s: %v", settingsStore.Name, err)
	}
	if err := rulesStore.Load(); err != nil {
		logx.Error(logx.ModuleConfig, "failed to load %s: %v", rulesStore.Name, err)
	}
	if err := tunnelsStore.Load(); err != nil {
		logx.Error(logx.ModuleConfig, "failed to load %s: %v", tunnelsStore.Name, err)
	}
	if err := metaStore.Load(); err != nil {
		logx.Error(logx.ModuleConfig, "failed to load %s: %v", metaStore.Name, err)
	}

	assembleCurrent()
	// 启动时**只探测不回收**。
	//
	// 回收需要「当前订阅表」作为判据，而它来自 settings.json：settings 不可信时
	// （内容损坏 / 读盘失败）内存态是空订阅表，此时若照常回收，会把 rules.json /
	// tunnels.json / subscription-meta.json 里所有按订阅名存放的条目判成孤儿并删除，
	// 且这三份文件没有备份——一次解析失败就能毁掉用户全部自定义规则与隧道。
	//
	// 孤儿本身不进生成链路（生成按订阅名查找），回收只是「让文件干净」而非正确性要求，
	// 因此完全可以推迟到下一次 SaveSettings（那时 settings 一定刚被成功写入）。
	// 这里只记一条日志，让排障时知道文件里有待回收的条目。
	probeOrphanResources()

	Mu.RLock()
	defer Mu.RUnlock()
	logx.Info(logx.ModuleConfig, "config loaded: file=%s subscriptions=%d custom_nodes=%d",
		settingsStore.FilePath(), len(Current.Subscriptions), len(Current.CustomNodes))
}

// assembleCurrent 依据四个 store 重新组装只读视图 Current。
//
// 依次访问各 store（不嵌套持锁），最后在 Mu 内整体替换：任何时刻读到的 Current 都是
// 「某一个完整状态」而不是拼装到一半的中间态。
func assembleCurrent() {
	assembleMu.Lock()
	defer assembleMu.Unlock()

	var cfg SubscribeConfig

	settingsStore.View(func(s *Settings) {
		cfg.ProxyPort = s.ProxyPort
		cfg.TproxyPort = s.TproxyPort
		cfg.PanelPort = s.PanelPort
		cfg.PanelSecret = s.PanelSecret
		cfg.RuleGroup = s.RuleGroup
		cfg.UIPanel = s.UIPanel
		cfg.MetaBackendURL = s.MetaBackendURL
		cfg.Mode = s.Mode
		cfg.ActiveSubscription = s.ActiveSubscription
		cfg.IPv6 = s.IPv6
		cfg.DNSIPv6 = s.DNSIPv6
		cfg.CustomNodes = copyCustomNodes(s.CustomNodes)
		cfg.Subscriptions = make([]Subscription, 0, len(s.Subscriptions))
		for _, ref := range s.Subscriptions {
			cfg.Subscriptions = append(cfg.Subscriptions, Subscription{
				Name:           ref.Name,
				URL:            ref.URL,
				UpdateInterval: ref.UpdateInterval,
				HealthInterval: ref.HealthInterval,
				Prefix:         ref.Prefix,
			})
		}
	})

	rulesStore.View(func(f *RulesFile) {
		cfg.MergeCustomRules = copyRuleMap(f.Merge)
		cfg.CustomModeRules = copyRules(f.CustomMode)
		for i := range cfg.Subscriptions {
			cfg.Subscriptions[i].CustomRules = copyRules(f.BySub[cfg.Subscriptions[i].Name])
		}
	})

	tunnelsStore.View(func(f *TunnelsFile) {
		cfg.MergeTunnels = copyTunnelMap(f.Merge)
		cfg.CustomModeTunnels = CopyTunnels(f.CustomMode)
		for i := range cfg.Subscriptions {
			cfg.Subscriptions[i].Tunnels = CopyTunnels(f.BySub[cfg.Subscriptions[i].Name])
		}
	})

	metaStore.View(func(m *MetaFile) {
		for i := range cfg.Subscriptions {
			if meta, ok := m.Subscriptions[cfg.Subscriptions[i].Name]; ok {
				cfg.Subscriptions[i].UpdatedAt = meta.UpdatedAt
				cfg.Subscriptions[i].SubscriptionInfo = copyAnyMap(meta.Info)
			}
		}
	})

	Mu.Lock()
	Current = cfg
	Mu.Unlock()
}

// SaveSettings 用视图里的「自有字段」覆盖 settings.json。
//
// 只取全局标量、订阅注册表（身份与拉取参数）与手工节点——规则、隧道、更新元数据
// 由各自的 store 持有，视图里带着的这三类字段会被**直接忽略**（前端把整份配置
// 传回来是常态，忽略而不是合并，才能避免「过期快照覆盖服务端」）。
//
// 另外处理两件与订阅身份相关的维护工作：订阅改名的资源搬迁、订阅删除后的孤儿回收。
func SaveSettings(view SubscribeConfig) error {
	next := Settings{
		ProxyPort:          view.ProxyPort,
		TproxyPort:         view.TproxyPort,
		PanelPort:          view.PanelPort,
		PanelSecret:        view.PanelSecret,
		RuleGroup:          view.RuleGroup,
		UIPanel:            view.UIPanel,
		MetaBackendURL:     view.MetaBackendURL,
		Mode:               view.Mode,
		ActiveSubscription: view.ActiveSubscription,
		IPv6:               view.IPv6,
		DNSIPv6:            view.DNSIPv6,
		Subscriptions:      make([]SubscriptionRef, 0, len(view.Subscriptions)),
		CustomNodes:        copyCustomNodes(view.CustomNodes),
	}
	// 写入口归一化：IPv6 关闭时不允许 DNS 继续返回 AAAA（见 NormalizeIPv6Flags）。
	if !next.IPv6 {
		next.DNSIPv6 = false
	}
	for _, sub := range view.Subscriptions {
		next.Subscriptions = append(next.Subscriptions, SubscriptionRef{
			Name:           sub.Name,
			URL:            sub.URL,
			UpdateInterval: sub.UpdateInterval,
			HealthInterval: sub.HealthInterval,
			Prefix:         sub.Prefix,
		})
	}

	var renames []renamePair
	if err := settingsStore.Update(func(s *Settings) error {
		renames = detectRenames(*s, next)
		*s = next
		return nil
	}); err != nil {
		return err
	}

	renameResources(renames)
	GCResources()
	assembleCurrent()
	return nil
}

// ——— 自定义规则（三作用域）———

// SubscriptionRules 取切换模式下某订阅的自定义规则（副本）。
func SubscriptionRules(name string) []CustomRule {
	var out []CustomRule
	rulesStore.View(func(f *RulesFile) { out = copyRules(f.BySub[name]) })
	return out
}

// UpdateSubscriptionRules 在 rules.json 的写锁内完成「读—改—写」。
//
// 闭包拿到的 current 是**此刻**的列表（不是请求开始时的快照），返回的列表会被原样
// 落盘：这样「读列表 → 改列表 → 整份写回」是一个不可分割的操作，同一作用域的并发
// 改动不会互相覆盖（此前用「快照 + SaveXxx(整份)」的形态存在丢失更新的窗口）。
//
// 闭包返回的非 nil 错误视为「调用方的业务错误」（找不到目标等），此时不落盘、原样返回，
// 由上层决定回 400 还是 500。
func UpdateSubscriptionRules(name string, mutate func(current []CustomRule) ([]CustomRule, error)) error {
	return updateRuleStore(rulesStore, name, mutate)
}

// TemplateRules 取模板级作用域（融合档位 base/full，或自定义模式的 custom）的规则。
func TemplateRules(scope string) []CustomRule {
	var out []CustomRule
	rulesStore.View(func(f *RulesFile) { out = CopyTemplateRules(f, scope) })
	return out
}

// UpdateTemplateRules 在 rules.json 的写锁内完成模板级作用域的「读—改—写」。
func UpdateTemplateRules(scope string, mutate func(current []CustomRule) ([]CustomRule, error)) error {
	if !IsValidRuleScope(scope) {
		return fmt.Errorf("保存规则失败：未知的作用域 %q", scope)
	}
	return updateRuleStore(rulesStore, "", mutate, scope)
}

// updateRuleStore 规则文件的公共「锁内读—改—写」实现：subscription 与 scope 二选一。
func updateRuleStore(store *Store[RulesFile], subscription string, mutate func([]CustomRule) ([]CustomRule, error), scope ...string) error {
	if subscription == "" && len(scope) == 0 {
		return fmt.Errorf("保存规则失败：缺少作用域")
	}
	if err := store.Update(func(f *RulesFile) error {
		var current []CustomRule
		if subscription != "" {
			current = f.BySub[subscription]
		} else {
			current = CopyTemplateRules(f, scope[0])
		}
		next, err := mutate(current)
		if err != nil {
			return err
		}
		if subscription != "" {
			setRuleList(f.BySub, subscription, next)
			return nil
		}
		if scope[0] == RuleScopeCustom {
			f.CustomMode = copyRules(next)
			return nil
		}
		setRuleList(f.Merge, scope[0], next)
		return nil
	}); err != nil {
		return err
	}
	assembleCurrent()
	return nil
}

// ——— 流量隧道（三作用域）———

// SubscriptionTunnels 取切换模式下某订阅的流量隧道（副本）。
func SubscriptionTunnels(name string) []Tunnel {
	var out []Tunnel
	tunnelsStore.View(func(f *TunnelsFile) { out = CopyTunnels(f.BySub[name]) })
	return out
}

// UpdateSubscriptionTunnels 在 tunnels.json 的写锁内完成「读—改—写」（语义同 UpdateSubscriptionRules）。
func UpdateSubscriptionTunnels(name string, mutate func(current []Tunnel) ([]Tunnel, error)) error {
	if name == "" {
		return fmt.Errorf("保存隧道失败：缺少订阅名")
	}
	return updateTunnelStore(tunnelsStore, name, mutate)
}

// TemplateTunnels 取模板级作用域的流量隧道。
func TemplateTunnels(scope string) []Tunnel {
	var out []Tunnel
	tunnelsStore.View(func(f *TunnelsFile) { out = CopyTemplateTunnels(f, scope) })
	return out
}

// UpdateTemplateTunnels 在 tunnels.json 的写锁内完成模板级作用域的「读—改—写」。
func UpdateTemplateTunnels(scope string, mutate func(current []Tunnel) ([]Tunnel, error)) error {
	if !IsValidRuleScope(scope) {
		return fmt.Errorf("保存隧道失败：未知的作用域 %q", scope)
	}
	return updateTunnelStore(tunnelsStore, "", mutate, scope)
}

// updateTunnelStore 隧道文件的公共「锁内读—改—写」实现。
func updateTunnelStore(store *Store[TunnelsFile], subscription string, mutate func([]Tunnel) ([]Tunnel, error), scope ...string) error {
	if subscription == "" && len(scope) == 0 {
		return fmt.Errorf("保存隧道失败：缺少作用域")
	}
	if err := store.Update(func(f *TunnelsFile) error {
		var current []Tunnel
		if subscription != "" {
			current = f.BySub[subscription]
		} else {
			current = CopyTemplateTunnels(f, scope[0])
		}
		next, err := mutate(current)
		if err != nil {
			return err
		}
		if subscription != "" {
			setTunnelList(f.BySub, subscription, next)
			return nil
		}
		if scope[0] == RuleScopeCustom {
			f.CustomMode = CopyTunnels(next)
			return nil
		}
		setTunnelList(f.Merge, scope[0], next)
		return nil
	}); err != nil {
		return err
	}
	assembleCurrent()
	return nil
}

// ——— 订阅元数据 ———

// SubscriptionMetaOf 取某订阅的更新元数据（副本）。
func SubscriptionMetaOf(name string) (string, map[string]any) {
	var updatedAt string
	var info map[string]any
	metaStore.View(func(f *MetaFile) {
		meta, ok := f.Subscriptions[name]
		if !ok {
			return
		}
		updatedAt = meta.UpdatedAt
		info = copyAnyMap(meta.Info)
	})
	return updatedAt, info
}

// SaveSubscriptionMeta 写入某订阅的更新元数据。
//
// 只写 subscription-meta.json：手动更新、定时更新、启动 ensure 都会落到这里，
// 不再连带重写订阅注册表与规则/隧道（这是拆分后最大的写放大消除点）。
func SaveSubscriptionMeta(name, updatedAt string, info map[string]any) error {
	if name == "" {
		return fmt.Errorf("保存订阅元数据失败：缺少订阅名")
	}
	// 空结果（时间与内容都没有）= 本轮没有任何可用信息，**不写也不删**。
	//
	// 融合模式下内核刚重载完就抓元数据、或机场不下发 subscription-userinfo 时都会得到
	// 空值；旧实现把这种情况当作「清空」删掉条目，于是上次已知的流量/到期被抹掉，
	// 卡片立刻变成「流量信息不可用」——而这正是最该用已知值兜住的时刻。
	if updatedAt == "" && len(info) == 0 {
		logx.Debug(logx.ModuleConfig, "no metadata to save for subscription %q, keeping previous values", name)
		return nil
	}
	if err := metaStore.Update(func(f *MetaFile) error {
		f.Subscriptions[name] = SubscriptionMeta{UpdatedAt: updatedAt, Info: copyAnyMap(info)}
		return nil
	}); err != nil {
		return err
	}
	assembleCurrent()
	return nil
}

// ——— 维护：孤儿回收与改名搬迁 ———

// renamePair 记录一次「同名不同名」的订阅改名。
type renamePair struct{ from, to string }

// detectRenames 识别订阅改名。
//
// 名字是订阅的唯一键，改名后其规则/隧道/元数据会变成孤儿。这里用「旧表独有 × 新表独有」
// 且 **URL 相同** 来配对：URL 恰好是「同一个订阅」最可靠的证据。同一 URL 在两侧都不唯一
// 时保守放弃（宁可留孤儿让 GC 清掉，也不把 A 的规则搬到 B 名下）。
func detectRenames(prev, next Settings) []renamePair {
	prevByName := map[string]string{}
	for _, ref := range prev.Subscriptions {
		prevByName[ref.Name] = ref.URL
	}
	nextByName := map[string]string{}
	for _, ref := range next.Subscriptions {
		nextByName[ref.Name] = ref.URL
	}

	// 两侧各自独有的名字，按 URL 归类
	removed := map[string][]string{} // url → 旧名字
	for name, url := range prevByName {
		if _, ok := nextByName[name]; !ok {
			removed[url] = append(removed[url], name)
		}
	}
	added := map[string][]string{} // url → 新名字
	for name, url := range nextByName {
		if _, ok := prevByName[name]; !ok {
			added[url] = append(added[url], name)
		}
	}

	var pairs []renamePair
	for url, oldNames := range removed {
		newNames := added[url]
		if url == "" || len(oldNames) != 1 || len(newNames) != 1 {
			continue
		}
		pairs = append(pairs, renamePair{from: oldNames[0], to: newNames[0]})
	}
	return pairs
}

// renameResources 把改名前的规则/隧道/元数据搬到新名字下。
func renameResources(pairs []renamePair) {
	if len(pairs) == 0 {
		return
	}
	for _, p := range pairs {
		if err := rulesStore.Update(func(f *RulesFile) error {
			if rules, ok := f.BySub[p.from]; ok {
				f.BySub[p.to] = rules
				delete(f.BySub, p.from)
			}
			return nil
		}); err != nil {
			logx.Error(logx.ModuleConfig, "failed to move rules on rename %q -> %q: %v", p.from, p.to, err)
		}
		if err := tunnelsStore.Update(func(f *TunnelsFile) error {
			if tunnels, ok := f.BySub[p.from]; ok {
				f.BySub[p.to] = tunnels
				delete(f.BySub, p.from)
			}
			return nil
		}); err != nil {
			logx.Error(logx.ModuleConfig, "failed to move tunnels on rename %q -> %q: %v", p.from, p.to, err)
		}
		if err := metaStore.Update(func(f *MetaFile) error {
			if meta, ok := f.Subscriptions[p.from]; ok {
				f.Subscriptions[p.to] = meta
				delete(f.Subscriptions, p.from)
			}
			return nil
		}); err != nil {
			logx.Error(logx.ModuleConfig, "failed to move metadata on rename %q -> %q: %v", p.from, p.to, err)
		}
		logx.Info(logx.ModuleConfig, "subscription renamed, resources moved: from=%q to=%q", p.from, p.to)
	}
}

// GCResources 清理注册表里已不存在的订阅所留下的规则/隧道/元数据。
//
// 这些孤儿不会进入配置生成链路（生成按订阅名查找，查不到即不注入），因此回收是
// 「让文件保持干净」而不是正确性要求——正因如此，拆文件才不需要跨文件事务。
//
// 调用方仅限 SaveSettings（settings 刚刚成功写入，订阅表一定是真相）。启动路径
// 走 probeOrphanResources：那时 settings 可能不可信，用它反推该删什么会毁数据。
func GCResources() {
	names, ok := subscriptionNamesForGC()
	if !ok {
		return
	}

	gcStore(rulesStore, names, func(f *RulesFile, keep map[string]struct{}) int {
		return dropMissing(f.BySub, keep)
	}, "orphan custom rules")
	gcStore(tunnelsStore, names, func(f *TunnelsFile, keep map[string]struct{}) int {
		return dropMissing(f.BySub, keep)
	}, "orphan tunnels")
	gcStore(metaStore, names, func(f *MetaFile, keep map[string]struct{}) int {
		return dropMissing(f.Subscriptions, keep)
	}, "orphan metadata")
}

// probeOrphanResources 只统计、不删除（启动路径用）。
//
// 与 GCResources 共用同一套判据，但把「发现孤儿」降级为一条日志：启动阶段 settings
// 的可靠性无法保证，宁可让文件里多留几条用不到的条目，也不能拿一份可能失真的订阅表
// 去删另一份文件里的数据。
func probeOrphanResources() {
	names, ok := subscriptionNamesForGC()
	if !ok {
		return
	}
	total := 0
	for _, n := range []int{
		probeStore(rulesStore, names, "orphan custom rules"),
		probeStore(tunnelsStore, names, "orphan tunnels"),
		probeStore(metaStore, names, "orphan metadata"),
	} {
		total += n
	}
	if total > 0 {
		logx.Info(logx.ModuleConfig,
			"found %d orphan resource entries left by removed subscriptions; they are unused and will be collected on the next settings save", total)
	}
}

// probeStore 统计单个 store 的孤儿条目数并记日志（只读）。
func probeStore[T any](store *Store[T], keep map[string]struct{}, label string) int {
	n := 0
	store.View(func(v *T) { n = probeOrphans(v, keep) })
	if n > 0 {
		logx.Info(logx.ModuleConfig, "orphan entries detected: %s in %s: count=%d", label, store.Name, n)
	}
	return n
}

// subscriptionNamesForGC 取出可作为回收判据的订阅名集合。
//
// ok=false 表示 settings 当前不可信（内容损坏或读盘失败，内存态是默认值），
// 此时任何以它为 keep 集合的删除都必须放弃。
func subscriptionNamesForGC() (map[string]struct{}, bool) {
	if settingsStore.Damaged() {
		logx.Error(logx.ModuleConfig,
			"settings store is not healthy (corrupt or unreadable); skipping resource garbage collection "+
				"to avoid deleting rules/tunnels/metadata that are still in use")
		return nil, false
	}
	names := map[string]struct{}{}
	settingsStore.View(func(s *Settings) {
		for _, ref := range s.Subscriptions {
			names[ref.Name] = struct{}{}
		}
	})
	return names, true
}

// gcStore 执行一次「删除名字不在 keep 中的条目」，仅在确有删除时落盘。
//
// 先 View 计数、再 Update 删除：**不得**在 View 回调里调用 Update——同一把
// sync.Mutex 不可重入，会造成自死锁（这里踩过一次）。
func gcStore[T any](store *Store[T], keep map[string]struct{}, drop func(*T, map[string]struct{}) int, label string) {
	orphans := 0
	store.View(func(v *T) { orphans = probeOrphans(v, keep) })
	if orphans == 0 {
		return
	}
	// 期间可能已被其它写入改变，Update 里再算一次，因此计数只用于「是否需要写」的快速判断
	if err := store.Update(func(v *T) error {
		drop(v, keep)
		return nil
	}); err != nil {
		logx.Error(logx.ModuleConfig, "failed to garbage collect %s in %s: %v", label, store.Name, err)
		return
	}
	logx.Info(logx.ModuleConfig, "garbage collected %s in %s: count=%d", label, store.Name, orphans)
}

// probeOrphans 统计该 store 中「名字不在 keep 里」的条目数（只读）。
func probeOrphans[T any](v *T, keep map[string]struct{}) int {
	switch f := any(v).(type) {
	case *RulesFile:
		return countMissing(f.BySub, keep)
	case *TunnelsFile:
		return countMissing(f.BySub, keep)
	case *MetaFile:
		return countMissing(f.Subscriptions, keep)
	}
	return 0
}

// countMissing 统计 map 中键不在 keep 里的条目数（不修改）。
func countMissing[V any](m map[string]V, keep map[string]struct{}) int {
	n := 0
	for name := range m {
		if _, ok := keep[name]; !ok {
			n++
		}
	}
	return n
}

// dropMissing 删除 map 中键不在 keep 里的条目，返回删除数量。
func dropMissing[V any](m map[string]V, keep map[string]struct{}) int {
	removed := 0
	for name := range m {
		if _, ok := keep[name]; !ok {
			delete(m, name)
			removed++
		}
	}
	return removed
}

// ——— 小工具：拷贝与装箱 ———

func setRuleList(m map[string][]CustomRule, key string, rules []CustomRule) {
	if len(rules) == 0 {
		delete(m, key)
		return
	}
	m[key] = copyRules(rules)
}

func setTunnelList(m map[string][]Tunnel, key string, tunnels []Tunnel) {
	if len(tunnels) == 0 {
		delete(m, key)
		return
	}
	m[key] = CopyTunnels(tunnels)
}

func copyRuleMap(in map[string][]CustomRule) map[string][]CustomRule {
	if in == nil {
		return nil
	}
	out := make(map[string][]CustomRule, len(in))
	for k, v := range in {
		out[k] = copyRules(v)
	}
	return out
}

func copyTunnelMap(in map[string][]Tunnel) map[string][]Tunnel {
	if in == nil {
		return nil
	}
	out := make(map[string][]Tunnel, len(in))
	for k, v := range in {
		out[k] = CopyTunnels(v)
	}
	return out
}

func copyCustomNodes(in []CustomNode) []CustomNode {
	if in == nil {
		return nil
	}
	out := make([]CustomNode, len(in))
	for i, node := range in {
		out[i] = node
		out[i].Config = copyAnyMap(node.Config)
	}
	return out
}

func copyAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// CopyTemplateRules 取模板作用域的规则（供 assemble 与 Getter 复用）。
func CopyTemplateRules(f *RulesFile, scope string) []CustomRule {
	if scope == RuleScopeCustom {
		return copyRules(f.CustomMode)
	}
	return copyRules(f.Merge[scope])
}

// CopyTemplateTunnels 取模板作用域的隧道。
func CopyTemplateTunnels(f *TunnelsFile, scope string) []Tunnel {
	if scope == RuleScopeCustom {
		return CopyTunnels(f.CustomMode)
	}
	return CopyTunnels(f.Merge[scope])
}
