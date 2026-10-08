<template>
  <div>
    <PageHeader title="成本大盘" desc="成本感知：按部门 / 应用 / Key 类型 / Key / 模型 / 厂商 / 供应商 / 类别拆解成本，追踪供应商折扣节省，并向使用人及 +1 主管推送周报">
      <el-date-picker v-model="dates" type="daterange" range-separator="→" start-placeholder="开始" end-placeholder="结束" value-format="YYYY-MM-DD" :clearable="false" :shortcuts="shortcuts" style="width: 250px" @change="load" />
      <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
    </PageHeader>

    <div class="row kpis4">
      <KpiCard label="总成本（折后）" :value="fmtMoney(kpi.cost)" :series="dailyTotals" color="#7C3AED" :hint="`调用 ${fmtNum(kpi.requests)} 次`" />
      <KpiCard label="供应商折扣节省" :value="fmtMoney(kpi.saved)" :hint="`标价 ${fmtMoney(kpi.list)}，折后为标价的 ${kpi.list ? ((kpi.cost / kpi.list) * 100).toFixed(1) : '100'}%`" />
      <KpiCard label="均价 / 百万 Token" :value="fmtMoney(kpi.perM)" :hint="`共 ${compact(kpi.tokens)} tokens`" />
      <KpiCard label="日均成本" :value="fmtMoney(kpi.cost / Math.max(days.length, 1))" :hint="`${days.length} 天周期`" />
    </div>

    <Panel title="每日成本" sub="按模型类别堆叠" class="mb">
      <ChartBox v-if="trend.days.length" :option="trendOption" height="260px" />
      <EmptyState v-else text="所选周期内暂无成本数据" height="260px" />
    </Panel>

    <Panel flush class="mb">
      <el-tabs v-model="dim" class="tabs" @tab-change="loadDim">
        <el-tab-pane v-for="d in DIMS" :key="d.value" :label="d.label" :name="d.value" />
      </el-tabs>
      <div class="dimgrid" v-loading="dimLoading">
        <div class="left"><div class="ctitle">成本 Top 10</div><RankBars :rows="topRows" :fmt="(v) => fmtMoney(v)" series-name="成本" :color="C.accent" height="320px" /></div>
        <el-table :data="overview" size="small" max-height="360" empty-text="暂无数据">
          <el-table-column :label="dimLabel" min-width="170" show-overflow-tooltip><template #default="{ row }">{{ nameOf(row.group_key) }}</template></el-table-column>
          <el-table-column label="调用量" width="90" align="right" sortable prop="requests"><template #default="{ row }"><span class="num">{{ fmtNum(row.requests) }}</span></template></el-table-column>
          <el-table-column label="Token" width="90" align="right" sortable prop="tokens"><template #default="{ row }"><span class="num">{{ compact(row.tokens) }}</span></template></el-table-column>
          <el-table-column label="折扣节省" width="100" align="right"><template #default="{ row }"><span class="num" :style="{ color: row.saved > 0 ? 'var(--success)' : undefined }">{{ row.saved > 0 ? fmtMoney(row.saved) : '—' }}</span></template></el-table-column>
          <el-table-column label="均价/百万" width="100" align="right" sortable prop="price_per_m"><template #default="{ row }"><span class="num">{{ fmtMoney(row.price_per_m) }}</span></template></el-table-column>
          <el-table-column label="成本（折后）" width="170" align="right" sortable prop="cost">
            <template #default="{ row }"><div class="costcell"><span class="bar"><i :style="{ width: row.share_of_cost + '%' }" /></span><span class="num">{{ fmtMoney(row.cost) }}</span></div></template>
          </el-table-column>
        </el-table>
      </div>
    </Panel>

    <Panel title="用量周报推送" sub="每周一 09:00 自动向使用人及 +1 主管推送近 7 日用量与成本（飞书 Webhook）" flush>
      <template #extra>
        <el-select v-model="digestDays" style="width: 110px" @change="loadDigest"><el-option :value="7" label="近 7 天" /><el-option :value="14" label="近 14 天" /><el-option :value="30" label="近 30 天" /></el-select>
        <el-button v-if="isSuper" type="primary" :icon="Promotion" :loading="sending" @click="send">立即推送</el-button>
      </template>
      <el-table :data="digest" v-loading="digestLoading" empty-text="暂无可推送的数据">
        <el-table-column label="使用人" min-width="120"><template #default="{ row }"><div class="cell-title">{{ row.owner }}</div><div class="cell-sub">{{ row.owner_email || '—' }}</div></template></el-table-column>
        <el-table-column label="+1 主管" min-width="120"><template #default="{ row }"><div>{{ row.manager || '—' }}</div><div class="cell-sub">{{ row.manager_email }}</div></template></el-table-column>
        <el-table-column label="应用" min-width="150" show-overflow-tooltip><template #default="{ row }">{{ row.apps.join('、') || '—' }}</template></el-table-column>
        <el-table-column label="Key" width="70" align="right"><template #default="{ row }"><span class="num">{{ row.keys }}</span></template></el-table-column>
        <el-table-column label="调用量" width="110" align="right"><template #default="{ row }"><span class="num">{{ fmtNum(row.requests) }}</span></template></el-table-column>
        <el-table-column label="Token" width="90" align="right"><template #default="{ row }"><span class="num">{{ compact(row.tokens) }}</span></template></el-table-column>
        <el-table-column label="成本" width="120" align="right"><template #default="{ row }"><span class="num">{{ fmtMoney(row.cost) }}</span></template></el-table-column>
        <el-table-column label="环比" width="100" align="right">
          <template #default="{ row }"><span v-if="row.prev_cost > 0" class="num" :style="{ color: row.change_pct > 10 ? 'var(--danger)' : row.change_pct < -10 ? 'var(--success)' : undefined }">{{ row.change_pct > 0 ? '+' : '' }}{{ row.change_pct.toFixed(1) }}%</span><span v-else class="faint">—</span></template>
        </el-table-column>
        <el-table-column label="最高成本 Key" min-width="150" show-overflow-tooltip prop="top_key" />
      </el-table>
    </Panel>
  </div>
