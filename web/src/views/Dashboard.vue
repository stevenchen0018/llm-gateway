<template>
  <div>
    <PageHeader title="用量与成本大盘" desc="调用量、Token、成本与稳定性总览，支持按 Key / 模型 / 供应商下钻">
      <el-radio-group v-model="groupBy" @change="load">
        <el-radio-button value="model">模型</el-radio-button>
        <el-radio-button value="key">Key</el-radio-button>
        <el-radio-button value="provider">供应商</el-radio-button>
      </el-radio-group>
      <el-date-picker v-model="range" type="daterange" range-separator="→" start-placeholder="开始" end-placeholder="结束" value-format="YYYY-MM-DD" :clearable="false" :shortcuts="shortcuts" style="width: 250px" @change="load" />
      <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
    </PageHeader>

    <div class="row kpis">
      <KpiCard label="总调用量" :value="fmtNum(kpi.requests)" :series="daily.map((d) => d.req)" :hint="`日均 ${fmtNum(Math.round(kpi.requests / Math.max(days.length, 1)))}`" />
      <KpiCard label="失败率" :value="kpi.failRate.toFixed(2)" unit="%" :danger="kpi.failRate > 5" :series="daily.map((d) => (d.req ? (d.failed / d.req) * 100 : 0))" color="#DC2626" :hint="`失败 ${fmtNum(kpi.failed)} 次`" />
      <KpiCard label="Token 消耗" :value="compact(kpi.tokens)" :series="daily.map((d) => d.tokens)" :hint="fmtNum(kpi.tokens) + ' tokens'" />
      <KpiCard label="总成本" :value="fmtMoney(kpi.cost)" :series="daily.map((d) => d.cost)" color="#7C3AED" :hint="`${fmtMoney(kpi.costPerM)} / 百万 Token`" />
      <KpiCard label="平均耗时" :value="fmtNum(Math.round(kpi.latency))" unit="ms" :series="daily.map((d) => d.lat)" color="#6B7280" hint="按调用量加权" />
    </div>

    <div class="row cols-2">
      <Panel title="调用量" :sub="`每日 · ${range[0]} ~ ${range[1]}`">
        <ChartBox v-if="hasData" :option="requestsOption" height="220px" />
        <EmptyState v-else text="所选时间范围内暂无调用" hint="调整日期范围或发起一次网关调用" height="220px" />
      </Panel>
      <Panel title="成本" :sub="`每日 · ${range[0]} ~ ${range[1]}`">
        <ChartBox v-if="hasData" :option="costOption" height="220px" />
        <EmptyState v-else text="所选时间范围内暂无成本" height="220px" />
      </Panel>
    </div>

    <div class="row cols-5-3">
      <Panel :title="`${groupLabel}维度明细`" :sub="`${groups.length} 项 · 按成本降序`" flush>
        <el-table :data="groups" size="small" empty-text="暂无数据" max-height="420">
          <el-table-column prop="name" :label="groupLabel" min-width="150" show-overflow-tooltip />
          <el-table-column label="调用量" prop="requests" align="right" sortable width="100"><template #default="{ row }"><span class="num">{{ fmtNum(row.requests) }}</span></template></el-table-column>
          <el-table-column label="失败率" align="right" sortable prop="failRate" width="90">
            <template #default="{ row }"><span class="num" :style="{ color: row.failRate > 5 ? 'var(--danger)' : undefined }">{{ row.failRate.toFixed(2) }}%</span></template>
          </el-table-column>
          <el-table-column label="Token" prop="tokens" align="right" sortable width="100"><template #default="{ row }"><span class="num">{{ compact(row.tokens) }}</span></template></el-table-column>
          <el-table-column label="耗时" align="right" width="90"><template #default="{ row }"><span class="num">{{ fmtNum(Math.round(row.latency)) }} ms</span></template></el-table-column>
          <el-table-column label="成本" prop="cost" align="right" sortable min-width="170">
            <template #default="{ row }">
              <div class="costcell"><span class="bar"><i :style="{ width: (kpi.cost ? (row.cost / kpi.cost) * 100 : 0) + '%' }" /></span><span class="num">{{ fmtMoney(row.cost) }}</span></div>
            </template>
          </el-table-column>
        </el-table>
      </Panel>

      <Panel title="最近告警" flush>
        <template #extra><el-button link type="primary" @click="$router.push('/alerts')">查看全部</el-button></template>
        <EmptyState v-if="!alerts.length" text="暂无告警" height="160px" />
        <ul v-else class="alerts">
          <li v-for="a in alerts" :key="a.id">
            <StatusBadge :text="levelText[a.level]" :tone="levelTone[a.level]" />
            <div class="body"><div class="msg">{{ a.message }}</div><div class="faint t">{{ typeText[a.type] }} · {{ fmtTime(a.created_at) }}</div></div>
          </li>
        </ul>
      </Panel>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import PageHeader from '../components/PageHeader.vue'
