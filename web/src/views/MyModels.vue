<template>
  <div>
    <PageHeader title="我的模型" desc="已接入的模型及其近 7 日表现，便于快速体验、接入与对比">
      <el-button type="primary" :icon="Shop" @click="$router.push('/market')">去模型市场接入</el-button>
    </PageHeader>

    <Panel v-if="!loading && !rows.length && !keyword"><EmptyState text="还没有接入模型" hint="在模型市场点击卡片右下角的星标，即可加入我的模型" /></Panel>

    <Panel v-else title="已接入模型" :count="pg.total.value" flush>
      <template #toolbar><el-input v-model="keyword" placeholder="搜索模型 / 厂商" :prefix-icon="Search" clearable style="width: 200px" @input="pg.search" /></template>
      <el-table :data="rows" v-loading="loading">
        <el-table-column label="模型" min-width="220">
          <template #default="{ row }">
            <div class="mcell"><VendorLogo :code="row.provider?.code" :size="30" /><div><div class="cell-title">{{ row.display_name }}</div><div class="cell-sub">{{ row.provider?.name }}</div></div></div>
          </template>
        </el-table-column>
        <el-table-column label="类型" width="110"><template #default="{ row }"><StatusBadge :text="categoryLabel(row.category)" tone="primary" /></template></el-table-column>
        <el-table-column label="上下文" width="90" align="right"><template #default="{ row }"><span class="num">{{ fmtContext(row.context_length) }}</span></template></el-table-column>
        <el-table-column label="近 7 日调用" width="120" align="right"><template #default="{ row }"><span class="num">{{ stat(row.id) ? fmtNum(stat(row.id)!.requests) : '—' }}</span></template></el-table-column>
        <el-table-column label="失败率" width="90" align="right"><template #default="{ row }"><span class="num" :style="{ color: (stat(row.id)?.failure_rate ?? 0) > 5 ? 'var(--danger)' : undefined }">{{ stat(row.id) ? stat(row.id)!.failure_rate.toFixed(2) + '%' : '—' }}</span></template></el-table-column>
        <el-table-column label="平均 RT" width="100" align="right"><template #default="{ row }"><span class="num">{{ stat(row.id) ? fmtDuration(stat(row.id)!.avg_latency_ms) : '—' }}</span></template></el-table-column>
        <el-table-column label="成本" width="110" align="right"><template #default="{ row }"><span class="num">{{ stat(row.id) ? fmtMoney(stat(row.id)!.cost) : '—' }}</span></template></el-table-column>
        <el-table-column label="状态" width="90"><template #default="{ row }"><StatusBadge :text="row.status === 'active' ? '可用' : '已下架'" :tone="row.status === 'active' ? 'success' : 'info'" /></template></el-table-column>
        <el-table-column label="" width="200" align="right" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="$router.push({ path: playgroundPath(row.category), query: { model: row.id } })">体验</el-button>
            <el-button link type="primary" @click="guide(row)">接入说明</el-button>
            <el-popconfirm title="移出我的模型？" @confirm="remove(row)"><template #reference><el-button link type="danger">移除</el-button></template></el-popconfirm>
          </template>
        </el-table-column>
      </el-table>
      <TablePager v-model:page="pg.page.value" v-model:page-size="pg.pageSize.value" :total="pg.total.value" />
    </Panel>

    <ModelGuideDialog v-model:visible="guideVisible" :model="guideModel" />
  </div>
</template>

<script setup lang="ts">
import TablePager from '../components/TablePager.vue'
import { usePaged } from '../composables/usePaged'
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Shop, Search } from '@element-plus/icons-vue'
import PageHeader from '../components/PageHeader.vue'
import Panel from '../components/Panel.vue'
import EmptyState from '../components/EmptyState.vue'
import StatusBadge from '../components/StatusBadge.vue'
import VendorLogo from '../components/VendorLogo.vue'
import ModelGuideDialog from '../components/model/ModelGuideDialog.vue'
import { models, monitor } from '../api'
import { categoryLabel, fmtContext, playgroundPath } from '../constants'
import { fmtDateTime, fmtDuration, fmtMoney, fmtNum } from '../utils'
import type { Model, RankRow } from '../types'

const keyword = ref('')
const pg = usePaged((p) => models.minePage({ ...p, q: keyword.value.trim() }))
const rows = pg.items
const loading = pg.loading
const stats = ref(new Map<string, RankRow>())
const stat = (id: number) => stats.value.get(String(id))

async function load() {
  await pg.refresh()
  try {
    const to = new Date(), from = new Date(to.getTime() - 7 * 86400e3)
    const rank = await monitor.ranking({ group_by: 'model', from: fmtDateTime(from), to: fmtDateTime(to) })
    stats.value = new Map(rank.map((r) => [r.group_key, r]))
  } catch { /* stats are optional */ }
}
onMounted(load)

async function remove(m: Model) { await models.removeMine(m.id); ElMessage.success('已移除'); await load() }

const guideVisible = ref(false)
const guideModel = ref<Model | null>(null)
function guide(m: Model) { guideModel.value = m; guideVisible.value = true }
</script>

<style scoped>
.mcell { display: flex; align-items: center; gap: 10px; }
</style>