</template>

<script setup lang="ts">
import { useAuth } from '../../composables/useAuth'
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Promotion, Refresh } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import KpiCard from '../../components/KpiCard.vue'
import ChartBox from '../../components/ChartBox.vue'
import EmptyState from '../../components/EmptyState.vue'
import RankBars from '../../components/RankBars.vue'
import { cost as costApi, monitor } from '../../api'
import { C, MAX_SERIES, OTHER_COLOR, OTHER_KEY, assignColors, compact, tipRow, tooltipBase, xAxis, yAxis } from '../../charts'
import { categoryLabel, KEY_CATEGORY } from '../../constants'
import { useLookups } from '../../composables/useLookups'
import { fmtDate, fmtMoney, fmtNum } from '../../utils'
import type { DigestItem, OverviewRow, SeriesResult } from '../../types'

const { isSuper } = useAuth()

const DIMS = [
  { value: 'department', label: '部门' }, { value: 'key_category', label: 'Key 类型' }, { value: 'app', label: '应用' }, { value: 'key', label: 'API Key' },
  { value: 'model', label: '模型' }, { value: 'vendor', label: '厂商' }, { value: 'provider', label: '供应商' }, { value: 'category', label: '模型类别' },
]
const { appName, keyName, modelLabel, providerName, deptName, vendorName } = useLookups()

const today = new Date()
const ago = (n: number) => { const d = new Date(); d.setDate(d.getDate() - n); return fmtDate(d) }
const dates = ref<[string, string]>([ago(6), fmtDate(today)])
const shortcuts = [
  { text: '近 7 天', value: () => [new Date(Date.now() - 6 * 864e5), today] },
  { text: '近 14 天', value: () => [new Date(Date.now() - 13 * 864e5), today] },
  { text: '近 30 天', value: () => [new Date(Date.now() - 29 * 864e5), today] },
]
const range = () => { const to = new Date(dates.value[1]); to.setDate(to.getDate() + 1); return { from: dates.value[0], to: fmtDate(to) } }

const loading = ref(false)
const byProvider = ref<OverviewRow[]>([])
const trendRes = ref<SeriesResult | null>(null)

const kpi = computed(() => {
  const r = byProvider.value
  const cost = r.reduce((s, x) => s + x.cost, 0), list = r.reduce((s, x) => s + x.list_cost, 0)
  const tokens = r.reduce((s, x) => s + x.tokens, 0), requests = r.reduce((s, x) => s + x.requests, 0)
  return { cost, list, saved: list - cost, tokens, requests, perM: tokens ? (cost / tokens) * 1e6 : 0 }
})

// ---- daily stacked trend by category (hourly buckets folded into local days) ----------
const days = computed(() => {
  const out: string[] = []
  const d = new Date(dates.value[0]); const end = new Date(dates.value[1])
  for (let i = 0; d <= end && i < 120; i++) { out.push(fmtDate(d)); d.setDate(d.getDate() + 1) }
  return out
})
const trend = computed(() => {
  const totals = new Map<string, number[]>()
  for (const s of trendRes.value?.series ?? []) {
    const arr = days.value.map(() => 0)
    for (const p of s.points) { const i = days.value.indexOf(fmtDate(new Date(p.ts))); if (i >= 0) arr[i] += Number(p.cost) }
    totals.set(s.group_key, arr)
  }
  const ranked = [...totals].sort((a, b) => b[1].reduce((x, y) => x + y, 0) - a[1].reduce((x, y) => x + y, 0))
  const head = ranked.slice(0, MAX_SERIES - 1), tail = ranked.slice(MAX_SERIES - 1)
  const series = head.map(([k, v]) => ({ key: k, data: v }))
  if (tail.length) series.push({ key: OTHER_KEY, data: days.value.map((_, i) => tail.reduce((s, [, v]) => s + v[i], 0)) })
  return { days: series.length ? days.value : [], series }
})
const dailyTotals = computed(() => trend.value.days.map((_, i) => trend.value.series.reduce((s, x) => s + x.data[i], 0)))

