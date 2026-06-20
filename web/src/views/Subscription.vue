<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiFetch } from '../utils/api'
import { MailOutline, EyeOutline, EyeOffOutline, SyncOutline, CreateOutline, TrashOutline, AddOutline, CloseOutline } from '@vicons/ionicons5'
import { useGlobalStore } from '../store/global'
import { storeToRefs } from 'pinia'
import { useConfigStore, type SubscriptionInfo, type SubscriptionItem } from '../store/config'

const globalStore = useGlobalStore()

const { t } = useI18n()


const showSecret = ref(false)
const showModal = ref(false)
const showUrls = ref<Record<number, boolean>>({})
const modalTitle = ref('')
const isUpdating = ref<Record<number, boolean>>({})
const isApplying = ref(false)

// 弹窗编辑项
const editingIndex = ref(-1)
const editForm = ref<SubscriptionItem>({
  name: '',
  url: '',
  update_interval: 86400,
  health_interval: 300,
  prefix: ''
})

const configStore = useConfigStore()
const { currentConfig, savedSubNames, coreStatus } = storeToRefs(configStore)

const loadConfig = configStore.loadConfig
const fetchSubscriptionInfo = configStore.fetchSubscriptionInfo
const enrichSubscriptions = configStore.enrichSubscriptions

const getHealthClass = (info?: SubscriptionInfo | null) => {
  if (!info || info.aliveCount === 0) return 'text-red-500'
  if (info.avgDelay === undefined || info.avgDelay === 0) return 'text-slate-400'
  if (info.avgDelay <= 150) return 'text-success'
  if (info.avgDelay <= 300) return 'text-amber-500'
  return 'text-red-400'
}

