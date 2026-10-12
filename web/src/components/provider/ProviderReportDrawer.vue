<template>
  <el-drawer :model-value="visible" size="960px" destroy-on-close @update:model-value="emit('update:visible', $event)">
    <template #header>
      <div class="dhead">
        <VendorLogo v-if="rep" :code="rep.provider.code" :size="28" />
        <div>
          <div class="dtitle">测试报告 · {{ rep?.provider.name ?? '' }}</div>
          <div v-if="rep" class="dsub"><ProtocolTag :provider="rep.provider" /> <span class="mono">{{ rep.provider.base_url }}</span></div>
        </div>
      </div>
    </template>

    <div class="bar">
      <el-radio-group v-model="days" size="small" @change="load"><el-radio-button :value="7">近 7 天</el-radio-button><el-radio-button :value="14">近 14 天</el-radio-button><el-radio-button :value="30">近 30 天</el-radio-button></el-radio-group>
      <span class="spacer" />
      <span v-if="rep" class="muted">生成于 {{ fmtTime(rep.generated_at) }}</span>
      <el-button size="small" :icon="Download" :disabled="!rep" @click="download">下载报告</el-button>
      <el-button v-if="isSuper && rep" size="small" type="primary" :icon="VideoPlay" @click="emit('test', rep.provider)">立即测试</el-button>
    </div>

    <div v-loading="loading" class="body">
      <template v-if="rep">
        <EmptyState v-if="!rep.runs" text="所选时间范围内还没有测试记录" hint="点击「立即测试」发起第一次测试，或在供应商测试页开启定时巡检" height="260px" />
        <template v-else>
          <!-- conclusion first -->
          <div class="verdict" :class="verdict.tone">
            <b>{{ verdict.title }}</b><span>{{ verdict.text }}</span>
          </div>

          <div class="kpis">
            <KpiCard label="可用率" :value="rep.availability < 0 ? '—' : pct(rep.availability)" :hint="`${rep.passed} / ${rep.checks} 项通过`" :danger="rep.availability >= 0 && rep.availability < 0.95" />
            <KpiCard label="平均耗时" :value="rep.avg_latency_ms ? fmtDuration(rep.avg_latency_ms) : '—'" :hint="`P50 ${rep.p50_latency_ms ? fmtDuration(rep.p50_latency_ms) : '—'}`" />
            <KpiCard label="P95 耗时" :value="rep.p95_latency_ms ? fmtDuration(rep.p95_latency_ms) : '—'" hint="模型调用成功的请求" />
            <KpiCard label="测试次数" :value="String(rep.runs)" :hint="`连通性 ${rep.probe_passed} 通过 / ${rep.probe_failed} 失败`" />
          </div>

          <div class="charts">
            <div class="chart">
              <div class="ctitle">每日可用率</div>
              <ChartBox :option="availOption" height="180px" />
            </div>
            <div class="chart">
              <div class="ctitle">每日平均耗时</div>
              <ChartBox :option="latencyOption" height="180px" />
            </div>
          </div>

          <div class="grid2">
            <section>
              <h4>模型可用性 <span class="muted">（可用率低的排在前面）</span></h4>
              <el-table :data="rep.models" size="small" max-height="320" empty-text="没有模型调用记录">
                <el-table-column label="模型" min-width="170" show-overflow-tooltip><template #default="{ row }"><span class="mono">{{ row.model_key }}</span></template></el-table-column>
                <el-table-column label="可用率" width="124">
                  <template #default="{ row }">
                    <div class="avail"><div class="track"><div class="fill" :class="tone(row.availability)" :style="{ width: `${Math.max(row.availability, 0) * 100}%` }" /></div><span class="num">{{ row.availability < 0 ? '—' : pct(row.availability) }}</span></div>
                  </template>
                </el-table-column>
                <el-table-column label="平均 / P95" width="156" align="right"><template #default="{ row }"><span class="num">{{ row.avg_latency_ms ? `${fmtDuration(row.avg_latency_ms)} / ${fmtDuration(row.p95_latency_ms)}` : '—' }}</span></template></el-table-column>
                <el-table-column label="最近错误" min-width="160" show-overflow-tooltip><template #default="{ row }"><span :class="row.last_error ? 'err' : 'faint'">{{ row.last_error || '—' }}</span></template></el-table-column>
              </el-table>
            </section>
            <section>
              <h4>失败原因分布</h4>
              <RankBars v-if="rep.errors.length" :rows="rep.errors.map((e) => ({ name: e.label, value: e.count }))" :fmt="(v) => `${v} 次`" series-name="失败次数" :color="C.danger" height="220px" integer />
              <EmptyState v-else text="没有失败记录" height="220px" />
            </section>
          </div>

          <section>
            <h4>测试记录</h4>
            <el-table :data="rep.recent_runs" size="small" max-height="300" class="runs" @row-click="openRun">
              <el-table-column label="时间" width="170"><template #default="{ row }"><span class="num">{{ fmtTime(row.started_at) }}</span></template></el-table-column>
              <el-table-column label="方式" width="90"><template #default="{ row }">{{ TEST_TRIGGER[row.trigger] ?? row.trigger }}</template></el-table-column>
              <el-table-column label="结果" width="112"><template #default="{ row }"><StatusBadge :text="TEST_RUN_STATUS[row.status]?.text ?? row.status" :tone="TEST_RUN_STATUS[row.status]?.tone ?? 'info'" /></template></el-table-column>
              <el-table-column label="摘要" min-width="260" show-overflow-tooltip prop="summary" />
              <el-table-column label="操作人" width="110" prop="operator" />
            </el-table>
            <div class="note">点击一条记录查看逐项明细。</div>
          </section>
        </template>
      </template>
    </div>

    <el-dialog v-model="runVisible" :title="`测试明细 #${runDetail?.id ?? ''}`" width="860px" append-to-body>
      <div v-if="runDetail" class="rsum"><StatusBadge :text="TEST_RUN_STATUS[runDetail.status]?.text ?? runDetail.status" :tone="TEST_RUN_STATUS[runDetail.status]?.tone ?? 'info'" /><span>{{ runDetail.summary }}</span></div>
      <TestResultTable v-if="runDetail?.results" :results="runDetail.results" max-height="460" />
    </el-dialog>
  </el-drawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Download, VideoPlay } from '@element-plus/icons-vue'
