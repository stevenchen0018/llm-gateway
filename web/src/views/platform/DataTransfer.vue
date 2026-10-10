<template>
  <div>
    <PageHeader title="数据导入导出" desc="按模板批量导入部门、用户、应用、厂商、供应商、模型、API Key 与过滤规则；导出各类列表与日志为 Excel / CSV。导入先校验预览、再确认提交，全程留痕" />

    <div v-for="g in groups" :key="g.name" class="group">
      <div class="g-title">{{ g.name }}</div>
      <div class="cards">
        <div v-for="e in g.items" :key="e.name" class="card">
          <div class="c-head">
            <el-icon :size="18" class="c-ico"><component :is="ICON[e.name] ?? 'Document'" /></el-icon>
            <b>{{ e.title }}</b>
            <span class="caps">
              <span v-if="e.can_import" class="cap imp">可导入</span>
              <span v-if="e.can_export" class="cap exp">可导出</span>
            </span>
          </div>
          <p class="c-desc">{{ e.desc }}</p>
          <div class="c-meta muted">{{ e.columns.filter((c) => !c.export_only).length }} 个导入字段 · {{ e.columns.filter((c) => !c.import_only).length }} 个导出字段</div>
          <div class="c-acts">
            <el-button v-if="e.can_import" size="small" type="primary" plain :icon="Upload" @click="openImport(e)">导入</el-button>
            <el-button v-if="e.can_export" size="small" :icon="Download" :loading="busy === e.name" @click="exp(e, 'xlsx')">导出 Excel</el-button>
            <el-button v-if="e.can_export" size="small" link @click="exp(e, 'csv')">CSV</el-button>
            <el-button v-if="e.can_import" size="small" link type="primary" @click="tpl(e)">下载模板</el-button>
          </div>
        </div>
      </div>
    </div>

    <Panel title="导入导出记录" :count="pg.total.value" flush>
      <template #toolbar>
        <el-radio-group v-model="fDir" @change="pg.reset"><el-radio-button value="">全部</el-radio-button><el-radio-button value="import">导入</el-radio-button><el-radio-button value="export">导出</el-radio-button></el-radio-group>
        <el-select v-model="fEntity" clearable placeholder="全部数据类型" style="width: 160px" @change="pg.reset"><el-option v-for="e in state.list" :key="e.name" :label="e.title" :value="e.name" /></el-select>
        <span class="spacer" />
        <el-button :icon="Refresh" @click="pg.load">刷新</el-button>
      </template>
      <el-table :data="pg.items.value" v-loading="pg.loading.value" empty-text="暂无导入导出记录">
        <el-table-column label="时间" width="170"><template #default="{ row }"><span class="num muted nowrap">{{ fmtTime(row.created_at) }}</span></template></el-table-column>
        <el-table-column label="类型" width="90"><template #default="{ row }"><StatusBadge :text="row.direction === 'import' ? '导入' : '导出'" :tone="row.direction === 'import' ? 'primary' : 'accent'" /></template></el-table-column>
        <el-table-column label="数据" width="130" prop="entity_title" />
        <el-table-column label="文件" min-width="220" show-overflow-tooltip><template #default="{ row }"><span class="mono">{{ row.file_name }}</span></template></el-table-column>
        <el-table-column label="结果" min-width="220">
          <template #default="{ row }">
            <span v-if="row.direction === 'export'" class="num">{{ row.total.toLocaleString() }} 行<span v-if="row.message" class="warn">（{{ row.message }}）</span></span>
            <span v-else class="res">
              <span>共 {{ row.total }}</span><span class="ok">新增 {{ row.created }}</span><span class="up">更新 {{ row.updated }}</span>
              <span v-if="row.skipped">跳过 {{ row.skipped }}</span><span v-if="row.failed" class="bad">失败 {{ row.failed }}</span>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100"><template #default="{ row }"><StatusBadge :text="STATUS[row.status as TransferJob['status']].text" :tone="STATUS[row.status as TransferJob['status']].tone" /></template></el-table-column>
        <el-table-column label="操作人" width="130" prop="operator" />
        <el-table-column label="操作" width="110" align="right" class-name="col-actions"><template #default="{ row }"><el-button v-if="row.errors?.length" link type="primary" @click="errJob = row">错误明细</el-button></template></el-table-column>
      </el-table>
      <TablePager v-model:page="pg.page.value" v-model:page-size="pg.pageSize.value" :total="pg.total.value" />
    </Panel>

    <ImportDialog v-if="importing" v-model:visible="importVisible" :entity="importing" @done="onImported" />

    <el-drawer :model-value="!!errJob" :title="`错误明细 · ${errJob?.entity_title ?? ''}`" size="560px" @update:model-value="(v: boolean) => !v && (errJob = null)">
      <el-table v-if="errJob" :data="errJob.errors" size="small">
        <el-table-column label="行号" width="70" prop="row" />
        <el-table-column label="列" width="130" prop="column" />
        <el-table-column label="问题" min-width="260" prop="message" />
      </el-table>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Download, Refresh, Upload } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import TablePager from '../../components/TablePager.vue'
