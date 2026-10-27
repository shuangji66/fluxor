<script setup lang="ts">
import { ref, onMounted, onActivated, onDeactivated, computed, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiFetch } from '../utils/api'
import { formatGB, formatDateTime } from '../utils/format'
import { MailOutline, EyeOutline, EyeOffOutline, SyncOutline, CreateOutline, TrashOutline, CloseOutline, HelpCircleOutline, OptionsOutline, AddOutline, SaveOutline } from '@vicons/ionicons5'
import { useGlobalStore } from '../store/global'
import { storeToRefs } from 'pinia'
import {
  useSubscriptionStore,
  buildSettingsPayload,
  type SubscriptionItem,
  type SubscriptionInfo,
  type CustomNode,
} from '../store/subscription'
import CustomRulesDialog from '../components/CustomRulesDialog.vue'
import CustomNodeDialog from '../components/CustomNodeDialog.vue'
import CustomNodeCard from '../components/CustomNodeCard.vue'
import { useRulesStore } from '../store/rules'
import { useProxyStore } from '../store/proxies'
import { useConfigStore } from '../store/config'
import { validateSubscriptionName, filterSubscriptionNameInput, MAX_SUBSCRIPTION_NAME_LENGTH } from '../utils/subscription-name'
import { useViewActive } from '../composables/useViewActive'
import PageHeader from '../components/PageHeader.vue'
import AppModal from '../components/AppModal.vue'

const globalStore = useGlobalStore()
const configStore = useConfigStore()
const { coreStatus } = storeToRefs(configStore)
const proxyStore = useProxyStore()
const { providerInfos } = storeToRefs(proxyStore)

const { t } = useI18n()

const showSecret = ref(false)
const showModal = ref(false)
const showUrls = ref<Record<number, boolean>>({})
const modalTitle = ref('')
const isUpdating = ref<Record<number, boolean>>({})
const isApplying = ref(false)
const pendingPhysicalDeletes = ref<string[]>([])
const showBackendUrl = ref(false)
const showHelpModal = ref(false)
// 视图激活态：KeepAlive 停用时必须收起 Teleport 浮层，否则会跨页残留
const isActive = useViewActive()

// ------- 自定义模式：节点列表与添加/编辑节点弹窗 -------
// 节点列表由本页面整体维护（与订阅列表同构）：改动先落在本地列表，
// 点「保存并应用」才随配置一起落库并重新生成 config.yaml。
const showNodeModal = ref(false)
const editingNodeIndex = ref(-1)
const editingNode = ref<CustomNode | null>(null)

// 弹窗编辑项
const editingIndex = ref(-1)
const editForm = ref<SubscriptionItem>({
  name: '',
  url: '',
  update_interval: 86400,
  health_interval: 600,
  prefix: '',
  custom_rules: []
})

// ------- 自定义规则弹窗（多作用域，两种模式共用 CustomRulesDialog）-------
// 弹窗自身状态（各作用域的 payload/表单/在途标志）全部由子组件持有，
// 父组件只负责：决定打开哪个 endpoint + 哪几个作用域，以及关闭时按「作用域是否生效」
// 决定要不要登记「规则列表已过期」。
const showRulesModal = ref(false)
const rulesDialogRef = ref<InstanceType<typeof CustomRulesDialog> | null>(null)
const rulesEndpoint = ref('')
// 流量隧道的接口前缀：与规则一一对应（切换=订阅、融合=档位、自定义=独立一份）。
// 两者服务的是同一批作用域，只改一个会让隧道表单打到错的入口上。
const rulesTunnelEndpoint = ref('')
const rulesScopes = ref<{ key: string, label: string, effective: boolean }[]>([])
const rulesTitle = ref('')
// 自定义规则弹窗的作用域提示：融合模式的规则挂在规则集档位上，切换模式在各订阅卡片上，文案随模式切换
const rulesHint = ref('')

// modeApplied 判断「界面上的模式是否已经生效（已保存应用）」。
//
// 三种模式的规则/隧道作用域是分开的，后端按 settings.json 里已保存的 mode 校验入口；
// 界面上的模式选择器却可以改了不保存，于是「界面已切到自定义模式、按钮仍按自定义入口
// 发请求、后端按融合模式拒绝」——用户只会看到一句「自定义规则仅在融合模式下可用」，
// 完全不知道问题出在还没保存并应用。这里在发请求之前就把下一步说清楚。
const modeApplied = () => currentConfig.value.mode === savedMode.value

const ensureModeApplied = () => {
  if (modeApplied()) return true
  globalStore.showToast(t('subscription.mode_unsaved_hint'), 'error')
  return false
}

// 打开订阅级（切换模式）自定义规则：作用域即该订阅名，单作用域 → 不渲染页签
const openSubRulesDialog = (name: string) => {
  if (!ensureModeApplied()) return
  rulesEndpoint.value = '/subscribe/custom-rules'
  rulesTunnelEndpoint.value = '/subscribe/custom-tunnels'
  rulesScopes.value = [{ key: name, label: name, effective: name === currentConfig.value.active_subscription }]
  rulesTitle.value = t('subscription.custom_rules_title', { name })
  rulesHint.value = ''
  showRulesModal.value = true
}

// 打开自定义规则弹窗（标题行按钮）：
//   - 融合模式：base / full 两档各自独立，用页签切换；
//   - 自定义模式：走独立的作用域与接口（/subscribe/custom-mode-rules/custom），规则与
//     融合模式各存各的、互不影响；作用域段固定为 custom，因此只开一个（不渲染页签）。
const openRulesDialog = () => {
  if (!ensureModeApplied()) return
  if (currentConfig.value.mode === 'custom') {
    rulesEndpoint.value = '/subscribe/custom-mode-rules'
    rulesTunnelEndpoint.value = '/subscribe/custom-mode-tunnels'
    rulesScopes.value = [{ key: 'custom', label: t('subscription.custom_rules'), effective: true }]
    rulesTitle.value = t('subscription.custom_rules_custom_title')
    rulesHint.value = t('subscription.custom_rules_custom_hint')
    showRulesModal.value = true
    return
  }
  openMergeRulesDialog()
}

// 打开规则集档位级（融合模式）自定义规则：base/full 两档各自独立，用页签切换
const openMergeRulesDialog = () => {
  rulesEndpoint.value = '/subscribe/merge-custom-rules'
  rulesTunnelEndpoint.value = '/subscribe/merge-custom-tunnels'
  rulesScopes.value = [
    { key: 'base', label: t('subscription.rule_group_base'), effective: currentConfig.value.rule_group === 'base' },
    { key: 'full', label: t('subscription.rule_group_full'), effective: currentConfig.value.rule_group === 'full' },
  ]
  rulesTitle.value = t('subscription.custom_rules_merge_title')
  rulesHint.value = t('subscription.custom_rules_merge_hint')
  showRulesModal.value = true
}

