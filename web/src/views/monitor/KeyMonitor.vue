<template>
  <div>
    <PageHeader title="Key 监控" desc="基于 API Key 的调用 / 用量 / 性能分时监控，序列按模型拆分；协助定位限流、异常与成本来源">
      <label class="auto muted"><el-switch v-model="auto" size="small" /> 每分钟自动刷新</label>
      <el-button :icon="Refresh" :loading="loading" @click="query">刷新</el-button>
    </PageHeader>

    <Panel class="filters">
      <div class="toolbar">
        <label class="f">API Key
          <el-radio-group v-model="keyCat" size="default" @change="() => { keyId = undefined; query() }"><el-radio-button value="">全部</el-radio-button><el-radio-button value="application">应用 Key</el-radio-button><el-radio-button value="personal">个人编码 Key</el-radio-button></el-radio-group>
          <KeySelect v-model="keyId" status="active,blacklisted" :category="keyCat || undefined" placeholder="全部 Key" width="240px" @change="query" />
        </label>
        <label class="f">时间范围<TimeRange ref="trRef" v-model="range" @change="query" /></label>
      </div>
    </Panel>

    <div class="row kpis5">
      <KpiCard label="总调用量" :value="fmtNum(kpi.requests)" :series="spark('requests')" :hint="`采样 ${stepText}`" />
      <KpiCard label="失败率" :value="kpi.failRate.toFixed(2)" unit="%" :danger="kpi.failRate > 5" color="#DC2626" :series="spark('failure_rate')" :hint="`失败 ${fmtNum(kpi.failed)} 次`" />
      <KpiCard label="Token 消耗" :value="compact(kpi.tokens)" color="#7C3AED" :series="spark('tokens')" :hint="`峰值 TPM ${compact(kpi.peakTpm)}`" />
      <KpiCard label="峰值 RPM" :value="fmtNum(Math.round(kpi.peakRpm))" :series="spark('rpm')" hint="单分钟最大调用" />
      <KpiCard label="平均 RT" :value="fmtDuration(kpi.rt)" color="#6B7280" :series="spark('rt')" hint="端到端，按调用量加权" />
    </div>

    <div v-loading="loading">
      <div class="row cols-2">
        <Panel title="每分钟调用次数（RPM）" sub="RPM/TPM 监控分时（基于 Key）"><TsChart :result="result" metric="rpm" :names="names" height="250px" /></Panel>
        <Panel title="每分钟消耗 Token 数（TPM）"><TsChart :result="result" metric="tpm" :names="names" height="250px" /></Panel>
      </div>
      <div class="row"><Panel title="平均响应时间（RT）" sub="端到端平均 RT 监控分时（基于 Key）"><TsChart :result="result" metric="rt" :names="names" height="260px" /></Panel></div>
      <div class="row cols-2">
        <Panel title="用量（Tokens）趋势" sub="调用 / 用量监控分时（基于 Key）">
          <template #extra><el-radio-group v-model="tokenMetric" size="small"><el-radio-button value="tokens">总量</el-radio-button><el-radio-button value="tokens_prompt">输入</el-radio-button><el-radio-button value="tokens_completion">输出</el-radio-button></el-radio-group></template>
          <TsChart :result="result" :metric="tokenMetric" :names="names" height="250px" />
        </Panel>
        <Panel title="调用次数趋势">
          <template #extra><el-radio-group v-model="callMetric" size="small"><el-radio-button value="requests">总调用量</el-radio-button><el-radio-button value="failed">失败数量</el-radio-button><el-radio-button value="failure_rate">失败率</el-radio-button></el-radio-group></template>
          <TsChart :result="result" :metric="callMetric" :names="names" height="250px" />
        </Panel>
      </div>
      <div class="row cols-2">
        <Panel title="调用失败率" sub="调用失败率和平均 RT（基于 Key）· 滚动鼠标查看更多"><RankBars :rows="failRows" :fmt="(v) => v.toFixed(2) + '%'" series-name="失败率" :color="C.danger" height="300px" /></Panel>
        <Panel title="平均响应时间（RT）"><RankBars :rows="rtRows" :fmt="fmtDuration" series-name="平均 RT" height="300px" /></Panel>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import KeySelect from '../../components/KeySelect.vue'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import KpiCard from '../../components/KpiCard.vue'
