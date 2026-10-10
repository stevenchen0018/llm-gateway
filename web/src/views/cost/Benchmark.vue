<template>
  <div>
    <PageHeader title="均价赛马" desc="按模型类别对比每百万 Token 均价（折后实测价优先），并给出同类模型切换与供应商折扣的降本建议">
      <el-radio-group v-model="days" @change="load"><el-radio-button :value="7">近 7 天</el-radio-button><el-radio-button :value="14">近 14 天</el-radio-button><el-radio-button :value="30">近 30 天</el-radio-button></el-radio-group>
      <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
    </PageHeader>

    <div class="row kpis4">
      <KpiCard label="可切换建议" :value="String(suggestions.length)" unit="条" hint="同类模型存在明显更低价格" />
      <KpiCard label="预计月度可省" :value="fmtMoney(totalMonthly)" color="#16A34A" hint="若全部切换到同类最低价模型" />
      <KpiCard label="折扣已节省" :value="fmtMoney(discountSaved)" :hint="`近 ${days} 天，供应商折扣`" />
      <KpiCard label="赛道数" :value="String(boards.length)" unit="类" :hint="`${modelCount} 个模型参赛`" />
    </div>

    <Panel flush class="mb">
      <el-tabs v-model="cat" class="tabs"><el-tab-pane v-for="b in boards" :key="b.category" :label="`${categoryLabel(b.category)}（${b.models.length}）`" :name="b.category" /></el-tabs>
      <div v-if="board" class="race" v-loading="loading">
        <div class="chart">
          <div class="ctitle">每百万 Token 均价<span class="muted">（深色为实测折后价，浅色为尚无调用的配置均价）</span></div>
          <ChartBox :option="raceOption" :height="`${Math.max(180, board.models.length * 34 + 50)}px`" />
        </div>
        <aside>
          <dl>
            <div><dt>类别均价</dt><dd class="num">{{ fmtMoney(board.avg_per_m) }}</dd></div>
            <div><dt>最低价</dt><dd class="num good">{{ fmtMoney(board.min_per_m) }}</dd></div>
            <div><dt>最高价</dt><dd class="num">{{ fmtMoney(board.max_per_m) }}</dd></div>
            <div><dt>价差倍数</dt><dd class="num">{{ board.min_per_m > 0 ? (board.max_per_m / board.min_per_m).toFixed(1) + '×' : '—' }}</dd></div>
            <div><dt>类别成本</dt><dd class="num">{{ fmtMoney(board.cost) }}</dd></div>
            <div><dt>类别 Token</dt><dd class="num">{{ compact(board.tokens) }}</dd></div>
          </dl>
          <p class="hint">「均价赛马」面向全域用户开放查看：让团队看清同类模型的真实价格差，推动切换到更低价的同类模型。</p>
        </aside>
      </div>
      <EmptyState v-else-if="!loading" text="暂无可对比的模型" />
    </Panel>

    <Panel title="模型切换建议" sub="同一类别内，存在价格更低且已上架的模型" flush class="mb">
      <el-table :data="suggestions" v-loading="loading" empty-text="暂无切换建议：当前各类别模型价格差距不大">
        <el-table-column label="当前模型" min-width="170"><template #default="{ row }"><div class="mcell"><VendorLogo :code="codeOf(row.from_model_id)" :size="22" /><div><div class="cell-title">{{ row.from_name }}</div><div class="cell-sub">{{ row.from_provider }}</div></div></div></template></el-table-column>
        <el-table-column width="44" align="center"><template #default><el-icon class="arrow"><Right /></el-icon></template></el-table-column>
        <el-table-column label="建议切换到" min-width="170"><template #default="{ row }"><div class="mcell"><VendorLogo :code="codeOf(row.to_model_id)" :size="22" /><div><div class="cell-title">{{ row.to_name }}</div><div class="cell-sub">{{ row.to_provider }}</div></div></div></template></el-table-column>
        <el-table-column label="类别" width="96"><template #default="{ row }"><StatusBadge :text="categoryLabel(row.category)" tone="primary" /></template></el-table-column>
        <el-table-column label="现价 → 目标（/百万）" width="170" align="right"><template #default="{ row }"><span class="num">{{ fmtMoney(row.from_per_m) }} → <b>{{ fmtMoney(row.to_per_m) }}</b></span></template></el-table-column>
        <el-table-column label="降幅" width="70" align="right"><template #default="{ row }"><span class="num good">-{{ row.saving_pct.toFixed(0) }}%</span></template></el-table-column>
        <el-table-column label="Token" width="90" align="right"><template #default="{ row }"><span class="num">{{ compact(row.tokens) }}</span></template></el-table-column>
        <el-table-column label="预计月度可省" width="120" align="right"><template #default="{ row }"><span class="num good">{{ fmtMoney(row.saving_monthly) }}</span></template></el-table-column>
        <el-table-column label="操作" width="120" align="right" fixed="right" class-name="col-actions"><template #default="{ row }"><el-button v-if="canWrite" link type="primary" @click="createPolicy(row)">创建切换策略</el-button></template></el-table-column>
      </el-table>
      <div class="note">切换前请先在「模型体验」对比效果；创建策略后可在「模型调度 · 路由模拟」中验证命中链路。</div>
    </Panel>

    <Panel title="供应商折扣" sub="折扣对接后，网关按「标价 × 折扣」核算成本，并在成本排序与切换建议中生效" flush>
      <el-table :data="state.providers" empty-text="暂无供应商">
        <el-table-column label="供应商" min-width="200"><template #default="{ row }"><div class="mcell"><VendorLogo :code="row.code" :size="24" /><div><div class="cell-title">{{ row.name }}</div><div class="cell-sub">{{ row.description || row.code }}</div></div></div></template></el-table-column>
        <el-table-column label="当前折扣" width="120"><template #default="{ row }"><StatusBadge :text="discountText(row.discount_rate)" :tone="Number(row.discount_rate) < 1 ? 'success' : 'info'" /></template></el-table-column>
        <el-table-column label="调整折扣率" width="190">
          <template #default="{ row }"><el-input-number :disabled="!isSuper" :model-value="draft(row)" :min="0.1" :max="1" :step="0.01" :precision="2" size="small" controls-position="right" @update:model-value="(v: number | undefined) => (edits[row.id] = v ?? 1)" /></template>
        </el-table-column>
        <el-table-column :label="`近 ${days} 天节省`" width="140" align="right"><template #default="{ row }"><span class="num good">{{ savedBy(row.id) > 0 ? fmtMoney(savedBy(row.id)) : '—' }}</span></template></el-table-column>
        <el-table-column label="近期成本（折后）" width="150" align="right"><template #default="{ row }"><span class="num">{{ fmtMoney(costBy(row.id)) }}</span></template></el-table-column>
        <el-table-column label="操作" width="90" align="right" class-name="col-actions"><template #default="{ row }"><el-button v-if="isSuper" link type="primary" :disabled="!dirty(row)" @click="saveDiscount(row)">保存</el-button></template></el-table-column>
      </el-table>
    </Panel>
  </div>