import ChartBox from '../ChartBox.vue'
import EmptyState from '../EmptyState.vue'
import KpiCard from '../KpiCard.vue'
import RankBars from '../RankBars.vue'
import StatusBadge from '../StatusBadge.vue'
import VendorLogo from '../VendorLogo.vue'
import ProtocolTag from './ProtocolTag.vue'
import TestResultTable from './TestResultTable.vue'
import { providerTests } from '../../api'
import { useAuth } from '../../composables/useAuth'
import { C, tipRow, tooltipBase, xAxis, yAxis } from '../../charts'
import { TEST_CHECK, TEST_ERROR, TEST_RUN_STATUS, TEST_TRIGGER } from '../../constants'
import { fmtDuration, fmtTime } from '../../utils'
import type { ProviderTestReport, ProviderTestRun } from '../../types'

const props = defineProps<{ visible: boolean; providerId: number | null }>()
const emit = defineEmits<{ 'update:visible': [boolean]; test: [ProviderTestReport['provider']] }>()
const { isSuper } = useAuth()

const days = ref(7)
const rep = ref<ProviderTestReport | null>(null)
const loading = ref(false)
const runVisible = ref(false)
const runDetail = ref<ProviderTestRun | null>(null)

const pct = (v: number) => `${(v * 100).toFixed(v === 1 ? 0 : 1)}%`
const tone = (v: number) => (v < 0 ? 'none' : v >= 0.99 ? 'good' : v >= 0.9 ? 'warn' : 'bad')

async function load() {
  if (!props.providerId) return
  loading.value = true
  try { rep.value = await providerTests.report(props.providerId, days.value) } finally { loading.value = false }
}
watch(() => [props.visible, props.providerId], ([v]) => { if (v) { rep.value = null; load() } }, { immediate: true })
defineExpose({ reload: load })

async function openRun(row: ProviderTestRun) {
  runDetail.value = await providerTests.run(row.id)
  runVisible.value = true
}

