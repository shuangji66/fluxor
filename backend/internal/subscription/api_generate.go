package subscription

import (
	"encoding/json"
	"fluxor/internal/config"
	"fluxor/internal/configgen"
	"fluxor/internal/core"
	"fluxor/internal/httpx"
	"fluxor/internal/logx"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// 本文件是「保存并应用」（POST /subscribe/generate）的统一入口。
//
// ============================ 契约 ============================
//
// 按写入者拆分存储之后，这个接口的输入与输出被重新划定：
//
//	请求体只承载**设置类字段**：全局标量（端口 / 密钥 / 模式 / 档位 / 面板 / 外部面板
//	地址）+ 订阅注册表（名称 / 链接 / 拉取间隔 / 健康检查间隔 / 前缀）+ 手工节点 +
//	待物理删除的订阅名。前端的 buildSettingsPayload 就是这么组装的。
//
//	规则与流量隧道**不在请求体里**：它们存在 rules.json / tunnels.json，由各自的专用
//	接口（/subscribe/custom-rules、/subscribe/merge-custom-tunnels …）维护。
//
// 由此得到一条硬性规则：**生成配置时绝不能拿请求体当输入**。
// 请求体里没有规则与隧道，拿它生成出来的 config.yaml 必然不含这两类内容；而界面与
// rules.json/tunnels.json 里它们都还在，于是「保存并应用」会静默地把它们从运行配置里
// 抹掉（隧道被抹掉意味着该监听端口不再转发流量）。这正是上一版删掉
// config.AdoptServerOwnedFields 时留下的回归：它按「持久化不需要合并」判断，忽略了
// 同一个 cfg 同时还是**生成输入**。
//
// 因此固定的顺序是：
//
//	1. 校验并归一化请求体（模式 / 档位 / 端口 / 订阅名 / 手工节点）；
//	2. SaveSettings 落库——订阅改名的资源搬迁与删除后的孤儿回收都在它内部完成；
//	3. 取**落库之后**的快照（config.CurrentSnapshot）：此刻订阅名与规则 / 隧道 /
//	   元数据一定是对齐的，改名后的订阅也能取到自己的规则；
//	4. 用快照生成或复制 config.yaml；
//	5. 重置定时器并热重载内核（重载失败按 warning 如实回报，不假装成功）。
//
// 三条链路（融合 / 切换 / 自定义）都遵守这个顺序，差异只在第 4 步。

// HandleGenerateConfig 处理 POST /subscribe/generate。
func HandleGenerateConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	req, err := decodeSettingsRequest(r)
	if err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 物理清理标记删除的配置文件。放在落库之前：即使后面生成失败，用户明确要求删除的
	// 文件也已经删掉了（残留文件会在下次「确保订阅文件」时被重新视为有效订阅文件）。
	physicallyDeleteSubscriptionFiles(req.DeletePhysical)
	req.DeletePhysical = nil // 清空临时字段避免持久化

	switch req.Mode {
	case config.ModeCustom:
		generateCustomConfig(w, req)
	case config.ModeSwitch:
		generateSwitchConfig(w, req)
	default:
		generateMergeConfig(w, req)
	}
}

// decodeSettingsRequest 解析并校验「设置类」请求体。
//
// 校验全部前移到落库之前：非法取值一旦写进 settings.json，生成链路与定时器会各自走进
// 「既不是 A 也不是 B」的分支，用户在界面上看到的仍是旧值，排查时无从下手。
func decodeSettingsRequest(r *http.Request) (config.SubscribeConfig, error) {
	// 与 /subscribe/config 一致：先预置默认值，避免漏传的 bool 被当作 false。
	req := config.DefaultSubscribeConfig()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, fmt.Errorf("无效的请求格式: %w", err)
	}
	config.NormalizeIPv6Flags(&req)

	mode, err := config.NormalizeMode(req.Mode)
	if err != nil {
		return req, err
	}
	req.Mode = mode

	ruleGroup, err := config.NormalizeRuleGroup(req.RuleGroup, mode)
	if err != nil {
		return req, err
	}
	req.RuleGroup = ruleGroup

	if err := config.ValidateLocalPorts(req); err != nil {
		return req, err
	}
	if req.MetaBackendURL != "" && !httpx.BackendURLRegex.MatchString(req.MetaBackendURL) {
		return req, fmt.Errorf("外部面板后端地址格式不正确")
	}
	// 订阅名会用作节点文件名与 provider 键，必须在此拦截非法字符
	if err := config.ValidateSubscriptionNames(req.Subscriptions); err != nil {
		return req, err
	}
	// 手工节点落库前校验并归一化（节点名、字段类型与必填、与模板组名冲突），
	// 否则内核会拒绝加载整份配置
	nodes, err := normalizeCustomNodes(req.CustomNodes)
	if err != nil {
		return req, err
	}
	req.CustomNodes = nodes
	return req, nil
}

