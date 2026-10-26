package tproxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fluxor/internal/config"
)

// useTempTproxyFile 把 tproxy.json 指到临时目录，并清理包级缓存。
//
// 载入后缓存里是「生效列表」（nil 会被还原成预填模板），因此用例结束时必须复位，
// 否则会污染同包其它用例（rules_test.go 依赖默认家族定义与解析结果）。
func useTempTproxyFile(t *testing.T) string {
	t.Helper()
	prevDataDir, prevFile := config.FluxorDataDir, config.FluxorTproxyFile
	dir := t.TempDir()
	config.SetDataDir(dir)
	t.Cleanup(func() {
		config.FluxorDataDir, config.FluxorTproxyFile = prevDataDir, prevFile
		exceptionsMu.Lock()
		tproxyDstExceptionsCache = nil
		tproxySrcExceptionsCache = nil
		exceptionsMu.Unlock()
	})
	return dir
}

// TestDefaultBypassTemplateNotPersisted 预填模板不落盘，只有用户改过才写文件。
//
// 旧实现把 20+ 行中文注释模板写进用户配置，空配置的文件体积九成都来自这里；
// 现在 nil 表示「用代码里的默认值」，序列化成 null——**键仍然在**（无 omitempty），
// 但值里不含任何模板正文。这里断言的是「模板正文不落盘」这个意图本身，
// 而不是某个具体的键名形态，因此去掉 omitempty 后依然成立。
func TestDefaultBypassTemplateNotPersisted(t *testing.T) {
	dir := useTempTproxyFile(t)

	LoadTproxyState()

	// 生效列表（内存缓存）必须是预填模板
	if got := dstExceptions(); len(got) != len(defaultDstExceptions()) {
		t.Fatalf("未自定义时应使用预填模板，实际 %d 条", len(got))
	}
	// 此时不该生成任何文件（只有开关状态默认值，也不需要落盘）
	if _, err := os.Stat(filepath.Join(dir, "tproxy.json")); !os.IsNotExist(err) {
		t.Fatalf("默认值不应落盘: %v", err)
	}
	// 三个开关默认全关：缺省值必须是不接管，由用户显式开启。
	if proxyLocalEnabled() {
		t.Fatal("本机流量接管默认应为关闭")
	}

	// 用户「恢复默认」= 保存一份与模板一致的列表 → 收敛为 nil，模板正文不落盘
	if err := SaveTproxyDstExceptions(defaultDstExceptions()); err != nil {
		t.Fatalf("SaveTproxyDstExceptions: %v", err)
	}
	raw := readTproxyRaw(t, dir)
	if strings.Contains(raw, "阿里公共 DNS") || strings.Contains(raw, "绕过") {
		t.Fatalf("预填模板正文不应落盘: %s", raw)
	}
	var stored config.TproxyFile
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if stored.DstExceptions != nil {
		t.Fatalf("与预填模板一致的列表应收敛为 null，实际 %v", stored.DstExceptions)
	}

	// 自定义列表则必须落盘并可重新载入
	custom := []string{"1.1.1.1", "8.8.8.8"}
	if err := SaveTproxyDstExceptions(custom); err != nil {
		t.Fatalf("SaveTproxyDstExceptions: %v", err)
	}
	LoadTproxyState() // 重新载入，验证自定义列表确实来自文件
	got := dstExceptions()
	if len(got) != len(custom) || got[0] != custom[0] {
		t.Fatalf("自定义列表未能往返: %v", got)
	}
}