// one-line conclusion: what the reader should do
const verdict = computed(() => {
  const r = rep.value
  if (!r) return { tone: 'good', title: '', text: '' }
  const last = r.last_run
  const weak = r.models.filter((m) => m.availability >= 0 && m.availability < 0.9).map((m) => m.model_key)
  if (last?.status === 'failed') {
    return { tone: 'bad', title: '最近一次测试失败', text: `${fmtTime(last.started_at)}：${last.summary}。请检查 Base URL、凭证与供应商服务状态。` }
  }
  if (r.availability >= 0 && r.availability < 0.95 || weak.length) {
    return { tone: 'warn', title: '存在不稳定的模型', text: weak.length ? `可用率低于 90%：${weak.slice(0, 5).join('、')}${weak.length > 5 ? ` 等 ${weak.length} 个` : ''}。建议在模型调度中为其配置备用供应商。` : `近 ${r.days} 天可用率 ${pct(r.availability)}，低于 95%。` }
  }
  return { tone: 'good', title: '供应商运行良好', text: `近 ${r.days} 天 ${r.runs} 次测试，可用率 ${r.availability < 0 ? '—' : pct(r.availability)}，平均耗时 ${r.avg_latency_ms ? fmtDuration(r.avg_latency_ms) : '—'}。` }
})

const dayLabels = computed(() => (rep.value?.daily ?? []).map((d) => d.date.slice(5)))
const availOption = computed(() => {
  const daily = rep.value?.daily ?? []
  return {
    grid: { left: 44, right: 12, top: 12, bottom: 24 },
    tooltip: { ...tooltipBase, trigger: 'axis', axisPointer: { type: 'shadow' },
      formatter: (ps: { dataIndex: number }[]) => {
        const d = daily[ps[0].dataIndex]
        return `<div style="margin-bottom:4px">${d.date}</div>` + (d.availability < 0 ? '<span style="color:#6B7280">无测试</span>'
          : tipRow(C.primary, '可用率', pct(d.availability)) + tipRow('#9CA3AF', '通过 / 执行', `${d.passed} / ${d.checks}`))
      } },
    xAxis: xAxis(dayLabels.value),
    yAxis: { ...yAxis((v: number) => `${Math.round(v * 100)}%`), min: 0, max: 1 },
    series: [{ type: 'bar', name: '可用率', barMaxWidth: 18, data: daily.map((d) => (d.availability < 0 ? null : d.availability)),
      itemStyle: { color: C.primary, borderRadius: [4, 4, 0, 0] } }],
  }
})
const latencyOption = computed(() => {
  const daily = rep.value?.daily ?? []
  return {
    grid: { left: 52, right: 12, top: 12, bottom: 24 },
    tooltip: { ...tooltipBase, trigger: 'axis',
      formatter: (ps: { dataIndex: number }[]) => {
        const d = daily[ps[0].dataIndex]
        return `<div style="margin-bottom:4px">${d.date}</div>` + (d.avg_latency_ms ? tipRow(C.accent, '平均耗时', fmtDuration(d.avg_latency_ms)) : '<span style="color:#6B7280">无成功调用</span>')
      } },
    xAxis: xAxis(dayLabels.value),
    yAxis: yAxis((v: number) => (v >= 1000 ? `${(v / 1000).toFixed(1)}s` : `${v}ms`)),
    series: [{ type: 'line', name: '平均耗时', connectNulls: false, symbol: 'circle', symbolSize: 6, showSymbol: true,
      lineStyle: { width: 2, color: C.accent }, itemStyle: { color: C.accent, borderColor: '#fff', borderWidth: 2 },
      data: daily.map((d) => d.avg_latency_ms || null) }],
  }
})

