<template><div ref="el" :style="{ height }" /></template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts/core'
import { BarChart, LineChart, PieChart } from 'echarts/charts'
import { DataZoomComponent, GridComponent, LegendComponent, MarkLineComponent, TitleComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { EChartsCoreOption } from 'echarts/core'

echarts.use([BarChart, LineChart, PieChart, GridComponent, LegendComponent, TitleComponent, TooltipComponent, DataZoomComponent, MarkLineComponent, CanvasRenderer])

const props = withDefaults(defineProps<{ option: EChartsCoreOption; height?: string }>(), { height: '300px' })
const el = ref<HTMLDivElement>()
let chart: echarts.ECharts | undefined
let ro: ResizeObserver | undefined

onMounted(() => {
  chart = echarts.init(el.value!)
  chart.setOption(props.option)
  ro = new ResizeObserver(() => chart?.resize())
  ro.observe(el.value!)
})
watch(() => props.option, (o) => chart?.setOption(o, true), { deep: true })
onBeforeUnmount(() => { ro?.disconnect(); chart?.dispose() })
</script>
