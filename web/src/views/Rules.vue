<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
<<<<<<< HEAD
import { storeToRefs } from 'pinia'
import { apiFetch } from '../utils/api'
import { RefreshOutline } from '@vicons/ionicons5'
import { useGlobalStore } from '../store/global'
import { useRulesStore, type RuleItem } from '../store/rules'

const globalStore = useGlobalStore()
const rulesStore = useRulesStore()
const { rules, providers, isLoadingRules, isLoadingProviders } = storeToRefs(rulesStore)

const { t } = useI18n()

const activeTab = ref<'rules' | 'providers'>('rules')
const searchText = ref('')
const isUpdatingAll = ref(false)
const isUpdating = ref<Record<string, boolean>>({})

=======
import { apiFetch } from '../utils/api'
import { RefreshOutline } from '@vicons/ionicons5'
import { useGlobalStore } from '../store/global'

const globalStore = useGlobalStore()

const { t } = useI18n()

export interface RuleItem {
  type: string
  payload: string
  proxy: string
  enabled: boolean
  index?: number
}

export interface ProviderItem {
  name: string
  type: string
  ruleCount: number
  updatedAt: string
}

const activeTab = ref<'rules' | 'providers'>('rules')
const rules = ref<RuleItem[]>([])
const providers = ref<ProviderItem[]>([])
const searchText = ref('')
const isLoadingRules = ref(false)
const isLoadingProviders = ref(false)
const isUpdatingAll = ref(false)
const isUpdating = ref<Record<string, boolean>>({})

// 获取所有规则
const fetchRules = async () => {
  isLoadingRules.value = true
  try {
    const resp = await apiFetch('/rules')
    if (resp.ok) {
      const data = await resp.json()
      const list = data.rules || []
      list.forEach((r: any, idx: number) => {
        r.index = idx
        r.enabled = !(r.extra?.disabled === true)
      })
      rules.value = list
    }
  } catch (e) {
    console.error('获取规则失败', e)
  } finally {
    isLoadingRules.value = false
  }
}

// 获取规则提供商
const fetchProviders = async () => {
  isLoadingProviders.value = true
  try {
    const resp = await apiFetch('/providers/rules')
    if (resp.ok) {
      const data = await resp.json()
      const list = Object.values(data.providers || {}) as ProviderItem[]
      providers.value = list
    }
  } catch (e) {
    console.error('获取规则提供商失败', e)
  } finally {
    isLoadingProviders.value = false
  }
}

>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
// 过滤规则
const filteredRules = computed(() => {
  const query = searchText.value.trim().toLowerCase()
  if (!query) return rules.value
  return rules.value.filter(rule => 
    rule.type.toLowerCase().includes(query) || 
    rule.payload.toLowerCase().includes(query) || 
    rule.proxy.toLowerCase().includes(query)
  )
})

// 过滤提供商
const filteredProviders = computed(() => {
  const query = searchText.value.trim().toLowerCase()
  if (!query) return providers.value
  return providers.value.filter(p => 
    p.name.toLowerCase().includes(query) || 
    p.type.toLowerCase().includes(query)
  )
})

<<<<<<< HEAD
// 刷新规则数据
const handleRefreshRules = async () => {
  await rulesStore.fetchRules()
  globalStore.showToast(t('rules.updated'), 'success')
}

=======
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
// 单个更新提供商
const handleUpdateProvider = async (name: string) => {
  isUpdating.value[name] = true
  try {
    const encoded = encodeURIComponent(name)
    const resp = await apiFetch(`/providers/rules/${encoded}`, { method: 'PUT' })
    if (resp.ok) {
      globalStore.showToast(t('rules.provider_update_success', { name }), 'success')
<<<<<<< HEAD
      await rulesStore.fetchProviders()
      await rulesStore.fetchRules(true)
=======
      fetchProviders()
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
    } else {
      globalStore.showToast(t('rules.provider_update_failed', { name }), 'error')
    }
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    isUpdating.value[name] = false
  }
}

