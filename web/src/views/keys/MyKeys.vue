<template>
  <div>
    <PageHeader title="我的 Key" desc="个人编码 Key：领取密钥、查看用量，并按指引接入 Claude Code、Cursor、Cline 等编码工具">
      <el-button v-if="!openKey && canApply" type="primary" :icon="Plus" @click="$router.push('/keys/apply')">申请个人编码 Key</el-button>
    </PageHeader>

    <Panel v-if="!loading && !items.length">
      <EmptyState text="你还没有个人编码 Key" :hint="canApply ? '点击右上角申请，部门管理员审批后即可在这里领取密钥' : '你的账号未归属部门，请联系部门管理员代为申请'" height="220px" />
    </Panel>

    <div v-else v-loading="loading" class="keys">
      <Panel v-for="k in items" :key="k.id" flush>
        <div class="kh">
          <div>
            <div class="kname">{{ k.name }} <StatusBadge :text="statusLabel(k)" :tone="statusTone(k)" /></div>
            <div class="cell-sub">
              <span v-if="hasSecret(k)" class="mono">{{ k.key_prefix }}…</span><span v-else>尚未领取密钥</span>
              · {{ deptName(k.department_id) }} · 申请于 {{ fmtTime(k.created_at) }}
            </div>
          </div>
          <div class="acts">
            <el-button v-if="k.status === 'active' && !hasSecret(k)" type="primary" :icon="Key" :loading="busy === k.id" @click="claim(k)">领取密钥</el-button>
            <el-button v-if="k.status === 'active' && hasSecret(k)" :icon="RefreshRight" :loading="busy === k.id" @click="rotate(k)">重置密钥</el-button>
          </div>
        </div>
        <div class="kb">
          <div><span class="lab">可用模型</span><span v-if="k.allowed_models?.length" class="chips"><code v-for="m in k.allowed_models" :key="m">{{ m }}</code></span><span v-else>全部已上架模型</span></div>
          <div><span class="lab">编码工具</span>{{ k.coding_tools?.join('、') || '—' }}</div>
          <div><span class="lab">配额</span><span class="num">TPM {{ k.tpm_quota ? fmtNum(k.tpm_quota) : '不限' }} · QPS {{ k.qps_quota || '不限' }}</span></div>
          <div v-if="k.ip_whitelist?.length"><span class="lab">IP 白名单</span><span class="mono">{{ k.ip_whitelist.join(', ') }}</span></div>
          <div v-if="k.status === 'pending'" class="note">等待部门管理员审批，审批通过后在此领取密钥。</div>
          <div v-if="k.status === 'blacklisted'" class="note bad">已被拉黑：{{ k.blacklist_reason || '—' }}</div>
        </div>
        <div v-if="k.status === 'active' && usage[k.id]?.length" class="usage">
          <div class="lab">近 7 天用量</div>
          <el-table :data="usage[k.id]" size="small">
            <el-table-column label="模型" min-width="180"><template #default="{ row }">{{ modelLabel(Number(row.group_key)) }}</template></el-table-column>
            <el-table-column label="调用" width="90" align="right"><template #default="{ row }"><span class="num">{{ fmtNum(row.requests) }}</span></template></el-table-column>
            <el-table-column label="Token" width="110" align="right"><template #default="{ row }"><span class="num">{{ compact(row.total_tokens) }}</span></template></el-table-column>
            <el-table-column label="成本" width="110" align="right"><template #default="{ row }"><span class="num">{{ fmtMoney(row.cost) }}</span></template></el-table-column>
          </el-table>
        </div>
      </Panel>
    </div>

    <Panel title="编码工具接入指引" sub="网关兼容 OpenAI 协议；把下面的地址与密钥配置到编码工具即可，模型名使用「可用模型」中的名称" class="guide">
      <div class="base">
        <span class="lab">Base URL</span><code class="mono">{{ baseURL }}</code><el-button link :icon="CopyDocument" @click="copy(baseURL)" />
        <span class="lab" style="margin-left: 16px">模型示例</span><code class="mono">{{ sampleModel }}</code>
      </div>
      <el-tabs v-model="tool">
        <el-tab-pane v-for="g in guides" :key="g.name" :label="g.name" :name="g.name">
          <ol class="gsteps"><li v-for="s in g.steps" :key="s" v-html="s" /></ol>
          <div v-if="g.code" class="code"><button type="button" @click="copy(g.code)">复制</button><pre>{{ g.code }}</pre></div>
        </el-tab-pane>
      </el-tabs>
      <div class="hint">密钥请勿提交到代码仓库或分享给他人；泄露后请立即「重置密钥」。个人编码 Key 的调用计入个人与部门成本。</div>
    </Panel>

    <SecretDialog :secret="secret" title="你的个人编码 Key" @close="secret = ''" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CopyDocument, Key, Plus, RefreshRight } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import EmptyState from '../../components/EmptyState.vue'
