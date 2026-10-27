<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { SearchOutline } from '@vicons/ionicons5'
import { apiFetch, readErrorMessage } from '../../utils/api'

/**
 * 内置 DNS 查询卡片。
 *
 * 原先内联在配置页里：输入框 + 类型下拉 + 结果区，状态与请求逻辑都是自成一体的，
 * 与配置页其余部分没有耦合。抽出来后配置页少一段「表单 + 请求 + 结果渲染」的样板，
 * 这个面板也能独立复用（例如后续放到概览页的排障区）。
 *
 * 可见性由父组件决定（仅在 `coreStatus.running` 时渲染）——内核未运行时该接口必然失败。
 */
const { t } = useI18n()

const dnsQuery = ref({
  name: '',
  type: 'A',
  result: '',
  loading: false,
})

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
        dnsQuery.value.result = data.Answer.map((a: { data?: string }) => a.data).join('\n')
      } else {
        dnsQuery.value.result = JSON.stringify(data, null, 2)
      }
    } else {
      // 失败时带上后端原因（如 DNS 服务器不可达），否则只剩一句「查询失败」无从排查
      dnsQuery.value.result = `${t('config.dns_query_failed')}: ${await readErrorMessage(resp)}`
    }
  } catch (e) {
    dnsQuery.value.result = `${t('config.dns_query_failed')}: ${(e as Error).message}`
  } finally {
    dnsQuery.value.loading = false
  }
}
</script>

<template>
  <div
    class="live-card bg-slate-50/50 dark:bg-slate-900/30 p-6 rounded-xl border border-slate-200/40 dark:border-slate-800/40 hover:border-slate-300/80 dark:hover:border-slate-700/80 hover:-translate-y-[3px] hover:shadow-md hover:bg-slate-100/80 dark:hover:bg-slate-900/80 duration-300 space-y-4 h-full transition-all flex flex-col relative"
  >
    <h4 class="font-bold text-sm border-b border-slate-100 dark:border-slate-800 pb-3 flex items-center gap-2">
      <SearchOutline class="w-4 h-4 text-accent" />
      {{ t('config.dns_query') }}
    </h4>

    <div class="flex-1 flex flex-col justify-between gap-4">
      <div class="flex flex-col gap-2">
        <input
          type="text"
          v-model="dnsQuery.name"
          :placeholder="t('config.dns_placeholder')"
          @keyup.enter="handleDNSQuery"
          class="w-full px-4 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none"
        />
        <div class="flex gap-2">
          <select
            v-model="dnsQuery.type"
            class="flex-1 px-3 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 focus:ring-2 focus:ring-accent outline-none"
          >
            <option value="A">A</option>
            <option value="AAAA">AAAA</option>
            <option value="MX">MX</option>
            <option value="TXT">TXT</option>
          </select>
          <button
            @click="handleDNSQuery"
            :disabled="dnsQuery.loading"
            class="flex-[2] py-2 bg-accent hover:bg-accent-hover text-white text-xs font-semibold rounded-lg shadow-sm transition-all flex items-center justify-center gap-1"
          >
            {{ dnsQuery.loading ? t('config.dns_querying') : t('config.dns_query_btn') }}
          </button>
        </div>
      </div>

      <!-- 结果容器：高度有上下限（下限保证空态/少行时不下塌，上限保证行数再多也不撑大卡片），
           超出上限的内容在容器内纵向滚动 -->
      <pre
        class="p-4 bg-slate-50 dark:bg-slate-900/50 font-mono text-xs rounded-xl overflow-y-auto whitespace-pre-wrap break-all flex-1 min-h-28 max-h-56 border border-slate-200 dark:border-slate-800 transition-all"
        :class="dnsQuery.result ? 'text-emerald-700 dark:text-emerald-400' : 'text-slate-400 dark:text-slate-500 italic flex items-center justify-center select-none'"
      >{{ dnsQuery.result || t('config.dns_result_default') }}</pre>
    </div>
  </div>
</template>
