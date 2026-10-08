<template>
  <div class="tr toolbar">
    <el-radio-group :model-value="preset" @change="pick">
      <el-radio-button v-for="p in PRESETS" :key="p.value" :value="p.value">{{ p.label }}</el-radio-button>
      <el-radio-button value="custom">自定义</el-radio-button>
    </el-radio-group>
    <el-date-picker v-if="preset === 'custom'" v-model="custom" type="datetimerange" range-separator="→" start-placeholder="开始时间" end-placeholder="结束时间"
      :clearable="false" style="width: 360px" @change="applyCustom" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

import type { Range } from '../utils'

const PRESETS = [
  { value: '1h', label: '1 小时', ms: 3600e3 },
  { value: '3h', label: '3 小时', ms: 3 * 3600e3 },
  { value: '6h', label: '6 小时', ms: 6 * 3600e3 },
  { value: '24h', label: '24 小时', ms: 24 * 3600e3 },
  { value: '7d', label: '7 天', ms: 7 * 86400e3 },
]

const props = defineProps<{ modelValue: Range }>()
const emit = defineEmits<{ 'update:modelValue': [v: Range]; change: [v: Range] }>()

const preset = ref(props.modelValue.preset)
const custom = ref<[Date, Date]>([props.modelValue.from, props.modelValue.to])

function emitRange(r: Range) {
  emit('update:modelValue', r)
  emit('change', r)
}

function pick(v: string | number | boolean | undefined) {
  const value = String(v)
  preset.value = value
  if (value === 'custom') return
  const p = PRESETS.find((x) => x.value === value)!
  const to = new Date()
  emitRange({ preset: value, from: new Date(to.getTime() - p.ms), to })
}

function applyCustom(v: [Date, Date]) {
  if (v) emitRange({ preset: 'custom', from: v[0], to: v[1] })
}

/** Re-anchor a relative preset to "now" (used by refresh buttons / auto refresh). */
function current(): Range {
  const p = PRESETS.find((x) => x.value === preset.value)
  if (!p) return props.modelValue
  const to = new Date()
  return { preset: preset.value, from: new Date(to.getTime() - p.ms), to }
}
defineExpose({ current })
</script>