</template>

<script setup lang="ts">
import { useAuth } from '../../composables/useAuth'
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Refresh, Right } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import KpiCard from '../../components/KpiCard.vue'
import ChartBox from '../../components/ChartBox.vue'
import EmptyState from '../../components/EmptyState.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import VendorLogo from '../../components/VendorLogo.vue'
import { cost as costApi, providers as providersApi } from '../../api'
import { C, compact, tipRow, tooltipBase } from '../../charts'
import { categoryLabel } from '../../constants'
import { useLookups } from '../../composables/useLookups'
import { fmtDateTime, fmtMoney } from '../../utils'
import type { CategoryBoard, OverviewRow, Provider, Suggestion } from '../../types'

const { isSuper, canWrite } = useAuth()

const router = useRouter()
const { state, modelById, reload } = useLookups()

const days = ref(7)
const loading = ref(false)
const boards = ref<CategoryBoard[]>([])
const suggestions = ref<Suggestion[]>([])
const byProvider = ref<OverviewRow[]>([])
const cat = ref('')

const board = computed(() => boards.value.find((b) => b.category === cat.value) ?? null)
const modelCount = computed(() => boards.value.reduce((s, b) => s + b.models.length, 0))
const totalMonthly = computed(() => suggestions.value.reduce((s, x) => s + x.saving_monthly, 0))
const discountSaved = computed(() => byProvider.value.reduce((s, x) => s + x.saved, 0))
const codeOf = (id: number) => modelById.value.get(id)?.provider?.code ?? ''

async function load() {
  loading.value = true
  const to = new Date(), from = new Date(to.getTime() - days.value * 86400e3)
  const p = { from: fmtDateTime(from), to: fmtDateTime(to) }
  try {
    const [b, s, prov] = await Promise.all([costApi.benchmark(p), costApi.suggestions(p), costApi.overview({ ...p, group_by: 'provider' })])
    boards.value = b; suggestions.value = s; byProvider.value = prov
    if (!b.some((x) => x.category === cat.value)) cat.value = b[0]?.category ?? ''
  } finally { loading.value = false }
}
onMounted(load)

