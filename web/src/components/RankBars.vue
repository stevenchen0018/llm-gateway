<template>
  <ChartBox v-if="rows.length" :option="option" :height="height" />
  <EmptyState v-else text="暂无数据" :height="height" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import ChartBox from './ChartBox.vue'
import EmptyState from './EmptyState.vue'
import { buildRankOption } from '../charts'

const props = withDefaults(defineProps<{
  rows: { name: string; value: number }[]
  fmt: (v: number) => string
  seriesName: string
  color?: string
  height?: string
}>(), { height: '300px' })

const option = computed(() => buildRankOption({ rows: props.rows, fmt: props.fmt, seriesName: props.seriesName, color: props.color }))
</script>