// flushRulesStaleMark 登记「规则页的列表已过期」，只在确实改动过当前生效作用域时登记。
//
// 为什么限定生效作用域：切换模式下只有激活订阅的规则会进入 config.yaml，融合模式下
// 只有当前 rule_group 那一档会重新生成配置；其余作用域的规则不进运行配置，内核规则
// 集合压根没变，刷新规则页纯属多余请求。
// 为什么由子组件在关闭时才上报：用户可能连续改多条，登记时机收敛到「关闭弹窗」，
// 标记由规则页切入时消费——用户一直不切过去就不会产生请求。
const flushRulesStaleMark = (mutatedKeys: string[] = []) => {
  if (mutatedKeys.length === 0) return
  // 自定义模式有独立的规则作用域（custom），其改动同样会改变运行中的规则集合
  const effectiveKey = currentConfig.value.mode === 'switch'
    ? currentConfig.value.active_subscription
    : (currentConfig.value.mode === 'custom' ? 'custom' : currentConfig.value.rule_group)
  if (!effectiveKey || !mutatedKeys.includes(effectiveKey)) return
  rulesStore.markNeedsRefresh()
}

const closeRulesDialog = (mutatedKeys: string[] = []) => {
  flushRulesStaleMark(mutatedKeys)
  showRulesModal.value = false
}

// 新增选中状态（绑定到 currentConfig.active_subscription）
const activeSub = computed({
    get: () => currentConfig.value.active_subscription || '',
    set: (val: string) => { currentConfig.value.active_subscription = val; }
})

// 名称长度提示。字符集问题由输入框实时过滤拦截，此处只需提示长度。
const nameHint = computed(() =>
  validateSubscriptionName(editForm.value.name) === 'tooLong'
    ? t('subscription.name_too_long')
    : ''
)

// 只有 MetaCubeXD 用得到「后端地址」，选 Zashboard 时该字段整块不渲染。
// 显隐只作用于渲染：meta_backend_url 始终留在 currentConfig 中，
// 「保存并应用」照常提交原内容，来回切换面板不会把已填地址弄丢。
const isMetaCubeXd = computed(() => currentConfig.value.ui_panel === 'metacubexd')
// 六列栅格里的跨度规则：
//   · 融合模式三项同排 → 桌面各 2 列；关闭 IPv6 时只剩两项，各 3 列。
//   · 切换模式只有两项 IPv6 → 桌面各 3 列；关闭时仅剩一项 3 列，下一项（面板密钥）
//     自动补到同一行，不会留下整行空白。
//   · 窄窗口成对排（3 列）；IPv6 关闭时该项占满整行，规则集与外置面板继续成对。
const ipv6SpanClass = computed(() => {
  if (!currentConfig.value.ipv6) return 'col-span-6 md:col-span-3'
  return currentConfig.value.mode === 'merge' ? 'col-span-3 md:col-span-2' : 'col-span-3 md:col-span-3'
})

const dnsSpanClass = computed(() =>
  currentConfig.value.mode === 'merge' ? 'col-span-3 md:col-span-2' : 'col-span-3 md:col-span-3'
)

const ruleSpanClass = computed(() =>
  currentConfig.value.ipv6 ? 'col-span-3 md:col-span-2' : 'col-span-3 md:col-span-3'
)

// 外置面板在融合模式下与规则集成对（各 3 列）；切换模式没有规则集，整行显示。
const uiSpanClass = computed(() =>
  currentConfig.value.mode === 'merge' ? 'col-span-3 md:col-span-3' : 'col-span-6 md:col-span-3'
)

// 窄窗口的先后：融合模式要与规则集同排，因此排在面板密钥之前；
// 切换模式没有规则集，回到「面板密钥 → 外置面板」的原有次序（桌面端两者都保持此序）。
const uiOrderClass = computed(() =>
  currentConfig.value.mode === 'merge' ? 'order-4 md:order-6' : 'order-6 md:order-6'
)


// 点击卡片选中
const selectSubscription = (name: string) => {
    if (currentConfig.value.mode === 'switch') {
        activeSub.value = activeSub.value === name ? '' : name; // 点击已选中的可取消选中（单选切换）
    }
}

const rulesStore = useRulesStore()
const subscriptionStore = useSubscriptionStore()
const { currentConfig, savedSubNames, savedMode, nodeProtocols } = storeToRefs(subscriptionStore)

// IPv6 两项是**生成期字段**（写入 config.yaml 的 ipv6 / dns.ipv6），
// 改动随「保存并应用」落盘并重载内核生效；不是运行时 PATCH。
//
// 两项联动：关闭 IPv6 支持时 DNS 必然不返回 IPv6（后端也强制归一化），DNS 项同时
// 被隐藏；重新开启时 DNS 返回 IPv6 **默认恢复为开启**——关掉总开关的那一刻 DNS 项
// 已不可见，若保留 false，用户会得到「IPv6 开着、却拿不到 AAAA」这个没有任何入口
// 可解释的组合。开启后仍可单独把 DNS 项关掉（例如节点只有 IPv4 出口）。
// 用 watch 而不是 @change，是为了不依赖 v-model 与事件回调的触发顺序。
// 必须放在 currentConfig 初始化之后：watch 的 getter 在 setup 阶段立即执行。
watch(
  () => currentConfig.value.ipv6,
  (enabled) => {
    currentConfig.value.dns_ipv6 = enabled
  },
)

// 是否处于自定义模式：自定义模式用「节点列表」替代「订阅列表」
const isCustomMode = computed(() => currentConfig.value.mode === 'custom')

// 模式分段控件（顶栏）：移动端单独一行，桌面端居中，两处共用同一份选项
const modeOptions = [
  { value: 'merge', label: 'subscription.mode_merge' },
  { value: 'switch', label: 'subscription.mode_switch' },
  { value: 'custom', label: 'subscription.mode_custom' },
] as const
const customNodes = computed<CustomNode[]>(() => currentConfig.value.custom_nodes || [])

// 节点卡片数据：协议展示名在此解析，卡片组件保持纯展示
const nodeCards = computed(() =>
  customNodes.value.map(node => ({
    node,
    protocolName: nodeProtocols.value.find(p => p.type === node.type)?.name || node.type,
  }))
)

// 打开「添加 / 编辑节点」弹窗（index < 0 表示新增）。
// 协议字段表懒加载：首次打开时拉取，失败则不开弹窗（空协议表无从渲染表单）。
const openNodeModal = async (index: number = -1) => {
  try {
    await subscriptionStore.loadNodeProtocols()
  } catch (e) {
    globalStore.showToast(`${t('subscription.operation_failed')}: ${(e as Error).message}`, 'error')
    return
  }
  editingNodeIndex.value = index
  editingNode.value = index >= 0 ? customNodes.value[index] : null
  showNodeModal.value = true
}