// ---- race chart --------------------------------------------------------------------------
const raceOption = computed(() => {
  const b = board.value
  if (!b) return {}
  const rows = b.models
  const names = rows.map((m) => `${m.rank}. ${m.name} · ${m.provider}`)
  return {
    grid: { left: 8, right: 76, top: 26, bottom: 8, containLabel: true },
    tooltip: { ...tooltipBase, trigger: 'axis' as const, axisPointer: { type: 'shadow' as const, shadowStyle: { color: 'rgba(17,24,39,.04)' } },
      formatter: (ps: { dataIndex: number }[]) => {
        const m = rows[ps[0].dataIndex]
        return `<div style="color:${C.text2};margin-bottom:4px">${m.name} · ${m.provider}</div>` +
          tipRow(C.primary, m.observed ? '实测折后均价' : '配置均价', fmtMoney(m.effective_per_m)) + tipRow('#9CA3AF', '标价均价', fmtMoney(m.list_per_m)) +
          tipRow('#9CA3AF', '较类别均价', `${m.vs_avg_pct > 0 ? '+' : ''}${m.vs_avg_pct.toFixed(0)}%`) + (m.observed ? tipRow('#9CA3AF', 'Token', compact(m.tokens)) : '')
      } },
    xAxis: { type: 'value' as const, axisLabel: { color: C.text2, fontSize: 11, formatter: (v: number) => '¥' + compact(v) }, splitLine: { lineStyle: { color: C.grid } }, axisLine: { show: false } },
    yAxis: { type: 'category' as const, inverse: true, data: names, axisTick: { show: false }, axisLine: { lineStyle: { color: C.axis } }, axisLabel: { color: C.text2, fontSize: 11.5, width: 240, overflow: 'truncate' as const } },
    series: [{
      type: 'bar' as const, barMaxWidth: 14,
      data: rows.map((m) => ({ value: Number(m.effective_per_m.toFixed(4)), itemStyle: { color: C.primary, opacity: m.observed ? 1 : 0.4, borderRadius: [0, 4, 4, 0] } })),
      label: { show: true, position: 'right' as const, color: C.text2, fontSize: 11, formatter: (p: { value: number }) => fmtMoney(p.value) },
      markLine: { symbol: 'none', silent: true, lineStyle: { color: '#9CA3AF', type: 'dashed' as const, width: 1 }, label: { formatter: `类别均价 ${fmtMoney(b.avg_per_m)}`, color: C.text2, fontSize: 11, position: 'end' as const }, data: [{ xAxis: b.avg_per_m }] },
    }],
  }
})

function createPolicy(s: Suggestion) {
  const from = modelById.value.get(s.from_model_id)
  router.push({ path: '/routing', query: { new: 1, source: from?.model_key ?? s.from_name, target: s.to_model_id } })
}

// ---- vendor discounts --------------------------------------------------------------------
const edits = reactive<Record<number, number>>({})
const rateOf = (p: Provider) => Number(p.discount_rate) || 1
const draft = (p: Provider) => edits[p.id] ?? rateOf(p)
const dirty = (p: Provider) => edits[p.id] !== undefined && Math.abs(edits[p.id] - rateOf(p)) > 1e-9
const discountText = (v: string) => (Number(v) >= 1 ? '无折扣' : `${(Number(v) * 10).toFixed(1).replace(/\.0$/, '')} 折`)
const savedBy = (id: number) => byProvider.value.find((r) => r.group_key === String(id))?.saved ?? 0
const costBy = (id: number) => byProvider.value.find((r) => r.group_key === String(id))?.cost ?? 0

async function saveDiscount(p: Provider) {
  await providersApi.update(p.id, { discount_rate: String(edits[p.id]) })
  delete edits[p.id]
  ElMessage.success('折扣已更新，后续调用按新折扣核算成本')
  await Promise.all([reload(), load()])
}
</script>

<style scoped>
.row.kpis4 { grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
@media (max-width: 960px) { .row.kpis4 { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
.mb { margin-bottom: 16px; }
.tabs { padding: 0 16px; }
.tabs :deep(.el-tabs__header) { margin: 0; }
.tabs :deep(.el-tabs__nav-wrap::after) { height: 1px; background: var(--border-soft); }
.race { display: grid; grid-template-columns: minmax(0, 1fr) 260px; }
@media (max-width: 1000px) { .race { grid-template-columns: 1fr; } }
.chart { padding: 12px 16px; min-width: 0; }
.ctitle { font-size: 13px; font-weight: 600; }
.ctitle .muted { font-size: 12px; font-weight: 400; margin-left: 8px; }
aside { padding: 16px; border-left: 1px solid var(--border-soft); background: #FAFBFC; }
dl { margin: 0; display: grid; gap: 12px; }
dl div { display: flex; justify-content: space-between; align-items: baseline; }
dt { font-size: 12.5px; color: var(--text-2); }
dd { margin: 0; font-size: 15px; font-weight: 600; }
.good { color: var(--success); }
.mcell { display: flex; align-items: center; gap: 8px; }
.arrow { color: var(--text-3); }
.note { padding: 10px 16px; font-size: 12px; color: var(--text-2); border-top: 1px solid var(--border-soft); background: #FCFCFD; }
</style>