// 手动更新单个订阅
const handleUpdateSub = async (index: number) => {
  if (!coreStatus.value.running) {
    globalStore.showToast(t('config.core_stopped') + '，' + t('common.operation_failed'), 'warning')
    return
  }
  const sub = currentConfig.value.subscriptions[index]
  if (!sub) return
  if (isUpdating.value[index]) return
  isUpdating.value[index] = true
  globalStore.showToast(t('rules.updating'), 'info')
  try {
    const encoded = encodeURIComponent(sub.name)
    const resp = await apiFetch(`/providers/proxies/${encoded}`, { method: 'PUT' })
    if (resp.ok) {
      globalStore.showToast(t('subscription.update_success', { name: sub.name }), 'success')
      const info = await fetchSubscriptionInfo(sub.name)
      currentConfig.value.subscriptions[index].info = info
    } else {
      globalStore.showToast(`${t('common.operation_failed')}: ${resp.status}`, 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    isUpdating.value[index] = false
  }
}

const isCheckingHealth = ref<Record<number, boolean>>({})

// 触发单个订阅的健康检查（测速）
const handleHealthCheckSub = async (index: number) => {
  if (!coreStatus.value.running) {
    globalStore.showToast(t('config.core_stopped') + '，' + t('common.operation_failed'), 'warning')
    return
  }
  const sub = currentConfig.value.subscriptions[index]
  if (!sub) return
  if (isCheckingHealth.value[index]) return
  isCheckingHealth.value[index] = true
  globalStore.showToast(t('subscription.health_check') + '...', 'info')
  try {
    const encoded = encodeURIComponent(sub.name)
    const resp = await apiFetch(`/providers/proxies/${encoded}/healthcheck`)
    if (resp.ok) {
      globalStore.showToast(t('subscription.health_check_complete', { name: sub.name }), 'success')
      const info = await fetchSubscriptionInfo(sub.name)
      currentConfig.value.subscriptions[index].info = info
    } else {
      globalStore.showToast(`${t('common.operation_failed')}: ${resp.status}`, 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    isCheckingHealth.value[index] = false
  }
}

// 打开模态框
const openSubModal = (index: number = -1) => {
  editingIndex.value = index
  if (index >= 0) {
    modalTitle.value = t('subscription.edit_modal_title')
    const sub = currentConfig.value.subscriptions[index]
    editForm.value = {
      name: sub.name || '',
      url: sub.url || '',
      update_interval: sub.update_interval || 86400,
      health_interval: sub.health_interval || 300,
      prefix: sub.prefix || ''
    }
  } else {
    modalTitle.value = t('subscription.add_modal_title')
    editForm.value = {
      name: '',
      url: '',
      update_interval: 86400,
      health_interval: 300,
      prefix: ''
    }
  }
  showModal.value = true
}

const closeSubModal = () => {
  showModal.value = false
}

// 保存至订阅列表
const saveSubToList = () => {
  const { name, url } = editForm.value
  if (!name.trim() || !url.trim()) {
    globalStore.showToast(t('common.name_required'), 'error')
    return
  }
  const isDuplicate = (currentConfig.value.subscriptions || []).some((sub, idx) => {
    return sub.name.trim() === name.trim() && idx !== editingIndex.value
  })
  if (isDuplicate) {
    globalStore.showToast(t('subscription.duplicate_name'), 'error')
    return
  }
  const subData = { ...editForm.value }
  if (editingIndex.value >= 0) {
    currentConfig.value.subscriptions[editingIndex.value] = subData
  } else {
    if (!currentConfig.value.subscriptions) {
      currentConfig.value.subscriptions = []
    }
    currentConfig.value.subscriptions.push(subData)
  }
  showModal.value = false
  enrichSubscriptions()
}

// 删除订阅
const handleDeleteSub = async (index: number) => {
  const ok = await globalStore.showConfirm({
    message: `${t('common.confirm')} ${t('common.delete')}?`
  })
  if (ok) {
    currentConfig.value.subscriptions.splice(index, 1)
  }
}

// 保存并应用
const saveAndApply = async () => {
  if (!currentConfig.value.proxy_port || !currentConfig.value.panel_port) {
    globalStore.showToast(t('subscription.proxy_port') + ' / ' + t('subscription.panel_port') + ' ' + t('common.required'), 'error')
    return
  }
  if (!currentConfig.value.rule_group || currentConfig.value.rule_group === 'none') {
    globalStore.showToast(t('subscription.rule_group') + ' ' + t('common.required'), 'error')
    return
  }
  isApplying.value = true
  try {
    const resp = await apiFetch('/subscribe/generate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(currentConfig.value)
    })
    const result = await resp.json()
    if (resp.ok && result.status === 'ok') {
      globalStore.showToast(result.message || t('subscription.apply_success'), 'success')
      loadConfig()
    } else {
      globalStore.showToast(`${t('subscription.operation_failed')}: ${result.message || ''}`, 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    isApplying.value = false
  }
}

// 辅助格式化
const formatGB = (bytes: number) => {
  if (!bytes) return '0.0 GB'
  return (bytes / (1024 * 1024 * 1024)).toFixed(1) + ' GB'
}

const formatUpdateTime = (dateStr: string | null) => {
  if (!dateStr) return null
  const date = new Date(dateStr)
  if (isNaN(date.getTime())) return null
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
}

const formatExpire = (expire: number) => {
  if (!expire) return t('subscription.expire_forever')
  return new Date(expire * 1000).toLocaleString()
}

onMounted(() => {
  configStore.fetchCoreStatus()
  loadConfig()
})
</script>

<template>
  <div class="space-y-6">
    <div class="bg-white dark:bg-[#1e293b] p-6 rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm hover:shadow-md hover:-translate-y-[1px] duration-300 transition-all">
      <h3 class="text-lg font-semibold mb-6 flex items-center gap-2">
        <MailOutline class="w-5 h-5 text-accent" />
        {{ t('subscription.title') }}
      </h3>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div class="flex flex-col gap-2">
          <label class="text-sm font-medium text-slate-600 dark:text-slate-400">{{ t('subscription.proxy_port') }}</label>
          <input type="number" v-model="currentConfig.proxy_port" class="px-4 py-2.5 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none" />
        </div>
        <div class="flex flex-col gap-2">
          <label class="text-sm font-medium text-slate-600 dark:text-slate-400">{{ t('subscription.panel_port') }}</label>
          <input type="number" v-model="currentConfig.panel_port" class="px-4 py-2.5 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none" />
        </div>
        <div class="flex flex-col gap-2">
          <label class="text-sm font-medium text-slate-600 dark:text-slate-400">{{ t('subscription.panel_secret') }}</label>
          <div class="relative flex items-center">
            <input :type="showSecret ? 'text' : 'password'" v-model="currentConfig.panel_secret" class="w-full pl-4 pr-10 py-2.5 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none" />
            <button @click="showSecret = !showSecret" class="absolute right-3 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200">
              <EyeOutline v-if="showSecret" class="w-5 h-5" />
              <EyeOffOutline v-else class="w-5 h-5" />
            </button>
          </div>
        </div>
        <div class="flex flex-col gap-2">
          <label class="text-sm font-medium text-slate-600 dark:text-slate-400">{{ t('subscription.rule_group') }}</label>
          <select v-model="currentConfig.rule_group" class="px-4 py-2.5 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none">
            <option value="lite">{{ t('subscription.rule_group_lite') }}</option>
            <option value="base">{{ t('subscription.rule_group_base') }}</option>
            <option value="full">{{ t('subscription.rule_group_full') }}</option>
          </select>
        </div>
        <div class="flex flex-col gap-2">
          <label class="text-sm font-medium text-slate-600 dark:text-slate-400">{{ t('subscription.ui_panel') }}</label>
          <select v-model="currentConfig.ui_panel" class="px-4 py-2.5 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none">
            <option value="metacubexd">MetaCubeXD</option>
            <option value="zashboard">Zashboard</option>
          </select>
        </div>
        <div class="flex flex-col gap-2">
          <label class="text-sm font-medium text-slate-600 dark:text-slate-400">{{ t('subscription.meta_backend_url') }}</label>
          <input type="text" v-model="currentConfig.meta_backend_url" :placeholder="t('subscription.meta_backend_url_placeholder')" class="px-4 py-2.5 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none" />
        </div>
      </div>

      <div class="flex justify-between items-center mt-8 mb-4">
        <h4 class="font-semibold text-base">{{ t('subscription.subscription_list') }}</h4>
        <button @click="openSubModal(-1)" class="px-3.5 py-1.5 bg-accent hover:bg-accent-hover text-white text-sm font-medium rounded-lg shadow-sm transition-all flex items-center gap-1">
          <AddOutline class="w-4 h-4" /> {{ t('subscription.add_subscription') }}
        </button>
      </div>

      <div id="subList" class="space-y-4">
        <div v-if="!currentConfig.subscriptions || currentConfig.subscriptions.length === 0" class="text-slate-400 dark:text-slate-600 text-sm py-4 text-center">
          {{ t('subscription.no_subscriptions') }}
        </div>
        <div v-else-if="currentConfig.subscriptions" v-for="(sub, idx) in currentConfig.subscriptions" :key="sub.name" class="p-4 rounded-xl border border-slate-100 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-900/30 flex flex-col gap-3 hover:-translate-y-[1px] hover:shadow-sm transition-all duration-300 relative overflow-hidden">
          <!-- 正在更新/健康检查的卡片遮罩层 -->
          <div v-if="isUpdating[idx] || isCheckingHealth[idx]" class="absolute inset-0 bg-white/75 dark:bg-[#1e293b]/75 backdrop-blur-[1px] z-10 flex items-center justify-center gap-2 animate-[fadeIn_0.15s_ease-out]">
            <div class="w-4 h-4 border-2 border-slate-300 dark:border-slate-700 !border-t-accent rounded-full animate-spin"></div>
            <span class="text-[11px] font-bold text-slate-500 dark:text-slate-400">
              {{ isUpdating[idx] ? t('rules.updating') : t('subscription.health_check') + '...' }}
            </span>
          </div>
          <div class="flex justify-between items-start gap-4">
            <div class="min-width-0 flex-1">
              <span class="font-semibold text-slate-800 dark:text-slate-100 break-all">{{ sub.name }}</span>
              <div class="text-xs text-slate-400 dark:text-slate-500 mt-1 select-all break-all flex items-center gap-1.5">
                <button @click="showUrls[idx] = !showUrls[idx]" class="text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 focus:outline-none" :title="showUrls[idx] ? t('subscription.hide_url') : t('subscription.show_url')">
                  <EyeOffOutline v-if="showUrls[idx]" class="w-3.5 h-3.5" />
                  <EyeOutline v-else class="w-3.5 h-3.5" />
                </button>
                <span>{{ showUrls[idx] ? sub.url : '••••••••' }}</span>
              </div>
            </div>
            <div class="flex gap-1.5">
              <button v-if="savedSubNames.has(sub.name)" @click="handleUpdateSub(idx)" :disabled="isUpdating[idx] || isCheckingHealth[idx]" class="p-2 hover:bg-slate-200 dark:hover:bg-slate-800 text-slate-500 dark:text-slate-400 rounded-lg transition-all" :title="t('rules.update')">
                <SyncOutline class="w-4 h-4 inline-block" :class="{ 'animate-spin': isUpdating[idx] }" />
              </button>
              <button @click="openSubModal(idx)" class="p-2 hover:bg-slate-200 dark:hover:bg-slate-800 text-slate-500 dark:text-slate-400 rounded-lg transition-all" :title="t('common.edit')">
                <CreateOutline class="w-4 h-4" />
              </button>
              <button @click="handleDeleteSub(idx)" class="p-2 hover:bg-red-500/10 hover:text-red-500 text-slate-500 dark:text-slate-400 rounded-lg transition-all" :title="t('common.delete')">
                <TrashOutline class="w-4 h-4" />
              </button>
            </div>
          </div>

          <div v-if="sub.info" class="space-y-2">
            <div class="flex items-center gap-3">
              <div class="flex-1 bg-slate-200 dark:bg-slate-800 h-2 rounded-full overflow-hidden">
                <div class="bg-accent h-full rounded-full transition-all" :style="{ width: Math.min(((sub.info.upload + sub.info.download) / (sub.info.total || 1)) * 100, 100) + '%' }"></div>
              </div>
              <span class="text-xs font-semibold text-accent">{{ ((sub.info.upload + sub.info.download) / (sub.info.total || 1) * 100).toFixed(1) }}%</span>
            </div>
            <div class="flex justify-between text-xs text-slate-500 dark:text-slate-400">
              <span>{{ formatGB(sub.info.upload + sub.info.download) }} / {{ formatGB(sub.info.total) }}</span>
              <span>{{ t('subscription.valid_until_label') }}{{ formatExpire(sub.info.expire) }}</span>
            </div>
            <div class="flex justify-between text-[11px] text-slate-400 dark:text-slate-500 mt-1">
              <span>{{ t('subscription.updated_at_label') }}{{ formatUpdateTime(sub.info.updatedAt) || t('common.unknown') }}</span>
            </div>
          </div>
          <div v-else class="text-xs text-slate-400 dark:text-slate-500">
            <template v-if="!savedSubNames.has(sub.name)">
              {{ t('subscription.save_to_show_info') }}
            </template>
            <template v-else>
              {{ !coreStatus.running ? `${t('config.core_stopped')}，${t('subscription.traffic_unavailable')}` : t('subscription.traffic_unavailable') }}
            </template>
          </div>
        </div>
      </div>

      <div class="mt-8 border-t border-slate-100 dark:border-slate-800 pt-6 flex items-center gap-4">
        <button @click="saveAndApply" :disabled="isApplying" class="px-6 py-2.5 bg-accent hover:bg-accent-hover text-white font-semibold rounded-xl shadow-lg shadow-accent/20 hover:shadow-accent/30 transition-all flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed">
          <SyncOutline v-if="isApplying" class="w-4 h-4 animate-spin inline-block" />
          {{ isApplying ? '正在保存并应用...' : t('subscription.save_and_apply') }}
        </button>
      </div>

      <!-- 保存并应用全屏模糊加载浮层 -->
      <div v-if="isApplying" class="fixed inset-0 bg-slate-900/10 dark:bg-slate-950/20 backdrop-blur-[1px] z-[9999] flex flex-col items-center justify-center gap-3 animate-[fadeIn_0.2s_ease-out]">
        <div class="bg-white/95 dark:bg-[#1e293b]/95 border border-slate-200/40 dark:border-slate-800/40 backdrop-blur-lg px-6 py-4 rounded-2xl shadow-xl flex items-center gap-3">
          <div class="w-5 h-5 border-2 border-slate-200 dark:border-slate-800 !border-t-accent rounded-full animate-spin"></div>
          <span class="text-xs font-bold text-slate-600 dark:text-slate-300">正在保存并应用订阅配置...</span>
        </div>
      </div>
    </div>

    <!-- Modal -->
    <div v-if="showModal" class="fixed inset-0 bg-slate-900/60 dark:bg-slate-950/80 backdrop-blur-sm z-50 flex items-center justify-center p-4">
      <div class="bg-white dark:bg-[#1e293b] w-full max-w-lg rounded-2xl shadow-xl border border-slate-200 dark:border-slate-800 p-6 flex flex-col gap-4 animate-[zoomIn_0.2s_ease-out]">
        <div class="flex justify-between items-center border-b border-slate-100 dark:border-slate-800 pb-3">
          <h2 class="text-lg font-bold">{{ modalTitle }}</h2>
          <button @click="closeSubModal" class="text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 flex items-center justify-center p-1 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all">
            <CloseOutline class="w-5 h-5" />
          </button>
        </div>

        <div class="space-y-4">
          <div class="flex flex-col gap-1.5">
            <label class="text-xs font-semibold text-slate-600 dark:text-slate-400">{{ t('subscription.name') }}</label>
            <input type="text" v-model="editForm.name" :placeholder="t('subscription.name_placeholder')" class="px-3.5 py-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none text-sm" />
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
          <button @click="closeSubModal" class="px-4 py-2 text-sm font-semibold rounded-lg bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 transition-all">
            {{ t('subscription.cancel') }}
          </button>
          <button @click="saveSubToList" class="px-4 py-2 text-sm font-semibold rounded-lg bg-accent hover:bg-accent-hover text-white transition-all shadow-md shadow-accent/15">
            {{ t('subscription.save_to_list') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style>
@keyframes zoomIn {
  from { opacity: 0; transform: scale(0.95); }
  to { opacity: 1; transform: scale(1); }
}
</style>
