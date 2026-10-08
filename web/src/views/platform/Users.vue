<template>
  <div>
    <PageHeader title="用户管理" :desc="isSuper ? '管理控制台账号与角色：超级管理员管理全平台；部门管理员管理本部门；只读成员仅可查看本部门数据' : `管理「${me?.department?.name ?? ''}」的成员账号`">
      <TransferBar name="users" :filters="{ department_id: fDept, role: fRole, status: fStatus, q: keyword }" @imported="load" />
      <el-button type="primary" :icon="Plus" @click="openCreate">新增用户</el-button>
    </PageHeader>

    <div class="roles">
      <div v-for="r in ROLES" :key="r.value" class="role"><StatusBadge :text="ROLE_TEXT[r.value]" :tone="ROLE_TONE[r.value]" /><span class="muted">{{ r.desc }}</span></div>
    </div>

    <Panel title="用户列表" :count="pg.total.value" flush>
      <template #toolbar>
        <el-select v-if="isSuper" v-model="fDept" clearable placeholder="全部部门" style="width: 150px" @change="pg.reset"><el-option v-for="d in state.depts" :key="d.id" :label="d.name" :value="d.id" /></el-select>
        <el-select v-model="fRole" clearable placeholder="全部角色" style="width: 130px" @change="pg.reset"><el-option v-for="r in ROLES" :key="r.value" :label="ROLE_TEXT[r.value]" :value="r.value" /></el-select>
        <el-select v-model="fStatus" clearable placeholder="全部状态" style="width: 110px" @change="pg.reset"><el-option label="启用" value="active" /><el-option label="禁用" value="disabled" /></el-select>
        <el-input v-model="keyword" placeholder="搜索用户名 / 姓名 / 邮箱" :prefix-icon="Search" clearable style="width: 220px" @input="pg.search" />
      </template>
      <el-table :data="pg.items.value" v-loading="pg.loading.value" empty-text="暂无用户">
        <el-table-column label="用户" min-width="200">
          <template #default="{ row }">
            <div class="ucell"><span class="avatar">{{ (row.display_name || row.username).slice(0, 1) }}</span>
              <div><div class="cell-title">{{ row.display_name || row.username }}<span v-if="row.id === me?.id" class="you">我</span></div><div class="cell-sub mono">{{ row.username }}</div></div></div>
          </template>
        </el-table-column>
        <el-table-column label="邮箱" min-width="170" prop="email" show-overflow-tooltip />
        <el-table-column label="角色" width="120"><template #default="{ row }"><StatusBadge :text="ROLE_TEXT[row.role as Role]" :tone="ROLE_TONE[row.role as Role]" /></template></el-table-column>
        <el-table-column label="部门" width="130"><template #default="{ row }">{{ row.department_id ? deptName(row.department_id) : '全平台' }}</template></el-table-column>
        <el-table-column label="状态" width="90"><template #default="{ row }"><StatusBadge :text="row.status === 'active' ? '正常' : '已禁用'" :tone="row.status === 'active' ? 'success' : 'danger'" /></template></el-table-column>
        <el-table-column label="最近登录" width="160"><template #default="{ row }"><span class="num muted">{{ fmtTime(row.last_login_at) }}</span></template></el-table-column>
        <el-table-column label="" width="200" align="right" fixed="right">
          <template #default="{ row }">
            <template v-if="row.id !== me?.id">
              <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
              <el-button link type="primary" @click="openReset(row)">重置密码</el-button>
              <el-button link :type="row.status === 'active' ? 'danger' : 'primary'" @click="toggle(row)">{{ row.status === 'active' ? '禁用' : '启用' }}</el-button>
            </template>
            <span v-else class="faint">当前账号</span>
          </template>
        </el-table-column>
      </el-table>
      <TablePager v-model:page="pg.page.value" v-model:page-size="pg.pageSize.value" :total="pg.total.value" />
    </Panel>

    <el-dialog v-model="visible" :title="editing ? '编辑用户' : '新增用户'" width="540px" destroy-on-close>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <div class="form-grid">
          <el-form-item label="用户名" prop="username"><el-input v-model="form.username" :disabled="!!editing" placeholder="登录名，字母数字 . _ -" /></el-form-item>
          <el-form-item label="姓名"><el-input v-model="form.display_name" /></el-form-item>
          <el-form-item label="邮箱" class="span2"><el-input v-model="form.email" /></el-form-item>
          <el-form-item label="角色" prop="role">
            <el-select v-model="form.role"><el-option v-for="r in assignableRoles" :key="r.value" :label="ROLE_TEXT[r.value]" :value="r.value" /></el-select>
          </el-form-item>
          <el-form-item label="所属部门" prop="department_id">
            <el-select v-if="isSuper" v-model="form.department_id" :disabled="form.role === 'super_admin'" :placeholder="form.role === 'super_admin' ? '超级管理员不属于部门' : '选择部门'" clearable>
              <el-option v-for="d in state.depts" :key="d.id" :label="d.name" :value="d.id" />
            </el-select>
            <el-input v-else :model-value="me?.department?.name" disabled />
          </el-form-item>
          <el-form-item v-if="!editing" label="初始密码" prop="password" class="span2">
            <el-input v-model="form.password" type="password" show-password autocomplete="new-password" />
            <div class="hint">至少 8 位，需同时包含字母和数字；请通过安全渠道告知用户并提醒其登录后修改</div>
          </el-form-item>
        </div>
      </el-form>
      <template #footer><el-button @click="visible = false">取消</el-button><el-button type="primary" :loading="saving" @click="save">保存</el-button></template>
    </el-dialog>

    <el-dialog v-model="resetVisible" :title="`重置密码 · ${target?.username}`" width="420px" destroy-on-close>
      <el-input v-model="newPassword" type="password" show-password placeholder="新密码（至少 8 位，含字母和数字）" autocomplete="new-password" />
      <template #footer><el-button @click="resetVisible = false">取消</el-button><el-button type="primary" :loading="saving" :disabled="newPassword.length < 8" @click="reset">确认重置</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import TransferBar from '../../components/transfer/TransferBar.vue'
