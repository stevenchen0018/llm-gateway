<template>
  <div class="frow">
    <span class="lab">{{ label }}</span>
    <div class="chips">
      <button type="button" class="chip" :class="{ on: !modelValue.length }" @click="set([])">全部</button>
      <button v-for="v in visible" :key="v.id" type="button" class="chip" :class="{ on: modelValue.includes(v.id) }" @click="toggle(v.id)">
        <VendorLogo :code="v.code" :size="16" />{{ v.name }}<span class="n">{{ v.count }}</span>
      </button>
      <el-popover v-if="vendors.length > topN" v-model:visible="open" placement="bottom-start" :width="480" trigger="click" popper-class="vendor-pop">
        <template #reference>
          <button type="button" class="chip more" :class="{ on: hiddenSelected > 0 }">
            更多{{ label.replace('模型', '') }}<span class="n">{{ vendors.length - topN }}</span><el-icon :size="12"><ArrowDown /></el-icon>
          </button>
        </template>
        <div class="pop">
          <el-input v-model="kw" :prefix-icon="Search" :placeholder="`搜索${label.replace('模型', '')}名称 / 编码`" clearable size="small" />
          <div class="groups">
            <template v-for="g in grouped" :key="g.label">
              <div class="glabel">{{ g.label }}<span>{{ g.items.length }}</span></div>
              <label v-for="v in g.items" :key="v.id" class="item" :class="{ off: v.status !== 'active' }">
                <el-checkbox :model-value="modelValue.includes(v.id)" @change="toggle(v.id)" />
                <VendorLogo :code="v.code" :size="18" />
                <span class="nm">{{ v.name }}</span>
                <span v-if="v.status !== 'active'" class="tag">停用</span>
                <span class="n">{{ v.count }}</span>
              </label>
            </template>
            <div v-if="!grouped.length" class="empty">没有匹配的项</div>
          </div>
          <div class="foot">
            <span>已选 {{ modelValue.length }} 家</span>
            <span><el-button link size="small" @click="set([])">清空</el-button><el-button link type="primary" size="small" @click="open = false">完成</el-button></span>
          </div>
        </div>
      </el-popover>
    </div>
  </div>
</template>

<script setup lang="ts">
// VendorFilter: the marketplace vendor facet built for dozens of vendors — the
// busiest vendors stay one click away as chips, the long tail lives in a
// searchable, grouped, multi-select popover, and chosen long-tail vendors are
// promoted into the chip row so the active filter is always visible.
import { computed, ref } from 'vue'
import { ArrowDown, Search } from '@element-plus/icons-vue'
import VendorLogo from './VendorLogo.vue'
import type { Provider } from '../types'

type Item = Pick<Provider, 'id' | 'code' | 'name' | 'status'>
const props = withDefaults(defineProps<{ modelValue: number[]; items: Item[]; counts: Map<number, number>; topN?: number; label?: string }>(), { topN: 8, label: '模型厂商' })
const emit = defineEmits<{ 'update:modelValue': [v: number[]] }>()

const open = ref(false)
const kw = ref('')

// vendors with models first, busiest first; empty vendors sink to the end
const vendors = computed(() => props.items
  .map((p) => ({ ...p, count: props.counts.get(p.id) ?? 0 }))
  .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name, 'zh')))

const top = computed(() => vendors.value.slice(0, props.topN))
const visible = computed(() => {
  const ids = new Set(top.value.map((v) => v.id))
  return [...top.value, ...vendors.value.filter((v) => !ids.has(v.id) && props.modelValue.includes(v.id))]
})
const hiddenSelected = computed(() => props.modelValue.filter((id) => !top.value.some((v) => v.id === id)).length)

const grouped = computed(() => {
  const k = kw.value.trim().toLowerCase()
  const match = vendors.value.filter((v) => !k || `${v.name} ${v.code}`.toLowerCase().includes(k))
  const groups = [
    { label: `常用${props.label.replace('模型', '')}（模型最多）`, items: match.filter((v) => top.value.some((t) => t.id === v.id)) },
    { label: `其他${props.label.replace('模型', '')}`, items: match.filter((v) => !top.value.some((t) => t.id === v.id) && v.count > 0) },
    { label: '暂无模型', items: match.filter((v) => !top.value.some((t) => t.id === v.id) && v.count === 0) },
  ]
  return groups.filter((g) => g.items.length)
})

function set(v: number[]) { emit('update:modelValue', v) }
function toggle(id: number) {
  set(props.modelValue.includes(id) ? props.modelValue.filter((x) => x !== id) : [...props.modelValue, id])
}
</script>

<style scoped>
.frow { display: flex; align-items: flex-start; gap: 12px; padding: 5px 0; }
.lab { flex: none; width: 64px; padding-top: 4px; font-size: 12px; color: var(--text-2); }
.chips { display: flex; flex-wrap: wrap; gap: 4px 6px; }
.chip { display: inline-flex; align-items: center; gap: 6px; height: 26px; padding: 0 10px; border: 1px solid transparent; border-radius: 6px; background: transparent; font: inherit; font-size: 13px; color: var(--text); cursor: pointer; transition: background .12s; }
.chip:hover { background: #F3F4F6; }
.chip.on { background: var(--primary-soft); color: var(--primary); font-weight: 500; }
.chip .n { font-size: 11.5px; color: var(--text-3); font-variant-numeric: tabular-nums; }
.chip.on .n { color: var(--primary); opacity: .75; }
.chip.more { border-color: var(--border); color: var(--text-2); }
.pop { display: grid; gap: 8px; }
.groups { max-height: 320px; overflow-y: auto; display: grid; grid-template-columns: 1fr 1fr; gap: 2px 10px; align-content: start; }
.glabel { grid-column: 1 / -1; margin-top: 6px; font-size: 11.5px; color: var(--text-3); display: flex; gap: 6px; }
.item { display: flex; align-items: center; gap: 8px; height: 32px; padding: 0 6px; border-radius: 6px; cursor: pointer; font-size: 13px; min-width: 0; }
.item:hover { background: #F9FAFB; }
.item.off .nm { color: var(--text-3); }
.item .nm { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.item .n { font-size: 12px; color: var(--text-3); font-variant-numeric: tabular-nums; }
.item .tag { font-size: 11px; color: var(--text-3); border: 1px solid var(--border); border-radius: 4px; padding: 0 4px; }
.empty { grid-column: 1 / -1; padding: 16px 0; text-align: center; color: var(--text-3); font-size: 12.5px; }
.foot { display: flex; justify-content: space-between; align-items: center; padding-top: 8px; border-top: 1px solid var(--border-soft); font-size: 12px; color: var(--text-2); }
</style>
