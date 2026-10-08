<template>
  <div>
    <PageHeader title="部门管理" desc="部门是租户：拥有应用、API Key、预算与部门级调度策略；可设置部门级 TPM / QPS 总配额，网关对部门内所有 Key 合并限流">
      <TransferBar name="departments" :filters="{ status: fStatus, q: keyword }" @imported="load" />
      <el-button v-if="isSuper" type="primary" :icon="Plus" @click="openCreate">新增部门</el-button>
    </PageHeader>

    <div class="row kpis4">
      <KpiCard label="部门数" :value="String(all.length)" :hint="`${all.filter((d) => d.status !== 'active').length} 个已停用`" />
      <KpiCard label="成员" :value="String(sum('users'))" hint="控制台账号" />
      <KpiCard label="应用 / Key" :value="`${sum('apps')} / ${sum('keys')}`" hint="归属于各部门" />
      <KpiCard label="近 30 天成本" :value="fmtMoney(all.reduce((s, d) => s + d.cost_30d, 0))" color="#7C3AED" :hint="`${compact(all.reduce((s, d) => s + d.tokens_30d, 0))} tokens`" />
    </div>

    <Panel title="部门列表" :count="pg.total.value" flush>
      <template #toolbar>
        <el-radio-group v-model="fStatus" @change="pg.reset"><el-radio-button value="">全部</el-radio-button><el-radio-button value="active">启用</el-radio-button><el-radio-button value="disabled">停用</el-radio-button></el-radio-group>
        <el-input v-model="keyword" placeholder="搜索部门名称 / 编码 / 负责人" :prefix-icon="Search" clearable style="width: 230px" @input="pg.search" />
      </template>
      <el-table :data="pg.items.value" v-loading="pg.loading.value" empty-text="暂无部门">
        <el-table-column label="部门" min-width="180">
          <template #default="{ row }"><div class="cell-title">{{ row.name }}</div><div class="cell-sub"><span class="mono">{{ row.code }}</span> · {{ row.description || '—' }}</div></template>
        </el-table-column>
        <el-table-column label="负责人" width="80" prop="leader" />
        <el-table-column label="成员/应用/Key" width="115" align="right"><template #default="{ row }"><span class="num">{{ row.users }} / {{ row.apps }} / {{ row.keys }}</span></template></el-table-column>
        <el-table-column label="部门 TPM" width="90" align="right"><template #default="{ row }"><span class="num">{{ row.tpm_quota ? compact(row.tpm_quota) : '不限' }}</span></template></el-table-column>
        <el-table-column label="部门 QPS" width="85" align="right"><template #default="{ row }"><span class="num">{{ row.qps_quota || '不限' }}</span></template></el-table-column>
        <el-table-column label="30 天调用" width="90" align="right"><template #default="{ row }"><span class="num">{{ compact(row.requests_30d) }}</span></template></el-table-column>
        <el-table-column label="30 天成本" min-width="160" align="right">
          <template #default="{ row }"><div class="costcell"><span class="bar"><i :style="{ width: share(row) + '%' }" /></span><span class="num">{{ fmtMoney(row.cost_30d) }}</span></div></template>
        </el-table-column>
        <el-table-column label="状态" width="80"><template #default="{ row }"><StatusBadge :text="row.status === 'active' ? '启用' : '停用'" :tone="row.status === 'active' ? 'success' : 'info'" /></template></el-table-column>
        <el-table-column label="" :width="isSuper ? 230 : 1" align="right" fixed="right" class-name="nowrap">
          <template #default="{ row }">
            <el-button v-if="isSuper" link type="primary" @click="viewData(row)">查看数据</el-button>
            <el-button v-if="isSuper" link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button v-if="isSuper" link :type="row.status === 'active' ? 'danger' : 'primary'" @click="toggle(row)">{{ row.status === 'active' ? '停用' : '启用' }}</el-button>
            <el-popconfirm v-if="isSuper" title="仅空部门可删除，确认删除？" @confirm="remove(row)"><template #reference><el-button link type="danger" :disabled="row.users + row.apps > 0">删除</el-button></template></el-popconfirm>
          </template>
        </el-table-column>
      </el-table>
      <TablePager v-model:page="pg.page.value" v-model:page-size="pg.pageSize.value" :total="pg.total.value" />
      <div class="note">停用部门后：该部门成员无法登录控制台，其全部 API Key 调用网关返回 403（department_disabled）。</div>
    </Panel>

    <el-dialog v-model="visible" :title="editing ? '编辑部门' : '新增部门'" width="560px" destroy-on-close>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <div class="form-grid">
          <el-form-item label="部门名称" prop="name"><el-input v-model="form.name" /></el-form-item>
          <el-form-item label="部门编码" prop="code"><el-input v-model="form.code" placeholder="如 ecom" /></el-form-item>
          <el-form-item label="负责人"><el-input v-model="form.leader" /></el-form-item>
          <el-form-item label="状态"><el-radio-group v-model="form.status"><el-radio value="active">启用</el-radio><el-radio value="disabled">停用</el-radio></el-radio-group></el-form-item>
          <el-form-item label="描述" class="span2"><el-input v-model="form.description" /></el-form-item>
          <el-form-item label="部门 TPM 总配额"><el-input-number v-model="form.tpm_quota" :min="0" :step="100000" controls-position="right" /></el-form-item>
          <el-form-item label="部门 QPS 总配额"><el-input-number v-model="form.qps_quota" :min="0" :step="10" controls-position="right" /></el-form-item>
          <div class="hint span2" style="margin-top: -8px">0 表示不限。部门配额与 Key 配额、模型容量同时生效，任一超限即返回 429；修改立即生效。</div>
        </div>
      </el-form>
      <template #footer><el-button @click="visible = false">取消</el-button><el-button type="primary" :loading="saving" @click="save">保存</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import TransferBar from '../../components/transfer/TransferBar.vue'
