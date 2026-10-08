<template>
  <div>
    <PageHeader title="告警中心" desc="限流、预算、容灾与安全事件；每 30 秒自动刷新，并推送至飞书 Webhook">
      <TransferBar name="alerts" :filters="{ type: typeFilter, level: levelFilter, q: keyword }" @imported="refreshAll" />
      <el-button :icon="Refresh" :loading="pg.loading.value" @click="refreshAll">刷新</el-button>
    </PageHeader>

    <div class="row kpis4">
      <KpiCard label="严重" :value="String(stats.critical)" :danger="stats.critical > 0" hint="累计，需立即处理" />
      <KpiCard label="警告" :value="String(stats.warning)" color="#F59E0B" hint="限流 / 预算阈值 / 容灾 / 安全" />
      <KpiCard label="提示" :value="String(stats.info)" color="#6B7280" hint="一般事件" />
      <KpiCard label="通知送达" :value="stats.all ? Math.round((stats.notified / stats.all) * 100) + '' : '—'" :unit="stats.all ? '%' : ''" hint="已成功推送 Webhook" />
    </div>

    <Panel title="告警事件" :count="pg.total.value" flush>
      <template #toolbar>
        <el-select v-model="typeFilter" clearable placeholder="全部类型" style="width: 120px" @change="pg.reset"><el-option v-for="(t, k) in typeText" :key="k" :label="t" :value="k" /></el-select>
        <el-select v-model="levelFilter" clearable placeholder="全部级别" style="width: 120px" @change="pg.reset"><el-option v-for="(t, k) in levelText" :key="k" :label="t" :value="k" /></el-select>
        <el-input v-model="keyword" placeholder="搜索告警内容" :prefix-icon="Search" clearable style="width: 200px" @input="pg.search" />
      </template>
      <el-table :data="pg.items.value" v-loading="pg.loading.value" empty-text="暂无告警">
        <el-table-column label="级别" width="100"><template #default="{ row }"><StatusBadge :text="levelText[row.level as Level]" :tone="levelTone[row.level as Level]" /></template></el-table-column>
        <el-table-column label="类型" width="90"><template #default="{ row }">{{ typeText[row.type as AlertEvent['type']] }}</template></el-table-column>
        <el-table-column prop="message" label="内容" min-width="380" show-overflow-tooltip />
        <el-table-column label="通知" width="100"><template #default="{ row }"><StatusBadge :text="row.notified_at ? '已发送' : '未发送'" :tone="row.notified_at ? 'success' : 'info'" /></template></el-table-column>
        <el-table-column label="时间" width="180"><template #default="{ row }"><span class="num muted">{{ fmtTime(row.created_at) }}</span></template></el-table-column>
      </el-table>
      <TablePager v-model:page="pg.page.value" v-model:page-size="pg.pageSize.value" :total="pg.total.value" />
    </Panel>
  </div>
</template>

<script setup lang="ts">
import TransferBar from '../components/transfer/TransferBar.vue'
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Refresh, Search } from '@element-plus/icons-vue'
import PageHeader from '../components/PageHeader.vue'
import Panel from '../components/Panel.vue'
import KpiCard from '../components/KpiCard.vue'
import StatusBadge from '../components/StatusBadge.vue'
import TablePager from '../components/TablePager.vue'
import { dashboard } from '../api'
import { usePaged } from '../composables/usePaged'
import type { AlertEvent } from '../types'
import { fmtTime } from '../utils'

type Level = AlertEvent['level']
const typeText = { quota: '限流', budget: '预算', failover: '容灾', report: '周报', security: '安全' } as const
const levelText = { info: '提示', warning: '警告', critical: '严重' } as const
const levelTone = { info: 'info', warning: 'warning', critical: 'danger' } as const

const typeFilter = ref('')
const levelFilter = ref('')
const keyword = ref('')
const pg = usePaged((p) => dashboard.alertsPage({ ...p, type: typeFilter.value, level: levelFilter.value, q: keyword.value.trim() }))

// KPI totals come from count-only queries (page_size=1), not from the rows on screen
const stats = reactive({ critical: 0, warning: 0, info: 0, all: 0, notified: 0 })
async function loadStats() {
  const count = (f: Record<string, string | boolean>) => dashboard.alertsPage({ page: 1, page_size: 1, ...f }).then((r) => r.total)
  const [critical, warning, info, all, notified] = await Promise.all([
    count({ level: 'critical' }), count({ level: 'warning' }), count({ level: 'info' }), count({}), count({ notified: true }),
  ])
  Object.assign(stats, { critical, warning, info, all, notified })
}
function refreshAll() { pg.load(); loadStats() }

let timer: number
onMounted(() => { refreshAll(); timer = window.setInterval(refreshAll, 30_000) })
onBeforeUnmount(() => clearInterval(timer))
</script>

<style scoped>
.row.kpis4 { grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
@media (max-width: 960px) { .row.kpis4 { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
</style>
