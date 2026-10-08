<template>
  <div>
    <PageHeader title="Key 申请" desc="平台提供两类 Key：线上应用调用的「应用 Key」，与员工日常编码使用的「个人编码 Key」，分别走不同的归属、配额与审批" />

    <div class="cats">
      <button v-for="c in CATS" :key="c.value" type="button" class="cat" :class="{ on: category === c.value, off: c.value === 'application' && !canWrite }"
        :disabled="c.value === 'application' && !canWrite" @click="category = c.value">
        <el-icon :size="22"><component :is="c.icon" /></el-icon>
        <div class="cat-body">
          <div class="cat-title">{{ KEY_CATEGORY[c.value].text }}<span v-if="c.value === 'application' && !canWrite" class="muted">（只读成员不可申请）</span></div>
          <div class="cat-desc">{{ KEY_CATEGORY[c.value].desc }}</div>
          <ul><li v-for="p in c.points" :key="p">{{ p }}</li></ul>
        </div>
      </button>
    </div>

    <div class="layout">
      <!-- ---- personal coding key ---- -->
      <Panel v-if="category === 'personal'" title="个人编码 Key 申请">
        <el-form ref="pRef" :model="pf" :rules="pRules" label-position="top">
          <div class="form-grid">
            <el-form-item v-if="canWrite" label="申请方式" class="span2">
              <el-radio-group v-model="pf.for_self"><el-radio-button :value="true">为本人申请</el-radio-button><el-radio-button :value="false">代员工申请</el-radio-button></el-radio-group>
              <div class="hint">{{ pf.for_self ? '审批通过后由你本人在「我的 Key」领取密钥，审批人看不到密钥。' : '适用于没有控制台账号的员工（如外包驻场），审批通过后密钥交由审批人转交。' }}</div>
            </el-form-item>
            <template v-if="pf.for_self">
              <el-form-item label="持有人"><el-input :model-value="`${me?.display_name || me?.username}（${me?.username}）`" disabled /></el-form-item>
              <el-form-item label="所属部门"><el-input :model-value="me?.department?.name ?? '—'" disabled /></el-form-item>
              <el-form-item label="邮箱"><el-input :model-value="me?.email || '—'" disabled /></el-form-item>
            </template>
            <template v-else>
              <el-form-item label="员工姓名" prop="owner"><el-input v-model="pf.owner" /></el-form-item>
              <el-form-item label="员工邮箱" prop="owner_email"><el-input v-model="pf.owner_email" placeholder="每位员工限一个个人编码 Key" /></el-form-item>
              <el-form-item v-if="isSuper" label="所属部门" prop="department_id"><el-select v-model="pf.department_id" placeholder="选择部门"><el-option v-for="d in state.depts" :key="d.id" :label="d.name" :value="d.id" /></el-select></el-form-item>
              <el-form-item v-else label="所属部门"><el-input :model-value="me?.department?.name ?? '—'" disabled /></el-form-item>
            </template>
            <el-form-item label="工号"><el-input v-model="pf.employee_no" placeholder="选填，便于成本归集" /></el-form-item>
            <el-form-item label="使用的编码工具" class="span2">
              <el-checkbox-group v-model="pf.coding_tools"><el-checkbox v-for="t in CODING_TOOLS" :key="t" :value="t">{{ t }}</el-checkbox></el-checkbox-group>
            </el-form-item>
            <el-form-item label="可用模型" class="span2">
              <el-select v-model="pf.allowed_models" multiple filterable allow-create default-first-option placeholder="留空表示可调用全部模型">
                <el-option v-for="m in modelNames" :key="m" :label="m" :value="m" />
              </el-select>
              <div class="hint">默认只开放编码类模型，控制成本与合规风险；需要其他模型可联系部门管理员调整。</div>
            </el-form-item>
            <el-form-item label="用途说明" class="span2"><el-input v-model="pf.scenario" type="textarea" :rows="2" resize="none" placeholder="如：日常开发、代码评审、单测生成" /></el-form-item>
            <template v-if="canWrite && !pf.for_self">
              <el-form-item label="TPM 配额"><el-input-number v-model="pf.tpm_quota" :min="0" :step="50000" controls-position="right" /></el-form-item>
              <el-form-item label="QPS 配额"><el-input-number v-model="pf.qps_quota" :min="0" controls-position="right" /></el-form-item>
            </template>
            <div v-else class="hint span2" style="margin-top: -8px">个人编码 Key 默认配额 TPM 200,000 / QPS 5，同时受部门总配额约束。</div>
          </div>
          <el-button type="primary" :loading="saving" @click="submitPersonal">提交申请</el-button>
        </el-form>
      </Panel>

      <!-- ---- application key ---- -->
      <Panel v-else title="应用 Key 申请">
        <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
          <div class="form-grid">
            <el-form-item label="Key 类型" class="span2">
              <el-radio-group v-model="form.key_type">
                <el-radio-button value="formal">正式 Key</el-radio-button>
                <el-radio-button value="trial">试用 Key（7 天）</el-radio-button>
              </el-radio-group>
              <div class="hint">{{ form.key_type === 'trial' ? '试用 Key 默认 TPM 20,000 / QPS 2，到期自动失效，用于模型评估，无需预算。' : '正式 Key 通过审批后，需在「预算管理」提交预算申请（按额度分级审批）后纳入成本管控。' }}</div>
            </el-form-item>
            <el-form-item label="所属应用" prop="app_id"><el-select v-model="form.app_id" filterable placeholder="选择应用" @change="fillFromApp"><el-option v-for="a in state.apps" :key="a.id" :label="a.name" :value="a.id" /></el-select></el-form-item>
            <el-form-item label="Key 名称" prop="name"><el-input v-model="form.name" placeholder="如 客服机器人-生产" /></el-form-item>
            <el-form-item label="使用场景" class="span2"><el-input v-model="form.scenario" type="textarea" :rows="2" resize="none" placeholder="说明业务场景，便于审批与成本分摊" /></el-form-item>
            <el-form-item label="负责人" prop="owner"><el-input v-model="form.owner" /></el-form-item>
            <el-form-item label="负责人邮箱"><el-input v-model="form.owner_email" /></el-form-item>
            <el-form-item label="+1 主管"><el-input v-model="form.manager" placeholder="将同步接收周用量与成本报告" /></el-form-item>
            <el-form-item label="主管邮箱"><el-input v-model="form.manager_email" /></el-form-item>
            <el-form-item label="共享人" class="span2"><el-select v-model="form.shared_users" multiple filterable allow-create default-first-option placeholder="输入后回车添加" /></el-form-item>
            <el-form-item label="可用模型（可选）" class="span2">
              <el-select v-model="form.allowed_models" multiple filterable allow-create default-first-option placeholder="留空表示可调用全部模型"><el-option v-for="m in modelNames" :key="m" :label="m" :value="m" /></el-select>
            </el-form-item>
            <el-form-item label="IP 白名单（可选）" class="span2">
              <el-select v-model="form.ip_whitelist" multiple filterable allow-create default-first-option :reserve-keyword="false" placeholder="输入 IP 或 CIDR 后回车，如 10.20.0.0/16；留空不限制来源" />
              <div v-if="badIPs.length" class="hint" style="color: var(--danger)">格式错误：{{ badIPs.join('、') }}</div>
            </el-form-item>
            <template v-if="form.key_type === 'formal'">
              <el-form-item label="TPM 配额"><el-input-number v-model="form.tpm_quota" :min="0" :step="10000" controls-position="right" /></el-form-item>
              <el-form-item label="QPS 配额"><el-input-number v-model="form.qps_quota" :min="0" controls-position="right" /></el-form-item>
              <div class="hint span2" style="margin-top: -8px">0 表示不限；建议按预估峰值申请，超限将触发限流与告警。</div>
            </template>
          </div>
          <el-button type="primary" :loading="saving" @click="submit">提交申请</el-button>
        </el-form>
      </Panel>

      <div class="side">
        <Panel title="申请流程">
          <ol class="steps">
            <li v-for="s in steps" :key="s.t"><b>{{ s.t }}</b><span>{{ s.d }}</span></li>
          </ol>
        </Panel>
        <Panel title="待审批的申请" :sub="`${pendingTotal} 条`" flush>
          <EmptyState v-if="!pending.length" text="暂无待审批申请" height="110px" />
          <ul v-else class="pend">
            <li v-for="k in pending" :key="k.id"><div><div class="cell-title">{{ k.name }}</div><div class="cell-sub">{{ k.owner }} · {{ fmtTime(k.created_at) }}</div></div><StatusBadge :text="KEY_CATEGORY[k.category].text" :tone="KEY_CATEGORY[k.category].tone" /></li>
          </ul>
        </Panel>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox, type FormInstance } from 'element-plus'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import EmptyState from '../../components/EmptyState.vue'
