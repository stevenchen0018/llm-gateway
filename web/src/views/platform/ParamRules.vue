<template>
  <div>
    <PageHeader title="接口参数适配" desc="厂商调整接口参数时（如 OpenAI 新模型由 max_tokens 改为 max_completion_tokens），按 协议 / 供应商 / 模型 改写发往供应商的请求；供应商仍拒绝某个参数时，网关自动调整重试并记录为自动规则">
      <el-button :icon="View" @click="openPreview()">规则预览</el-button>
      <el-button v-if="isSuper" type="primary" :icon="Plus" @click="openCreate">新增规则</el-button>
    </PageHeader>

    <div class="flow">
      <div class="step"><b>1 · 内置预设</b><span>常见的厂商参数变更，开箱即用，可停用或修改</span></div>
      <el-icon class="arrow"><Right /></el-icon>
      <div class="step"><b>2 · 手动规则</b><span>为某家供应商 / 某个模型单独指定参数，越具体越后执行、覆盖通用规则</span></div>
      <el-icon class="arrow"><Right /></el-icon>
      <div class="step"><b>3 · 自动学习</b><span>供应商返回“参数不支持”时自动调整并重试，成功后为该供应商 + 模型记录规则并告警</span></div>
    </div>

    <div class="kpis">
      <KpiCard label="启用的规则" :value="String(rows.filter((r) => r.enabled).length)" :hint="`共 ${rows.length} 条`" />
      <KpiCard label="自动学习" :value="String(rows.filter((r) => r.source === 'auto').length)" hint="由供应商报错学到的适配" />
      <KpiCard label="累计改写请求" :value="fmtNum(rows.reduce((s, r) => s + r.hit_count, 0))" :hint="lastHit ? `最近 ${fmtTime(lastHit)}` : '暂无'" />
      <KpiCard label="涉及供应商" :value="String(new Set(rows.filter((r) => r.provider_id).map((r) => r.provider_id)).size)" hint="有供应商专属规则的数量" />
    </div>

    <Panel title="适配规则" sub="按执行顺序排列：通用 → 协议 → 模型 → 供应商 → 供应商 + 模型 → 自动学习" :count="filtered.length" flush>
      <template #toolbar>
        <el-radio-group v-model="fSource"><el-radio-button value="">全部</el-radio-button><el-radio-button v-for="(t, k) in SOURCE" :key="k" :value="k">{{ t.text }}</el-radio-button></el-radio-group>
        <el-select v-model="fProtocol" clearable placeholder="全部协议" style="width: 130px"><el-option v-for="k in ['openai', 'anthropic', 'gemini']" :key="k" :label="PROTOCOL[k].text" :value="k" /></el-select>
        <el-select v-model="fProvider" clearable filterable placeholder="全部供应商" style="width: 170px"><el-option v-for="p in state.providers" :key="p.id" :label="p.name" :value="p.id" /></el-select>
        <el-input v-model="keyword" placeholder="搜索规则 / 模型 / 参数" :prefix-icon="Search" clearable style="width: 220px" />
      </template>
      <el-table :data="lp.rows.value" v-loading="loading" empty-text="暂无规则">
        <el-table-column label="规则" min-width="230">
          <template #default="{ row }">
            <div class="cell-title">{{ row.name }}</div>
            <div class="cell-sub clamp2" :title="row.remark">{{ row.remark || '—' }}</div>
          </template>
        </el-table-column>
        <el-table-column label="来源" width="112"><template #default="{ row }"><StatusBadge :text="SOURCE[row.source].text" :tone="SOURCE[row.source].tone" /></template></el-table-column>
        <el-table-column label="作用范围" min-width="230">
          <template #default="{ row }">
            <div class="scope">
              <span class="chip">{{ row.protocol ? PROTOCOL[row.protocol].text : '任意协议' }}</span>
              <span class="chip" :class="{ strong: row.provider_id }">{{ row.provider_id ? providerName(row.provider_id) : '任意供应商' }}</span>
              <span class="chip mono" :class="{ strong: row.model_pattern }" :title="row.model_pattern">{{ row.model_pattern || '任意模型' }}</span>
              <span class="chip">{{ OP[row.op] }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="参数改写" min-width="240">
          <template #default="{ row }"><div class="acts"><code v-for="(a, i) in row.actions" :key="i">{{ actionText(a) }}</code></div></template>
        </el-table-column>
        <el-table-column label="命中" width="128" align="right">
          <template #default="{ row }"><div class="num">{{ fmtNum(row.hit_count) }}</div><div class="cell-sub">{{ row.last_hit_at ? fmtTime(row.last_hit_at) : '未命中' }}</div></template>
        </el-table-column>
        <el-table-column label="启用" width="72"><template #default="{ row }"><el-switch v-model="row.enabled" size="small" :disabled="!isSuper" @change="(v: string | number | boolean) => toggle(row, !!v)" /></template></el-table-column>
        <el-table-column label="操作" :width="isSuper ? 150 : 80" align="right" fixed="right" class-name="col-actions">
          <template #default="{ row }">
            <el-button link type="primary" @click="openPreview(row)">预览</el-button>
            <template v-if="isSuper">
              <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
              <el-popconfirm :title="row.source === 'builtin' ? '删除内置预设？（可改为停用）' : '删除该规则？'" width="220" @confirm="remove(row)"><template #reference><el-button link type="danger">删除</el-button></template></el-popconfirm>
            </template>
          </template>
        </el-table-column>
      </el-table>
      <TablePager v-model:page="lp.page.value" v-model:page-size="lp.pageSize.value" :total="lp.total.value" />
    </Panel>

    <!-- editor -->
    <el-dialog v-model="visible" :title="editing ? '编辑规则' : '新增规则'" width="760px" destroy-on-close>
      <el-form label-position="top">
        <div class="form-grid">
          <el-form-item label="规则名称" class="span2"><el-input v-model="form.name" maxlength="120" placeholder="如 Azure 旧版接口仍使用 max_tokens" /></el-form-item>
          <el-form-item label="供应商协议"><el-select v-model="form.protocol"><el-option label="任意协议" value="" /><el-option v-for="k in ['openai', 'anthropic', 'gemini']" :key="k" :label="PROTOCOL[k].text" :value="k" /></el-select></el-form-item>
          <el-form-item label="供应商"><el-select v-model="form.provider_id" clearable filterable placeholder="任意供应商"><el-option v-for="p in state.providers" :key="p.id" :label="p.name" :value="p.id" /></el-select></el-form-item>
          <el-form-item label="模型（上游模型名，支持 * 通配，逗号分隔）"><el-input v-model="form.model_pattern" class="mono-input" placeholder="如 gpt-6*, o3*；留空为任意模型" /></el-form-item>
          <el-form-item label="接口"><el-select v-model="form.op"><el-option v-for="(t, k) in OP" :key="k" :label="t" :value="k" /></el-select></el-form-item>
        </div>
        <el-form-item label="参数改写（按顺序执行；参数名支持 a.b 访问嵌套字段，如 generationConfig.maxOutputTokens）">
          <div class="actions-edit">
            <div v-for="(a, i) in form.actions" :key="i" class="arow">
              <el-select v-model="a.type" style="width: 120px"><el-option v-for="(t, k) in ACT" :key="k" :label="t" :value="k" /></el-select>
              <el-input v-model="a.field" class="mono-input" placeholder="参数名" style="width: 200px" />
              <template v-if="a.type === 'rename'"><span class="muted">改名为</span><el-input v-model="a.to" class="mono-input" placeholder="新参数名" style="width: 200px" /></template>
              <template v-else-if="a.type === 'set' || a.type === 'default'"><span class="muted">{{ a.type === 'set' ? '设为' : '缺省时设为' }}</span><el-input v-model="a.valueText" class="mono-input" placeholder='JSON 值，如 1024、"high"、true' style="width: 200px" /></template>
              <template v-else-if="a.type === 'clamp'"><span class="muted">限制在</span><el-input-number v-model="a.min" :controls="false" placeholder="最小" style="width: 92px" /><span class="muted">~</span><el-input-number v-model="a.max" :controls="false" placeholder="最大" style="width: 92px" /></template>
              <el-button link type="danger" :icon="Delete" :disabled="form.actions.length === 1" @click="form.actions.splice(i, 1)" />
            </div>
            <div class="abar">
              <el-button link type="primary" :icon="Plus" @click="form.actions.push(blankAction())">添加动作</el-button>
              <span class="muted">常用：</span>
              <el-button v-for="t in TEMPLATES" :key="t.label" size="small" @click="useTemplate(t)">{{ t.label }}</el-button>
            </div>
          </div>
        </el-form-item>
        <div class="form-grid">
          <el-form-item label="优先级（同一具体程度内，小的先执行）"><el-input-number v-model="form.priority" :min="1" :max="9999" controls-position="right" /></el-form-item>
          <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
          <el-form-item label="备注" class="span2"><el-input v-model="form.remark" type="textarea" :rows="2" resize="none" maxlength="500" placeholder="记录厂商变更说明、生效日期或工单号" /></el-form-item>
        </div>
      </el-form>
      <template #footer><el-button @click="visible = false">取消</el-button><el-button type="primary" :loading="saving" @click="save">保存</el-button></template>
    </el-dialog>

    <!-- preview -->
    <el-drawer v-model="pvVisible" title="规则预览" size="760px">
      <p class="hint">选择供应商与模型，查看网关发往该供应商的请求体会被如何改写（不会真正调用供应商）。</p>
      <el-form label-position="top">
        <div class="form-grid">
          <el-form-item label="供应商"><el-select v-model="pv.provider_id" filterable placeholder="选择供应商"><el-option v-for="p in state.providers" :key="p.id" :label="`${p.name}（${PROTOCOL[p.protocol || 'openai'].text}）`" :value="p.id" /></el-select></el-form-item>
          <el-form-item label="上游模型名"><el-autocomplete v-model="pv.model" :fetch-suggestions="suggestModels" placeholder="如 gpt-6.1-sol" clearable style="width: 100%" /></el-form-item>
          <el-form-item label="接口"><el-select v-model="pv.op"><el-option v-for="k in ['chat', 'embedding', 'image', 'video']" :key="k" :label="OP[k]" :value="k" /></el-select></el-form-item>
        </div>
        <el-form-item label="请求体（网关转换后、改写前的 JSON）"><el-input v-model="pv.body" type="textarea" :rows="8" resize="vertical" class="mono-input" /></el-form-item>
        <el-button type="primary" :loading="previewing" :disabled="!pv.provider_id || !pv.model" @click="runPreview">预览改写结果</el-button>
      </el-form>
      <div v-if="pvResult" class="pvres">
        <h5>命中规则（{{ pvResult.rules.length }}）</h5>
        <div v-if="pvResult.applied.length" class="acts"><code v-for="(a, i) in pvResult.applied" :key="i">{{ a }}</code></div>
        <div v-else class="faint">没有规则改写该请求</div>
        <h5>发往供应商的请求体</h5>
        <pre class="json">{{ JSON.stringify(pvResult.body, null, 2) }}</pre>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Delete, Plus, Right, Search, View } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import KpiCard from '../../components/KpiCard.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import TablePager from '../../components/TablePager.vue'
