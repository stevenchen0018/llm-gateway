<template>
  <div class="shell">
    <aside class="side">
      <div class="brand"><Logo /><div><b>LLM Gateway</b><span>AI Infra Console</span></div></div>
      <nav>
        <div v-for="g in groups" :key="g.label" class="group">
          <div class="glabel">{{ g.label }}</div>
          <router-link v-for="m in g.items" :key="m.path" :to="m.path" class="item" :class="{ on: route.path === m.path }">
            <el-icon :size="16"><component :is="m.icon" /></el-icon><span>{{ m.title }}</span>
          </router-link>
        </div>
      </nav>
      <div class="side-foot">
        <span class="dot" :class="health" />
        <span>{{ healthText }}</span>
        <span class="ver">v0.1</span>
      </div>
    </aside>

    <div class="main">
      <header class="top">
        <div class="crumb"><span class="faint">控制台</span><span class="sep">/</span><span v-if="groupLabel" class="faint">{{ groupLabel }}</span><span v-if="groupLabel" class="sep">/</span><b>{{ route.meta.title }}</b></div>
        <AnnouncementBar class="ann" />
        <div class="right">
          <StatusBadge :text="health === 'up' ? '网关运行中' : health === 'down' ? '网关不可达' : '检测中'" :tone="health === 'up' ? 'success' : health === 'down' ? 'danger' : 'info'" />
          <el-select v-if="isSuper" :model-value="auth.scope ?? 0" class="scope" size="default" @change="changeScope">
            <template #prefix><el-icon><OfficeBuilding /></el-icon></template>
            <el-option :value="0" label="全部部门" />
            <el-option v-for="d in lookups.depts" :key="d.id" :label="d.name" :value="d.id" />
          </el-select>
          <span v-else-if="me?.department" class="dept-chip"><el-icon><OfficeBuilding /></el-icon>{{ me.department.name }}</span>
          <el-dropdown trigger="click" @command="onUserCommand">
            <button class="user"><span class="avatar">{{ initial }}</span><span>{{ me?.display_name || me?.username }}</span><el-icon :size="12"><ArrowDown /></el-icon></button>
            <template #dropdown>
              <div class="who">
                <b>{{ me?.display_name || me?.username }}</b>
                <span class="faint">{{ me?.username }}</span>
                <div class="tags"><StatusBadge v-if="me" :text="ROLE_TEXT[me.role]" :tone="ROLE_TONE[me.role]" /><span v-if="me?.department" class="faint">{{ me.department.name }}</span></div>
              </div>
              <el-dropdown-menu>
                <el-dropdown-item command="password" :icon="Lock">修改密码</el-dropdown-item>
                <el-dropdown-item command="logout" :icon="SwitchButton" divided>退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </header>
      <main class="content"><div class="page"><router-view :key="route.fullPath + '|' + (auth.scope ?? 0)" /></div></main>
    </div>

    <el-dialog v-model="pwVisible" title="修改密码" width="420px" destroy-on-close>
      <el-form ref="pwRef" :model="pw" :rules="pwRules" label-position="top">
        <el-form-item label="当前密码" prop="old"><el-input v-model="pw.old" type="password" show-password autocomplete="current-password" /></el-form-item>
        <el-form-item label="新密码" prop="new"><el-input v-model="pw.new" type="password" show-password autocomplete="new-password" /><div class="hint">至少 8 位，需同时包含字母和数字</div></el-form-item>
        <el-form-item label="确认新密码" prop="confirm"><el-input v-model="pw.confirm" type="password" show-password autocomplete="new-password" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="pwVisible = false">取消</el-button><el-button type="primary" :loading="pwSaving" @click="savePassword">保存</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { reactive } from 'vue'
import { ElMessage, type FormInstance } from 'element-plus'
import { Lock, SwitchButton } from '@element-plus/icons-vue'
import { auth as authApi } from '../api'
import { ROLE_TEXT, ROLE_TONE, clearAuth, setScope, useAuth } from '../composables/useAuth'
import { loadLookups, useLookups } from '../composables/useLookups'
import Logo from '../components/Logo.vue'
import StatusBadge from '../components/StatusBadge.vue'
import AnnouncementBar from '../components/AnnouncementBar.vue'

const route = useRoute()
const router = useRouter()

// Sidebar structure follows the console: 模型市场 / 我的模型 / 模型体验 / 模型调度 /
// 监控与容量 / 模型成本 / API Key / 平台管理.
const structure = [
  { label: '概览', paths: ['/dashboard'] },
  { label: '模型', paths: ['/market', '/my-models', '/routing'] },
  { label: '模型体验', paths: ['/playground/text', '/playground/vision', '/playground/image', '/playground/video'] },
  { label: '监控与容量', paths: ['/monitor/model', '/monitor/key', '/capacity', '/alerts', '/logs'] },
  { label: '模型成本', paths: ['/cost', '/benchmark', '/budgets'] },
  { label: 'API Key', paths: ['/keys/mine', '/keys/apply', '/keys', '/blacklist'] },
  { label: '安全与合规', paths: ['/security/filter', '/security/requests'] },
  { label: '平台管理', paths: ['/departments', '/users', '/applications', '/vendors', '/providers', '/announcements', '/audit', '/data'] },
  { label: '开发者', paths: ['/docs'] },
]
const groupLabel = computed(() => structure.find((g) => g.paths.includes(route.path))?.label ?? '')
const { state: auth, me, isSuper } = useAuth()
const { state: lookups } = useLookups()