import TimeRange from '../../components/TimeRange.vue'
import TsChart from '../../components/TsChart.vue'
import RankBars from '../../components/RankBars.vue'
import { monitor } from '../../api'
import { C, METRICS, compact, type MetricKey } from '../../charts'
import { useLookups } from '../../composables/useLookups'
import { defaultRange, fmtDateTime, fmtDuration, fmtNum } from '../../utils'
import type { RankRow, SeriesResult } from '../../types'

const { state, appName, modelLabel } = useLookups()
const keyId = ref<number>()
const keyCat = ref<'' | 'application' | 'personal'>('')
const range = ref(defaultRange('3h'))
const trRef = ref<InstanceType<typeof TimeRange>>()
const loading = ref(false)
const auto = ref(false)
const result = ref<SeriesResult | null>(null)
const ranking = ref<RankRow[]>([])
const tokenMetric = ref<MetricKey>('tokens')
const callMetric = ref<MetricKey>('requests')

const usableKeys = computed(() => state.keys.filter((k) => k.status !== 'pending'))
const names = computed(() => Object.fromEntries(state.models.map((m) => [String(m.id), modelLabel(m.id)])))
const stepText = computed(() => { const s = result.value?.step_seconds ?? 60; return s >= 3600 ? `${s / 3600} 小时` : `${s / 60} 分钟` })

async function query() {
  const r = trRef.value?.current() ?? range.value
  const p = { from: fmtDateTime(r.from), to: fmtDateTime(r.to), key_id: keyId.value, key_category: keyCat.value || undefined }
  loading.value = true
  try {
    const [s, rk] = await Promise.all([monitor.series({ ...p, group_by: 'model' }), monitor.ranking({ ...p, group_by: 'model' })])
    result.value = s
    ranking.value = rk
  } finally { loading.value = false }
}

// default to the busiest active key once the key list is available
watch(() => state.loaded, (loaded) => { if (loaded && keyId.value === undefined) { keyId.value = usableKeys.value.find((k) => k.status === 'active')?.id; query() } }, { immediate: true })
let timer: number | undefined
watch(auto, (on) => { clearInterval(timer); if (on) timer = window.setInterval(query, 60_000) })
onBeforeUnmount(() => clearInterval(timer))

const nameOf = (k: string) => names.value[k] ?? `#${k}`
const failRows = computed(() => ranking.value.filter((r) => r.requests > 0).map((r) => ({ name: nameOf(r.group_key), value: r.failure_rate })))
const rtRows = computed(() => ranking.value.filter((r) => r.requests > 0).map((r) => ({ name: nameOf(r.group_key), value: r.avg_latency_ms })))

const kpi = computed(() => {
  const step = (result.value?.step_seconds ?? 60) / 60
  let requests = 0, failed = 0, tokens = 0, lat = 0, peakTpm = 0, peakRpm = 0
  const perTs = new Map<string, { req: number; tok: number }>()
  for (const s of result.value?.series ?? []) for (const p of s.points) {
    requests += p.requests; failed += p.failed; tokens += p.total_tokens; lat += p.latency_sum_ms
    const a = perTs.get(p.ts) ?? { req: 0, tok: 0 }
    a.req += p.requests; a.tok += p.total_tokens
    perTs.set(p.ts, a)
  }
  for (const v of perTs.values()) { peakTpm = Math.max(peakTpm, v.tok / step); peakRpm = Math.max(peakRpm, v.req / step) }
  return { requests, failed, tokens, peakTpm, peakRpm, failRate: requests ? (failed / requests) * 100 : 0, rt: requests ? lat / requests : 0 }
})

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
.auto { display: inline-flex; align-items: center; gap: 6px; font-size: 13px; }
.row.kpis5 { grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 12px; }
@media (max-width: 1180px) { .row.kpis5 { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
</style>