import { keys } from '../../api'
import { useAuth } from '../../composables/useAuth'
import { loadLookups, useLookups } from '../../composables/useLookups'
import { CODING_TOOLS, DEFAULT_CODING_MODELS, KEY_CATEGORY } from '../../constants'
import { fmtTime, isIPOrCIDR } from '../../utils'
import type { ApiKey, KeyCategory } from '../../types'

const router = useRouter()
const { state, appById } = useLookups()
const { me, canWrite, isSuper } = useAuth()

const CATS: { value: KeyCategory; icon: string; points: string[] }[] = [
  { value: 'personal', icon: 'Monitor', points: ['Claude Code / Cursor / Cline 等编码工具', '一人一 Key，本人领取密钥', '默认仅开放编码模型'] },
  { value: 'application', icon: 'Grid', points: ['归属应用与部门，按应用核算成本', '正式 Key 需申请预算', '支持试用 Key（7 天）'] },
]
const category = ref<KeyCategory>(canWrite.value ? 'application' : 'personal')
const saving = ref(false)
const modelNames = computed(() => {
  const names = [...new Set(state.models.filter((m) => m.status === 'active').map((m) => m.display_name))]
  return [...DEFAULT_CODING_MODELS.filter((n) => names.includes(n)), ...names.filter((n) => !DEFAULT_CODING_MODELS.includes(n)).sort()]
})