// hide pages the signed-in role cannot open (the router guard enforces the same rule)
const allowed = (perm?: unknown) => !perm || !!me.value?.permissions[perm as keyof NonNullable<typeof me.value>['permissions']]
const groups = computed(() => {
  const all = router.getRoutes().filter((r) => r.meta.title && allowed(r.meta.perm))
  return structure.map((g) => ({
    label: g.label,
    items: g.paths.map((p) => all.find((r) => r.path === p)).filter(Boolean).map((r) => ({ path: r!.path, title: r!.meta.title as string, icon: r!.meta.icon as string })),
  })).filter((g) => g.items.length)
})
const initial = computed(() => (me.value?.display_name || me.value?.username || '?').slice(0, 1).toUpperCase())

async function changeScope(v: number) {
  setScope(v || undefined)
  await loadLookups(true) // names/filters must reflect the new department slice
}

const health = ref<'unknown' | 'up' | 'down'>('unknown')
const healthText = computed(() => ({ unknown: '检测中', up: '服务正常', down: '服务异常' })[health.value])
async function ping() {
  try { // /readyz covers PostgreSQL, Redis and the schema version, not just process liveness
    health.value = (await fetch('/readyz')).ok ? 'up' : 'down' } catch { health.value = 'down' }
}
let timer: number
onMounted(() => { ping(); timer = window.setInterval(ping, 30_000) })
onBeforeUnmount(() => clearInterval(timer))

const logout = () => { clearAuth(); router.replace('/login') }

const pwVisible = ref(false)
const pwSaving = ref(false)
const pwRef = ref<FormInstance>()
const pw = reactive({ old: '', new: '', confirm: '' })
const pwRules = {
  old: [{ required: true, message: '请输入当前密码', trigger: 'blur' }],
  new: [{ required: true, message: '请输入新密码', trigger: 'blur' }, { min: 8, message: '至少 8 位', trigger: 'blur' }],
  confirm: [{ validator: (_r: unknown, v: string, cb: (e?: Error) => void) => (v === pw.new ? cb() : cb(new Error('两次输入不一致'))), trigger: 'blur' }],
}
function onUserCommand(cmd: string) {
  if (cmd === 'logout') return logout()
  Object.assign(pw, { old: '', new: '', confirm: '' })
  pwVisible.value = true
}
async function savePassword() {
  if (!(await pwRef.value?.validate().catch(() => false))) return
  pwSaving.value = true
  try { await authApi.changePassword(pw.old, pw.new); ElMessage.success('密码已修改'); pwVisible.value = false } finally { pwSaving.value = false }
}
</script>

<style scoped>
.shell { display: flex; height: 100%; }
.side { width: 232px; flex: none; background: var(--sidebar); color: var(--sidebar-text); display: flex; flex-direction: column; }
.brand { display: flex; align-items: center; gap: 10px; height: 56px; padding: 0 18px; border-bottom: 1px solid rgba(255, 255, 255, .06); }
.brand b { display: block; color: #fff; font-size: 14px; line-height: 18px; font-weight: 600; }
.brand span { display: block; font-size: 11px; color: #64748B; line-height: 14px; }
nav { flex: 1; overflow-y: auto; padding: 12px 10px; }
.group { margin-bottom: 14px; }
.glabel { padding: 0 10px 6px; font-size: 11px; font-weight: 500; letter-spacing: .06em; color: #64748B; }
.item { position: relative; display: flex; align-items: center; gap: 10px; height: 34px; padding: 0 10px; margin-bottom: 2px; border-radius: 6px; color: var(--sidebar-text); font-size: 13px; transition: background .12s, color .12s; }
.item:hover { background: rgba(255, 255, 255, .06); color: #fff; }
.item.on { background: rgba(255, 255, 255, .09); color: #fff; }
.item.on::before { content: ''; position: absolute; left: -10px; top: 8px; bottom: 8px; width: 3px; border-radius: 0 3px 3px 0; background: var(--accent); }
.item.on .el-icon { color: #A78BFA; }
.side-foot { display: flex; align-items: center; gap: 8px; padding: 12px 18px; border-top: 1px solid rgba(255, 255, 255, .06); font-size: 12px; color: #94A3B8; }
.side-foot .ver { margin-left: auto; color: #64748B; }
.dot { width: 7px; height: 7px; border-radius: 50%; background: #64748B; }
.dot.up { background: #22C55E; box-shadow: 0 0 0 3px rgba(34, 197, 94, .18); }
.dot.down { background: #EF4444; box-shadow: 0 0 0 3px rgba(239, 68, 68, .18); }

.main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
.top { height: 56px; flex: none; display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 0 24px; background: var(--card); border-bottom: 1px solid var(--border); }
.crumb { display: flex; align-items: center; gap: 8px; font-size: 13px; }
.crumb .sep { color: #D1D5DB; }
.ann { flex: 1; margin: 0 24px; justify-self: center; }
.crumb b { font-weight: 600; }
.right { display: flex; align-items: center; gap: 12px; }
.user { display: inline-flex; align-items: center; gap: 8px; height: 32px; padding: 0 8px 0 4px; border: 1px solid transparent; border-radius: 6px; background: none; cursor: pointer; font: inherit; color: var(--text); }
.user:hover { background: #F3F4F6; }
.scope { width: 170px; }
.dept-chip { display: inline-flex; align-items: center; gap: 6px; height: 28px; padding: 0 10px; border-radius: 6px; background: #F3F4F6; color: var(--text-2); font-size: 12.5px; }
.who { display: grid; gap: 2px; padding: 10px 16px 8px; min-width: 200px; border-bottom: 1px solid var(--border-soft); }
.who .tags { display: flex; align-items: center; gap: 8px; margin-top: 4px; font-size: 12px; }
.avatar { width: 24px; height: 24px; border-radius: 50%; background: var(--accent-soft); color: var(--accent); display: inline-flex; align-items: center; justify-content: center; font-size: 12px; font-weight: 600; }
.content { flex: 1; overflow-y: auto; padding: 24px; }
</style>
