<script setup lang="ts">
import { ref, computed, onMounted, onActivated } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiFetch, readErrorMessage } from '../utils/api'
import { OptionsOutline, HardwareChipOutline, ShieldCheckmarkOutline, BuildOutline, SearchOutline, SyncOutline, ColorPaletteOutline, SettingsOutline, InformationCircleOutline, DocumentTextOutline } from '@vicons/ionicons5'
import { useGlobalStore } from '../store/global'
import { storeToRefs } from 'pinia'
import { useConfigStore, type ConfigData } from '../store/config'
import { useOverviewStore, CORE_VERSION_LOADING, CORE_VERSION_UNKNOWN } from '../store/overview'
import { useProxyStore } from '../store/proxies'
import { useRulesStore } from '../store/rules'
import FormSwitch from '../components/FormSwitch.vue'
import { useViewActive } from '../composables/useViewActive'

const { t, locale } = useI18n()
const globalStore = useGlobalStore()
const configStore = useConfigStore()
const overviewStore = useOverviewStore()
const proxyStore = useProxyStore()
const rulesStore = useRulesStore()
const { configs, configsLoading, coreStatus } = storeToRefs(configStore)
const { stats } = storeToRefs(overviewStore)
const fetchConfigs = configStore.fetchConfigs

const coreVersion = computed(() => {
  if (stats.value.coreVersion === CORE_VERSION_LOADING) return t('common.loading')
  if (stats.value.coreVersion === CORE_VERSION_UNKNOWN) return ''
  return 'v' + stats.value.coreVersion
})

export interface CoreStatus {
  running: boolean
  loading: boolean
}

// DNS 查询测试
const dnsQuery = ref({
  name: '',
  type: 'A',
  result: '',
  loading: false
})

const isUpgrading = ref(false)
const isReloading = ref(false)
const isFlushingFakeIP = ref(false)
const isFlushingDNS = ref(false)
const isUpdatingGeo = ref(false)

const interfaces = ref<string[]>([])
const fetchInterfaces = async () => {
  try {
    const resp = await apiFetch('/interfaces')
    if (resp.ok) {
      interfaces.value = await resp.json()
    }
  } catch (e) {
    console.error('获取网卡列表失败:', e)
  }
}

// TUN 高级设置弹窗：TUN 设备名 + 出口网卡
// 实时修改（字段变更即写回后端），无保存按钮，仅关闭。
const showTunAdvancedDialog = ref(false)

// TProxy 绕过列表弹窗
const showTproxyExceptionsDialog = ref(false)

// 视图激活态：KeepAlive 停用时必须收起 Teleport 弹窗，否则会跨页残留
const isActive = useViewActive()

const tproxyDstExceptionsText = ref('')
const tproxySrcExceptionsText = ref('')

// 后端预填的绕过列表（GET /config/tproxy/exceptions 的 defaults 字段）。
// 只在 store.go 维护一份清单，前端不复制内容，「恢复默认」按钮直接用这里的值。
const tproxyDefaults = ref<{ dst: string[], src: string[] } | null>(null)

// 本机流量接管开关（已从弹窗移至开关行下方，故状态提升为页面级）
const tproxyProxyLocal = ref(false)
const tproxyProxyLocalLoaded = ref(false)

// 接管 IPv6 开关（默认关闭）。与「接管本机流量」同为页面级状态：
// 二者都只在 TProxy 开关行下方展示，且启用 TProxy 期间均置灰不可改。
const tproxyIPv6 = ref(false)
const tproxyIPv6Loaded = ref(false)
// 订阅中心的「IPv6 支持」总开关（生成期设置）。关闭时 TUN / TProxy 的 IPv6 接管
// 偏好照旧保存，但**不会生效**；这里只用于把控件置灰并说明原因。
const globalIPv6 = ref(true)

// 打开 TUN 高级设置弹窗：弹窗内两项均为实时修改，无需预设值，
// 仅确保网卡列表已加载
const openTunAdvancedDialog = async () => {
  if (interfaces.value.length === 0) {
    await fetchInterfaces()
  }
  showTunAdvancedDialog.value = true
}

// 读取本机流量接管开关状态
const fetchTproxyProxyLocal = async () => {
  try {
    const resp = await apiFetch('/config/tproxy/proxy-local')
    if (resp.ok) {
      const data = await resp.json()
      tproxyProxyLocal.value = data.enabled
      tproxyProxyLocalLoaded.value = true
    }
  } catch (e) {
    console.error('获取本机流量接管开关失败:', e)
  }
}

// 读取接管 IPv6 开关状态（默认关闭）
const fetchTproxyIPv6 = async () => {
  try {
    const resp = await apiFetch('/config/tproxy/proxy-ipv6')
    if (resp.ok) {
      const data = await resp.json()
      tproxyIPv6.value = data.enabled
      if (typeof data.global_ipv6 === 'boolean') globalIPv6.value = data.global_ipv6
      tproxyIPv6Loaded.value = true
    }
  } catch (e) {
    console.error('获取 IPv6 接管开关失败:', e)
  }
}

// 打开 TProxy 高级设置弹窗（IPv6 接管 + 绕过列表）：同时取回绕过列表本体与后端预填内容（供「恢复默认」用）
const openTproxyExceptionsDialog = async () => {
  if (configStore.tproxyEnabled) {
    globalStore.showToast(t('config.tproxy_exceptions_disabled_message'), 'warning')
    return
  }
  try {
    const resp = await apiFetch('/config/tproxy/exceptions')
    if (resp.ok) {
      const data = await resp.json()
      tproxyDstExceptionsText.value = (data.dst || []).join('\n')
      // 直接使用后端数据，不额外填充默认值（老配置里已有用户的列表，不能被默认值覆盖）
      tproxySrcExceptionsText.value = (data.src || []).join('\n')
      tproxyDefaults.value = data.defaults || null
    }
    showTproxyExceptionsDialog.value = true
  } catch (e) {
    globalStore.showToast(t('common.error'), 'error')
  }
}

// 恢复默认：把后端预填内容灌回文本框。刻意不直接落盘——用户可能只想参考默认值，
// 再按自己的网络改几行，因此仍需手动点「保存」。
const restoreTproxyDefaults = () => {
  const defaults = tproxyDefaults.value
  if (!defaults) {
    globalStore.showToast(t('config.tproxy_defaults_unavailable'), 'warning')
    return
  }
  tproxyDstExceptionsText.value = (defaults.dst || []).join('\n')
  tproxySrcExceptionsText.value = (defaults.src || []).join('\n')
  globalStore.showToast(t('config.tproxy_defaults_filled'), 'info')
}

// 仅保存绕过列表（本机流量接管开关已移出该弹窗）
const saveTproxyExceptions = async () => {
  const dstLines = tproxyDstExceptionsText.value.split('\n').map(s => s.trim()).filter(s => s !== '')
  const srcLines = tproxySrcExceptionsText.value.split('\n').map(s => s.trim()).filter(s => s !== '')
  try {
    const resp = await apiFetch('/config/tproxy/exceptions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dst: dstLines, src: srcLines })
    })
    if (resp.ok) {
      globalStore.showToast(t('config.tproxy_exceptions_saved'), 'success')
      showTproxyExceptionsDialog.value = false
    } else {
      globalStore.showToast(t('common.operation_failed'), 'error')
    }
  } catch (e) {
    globalStore.showToast(t('common.error'), 'error')
  }
}