// physicallyDeleteSubscriptionFiles 删除用户明确要求删除的订阅节点文件。
//
// 与写入侧共用同一套文件名规则（SanitizeSubscriptionFileName），避免删错文件；
// 删除失败只记日志——文件残留比「保存失败」轻得多，而且下次下载会覆盖它。
func physicallyDeleteSubscriptionFiles(names []string) {
	for _, name := range names {
		fileName := config.SanitizeSubscriptionFileName(name)
		if fileName == ".yaml" {
			continue
		}
		targetFile := filepath.Join(config.CoreWorkDir, "proxies", fileName)
		if _, err := os.Stat(targetFile); err != nil {
			continue
		}
		if err := os.Remove(targetFile); err != nil {
			logx.Error(logx.ModuleSub, "physical delete of subscription file %s failed: %v", targetFile, err)
			continue
		}
		logx.Info(logx.ModuleSub, "subscription file physically deleted: %s", targetFile)
	}
}

// generateSwitchConfig 切换模式：config.yaml = 激活订阅文件副本 + 该订阅的自定义规则与隧道。
func generateSwitchConfig(w http.ResponseWriter, req config.SubscribeConfig) {
	// 没有订阅：退回基础配置（界面上的「清除订阅」）。
	// 注意 base 配置也必须从落库后的快照生成：active_subscription 要在这一步被清掉。
	if len(req.Subscriptions) == 0 {
		req.ActiveSubscription = ""
		if err := config.SaveSettings(req); err != nil {
			logx.Error(logx.ModuleSub, "saving settings failed: mode=switch subscription=none err=%v", err)
			httpx.WriteJSONError(w, http.StatusInternalServerError, "保存配置失败: "+err.Error())
			return
		}
		snap := config.CurrentSnapshot()
		if err := configgen.GenerateBaseConfig(snap); err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, "生成基础配置失败: "+err.Error())
			return
		}
		resetTimers()
		finishGenerate(w, "已清除订阅，切换到基础配置", "")
		return
	}

	// 选中名必须是「这次要落库的注册表」里的成员：改名 / 删除后客户端仍可能提交旧名，
	// 只判空字符串会让旧名一路走到下面，按旧名复制旧订阅文件，出现
	// 「界面显示新名字、实际生效旧配置」的静默错配。
	if !activeSubscriptionExists(req) {
		httpx.WriteJSONError(w, http.StatusBadRequest, "选中的订阅不存在: "+req.ActiveSubscription)
		return
	}

	if err := config.SaveSettings(req); err != nil {
		logx.Error(logx.ModuleSub, "saving settings failed: mode=switch err=%v", err)
		httpx.WriteJSONError(w, http.StatusInternalServerError, "保存配置失败: "+err.Error())
		return
	}

	// 落库后的权威快照：改名搬迁、孤儿回收都已生效，规则与隧道也随之对齐
	snap := config.CurrentSnapshot()

	// 只补缺失的订阅文件（原子替换，失败不动本地副本）。
	// 单个订阅失败不阻断本次保存：只有「激活订阅的文件确实不可用」才没有配置可生成。
	downloadErr := ensureSubscriptionFiles(&snap)

	srcFile := subscriptionFilePath(snap.ActiveSubscription)
	if _, err := os.Stat(srcFile); err != nil {
		msg := "选中的订阅文件不可用: " + err.Error()
		if downloadErr != nil {
			msg += "；下载失败: " + downloadErr.Error()
		}
		httpx.WriteJSONError(w, http.StatusInternalServerError, msg)
		return
	}

	// 自定义规则与流量隧道一律取自**落库后**的快照，不看请求体（请求体里没有它们）。
	if _, err := writeRuntimeConfig(snap.ActiveSubscription,
		copyCustomRules(snap, snap.ActiveSubscription),
		copyCustomTunnels(snap, snap.ActiveSubscription)); err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, "生成运行配置失败: "+err.Error())
		return
	}

	resetTimers()

	warning := ""
	if downloadErr != nil {
		warning = "部分订阅文件未能更新（已沿用本地已有副本）: " + downloadErr.Error()
	}
	finishGenerate(w, "已切换到订阅 "+snap.ActiveSubscription+" 的配置", warning)
}