import { paramRules } from '../../api'
import { useAuth } from '../../composables/useAuth'
import { useLookups } from '../../composables/useLookups'
import { useLocalPage } from '../../composables/usePaged'
import { PROTOCOL } from '../../constants'
import { fmtNum, fmtTime } from '../../utils'
import type { ParamAction, ParamActionType, ParamPreview, ParamRule } from '../../types'

const { isSuper } = useAuth()
const { state } = useLookups()

const SOURCE: Record<string, { text: string; tone: 'primary' | 'accent' | 'warning' }> = {
  builtin: { text: '内置预设', tone: 'primary' },
  manual: { text: '手动', tone: 'accent' },
  auto: { text: '自动学习', tone: 'warning' },
}
const OP: Record<string, string> = { '': '全部接口', chat: '对话', embedding: '向量', image: '图片生成', video: '视频生成' }
const ACT: Record<ParamActionType, string> = { rename: '改名', drop: '去掉', set: '强制设为', default: '缺省值', clamp: '数值范围' }

const rows = ref<ParamRule[]>([])
const loading = ref(false)
const fSource = ref('')
const fProtocol = ref('')
const fProvider = ref<number>()
const keyword = ref('')
const lastHit = computed(() => rows.value.map((r) => r.last_hit_at).filter(Boolean).sort().pop() ?? '')
const providerName = (id: number) => state.providers.find((p) => p.id === id)?.name ?? `#${id}`
const filtered = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  return rows.value.filter((r) => (!fSource.value || r.source === fSource.value) && (!fProtocol.value || r.protocol === fProtocol.value) &&
    (!fProvider.value || r.provider_id === fProvider.value) &&
    (!q || [r.name, r.model_pattern, r.remark, ...r.actions.map(actionText)].some((s) => s.toLowerCase().includes(q))))
})
const lp = useLocalPage(filtered)

