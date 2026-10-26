package tproxy

import (
	"errors"
	"strings"
	"testing"
)

// TestEnableGuardBlocksRulesApply 钉住 TUN 互斥的接入点。
//
// guard 返回错误时 EnableTProxyRules 必须在写任何规则之前返回：这里注入一个必失败的
// 回调，因此正常路径不会触碰宿主网络（不需要 root，也不会执行 nft/ip 命令）。校验放在
// EnableTProxyRules 而不是 handler，是为了同时覆盖「开关」「端口变更后重建」
// 「重载后重建」三条入口。
//
// PATH 指向空目录是安全网：万一以后有人把 guard 调用挪走，规则装配会立刻失败于
// 「找不到 nft/ip」，而不是在跑测试的 NAS 上真的改掉防火墙。
func TestEnableGuardBlocksRulesApply(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Cleanup(func() { SetEnableGuard(nil) })
	SetEnableGuard(func() error {
		return errors.New("TUN 正在接管流量，请先关闭 TUN 再启用 TPROXY（两者互斥）")
	})

	err := EnableTProxyRules(7893)
	if err == nil || !strings.Contains(err.Error(), "TUN 正在接管流量") {
		t.Fatalf("EnableTProxyRules err = %v，期望原样返回 guard 的错误", err)
	}
}

// TestGuardRulesApplyWithoutGuard 保证未注入校验时默认放行（库的零值语义）。
func TestGuardRulesApplyWithoutGuard(t *testing.T) {
	SetEnableGuard(nil)
	if err := guardRulesApply(); err != nil {
		t.Fatalf("guardRulesApply() = %v，期望 nil", err)
	}
}

// TestGuardRulesApplyPropagates 保证注入的回调被真正调用且错误原样传出。
func TestGuardRulesApplyPropagates(t *testing.T) {
	t.Cleanup(func() { SetEnableGuard(nil) })
	want := errors.New("boom")
	SetEnableGuard(func() error { return want })

	if err := guardRulesApply(); !errors.Is(err, want) {
		t.Fatalf("guardRulesApply() = %v，期望 %v", err, want)
	}
}