import TablePager from '../../components/TablePager.vue'
import { usePaged } from '../../composables/usePaged'
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox, type FormInstance } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import KpiCard from '../../components/KpiCard.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import { tenancy } from '../../api'
import { compact } from '../../charts'
import { setScope, useAuth } from '../../composables/useAuth'
import { loadLookups, useLookups } from '../../composables/useLookups'
import { fmtMoney } from '../../utils'
import type { DepartmentView } from '../../types'

const router = useRouter()
const { isSuper } = useAuth()
const fStatus = ref('')
const keyword = ref('')
const pg = usePaged((p) => tenancy.departmentsPage({ ...p, status: fStatus.value, q: keyword.value.trim() }))
// KPIs and the cost-share bars span every department (the lookup cache holds the full list)
const { state: lookups } = useLookups()
const all = computed(() => lookups.depts)
const sum = (k: 'users' | 'apps' | 'keys') => all.value.reduce((s, d) => s + d[k], 0)
const share = (d: DepartmentView) => { const max = Math.max(...all.value.map((x) => x.cost_30d), 0); return max ? (d.cost_30d / max) * 100 : 0 }

async function load() { await Promise.all([pg.refresh(), loadLookups(true)]) }
onMounted(pg.load)

const visible = ref(false)
const saving = ref(false)
const editing = ref<DepartmentView | null>(null)
const formRef = ref<FormInstance>()
const blank = () => ({ code: '', name: '', description: '', leader: '', tpm_quota: 0, qps_quota: 0, status: 'active' })
const form = reactive(blank())
const rules = {
  name: [{ required: true, message: '请输入部门名称', trigger: 'blur' }],
  code: [{ required: true, message: '请输入部门编码', trigger: 'blur' }, { pattern: /^[a-zA-Z0-9_-]+$/, message: '仅字母、数字、- 与 _', trigger: 'blur' }],
}

function openCreate() { editing.value = null; Object.assign(form, blank()); visible.value = true }
function openEdit(d: DepartmentView) {
  editing.value = d
  Object.assign(form, { code: d.code, name: d.name, description: d.description, leader: d.leader, tpm_quota: d.tpm_quota, qps_quota: d.qps_quota, status: d.status })
  visible.value = true
}
async function save() {
  if (!(await formRef.value?.validate().catch(() => false))) return
  saving.value = true
  try {
    if (editing.value) await tenancy.updateDepartment(editing.value.id, { ...form })
    else await tenancy.createDepartment({ ...form })
    ElMessage.success('已保存'); visible.value = false
    await load()
  } finally { saving.value = false }
}
async function toggle(d: DepartmentView) {
  const off = d.status === 'active'
  if (off) await ElMessageBox.confirm(`停用「${d.name}」后，其成员无法登录、全部 API Key 将被网关拒绝。确认停用？`, '停用部门', { type: 'warning' })
  await tenancy.updateDepartment(d.id, { ...d, status: off ? 'disabled' : 'active' })
  ElMessage.success(off ? '已停用' : '已启用')
  await load()
}
async function remove(d: DepartmentView) { await tenancy.deleteDepartment(d.id); ElMessage.success('已删除'); await load() }
async function viewData(d: DepartmentView) { setScope(d.id); await loadLookups(true); router.push('/dashboard') }
</script>

<style scoped>
.row.kpis4 { grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
@media (max-width: 960px) { .row.kpis4 { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
.costcell { display: flex; align-items: center; justify-content: flex-end; gap: 8px; }
.bar { width: 56px; height: 4px; border-radius: 2px; background: #EEF0F3; overflow: hidden; flex: none; }
.bar i { display: block; height: 100%; background: var(--accent); }
.el-table :deep(.nowrap .cell) { white-space: nowrap; }
.note { padding: 10px 16px; font-size: 12px; color: var(--text-2); border-top: 1px solid var(--border-soft); background: #FCFCFD; }
</style>
