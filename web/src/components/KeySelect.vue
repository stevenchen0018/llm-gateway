<template>
  <el-select
    :model-value="modelValue ?? undefined"
    filterable
    remote
    :remote-method="onSearch"
    :loading="loading"
    :clearable="clearable"
    :placeholder="placeholder"
    :style="{ width }"
    popper-class="key-select-pop"
    remote-show-suffix
    @visible-change="onVisible"
    @update:model-value="onPick"
  >
    <el-option v-if="special" :value="special.value" :label="special.label" />
    <el-option v-for="k in options" :key="k.id" :value="k.id" :label="labelOf(k)" :disabled="excludeIds?.includes(k.id)">
      <div class="opt">
        <span class="name">{{ k.name }}</span>
        <span class="meta">
          <span v-if="k.key_prefix.startsWith('sk-')" class="mono">{{ k.key_prefix }}…</span>
          <span v-if="k.app_id">{{ appName(k.app_id) }}</span>
          <span v-if="k.status !== 'active'" class="st" :class="k.status">{{ STATUS[k.status] }}</span>
        </span>
      </div>
    </el-option>
    <template #footer>
      <div class="foot">
        <span v-if="total > options.length">共 {{ total }} 个，已显示 {{ options.length }} 个 · 输入名称 / 负责人 / 前缀 / 应用 / #ID 缩小范围</span>
        <span v-else>共 {{ total }} 个</span>
      </div>
    </template>
  </el-select>
</template>

<script setup lang="ts">
// KeySelect: a server-searched API key picker. It never loads every key: it
// fetches the first page on open and queries the backend as the user types,
// so it stays fast with tens of thousands of keys.
import { ref, watch } from 'vue'
import { keys as keysApi } from '../api'
import { useLookups } from '../composables/useLookups'
import type { ApiKey, KeyStatus } from '../types'

const props = withDefaults(defineProps<{
  modelValue?: number | null
  status?: string // e.g. "active" or "active,pending"
  keyType?: 'formal' | 'trial'
  category?: 'application' | 'personal'
  placeholder?: string
  width?: string
  clearable?: boolean
  special?: { value: number; label: string } // an extra leading option, e.g. 0 = 全局
  excludeIds?: number[] // shown but not selectable
}>(), { placeholder: '搜索 API Key', width: '220px', clearable: true })
const emit = defineEmits<{ 'update:modelValue': [v: number | undefined]; change: [v: number | undefined, key?: ApiKey] }>()

const STATUS: Record<KeyStatus, string> = { pending: '待审批', active: '生效中', blacklisted: '已拉黑' }
const { appName } = useLookups()
const PAGE = 30
const options = ref<ApiKey[]>([])
const total = ref(0)
const loading = ref(false)
let seq = 0
let timer: ReturnType<typeof setTimeout> | undefined

const labelOf = (k: ApiKey) => k.name

async function fetch(q: string) {
  const my = ++seq
  loading.value = true
  try {
    const res = await keysApi.search({ q, status: props.status, key_type: props.keyType, category: props.category, page_size: PAGE })
    if (my !== seq) return // a newer query is in flight
    options.value = withSelected(res.items)
    total.value = res.total
  } finally { if (my === seq) loading.value = false }
}

// keep the selected key in the option list so its label always renders
const selected = ref<ApiKey | undefined>()
function withSelected(list: ApiKey[]) {
  const s = selected.value
  return s && !list.some((k) => k.id === s.id) ? [s, ...list] : list
}

function onSearch(q: string) {
  clearTimeout(timer)
  timer = setTimeout(() => fetch(q.trim()), 250)
}
function onVisible(open: boolean) { if (open && !options.value.length) fetch('') }
function onPick(raw: number | '' | undefined) {
  const v = typeof raw === 'number' ? raw : undefined // clearing yields '' or undefined
  const k = options.value.find((o) => o.id === v)
  selected.value = k
  emit('update:modelValue', v)
  emit('change', v, k)
}

// a category switch invalidates the loaded options
watch(() => props.category, () => { options.value = []; total.value = 0 })

// resolve a preset value (e.g. from a route query) to its label
watch(() => props.modelValue, async (v) => {
  if (!v || options.value.some((k) => k.id === v)) return
  const res = await keysApi.search({ ids: String(v), page_size: 1 })
  if (res.items[0]) { selected.value = res.items[0]; options.value = withSelected(options.value) }
}, { immediate: true })
</script>

<style scoped>
.opt { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-width: 0; }
.name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.meta { display: flex; gap: 8px; flex: none; font-size: 12px; color: var(--text-3); }
.st { font-size: 11px; padding: 0 5px; border-radius: 4px; background: #F3F4F6; }
.st.pending { color: #B45309; background: #FEF3C7; }
.st.blacklisted { color: #B91C1C; background: #FEE2E2; }
.foot { font-size: 12px; color: var(--text-3); }
</style>

<style>
.key-select-pop .el-select-dropdown__item { height: auto; min-height: 34px; line-height: 34px; }
.key-select-pop { min-width: 380px !important; }
</style>