const closeNodeModal = () => {
  showNodeModal.value = false
}

// 保存到节点列表：重名先在本地拦一道（后端也有兜底校验，且会拒绝生成配置）
const saveNodeToList = (node: CustomNode) => {
  const duplicated = customNodes.value.some((item, idx) => idx !== editingNodeIndex.value && item.name === node.name)
  if (duplicated) {
    globalStore.showToast(t('subscription.node_duplicate_name'), 'error')
    return
  }
  if (!currentConfig.value.custom_nodes) {
    currentConfig.value.custom_nodes = []
  }
  if (editingNodeIndex.value >= 0) {
    currentConfig.value.custom_nodes[editingNodeIndex.value] = node
  } else {
    currentConfig.value.custom_nodes.push(node)
  }
  showNodeModal.value = false
}

// 从列表移除节点：只改本地列表，落库同样依赖「保存并应用」
const deleteNode = async (index: number) => {
  const node = customNodes.value[index]
  if (!node) return
  const confirmed = await globalStore.showConfirm({
    title: t('common.confirm_delete'),
    message: `${t('common.confirm_delete')} ${node.name}?`,
    type: 'danger',
  })
  if (confirmed) {
    currentConfig.value.custom_nodes.splice(index, 1)
  }
}

// 从后端重新拉取真实配置（force=true）。store 在已加载时会直接 return 旧快照，
// 因此任何「改完后要看到最新状态」的场景都必须走这里，不能用 loadConfig()。
const reloadConfig = async () => {
  try {
    await subscriptionStore.loadConfig(true)
  } catch (e) {
    console.error('加载订阅配置失败', e)
  }
}
// 记录正在轮询的定时器，避免多次触发或卸载泄露
const activePolls = new Map<number, any>()

const clearPoll = (index: number) => {
  const timer = activePolls.get(index)
  if (timer) {
    clearInterval(timer)
    activePolls.delete(index)
  }
}

// providerInfoToDisplay 把内核 /providers/proxies 返回的 provider 元数据转成展示用的 info。
//
// 键名是内核的写法（大写 Upload/Download/Total/Expire），与持久化元数据的小写键不同——
// 两种来源的归一化都收敛在这里，避免各处各写一份。
const providerInfoToDisplay = (provider: any): SubscriptionInfo | null => {
  if (!provider?.subscriptionInfo) return null
  const info = provider.subscriptionInfo
  return {
    upload: info.Upload || 0,
    download: info.Download || 0,
    total: info.Total || 0,
    expire: info.Expire || 0,
    updatedAt: provider.updatedAt || null,
  }
}

// 获取订阅显示信息。
//
// 融合模式优先用内核 provider 的实时数据；实时数据拿不到时**回退到持久化元数据**
// （后端 subscription-meta.json 里的那份，经 GET /subscribe/config 以 sub.info 返回）：
//   - 刚从切换模式切到融合模式时，内核里还没有这些 provider（要等「保存并应用」重新
//     生成配置并重载），此时前者必然为空——过去这里直接返回 null，卡片就整块变成
//     「流量信息不可用」，而其实上一次更新（切换模式或上一轮融合）存下的流量/到期
//     完全可用；
//   - 内核已加载 provider 但该机场没下发 userinfo 时同样回退，卡片里的「更新于」显示的
//     是缓存那次的时间戳，用户能分辨这是上次已知值而不是实时值。
// 切换模式没有 provider 可查，一直用持久化数据。
const getSubscriptionDisplayInfo = (sub: SubscriptionItem): SubscriptionInfo | null => {
  if (currentConfig.value.mode === 'merge') {
    const live = providerInfoToDisplay(providerInfos.value[sub.name])
    if (live) return live
  }
  return sub.info || null
}

// 预处理所有订阅的显示信息
const subscriptionsWithDisplay = computed(() => {
  return (currentConfig.value.subscriptions || []).map((sub) => ({
    ...sub,
    displayInfo: getSubscriptionDisplayInfo(sub),
  }))
})