function actionText(a: ParamAction): string {
  switch (a.type) {
    case 'rename': return `${a.field} → ${a.to}`
    case 'drop': return `去掉 ${a.field}`
    case 'set': return `${a.field} = ${JSON.stringify(a.value)}`
    case 'default': return `${a.field} 缺省 ${JSON.stringify(a.value)}`
    case 'clamp': return `${a.field} ∈ [${a.min ?? '-∞'}, ${a.max ?? '∞'}]`
  }
  return a.type
}

async function load() {
  loading.value = rows.value.length === 0
  try { rows.value = await paramRules.list() } finally { loading.value = false }
}
onMounted(load)

// ---- editor ----
type EditAction = { type: ParamActionType; field: string; to: string; valueText: string; min?: number; max?: number }
const blankAction = (): EditAction => ({ type: 'rename', field: '', to: '', valueText: '' })
const TEMPLATES: { label: string; actions: EditAction[] }[] = [
  { label: 'max_tokens → max_completion_tokens', actions: [{ type: 'rename', field: 'max_tokens', to: 'max_completion_tokens', valueText: '' }] },
  { label: 'max_completion_tokens → max_tokens', actions: [{ type: 'rename', field: 'max_completion_tokens', to: 'max_tokens', valueText: '' }] },
  { label: '去掉 temperature / top_p', actions: [{ type: 'drop', field: 'temperature', to: '', valueText: '' }, { type: 'drop', field: 'top_p', to: '', valueText: '' }] },
  { label: '缺省 reasoning_effort', actions: [{ type: 'default', field: 'reasoning_effort', to: '', valueText: '"medium"' }] },
]
const visible = ref(false)
const saving = ref(false)
const editing = ref<ParamRule | null>(null)
const form = reactive({ name: '', protocol: '' as string, provider_id: undefined as number | undefined, model_pattern: '', op: 'chat', actions: [blankAction()], priority: 100, enabled: true, remark: '' })

