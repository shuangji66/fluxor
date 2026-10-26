package subscription

import (
	"encoding/json"
	"fluxor/internal/config"
	"fluxor/internal/httpx"
	"fluxor/internal/logx"
	"fluxor/internal/nodespec"
	"net/http"
)

// HandleSubscribeConfigAPI 处理 GET /subscribe/config 和 POST /subscribe/config
func HandleSubscribeConfigAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// 锁内只做数据取用，**不**在网络写出期间持锁。
		//
		// 响应体包含全部订阅注册表与手工节点，MaterializeAll 还要为每个手工节点补齐
		// 全量协议字段。若把 json.Encode 也留在锁内，一个不读响应（或链路极慢）的客户端
		// 就能长期占住读锁；而 Go 的 RWMutex 在有写者排队后会阻塞后续读者，于是不只是
		// 「保存并应用」要排队，core.CoreRequest / wsproxy / quality / tproxy 等十余处
		// 读 Current 的路径会一起卡住。
		//
		// 取一份加锁快照、在锁外补齐并编码：复制结构体再替换切片，不就地改写 config.Current。
		view := config.CurrentSnapshot()
		view.CustomNodes = nodespec.MaterializeAll(view.CustomNodes)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(view); err != nil {
			logx.Error(logx.ModuleConfig, "encoding subscription config failed: %v", err)
		}

	case http.MethodPost:
		// 先预置默认值再解码：IPv6 两个开关是 bool，无法区分「没传」与「显式 false」，
		// 不预置的话漏传就会被当成「关闭 IPv6」。
		newConfig := config.DefaultSubscribeConfig()
		if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "无效的请求格式: "+err.Error())
			return
		}
		// 模式与档位先归一化再落库：落一个不存在的取值进 settings.json 之后，
		// 生成链路、定时器启停会各自走进「既不是 A 也不是 B」的分支，行为无从预期。
		config.NormalizeIPv6Flags(&newConfig)
		mode, err := config.NormalizeMode(newConfig.Mode)
		if err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		newConfig.Mode = mode
		ruleGroup, err := config.NormalizeRuleGroup(newConfig.RuleGroup, mode)
		if err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		newConfig.RuleGroup = ruleGroup
		// 端口在写盘前校验：前端那套 1025-65535 + 不重复的规则只是体验，后端才是边界，
		// 绕过前端写入的非法端口会让内核起不来（表现为「配置加载失败」，很难归因到端口）。
		if err := config.ValidateLocalPorts(newConfig); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if newConfig.MetaBackendURL != "" && !httpx.BackendURLRegex.MatchString(newConfig.MetaBackendURL) {
			httpx.WriteJSONError(w, http.StatusBadRequest, "外部面板后端地址格式不正确")
			return
		}
		// 订阅名会用作节点文件名与 provider 键，必须在此拦截非法字符
		if err := config.ValidateSubscriptionNames(newConfig.Subscriptions); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		// 请求体里的自定义规则与流量隧道一律忽略：它们存在 rules.json / tunnels.json 里，
		// 由各自的专用接口维护；SaveSettings 只取设置与订阅注册表，因此请求体里的过期快照
		// 无法覆盖服务端（这正是旧 AdoptServerOwnedFields 补丁要解决的问题）。

		// 自定义节点列表与订阅列表一样由本接口整体覆盖：落库前先校验并归一化
		// （节点名、字段类型与必填、与模板组名冲突），否则内核会拒绝加载整份配置。
		nodes, err := normalizeCustomNodes(newConfig.CustomNodes)
		if err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		newConfig.CustomNodes = nodes

		// 落库与重组装 Current 由 SaveSettings 一并完成（内部取 config.Mu，故此处不得持锁）
		if err := config.SaveSettings(newConfig); err != nil {
			logx.Error(logx.ModuleConfig, "saving settings failed: %v", err)
			httpx.WriteJSONError(w, http.StatusInternalServerError, "保存配置失败: "+err.Error())
			return
		}

		// 重置定时器（先停止再启动）
		StopAllTimers()
		StartAllTimers()

		httpx.RespondJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"message": "配置已保存",
		})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}