// 全部更新提供商
const handleUpdateAllProviders = async () => {
  if (providers.value.length === 0) return
  isUpdatingAll.value = true
  try {
    const promises = providers.value.map(p => {
      const encoded = encodeURIComponent(p.name)
      return apiFetch(`/providers/rules/${encoded}`, { method: 'PUT' })
        .then(resp => ({ name: p.name, ok: resp.ok }))
        .catch(() => ({ name: p.name, ok: false }))
    })
    const results = await Promise.all(promises)
    const successCount = results.filter(r => r.ok).length
    const failCount = results.length - successCount
    globalStore.showToast(t('rules.batch_update_partial', { success: successCount, fail: failCount }), failCount > 0 ? 'warning' : 'success')
<<<<<<< HEAD
    await rulesStore.fetchProviders()
    await rulesStore.fetchRules(true)
=======
    fetchProviders()
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    isUpdatingAll.value = false
  }
}

<<<<<<< HEAD
// 规则禁用启用开关（乐观更新及失败回滚）
const handleToggleRule = async (rule: RuleItem) => {
  const originalIdx = rule.index
  if (originalIdx === undefined) return

  const originalEnabled = rule.enabled
  rule.enabled = !originalEnabled

  try {
    const payload = { [originalIdx]: !originalEnabled }
=======
// 规则禁用启用开关
const handleToggleRule = async (rule: RuleItem) => {
  const originalIdx = rule.index
  if (originalIdx === undefined) return
  try {
    const payload = { [originalIdx]: !rule.enabled }
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
    const resp = await apiFetch('/rules/disable', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (resp.ok) {
<<<<<<< HEAD
      globalStore.showToast(t('rules.updated'), 'success')
    } else {
      rule.enabled = originalEnabled
      globalStore.showToast(t('common.operation_failed'), 'error')
    }
  } catch (e) {
    rule.enabled = originalEnabled
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  }
}

=======
      const found = rules.value.find(r => r.index === originalIdx)
      if (found) {
        found.enabled = !rule.enabled
      }
    }
  } catch (e) {
    console.error('修改规则状态失败', e)
  }
}


>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
const formatDate = (dateStr: string) => {
  if (!dateStr || dateStr.startsWith('0001')) return t('rules.unknown_time')
  const date = new Date(dateStr)
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
}

onMounted(() => {
<<<<<<< HEAD
  const hasData = rules.value.length > 0
  rulesStore.fetchRules(hasData)
  rulesStore.fetchProviders(hasData)
=======
  fetchRules()
  fetchProviders()
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
})
</script>

<template>
  <div class="space-y-4">
    <div class="bg-white dark:bg-[#1e293b] p-4 rounded-xl border border-slate-200 dark:border-slate-800 shadow-sm flex flex-wrap gap-4 items-center justify-between transition-all">
      <div class="flex bg-slate-100 dark:bg-slate-800 p-1 rounded-lg">
        <button @click="activeTab = 'rules'" class="px-4 py-1.5 text-xs font-semibold rounded-md transition-all" :class="activeTab === 'rules' ? 'bg-white dark:bg-slate-700 shadow-sm text-slate-800 dark:text-slate-100' : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'">
          {{ t('rules.rules_tab') }}
        </button>
        <button @click="activeTab = 'providers'" class="px-4 py-1.5 text-xs font-semibold rounded-md transition-all" :class="activeTab === 'providers' ? 'bg-white dark:bg-slate-700 shadow-sm text-slate-800 dark:text-slate-100' : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'">
          {{ t('rules.providers_tab') }}
        </button>
      </div>

      <div class="flex gap-2 items-center flex-1 sm:flex-initial min-w-[200px] sm:min-w-0">
        <input type="text" v-model="searchText" :placeholder="t('rules.search_placeholder')" class="w-full sm:w-60 px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none" />
      </div>

<<<<<<< HEAD
      <div>
        <button v-if="activeTab === 'rules'" @click="handleRefreshRules" :disabled="isLoadingRules" class="px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all flex items-center gap-1.5">
          <RefreshOutline class="w-3.5 h-3.5" :class="{ 'animate-spin': isLoadingRules }" />
          {{ t('common.refresh') }}
        </button>
        <button v-else-if="activeTab === 'providers'" @click="handleUpdateAllProviders" :disabled="isUpdatingAll" class="px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all flex items-center gap-1.5">