function useTemplate(t: { actions: EditAction[] }) {
  const empty = form.actions.length === 1 && !form.actions[0].field
  form.actions = [...(empty ? [] : form.actions), ...t.actions.map((a) => ({ ...a }))]
}
function openCreate() {
  editing.value = null
  Object.assign(form, { name: '', protocol: 'openai', provider_id: undefined, model_pattern: '', op: 'chat', actions: [blankAction()], priority: 100, enabled: true, remark: '' })
  visible.value = true
}
function openEdit(r: ParamRule) {
  editing.value = r
  Object.assign(form, {
    name: r.name, protocol: r.protocol, provider_id: r.provider_id ?? undefined, model_pattern: r.model_pattern, op: r.op, priority: r.priority, enabled: r.enabled, remark: r.remark,
    actions: r.actions.map((a) => ({ type: a.type, field: a.field, to: a.to ?? '', valueText: a.value === undefined ? '' : JSON.stringify(a.value), min: a.min ?? undefined, max: a.max ?? undefined })),
  })
  visible.value = true
}
function toActions(): ParamAction[] | null {
  const out: ParamAction[] = []
  for (const [i, a] of form.actions.entries()) {
    const act: ParamAction = { type: a.type, field: a.field.trim() }
    if (a.type === 'rename') act.to = a.to.trim()
    if (a.type === 'set' || a.type === 'default') {
      try { act.value = JSON.parse(a.valueText) } catch { ElMessage.error(`第 ${i + 1} 个动作的值不是合法 JSON（字符串需加引号，如 "high"）`); return null }
    }
    if (a.type === 'clamp') { act.min = a.min ?? null; act.max = a.max ?? null }
    out.push(act)
  }
  return out
}
async function save() {
  const actions = toActions()
  if (!actions) return
  saving.value = true
  try {
    const body = { name: form.name, protocol: form.protocol, provider_id: form.provider_id ?? 0, model_pattern: form.model_pattern, op: form.op, actions, priority: form.priority, enabled: form.enabled, remark: form.remark }
    if (editing.value) await paramRules.update(editing.value.id, body)
    else await paramRules.create(body)
    ElMessage.success('已保存，10 秒内在所有节点生效')
    visible.value = false
    await load()
  } finally { saving.value = false }
}
async function toggle(r: ParamRule, on: boolean) {
  try { await paramRules.update(r.id, { enabled: on }); ElMessage.success(on ? '已启用' : '已停用') } catch { r.enabled = !on }
}
async function remove(r: ParamRule) { await paramRules.remove(r.id); ElMessage.success('已删除'); await load() }

