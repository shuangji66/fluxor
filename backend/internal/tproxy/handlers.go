package tproxy

import (
	"encoding/json"
	"fluxor/internal/config"
	"fluxor/internal/httpx"
	"fluxor/internal/logx"
	"net/http"
)

// tproxyPort 读取当前生效的 TProxy 端口。
//
// 唯一真相是 settings.json（config.Current.TproxyPort）：内核 config.yaml 的
// tproxy-port 由它生成，防火墙规则也必须由它构造。此前 /configs PATCH 用请求体里的
// 值重建规则、开关路径用这里的值，两者一旦不同就会出现「规则指向 A 端口、内核听
// B 端口」的断网；现在所有重建入口都收敛到本函数。
func tproxyPort() int {
	config.Mu.RLock()
	defer config.Mu.RUnlock()
	return config.Current.TproxyPort
}

// ReapplyTproxyRules 按当前 settings 里的端口重建 TProxy 规则（开关项变更、内核重载、
// 端口变更后调用）。
//
// 语义：未启用或端口无效时什么也不做；否则先清理再按当前开关组合重新下发。
// 失败时**回滚到底**——清掉可能已部分下发的规则并把开关置为关闭，并把错误交给
// 调用方回报。这是必须的：重建流程已先删掉旧规则，若失败只记一行日志，系统里就会
// 留下「面板显示已启用、实际只有一半规则（甚至完全没有）」的静默错配；而 enable
// 是按家族逐个下发的（v4 成功、v6 失败即属此类）。
func ReapplyTproxyRules() error {
	if !GetTproxyState() {
		return nil
	}
	port := tproxyPort()
	if port <= 0 {
		// 端口为 0 = 内核不再监听该端口，接管必然无法生效。此刻若还把开关留着「已启用」，
		// 面板显示与实际就正好相反（规则若还在，流量会被导向无人监听的端口 = 断网）。
		// 正常路径不会走到这里——启用接管要求端口 > 0，端口置 0 时 /configs PATCH 会直接
		// 拒绝；只有手工改过 settings.json 之类的不一致状态才会落进来，因此这里做收敛
		// 而不是报错：清规则 + 关开关，让三态（内存/磁盘/内核）重新一致。
		logx.Warn(logx.ModuleTproxy, "tproxy port is 0 while the switch is on: disabling takeover to avoid a stale black-hole route")
		DisableTProxyRules()
		SetTproxyEnabled(false)
		return nil
	}

	DisableTProxyRules()
	if err := EnableTProxyRules(port); err != nil {
		DisableTProxyRules()
		SetTproxyEnabled(false)
		return err
	}
	return nil
}

// HandleTproxyState 处理 TProxy 开关状态：GET 查询，POST 切换。
func HandleTproxyState(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		httpx.RespondJSON(w, http.StatusOK, map[string]bool{"enabled": GetTproxyState()})

	case http.MethodPost:
		var req struct{ Enable bool }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "无效的请求格式")
			return
		}

		if !req.Enable {
			DisableTProxyRules()
			SetTproxyEnabled(false)
			httpx.RespondJSON(w, http.StatusOK, map[string]bool{"enabled": false})
			return
		}

		port := tproxyPort()
		if port <= 0 {
			// 端口为 0 时内核不会监听，规则必然无法生效；明确拒绝而不是
			// 记一条日志后仍回报 enabled=true（那会让面板显示与实际不符）
			httpx.WriteJSONError(w, http.StatusBadRequest, "TProxy 端口为 0，请先配置端口")
			return
		}

		DisableTProxyRules() // 先清理，保证重复开启时幂等
		if err := EnableTProxyRules(port); err != nil {
			// 规则没装成功，状态必须回滚为关闭，避免「面板显示已启用、实际未生效」。
			// 回滚 = 改状态 + 清掉可能已部分下发的规则：enable 是按家族逐个下发的，
			// 若 v4 成功、v6 失败（或策略路由探测不过），只改状态会留下
			// 「面板显示已关闭、流量仍被劫持」的静默错配。
			DisableTProxyRules()
			SetTproxyEnabled(false)
			logx.Error(logx.ModuleTproxy, "failed to apply tproxy rules: %v", err)
			httpx.WriteJSONError(w, http.StatusInternalServerError, "启用 TProxy 失败: "+err.Error())
			return
		}
		SetTproxyEnabled(true)
		httpx.RespondJSON(w, http.StatusOK, map[string]bool{"enabled": true})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// HandleTproxyExceptions 处理绕过列表的获取和更新