=======
      <div v-if="activeTab === 'providers'">
        <button @click="handleUpdateAllProviders" :disabled="isUpdatingAll" class="px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all flex items-center gap-1.5">
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
          <RefreshOutline class="w-3.5 h-3.5" :class="{ 'animate-spin': isUpdatingAll }" />
          {{ t('rules.update_all') }}
        </button>
      </div>
    </div>

    <div v-if="activeTab === 'rules'" class="bg-white dark:bg-[#1e293b] rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm overflow-hidden transition-all">
      <div v-if="isLoadingRules" class="p-8 text-center text-slate-400 dark:text-slate-600 text-sm">
        {{ t('common.loading') }}
      </div>
      <div v-else-if="filteredRules.length === 0" class="p-8 text-center text-slate-400 dark:text-slate-600 text-sm">
        {{ t('rules.no_rules_found') }}
      </div>
      <div v-else class="divide-y divide-slate-100 dark:divide-slate-800">
        <div v-for="(rule, idx) in filteredRules" :key="idx" class="px-5 py-3.5 flex items-center justify-between gap-4 hover:bg-slate-50/50 dark:hover:bg-slate-900/10 transition-colors">
          <div class="flex-1 min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <span class="px-2 py-0.5 text-[10px] font-bold uppercase rounded tracking-wide shrink-0" :class="{
                'bg-blue-500/10 text-blue-500': rule.type === 'IP-CIDR' || rule.type === 'IP-CIDR6',
                'bg-amber-500/10 text-amber-500': rule.type === 'DOMAIN-SUFFIX' || rule.type === 'DOMAIN-KEYWORD',
                'bg-purple-500/10 text-purple-500': rule.type === 'DOMAIN',
                'bg-slate-500/10 text-slate-500': rule.type === 'MATCH'
              }">
                {{ rule.type }}
              </span>
              <span class="text-sm font-medium text-slate-700 dark:text-slate-200 break-all select-all">{{ rule.payload }}</span>
            </div>
            <div class="text-xs text-slate-400 dark:text-slate-500 mt-1">
              {{ t('rules.proxy') }}: <span class="font-semibold text-accent">{{ rule.proxy }}</span>
            </div>
          </div>

          <div class="shrink-0">
            <button @click="handleToggleRule(rule)" class="w-10 h-6 flex items-center rounded-full p-0.5 transition-all" :class="rule.enabled ? 'bg-accent justify-end' : 'bg-slate-200 dark:bg-slate-700 justify-start'">
              <span class="w-5 h-5 rounded-full bg-white shadow-md"></span>
            </button>
          </div>
        </div>
      </div>
    </div>

    <div v-if="activeTab === 'providers'" class="bg-white dark:bg-[#1e293b] rounded-2xl border border-slate-200 dark:border-slate-800 shadow-sm overflow-hidden transition-all">
      <div v-if="isLoadingProviders" class="p-8 text-center text-slate-400 dark:text-slate-600 text-sm">
        {{ t('common.loading') }}
      </div>
      <div v-else-if="filteredProviders.length === 0" class="p-8 text-center text-slate-400 dark:text-slate-600 text-sm">
        {{ t('rules.no_providers') }}
      </div>
      <div v-else class="divide-y divide-slate-100 dark:divide-slate-800">
        <div v-for="p in filteredProviders" :key="p.name" class="p-5 flex items-center justify-between gap-4 hover:bg-slate-50/50 dark:hover:bg-slate-900/10 transition-colors">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2">
              <strong class="text-sm font-bold text-slate-800 dark:text-slate-100 break-all">{{ p.name }}</strong>
              <span class="px-1.5 py-0.5 text-[9px] font-semibold bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 rounded uppercase">{{ p.type }}</span>
            </div>
            <div class="text-xs text-slate-500 dark:text-slate-400 mt-1.5 space-x-4">
              <span>{{ t('rules.rule_count') }}: <strong class="text-slate-700 dark:text-slate-300">{{ p.ruleCount }}</strong></span>
              <span>{{ t('rules.updated_at') }}: <span class="text-slate-400">{{ formatDate(p.updatedAt) }}</span></span>
            </div>
          </div>

          <div class="shrink-0">
            <button @click="handleUpdateProvider(p.name)" :disabled="isUpdating[p.name]" class="p-2 hover:bg-slate-200 dark:hover:bg-slate-800 text-slate-500 dark:text-slate-400 rounded-lg transition-all">
              <RefreshOutline class="w-4 h-4" :class="{ 'animate-spin': isUpdating[p.name] }" />
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