// 统一修改配置
//
// 失败时必须**回读服务端**：调用方（FormSwitch / 端口输入框）已经用 v-model 把新值
// 写进了 configs，而内核并没有接受它。若只弹一句提示、不做任何回读，界面上会留下一个
// 「看起来已生效、实际没生效」的开关——比回滚成旧值更难发现。这里用 fetchConfigs 的
// 静默模式把内核的真实状态拉回来覆盖本地，并在提示里带上后端给出的原因。
const patchConfig = async (payload: Partial<ConfigData>): Promise<boolean> => {
  try {
    const resp = await apiFetch('/configs', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (resp.ok) {
      // 静默同步：仅就地更新开关/端口/选项状态，不触发整卡遮罩与重渲染
      fetchConfigs(false, true)
      return true
    }
    globalStore.showToast(
      `${t('common.operation_failed')}: ${await readErrorMessage(resp)}`,
      'error'
    )
    // 强制回读（forceLoading=false 但 silent=false 会走整卡加载，此处仍用静默模式 +
    // 显式 force，确保拿到的是内核此刻的真实取值）
    fetchConfigs(true, true)
    return false
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
    fetchConfigs(true, true)
    return false
  }
}

// tproxyPortBeforeEdit 记录「聚焦端口输入框那一刻」的取值。
//
// v-model 会在输入过程中直接改写 configs['tproxy-port']，等到 savePorts 执行时已经
// 无法判断用户是否真的改过；而这个判断是必要的——端口表单是「一失焦就整体提交」的，
// 只是点一下输入框再点走（移动端很容易发生）不该弹「已修改」的提示。
const tproxyPortBeforeEdit = ref<number | null>(null)

const onTproxyPortFocus = () => {
  tproxyPortBeforeEdit.value = configs.value['tproxy-port'] ?? 0
}

const toggleAllowLan = () => {
  patchConfig({ 'allow-lan': configs.value['allow-lan'] })
}

const changeMode = () => {
  patchConfig({ mode: configs.value.mode })
}

const changeLogLevel = () => {
  patchConfig({ 'log-level': configs.value['log-level'] })
}

const saveInterface = () => {
  patchConfig({ 'interface-name': configs.value['interface-name'] })
}

const savePorts = async (e?: Event) => {
  if (e && e.type === 'keyup' && e.target instanceof HTMLElement) {
    e.target.blur()
    return
  }

  const port = configs.value.port || 0
  const socksPort = configs.value['socks-port'] || 0
  const redirPort = configs.value['redir-port'] || 0
  const tproxyPort = configs.value['tproxy-port'] || 0
  const mixedPort = configs.value['mixed-port'] || 0

  const ports = [port, socksPort, redirPort, tproxyPort, mixedPort]

  for (const p of ports) {
    if (p !== 0 && (p < 1025 || p > 65535)) {
      globalStore.showToast(t('config.port_invalid_hint'), 'error')
      fetchConfigs(false, true)
      return
    }
  }

  const activePorts = ports.filter(p => p !== 0)
  if (new Set(activePorts).size !== activePorts.length) {
    globalStore.showToast(t('config.port_duplicate_hint'), 'error')
    fetchConfigs(false, true)
    return
  }

  // TProxy 端口与前一次「聚焦时记下的取值」比较：只有真的改了才需要额外反馈
  const tproxyPortChanged = configStore.tproxyEnabled
    && tproxyPortBeforeEdit.value !== null
    && tproxyPortBeforeEdit.value !== tproxyPort

  // 仅 PATCH 内核运行时配置，不触碰订阅配置的持久化和内存状态
  const ok = await patchConfig({
    port,
    'socks-port': socksPort,
    'redir-port': redirPort,
    'tproxy-port': tproxyPort,
    'mixed-port': mixedPort
  })

  if (tproxyPortChanged) {
    // 失败时 patchConfig 已经弹过带后端原因的 error toast（例如「TPROXY 已启用，
    // 不能把端口设为 0」），这里不再叠加一条。
    if (ok) {
      globalStore.showToast(t('config.tproxy_port_updated'), 'success')
      tproxyPortBeforeEdit.value = tproxyPort
    } else {
      // 失败时后端已把端口回滚、上面的 fetchConfigs 会把内核真实取值拉回来，
      // 此时「编辑前取值」已无意义：置空等下次聚焦重新记录，否则用户随即再失焦
      // 会被判成「刚改过」而多弹一次成功提示。
      tproxyPortBeforeEdit.value = null
    }
  }
}

// ---------- TUN 运行参数（tun.json）与启停 ----------
//
// TUN 的「参数」与「开关」是两套东西：
//   - 参数（stack / device / dns-mode / ipv6）落 tun.json（稀疏存储），**只在下次开启时生效**；
//   - 开关（enable）是纯运行时状态：不落盘、不回放。config.yaml 里的 tun.enable 恒为
//     false（NAS 安全策略），任何重载/重启都会关闭 TUN，需要用户重新手动开启。
// 所以这里拆成三个调用：saveTunSettings 只落参数、setTunEnabled 只动内核、
// fetchTunSettings 读生效值与宿主 IPv6 能力。
interface TunSettings {
  stack: string
  device: string
  dns_mode: string
  ipv6: string
}

const tunSettings = ref<TunSettings>({ stack: 'mixed', device: '', dns_mode: 'hijack', ipv6: 'off' })
const tunHostIPv6 = ref(true)

const fetchTunSettings = async () => {
  try {
    const resp = await apiFetch('/config/tun')
    if (!resp.ok) return
    const data = await resp.json()
    tunSettings.value = data.effective
    tunHostIPv6.value = !!data.host_ipv6
    if (typeof data.global_ipv6 === 'boolean') globalIPv6.value = data.global_ipv6
  } catch (e) {
    console.error('获取 TUN 设置失败:', e)
  }
}

const saveTunSettings = async (patch: Partial<TunSettings>): Promise<boolean> => {
  try {
    const resp = await apiFetch('/config/tun', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch)
    })
    if (!resp.ok) {
      globalStore.showToast(`${t('common.operation_failed')}: ${await readErrorMessage(resp)}`, 'error')
      // 失败时回读服务端：下拉框已经用 v-model 改成本地值，不回读会留下一个
      // 「看起来改过、实际没保存」的选项（例如宿主不支持 IPv6 时选了强制开启）。
      await fetchTunSettings()
      return false
    }
    const data = await resp.json()
    tunSettings.value = data.effective
    tunHostIPv6.value = !!data.host_ipv6
    if (typeof data.global_ipv6 === 'boolean') globalIPv6.value = data.global_ipv6
    // 参数只在开启 TUN 时下发；正在运行就先提示，避免用户以为已经生效
    if (configs.value.tun.enable) {
      globalStore.showToast(t('config.tun_setting_next_enable'), 'warning')
    }
    return true
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
    await fetchTunSettings()
    return false
  }
}

const setTunEnabled = async (enable: boolean): Promise<boolean> => {
  try {
    const resp = await apiFetch('/config/tun/state', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enable })
    })
    if (!resp.ok) {
      globalStore.showToast(`${t('common.operation_failed')}: ${await readErrorMessage(resp)}`, 'error')
      return false
    }
    return true
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
    return false
  }
}

// disableTproxyOnBackend 通知后端关闭 TProxy，并同步本地开关。
// 后端会把开关状态落盘，因此这里以服务端返回值为准。
const disableTproxyOnBackend = async () => {
  try {
    const resp = await apiFetch('/config/tproxy', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enable: false })
    })
    if (!resp.ok) {
      globalStore.showToast(t('common.operation_failed'), 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    // 无论成败都以后端实际状态为准，避免本地与后端不一致
    await configStore.refreshTproxyState()
  }
}

const toggleTProxy = async (enable: boolean) => {
  if (enable) {
    const port = configs.value['tproxy-port'] || 0
    if (port === 0) {
      globalStore.showToast(t('config.tproxy_port_zero_warning'), 'warning')
      // 回退开关状态
      configStore.tproxyEnabled = false
      return
    }
    // 互斥：如果 TUN 开启，关闭 TUN（走 /config/tun/state，不落盘、不回放）
    if (configs.value.tun.enable) {
      await setTunEnabled(false)
      await fetchConfigs(false, true)
    }
  }

  // 调用后端更新状态（不涉及端口）
  //
  // 后端在规则安装失败或端口为 0 时会拒绝请求并回滚开关状态，
  // 因此这里必须检查响应，把失败原因反馈给用户，并统一以服务端状态为准，
  // 避免出现「界面显示已启用、实际规则未生效」的静默错配。
  try {
    const resp = await apiFetch('/config/tproxy', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enable })
    })
    if (!resp.ok) {
      let msg = t('common.operation_failed')
      try {
        const data = await resp.json()
        if (data.message) msg = data.message
      } catch (_) {}
      globalStore.showToast(msg, 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    // 刷新状态（以服务端返回值为准，失败时即完成回滚）
    await configStore.refreshTproxyState()
  }
}