// 手动更新单个订阅
const handleUpdateSub = async (index: number) => {
  const sub = currentConfig.value.subscriptions[index]
  if (!sub) return
  if (isUpdating.value[index]) return

  // 记录初始的更新时间以比对
  const initialTime = sub.info?.updatedAt || null

  isUpdating.value[index] = true
  globalStore.showToast(t('rules.updating'), 'info')
  try {
    const encoded = encodeURIComponent(sub.name)
    const resp = await apiFetch(`/subscribe/update/${encoded}`, { method: 'POST' })
    const result = await resp.json()

    if (!resp.ok) {
      globalStore.showToast(`${t('subscription.operation_failed')}: ${result.message || ''}`, 'error')
      await reloadConfig()
      isUpdating.value[index] = false
      return
    }

    if (result.status === 'processing') {
      // 融合模式：异步更新，前端轮询 2s 间隔，最长 30s
      let retries = 0
      const maxRetries = 15
      clearPoll(index)

      const timer = setInterval(async () => {
        retries++
        try {
          // 必须强制拉取：store 已加载时会直接返回旧快照，轮询将永远看不到 updatedAt 变化
          await subscriptionStore.loadConfig(true)
          const updatedSub = currentConfig.value.subscriptions.find(s => s.name === sub.name)
          if (updatedSub && updatedSub.info?.updatedAt !== initialTime) {
            clearPoll(index)
            isUpdating.value[index] = false
            globalStore.showToast(t('subscription.update_success', { name: sub.name }), 'success')
            rulesStore.fetchRules(true)
            rulesStore.fetchProviders(true)
            proxyStore.fetchProxies(true)
          } else if (retries >= maxRetries) {
            clearPoll(index)
            isUpdating.value[index] = false
            globalStore.showToast(`${t('subscription.operation_failed')}: ${t('proxies.timeout')}`, 'error')
          }
        } catch (pollErr) {
          console.error('轮询订阅配置出错:', pollErr)
        }
      }, 2000)

      activePolls.set(index, timer)
    } else if (result.status === 'ok') {
      // 切换模式：同步更新成功
      globalStore.showToast(result.message || t('subscription.update_success', { name: sub.name }), 'success')
      if (result.info) {
        currentConfig.value.subscriptions[index].info = {
          upload: result.info.upload || 0,
          download: result.info.download || 0,
          total: result.info.total || 0,
          expire: result.info.expire || 0,
          updatedAt: result.info.updatedAt || null,
        }
      }
      isUpdating.value[index] = false
      rulesStore.fetchRules(true)
      rulesStore.fetchProviders(true)
      proxyStore.fetchProxies(true)
      // 切换模式下更新激活订阅会重写 config.yaml 并重载内核，而那份文件的 mode 等字段
      // 来自机场：内核常规配置可能因此变化，登记待办让配置/代理/概览页补拉。
      configStore.markCoreConfigStale()
    } else {
      globalStore.showToast(`${t('subscription.operation_failed')}: ${result.message || ''}`, 'error')
      isUpdating.value[index] = false
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
    isUpdating.value[index] = false
  }
}
// 打开模态框
const openSubModal = (index: number = -1) => {
  editingIndex.value = index
  if (index >= 0) {
    modalTitle.value = t('subscription.edit_modal_title')
    const sub = currentConfig.value.subscriptions[index]
    // 复制整个订阅对象而非逐字段重建：列表项上还有展示用的 info（流量/到期）等字段，
    // 重命名或改链接时不该把它们弄丢。规则/隧道属于服务端持有（见 store 的
    // SettingsPayload），保存请求体只挑注册表字段，因此这里带上它们只是保持列表完整。
    editForm.value = { ...sub, custom_rules: sub.custom_rules ?? [] }
  } else {
    modalTitle.value = t('subscription.add_modal_title')
    editForm.value = {
      name: '',
      url: '',
      update_interval: 86400,
      health_interval: 600,
      prefix: '',
      custom_rules: []
    }
  }
  showModal.value = true
}

const closeSubModal = () => {
  showModal.value = false
}

// 保存至订阅列表
const saveSubToList = async () => {
  const { name, url } = editForm.value
  if (!name.trim() || !url.trim()) {
    globalStore.showToast(t('common.name_required'), 'error')
    return
  }
  // 订阅名会用作节点文件名与 provider 键，限制字符集（后端有同样的校验兜底）
  const nameError = validateSubscriptionName(name)
  if (nameError === 'tooLong') {
    globalStore.showToast(t('subscription.name_too_long'), 'error')
    return
  }
  if (nameError !== null) {
    globalStore.showToast(t('subscription.name_invalid'), 'error')
    return
  }
  const isDuplicate = (currentConfig.value.subscriptions || []).some((sub, idx) => {
    return sub.name.trim() === name.trim() && idx !== editingIndex.value
  })
  if (isDuplicate) {
    globalStore.showToast(t('subscription.duplicate_name'), 'error')
    return
  }
  // 后端按「旧订阅名消失 × 新订阅名出现 × 链接相同」识别改名，并据此搬运该订阅的
  // 规则/隧道（rules.json / tunnels.json 里的条目以订阅名为键）。改名同时改链接时无法
  // 判定是同一个订阅，旧条目会被当作孤儿回收——这里先确认，避免静默丢规则。
  if (editingIndex.value >= 0) {
    const editing = currentConfig.value.subscriptions[editingIndex.value]
    const renaming = !!editing && editing.name !== name.trim()
    const urlChanged = !!editing && editing.url !== url.trim()
    const hasResources = (editing?.custom_rules?.length || 0) > 0 || (editing?.tunnels?.length || 0) > 0
    if (renaming && urlChanged && hasResources) {
      const ok = await globalStore.showConfirm({
        title: t('subscription.edit_modal_title'),
        message: t('subscription.rename_url_change_confirm'),
        type: 'warning',
      })
      if (!ok) return
    }
  }
  const subData = { ...editForm.value }
  const wasEmpty = (currentConfig.value.subscriptions || []).length === 0
  if (editingIndex.value >= 0) {
    // 重命名时必须同步迁移选中态：active_subscription 以订阅名为键，
    // 若停留在旧名，切换模式下会「看起来没选中」却仍按旧名保存（复制旧文件）。
    const oldSub = currentConfig.value.subscriptions[editingIndex.value]
    const oldName = oldSub?.name
    currentConfig.value.subscriptions[editingIndex.value] = subData
    if (oldName && oldName !== subData.name && currentConfig.value.active_subscription === oldName) {
      currentConfig.value.active_subscription = subData.name
    }
  } else {
    if (!currentConfig.value.subscriptions) {
      currentConfig.value.subscriptions = []
    }
    currentConfig.value.subscriptions.push(subData)
  }
  showModal.value = false

  // 如果是切换模式且原列表为空（即第一个订阅），自动选中该订阅
  if (currentConfig.value.mode === 'switch' && wasEmpty) {
    currentConfig.value.active_subscription = name
  }
  // 从物理删除暂存列表中移除，以防同名冲突
  pendingPhysicalDeletes.value = pendingPhysicalDeletes.value.filter(n => n !== name.trim())
}

// 删除订阅
const handleDeleteSub = async (index: number) => {
  const sub = currentConfig.value.subscriptions[index]
  if (!sub) return

  const result = await globalStore.showConfirm({
    title: t('common.confirm_delete'),
    message: `${t('common.confirm_delete')} ${sub.name}?`,
    type: 'danger',
    checkboxLabel: t('subscription.delete_physical_file'),
    checkboxDefault: true
  })

  if (result.confirmed) {
    if (result.checkboxChecked) {
      if (!pendingPhysicalDeletes.value.includes(sub.name)) {
        pendingPhysicalDeletes.value.push(sub.name)
      }
    }
    // 删除的若是当前选中的订阅，同步清空选中态，避免残留一个指向已删除订阅的旧名
    if (currentConfig.value.active_subscription === sub.name) {
      currentConfig.value.active_subscription = ''
    }
    currentConfig.value.subscriptions.splice(index, 1)
  }
}

// 保存并应用
const saveAndApply = async () => {
  // 自定义模式：节点列表为空时配置里将没有任何代理（规则与代理组仍在），先确认一次，
  // 避免用户误以为「保存成功 = 有节点可用」。
  if (isCustomMode.value && customNodes.value.length === 0) {
    const confirmed = await globalStore.showConfirm({
      title: t('subscription.save_and_apply'),
      message: t('subscription.custom_no_nodes_confirm'),
      type: 'warning',
    })
    if (!confirmed) return
  }

  // 切换模式下的选中校验：
  // 有订阅时必须选中其中之一，且选中名必须真实存在——改名/删除后可能残留旧名
  // （后端只判空字符串），若不拦截，后端会按旧名复制旧订阅文件，而前端既不选中
  // 任何卡片、界面又显示新名字，造成「显示与生效不一致」。
  // 无订阅时允许保存（清空选中态），后端会据此生成基础配置。
  const subsForSelection = currentConfig.value.subscriptions || []
  if (currentConfig.value.mode === 'switch') {
    if (subsForSelection.length === 0) {
      currentConfig.value.active_subscription = ''
    } else if (!subsForSelection.some(s => s.name === currentConfig.value.active_subscription)) {
      currentConfig.value.active_subscription = ''
      globalStore.showToast(t('subscription.switch_no_selection'), 'error')
      return
    }
  }
  // 端口必填
  if (!currentConfig.value.proxy_port || !currentConfig.value.panel_port) {
    globalStore.showToast(t('subscription.proxy_port') + ' / ' + t('subscription.panel_port') + ' ' + t('common.required'), 'error')
    return
  }
  
  // 端口范围及冲突校验
  const proxyPort = currentConfig.value.proxy_port
  const panelPort = currentConfig.value.panel_port
  const tproxyPort = currentConfig.value.tproxy_port || 0

  const portsToCheck = [proxyPort, panelPort]
  if (tproxyPort !== 0) {
    portsToCheck.push(tproxyPort)
  }

  for (const p of portsToCheck) {
    if (p < 1025 || p > 65535) {
      globalStore.showToast(t('config.port_invalid_hint'), 'error')
      return
    }
  }

  if (new Set(portsToCheck).size !== portsToCheck.length) {
    globalStore.showToast(t('config.port_duplicate_hint'), 'error')
    return
  }
  // 规则集档位：控件只在融合模式下渲染（切换模式用订阅自带规则），因此「必填」校验也只在融合模式下生效。
  // 切换模式遇到缺失/非法档位时归一化为 base 而不是拦截：用户看不到该控件，报必填等于把保存永久卡死；
  // 归一化同时保证之后切回融合模式时生成链路拿到的是合法档位（后者遇到未知档位会拒绝生成配置）。
  if (currentConfig.value.rule_group !== 'base' && currentConfig.value.rule_group !== 'full') {
    if (currentConfig.value.mode === 'merge') {
      globalStore.showToast(t('subscription.rule_group') + ' ' + t('common.required'), 'error')
      return
    }
    currentConfig.value.rule_group = 'base'
  }

  isApplying.value = true
  try {
    // 请求体只含设置类字段（全局参数 + 订阅注册表 + 手工节点），由 store 统一挑字段：
    // 规则/隧道/机场元数据由各自的专用接口维护，不再随「保存并应用」提交
    //（后端即使收到也会忽略；此处不发送是为了让契约显式化，也少传一份元数据）
    const payload = buildSettingsPayload(currentConfig.value, pendingPhysicalDeletes.value)

    const resp = await apiFetch('/subscribe/generate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    const result = await resp.json()

    // 三态处理。后端在「配置已写盘、但热重载内核失败」时返回 200 + status:"warning"
    // （内核未运行是最常见的原因）——那是「已保存、未生效」，不是失败：
    //   - 若当成失败：提示语误导（用户以为没保存），且会跳过下面的状态回刷，
    //     于是 savedMode 停留在旧值，紧接着打开规则/隧道弹窗会被「请先保存并应用」拦下，
    //     而用户刚点过保存，只会反复困惑；
    //   - 待物理删除列表也必须清空，否则下次保存会重复删除同名文件。
    // 同一约定在 CustomRulesDialog 的 notifyMutation 里处理正确，这里补齐。
    if (resp.ok && (result.status === 'ok' || result.status === 'warning')) {
      const isWarning = result.status === 'warning'
      globalStore.showToast(
        result.message || (isWarning ? t('subscription.apply_unsynced') : t('subscription.apply_success')),
        isWarning ? 'warning' : 'success'
      )
      // 清空待物理删除列表
      pendingPhysicalDeletes.value = []
      // 重新加载配置，保持前后端数据一致（必须 force，否则只会读回本地旧快照）
      await reloadConfig()
      // 刷新规则和代理列表
      rulesStore.fetchRules(true)
      rulesStore.fetchProviders(true)
      proxyStore.fetchProxies(true)
      // 登记「内核常规配置已过期」：本次保存重写了 config.yaml 的 mixed-port /
      // tproxy-port / secret / external-controller 并重载了内核，而配置页那一份
      // /configs 快照不会自己更新——它可能在别处挂着，也可能用户随后才切过去。
      // 只留待办，由配置页在 onActivated 里消费（同一套做法见 utils/staleFlag.ts）。
      configStore.markCoreConfigStale()
    } else {
      globalStore.showToast(`${t('subscription.operation_failed')}: ${result.message || ''}`, 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    isApplying.value = false
  }
}

// TPROXY 端口输入框清空时自动置 0
const onTproxyPortInput = (event: Event) => {
  const target = event.target as HTMLInputElement
  if (target.value === '') {
    currentConfig.value.tproxy_port = 0
  }
}

// 辅助格式化：订阅卡片固定 GB 口径，更新时间失败时回退「未知」（见模板）

const formatExpire = (expire: number) => {
  if (!expire) return t('subscription.expire_forever')
  return new Date(expire * 1000).toLocaleString()
}

onMounted(() => {
  // 视图重新挂载时静默强制刷新订阅配置，避免长期显示陈旧数据（§4.1）
  subscriptionStore.loadConfig(true)
})

onActivated(() => {
  subscriptionStore.loadConfig(true)
})

onDeactivated(() => {
  // 改完规则后未关弹窗直接切走：子组件只是被 isActive 隐藏、不会触发 close。
  // 若就此丢弃这次改动，规则页会一直显示旧列表——「漏登记」的代价远大于「多登记一次」，
  // 因此把待上报的作用域从子组件取出来，走与关闭时同一个判定逻辑登记。
  if (!showRulesModal.value) return
  flushRulesStaleMark(rulesDialogRef.value?.takeMutatedScopes() ?? [])
  showRulesModal.value = false
})

onUnmounted(() => {
  // 清理所有未完成的订阅轮询定时器
  activePolls.forEach(timer => clearInterval(timer))
  activePolls.clear()
})
</script>

<template>
  <div class="flex flex-col flex-1 min-h-0 gap-4 h-full">
    <!-- 顶部操作栏：移动端两行（标题+保存 / 模式切换），桌面端一行且模式切换居中
         ——与代理页顶栏同一套布局。 -->
    <PageHeader :icon="MailOutline" gap="gap-3 md:gap-4">
      <template #title>
        {{ t('subscription.title') }}
        <button @click="showHelpModal = true" class="flex items-center justify-center text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition-all hover:scale-105 active:scale-95 p-0.5 -ml-0.5" :title="t('subscription.help_title')">
          <HelpCircleOutline class="w-4 h-4" />
        </button>
      </template>

      <!-- 模式切换：桌面端占据剩余空间并居中，移动端独占第二行 -->
      <div class="flex justify-center order-last w-full md:order-none md:w-auto md:flex-1">
        <div class="flex bg-slate-100 dark:bg-slate-800 rounded-lg p-0.5 w-full md:w-auto transition-all">
          <button
            v-for="m in modeOptions"
            :key="m.value"
            @click="currentConfig.mode = m.value"
            class="flex-1 md:flex-none px-4 md:px-6 py-1.5 text-xs font-semibold rounded-md transition-all duration-200"
            :class="currentConfig.mode === m.value ? 'bg-accent text-white shadow-sm' : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'"
          >
            {{ t(m.label) }}
          </button>
        </div>
      </div>

      <template #actions>
        <button @click="saveAndApply" :disabled="isApplying" class="px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap">
          <SyncOutline v-if="isApplying" class="w-3.5 h-3.5 animate-spin" />
          <SaveOutline v-else class="w-3.5 h-3.5" />
          {{ isApplying ? t('subscription.applying_short') : t('subscription.save_and_apply') }}
        </button>
      </template>
    </PageHeader>

    <!-- 内容区域内滚动容器 -->
    <div class="flex-1 min-h-0 overflow-y-auto glass-medium shadow-none p-6 rounded-xl border border-slate-200/50 dark:border-slate-800/50 transition-all pr-4">
      <!-- 六列栅格：桌面三项同排（各占 2 列）、窄窗口成对排（各占 3 列）。
           刻意不用嵌套包裹层——关闭某项（如 IPv6 支持）时后续字段会自动补位，
           不会留一整行空白；行的归属完全由各项的 col-span 决定。 -->
      <div class="grid grid-cols-6 gap-x-2 gap-y-4 md:gap-x-4">
        <!-- 端口三件套：桌面与窄窗口都排在同一行（整行占满）。
             窄窗口列宽有限，输入框左右内边距在 sm 以下收紧，避免 5 位端口号被数字调节箭头挤掉。 -->
        <div class="col-span-6 grid grid-cols-3 gap-2 md:order-1">
          <div class="flex flex-col gap-2">
            <label class="text-xs font-medium text-slate-600 dark:text-slate-400">{{ t('subscription.proxy_port') }}</label>
            <input type="number" v-model="currentConfig.proxy_port" min="0" max="65535" step="1" class="px-2.5 sm:px-4 py-2.5 text-sm rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none" />
          </div>
          <div class="flex flex-col gap-2">
            <label class="text-xs font-medium text-slate-600 dark:text-slate-400">{{ t('subscription.panel_port') }}</label>
            <input type="number" v-model="currentConfig.panel_port" min="0" max="65535" step="1" class="px-2.5 sm:px-4 py-2.5 text-sm rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none" />
          </div>
          <div class="flex flex-col gap-2">
            <label class="text-xs font-medium text-slate-600 dark:text-slate-400">{{ t('subscription.tproxy_port') }}</label>
            <input
              type="number"
              v-model.number="currentConfig.tproxy_port"
              min="0" max="65535" step="1" 
              @input="onTproxyPortInput"
              :placeholder="t('config.port_disabled_hint')"
              class="px-2.5 sm:px-4 py-2.5 text-sm rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none"
            />
          </div>
        </div>

        <!-- IPv6 支持：总开关。关闭时 DNS 返回项隐藏、值归一为关闭（后端同样强制），
             该行剩下的空位由后面的规则集/面板密钥自动补上。 -->
        <div class="flex flex-col gap-2 order-1 md:order-2" :class="ipv6SpanClass">
          <label class="text-xs font-medium text-slate-600 dark:text-slate-400">{{ t('subscription.ipv6_label') }}</label>
          <select v-model="currentConfig.ipv6" class="px-4 py-2.5 text-sm rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none">
            <option :value="true">{{ t('common.on') }}</option>
            <option :value="false">{{ t('common.off') }}</option>
          </select>
        </div>

        <!-- DNS 返回 IPv6：仅总开关开启时出现。 -->
        <div v-if="currentConfig.ipv6" class="flex flex-col gap-2 order-2 md:order-3" :class="dnsSpanClass">
          <label class="text-xs font-medium text-slate-600 dark:text-slate-400">{{ t('subscription.dns_ipv6_label') }}</label>
          <select v-model="currentConfig.dns_ipv6" class="px-4 py-2.5 text-sm rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none">
            <option :value="true">{{ t('common.on') }}</option>
            <option :value="false">{{ t('common.off') }}</option>
          </select>
        </div>

        <!-- 规则集档位：仅融合模式参与生成（切换模式用订阅自带规则）。 -->
        <div v-if="currentConfig.mode === 'merge'" class="flex flex-col gap-2 order-3 md:order-4" :class="ruleSpanClass">
          <label class="text-xs font-medium text-slate-600 dark:text-slate-400">{{ t('subscription.rule_group') }}</label>
          <select v-model="currentConfig.rule_group" class="px-4 py-2.5 text-sm rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none">
            <option value="base">{{ t('subscription.rule_group_base') }}</option>
            <option value="full">{{ t('subscription.rule_group_full') }}</option>
          </select>
        </div>

        <!-- 外置面板：DOM 顺序排在面板密钥之前，窄窗口才能与规则集同排；
             桌面靠 md:order-6 回到「面板密钥 → 外置面板」的原有次序。 -->
        <div class="flex flex-col gap-2" :class="[uiSpanClass, uiOrderClass]">
          <label class="text-xs font-medium text-slate-600 dark:text-slate-400">{{ t('subscription.ui_panel') }}</label>
          <select v-model="currentConfig.ui_panel" class="px-4 py-2.5 text-sm rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none">
            <option value="metacubexd">MetaCubeXD</option>
            <option value="zashboard">Zashboard</option>
          </select>
        </div>

        <!-- 面板密钥 -->
        <div class="col-span-6 md:col-span-3 flex flex-col gap-2 order-5 md:order-5">
          <label class="text-xs font-medium text-slate-600 dark:text-slate-400">{{ t('subscription.panel_secret') }}</label>
          <div class="relative flex items-center">
            <input :type="showSecret ? 'text' : 'password'" v-model="currentConfig.panel_secret" class="w-full pl-4 pr-10 py-2.5 text-sm rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none" />
            <button @click="showSecret = !showSecret" class="absolute right-3 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200">
              <EyeOutline v-if="showSecret" class="w-5 h-5" />
              <EyeOffOutline v-else class="w-5 h-5" />
            </button>
          </div>
        </div>

        <!-- MetaCubeXD 后端地址：只有 MetaCubeXD 需要，选 Zashboard 时整块不渲染。
             隐藏的只是输入框本身，meta_backend_url 仍留在 currentConfig 里，「保存并应用」
             照常把原内容提交给后端——切来切去不会把已填的地址弄丢。 -->
        <div v-if="isMetaCubeXd" class="col-span-6 md:col-span-3 flex flex-col gap-2 order-7 md:order-7">
          <label class="text-xs font-medium text-slate-600 dark:text-slate-400">{{ t('subscription.meta_backend_url') }}</label>
          <div class="relative flex items-center">
            <input
              :type="showBackendUrl ? 'text' : 'password'"
              v-model="currentConfig.meta_backend_url"
              :placeholder="t('subscription.meta_backend_url_placeholder')"
              class="w-full pl-4 pr-10 py-2.5 text-sm rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none"
            />
            <button
              @click="showBackendUrl = !showBackendUrl"
              class="absolute right-3 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200"
            >
              <EyeOutline v-if="showBackendUrl" class="w-5 h-5" />
              <EyeOffOutline v-else class="w-5 h-5" />
            </button>
          </div>
        </div>
      </div>

      <div class="flex flex-wrap gap-y-3 gap-x-4 items-center justify-between mt-8 mb-4">
        <h4 class="font-semibold text-base shrink-0">{{ isCustomMode ? t('subscription.node_list') : t('subscription.subscription_list') }}</h4>
        <!-- 操作按钮组：贴右显示 -->
        <div class="flex items-center gap-2 ml-auto shrink-0">
          <!-- 自定义规则：融合模式与自定义模式共用同一按钮与同一弹窗（自定义模式固定标准档位） -->
          <button
            v-if="currentConfig.mode === 'merge' || isCustomMode"
            @click="openRulesDialog"
            :title="modeApplied() ? undefined : t('subscription.mode_unsaved_hint')"
            class="px-3.5 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all flex items-center gap-1.5"
            :class="modeApplied() ? '' : 'opacity-60'"
          >
            <OptionsOutline class="w-4 h-4" /> {{ t('subscription.custom_rules') }}
          </button>
          <button @click="isCustomMode ? openNodeModal(-1) : openSubModal(-1)" class="px-3.5 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all flex items-center gap-1.5">
            <AddOutline class="w-4 h-4" /> {{ isCustomMode ? t('subscription.add_node') : t('subscription.add_subscription') }}
          </button>
        </div>
      </div>

      <div id="subList" class="space-y-4">
        <!-- 自定义模式：节点列表（订阅列表不参与生成，故整块替换为节点卡片） -->
        <template v-if="isCustomMode">
          <div v-if="nodeCards.length === 0" class="text-slate-400 dark:text-slate-600 text-sm py-4 text-center">
            {{ t('subscription.no_nodes') }}
          </div>
          <template v-else>
            <CustomNodeCard
              v-for="(card, idx) in nodeCards"
              :key="card.node.id || idx"
              :node="card.node"
              :protocol-name="card.protocolName"
              @edit="openNodeModal(idx)"
              @delete="deleteNode(idx)"
            />
          </template>
        </template>
        <template v-else>
        <div v-if="!currentConfig.subscriptions || currentConfig.subscriptions.length === 0" class="text-slate-400 dark:text-slate-600 text-sm py-4 text-center">
          {{ t('subscription.no_subscriptions') }}
        </div>
        <!-- 卡片循环（已修改：支持点击选中和高亮） -->
        <div 
          v-else 
          v-for="(item, idx) in subscriptionsWithDisplay" 
          :key="item.name" 
          @click="selectSubscription(item.name)"
          class="live-card p-4 rounded-xl border border-slate-200/40 dark:border-slate-800/40 bg-slate-50/50 dark:bg-slate-900/30 flex flex-col gap-3 hover:border-slate-300/80 dark:hover:border-slate-700/80 hover:-translate-y-[3px] hover:shadow-md hover:bg-slate-100/80 dark:hover:bg-slate-900/80 transition-all duration-300 relative overflow-hidden cursor-pointer"
          :class="{
            'border-accent ring-2 ring-accent/30': currentConfig.mode === 'switch' && currentConfig.active_subscription === item.name
          }"
        >
          <!-- 正在更新的卡片遮罩层 -->
          <div v-if="isUpdating[idx]" class="absolute inset-0 glass-light rounded-xl z-10 flex items-center justify-center gap-2 animate-[fadeIn_0.15s_ease-out]">
            <div class="w-4 h-4 border-2 border-slate-300 dark:border-slate-700 !border-t-accent rounded-full animate-spin"></div>
            <span class="text-[11px] font-bold text-slate-500 dark:text-slate-400">
              {{ t('rules.updating') }}
            </span>
          </div>
          <!-- 第一行：订阅名与操作按钮同排（按钮 shrink-0 不换行，名称过长时自行折行） -->
          <div class="flex justify-between items-start gap-3">
            <span class="min-w-0 font-semibold text-slate-800 dark:text-slate-100 break-all">{{ item.name }}</span>
            <div class="flex gap-1.5 shrink-0" @click.stop>
              <button v-if="savedSubNames.has(item.name)" @click="handleUpdateSub(idx)" :disabled="isUpdating[idx]" class="p-2 hover:bg-slate-200 dark:hover:bg-slate-800 text-slate-500 dark:text-slate-400 rounded-lg transition-all" :title="t('rules.update')">
                <SyncOutline class="w-4 h-4 inline-block" :class="{ 'animate-spin': isUpdating[idx] }" />
              </button>
              <button @click="openSubModal(idx)" class="p-2 hover:bg-slate-200 dark:hover:bg-slate-800 text-slate-500 dark:text-slate-400 rounded-lg transition-all" :title="t('common.edit')">
                <CreateOutline class="w-4 h-4" />
              </button>
              <button v-if="currentConfig.mode === 'switch' && savedSubNames.has(item.name)" @click="openSubRulesDialog(item.name)" class="p-2 hover:bg-slate-200 dark:hover:bg-slate-800 text-slate-500 dark:text-slate-400 rounded-lg transition-all" :title="t('subscription.custom_rules')">
                <OptionsOutline class="w-4 h-4" />
              </button>
              <button @click="handleDeleteSub(idx)" class="p-2 hover:bg-red-500/10 hover:text-red-500 text-slate-500 dark:text-slate-400 rounded-lg transition-all" :title="t('common.delete')">
                <TrashOutline class="w-4 h-4" />
              </button>
            </div>
          </div>

          <!-- 第二行：订阅链接独占整行（不再与按钮共享宽度），窄屏下也能拿到卡片满宽 -->
          <div class="-mt-2 text-xs text-slate-400 dark:text-slate-500 select-none break-all flex items-start gap-1.5 min-w-0">
            <button @click.stop="showUrls[idx] = !showUrls[idx]" class="shrink-0 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 focus:outline-none" :title="showUrls[idx] ? t('subscription.hide_url') : t('subscription.show_url')">
              <EyeOffOutline v-if="showUrls[idx]" class="w-3.5 h-3.5" />
              <EyeOutline v-else class="w-3.5 h-3.5" />
            </button>
            <span class="min-w-0">{{ showUrls[idx] ? item.url : '••••••••' }}</span>
          </div>

          <!-- 信息展示 -->
          <div v-if="item.displayInfo" class="space-y-2">
            <div class="flex items-center gap-3">
              <div class="flex-1 bg-slate-200 dark:bg-slate-800 h-2 rounded-full overflow-hidden">
                <div class="bg-accent h-full rounded-full transition-all" :style="{ width: Math.min(((item.displayInfo.upload + item.displayInfo.download) / (item.displayInfo.total || 1)) * 100, 100) + '%' }">
                </div>
              </div>
              <span class="text-xs font-semibold text-accent">{{ ((item.displayInfo.upload + item.displayInfo.download) / (item.displayInfo.total || 1) * 100).toFixed(1) }}%</span>
            </div>
            <div class="flex justify-between text-xs text-slate-500 dark:text-slate-400">
              <span>{{ formatGB(item.displayInfo.upload + item.displayInfo.download) }} / {{ formatGB(item.displayInfo.total) }}</span>
              <span>{{ t('subscription.valid_until_label') }}{{ formatExpire(item.displayInfo.expire) }}</span>
            </div>
            <div class="flex justify-between text-[11px] text-slate-400 dark:text-slate-500 mt-1">
              <span>{{ t('subscription.updated_at_label') }}{{ formatDateTime(item.displayInfo.updatedAt) || t('common.unknown') }}</span>
            </div>
          </div>
          <div v-else class="text-xs text-slate-400 dark:text-slate-500">
            <template v-if="!savedSubNames.has(item.name)">
              {{ t('subscription.save_to_show_info') }}
            </template>
            <template v-else>
              {{ !coreStatus.running ? `${t('config.core_stopped')}${t('common.list_sep')}${t('subscription.traffic_unavailable')}` : t('subscription.traffic_unavailable') }}
            </template>
          </div>
        </div>
        </template>
      </div>
    </div>

    <!-- 保存并应用全屏模糊加载浮层 -->
    <Teleport to="body">
      <div v-if="isActive && isApplying" class="fixed inset-0 glass-mask z-[9999] flex flex-col items-center justify-center gap-3 animate-[fadeIn_0.2s_ease-out]">
        <div class="glass-medium border px-6 py-4 rounded-2xl shadow-xl flex items-center gap-3">
          <div class="w-5 h-5 border-2 border-slate-200 dark:border-slate-800 !border-t-accent rounded-full animate-spin"></div>
          <span class="text-xs font-bold text-slate-600 dark:text-slate-300">{{ t('subscription.applying') }}</span>
        </div>
      </div>
    </Teleport>

    <!-- 使用说明弹窗 -->
    <AppModal
      :open="isActive && showHelpModal"
      :label="t('subscription.help_title')"
      panel-class="w-full max-w-lg max-h-[85vh] rounded-[20px] p-6 flex flex-col gap-4"
      @close="showHelpModal = false"
    >
      <div class="flex justify-between items-center border-b border-slate-100 dark:border-slate-800 pb-3 shrink-0">
        <h2 class="text-lg font-bold">{{ t('subscription.help_title') }}</h2>
        <button @click="showHelpModal = false" class="text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 flex items-center justify-center p-1 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all">
          <CloseOutline class="w-5 h-5" />
        </button>
      </div>
      <!-- pr-2 给滚动条留位：覆盖式滚动条会压住文字右边缘。 -->
      <div class="flex-1 min-h-0 overflow-y-auto pr-2 text-sm text-slate-700 dark:text-slate-300 whitespace-pre-wrap leading-relaxed">
        {{ t('subscription.help_content') }}
      </div>
    </AppModal>

    <!-- 添加 / 编辑订阅弹窗 -->
    <AppModal
      :open="isActive && showModal"
      :label="modalTitle"
      :close-on-backdrop="false"
      panel-class="w-full max-w-lg rounded-[20px] p-6 flex flex-col gap-4"
      @close="closeSubModal"
    >
      <div class="flex justify-between items-center border-b border-slate-100 dark:border-slate-800 pb-3">
        <h2 class="text-lg font-bold">{{ modalTitle }}</h2>
        <button @click="closeSubModal" class="text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 flex items-center justify-center p-1 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all">
          <CloseOutline class="w-5 h-5" />
        </button>
      </div>

      <div class="space-y-4">
        <div class="flex flex-col gap-1.5">
          <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('subscription.name') }}</label>
          <input
            type="text"
            :value="editForm.name"
            @input="editForm.name = filterSubscriptionNameInput(($event.target as HTMLInputElement).value)"
            :maxlength="MAX_SUBSCRIPTION_NAME_LENGTH"
            :placeholder="t('subscription.name_placeholder')"
            class="px-3.5 py-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none text-sm"
          />
          <p v-if="nameHint" class="text-[11px] text-danger">{{ nameHint }}</p>
        </div>
        <div class="flex flex-col gap-1.5">
          <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('subscription.url') }}</label>
          <input type="text" v-model="editForm.url" placeholder="https://example.com/sub" class="px-3.5 py-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none text-sm" />
        </div>
        <div class="flex flex-col gap-1.5">
          <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('subscription.update_interval') }}</label>
          <input type="number" v-model="editForm.update_interval" placeholder="86400" class="px-3.5 py-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none text-sm" />
        </div>
        <div class="flex flex-col gap-1.5">
          <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('subscription.health_interval') }}</label>
          <input type="number" v-model="editForm.health_interval" placeholder="300" class="px-3.5 py-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none text-sm" />
        </div>
        <div class="flex flex-col gap-1.5">
          <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('subscription.prefix') }}</label>
          <input type="text" v-model="editForm.prefix" :placeholder="t('subscription.prefix_placeholder')" class="px-3.5 py-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none text-sm" />
        </div>
      </div>

      <p class="text-xs text-slate-400 dark:text-slate-500 leading-normal">{{ t('subscription.modal_hint') }}</p>

      <div class="flex justify-end gap-2.5 pt-4 border-t border-slate-100 dark:border-slate-800">
        <button @click="closeSubModal" class="px-4 py-2 text-xs font-semibold rounded-lg bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 transition-all">
          {{ t('subscription.cancel') }}
        </button>
        <button @click="saveSubToList" class="px-4 py-2 text-xs font-semibold rounded-lg bg-accent hover:bg-accent-hover text-white transition-all shadow-md shadow-accent/15">
          {{ t('subscription.save_to_list') }}
        </button>
      </div>
    </AppModal>

    <!-- 自定义规则弹窗：切换模式（作用域=订阅）、融合模式（作用域=base/full 两档）、
         自定义模式（作用域=标准档位）共用同一组件。
         写一条保存一条，没有整体保存按钮；关闭时由组件回报「改动过的作用域」，父组件据此登记规则页的过期标记 -->
    <CustomRulesDialog
      ref="rulesDialogRef"
      :visible="showRulesModal"
      :is-active="isActive"
      :title="rulesTitle"
      :hint="rulesHint"
      :endpoint="rulesEndpoint"
      :tunnel-endpoint="rulesTunnelEndpoint"
      :scopes="rulesScopes"
      @close="closeRulesDialog"
    />

    <!-- 添加 / 编辑节点弹窗（自定义模式）：表单由后端下发的协议字段表驱动 -->
    <CustomNodeDialog
      :visible="showNodeModal"
      :is-active="isActive"
      :protocols="nodeProtocols"
      :node="editingNode"
      :taken-names="customNodes.filter((_, idx) => idx !== editingNodeIndex).map(node => node.name)"
      @save="saveNodeToList"
      @close="closeNodeModal"
    />
  </div>

</template>