import Panel from '../components/Panel.vue'
import KpiCard from '../components/KpiCard.vue'
import ChartBox from '../components/ChartBox.vue'
import EmptyState from '../components/EmptyState.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { dashboard, keys, models, providers } from '../api'
import type { AlertEvent, UsageSummary } from '../types'
import { fmtDate, fmtMoney, fmtNum, fmtTime } from '../utils'
import { C, compact, tipRow, tooltipBase, xAxis, yAxis } from '../charts'

const groupBy = ref<'model' | 'key' | 'provider'>('model')
const today = new Date()
const daysAgo = (n: number) => { const d = new Date(); d.setDate(d.getDate() - n); return fmtDate(d) }
const range = ref<[string, string]>([daysAgo(6), fmtDate(today)])
const shortcuts = [
  { text: '今天', value: () => [today, today] },
  { text: '近 7 天', value: () => [new Date(Date.now() - 6 * 864e5), today] },
  { text: '近 30 天', value: () => [new Date(Date.now() - 29 * 864e5), today] },
]

const loading = ref(false)
const rows = ref<UsageSummary[]>([])
const alerts = ref<AlertEvent[]>([])
const names = ref<Record<string, Record<string, string>>>({ model: {}, key: {}, provider: {} })

const levelText = { info: '提示', warning: '警告', critical: '严重' } as const
const levelTone = { info: 'info', warning: 'warning', critical: 'danger' } as const
const typeText = { quota: '限流', budget: '预算', failover: '容灾', report: '周报', security: '安全' } as const
const groupLabel = computed(() => ({ model: '模型', key: 'Key', provider: '供应商' })[groupBy.value])
const nameOf = (k: string) => names.value[groupBy.value][k] || `#${k}`

async function load() {
  loading.value = true
  try {
    const to = new Date(range.value[1]); to.setDate(to.getDate() + 1) // backend range is [from, to)
    const [usage, ms, ks, ps, al] = await Promise.all([
      dashboard.usage({ from: range.value[0], to: fmtDate(to), group_by: groupBy.value }),
      models.list(), keys.list(), providers.list(), dashboard.alerts(6),
    ])
    rows.value = usage
    alerts.value = al
    names.value = {
      model: Object.fromEntries(ms.map((m) => [m.id, m.display_name])),
      key: Object.fromEntries(ks.map((k) => [k.id, k.name])),
      provider: Object.fromEntries(ps.map((p) => [p.id, p.name])),
    }
  } finally { loading.value = false }
}
onMounted(load)

const sum = (f: (r: UsageSummary) => number) => rows.value.reduce((s, r) => s + f(r), 0)
const hasData = computed(() => rows.value.length > 0)

const kpi = computed(() => {
  const requests = sum((r) => r.request_count)
  const failed = sum((r) => r.failed_count)
  const tokens = sum((r) => r.total_tokens)
  const cost = sum((r) => Number(r.cost))
  const latency = requests ? sum((r) => r.avg_latency_ms * r.request_count) / requests : 0
  return { requests, failed, tokens, cost, latency, failRate: requests ? (failed / requests) * 100 : 0, costPerM: tokens ? (cost / tokens) * 1e6 : 0 }
})