import SecretDialog from '../../components/SecretDialog.vue'
import { keys, monitor } from '../../api'
import { useAuth } from '../../composables/useAuth'
import { useLookups } from '../../composables/useLookups'
import { compact } from '../../charts'
import { fmtDateTime, fmtMoney, fmtNum, fmtTime } from '../../utils'
import type { ApiKey, RankRow } from '../../types'

const { state: auth } = useAuth()
const { deptName, modelLabel } = useLookups()
const canApply = computed(() => !!auth.me?.permissions.apply_personal)

const items = ref<ApiKey[]>([])
const loading = ref(false)
const busy = ref<number>()
const secret = ref('')
const usage = reactive<Record<number, RankRow[]>>({})

const hasSecret = (k: ApiKey) => k.key_prefix.startsWith('sk-')
const statusLabel = (k: ApiKey) => (k.status === 'active' && !hasSecret(k) ? '待领取' : { pending: '待审批', active: '生效中', blacklisted: '已拉黑' }[k.status])
const statusTone = (k: ApiKey) => (k.status === 'active' && !hasSecret(k) ? 'primary' : ({ pending: 'warning', active: 'success', blacklisted: 'danger' } as const)[k.status])
const openKey = computed(() => items.value.find((k) => k.category === 'personal' && k.status !== 'blacklisted'))

async function load() {
  loading.value = true
  try {
    items.value = (await keys.mine({ page: 1, page_size: 20 })).items
    const to = new Date(), from = new Date(to.getTime() - 7 * 86400e3)
    await Promise.all(items.value.filter((k) => k.status === 'active').map(async (k) => {
      usage[k.id] = await monitor.ranking({ group_by: 'model', key_id: k.id, from: fmtDateTime(from), to: fmtDateTime(to) }).catch(() => [])
    }))
  } finally { loading.value = false }
}
onMounted(load)

async function claim(k: ApiKey) {
  busy.value = k.id
  try { secret.value = (await keys.claim(k.id)).secret; await load() } finally { busy.value = undefined }
}
async function rotate(k: ApiKey) {
  await ElMessageBox.confirm('重置后旧密钥立即失效，所有编码工具需要换成新密钥。确认重置？', '重置密钥', { type: 'warning' })
  busy.value = k.id
  try { const r = await keys.rotate(k.id); secret.value = r.secret ?? ''; await load() } finally { busy.value = undefined }
}
async function copy(s: string) { await navigator.clipboard.writeText(s); ElMessage.success('已复制') }