// 登记「代理页 / 规则页数据已过期」（机制见 utils/staleFlag）。
//
// 内核的每一次运行态重建都发生在本页：启动、重启、重载、升级内核都会让内核按磁盘上的
// config.yaml 重新加载（升级还会换掉二进制），代理组/节点/当前选择与规则集合都可能与
// 前端快照不同。此处不替那两个页面发请求——它们可能根本没挂载，也无从判断用户接下来
// 会不会看——只留下待办，等用户真正切过去时由它们消费并静默补拉。
const markKernelDataStale = () => {
  proxyStore.markNeedsRefresh()
  rulesStore.markNeedsRefresh()
}

// 内核进程管理
const handleStartCore = async () => {
  coreStatus.value.loading = true
  try {
    const resp = await apiFetch('/core/start', { method: 'POST' })
    const data = await resp.json()
    if (resp.ok && data.status === 'ok') {
      globalStore.showToast(t('config.core_start_success'), 'success')
      configStore.refreshCoreStatus()
      // 立即登记一次：覆盖「用户在下面的等待窗口内就切到代理/规则页」的情形
      // （那一刻内核多半还没起来，请求会失败，消费方会把标记放回去，下次切入重试）
      markKernelDataStale()
      setTimeout(() => {
        fetchConfigs(true)
        overviewStore.fetchVersionAndStatus()
        // 内核此刻才真正回来，运行态以这一次为准，再登记一次
        markKernelDataStale()
      }, 1500)
    } else {
      globalStore.showToast(t('config.core_start_failed') + ': ' + (data.message || ''), 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    coreStatus.value.loading = false
  }
}

const handleStopCore = async () => {
  const ok = await globalStore.showConfirm({
    message: t('config.confirm_stop_core'),
    type: 'danger'
  })
  if (!ok) return
  coreStatus.value.loading = true
  try {
    const resp = await apiFetch('/core/stop', { method: 'POST' })
    const data = await resp.json()
    if (resp.ok && data.status === 'ok') {
      globalStore.showToast(t('config.core_stopped_success'), 'success')
      configStore.refreshCoreStatus()
      configsLoading.value = true
      configs.value = {
        'allow-lan': false,
        ipv6: false,
        mode: 'Rule',
        'log-level': 'silent',
        'interface-name': '',
        tun: { enable: false, stack: 'System', device: '' },
        port: 0,
        'socks-port': 0,
        'redir-port': 0,
        'tproxy-port': 0,
        'mixed-port': 0
      }
    } else {
      globalStore.showToast(t('config.core_stop_failed') + ': ' + (data.message || ''), 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    coreStatus.value.loading = false
  }
}

const handleRestartCore = async () => {
  const ok = await globalStore.showConfirm({
    message: t('config.confirm_restart'),
    type: 'warning'
  })
  if (!ok) return
  coreStatus.value.loading = true
  try {
    const resp = await apiFetch('/restart', { method: 'POST' })
    if (resp.ok) {
      globalStore.showToast(t('config.restart_sent'), 'success')
      // 与启动内核同理：立即登记一次以覆盖等待窗口内的切页，延迟再登记一次为准
      markKernelDataStale()
      setTimeout(async () => {
        // 以服务端为准重新确认内核状态：顺带复位 coreStatus.loading。
        //
        // 该标记的含义是「内核状态未知」，为真时整页渲染成加载占位，而它此前只有
        // App.vue 中监听运行状态变化的 watcher 会复位——内核自重启若未产生运行状态变化
        // （PID 未变、后端不推 SSE，实测如此），成功分支就再没有任何复位点，配置页会永久
        // 卡在 spinner 上。refreshCoreStatus 会更新 running，且无论请求成败都会把 loading
        // 置回 false（/core/status 读的是后端 PID 文件，不依赖内核 API 是否已就绪）。
        await configStore.refreshCoreStatus()
        fetchConfigs(true)
        overviewStore.fetchVersionAndStatus()
        markKernelDataStale()
      }, 1500)
    } else {
      globalStore.showToast(t('config.restart_failed'), 'error')
      coreStatus.value.loading = false
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
    coreStatus.value.loading = false
  }
}

const handleReloadConfig = async () => {
  isReloading.value = true
  try {
    const resp = await apiFetch('/configs', { method: 'PUT' })
    if (resp.ok) {
      globalStore.showToast(t('config.reload_success'), 'success')
      // 重载属于配置同步，同样静默刷新，避免卡片标题闪烁
      fetchConfigs(false, true)
      // 重载会让内核按磁盘上的 config.yaml 重建整个运行态：代理组/节点与规则集合
      // 都可能与前端快照不同。此处不替代理页/规则页发请求（它们可能根本没挂载，
      // 也无法判断用户接下来会不会看），只登记过期标记；对应页面在切入时消费并静默补拉。
      markKernelDataStale()
    } else {
      globalStore.showToast(t('config.reload_failed'), 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    isReloading.value = false
  }
}

const handleFlushFakeIP = async () => {
  isFlushingFakeIP.value = true
  try {
    const resp = await apiFetch('/cache/fakeip/flush', { method: 'POST' })
    if (resp.ok) globalStore.showToast(t('config.flush_fakeip_success'), 'success')
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    isFlushingFakeIP.value = false
  }
}

const handleFlushDNS = async () => {
  isFlushingDNS.value = true
  try {
    const resp = await apiFetch('/cache/dns/flush', { method: 'POST' })
    if (resp.ok) globalStore.showToast(t('config.flush_dns_success'), 'success')
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    isFlushingDNS.value = false
  }
}

const handleUpdateGeo = async () => {
  isUpdatingGeo.value = true
  try {
    let resp = await apiFetch('/providers/geo', { method: 'POST' }).catch(() => null)
    if (!resp || !resp.ok) {
      resp = await apiFetch('/configs/geo', { method: 'POST' })
    }
    if (resp.ok) {
      globalStore.showToast(t('config.update_geo_sent'), 'success')
    } else {
      globalStore.showToast(t('config.update_geo_failed'), 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    isUpdatingGeo.value = false
  }
}

// 内核更新通道：由原生下拉选择，供「更新」按钮使用
const selectedChannel = ref<'release' | 'alpha'>('release')

// 升级核心（通道取自下拉选择：release=稳定版 / alpha=预览版）
const handleUpgradeCore = async () => {
  if (isUpgrading.value) return
  const channel = selectedChannel.value
  isUpgrading.value = true
  try {
    const url = `/upgrade?channel=${channel}`
    const resp = await apiFetch(url, { method: 'POST' })
    if (resp.ok) {
      // **** 关键修复：强制重置版本号为 loading 哨兵，使下次请求重新获取 ****
      overviewStore.stats.coreVersion = CORE_VERSION_LOADING
      globalStore.showToast(t('config.upgrade_success'), 'success')
      // 升级完成后内核会自行重启并重新加载 config.yaml，运行态随之重建。
      // 立即登记一次，覆盖「升级/重启尚未完成、用户已切到代理/规则页」的情形
      markKernelDataStale()
      // 延迟刷新，等待内核完成更新
      setTimeout(async () => {
        let attempts = 0
        const maxAttempts = 10 // 增加尝试次数
        while (attempts < maxAttempts) {
          try {
            await overviewStore.fetchVersionAndStatus()
            // 检查是否成功获取到有效版本（非 loading 哨兵或 unknown 哨兵）
            if (overviewStore.stats.coreVersion && 
                overviewStore.stats.coreVersion !== CORE_VERSION_LOADING && 
                overviewStore.stats.coreVersion !== CORE_VERSION_UNKNOWN) {
              break
            }
          } catch (_) {}
          attempts++
          await new Promise(resolve => setTimeout(resolve, 1000))
        }
        fetchConfigs(true)
        // 版本已能取到＝内核已回来且已按 config.yaml 重建运行态，以这次登记为准
        markKernelDataStale()
      }, 2000) // 初始延迟 2 秒
    } else {
      // 错误处理（保持不变）
      let errorMsg = ''
      try {
        const data = await resp.json()
        if (data.message) errorMsg = data.message
      } catch (_) {
        try {
          errorMsg = await resp.text()
        } catch (_) {}
      }
      if (errorMsg.includes('already using latest version')) {
        globalStore.showToast(t('config.upgrade_already_latest_channel', {
          channel: channel === 'alpha' ? t('config.upgrade_alpha') : t('config.upgrade_stable'),
        }), 'warning')
      } else {
        globalStore.showToast(`${t('config.upgrade_failed')}${errorMsg ? ': ' + errorMsg : ''}`, 'error')
      }
    }
  } catch (e) {
    globalStore.showToast(t('config.upgrade_network_error'), 'error')
  } finally {
    isUpgrading.value = false
  }
}

const handleDNSQuery = async (e?: Event) => {
  if (!dnsQuery.value.name.trim()) return

  // 主动释放焦点，在移动端自动收起虚拟键盘，以便用户看清下方的解析结果
  if (e && e.target instanceof HTMLElement) {
    e.target.blur()
  }

  dnsQuery.value.loading = true
  dnsQuery.value.result = ''
  try {
    const query = `name=${encodeURIComponent(dnsQuery.value.name)}&type=${dnsQuery.value.type}`
    const resp = await apiFetch(`/dns/query?${query}`)
    if (resp.ok) {
      const data = await resp.json()
      if (data.Status === 0 && data.Answer && data.Answer.length > 0) {
        dnsQuery.value.result = data.Answer.map((a: any) => a.data).join('\n')
      } else {
        dnsQuery.value.result = JSON.stringify(data, null, 2)
      }
    } else {
      dnsQuery.value.result = t('config.dns_query_failed')
    }
  } catch (e) {
    dnsQuery.value.result = `${t('config.dns_query_failed')}: ${(e as Error).message}`
  } finally {
    dnsQuery.value.loading = false
  }
}

// 处理本机流量接管开关。
//
// 语义（与用户约定一致）：
//   - 开启：先二次确认，确认后才写入后端持久化文件；
//   - 关闭：直接写入后端持久化，不确认。
// 后端成功写入后即以服务端返回值为准回写本地状态，避免界面与磁盘不一致。
const handleProxyLocalToggle = async (newVal: boolean) => {
  if (newVal) {
    const confirmed = await globalStore.showConfirm({
      title: t('common.warning'),
      message: t('config.tproxy_proxy_local_warning'),
      type: 'warning'
    })
    if (!confirmed) return // 取消则保持原值
  }
  await persistProxyLocal(newVal)
}

// TProxy 开启时该开关被锁定（切换会重建正被 TProxy 持有的 nft 规则）。
// 此时点击不改变值，只说明原因——与「绕过设置齿轮」同一套交互。
const onProxyLocalBlocked = () => {
  globalStore.showToast(t('config.tproxy_proxy_local_readonly_warning'), 'warning')
}

// persistProxyLocal 写入本机流量接管开关并同步本地状态
const persistProxyLocal = async (enabled: boolean) => {
  try {
    const resp = await apiFetch('/config/tproxy/proxy-local', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled })
    })
    if (!resp.ok) {
      globalStore.showToast(t('common.operation_failed'), 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    await fetchTproxyProxyLocal() // 以服务端状态为准
  }
}

// 处理接管 IPv6 开关。语义与「接管本机流量」一致：
//   - 开启：先二次确认（额外说明「节点无 IPv6 出口会导致原本直连可达的 IPv6 目标失败」），
//     确认后才写入后端持久化文件；
//   - 关闭：直接写入后端持久化，不确认。
// 后端写入成功后以服务端返回值为准回写本地状态。
const handleTproxyIPv6Toggle = async (newVal: boolean) => {
  if (newVal) {
    const confirmed = await globalStore.showConfirm({
      title: t('common.warning'),
      message: t('config.tproxy_ipv6_warning'),
      type: 'warning'
    })
    if (!confirmed) return // 取消则保持原值
  }
  await persistTproxyIPv6(newVal)
}

// TProxy 开启时该开关被锁定（切换会重建正被 TProxy 持有的 nft 规则）。
// 此时点击不改变值，只说明原因——与「绕过设置齿轮」同一套交互。
const onTproxyIPv6Blocked = () => {
  // 两种置灰原因：TProxy 正在接管（需先关开关）、订阅中心关闭了 IPv6 支持。
  const message = configStore.tproxyEnabled
    ? t('config.tproxy_ipv6_readonly_warning')
    : t('config.tproxy_ipv6_global_disabled_hint')
  globalStore.showToast(message, 'warning')
}

// persistTproxyIPv6 写入 IPv6 接管开关并同步本地状态
const persistTproxyIPv6 = async (enabled: boolean) => {
  try {
    const resp = await apiFetch('/config/tproxy/proxy-ipv6', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled })
    })
    if (!resp.ok) {
      // 后端在「TProxy 已启用且重新应用规则失败」时会带上原因，需如实透出
      let msg = t('common.operation_failed')
      try {
        const data = await resp.json()
        if (data.message) msg = data.message
      } catch (_) {}
      globalStore.showToast(msg, 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    await fetchTproxyIPv6() // 以服务端状态为准
  }
}

// 处理 TUN 开关切换（开启时弹窗确认）
//
// 开关由后端组装**完整**的 tun 对象再 PATCH 内核（stack/dns-mode/IPv6 都来自 tun.json），
// 前端只发 {enable}。成功后静默回读内核状态——失败时后端的错误已弹过 toast，
// 界面不能被本地乐观值留在「看起来开了」的状态。
const handleTunToggle = async (newVal: boolean) => {
  if (newVal) {
    const confirmed = await globalStore.showConfirm({
      title: t('common.warning'),
      message: t('config.tun_enable_warning'),
      type: 'warning'
    })
    if (!confirmed) return
    // 互斥：开启 TUN 时先关 TProxy（后端也会校验，这里只是让顺序确定）
    if (configStore.tproxyEnabled) {
      await disableTproxyOnBackend()
    }
  }

  const ok = await setTunEnabled(newVal)
  await fetchConfigs(false, true)
  if (ok && newVal) {
    globalStore.showToast(t('config.tun_enabled_toast'), 'success')
  }
}

const changeLang = () => {
  localStorage.setItem('lang', locale.value)
}

onMounted(async () => {
  fetchInterfaces()
  fetchTproxyProxyLocal()
  fetchTproxyIPv6()
  fetchTunSettings()
})

// 本页的 configs 来自启动时的那一次 GET /configs（onMounted 只拉网卡与两个 TProxy 开关），
// 而 /configs 会被**别的页面**改掉：订阅中心「保存并应用」会重写 config.yaml 的
// mixed-port / tproxy-port / secret / external-controller 并重载内核，代理页切 mode 也改内核配置。
// 此前切回本页看到的仍是旧值（实测：订阅中心改 tproxy 端口后本页那一栏不刷新，而 nft 规则
// 其实已按新端口重建）。这里消费别处留下的待办，静默补拉一次——silent 模式不触发整卡遮罩，
// 用户看不到闪烁；没有待办时不发任何请求。
onActivated(async () => {
  if (!configStore.consumeCoreConfigStale()) return
  // 内核的常规配置与 TProxy 开关状态一起对齐：重载后若规则重建失败，后端会把开关
  // 回滚为关闭（ReapplyTproxyRules 的语义），只刷新 configs 会让开关显示不实。
  await Promise.all([
    fetchConfigs(true, true),
    configStore.refreshTproxyState(),
    // IPv6 总开关可能在订阅中心改过：TUN/TProxy 两个控件的置灰状态依赖它
    fetchTproxyIPv6(),
    fetchTunSettings(),
  ])
})

</script>

<template>
  <div class="flex flex-col flex-1 min-h-0 gap-4 h-full">
    <!-- 顶部操作栏 -->
    <div class="glass-medium shadow-none px-6 py-3 md:py-0 rounded-xl border border-slate-200/50 dark:border-slate-800/50 flex flex-wrap gap-4 items-center justify-between transition-all shrink-0 h-auto min-h-[56px] md:h-[56px]">
      <h3 class="text-base font-semibold flex items-center gap-2">
        <SettingsOutline class="w-5 h-5 text-accent" />
        {{ t('nav.config') }}
      </h3>

      <!-- 文档 + 关于 按钮组（紧挨） -->
        <div class="flex items-center gap-1">
          <a href="https://ttq.fjb.dpdns.org" target="_blank" rel="noopener noreferrer"
            class="flex items-center justify-center text-xs font-semibold rounded-xl bg-slate-50 hover:bg-slate-100 dark:bg-slate-800/40 dark:hover:bg-slate-800/80 transition-all text-slate-600 dark:text-slate-300 hover:scale-105 active:scale-95 border border-slate-200/50 dark:border-slate-800/30 px-3 py-1.5 gap-1.5 group"
            :title="t('nav.docs')">
            <DocumentTextOutline class="w-4 h-4 shrink-0 transition-transform duration-300 group-hover:scale-110" />
            <span>{{ t('nav.docs') }}</span>
          </a>
    
          <button @click="globalStore.showAbout = true"
            class="flex items-center justify-center text-xs font-semibold rounded-xl bg-slate-50 hover:bg-slate-100 dark:bg-slate-800/40 dark:hover:bg-slate-800/80 transition-all text-slate-600 dark:text-slate-300 hover:scale-105 active:scale-95 border border-slate-200/50 dark:border-slate-800/30 px-3 py-1.5 gap-1.5 group"
            :title="t('about.title')">
            <InformationCircleOutline class="w-4 h-4 shrink-0 transition-transform duration-300 group-hover:rotate-12" />
            <span>{{ t('about.title') }}</span>
          </button>
        </div>
    </div>

    <!-- 核心状态加载中的优雅 Loading 占位 -->
    <div v-if="coreStatus.loading" class="flex-1 flex flex-col items-center justify-center gap-3 select-none">
      <div class="w-7 h-7 border-2 border-slate-200 dark:border-slate-800 !border-t-accent rounded-full animate-spin"></div>
      <span class="text-xs font-bold text-slate-400 dark:text-slate-500 tracking-wider">{{ t('config.loading_system_params') }}</span>
    </div>

    <!-- 加载完成后的内滚动内容区 (已升级为统一大内容卡片) -->
    <div v-else class="flex-1 min-h-0 overflow-y-auto glass-medium shadow-none rounded-xl border border-slate-200/50 dark:border-slate-800/50 p-6">
      <div class="grid grid-cols-1 gap-6 items-start w-full animate-[fadeIn_0.25s_ease-out]"
        :class="[
          coreStatus.running 
            ? 'md:grid-cols-2 lg:grid-cols-3' 
            : 'md:grid-cols-2 max-w-4xl mx-auto'
        ]">
        <!-- 1. 配置参数面板区（常规参数，内核启动时显示） -->
        <div v-if="coreStatus.running"
          class="live-card bg-slate-50/50 dark:bg-slate-900/30 p-6 rounded-xl border border-slate-200/40 dark:border-slate-800/40 hover:border-slate-300/80 dark:hover:border-slate-700/80 hover:-translate-y-[3px] hover:shadow-md hover:bg-slate-100/80 dark:hover:bg-slate-900/80 duration-300 space-y-5 h-full transition-all flex flex-col relative">
          <!-- 同步配置遮罩屏 -->
          <div v-if="configsLoading"
            class="absolute inset-0 glass-light z-30 flex flex-col items-center justify-center rounded-2xl gap-2 select-none border shadow-sm transition-all duration-300">
            <div class="w-5 h-5 border-2 border-slate-200 dark:border-slate-700 !border-t-accent rounded-full animate-spin"></div>
            <span class="text-xs font-bold text-slate-500 dark:text-slate-400 tracking-wider">{{ t('config.syncing_configs') }}</span>
          </div>

          <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
            <OptionsOutline class="w-4 h-4 text-accent" />
            {{ t('config.general_settings') }}
          </h4>

          <div class="flex items-center justify-between">
            <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.allow_lan') }}</label>
            <FormSwitch v-model="configs['allow-lan']" @update:model-value="toggleAllowLan" />
          </div>

          <div class="flex flex-col gap-1.5">
            <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.mode') }}</label>
            <select v-model="configs.mode" @change="changeMode"
              class="px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full">
              <option value="Rule">{{ t('config.mode_rule') }}</option>
              <option value="Global">{{ t('config.mode_global') }}</option>
              <option value="Direct">{{ t('config.mode_direct') }}</option>
            </select>
          </div>

          <div class="flex flex-col gap-1.5">
            <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.log_level') }}</label>
            <select v-model="configs['log-level']" @change="changeLogLevel"
              class="px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full">
              <option value="silent">Silent</option>
              <option value="info">Info</option>
              <option value="warning">Warning</option>
              <option value="error">Error</option>
              <option value="debug">Debug</option>
            </select>
          </div>
        </div>

        <!-- 2. 端口设置（内核启动时显示） -->
        <div v-if="coreStatus.running"
          class="live-card bg-slate-50/50 dark:bg-slate-900/30 p-6 rounded-xl border border-slate-200/40 dark:border-slate-800/40 hover:border-slate-300/80 dark:hover:border-slate-700/80 hover:-translate-y-[3px] hover:shadow-md hover:bg-slate-100/80 dark:hover:bg-slate-900/80 duration-300 space-y-5 h-full transition-all flex flex-col relative">
          <!-- 同步配置遮罩屏 -->
          <div v-if="configsLoading"
            class="absolute inset-0 glass-light z-30 flex flex-col items-center justify-center rounded-2xl gap-2 select-none border shadow-sm transition-all duration-300">
            <div class="w-5 h-5 border-2 border-slate-200 dark:border-slate-700 !border-t-accent rounded-full animate-spin"></div>
            <span class="text-xs font-bold text-slate-500 dark:text-slate-400 tracking-wider">{{ t('config.syncing_configs') }}</span>
          </div>

          <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
            <HardwareChipOutline class="w-4 h-4 text-accent" />
            {{ t('config.port_settings') }}
          </h4>

          <div class="grid grid-cols-2 gap-4">
            <div class="flex flex-col gap-1">
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.mixed_port') }}</label>
              <input type="number" v-model.number="configs['mixed-port']" min="0" max="65535" step="1" @blur="savePorts" @keyup.enter="savePorts" :placeholder="t('config.port_disabled_hint')"
                class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full" />
            </div>
            <div class="flex flex-col gap-1">
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.http_port') }}</label>
              <input type="number" v-model.number="configs.port" min="0" max="65535" step="1" @blur="savePorts" @keyup.enter="savePorts" :placeholder="t('config.port_disabled_hint')"
                class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full" />
            </div>
            <div class="flex flex-col gap-1">
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.socks_port') }}</label>
              <input type="number" v-model.number="configs['socks-port']" min="0" max="65535" step="1" @blur="savePorts" @keyup.enter="savePorts" :placeholder="t('config.port_disabled_hint')"
                class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full" />
            </div>
            <div class="flex flex-col gap-1">
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.redir_port') }}</label>
              <input type="number" v-model.number="configs['redir-port']" min="0" max="65535" step="1" @blur="savePorts" @keyup.enter="savePorts" :placeholder="t('config.port_disabled_hint')"
                class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full" />
            </div>
            <div class="flex flex-col gap-1 col-span-2">
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.tproxy_port') }}</label>
              <!-- 这里**不能**用 :readonly 来「保护」端口：移动端只读输入框点击后不会
                   拉起虚拟键盘（它不是 disabled，所以点击事件照常触发），用户看到一句
                   提示却永远改不成端口——真机上的表现就是「点了没反应、键盘不弹」。
                   后端已支持「启用中改端口」：先落库、再改内核，内核拒绝就自动回滚，
                   成功后才按新端口重建规则（失败还会关掉开关并如实报错）。
                   改动的结果（成功/失败）由保存后的 toast 反馈，不再用内联文案占位。 -->
              <input type="number" v-model.number="configs['tproxy-port']"
                min="0" max="65535" step="1"
                @focus="onTproxyPortFocus" @blur="savePorts" @keyup.enter="savePorts"
                :placeholder="t('config.port_disabled_hint')"
                class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full" />
            </div>
          </div>
        </div>

        <!-- 3. TUN与网卡设置（内核启动时显示） -->
        <div v-if="coreStatus.running"
          class="live-card bg-slate-50/50 dark:bg-slate-900/30 p-6 rounded-xl border border-slate-200/40 dark:border-slate-800/40 hover:border-slate-300/80 dark:hover:border-slate-700/80 hover:-translate-y-[3px] hover:shadow-md hover:bg-slate-100/80 dark:hover:bg-slate-900/80 duration-300 space-y-5 h-full transition-all flex flex-col relative">
          <!-- 同步配置遮罩屏 -->
          <div v-if="configsLoading"
            class="absolute inset-0 glass-light z-30 flex flex-col items-center justify-center rounded-2xl gap-2 select-none border shadow-sm transition-all duration-300">
            <div class="w-5 h-5 border-2 border-slate-200 dark:border-slate-700 !border-t-accent rounded-full animate-spin"></div>
            <span class="text-xs font-bold text-slate-500 dark:text-slate-400 tracking-wider">{{ t('config.syncing_configs') }}</span>
          </div>

          <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
            <ShieldCheckmarkOutline class="w-4 h-4 text-accent" />
            {{ t('config.tun_settings') }}
          </h4>

          <!-- TUN 模式。齿轮按钮与「启用 TProxy」一致：点击打开高级设置弹窗，
               弹窗内为 TUN 设备名 / 出口网卡 / DNS 处理 / IPv6 接管。 -->
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.tun_enable') }}</label>
              <button
                @click="openTunAdvancedDialog"
                class="p-1 text-slate-400 hover:text-accent rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all"
                :title="t('config.tun_advanced_title')"
              >
                <SettingsOutline class="w-4 h-4" />
              </button>
            </div>
            <FormSwitch :model-value="configs.tun.enable" @update:model-value="handleTunToggle"/>
          </div>
          <div class="flex flex-col gap-1">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('config.tun_stack') }}</label>
            <select v-model="tunSettings.stack" @change="saveTunSettings({ stack: tunSettings.stack })"
              class="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full">
              <option value="gvisor">gVisor</option>
              <option value="system">System</option>
              <option value="mixed">Mixed</option>
              <option value="mips">Mips</option>
            </select>
          </div>

          <!-- 分割线 -->
          <div class="h-px bg-slate-200 dark:bg-slate-700 my-1.5"></div>

          <!-- TProxy 透明代理 -->
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">
                {{ t('config.tproxy_enable') }}
              </label>
              <button
                @click="openTproxyExceptionsDialog"
                class="p-1 text-slate-400 hover:text-accent rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all"
                :class="configStore.tproxyEnabled ? 'opacity-40 cursor-not-allowed' : 'hover:text-accent'"
                :title="configStore.tproxyEnabled ? t('config.tproxy_exceptions_disabled_message') : t('config.tproxy_exceptions_title')"
              >
                <SettingsOutline class="w-4 h-4" />
              </button>
            </div>
            <div v-if="!configStore.tproxyStateLoaded" class="w-7 h-4 rounded-full bg-slate-200 dark:bg-slate-700 animate-pulse"></div>
            <FormSwitch v-else v-model="configStore.tproxyEnabled" @update:model-value="toggleTProxy" />
          </div>

          <!-- 接管本机流量（由弹窗移出）。
               启用 TProxy 时禁止修改：此时切换会重建 nft 规则，而 TProxy 开关
               正持有规则，故禁用——置灰原因经 disabled-hint 外露（悬停 title +
               点击 toast），避免开关无声不响应。 -->
          <div class="flex items-center justify-between">
            <label
              class="text-xs font-semibold"
              :class="configStore.tproxyEnabled ? 'text-slate-400 dark:text-slate-500' : 'text-slate-700 dark:text-slate-300'"
            >
              {{ t('config.tproxy_proxy_local_label') }}
            </label>
            <div v-if="!tproxyProxyLocalLoaded" class="w-7 h-4 rounded-full bg-slate-200 dark:bg-slate-700 animate-pulse"></div>
            <FormSwitch
              v-else
              :model-value="tproxyProxyLocal"
              :disabled="configStore.tproxyEnabled"
              :disabled-hint="t('config.tproxy_proxy_local_readonly_warning')"
              @update:model-value="handleProxyLocalToggle"
              @blocked="onProxyLocalBlocked"
            />
          </div>

        </div>

        <!-- 4. 运维控制（始终显示） -->
        <div
          class="live-card bg-slate-50/50 dark:bg-slate-900/30 p-6 rounded-xl border border-slate-200/40 dark:border-slate-800/40 hover:border-slate-300/80 dark:hover:border-slate-700/80 hover:-translate-y-[3px] hover:shadow-md hover:bg-slate-100/80 dark:hover:bg-slate-900/80 duration-300 space-y-5 h-full transition-all flex flex-col relative">
          <div class="border-b border-slate-100 dark:border-slate-800 pb-4">
            <h4 class="font-bold text-sm flex items-center gap-2">
              <BuildOutline class="w-4 h-4 text-accent" />
              {{ t('config.advanced_maintenance') }}
            </h4>
          </div>

          <div class="space-y-4 flex-1 flex flex-col justify-between">
            <!-- 内核状态 -->
            <div class="flex items-center justify-between px-3.5 py-2.5 bg-slate-50 dark:bg-slate-900/40 rounded-xl border border-slate-100 dark:border-slate-800/80">
              <span class="text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('config.core_status') }}</span>
              <div class="flex items-center gap-2.5 text-xs">
                <span class="w-2 h-2 rounded-full flex shrink-0"
                  :class="coreStatus.loading ? 'bg-slate-400 animate-pulse' : (coreStatus.running ? 'bg-success' : 'bg-red-500')"></span>
                <span class="font-bold text-slate-700 dark:text-slate-200">
                  {{ coreStatus.loading ? t('config.core_checking') : (coreStatus.running ? t('config.core_running') : t('config.core_stopped')) }}
                </span>
                <span v-if="coreStatus.running && stats.coreVersion !== CORE_VERSION_UNKNOWN && stats.coreVersion !== CORE_VERSION_LOADING"
                  class="px-1.5 py-0.5 font-mono text-[10px] bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 rounded">
                  {{ coreVersion }}
                </span>
              </div>
            </div>

            <!-- 内核核心控制 -->
            <div class="grid gap-3 w-full"
              :class="coreStatus.running ? 'grid-cols-2' : 'grid-cols-1'">
              <button v-if="!coreStatus.running" @click="handleStartCore" :disabled="coreStatus.loading"
                class="py-2 bg-success hover:bg-success-hover text-white text-xs font-semibold rounded-xl shadow-md shadow-success/15 hover:shadow-success/25 transition-all flex items-center justify-center gap-1.5 w-full">
                <SyncOutline v-if="coreStatus.loading" class="w-3.5 h-3.5 animate-spin inline-block" />
                {{ coreStatus.loading ? t('config.core_starting') : t('config.start_core') }}
              </button>
              <template v-else>
                <button @click="handleStopCore" :disabled="coreStatus.loading"
                  class="py-2 bg-red-500 hover:bg-red-600 text-white text-xs font-semibold rounded-xl shadow-md shadow-red-500/15 hover:shadow-red-500/25 transition-all flex items-center justify-center gap-1.5 w-full">
                  <SyncOutline v-if="coreStatus.loading" class="w-3.5 h-3.5 animate-spin inline-block" />
                  {{ coreStatus.loading ? t('config.core_stopping') : t('config.stop_core') }}
                </button>
                <button @click="handleRestartCore" :disabled="coreStatus.loading"
                  class="py-2 bg-amber-500 hover:bg-amber-600 text-white text-xs font-semibold rounded-xl shadow-md shadow-amber-500/15 hover:shadow-amber-500/25 transition-all flex items-center justify-center w-full">
                  {{ t('config.restart') }}
                </button>
              </template>
              <!-- 更新按钮 + 通道选择：通道用浏览器原生 select（保留系统自带下拉样式）。
                   两列等宽、等高、彼此独立，间隔正好落在这一行正中。 -->
              <div class="col-span-full grid grid-cols-2 gap-2">
                <button
                  @click="handleUpgradeCore"
                  :disabled="isUpgrading || !coreStatus.running"
                  class="w-full h-9 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-xl shadow-md shadow-accent/15 hover:shadow-accent/25 transition-all flex items-center justify-center disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {{ isUpgrading ? t('config.upgrading_core') : t('config.upgrade_core') }}
                </button>
                <select
                  v-model="selectedChannel"
                  :disabled="isUpgrading || !coreStatus.running"
                  class="w-full min-w-0 h-9 rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 px-2 text-xs font-semibold text-slate-700 dark:text-slate-200 text-center focus:ring-2 focus:ring-accent outline-none disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  <option value="release">{{ t('config.upgrade_stable') }}</option>
                  <option value="alpha">{{ t('config.upgrade_alpha') }}</option>
                </select>
              </div>
            </div>
   
            <!-- 分割线 -->
            <div class="h-px bg-slate-200 dark:bg-slate-700 my-1.5"></div>
   
            <!-- 常规运维 -->
            <div class="grid grid-cols-2 gap-3">
              <button @click="handleReloadConfig" :disabled="!coreStatus.running || isReloading"
                class="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-xl text-slate-700 dark:text-slate-200 transition-all border border-slate-200/20 disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-1.5">
                <div v-if="isReloading" class="w-3 h-3 border border-slate-300 dark:border-slate-600 !border-t-accent rounded-full animate-spin"></div>
                {{ isReloading ? t('config.reloading') : t('config.reload') }}
              </button>
              <button @click="handleFlushFakeIP" :disabled="!coreStatus.running || isFlushingFakeIP"
                class="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-xl text-slate-700 dark:text-slate-200 transition-all border border-slate-200/20 disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-1.5">
                <div v-if="isFlushingFakeIP" class="w-3 h-3 border border-slate-300 dark:border-slate-600 !border-t-accent rounded-full animate-spin"></div>
                {{ isFlushingFakeIP ? t('config.flushing') : t('config.flush_fakeip') }}
              </button>
              <button @click="handleFlushDNS" :disabled="!coreStatus.running || isFlushingDNS"
                class="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-xl text-slate-700 dark:text-slate-200 transition-all border border-slate-200/20 disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-1.5">
                <div v-if="isFlushingDNS" class="w-3 h-3 border border-slate-300 dark:border-slate-600 !border-t-accent rounded-full animate-spin"></div>
                {{ isFlushingDNS ? t('config.flushing') : t('config.flush_dns') }}
              </button>
              <button @click="handleUpdateGeo" :disabled="!coreStatus.running || isUpdatingGeo"
                class="px-4 py-2.5 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-xs font-semibold rounded-xl text-slate-700 dark:text-slate-200 transition-all border border-slate-200/20 disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-1.5">
                <div v-if="isUpdatingGeo" class="w-3 h-3 border border-slate-300 dark:border-slate-600 !border-t-accent rounded-full animate-spin"></div>
                {{ isUpdatingGeo ? t('config.upgrading_core') : t('config.update_geo') }}
              </button>
            </div>
          </div>
        </div>

        <!-- 5. 界面设置（始终显示） -->
        <div
          class="live-card bg-slate-50/50 dark:bg-slate-900/30 p-6 rounded-xl border border-slate-200/40 dark:border-slate-800/40 hover:border-slate-300/80 dark:hover:border-slate-700/80 hover:-translate-y-[3px] hover:shadow-md hover:bg-slate-100/80 dark:hover:bg-slate-900/80 duration-300 space-y-5 h-full transition-all flex flex-col relative">
          <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
            <ColorPaletteOutline class="w-4 h-4 text-accent" />
            {{ t('config.interface_settings') }}
          </h4>

          <div class="space-y-4 flex-1">
            <div class="flex flex-col gap-1.5">
              <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.language') }}</label>
              <select v-model="locale" @change="changeLang"
                class="px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full">
                <option value="zh">{{ t('config.lang_zh') }}</option>
                <option value="en">{{ t('config.lang_en') }}</option>
              </select>
            </div>

            <div class="flex flex-col gap-1.5">
              <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.theme') }}</label>
              <select v-model="globalStore.theme"
                class="px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full">
                <option value="light">{{ t('config.theme_light') }}</option>
                <option value="dark">{{ t('config.theme_dark') }}</option>
                <option value="purple">{{ t('config.theme_purple') }}</option>
                <option value="pink">{{ t('config.theme_pink') }}</option>
                <option value="green">{{ t('config.theme_green') }}</option>
                <option value="blue">{{ t('config.theme_blue') }}</option>
                <option value="system">{{ t('config.theme_system') }}</option>
              </select>
            </div>

            <div class="flex flex-col gap-1.5">
              <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">{{ t('config.start_page') }}</label>
              <select v-model="globalStore.startPage"
                class="px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none w-full">
                <option value="last">{{ t('config.start_page_last') }}</option>
                <option value="overview">{{ t('nav.overview') }}</option>
                <option value="proxies">{{ t('nav.proxies') }}</option>
                <option value="subscription">{{ t('nav.subscription') }}</option>
                <option value="rules">{{ t('nav.rules') }}</option>
                <option value="connections">{{ t('nav.connections') }}</option>
                <option value="logs">{{ t('nav.logs') }}</option>
                <option value="config">{{ t('nav.config') }}</option>
              </select>
            </div>
          </div>
        </div>

        <!-- 6. DNS 查询（内核启动时显示） -->
        <div v-if="coreStatus.running"
          class="live-card bg-slate-50/50 dark:bg-slate-900/30 p-6 rounded-xl border border-slate-200/40 dark:border-slate-800/40 hover:border-slate-300/80 dark:hover:border-slate-700/80 hover:-translate-y-[3px] hover:shadow-md hover:bg-slate-100/80 dark:hover:bg-slate-900/80 duration-300 space-y-4 h-full transition-all flex flex-col relative">
          <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
            <SearchOutline class="w-4 h-4 text-accent" />
            {{ t('config.dns_query') }}
          </h4>

          <div class="flex-1 flex flex-col justify-between gap-4">
            <div class="flex flex-col gap-2">
              <input type="text" v-model="dnsQuery.name" :placeholder="t('config.dns_placeholder')"
                @keyup.enter="handleDNSQuery"
                class="w-full px-4 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none" />
              <div class="flex gap-2">
                <select v-model="dnsQuery.type"
                  class="flex-1 px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none">
                  <option value="A">A</option>
                  <option value="AAAA">AAAA</option>
                  <option value="MX">MX</option>
                  <option value="TXT">TXT</option>
                </select>
                <button @click="handleDNSQuery" :disabled="dnsQuery.loading"
                  class="flex-[2] py-2 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all flex items-center justify-center gap-1">
                  {{ dnsQuery.loading ? t('config.dns_querying') : t('config.dns_query_btn') }}
                </button>
              </div>
            </div>

            <!-- 结果容器：高度有上下限（下限保证空态/少行时不下塌，上限保证行数再多也不撑大卡片），
                 超出上限的内容在容器内纵向滚动 -->
            <pre
              class="p-4 bg-slate-50 dark:bg-slate-900/50 font-mono text-xs rounded-xl overflow-y-auto whitespace-pre-wrap break-all flex-1 min-h-28 max-h-56 border border-slate-200 dark:border-slate-800 transition-all"
              :class="dnsQuery.result ? 'text-emerald-700 dark:text-emerald-400' : 'text-slate-400 dark:text-slate-500 italic flex items-center justify-center select-none'">{{ dnsQuery.result || t('config.dns_result_default') }}</pre>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- ====== TProxy 绕过列表弹窗 ====== -->
  <Teleport to="body">
    <div v-if="isActive && showTproxyExceptionsDialog" class="fixed inset-0 glass-mask z-[9999] flex items-center justify-center p-4" @click.self="showTproxyExceptionsDialog = false">
      <!-- 限制弹窗最大高度为视口 90%，flex 列布局 -->
      <div class="glass-heavy w-full max-w-lg max-h-[90vh] rounded-[20px] shadow-2xl border p-6 flex flex-col gap-4 animate-[zoomIn_0.15s_ease-out]">
        <!-- 标题 + 恢复默认，固定不滚动 -->
        <div class="flex items-center justify-between gap-3 flex-shrink-0">
          <h4 class="text-lg font-bold">{{ t('config.tproxy_exceptions_title') }}</h4>
          <button
            @click="restoreTproxyDefaults"
            class="px-2.5 py-1 text-xs font-semibold rounded-lg text-accent hover:bg-accent/10 border border-accent/30 transition-all shrink-0"
            :title="t('config.tproxy_restore_defaults_hint')"
          >
            {{ t('config.tproxy_restore_defaults') }}
          </button>
        </div>

        <!-- 可滚动内容区域：占据剩余空间，超出时滚动。
             pr-2 给滚动条留位：滚动条是覆盖式绘制，不留白会压住输入框右边缘。 -->
        <div class="flex-1 min-h-0 overflow-y-auto pr-2">
          <div class="space-y-4">
            <!-- IPv6 接管（置于弹窗顶部：它决定 ip6 家族规则是否随 TProxy 下发）。
                 开关即时生效（与下方文本的「保存」无关）；TProxy 启用期间或订阅中心
                 关闭「IPv6 支持」时置灰，偏好保留不改写。 -->
            <div class="pb-3 border-b border-slate-100 dark:border-slate-800/60">
              <div class="flex items-center justify-between gap-3">
                <label
                  class="text-xs font-semibold cursor-help"
                  :class="configStore.tproxyEnabled || !globalIPv6 ? 'text-slate-400 dark:text-slate-500' : 'text-slate-700 dark:text-slate-300'"
                  :title="t('config.tproxy_ipv6_hint')"
                >
                  {{ t('config.tproxy_ipv6_label') }}
                </label>
                <div v-if="!tproxyIPv6Loaded" class="w-7 h-4 rounded-full bg-slate-200 dark:bg-slate-700 animate-pulse"></div>
                <FormSwitch
                  v-else
                  :model-value="tproxyIPv6"
                  :disabled="configStore.tproxyEnabled || !globalIPv6"
                  :disabled-hint="configStore.tproxyEnabled ? t('config.tproxy_ipv6_readonly_warning') : t('config.tproxy_ipv6_global_disabled_hint')"
                  @update:model-value="handleTproxyIPv6Toggle"
                  @blocked="onTproxyIPv6Blocked"
                />
              </div>
              <p class="text-[11px] text-slate-400 dark:text-slate-500 mt-1 leading-relaxed">
                {{ globalIPv6 ? t('config.tproxy_ipv6_hint') : t('config.tproxy_ipv6_global_disabled_hint') }}
              </p>
            </div>

            <!-- 目的绕过 -->
            <div>
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">
                {{ t('config.tproxy_dst_exceptions_label') }}
              </label>
              <p class="text-[11px] text-slate-400 dark:text-slate-500 mt-0.5">{{ t('config.tproxy_dst_exceptions_hint') }}</p>
              <textarea
                v-model="tproxyDstExceptionsText"
                rows="6"
                class="w-full p-3 text-sm font-mono rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none resize-y min-h-[100px]"
                :placeholder="t('config.tproxy_dst_exceptions_placeholder')"
              ></textarea>
            </div>

            <!-- 源绕过 -->
            <div class="pt-2 border-t border-slate-100 dark:border-slate-800/60">
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">
                {{ t('config.tproxy_src_exceptions_label') }}
              </label>
              <p class="text-[11px] text-slate-400 dark:text-slate-500 mt-0.5">{{ t('config.tproxy_src_exceptions_hint') }}</p>
              <textarea
                v-model="tproxySrcExceptionsText"
                rows="6"
                class="w-full p-3 text-sm font-mono rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none resize-y min-h-[100px]"
                :placeholder="t('config.tproxy_src_exceptions_placeholder')"
              ></textarea>
            </div>

          </div>
        </div>

        <!-- 底部按钮，固定不滚动 -->
        <div class="flex justify-end gap-2.5 pt-3 border-t border-slate-100 dark:border-slate-800/60 flex-shrink-0">
          <button @click="showTproxyExceptionsDialog = false" class="px-4 py-2 text-sm font-semibold rounded-xl bg-white border border-slate-200 hover:bg-slate-50 dark:bg-slate-800 dark:border-slate-700 dark:hover:bg-slate-700/60 text-slate-600 dark:text-slate-300 transition-all">
            {{ t('common.cancel') }}
          </button>
          <button @click="saveTproxyExceptions" class="px-4 py-2 text-sm font-semibold rounded-xl bg-accent hover:bg-accent-hover text-white transition-all shadow-md shadow-accent/15">
            {{ t('common.save') }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>

  <!-- ====== TUN 高级设置弹窗（实时修改，仅关闭） ====== -->
  <Teleport to="body">
    <div v-if="isActive && showTunAdvancedDialog" class="fixed inset-0 glass-mask z-[9999] flex items-center justify-center p-4" @click.self="showTunAdvancedDialog = false">
      <div class="glass-heavy w-full max-w-lg max-h-[90vh] rounded-[20px] shadow-2xl border p-6 flex flex-col gap-4 animate-[zoomIn_0.15s_ease-out]">
        <div class="flex-shrink-0">
          <h4 class="text-lg font-bold">{{ t('config.tun_advanced_title') }}</h4>
        </div>

        <!-- pr-2 给滚动条留位：覆盖式滚动条会压住输入框右边缘。 -->
        <div class="flex-1 min-h-0 overflow-y-auto pr-2">
          <div class="space-y-4">
            <!-- TUN 设备名（失焦/回车写回 tun.json，下次开启 TUN 生效） -->
            <div class="flex flex-col gap-1">
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">
                {{ t('config.tun_device') }}
              </label>
              <input
                type="text"
                v-model="tunSettings.device"
                @blur="saveTunSettings({ device: tunSettings.device })"
                @keyup.enter="saveTunSettings({ device: tunSettings.device })"
                :placeholder="t('config.interface_name_placeholder')"
                class="px-3 py-2 text-sm rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none w-full"
              />
            </div>

            <!-- 出口网卡（选择即写回；这是内核顶层字段，与 TUN 参数不同，仍是运行时 PATCH） -->
            <div class="flex flex-col gap-1 pt-2 border-t border-slate-100 dark:border-slate-800/60">
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">
                {{ t('config.interface_name') }}
              </label>
              <select
                v-model="configs['interface-name']"
                @change="saveInterface"
                class="px-3 py-2 text-sm rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none w-full"
              >
                <option value="">{{ t('config.interface_name_auto') }}</option>
                <option v-for="iface in interfaces" :key="iface" :value="iface">{{ iface }}</option>
              </select>
              <p class="text-[11px] text-slate-400 dark:text-slate-500 mt-0.5">
                {{ t('config.tun_advanced_interface_hint') }}
              </p>
            </div>

            <!-- DNS 处理方式（tun.dns-mode） -->
            <div class="flex flex-col gap-1 pt-2 border-t border-slate-100 dark:border-slate-800/60">
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">
                {{ t('config.tun_dns_mode') }}
              </label>
              <select
                v-model="tunSettings.dns_mode"
                @change="saveTunSettings({ dns_mode: tunSettings.dns_mode })"
                class="px-3 py-2 text-sm rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none w-full"
              >
                <option value="hijack">{{ t('config.tun_dns_mode_hijack') }}</option>
                <option value="native">{{ t('config.tun_dns_mode_native') }}</option>
                <option value="disabled">{{ t('config.tun_dns_mode_disabled') }}</option>
              </select>
              <p class="text-[11px] text-slate-400 dark:text-slate-500 mt-0.5">
                {{ t('config.tun_dns_mode_hint') }}
              </p>
            </div>

            <!-- IPv6 接管（tun.inet6-address） -->
            <div class="flex flex-col gap-1 pt-2 border-t border-slate-100 dark:border-slate-800/60">
              <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">
                {{ t('config.tun_ipv6_mode') }}
              </label>
              <select
                v-model="tunSettings.ipv6"
                @change="saveTunSettings({ ipv6: tunSettings.ipv6 })"
                :disabled="!globalIPv6"
                class="px-3 py-2 text-sm rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none w-full disabled:opacity-60 disabled:cursor-not-allowed"
              >
                <option value="auto">{{ t('config.tun_ipv6_auto') }}</option>
                <option value="off">{{ t('config.tun_ipv6_off') }}</option>
              </select>
              <p class="text-[11px] text-slate-400 dark:text-slate-500 mt-0.5">
                {{ !globalIPv6 ? t('config.tun_ipv6_global_disabled') : (tunHostIPv6 ? t('config.tun_ipv6_host_ok') : t('config.tun_ipv6_host_disabled')) }}
              </p>
              <p class="text-[11px] text-slate-400 dark:text-slate-500">
                {{ t('config.tun_ipv6_hint') }}
              </p>
            </div>
          </div>
        </div>

        <!-- 仅关闭按钮：参数只在下次开启 TUN 时生效，无需"保存" -->
        <div class="flex justify-end pt-3 border-t border-slate-100 dark:border-slate-800/60 flex-shrink-0">
          <button @click="showTunAdvancedDialog = false" class="px-4 py-2 text-sm font-semibold rounded-xl bg-accent hover:bg-accent-hover text-white transition-all shadow-md shadow-accent/15">
            {{ t('common.close') }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