// generateMergeConfig 融合模式：模板 + 全部订阅 provider + 档位规则集 + 该档位的自定义规则与隧道。
func generateMergeConfig(w http.ResponseWriter, req config.SubscribeConfig) {
	if err := config.SaveSettings(req); err != nil {
		logx.Error(logx.ModuleSub, "saving settings failed: mode=merge err=%v", err)
		httpx.WriteJSONError(w, http.StatusInternalServerError, "保存配置失败: "+err.Error())
		return
	}
	resetTimers()

	snap := config.CurrentSnapshot()

	// GenerateConfig 内部按 snap.RuleGroup 取该档位的代理组、规则集以及自定义规则/隧道，
	// 因此必须传落库后的快照——传请求体会得到一份不含自定义规则与隧道的配置。
	if err := configgen.GenerateConfig(snap); err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, "生成配置文件失败: "+err.Error())
		return
	}

	if snap.MetaBackendURL != "" {
		if err := modifyMetaConfig(snap.MetaBackendURL); err != nil {
			logx.Warn(logx.ModuleSub, "updating MetaCubeXD backend URL failed: %v", err)
		}
	}

	// 内核未运行 / 重载失败时不再抓元数据：provider 尚未重建，抓到的必然是空值，
	// 而空值会被 SaveSubscriptionMeta 判为「无可用信息」而不落库，白跑一趟。
	if err := core.ReloadCore(); err != nil {
		httpx.RespondJSON(w, http.StatusOK, map[string]string{
			"status":  "warning",
			"message": "配置文件已生成，但重载内核失败: " + err.Error(),
		})
		return
	}

	// 元数据各自落库（每个订阅只写自己的条目），入参用本地副本，不回写 Current。
	updateAllSubscriptionsMetadata(&snap)

	httpx.RespondJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "配置文件已生成并成功重载内核",
	})
}

// resetTimers 按当前模式重启全部定时器。
//
// 先 Stop 再 Start：模式或订阅列表刚刚变过，旧定时器可能仍在追逐已删除的订阅。
// 两者各自幂等且有专用互斥锁串行化，这里不持任何业务锁（见 AGENTS 3.6）。
func resetTimers() {
	StopAllTimers()
	StartAllTimers()
}

// finishGenerate 生成成功后的统一收尾：重载内核并按结果组装响应。
//
// 两种情况都是 HTTP 200，靠 body 里的 status 区分（前端据此提示 warning 而不是失败）：
//   - "ok"：配置已生成且内核已重载；
//   - "warning"：配置已生成，但热重载失败（内核未运行是最常见的原因）。
//
// 后者的语义是「已保存、未生效」，绝不是「操作失败」——settings.json 与 config.yaml
// 都已经写好，内核下次启动或用户手动重载即可生效。
func finishGenerate(w http.ResponseWriter, successMsg, warning string) {
	if err := core.ReloadCore(); err != nil {
		httpx.RespondJSON(w, http.StatusOK, map[string]string{
			"status":  "warning",
			"message": joinMessage(warning, "配置已更新，但重载内核失败: "+err.Error()),
		})
		return
	}
	httpx.RespondJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": joinMessage(warning, successMsg),
	})
}