// ---- integration guides --------------------------------------------------------------------
const baseURL = `${location.origin}/v1`
const sampleModel = computed(() => openKey.value?.allowed_models?.[0] ?? 'deepseek-v3')
const tool = ref('Cline')
const KEY = 'sk-你的密钥'
const esc = (v: string) => v.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!)
const guides = computed(() => {
  // m is interpolated into v-html steps: escape it (model names are admin-editable)
  const m = esc(sampleModel.value)
  return [
    { name: 'Cline', steps: ['VS Code 打开 Cline 设置 → <b>API Provider</b> 选择 <b>OpenAI Compatible</b>', `<b>Base URL</b> 填 <code>${baseURL}</code>，<b>API Key</b> 填你的密钥`, `<b>Model ID</b> 填 <code>${m}</code>（可用模型中的任一名称）`], code: '' },
    { name: 'Continue', steps: ['编辑 <code>~/.continue/config.yaml</code>，加入以下模型配置', '保存后在 Continue 面板选择该模型'],
      code: `models:\n  - name: ${m}（企业网关）\n    provider: openai\n    model: ${m}\n    apiBase: ${baseURL}\n    apiKey: ${KEY}\n    roles: [chat, edit, apply]` },
    { name: 'Cursor', steps: ['Cursor Settings → <b>Models</b> → 打开 <b>OpenAI API Key</b>，填入你的密钥', `展开 <b>Override OpenAI Base URL</b>，填 <code>${baseURL}</code>`, `在模型列表中 <b>Add model</b>：<code>${m}</code>，然后点击 Verify`], code: '' },
    { name: 'Codex CLI', steps: ['编辑 <code>~/.codex/config.toml</code>，并在 shell 中 <code>export LLMGW_API_KEY=你的密钥</code>'],
      code: `model = "${m}"\nmodel_provider = "llmgw"\n\n[model_providers.llmgw]\nname = "LLM Gateway"\nbase_url = "${baseURL}"\nenv_key = "LLMGW_API_KEY"\nwire_api = "chat"` },
    { name: 'Claude Code', steps: ['Claude Code 使用 Anthropic 协议，需通过 OpenAI 协议转接工具接入，例如 <code>claude-code-router</code>（<code>npm i -g @musistudio/claude-code-router</code>）', '编辑 <code>~/.claude-code-router/config.json</code>，然后用 <code>ccr code</code> 启动'],
      code: JSON.stringify({ Providers: [{ name: 'llmgw', api_base_url: `${baseURL}/chat/completions`, api_key: KEY, models: openKey.value?.allowed_models?.length ? openKey.value.allowed_models : [m] }], Router: { default: `llmgw,${m}` } }, null, 2) },
    { name: 'Aider', steps: ['命令行直接指定网关地址与模型'], code: `aider --openai-api-base ${baseURL} --openai-api-key ${KEY} --model openai/${m}` },
    { name: 'OpenAI SDK', steps: ['脚本或自研工具中使用官方 SDK，只需替换 base_url'],
      code: `from openai import OpenAI\n\nclient = OpenAI(base_url="${baseURL}", api_key="${KEY}")\nresp = client.chat.completions.create(model="${m}", messages=[{"role": "user", "content": "用 Go 写一个 LRU 缓存"}])\nprint(resp.choices[0].message.content)` },
  ]
})
</script>

<style scoped>
.keys { display: grid; gap: 12px; margin-bottom: 16px; }
.kh { display: flex; justify-content: space-between; align-items: center; gap: 12px; padding: 14px 16px; border-bottom: 1px solid var(--border-soft); }
.kname { font-weight: 600; font-size: 15px; display: flex; align-items: center; gap: 8px; }
.kb { display: grid; gap: 6px; padding: 12px 16px; font-size: 13px; }
.lab { display: inline-block; min-width: 72px; color: var(--text-2); font-size: 12.5px; }
.chips { display: inline-flex; flex-wrap: wrap; gap: 4px; }
.chips code, .base code { padding: 1px 6px; border-radius: 4px; background: #F3F4F6; font-size: 12px; }
.note { color: var(--text-2); font-size: 12.5px; }
.note.bad { color: var(--danger); }
.usage { padding: 0 16px 14px; }
.guide { margin-top: 4px; }
.base { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; margin-bottom: 8px; }
.gsteps { margin: 4px 0 10px; padding-left: 18px; line-height: 1.9; font-size: 13px; }
.gsteps :deep(code) { padding: 1px 5px; border-radius: 4px; background: #F3F4F6; font-size: 12px; }
.code { position: relative; margin-bottom: 10px; }
.code pre { margin: 0; padding: 12px 14px; background: #0F172A; color: #E2E8F0; border-radius: 8px; font: 12px/1.6 var(--mono); overflow-x: auto; }
.code button { position: absolute; top: 8px; right: 10px; border: 1px solid #334155; background: #1E293B; color: #CBD5E1; border-radius: 4px; font-size: 11.5px; padding: 1px 8px; cursor: pointer; }
</style>