import ImportDialog from '../../components/transfer/ImportDialog.vue'
import { transfer } from '../../api'
import { useTransferEntities } from '../../composables/useTransfer'
import { usePaged } from '../../composables/usePaged'
import { loadLookups } from '../../composables/useLookups'
import { fmtTime } from '../../utils'
import type { TransferEntity, TransferJob } from '../../types'

const ICON: Record<string, string> = {
  departments: 'OfficeBuilding', users: 'User', applications: 'Grid', vendors: 'Management', suppliers: 'Connection', models: 'Cpu',
  keys: 'Key', filter_rules: 'Filter', budgets: 'Wallet', audit_logs: 'Tickets', alerts: 'Bell', call_logs: 'Document', request_logs: 'ChatLineSquare',
}
const STATUS = { success: { text: '成功', tone: 'success' }, partial: { text: '部分成功', tone: 'warning' }, failed: { text: '失败', tone: 'danger' } } as const

const state = useTransferEntities()
const groups = computed(() => {
  const by = new Map<string, TransferEntity[]>()
  for (const e of state.list) by.set(e.group, [...(by.get(e.group) ?? []), e])
  return [...by].map(([name, items]) => ({ name, items }))
})

const fDir = ref('')
const fEntity = ref('')
const pg = usePaged((p) => transfer.jobs({ ...p, direction: fDir.value, entity: fEntity.value }))
onMounted(pg.load)

const busy = ref('')
async function exp(e: TransferEntity, format: 'xlsx' | 'csv') {
  busy.value = e.name
  try {
    const r = await transfer.export(e.name, format)
    ElMessage.success(r.truncated ? `已导出前 ${r.rows} 行（已截断）` : `已导出 ${r.rows.toLocaleString()} 行`)
    pg.refresh()
  } catch (err) { ElMessage.error((err as Error).message) } finally { busy.value = '' }
}
async function tpl(e: TransferEntity) {
  try { await transfer.template(e.name) } catch (err) { ElMessage.error((err as Error).message) }
}

const importing = ref<TransferEntity | null>(null)
const importVisible = ref(false)
function openImport(e: TransferEntity) { importing.value = e; importVisible.value = true }
function onImported() { pg.refresh(); loadLookups(true) }

const errJob = ref<TransferJob | null>(null)
</script>

<style scoped>
.group { margin-bottom: 18px; }
.g-title { font-size: 12px; font-weight: 600; color: var(--text-2); letter-spacing: .04em; margin-bottom: 8px; }
.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 12px; }
.card { display: flex; flex-direction: column; gap: 6px; padding: 14px; background: var(--card); border: 1px solid var(--border); border-radius: var(--radius); transition: border-color .15s, box-shadow .15s; }
.card:hover { border-color: #C7D2FE; box-shadow: 0 2px 10px rgba(37, 99, 235, .06); }
.c-head { display: flex; align-items: center; gap: 8px; font-size: 14px; }
.c-ico { color: var(--primary); }
.caps { margin-left: auto; display: flex; gap: 4px; }
.cap { font-size: 11px; padding: 0 6px; border-radius: 4px; line-height: 18px; }
.cap.imp { background: var(--primary-soft); color: var(--primary); }
.cap.exp { background: var(--accent-soft); color: var(--accent); }
.c-desc { margin: 0; font-size: 12.5px; color: var(--text-2); line-height: 1.6; }
.c-meta { font-size: 12px; }
.c-acts { display: flex; align-items: center; flex-wrap: wrap; gap: 6px 4px; margin-top: auto; padding-top: 10px; border-top: 1px solid var(--border-soft); }
.c-acts .el-button + .el-button { margin-left: 0; }
.res { display: inline-flex; gap: 10px; font-size: 12.5px; }
.res .ok { color: var(--success); }
.res .up { color: var(--primary); }
.res .bad, .warn { color: var(--danger); }
</style>
