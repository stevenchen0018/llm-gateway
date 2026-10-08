<template>
  <div>
    <PageHeader title="应用管理" desc="应用是 Key、预算与调度策略的归属单元；负责人与 +1 主管将接收用量与成本周报">
      <TransferBar name="applications" :filters="{ status: fStatus, q: keyword }" @imported="load" />
      <el-button v-if="canWrite" type="primary" :icon="Plus" @click="openCreate">新增应用</el-button>
    </PageHeader>

    <Panel title="应用列表" :count="pg.total.value" flush>
      <template #toolbar>
        <el-radio-group v-model="fStatus" @change="pg.reset"><el-radio-button value="">全部</el-radio-button><el-radio-button value="active">启用</el-radio-button><el-radio-button value="disabled">停用</el-radio-button></el-radio-group>
        <el-input v-model="keyword" placeholder="搜索应用 / 负责人 / 描述" :prefix-icon="Search" clearable style="width: 220px" @input="pg.search" />
      </template>
      <el-table :data="pg.items.value" v-loading="pg.loading.value" empty-text="暂无应用">
        <el-table-column label="应用" min-width="220"><template #default="{ row }"><div class="cell-title">{{ row.name }}</div><div class="cell-sub">{{ row.description || '—' }}</div></template></el-table-column>
        <el-table-column label="部门" width="130"><template #default="{ row }">{{ deptName(row.department_id) }}</template></el-table-column>
        <el-table-column label="负责人" min-width="160"><template #default="{ row }"><div>{{ row.owner }}</div><div class="cell-sub">{{ row.owner_email }}</div></template></el-table-column>
        <el-table-column label="+1 主管" min-width="160"><template #default="{ row }"><div>{{ row.manager || '—' }}</div><div class="cell-sub">{{ row.manager_email }}</div></template></el-table-column>
        <el-table-column label="Key 数" width="80" align="right"><template #default="{ row }"><span class="num">{{ keyCount(row.id) }}</span></template></el-table-column>
        <el-table-column label="状态" width="90"><template #default="{ row }"><StatusBadge :text="row.status === 'active' ? '启用' : '停用'" :tone="row.status === 'active' ? 'success' : 'info'" /></template></el-table-column>
        <el-table-column label="" width="80" align="right" fixed="right"><template #default="{ row }"><el-button v-if="canWrite" link type="primary" @click="openEdit(row)">编辑</el-button></template></el-table-column>
      </el-table>
      <TablePager v-model:page="pg.page.value" v-model:page-size="pg.pageSize.value" :total="pg.total.value" />
    </Panel>

    <el-dialog v-model="visible" :title="editing ? '编辑应用' : '新增应用'" width="560px" destroy-on-close>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <div class="form-grid">
          <el-form-item label="应用名称" prop="name"><el-input v-model="form.name" /></el-form-item>
          <el-form-item label="所属部门" prop="department_id">
            <el-select v-if="isSuper" v-model="form.department_id" placeholder="选择部门"><el-option v-for="d in state.depts" :key="d.id" :label="d.name" :value="d.id" /></el-select>
            <el-input v-else :model-value="me?.department?.name" disabled />
          </el-form-item>
          <el-form-item label="应用描述" class="span2"><el-input v-model="form.description" type="textarea" :rows="2" resize="none" /></el-form-item>
          <el-form-item label="负责人" prop="owner"><el-input v-model="form.owner" /></el-form-item>
          <el-form-item label="负责人邮箱"><el-input v-model="form.owner_email" /></el-form-item>
          <el-form-item label="+1 主管"><el-input v-model="form.manager" /></el-form-item>
          <el-form-item label="主管邮箱"><el-input v-model="form.manager_email" /></el-form-item>
          <el-form-item v-if="editing" label="状态"><el-radio-group v-model="form.status"><el-radio value="active">启用</el-radio><el-radio value="disabled">停用</el-radio></el-radio-group></el-form-item>
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
import { useAuth } from '../../composables/useAuth'
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, type FormInstance } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import { platform } from '../../api'
import { useLookups } from '../../composables/useLookups'
import type { Application } from '../../types'

const { me, isSuper, canWrite } = useAuth()

const { state, reload, deptName } = useLookups()
const fStatus = ref('')
const keyword = ref('')
const pg = usePaged((p) => platform.applicationsPage({ ...p, status: fStatus.value, q: keyword.value.trim() }))
const keyCount = (id: number) => state.keys.filter((k) => k.app_id === id).length

const load = pg.refresh
onMounted(load)

const visible = ref(false)
const saving = ref(false)
const editing = ref<Application | null>(null)
const formRef = ref<FormInstance>()
const blank = () => ({ name: '', description: '', department_id: (me.value?.department_id ?? undefined) as number | undefined, owner: '', owner_email: '', manager: '', manager_email: '', status: 'active' })
const form = reactive(blank())
const rules = { department_id: [{ validator: (_r: unknown, v: number | undefined, cb: (e?: Error) => void) => (!isSuper.value || v ? cb() : cb(new Error('请选择部门'))), trigger: 'change' }], name: [{ required: true, message: '请输入应用名称', trigger: 'blur' }], owner: [{ required: true, message: '请输入负责人', trigger: 'blur' }] }

function openCreate() { editing.value = null; Object.assign(form, blank()); visible.value = true }
function openEdit(a: Application) { editing.value = a; Object.assign(form, { ...a }); visible.value = true }
async function save() {
  if (!(await formRef.value?.validate().catch(() => false))) return
  saving.value = true
  try {
    if (editing.value) await platform.updateApplication(editing.value.id, { ...form })
    else await platform.createApplication({ ...form })
    ElMessage.success('已保存'); visible.value = false
    await Promise.all([load(), reload()])
  } finally { saving.value = false }
}
</script>
