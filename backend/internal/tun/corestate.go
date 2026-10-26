package tun

import (
	"encoding/json"
	"fmt"
	"net/http"

	"fluxor/internal/core"
	"fluxor/internal/httpx"
)

// CoreTunEnabled 查询内核当前是否已启用 TUN（运行时真相，不读任何本地缓存）。
//
// TUN 的开关不落盘、不回放，而且本面板之外（MetaCubeXD / curl / 上一次面板会话）
// 也能打开它，因此「TUN 现在是否在接管」只有内核知道。TProxy 启用前的互斥校验
// 用的就是这个实时值：任何本地标志都会在面板重启、内核重载、第三方操作之后失真。
//
// 返回错误只代表「查不出来」，不代表 TUN 没开；调用方需自行决定降级策略。
func CoreTunEnabled() (bool, error) {
	resp, err := core.CoreRequest(http.MethodGet, "/configs", nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("内核返回状态 %d", resp.StatusCode)
	}
	body, err := httpx.ReadAllLimited(resp.Body, httpx.MaxUpstreamBody)
	if err != nil {
		return false, fmt.Errorf("读取内核配置失败: %w", err)
	}

	var payload struct {
		Tun struct {
			Enable bool `json:"enable"`
		} `json:"tun"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false, fmt.Errorf("解析内核配置失败: %w", err)
	}
	return payload.Tun.Enable, nil
}
