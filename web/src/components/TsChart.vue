<template>
  <ChartBox v-if="hasData" :option="option" :height="height" />
  <EmptyState v-else text="所选时间范围内暂无数据" hint="调整时间范围或筛选条件" :height="height" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import ChartBox from './ChartBox.vue'
import EmptyState from './EmptyState.vue'
import { buildTsOption, type MetricKey } from '../charts'
import type { SeriesResult } from '../types'

const props = withDefaults(defineProps<{
  result: SeriesResult | null
  metric: MetricKey
  names: Record<string, string>
  height?: string
  area?: boolean
}>(), { height: '260px' })

const hasData = computed(() => !!props.result && props.result.series.some((s) => s.points.length > 0))
const option = computed(() => (props.result ? buildTsOption({ result: props.result, metric: props.metric, names: props.names, area: props.area }) : {}))
</script>