const trendOption = computed(() => {
  const colors = assignColors(trend.value.series.filter((s) => s.key !== OTHER_KEY).map((s) => s.key))
  const col = (k: string) => (k === OTHER_KEY ? OTHER_COLOR : colors[k])
  const nm = (k: string) => (k === OTHER_KEY ? '其他' : categoryLabel(k))
  return {
    grid: { left: 58, right: 16, top: 36, bottom: 26 },
    legend: { type: 'scroll' as const, top: 0, left: 0, itemWidth: 10, itemHeight: 10, textStyle: { color: C.text2, fontSize: 11 } },
    tooltip: { ...tooltipBase, trigger: 'axis' as const, axisPointer: { type: 'shadow' as const, shadowStyle: { color: 'rgba(17,24,39,.04)' } },
      formatter: (ps: { seriesName: string; value: number; color: string; axisValue: string }[]) =>
        `<div style="color:${C.text2};margin-bottom:4px">${ps[0].axisValue}</div>` + ps.filter((p) => p.value > 0).sort((a, b) => b.value - a.value).map((p) => tipRow(p.color, p.seriesName, fmtMoney(p.value))).join('') },
    xAxis: { ...xAxis(trend.value.days), axisLabel: { ...xAxis([]).axisLabel, formatter: (v: string) => v.slice(5) } },
    yAxis: yAxis((v) => '¥' + compact(v)),
    series: trend.value.series.map((s, i, arr) => ({
      name: nm(s.key), type: 'bar' as const, stack: 'cost', data: s.data.map((v) => Number(v.toFixed(4))), barMaxWidth: 30,
      itemStyle: { color: col(s.key), borderColor: C.surface, borderWidth: 1.5, borderRadius: i === arr.length - 1 ? [4, 4, 0, 0] : 0 },
    })),
  }
})

// ---- dimension breakdown ---------------------------------------------------------------
const dim = ref(isSuper.value ? 'department' : 'app')
const dimLoading = ref(false)
const overview = ref<OverviewRow[]>([])
const dimLabel = computed(() => DIMS.find((d) => d.value === dim.value)!.label)
const nameOf = (k: string) => {
  switch (dim.value) {
    case 'app': return k === '0' ? '未归属应用' : appName(Number(k))
    case 'key': return keyName(Number(k))
    case 'model': return modelLabel(Number(k))
    case 'provider': return providerName(Number(k))
    case 'vendor': return k === '0' ? '未归属厂商' : vendorName(Number(k))
    case 'key_category': return KEY_CATEGORY[k]?.text ?? k
    case 'department': return k === '0' ? '未归属部门' : deptName(Number(k))
    default: return categoryLabel(k)
  }
}
const topRows = computed(() => overview.value.slice(0, 10).map((r) => ({ name: nameOf(r.group_key), value: r.cost })))

async function loadDim() {
  dimLoading.value = true
  try { overview.value = await costApi.overview({ ...range(), group_by: dim.value }) } finally { dimLoading.value = false }
}

async function load() {
  loading.value = true
  try {
    const r = range()
    const [prov, trendRes_] = await Promise.all([
      costApi.overview({ ...r, group_by: 'provider' }),
      monitor.series({ ...r, group_by: 'category', step: 3600 }),
    ])
    byProvider.value = prov
    trendRes.value = trendRes_
    await loadDim()
  } finally { loading.value = false }
}

// ---- weekly digest --------------------------------------------------------------------------
const digest = ref<DigestItem[]>([])
const digestDays = ref(7)
const digestLoading = ref(false)
const sending = ref(false)
async function loadDigest() { digestLoading.value = true; try { digest.value = await costApi.digest(digestDays.value) } finally { digestLoading.value = false } }
async function send() {
  sending.value = true
  try { const r = await costApi.sendDigest(digestDays.value); ElMessage.success(`已推送 ${r.sent} 份周报（含 +1 主管），可在告警中心查看记录`) } finally { sending.value = false }
}

onMounted(() => { load(); loadDigest() })
</script>

<style scoped>
.row.kpis4 { grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
@media (max-width: 960px) { .row.kpis4 { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
.mb { margin-bottom: 16px; }
.tabs { padding: 0 16px; }
.tabs :deep(.el-tabs__header) { margin: 0; }
.tabs :deep(.el-tabs__nav-wrap::after) { height: 1px; background: var(--border-soft); }
.dimgrid { display: grid; grid-template-columns: minmax(0, 3fr) minmax(0, 5fr); min-height: 300px; }
@media (max-width: 1100px) { .dimgrid { grid-template-columns: 1fr; } }
.left { padding: 12px 16px; border-right: 1px solid var(--border-soft); }
.ctitle { font-size: 13px; font-weight: 600; margin-bottom: 4px; }
.costcell { display: flex; align-items: center; justify-content: flex-end; gap: 8px; }
.bar { width: 44px; height: 4px; border-radius: 2px; background: #EEF0F3; overflow: hidden; flex: none; }
.bar i { display: block; height: 100%; background: var(--accent); }
</style>