import TablePager from '../../components/TablePager.vue'
import { usePaged } from '../../composables/usePaged'
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import { tenancy } from '../../api'
import { ROLE_TEXT, ROLE_TONE, useAuth } from '../../composables/useAuth'
import { useLookups } from '../../composables/useLookups'
import { fmtTime } from '../../utils'
import type { AdminUser, Role } from '../../types'

const ROLES: { value: Role; desc: string }[] = [
  { value: 'super_admin', desc: '全平台：厂商、模型、部门、用户、全局调度策略、CTO 级预算审批' },
  { value: 'dept_admin', desc: '本部门：应用、Key 审批与配额、部门调度策略、总监级预算审批、成员' },
  { value: 'viewer', desc: '本部门只读：监控、成本、日志与模型体验' },
]
const { me, isSuper } = useAuth()
const { state, deptName } = useLookups()
const assignableRoles = computed(() => ROLES.filter((r) => isSuper.value || r.value !== 'super_admin'))

const fDept = ref<number>()
const fRole = ref<Role>()
const fStatus = ref('')
const keyword = ref('')
// department_id is the explicit filter here, overriding the console-wide department scope
const pg = usePaged((p) => tenancy.usersPage({ ...p, department_id: fDept.value, role: fRole.value, status: fStatus.value, q: keyword.value.trim() }))
const load = pg.refresh
onMounted(load)

const visible = ref(false)
const saving = ref(false)
const editing = ref<AdminUser | null>(null)
const formRef = ref<FormInstance>()
const blank = () => ({ username: '', display_name: '', email: '', role: 'viewer' as Role, department_id: undefined as number | undefined, password: '' })
const form = reactive(blank())
const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }, { pattern: /^[a-zA-Z0-9_.-]{3,64}$/, message: '3-64 位字母、数字、. _ -', trigger: 'blur' }],
  role: [{ required: true, message: '请选择角色', trigger: 'change' }],
  department_id: [{ validator: (_r: unknown, v: number | undefined, cb: (e?: Error) => void) => (!isSuper.value || form.role === 'super_admin' || v ? cb() : cb(new Error('请选择部门'))), trigger: 'change' }],
  password: [{ required: true, message: '请输入初始密码', trigger: 'blur' }, { min: 8, message: '至少 8 位', trigger: 'blur' }],
}

function openCreate() { editing.value = null; Object.assign(form, blank()); if (!isSuper.value) form.role = 'viewer'; visible.value = true }
function openEdit(u: AdminUser) {
  editing.value = u
  Object.assign(form, { username: u.username, display_name: u.display_name, email: u.email, role: u.role, department_id: u.department_id ?? undefined, password: '' })
  visible.value = true
}
async function save() {
  if (!(await formRef.value?.validate().catch(() => false))) return
  saving.value = true
  const body = { display_name: form.display_name, email: form.email, role: form.role, department_id: form.role === 'super_admin' ? undefined : form.department_id }
  try {
    if (editing.value) await tenancy.updateUser(editing.value.id, body)
    else await tenancy.createUser({ ...body, username: form.username, password: form.password })
    ElMessage.success('已保存'); visible.value = false; await load()
  } finally { saving.value = false }
}
async function toggle(u: AdminUser) {
  const off = u.status === 'active'
  if (off) await ElMessageBox.confirm(`禁用后「${u.username}」将立即无法使用控制台（已登录会话同时失效）。`, '禁用用户', { type: 'warning' })
  await tenancy.updateUser(u.id, { status: off ? 'disabled' : 'active' })
  ElMessage.success(off ? '已禁用' : '已启用'); await load()
}

const resetVisible = ref(false)
const target = ref<AdminUser | null>(null)
const newPassword = ref('')
function openReset(u: AdminUser) { target.value = u; newPassword.value = ''; resetVisible.value = true }
async function reset() {
  saving.value = true
  try { await tenancy.resetPassword(target.value!.id, newPassword.value); ElMessage.success('密码已重置'); resetVisible.value = false } finally { saving.value = false }
}
</script>

<style scoped>
.roles { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; margin-bottom: 16px; }
@media (max-width: 1000px) { .roles { grid-template-columns: 1fr; } }
.role { display: flex; flex-direction: column; align-items: flex-start; gap: 6px; padding: 12px 14px; background: var(--card); border: 1px solid var(--border); border-radius: var(--radius); font-size: 12.5px; line-height: 1.6; }
.ucell { display: flex; align-items: center; gap: 10px; }
.avatar { width: 28px; height: 28px; border-radius: 50%; background: var(--primary-soft); color: var(--primary); display: inline-flex; align-items: center; justify-content: center; font-weight: 600; font-size: 12px; flex: none; }
.you { margin-left: 6px; padding: 0 6px; border-radius: 4px; background: var(--accent-soft); color: var(--accent); font-size: 11px; font-weight: 500; }
</style>