// continuous day axis: days without traffic show as zero rather than disappearing
const days = computed(() => {
  const out: string[] = []
  const d = new Date(range.value[0]); const end = new Date(range.value[1])
  for (let i = 0; d <= end && i < 400; i++) { out.push(fmtDate(d)); d.setDate(d.getDate() + 1) }
  return out
})
const daily = computed(() => {
  const m = new Map<string, { req: number; failed: number; tokens: number; cost: number; lat: number }>()
  for (const r of rows.value) {
    const x = m.get(r.bucket) ?? { req: 0, failed: 0, tokens: 0, cost: 0, lat: 0 }
    x.req += r.request_count; x.failed += r.failed_count; x.tokens += r.total_tokens
    x.cost += Number(r.cost); x.lat += r.avg_latency_ms * r.request_count
    m.set(r.bucket, x)
  }
  return days.value.map((d) => { const x = m.get(d); return x ? { ...x, lat: x.req ? x.lat / x.req : 0 } : { req: 0, failed: 0, tokens: 0, cost: 0, lat: 0 } })
})

const groups = computed(() => {
  const m = new Map<string, { name: string; requests: number; failed: number; tokens: number; cost: number; lat: number }>()
  for (const r of rows.value) {
    const g = m.get(r.group_key) ?? { name: nameOf(r.group_key), requests: 0, failed: 0, tokens: 0, cost: 0, lat: 0 }
    g.requests += r.request_count; g.failed += r.failed_count; g.tokens += r.total_tokens
    g.cost += Number(r.cost); g.lat += r.avg_latency_ms * r.request_count
    m.set(r.group_key, g)
  }
  return [...m.values()]
    .map((g) => ({ ...g, failRate: g.requests ? (g.failed / g.requests) * 100 : 0, latency: g.requests ? g.lat / g.requests : 0 }))
    .sort((a, b) => b.cost - a.cost)
})

const grid = { left: 44, right: 24, top: 12, bottom: 24 }
const dayLabel = (s: string) => s.slice(5)

const requestsOption = computed(() => ({
  grid,
  tooltip: { ...tooltipBase, trigger: 'axis', axisPointer: { type: 'shadow', shadowStyle: { color: 'rgba(37,99,235,.06)' } },
    formatter: (p: any) => `<div style="color:${C.text2};margin-bottom:4px">${p[0].axisValue}</div>` + tipRow(C.primary, '调用量', fmtNum(p[0].value)) },
  xAxis: { ...xAxis(days.value), axisLabel: { ...xAxis([]).axisLabel, formatter: dayLabel } },
  yAxis: yAxis(),
  series: [{ type: 'bar', data: daily.value.map((d) => d.req), barMaxWidth: 20, itemStyle: { color: C.primary, borderRadius: [4, 4, 0, 0] }, emphasis: { itemStyle: { color: '#1D4ED8' } } }],
}))

const costOption = computed(() => ({
  grid: { ...grid, left: 52 },
  tooltip: { ...tooltipBase, trigger: 'axis', axisPointer: { type: 'line', lineStyle: { color: C.axis } },
    formatter: (p: any) => `<div style="color:${C.text2};margin-bottom:4px">${p[0].axisValue}</div>` + tipRow(C.accent, '成本', fmtMoney(p[0].value)) },
  xAxis: { ...xAxis(days.value), boundaryGap: false, axisLabel: { ...xAxis([]).axisLabel, formatter: dayLabel } },
  yAxis: yAxis((v) => '¥' + (v >= 100 ? v.toFixed(0) : String(Number(v.toFixed(v >= 1 ? 1 : 4))))),
  series: [{
    type: 'line', data: daily.value.map((d) => d.cost), lineStyle: { width: 2, color: C.accent }, itemStyle: { color: C.accent, borderColor: C.surface, borderWidth: 2 },
    showSymbol: days.value.length <= 2, symbolSize: 8, emphasis: { scale: 1.1 }, areaStyle: { color: C.accent, opacity: 0.07 },
  }],
}))
</script>

<style scoped>
.costcell { display: flex; align-items: center; justify-content: flex-end; gap: 10px; }
.bar { width: 56px; height: 4px; border-radius: 2px; background: #EEF0F3; overflow: hidden; flex: none; }
.bar i { display: block; height: 100%; background: var(--accent); border-radius: 2px; }
.alerts { list-style: none; margin: 0; padding: 0; }
.alerts li { display: flex; gap: 10px; align-items: flex-start; padding: 10px 16px; border-bottom: 1px solid var(--border-soft); }
.alerts li:last-child { border-bottom: none; }
.alerts li:hover { background: #F9FAFB; }
.body { min-width: 0; flex: 1; }
.msg { font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.t { font-size: 12px; margin-top: 1px; }
</style>