const steps = computed(() => category.value === 'personal'
  ? [
      { t: '提交申请', d: '选择编码工具与可用模型；每位员工限一个个人编码 Key' },
      { t: '部门管理员审批', d: '在「Key 管理 · 个人编码 Key」中审批' },
      { t: '本人领取密钥', d: '持有人在「我的 Key」领取（仅展示一次），审批人不接触密钥' },
      { t: '配置编码工具', d: '按「我的 Key」中的接入指引配置 Claude Code / Cursor 等' },
    ]
  : [
      { t: '提交申请', d: '填写应用、场景与责任人' },
      { t: '管理员审批', d: '在「Key 管理」审批，自动分发密钥（仅展示一次）' },
      { t: '申请预算', d: '正式 Key 提交预算，≤ 1 万总监审批，> 1 万 CTO 审批' },
      { t: '接入调用', d: '按 OpenAI 协议调用统一 API，按 Key + 模型限流与调度' },
    ])

// ---- pending list (server side) --------------------------------------------------------------
const pending = ref<ApiKey[]>([])
const pendingTotal = ref(0)
async function loadPending() {
  const res = await keys.search({ status: 'pending', category: category.value, page: 1, page_size: 8 })
  pending.value = res.items; pendingTotal.value = res.total
}
watch(category, loadPending)
onMounted(loadPending)

// ---- personal -----------------------------------------------------------------------------------
const pRef = ref<FormInstance>()
const pBlank = () => ({
  for_self: true, owner: '', owner_email: '', employee_no: '', department_id: undefined as number | undefined,
  coding_tools: ['Claude Code'] as string[], allowed_models: [...DEFAULT_CODING_MODELS], scenario: '', tpm_quota: 200000, qps_quota: 5,
})
const pf = reactive(pBlank())
const pRules = {
  owner: [{ validator: (_r: unknown, v: string, cb: (e?: Error) => void) => (pf.for_self || v.trim() ? cb() : cb(new Error('请输入员工姓名'))), trigger: 'blur' }],
  owner_email: [{ validator: (_r: unknown, v: string, cb: (e?: Error) => void) => (pf.for_self || /.+@.+\..+/.test(v) ? cb() : cb(new Error('请输入有效邮箱'))), trigger: 'blur' }],
  department_id: [{ validator: (_r: unknown, v: number | undefined, cb: (e?: Error) => void) => (pf.for_self || !isSuper.value || v ? cb() : cb(new Error('请选择部门'))), trigger: 'change' }],
}
async function submitPersonal() {
  if (!(await pRef.value?.validate().catch(() => false))) return
  if (pf.for_self && !me.value?.department_id) { ElMessage.error('你的账号未归属部门，请由部门管理员代为申请'); return }
  saving.value = true
  try {
    await keys.apply({
      category: 'personal', for_self: pf.for_self, owner: pf.owner.trim(), owner_email: pf.owner_email.trim(), employee_no: pf.employee_no.trim(),
      department_id: pf.department_id, coding_tools: pf.coding_tools, allowed_models: pf.allowed_models, scenario: pf.scenario.trim(),
      tpm_quota: pf.for_self ? 0 : pf.tpm_quota, qps_quota: pf.for_self ? 0 : pf.qps_quota,
    })
    if (pf.for_self) {
      await ElMessageBox.alert('申请已提交。部门管理员审批通过后，请到「我的 Key」领取密钥并查看编码工具接入指引。', '已提交', { type: 'success', confirmButtonText: '去我的 Key' })
      router.push('/keys/mine')
    } else {
      ElMessage.success('申请已提交，请等待审批')
      Object.assign(pf, pBlank(), { for_self: false })
    }
    await Promise.all([loadPending(), loadLookups(true)])
  } finally { saving.value = false }
}

