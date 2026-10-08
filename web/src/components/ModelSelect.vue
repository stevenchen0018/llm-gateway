<template>
  <el-select :model-value="modelValue" filterable placeholder="选择模型" style="width: 100%" @update:model-value="(v: number) => $emit('update:modelValue', v)">
    <el-option-group v-for="g in groups" :key="g.vendor" :label="g.vendor">
      <el-option v-for="m in g.models" :key="m.id" :label="`${m.display_name} · ${m.provider?.name ?? ''}`" :value="m.id">
        <span>{{ m.display_name }}</span><span class="faint" style="float: right; font-size: 12px">{{ m.provider?.name }} · {{ categoryLabel(m.category) }}</span>
      </el-option>
    </el-option-group>
  </el-select>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import { categoryLabel } from '../constants'
import { useLookups } from '../composables/useLookups'

const props = defineProps<{ modelValue?: number; categories: string[] }>()
const emit = defineEmits<{ 'update:modelValue': [v: number | undefined] }>()

const { state } = useLookups()
const route = useRoute()

const candidates = computed(() => state.models.filter((m) => m.status === 'active' && props.categories.includes(m.category)))
const groups = computed(() => {
  const by = new Map<string, typeof candidates.value>()
  // grouped by vendor (厂商); each option names its supplier (供应商)
  for (const m of candidates.value) { const k = m.vendor?.name ?? '其他'; by.set(k, [...(by.get(k) ?? []), m]) }
  return [...by].map(([vendor, models]) => ({ vendor, models }))
})

// preselect ?model=<id> (from the marketplace) or the first available model
watch(candidates, (list) => {
  if (props.modelValue && list.some((m) => m.id === props.modelValue)) return
  const q = Number(route.query.model)
  emit('update:modelValue', list.find((m) => m.id === q)?.id ?? list[0]?.id)
}, { immediate: true })
</script>