// TestExplicitEmptyBypassListPersists 用户把绕过列表清空时，重启后必须仍然是空的。
//
// 这是去掉 omitempty 的原因：空列表用 `[]` 落盘（「显式清空」），nil 用 `null` 落盘
// （「未自定义，用预填模板」）。带 omitempty 时两者都写成「键缺失」，重启后预填模板
// 会静默回来——用户明确表达的「一条绕过都不要」被无声撤销。
func TestExplicitEmptyBypassListPersists(t *testing.T) {
	dir := useTempTproxyFile(t)

	LoadTproxyState()
	if err := SaveTproxyDstExceptions([]string{}); err != nil {
		t.Fatalf("SaveTproxyDstExceptions: %v", err)
	}
	if err := SaveTproxySrcExceptions([]string{}); err != nil {
		t.Fatalf("SaveTproxySrcExceptions: %v", err)
	}

	raw := readTproxyRaw(t, dir)
	var stored config.TproxyFile
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if stored.DstExceptions == nil || len(stored.DstExceptions) != 0 {
		t.Fatalf("显式清空应落盘为空数组，实际 %v（raw=%s）", stored.DstExceptions, raw)
	}

	// 重新载入：生效列表必须是空的，不能变回预填模板
	exceptionsMu.Lock()
	tproxyDstExceptionsCache = nil
	tproxySrcExceptionsCache = nil
	exceptionsMu.Unlock()
	LoadTproxyState()

	if got := dstExceptions(); len(got) != 0 {
		t.Fatalf("重启后目的绕过列表应仍为空，实际 %d 条", len(got))
	}
	if got := srcExceptions(); len(got) != 0 {
		t.Fatalf("重启后源绕过列表应仍为空，实际 %d 条", len(got))
	}
}

// readTproxyRaw 读取 tproxy.json 的原始内容。
func readTproxyRaw(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "tproxy.json"))
	if err != nil {
		t.Fatalf("读取 tproxy.json 失败: %v", err)
	}
	return string(data)
}

// TestLegacyDefaultTemplateIsDropped 迁移过来的旧默认模板在载入时被移除（自动瘦身）。
func TestLegacyDefaultTemplateIsDropped(t *testing.T) {
	dir := useTempTproxyFile(t)

	legacy := config.TproxyFile{
		ProxyLocal:    true,
		DstExceptions: defaultDstExceptions(),
		SrcExceptions: defaultSrcExceptions(),
	}
	writeTproxyFile(t, dir, legacy)

	LoadTproxyState()

	raw := readTproxyRaw(t, dir)
	if strings.Contains(raw, "阿里公共 DNS") || strings.Contains(raw, "Docker 默认 bridge") {
		t.Fatalf("旧文件里的默认模板正文应被移除: %s", raw)
	}
	var stored config.TproxyFile
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if stored.DstExceptions != nil || stored.SrcExceptions != nil {
		t.Fatalf("模板一致的列表应收敛为 null，实际 dst=%v src=%v", stored.DstExceptions, stored.SrcExceptions)
	}
	if len(dstExceptions()) != len(defaultDstExceptions()) {
		t.Fatal("移除后生效列表应仍是默认模板")
	}
}

// TestSwitchesRoundTrip 三个开关能落盘并重新载入。
func TestSwitchesRoundTrip(t *testing.T) {
	dir := useTempTproxyFile(t)

	LoadTproxyState()
	if err := SaveTproxyProxyLocal(false); err != nil {
		t.Fatalf("SaveTproxyProxyLocal: %v", err)
	}
	if err := SaveTproxyIPv6(true); err != nil {
		t.Fatalf("SaveTproxyIPv6: %v", err)
	}
	SetTproxyEnabled(true)

	// 清掉缓存后重新载入，验证确实来自文件
	exceptionsMu.Lock()
	tproxyProxyLocal, tproxyIPv6 = true, false
	exceptionsMu.Unlock()

	LoadTproxyState()
	if proxyLocalEnabled() {
		t.Fatal("proxy_local 未持久化")
	}
	if !ipv6Enabled() {
		t.Fatal("ipv6 未持久化")
	}
	if !LoadTproxyEnabled() {
		t.Fatal("enabled 未持久化")
	}
	_ = dir
}

func writeTproxyFile(t *testing.T, dir string, v config.TproxyFile) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tproxy.json"), data, 0644); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
}
