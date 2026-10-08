<template>
  <div>
    <PageHeader title="模型监控" desc="基于模型维度的分钟级调用、用量与性能监控；采样粒度随时间范围自动调整" />

    <Panel class="filters">
      <div class="toolbar">
        <label class="f">厂商
          <el-select v-model="vendorId" clearable filterable placeholder="全部厂商" style="width: 140px" @change="modelId = undefined"><el-option v-for="v in state.vendors" :key="v.id" :label="v.name" :value="v.id" /></el-select>
        </label>
        <label class="f">供应商
          <el-select v-model="providerId" clearable filterable placeholder="全部供应商" style="width: 160px" @change="modelId = undefined"><el-option v-for="p in state.providers" :key="p.id" :label="p.name" :value="p.id" /></el-select>
        </label>
        <label class="f">模型名称
          <el-select v-model="modelId" clearable filterable placeholder="全部模型" style="width: 240px"><el-option v-for="m in modelOptions" :key="m.id" :label="m.display_name" :value="m.id" /></el-select>
        </label>
        <label class="f">选择时间<TimeRange ref="trRef" v-model="range" /></label>
        <el-button type="primary" :icon="Search" :loading="loading" @click="query">查询</el-button>
      </div>
    </Panel>

    <div class="row kpis4">
      <KpiCard label="总调用量" :value="fmtNum(kpi.requests)" :series="spark('requests')" :hint="`采样 ${stepText}`" />
      <KpiCard label="失败率" :value="kpi.failRate.toFixed(2)" unit="%" :danger="kpi.failRate > 5" color="#DC2626" :series="spark('failure_rate')" :hint="`失败 ${fmtNum(kpi.failed)} 次`" />
      <KpiCard label="Token 消耗" :value="compact(kpi.tokens)" :series="spark('tokens')" color="#7C3AED" :hint="`峰值 TPM ${compact(kpi.peakTpm)}`" />
      <KpiCard label="平均 RT" :value="fmtDuration(kpi.rt)" color="#6B7280" :series="spark('rt')" hint="按调用量加权" />
    </div>

    <Panel flush>
      <el-tabs v-model="tab" class="tabs"><el-tab-pane label="调用监控" name="call" /><el-tab-pane label="性能监控" name="perf" /></el-tabs>
      <div v-loading="loading" class="charts">
        <template v-if="tab === 'call'">
          <section><h4>调用量<span class="muted">次 / {{ stepText }}</span></h4><TsChart :result="result" metric="requests" :names="names" height="250px" /></section>
          <section><h4>用量（Tokens）<span class="muted">总量 / {{ stepText }}</span></h4><TsChart :result="result" metric="tokens" :names="names" height="250px" /></section>
          <section><h4>失败率</h4><TsChart :result="result" metric="failure_rate" :names="names" height="250px" /></section>
        </template>
        <template v-else>
          <section><h4>每分钟 Token 数（TPM）</h4><TsChart :result="result" metric="tpm" :names="names" height="250px" /></section>
          <section><h4>每分钟调用次数（RPM）</h4><TsChart :result="result" metric="rpm" :names="names" height="250px" /></section>
          <section><h4>平均响应时间（RT）</h4><TsChart :result="result" metric="rt" :names="names" height="250px" /></section>
        </template>
      </div>
    </Panel>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Search } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import KpiCard from '../../components/KpiCard.vue'
import TimeRange from '../../components/TimeRange.vue'
import TsChart from '../../components/TsChart.vue'
import { monitor } from '../../api'
import { METRICS, compact, type MetricKey } from '../../charts'
import { useLookups } from '../../composables/useLookups'
import { defaultRange, fmtDateTime, fmtDuration, fmtNum } from '../../utils'
import type { SeriesResult } from '../../types'

const { state, modelLabel } = useLookups()
const providerId = ref<number>()
const modelId = ref<number>()
const range = ref(defaultRange('3h'))
const trRef = ref<InstanceType<typeof TimeRange>>()
const tab = ref('call')
const loading = ref(false)
const result = ref<SeriesResult | null>(null)

const vendorId = ref<number>()
const modelOptions = computed(() => state.models.filter((m) => (!providerId.value || m.provider_id === providerId.value) && (!vendorId.value || m.vendor_id === vendorId.value)))
const names = computed(() => Object.fromEntries(state.models.map((m) => [String(m.id), modelLabel(m.id)])))
const stepText = computed(() => { const s = result.value?.step_seconds ?? 60; return s >= 3600 ? `${s / 3600} 小时` : `${s / 60} 分钟` })

async function query() {
  const r = trRef.value?.current() ?? range.value
  loading.value = true
  try {
    result.value = await monitor.series({
      group_by: 'model', from: fmtDateTime(r.from), to: fmtDateTime(r.to),
      provider_id: providerId.value, vendor_id: vendorId.value, model_id: modelId.value,
    })
  } finally { loading.value = false }
}
onMounted(query)

const kpi = computed(() => {
  const step = (result.value?.step_seconds ?? 60) / 60
  let requests = 0, failed = 0, tokens = 0, lat = 0, peakTpm = 0
  const perTs = new Map<string, number>()
  for (const s of result.value?.series ?? []) for (const p of s.points) {
    requests += p.requests; failed += p.failed; tokens += p.total_tokens; lat += p.latency_sum_ms
    perTs.set(p.ts, (perTs.get(p.ts) ?? 0) + p.total_tokens)
  }
  for (const v of perTs.values()) peakTpm = Math.max(peakTpm, v / step)
  return { requests, failed, tokens, peakTpm, failRate: requests ? (failed / requests) * 100 : 0, rt: requests ? lat / requests : 0 }
})

// aggregate across series into one sparkline
function spark(metric: MetricKey): number[] {
  const step = (result.value?.step_seconds ?? 60) / 60
  const agg = new Map<string, { requests: number; failed: number; tokens: number; lat: number }>()
  for (const s of result.value?.series ?? []) for (const p of s.points) {
    const a = agg.get(p.ts) ?? { requests: 0, failed: 0, tokens: 0, lat: 0 }
    a.requests += p.requests; a.failed += p.failed; a.tokens += p.total_tokens; a.lat += p.latency_sum_ms
    agg.set(p.ts, a)
  }
  const def = METRICS[metric]
  return [...agg.entries()].sort((a, b) => +new Date(a[0]) - +new Date(b[0])).map(([ts, a]) => def.value(
    { ts, requests: a.requests, failed: a.failed, prompt_tokens: 0, completion_tokens: 0, total_tokens: a.tokens, latency_sum_ms: a.lat, cost: '0' }, step) ?? 0)
}
</script>

<style scoped>
.filters { margin-bottom: 16px; }
.f { display: inline-flex; align-items: center; gap: 8px; font-size: 13px; color: var(--text-2); }
.row.kpis4 { grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
@media (max-width: 960px) { .row.kpis4 { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
.tabs { padding: 0 16px; }
.tabs :deep(.el-tabs__header) { margin: 0; }
.tabs :deep(.el-tabs__nav-wrap::after) { height: 1px; background: var(--border-soft); }
.charts { padding: 8px 16px 16px; display: grid; gap: 4px; min-height: 200px; }
h4 { margin: 14px 0 4px; font-size: 13.5px; font-weight: 600; display: flex; align-items: baseline; gap: 8px; }
h4 .muted { font-size: 12px; font-weight: 400; }
</style>
