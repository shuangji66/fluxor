<script setup lang="ts">
// 自定义模式的「添加 / 编辑节点」弹窗。
//
// 表单完全由后端下发的协议字段表驱动：
//   基础项 → 传输层（network + ws-opts/grpc-opts 等，按 network 条件显示）→ TLS → 高级。
// 默认值预填自后端声明（default），留空即用协议默认值。组件不做任何 API 请求——
// 保存只把节点对象交给父组件放进列表，真正落库发生在「保存并应用」。
import { computed, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { CloseOutline, ChevronDownOutline, ChevronForwardOutline, InformationCircleOutline } from '@vicons/ionicons5'
import NodeFieldInput from './NodeFieldInput.vue'
import AppModal from './AppModal.vue'
import FieldLabel from './FieldLabel.vue'
import type { CustomNode, NodeFieldSpec, NodeFieldSection, NodeProtocolSpec } from '../store/subscription'

const props = defineProps<{
  visible: boolean
  // 视图激活态：KeepAlive 停用时必须收起 Teleport 浮层，否则会跨页残留
  isActive: boolean
  protocols: NodeProtocolSpec[]
  /** 待编辑的节点（null 表示新增） */
  node: CustomNode | null
  /** 列表里已有的其他节点名，用于即时提示重名 */
  takenNames: string[]
}>()

const emit = defineEmits<{
  (e: 'save', node: CustomNode): void
  (e: 'close'): void
}>()

const { t, getLocaleMessage } = useI18n()

// 中文界面下配置标题要附英文小字（见 FieldLabel）：标题类文案的英文统一从 en 语言包取，
// 避免在组件里再写一份英文常量而与 i18n 漂移。
const englishText = (key: string): string =>
  ((getLocaleMessage('en') as Record<string, string>)[key] || '')

// 折叠区（跳过基础项：它始终展开）。顺序即界面顺序。
const sections: { key: Exclude<NodeFieldSection, ''>, titleKey: string }[] = [
  { key: 'transport', titleKey: 'subscription.section_transport' },
  { key: 'tls', titleKey: 'subscription.section_tls' },
  { key: 'advanced', titleKey: 'subscription.node_advanced' },
]

// 表单状态：values 按字段键存「界面原始取值」（字符串/数字/布尔/分隔文本/子字段对象），
// 提交时原样交给后端归一化（拆分列表与键值对、类型转换、剔除默认值都在后端完成）。
const form = reactive({
  protocolType: '',
  name: '',
  values: {} as Record<string, any>,
})

const openSections = reactive<Record<string, boolean>>({ transport: false, tls: false, advanced: false })

const selectedProtocol = computed(() =>
  props.protocols.find(p => p.type === form.protocolType) || null
)

const fields = computed<NodeFieldSpec[]>(() => selectedProtocol.value?.fields || [])

// 空值判定：与后端 nodespec.IsEmpty 严格同义
//（空串 / 0 / false / 空列表 / 空对象都算未填写）。false 必须算空，否则每个
// 默认关闭的开关都会被误判成「用户改过」，分区会在初次打开时就全部展开。
const isEmpty = (value: any) => {
  if (value === undefined || value === null || value === '' || value === 0 || value === false) return true
  if (Array.isArray(value)) return value.length === 0
  if (typeof value === 'object') return Object.keys(value).length === 0
  return false
}

// 嵌套块是否「被填写」：与后端 blockFilled 同义——读取接口下发的块会把每个子字段
// 都补成默认值，只看「对象非空」会把空块当成已填，于是块内必填项对着空气报错。
const groupFilled = (field: NodeFieldSpec, value: any): boolean => {
  if (field.kind !== 'group') return !isEmpty(value)
  return (field.children || []).some(child => groupFilled(child, value?.[child.key]))
}

// 条件显示：同层字段取值命中时才渲染（如 ws-opts 只在 network=ws 时出现）
const isVisible = (field: NodeFieldSpec) => {
  const gate = field.visible_when
  if (!gate) return true
  return gate.values.includes(String(form.values[gate.key] ?? ''))
}

// 某分区里当前可见的字段
const sectionFields = (section: Exclude<NodeFieldSection, ''>) =>
  fields.value.filter(f => (f.section || '') === section && isVisible(f))

const basicFields = computed(() => fields.value.filter(f => !f.section && isVisible(f)))

const title = computed(() => (props.node ? t('subscription.edit_node_modal_title') : t('subscription.add_node_modal_title')))

const nameError = computed(() => {
  const name = form.name.trim()
  if (!name) return t('subscription.node_name_required')
  if (props.takenNames.includes(name)) return t('subscription.node_duplicate_name')
  return ''
})

// 字段默认取值：显式声明了 default 用它；嵌套块递归展开子字段；其余按类型给空值
const defaultFor = (field: NodeFieldSpec): any => {
  if (field.kind === 'group') {
    const nested: Record<string, any> = {}
    for (const child of field.children || []) nested[child.key] = defaultFor(child)
    return nested
  }
  if (field.default !== undefined && field.default !== null) return field.default
  switch (field.kind) {
    case 'int': return 0
    case 'bool': return false
    case 'list': return ''
    case 'map': return ''
    default: return ''
  }
}

// 列表 / 键值对在界面上按分隔文本编辑；后端补齐的结果（数组 / 映射）转回文本
const toInputValue = (field: NodeFieldSpec, raw: any): any => {
  if (field.kind === 'group') {
    const nested: Record<string, any> = {}
    for (const child of field.children || []) nested[child.key] = toInputValue(child, raw?.[child.key])
    return nested
  }
  if (raw === undefined || raw === null) return defaultFor(field)
  if (field.kind === 'list') return Array.isArray(raw) ? raw.join(', ') : String(raw)
  if (field.kind === 'map') {
    if (typeof raw === 'object') {
      return Object.entries(raw as Record<string, string>).map(([k, v]) => `${k}: ${v}`).join('\n')
    }
    return String(raw)
  }
  return raw
}

const valuesForProtocol = (protocol: NodeProtocolSpec | null, source?: Record<string, any>): Record<string, any> => {
  const values: Record<string, any> = {}
  for (const field of protocol?.fields || []) {
    values[field.key] = source ? toInputValue(field, source[field.key]) : defaultFor(field)
  }
  return values
}

// 单个字段是否被改过（与默认值不同）：嵌套块递归判断，避免「子项默认值组成的对象」
// 被当成用户已填内容——那会让 TLS / 传输层分区在初次打开时就全部展开。
const isCustomized = (field: NodeFieldSpec, value: any): boolean => {
  if (field.kind === 'group') {
    return (field.children || []).some(child => isCustomized(child, value?.[child.key]))
  }
  if (isEmpty(value)) return false
  return String(value) !== String(field.default ?? '')
}

// 协议里是否存在 tls 开关（存在则由用户显式开启 TLS，不存在说明 TLS 是该协议的固有部分）

// 某（可见）分区是否已有「非默认」取值：用于决定初次渲染时是否展开
const hasCustomized = (section: Exclude<NodeFieldSection, ''>) =>
  sectionFields(section).some(f => isCustomized(f, form.values[f.key]))

// 折叠区默认展开规则：
//   - 传输层：已选 network，或块内有已填取值；
//   - TLS：协议没有 tls 开关（TLS 隐式启用，如 trojan）时默认展开，否则仅在有已填取值时展开；
//   - 高级：仅在编辑已有节点且确实填过值时展开。
const syncOpenSections = () => {
  const hasTlsSwitch = fields.value.some(f => f.key === 'tls')
  openSections.transport = !isEmpty(form.values.network) || hasCustomized('transport')
  openSections.tls = (!hasTlsSwitch && sectionFields('tls').length > 0) || hasCustomized('tls')
  openSections.advanced = hasCustomized('advanced')
}

// 初始化表单：编辑用节点已保存的取值（后端下发时已补齐默认值），新增用协议默认值预填
const resetForm = () => {
  if (props.node) {
    form.protocolType = props.node.type
    form.name = props.node.name
    form.values = valuesForProtocol(selectedProtocol.value, props.node.config)
  } else {
    form.protocolType = form.protocolType || props.protocols[0]?.type || ''
    form.name = ''
    form.values = valuesForProtocol(selectedProtocol.value)
  }
  syncOpenSections()
}

// 切换协议：字段整体换一套，名称保留（用户通常先填名字再挑协议）
const onProtocolChange = () => {
  form.values = valuesForProtocol(selectedProtocol.value)
  syncOpenSections()
}

// 选好传输方式 / 打开 TLS 后自动展开对应分区，否则用户看不到刚出现的选项块
watch(
  () => [form.values.network, form.values.tls] as const,
  ([network, tls]) => {
    if (!isEmpty(network)) openSections.transport = true
    if (tls === true) openSections.tls = true
  }
)

watch(
  () => props.visible,
  (visible) => {
    if (visible) resetForm()
  },
  { immediate: true }
)

// 必填校验：嵌套块「要么整块不填、要么填齐必填子项」
const collectMissingRequired = (list: NodeFieldSpec[], values: Record<string, any>, prefix = ''): string[] => {
  const missing: string[] = []
  for (const field of list) {
    if (!isVisible(field)) continue
    const value = values?.[field.key]
    if (field.kind === 'group') {
      if (!groupFilled(field, value)) continue
      missing.push(...collectMissingRequired(field.children || [], value, `${field.key}.`))
      continue
    }
    if (field.required && isEmpty(value)) missing.push(prefix + field.key)
  }
  return missing
}

const missingRequired = computed(() => collectMissingRequired(fields.value, form.values))

// 备用组合校验（RequireAny：外层「或」、内层「与」），与后端同义
const missingRequireAny = computed(() => {
  const groups = selectedProtocol.value?.require_any || []
  if (groups.length === 0) return []
  const satisfied = groups.some(group => group.every(key => !isEmpty(form.values[key])))
  return satisfied ? [] : groups.map(group => group.join(' + '))
})

const handleSave = () => {
  if (nameError.value || missingRequired.value.length > 0 || missingRequireAny.value.length > 0) return
  const config: Record<string, any> = {}
  for (const field of fields.value) {
    config[field.key] = form.values[field.key]
  }
  emit('save', {
    id: props.node?.id || '',
    name: form.name.trim(),
    type: form.protocolType,
    config,
  })
}
</script>

<template>
  <AppModal
    :open="isActive && visible"
    :label="title"
    panel-class="w-full max-w-3xl max-h-[88vh] rounded-[20px] p-6 flex flex-col gap-4"
    :close-on-backdrop="false"
    @close="emit('close')"
  >
    <div class="flex justify-between items-center border-b border-slate-100 dark:border-slate-800 pb-3 shrink-0">
      <h2 class="text-lg font-bold">{{ title }}</h2>
      <button @click="emit('close')" class="text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 flex items-center justify-center p-1 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-all">
        <CloseOutline class="w-5 h-5" />
      </button>
    </div>

    <!-- pr-2/pb-2：右侧给覆盖式滚动条留位，底部避免末项底边被滚动区边界裁掉。 -->
    <div class="flex-1 min-h-0 overflow-y-auto pr-2 pb-2 space-y-4">
      <!-- 第一项：协议（决定下面渲染哪些字段） -->
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <div class="flex flex-col gap-1.5">
          <label class="text-xs font-semibold text-slate-600 dark:text-slate-400"><FieldLabel :text="t('subscription.node_protocol')" :english="englishText('subscription.node_protocol')" /> <span class="text-danger">*</span></label>
          <select
            v-model="form.protocolType"
            @change="onProtocolChange"
            class="px-3.5 py-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none text-sm"
          >
            <option v-for="protocol in protocols" :key="protocol.type" :value="protocol.type">{{ protocol.name }}</option>
          </select>
          <!-- 过时协议仅提示，不禁用、不阻止保存：既有节点与机场仍在用的协议都要能存进去 -->
          <p v-if="selectedProtocol?.deprecated" class="text-[11px] text-danger">{{ t('subscription.node_protocol_deprecated') }}</p>
          <!-- 隧道类协议：中性小字（不是错误），提示按目标网段引流；同样不参与任何校验 -->
          <p v-else-if="selectedProtocol?.tunnel" class="text-[11px] text-slate-400 dark:text-slate-500">{{ t('subscription.node_protocol_tunnel_hint') }}</p>
        </div>
        <div class="flex flex-col gap-1.5">
          <label class="text-xs font-semibold text-slate-600 dark:text-slate-400"><FieldLabel :text="t('subscription.node_name')" :english="englishText('subscription.node_name')" /> <span class="text-danger">*</span></label>
          <input
            type="text"
            v-model="form.name"
            :placeholder="t('subscription.node_name_placeholder')"
            class="px-3.5 py-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/50 focus:ring-2 focus:ring-accent outline-none text-sm"
          />
          <p v-if="nameError" class="text-[11px] text-danger">{{ nameError }}</p>
        </div>
      </div>

      <!-- 基础项：始终展开 -->
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <NodeFieldInput
          v-for="field in basicFields"
          :key="field.key"
          :field="field"
          :protocol-type="form.protocolType"
          v-model="form.values[field.key]"
        />
      </div>

      <!-- 传输层 / TLS / 高级：各自可折叠，含条件显示的选项块 -->
      <div v-for="section in sections" :key="section.key" class="border-t border-slate-100 dark:border-slate-800 pt-3">
        <template v-if="sectionFields(section.key).length > 0">
          <button
            type="button"
            @click="openSections[section.key] = !openSections[section.key]"
            class="flex items-center gap-1.5 text-xs font-semibold text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 transition-all"
          >
            <ChevronDownOutline v-if="openSections[section.key]" class="w-3.5 h-3.5" />
            <ChevronForwardOutline v-else class="w-3.5 h-3.5" />
            <FieldLabel :text="t(section.titleKey)" :english="englishText(section.titleKey)" /> ({{ sectionFields(section.key).length }})
          </button>
          <div v-if="openSections[section.key]" class="grid grid-cols-1 sm:grid-cols-2 gap-3 mt-3">
            <NodeFieldInput
              v-for="field in sectionFields(section.key)"
              :key="field.key"
              :field="field"
              :protocol-type="form.protocolType"
              v-model="form.values[field.key]"
            />
          </div>
        </template>
      </div>

      <p v-if="missingRequired.length > 0" class="text-xs text-danger leading-normal">
        {{ t('subscription.node_required_hint', { fields: missingRequired.join(' / ') }) }}
      </p>
      <p v-if="missingRequireAny.length > 0" class="text-xs text-danger leading-normal">
        {{ t('subscription.node_require_any_hint', { groups: missingRequireAny.join(' / ') }) }}
      </p>

      <p class="text-xs text-slate-400 dark:text-slate-500 leading-normal flex items-start gap-1.5">
        <InformationCircleOutline class="w-3.5 h-3.5 shrink-0 mt-0.5" />
        {{ t('subscription.node_modal_hint') }}
      </p>
    </div>

    <div class="flex justify-end gap-2.5 pt-4 border-t border-slate-100 dark:border-slate-800 shrink-0">
      <button @click="emit('close')" class="px-4 py-2 text-xs font-semibold rounded-lg bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 transition-all">
        {{ t('subscription.cancel') }}
      </button>
      <button
        @click="handleSave"
        :disabled="!!nameError || missingRequired.length > 0 || missingRequireAny.length > 0"
        class="px-4 py-2 text-xs font-semibold rounded-lg bg-accent hover:bg-accent-hover text-white transition-all shadow-md shadow-accent/15 disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {{ t('subscription.save_to_list') }}
      </button>
    </div>
  </AppModal>
</template>