// ---- application --------------------------------------------------------------------------------
const formRef = ref<FormInstance>()
const blank = () => ({ key_type: 'formal', app_id: undefined as number | undefined, name: '', scenario: '', owner: '', owner_email: '', manager: '', manager_email: '', shared_users: [] as string[], ip_whitelist: [] as string[], allowed_models: [] as string[], tpm_quota: 0, qps_quota: 0 })
const form = reactive(blank())
const rules = {
  app_id: [{ required: true, message: '请选择应用', trigger: 'change' }],
  name: [{ required: true, message: '请输入 Key 名称', trigger: 'blur' }],
  owner: [{ required: true, message: '请输入负责人', trigger: 'blur' }],
}
const badIPs = computed(() => form.ip_whitelist.filter((x) => !isIPOrCIDR(x.trim())))

function fillFromApp(id: number) {
  const a = appById.value.get(id)
  if (!a) return
  Object.assign(form, { owner: form.owner || a.owner, owner_email: form.owner_email || a.owner_email, manager: form.manager || a.manager, manager_email: form.manager_email || a.manager_email })
}

async function submit() {
  if (!(await formRef.value?.validate().catch(() => false))) return
  if (badIPs.value.length) { ElMessage.error('IP 白名单格式有误'); return }
  saving.value = true
  try {
    await keys.apply({ ...form, category: 'application', tpm_quota: form.key_type === 'trial' ? 0 : form.tpm_quota, qps_quota: form.key_type === 'trial' ? 0 : form.qps_quota })
    ElMessage.success('申请已提交，请等待管理员审批')
    Object.assign(form, blank())
    await Promise.all([loadPending(), loadLookups(true)])
  } finally { saving.value = false }
}
</script>

<style scoped>
.cats { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin-bottom: 16px; }
@media (max-width: 900px) { .cats { grid-template-columns: 1fr; } }
.cat { display: flex; gap: 14px; align-items: flex-start; text-align: left; padding: 16px 18px; border: 1px solid var(--border); border-radius: var(--radius); background: var(--card); font: inherit; color: var(--text); cursor: pointer; transition: border-color .12s, background .12s; }
.cat:hover:not(:disabled) { border-color: var(--primary); }
.cat.on { border-color: var(--primary); background: var(--primary-soft); box-shadow: inset 0 0 0 1px var(--primary); }
.cat.on .el-icon { color: var(--primary); }
.cat.off { cursor: not-allowed; opacity: .55; }
.cat .el-icon { margin-top: 2px; color: var(--text-2); }
.cat-title { font-weight: 600; font-size: 15px; }
.cat-desc { font-size: 12.5px; color: var(--text-2); margin: 2px 0 6px; }
.cat ul { margin: 0; padding-left: 16px; font-size: 12.5px; color: var(--text-2); line-height: 1.7; }
.layout { display: grid; grid-template-columns: minmax(0, 3fr) minmax(0, 2fr); gap: 16px; align-items: start; }
@media (max-width: 1000px) { .layout { grid-template-columns: 1fr; } }
.side { display: grid; gap: 16px; }
.steps { margin: 0; padding: 0; list-style: none; counter-reset: s; display: grid; gap: 14px; }
.steps li { position: relative; padding-left: 34px; counter-increment: s; }
.steps li::before { content: counter(s); position: absolute; left: 0; top: 1px; width: 22px; height: 22px; border-radius: 50%; background: var(--primary-soft); color: var(--primary); font-size: 12px; font-weight: 600; display: flex; align-items: center; justify-content: center; }
.steps b { display: block; font-weight: 600; }
.steps span { display: block; margin-top: 2px; font-size: 12.5px; color: var(--text-2); line-height: 1.5; }
.pend { list-style: none; margin: 0; padding: 0; }
.pend li { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 16px; border-bottom: 1px solid var(--border-soft); }
.pend li:last-child { border-bottom: none; }
</style>