func HandleTproxyExceptions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		exceptionsMu.RLock()
		dst := tproxyDstExceptionsCache
		src := tproxySrcExceptionsCache
		exceptionsMu.RUnlock()
		httpx.RespondJSON(w, http.StatusOK, map[string]interface{}{
			"dst": dst,
			"src": src,
			// 预填内容随查询一并返回：老配置里已存在该字段，改默认值对他们
			// 不生效，前端「恢复默认」按钮靠这份数据把新预填灌回文本框。清单只在
			// store.go 维护一份，避免前后端各写一份而漂移。
			"defaults": map[string][]string{
				"dst": defaultDstExceptions(),
				"src": defaultSrcExceptions(),
			},
		})
	case http.MethodPost:
		var req struct {
			Dst []string `json:"dst"`
			Src []string `json:"src"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "无效请求格式")
			return
		}
		// 分别保存
		if err := SaveTproxyDstExceptions(req.Dst); err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, "保存目的绕过失败")
			return
		}
		if err := SaveTproxySrcExceptions(req.Src); err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, "保存源绕过失败")
			return
		}
		// 如果 TProxy 启用则重载
		if err := ReapplyTproxyRules(); err != nil {
			logx.Error(logx.ModuleTproxy, "failed to reapply tproxy rules after bypass list update: %v", err)
			httpx.WriteJSONError(w, http.StatusInternalServerError, "重新应用规则失败（TProxy 已自动关闭）: "+err.Error())
			return
		}
		httpx.RespondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// HandleTproxyProxyLocal 处理本机代理开关的获取和设置
func HandleTproxyProxyLocal(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		httpx.RespondJSON(w, http.StatusOK, map[string]bool{"enabled": proxyLocalEnabled()})
	case http.MethodPost:
		var req struct{ Enabled bool }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "无效请求格式")
			return
		}
		if err := SaveTproxyProxyLocal(req.Enabled); err != nil {
			logx.Error(logx.ModuleTproxy, "failed to persist proxy-local switch: %v", err)
			httpx.WriteJSONError(w, http.StatusInternalServerError, "保存失败")
			return
		}
		// 如果 TProxy 当前启用，立即重新应用规则
		if err := ReapplyTproxyRules(); err != nil {
			logx.Error(logx.ModuleTproxy, "failed to reapply tproxy rules: %v", err)
			httpx.WriteJSONError(w, http.StatusInternalServerError, "重新应用规则失败（TProxy 已自动关闭）: "+err.Error())
			return
		}
		httpx.RespondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// HandleTproxyProxyIPv6 处理「接管 IPv6 流量」开关的获取和设置。
//
// 该开关默认关闭，且与防火墙是否启用无关：它只决定 IPv6 家族（nft ip6 表 +
// `ip -6` 策略路由）是否随 TProxy 一起下发。前端在 TProxy 启用期间禁止修改
// （置灰 + 说明原因），因为此时改动会重建正被持有的规则。
func HandleTproxyProxyIPv6(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// global_ipv6 是订阅中心的「IPv6 支持」总开关：它关闭时 enabled（偏好）仍然
		// 如实返回，但规则不会下发——前端据此置灰并说明"当前不生效"。
		httpx.RespondJSON(w, http.StatusOK, map[string]bool{
			"enabled":     ipv6Enabled(),
			"global_ipv6": globalIPv6Enabled(),
		})
	case http.MethodPost:
		var req struct{ Enabled bool }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "无效请求格式")
			return
		}
		if err := SaveTproxyIPv6(req.Enabled); err != nil {
			logx.Error(logx.ModuleTproxy, "failed to persist IPv6 takeover switch: %v", err)
			httpx.WriteJSONError(w, http.StatusInternalServerError, "保存失败")
			return
		}
		// 如果 TProxy 当前启用，立即重新应用规则（开启时新增 ip6 规则，关闭时清掉）
		if err := ReapplyTproxyRules(); err != nil {
			// 规则没装成功就必须让调用方知道：此处不能默默吞掉，
			// 否则面板会显示「已启用」而 IPv6 实际未被接管。
			logx.Error(logx.ModuleTproxy, "failed to reapply tproxy rules after IPv6 takeover change: %v", err)
			httpx.WriteJSONError(w, http.StatusInternalServerError, "重新应用规则失败（TProxy 已自动关闭）: "+err.Error())
			return
		}
		httpx.RespondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}