// ---- preview ----
const SAMPLE = { model: 'gpt-6.1-sol', messages: [{ role: 'user', content: '你好' }], max_tokens: 1024, temperature: 0.7 }
const pvVisible = ref(false)
const previewing = ref(false)
const pv = reactive({ provider_id: undefined as number | undefined, model: '', op: 'chat', body: '' })
const pvResult = ref<ParamPreview | null>(null)
function openPreview(r?: ParamRule) {
  pvResult.value = null
  pv.provider_id = r?.provider_id ?? pv.provider_id ?? state.providers[0]?.id
  // a concrete example of the rule's pattern: "gpt-6*, o3*" → "gpt-6"
  const first = r?.model_pattern.split(',')[0].trim().replace(/[*?].*$/, '')
  pv.model = first || pv.model || 'gpt-6.1-sol'
  pv.op = r?.op || 'chat'
  pv.body = JSON.stringify({ ...SAMPLE, model: pv.model }, null, 2)
  pvVisible.value = true
}
function suggestModels(q: string, cb: (x: { value: string }[]) => void) {
  const ms = state.models.filter((m) => !pv.provider_id || m.provider_id === pv.provider_id).map((m) => m.model_key)
  cb([...new Set(ms)].filter((m) => m.toLowerCase().includes(q.toLowerCase())).slice(0, 20).map((value) => ({ value })))
}
async function runPreview() {
  let body: object
  try { body = JSON.parse(pv.body) } catch { ElMessage.error('请求体不是合法的 JSON'); return }
  previewing.value = true
  try { pvResult.value = await paramRules.preview({ provider_id: pv.provider_id!, model: pv.model.trim(), op: pv.op, body }) } finally { previewing.value = false }
}
</script>

<style scoped>
.flow { display: flex; align-items: stretch; gap: 10px; margin-bottom: 14px; }
.step { flex: 1; display: flex; flex-direction: column; gap: 2px; padding: 10px 14px; background: var(--card); border: 1px solid var(--border); border-radius: var(--radius); font-size: 12.5px; color: var(--text-2); }
.step b { color: var(--text); font-size: 13px; font-weight: 600; }
.arrow { align-self: center; color: var(--text-3); }
.kpis { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin-bottom: 16px; }
@media (max-width: 1100px) { .kpis { grid-template-columns: repeat(2, 1fr); } .flow { flex-direction: column; } .arrow { display: none; } }
.scope { display: flex; flex-wrap: wrap; gap: 4px; }
.chip { padding: 1px 7px; border-radius: 4px; background: #F3F4F6; color: var(--text-2); font-size: 12px; max-width: 220px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.chip.strong { background: var(--primary-soft); color: #1D4ED8; }
.acts { display: flex; flex-wrap: wrap; gap: 4px; }
.acts code { padding: 1px 7px; border-radius: 4px; background: #F5F3FF; color: #6D28D9; font-family: var(--mono); font-size: 12px; }
.clamp2 { display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.actions-edit { width: 100%; display: grid; gap: 8px; }
.arow { display: flex; align-items: center; gap: 8px; }
.abar { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.abar .el-button + .el-button { margin-left: 0; }
.mono-input :deep(input), .mono-input :deep(textarea) { font-family: var(--mono); font-size: 12px; }
.hint { font-size: 12.5px; color: var(--text-2); margin: 0 0 12px; }
.pvres { margin-top: 16px; }
h5 { margin: 14px 0 8px; font-size: 13px; font-weight: 600; }
pre.json { margin: 0; padding: 12px 14px; border-radius: 8px; background: #0B1220; color: #E2E8F0; font-family: var(--mono); font-size: 12px; line-height: 1.6; overflow: auto; max-height: 420px; }
</style>
