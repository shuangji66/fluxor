<script setup lang="ts">
import { ref, onMounted, onUnmounted, onActivated, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { apiFetch } from '../utils/api'
import { SyncOutline, LayersOutline } from '@vicons/ionicons5'
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

// 滚动加载显示的最大条数
const displayLimit = ref(1000)

// 用户输入搜索词时重置显示条数，保障过滤秒开
watch(searchText, () => {
  displayLimit.value = 1000
})

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

// 最终在 DOM 中渲染的截断规则列表
const visibleRules = computed(() => {
  return filteredRules.value.slice(0, displayLimit.value)
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

// 单个更新提供商
const handleUpdateProvider = async (name: string) => {
  isUpdating.value[name] = true
  try {
    const encoded = encodeURIComponent(name)
    const resp = await apiFetch(`/providers/rules/${encoded}`, { method: 'PUT' })
    if (resp.ok) {
      globalStore.showToast(t('rules.provider_update_success', { name }), 'success')
      await rulesStore.fetchProviders()
      await rulesStore.fetchRules(true)
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
    const queue = [...providers.value]
    const results: { name: string; ok: boolean }[] = []

    // 并发度为 3 的 Worker 机制
    const workers = Array.from({ length: Math.min(3, queue.length) }, async () => {
      while (queue.length > 0) {
        const next = queue.shift()
        if (next) {
          try {
            const encoded = encodeURIComponent(next.name)
            const resp = await apiFetch(`/providers/rules/${encoded}`, { method: 'PUT' })
            results.push({ name: next.name, ok: resp.ok })
          } catch (err) {
            results.push({ name: next.name, ok: false })
          }
        }
      }
    })

    await Promise.all(workers)

    const successCount = results.filter(r => r.ok).length
    const failCount = results.length - successCount
    globalStore.showToast(t('rules.batch_update_partial', { success: successCount, fail: failCount }), failCount > 0 ? 'warning' : 'success')
    await rulesStore.fetchProviders()
    await rulesStore.fetchRules(true)
  } catch (e) {
    globalStore.showToast(`${t('common.error')}: ${(e as Error).message}`, 'error')
  } finally {
    isUpdatingAll.value = false
  }
}

// 规则禁用启用开关（乐观更新及失败回滚）
const handleToggleRule = async (rule: RuleItem) => {
  const originalIdx = rule.index
  if (originalIdx === undefined) return

  const originalEnabled = rule.enabled
  rule.enabled = !originalEnabled

  try {
    const payload = { [originalIdx]: originalEnabled }
    const resp = await apiFetch('/rules/disable', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
    if (resp.ok) {
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

const formatDate = (dateStr: string) => {
  if (!dateStr || dateStr.startsWith('0001')) return t('rules.unknown_time')
  const date = new Date(dateStr)
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
}

const loadMoreTrigger = ref<HTMLElement | null>(null)
let observer: IntersectionObserver | null = null

// 监听 Trigger 元素的挂载与销毁，动态绑定/解绑观察器
watch(loadMoreTrigger, (newEl, oldEl) => {
  if (oldEl && observer) {
    observer.unobserve(oldEl)
  }
  if (newEl) {
    if (!observer) {
      const scrollContainer = document.querySelector('main')
      observer = new IntersectionObserver((entries) => {
        if (entries[0].isIntersecting) {
          if (displayLimit.value < filteredRules.value.length) {
            displayLimit.value += 1000
          }
        }
      }, { 
        root: scrollContainer,
        rootMargin: '150px' 
      })
    }
    observer.observe(newEl)
  }
})

onMounted(() => {
  const hasData = rules.value.length > 0
  rulesStore.fetchRules(hasData)
  rulesStore.fetchProviders(hasData)
})

onActivated(() => {
  rulesStore.fetchRules(true)
  rulesStore.fetchProviders(true)
})

onUnmounted(() => {
  if (observer) {
    observer.disconnect()
  }
})
</script>

<template>
  <div class="flex flex-col flex-1 min-h-0 gap-4 h-full">
    <div class="glass-medium shadow-none px-6 py-3 md:py-0 rounded-xl border border-slate-200/50 dark:border-slate-800/50 flex flex-wrap gap-4 items-center justify-between transition-all shrink-0 h-auto min-h-[56px] md:h-[56px]">
      <div class="flex items-center justify-between md:justify-start gap-4 flex-1 md:flex-initial">
        <h3 class="text-base font-semibold flex items-center gap-2">
          <LayersOutline class="w-5 h-5 text-accent" />
          {{ t('nav.rules') }}
        </h3>
        <div class="flex bg-slate-100 dark:bg-slate-800 rounded-lg p-0.5 transition-all shrink-0">
          <button @click="activeTab = 'rules'" class="px-4 py-1.5 text-xs font-semibold rounded-md transition-all duration-200" :class="activeTab === 'rules' ? 'bg-accent text-white shadow-sm' : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'">
            {{ t('rules.rules_tab') }} ({{ rules.length }})
          </button>
          <button @click="activeTab = 'providers'" class="px-4 py-1.5 text-xs font-semibold rounded-md transition-all duration-200" :class="activeTab === 'providers' ? 'bg-accent text-white shadow-sm' : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'">
            {{ t('rules.providers_tab') }} ({{ providers.length }})
          </button>
        </div>
      </div>
      <!-- 右侧：搜索框与控制按钮合并容器（在大屏下横向右对齐，绝不换行） -->
      <div class="flex items-center gap-3 flex-1 justify-end min-w-[280px] sm:min-w-0 flex-nowrap">
        <input type="text" v-model="searchText" :placeholder="t('rules.search_placeholder')" class="w-full sm:w-60 px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none" />
        
        <div class="flex gap-2 shrink-0">
          <button v-if="activeTab === 'providers'" @click="handleUpdateAllProviders" :disabled="isUpdatingAll" class="px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap">
            <SyncOutline class="w-3.5 h-3.5" :class="{ 'animate-spin': isUpdatingAll }" />
            {{ t('rules.update_all') }}
          </button>
        </div>
      </div>
    </div>

    <!-- 统一大内容卡片 -->
    <div class="flex-1 min-h-0 glass-medium shadow-none rounded-xl border border-slate-200/50 dark:border-slate-800/50 flex flex-col overflow-hidden">
      <!-- 规则列表 -->
      <div v-if="activeTab === 'rules'" class="flex-1 min-h-0 overflow-y-auto pr-1">
        <div v-if="isLoadingRules && rules.length === 0" class="p-8 text-center text-slate-400 dark:text-slate-600 text-sm">
          {{ t('common.loading') }}
        </div>
        <template v-else>
          <div v-if="filteredRules.length === 0" class="p-8 text-center text-slate-400 dark:text-slate-600 text-sm">
            {{ t('rules.no_rules_found') }}
          </div>
          <div v-else class="divide-y divide-slate-100 dark:divide-slate-800">
            <div v-for="(rule, idx) in visibleRules" :key="idx" class="px-5 py-3.5 flex items-center justify-between gap-4 hover:bg-slate-50/50 dark:hover:bg-slate-900/10 transition-colors">
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
            <div v-if="filteredRules.length > displayLimit" class="py-3 text-center text-xs text-slate-400 flex items-center justify-center gap-1.5 border-t border-slate-100 dark:border-slate-800 bg-slate-50/20 dark:bg-slate-900/5">
              <div class="w-3.5 h-3.5 border border-slate-300 dark:border-slate-700 !border-t-accent rounded-full animate-spin"></div>
              <span>{{ t('common.loading') }}</span>
            </div>
            <div ref="loadMoreTrigger" class="h-2"></div>
          </div>
        </template>
      </div>

      <!-- 提供商列表 -->
      <div v-if="activeTab === 'providers'" class="flex-1 min-h-0 overflow-y-auto pr-1">
        <div v-if="isLoadingProviders && providers.length === 0" class="p-8 text-center text-slate-400 dark:text-slate-600 text-sm">
          {{ t('common.loading') }}
        </div>
        <template v-else>
          <div v-if="filteredProviders.length === 0" class="p-8 text-center text-slate-400 dark:text-slate-600 text-sm">
            {{ t('rules.no_providers') }}
          </div>
          <div v-else class="divide-y divide-slate-100 dark:divide-slate-800">
            <div v-for="p in filteredProviders" :key="p.name" class="p-5 flex items-center justify-between gap-4 hover:bg-slate-50/50 dark:hover:bg-slate-900/10 transition-colors relative overflow-hidden">
              <!-- 规则更新局部加载遮罩 -->
              <div v-if="isUpdating[p.name] || isUpdatingAll" class="absolute inset-0 glass-light z-10 flex items-center justify-center gap-2 animate-[fadeIn_0.15s_ease-out]">
                <div class="w-4 h-4 border-2 border-slate-300 dark:border-slate-700 !border-t-accent rounded-full animate-spin"></div>
                <span class="text-[11px] font-bold text-slate-500 dark:text-slate-400">
                  {{ t('rules.updating') }}
                </span>
              </div>
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
                  <SyncOutline class="w-4 h-4 inline-block" :class="{ 'animate-spin': isUpdating[p.name] }" />
                </button>
              </div>
            </div>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>