// CSV report: summary + per-model table + run history (opens in Excel)
function download() {
  const r = rep.value
  if (!r) return
  const esc = (v: unknown) => {
    let s = String(v ?? '')
    if (/^[=+\-@]/.test(s)) s = `'${s}`
    return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
  }
  const rows: unknown[][] = [
    ['供应商测试报告', r.provider.name, r.provider.code],
    ['统计范围', `近 ${r.days} 天`, '生成时间', fmtTime(r.generated_at)],
    ['测试次数', r.runs, '执行检查', r.checks, '通过', r.passed, '失败', r.failed],
    ['可用率', r.availability < 0 ? '' : pct(r.availability), '平均耗时(ms)', r.avg_latency_ms, 'P50(ms)', r.p50_latency_ms, 'P95(ms)', r.p95_latency_ms],
    [],
    ['模型', '检查类型', '执行', '通过', '失败', '可用率', '平均耗时(ms)', 'P95(ms)', '最近错误'],
    ...r.models.map((m) => [m.model_key, TEST_CHECK[m.check_type] ?? m.check_type, m.checks, m.passed, m.failed, m.availability < 0 ? '' : pct(m.availability), m.avg_latency_ms, m.p95_latency_ms, m.last_error]),
    [],
    ['失败原因', '次数'],
    ...r.errors.map((e) => [TEST_ERROR[e.type] ?? e.label, e.count]),
    [],
    ['测试时间', '方式', '结果', '通过', '失败', '跳过', '平均耗时(ms)', 'P95(ms)', '摘要', '操作人'],
    ...r.recent_runs.map((x) => [fmtTime(x.started_at), TEST_TRIGGER[x.trigger] ?? x.trigger, TEST_RUN_STATUS[x.status]?.text ?? x.status, x.passed, x.failed, x.skipped, x.avg_latency_ms, x.p95_latency_ms, x.summary, x.operator]),
  ]
  const csv = '﻿' + rows.map((row) => row.map(esc).join(',')).join('\n')
  const a = document.createElement('a')
  a.href = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' }))
  a.download = `供应商测试报告_${r.provider.code}_${new Date().toISOString().slice(0, 10)}.csv`
  a.click()
  URL.revokeObjectURL(a.href)
}
</script>

<style scoped>
.dhead { display: flex; align-items: center; gap: 10px; }
.dtitle { font-size: 15px; font-weight: 600; color: var(--text); }
.dsub { display: flex; align-items: center; gap: 8px; font-size: 12px; color: var(--text-2); font-weight: 400; margin-top: 2px; }
.bar { display: flex; align-items: center; gap: 10px; margin-bottom: 14px; }
.spacer { flex: 1; }
.body { min-height: 300px; }
.verdict { display: flex; gap: 10px; align-items: baseline; padding: 10px 14px; border-radius: var(--radius-sm); margin-bottom: 14px; font-size: 13px; border: 1px solid; }
.verdict b { font-weight: 600; white-space: nowrap; }
.verdict span { color: var(--text-2); }
.verdict.good { background: var(--success-soft); border-color: #BBF7D0; }
.verdict.good b { color: #15803D; }
.verdict.warn { background: var(--warning-soft); border-color: #FDE68A; }
.verdict.warn b { color: #B45309; }
.verdict.bad { background: var(--danger-soft); border-color: #FECACA; }
.verdict.bad b { color: #B91C1C; }
.kpis { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin-bottom: 14px; }
.charts { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin-bottom: 14px; }
.chart { border: 1px solid var(--border); border-radius: var(--radius); padding: 10px 12px 4px; }
.ctitle { font-size: 12.5px; font-weight: 600; color: var(--text); }
.grid2 { display: grid; grid-template-columns: 3fr 2fr; gap: 16px; margin-bottom: 14px; }
h4 { margin: 0 0 8px; font-size: 13px; font-weight: 600; }
h4 .muted { font-weight: 400; font-size: 12px; }
.avail { display: flex; align-items: center; gap: 8px; }
.track { flex: 1; height: 6px; border-radius: 3px; background: #EEF0F3; overflow: hidden; }
.fill { height: 100%; border-radius: 3px; }
.fill.good { background: #16A34A; }
.fill.warn { background: #F59E0B; }
.fill.bad { background: #DC2626; }
.avail .num { width: 44px; text-align: right; font-size: 12px; }
.err { color: var(--danger); font-size: 12px; }
.runs :deep(tr) { cursor: pointer; }
.note { font-size: 12px; color: var(--text-3); margin-top: 6px; }
.rsum { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; color: var(--text-2); }
</style>
