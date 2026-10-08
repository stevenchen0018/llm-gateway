<template>
  <div>
    <PageHeader title="API 文档" desc="OpenAI 兼容的统一大模型接入文档：鉴权、接口定义、错误码、限流规则、多语言示例与在线调试">
      <el-button :icon="Download" @click="openSpec">OpenAPI 规范</el-button>
      <el-button type="primary" :icon="Key" @click="$router.push('/keys/apply')">申请 API Key</el-button>
    </PageHeader>

    <div v-if="!spec" v-loading="true" style="height: 320px" />
    <div v-else class="docs">
      <nav class="toc">
        <div class="tgroup">接入指南</div>
        <a v-for="s in GUIDE" :key="s.id" :class="{ on: active === s.id }" @click="go(s.id)">{{ s.title }}</a>
        <div class="tgroup">接口</div>
        <a v-for="e in eps" :key="e.op.operationId" :class="{ on: active === e.op.operationId }" @click="go(e.op.operationId)">
          <span class="m" :class="e.method.toLowerCase()">{{ e.method }}</span>{{ e.op.summary }}
        </a>
      </nav>

      <div ref="bodyRef" class="body">
        <!-- quick start -->
        <Panel id="quickstart" title="快速开始">
          <ol class="steps">
            <li><b>申请 API Key</b><span>在「API Key · Key 申请」提交工单，审批通过后获得 <code>sk-</code> 开头的密钥（只展示一次）。正式 Key 需申请预算后受成本管控。</span></li>
            <li><b>配置 Base URL</b><span>把 OpenAI SDK 的 <code>base_url</code> 指向网关，其余代码不变：</span></li>
          </ol>
          <div class="kv"><span>Base URL</span><code>{{ base }}</code><el-button link type="primary" @click="copy(base)">复制</el-button></div>
          <ol class="steps" start="3"><li><b>发起调用</b><span>业务只需携带模型名；厂商匹配、调度策略、容灾切换与计费由网关完成。</span></li></ol>
          <CodeTabs :codes="quickCodes" />
          <p class="hint" v-html="md(spec.info.description.split('\n\n')[1] ?? '')" />
        </Panel>

        <Panel id="auth" title="鉴权">
          <p>所有接口使用 HTTP Bearer 鉴权：<code>Authorization: Bearer sk-xxxxxxxx</code>。</p>
          <ul class="bullets">
            <li>Key 被拉黑、未审批、已过期（试用 Key 7 天）或所属部门停用时，请求会被拒绝（401 / 403）。</li>
            <li>Key 归属于某个应用和部门；调用量、成本与告警均按部门隔离统计。</li>
            <li>请勿在前端或客户端代码中暴露 Key；泄露后请联系管理员拉黑并重新申请。</li>
          </ul>
        </Panel>

        <Panel id="routing" title="模型名与路由">
          <ul class="bullets">
            <li><code>model</code> 传模型名（如 <code>deepseek-r1</code>），网关先匹配该模型的厂商与提供它的供应商，再按你的 Key 命中调度策略（Key 专属 &gt; 全局 &gt; 厂商的供应商选择）。</li>
            <li>同一模型可由多家供应商提供：传 <code>供应商/模型</code>（如 <code>aliyun/deepseek-r1</code>）可固定调用渠道，传 <code>厂商/模型</code> 可限定模型厂商；不指定时按调度策略或厂商的供应商选择方式路由，并在供应商间自动容灾。</li>
            <li><code>stream: true</code> 以 SSE 返回（兼容 Cursor、Cline、Continue 等编码工具），个人编码 Key 的工具配置见「API Key · 我的 Key」。</li>
            <li>候选厂商调用失败或不健康时自动切换到下一个候选，响应中的 <code>model</code> 保持你请求的名称。</li>
            <li>每次响应都带 <code>X-Request-Id</code>，排查问题时提供给管理员，可在「调用日志」检索。</li>
          </ul>
        </Panel>

        <Panel id="limits" title="限流与配额" flush>
          <el-table :data="LIMITS" size="small">
            <el-table-column label="维度" width="120" prop="dim" />
            <el-table-column label="作用范围" min-width="220" prop="scope" />
            <el-table-column label="超限表现" min-width="240" prop="effect" />
          </el-table>
          <div class="pad">
            <p>QPS 按秒、TPM 按自然分钟的固定窗口计算。TPM 在请求前按 <code>输入估算 + max_tokens</code> 预占，响应后按实际 <code>usage.total_tokens</code> 结算退还——<b>请按需设置 <code>max_tokens</code></b>，过大会提前占满 TPM。</p>
            <p>收到 <code>429</code> 时请使用指数退避重试（例如 1s、2s、4s，最多 3–5 次），不要立即重试。</p>
          </div>
        </Panel>

        <Panel id="errors" title="错误码" flush>
          <el-table :data="errorRows" size="small">
            <el-table-column label="HTTP" width="80"><template #default="{ row }"><span class="num">{{ row.status }}</span></template></el-table-column>
            <el-table-column label="code" width="200"><template #default="{ row }"><code>{{ row.code }}</code></template></el-table-column>
            <el-table-column label="说明" min-width="300" prop="desc" />
            <el-table-column label="建议处理" min-width="220" prop="action" />
          </el-table>
          <div class="pad"><p>错误响应体与 OpenAI 一致：</p><pre class="json">{{ JSON.stringify({ error: { message: 'Rate limit exceeded, please retry later', type: 'rate_limit_error', code: 'rate_limited' } }, null, 2) }}</pre></div>
        </Panel>

        <!-- endpoints -->
        <Panel v-for="e in eps" :id="e.op.operationId" :key="e.op.operationId" class="ep">
          <template #extra><el-button type="primary" plain size="small" :icon="VideoPlay" @click="openTry(e)">在线调试</el-button></template>
          <div class="ephead"><span class="m big" :class="e.method.toLowerCase()">{{ e.method }}</span><code class="path">/v1{{ e.path }}</code><span class="sum">{{ e.op.summary }}</span></div>
          <p class="desc" v-html="md(e.op.description ?? '')" />

          <template v-if="reqSchema(e)">
            <h5>请求参数（application/json）</h5>
            <el-table :data="flatten(spec, reqSchema(e))" size="small" class="params">
              <el-table-column label="参数" min-width="170"><template #default="{ row }"><code>{{ row.name }}</code></template></el-table-column>
              <el-table-column label="类型" min-width="150"><template #default="{ row }"><span class="type">{{ row.type }}</span></template></el-table-column>
              <el-table-column label="必填" width="70"><template #default="{ row }"><StatusBadge v-if="row.required" text="必填" tone="danger" /><span v-else class="faint">可选</span></template></el-table-column>
              <el-table-column label="说明" min-width="260"><template #default="{ row }"><span v-html="md(row.description)" /></template></el-table-column>
            </el-table>
          </template>

          <h5>响应字段（200）</h5>
          <el-table :data="flatten(spec, okSchema(e))" size="small" class="params">
            <el-table-column label="字段" min-width="220"><template #default="{ row }"><code>{{ row.name }}</code></template></el-table-column>
            <el-table-column label="类型" min-width="120"><template #default="{ row }"><span class="type">{{ row.type }}</span></template></el-table-column>
            <el-table-column label="说明" min-width="260" prop="description" />
          </el-table>

          <div class="examples">
            <div><h5>示例代码</h5><CodeTabs :codes="codesFor(e)" /></div>
            <div><h5>响应示例</h5><pre class="json">{{ JSON.stringify(okExample(e), null, 2) }}</pre></div>
          </div>

          <h5>错误响应</h5>
          <div class="errs">
            <span v-for="(r, code) in errorsOf(e)" :key="code" class="err"><b class="num">{{ code }}</b><span v-html="md(r.description)" /></span>
          </div>
        </Panel>
      </div>
    </div>

    <!-- try-it console -->
    <el-drawer v-model="tryVisible" :title="`在线调试 · ${tryEp?.method} /v1${tryEp?.path}`" size="640px">
      <el-form label-position="top">
        <el-form-item label="API Key">
          <el-input v-model="tryKey" type="password" show-password placeholder="sk-...（仅保存在本次浏览器会话中）" @change="saveKey" />
          <div class="hint">请求直接发往网关 <code>{{ base }}</code>，会真实计入该 Key 的配额、预算与调用日志。</div>
        </el-form-item>
        <el-form-item v-if="tryEp?.method !== 'GET'" label="请求体（JSON）">
          <el-input v-model="tryBody" type="textarea" :rows="12" resize="vertical" class="mono-input" />
        </el-form-item>
        <el-button type="primary" :loading="trying" :disabled="!tryKey" :icon="Promotion" @click="send">发送请求</el-button>
        <el-button @click="resetBody">恢复示例</el-button>
      </el-form>
      <div v-if="tryRes" class="result">
        <div class="rhead">
          <StatusBadge :text="`HTTP ${tryRes.status}`" :tone="tryRes.status < 300 ? 'success' : tryRes.status === 429 ? 'warning' : 'danger'" />
          <span class="num muted">{{ tryRes.ms }} ms</span>
          <span v-if="tryRes.requestId" class="muted">X-Request-Id <code>{{ tryRes.requestId }}</code></span>
        </div>
        <pre class="json">{{ tryRes.body }}</pre>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Download, Key, Promotion, VideoPlay } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import { LANGS, type Endpoint, type Lang, type Resp, type Spec, endpoints, flatten, resolveResp, sample } from './openapi'

const GUIDE = [
  { id: 'quickstart', title: '快速开始' }, { id: 'auth', title: '鉴权' }, { id: 'routing', title: '模型名与路由' },
  { id: 'limits', title: '限流与配额' }, { id: 'errors', title: '错误码' },
]
const LIMITS = [
  { dim: 'Key 配额', scope: '单个 API Key 的 QPS / TPM，覆盖它调用的所有模型', effect: '429 rate_limited' },
  { dim: '部门配额', scope: 'Key 所属部门下全部 Key 合计的 QPS / TPM', effect: '429 rate_limited' },
  { dim: '模型容量', scope: '某厂商模型对所有 Key 合计的 QPS / TPM', effect: '自动切换到下一个候选；全部饱和时 429' },
  { dim: '预算', scope: 'Key 绑定的已审批预算（按实际成本扣减）', effect: '耗尽后 402 budget_exceeded' },
]
const ERRORS = [
  { status: 400, code: 'invalid_argument', desc: '参数错误，或该模型名没有可用路由', action: '检查 model 与请求体；在模型市场确认模型名' },
  { status: 400, code: 'content_blocked', desc: '提示词命中了平台 / 部门的内容过滤规则，请求未发送给厂商且不计费', action: '修改提示词；如属误拦截联系部门管理员' },
  { status: 401, code: 'invalid_api_key', desc: 'Key 无效、未审批、已拉黑或已过期', action: '检查 Key；联系管理员' },
  { status: 402, code: 'budget_exceeded', desc: 'Key 绑定的预算已耗尽', action: '在「预算管理」申请追加预算' },
  { status: 403, code: 'model_not_allowed', desc: '请求的模型不在该 Key 的可用模型名单中（个人编码 Key 默认仅开放编码模型）', action: '改用名单内模型，或请部门管理员在「Key 管理」调整可用模型' },
  { status: 403, code: 'ip_not_allowed', desc: '调用方 IP 不在该 Key 的 IP 白名单内', action: '从白名单内的机器调用，或请管理员在「Key 管理」添加 IP / 网段' },
  { status: 403, code: 'department_disabled', desc: 'Key 所属部门已停用', action: '联系平台管理员' },
  { status: 429, code: 'rate_limited', desc: 'Key / 部门 / 模型的 QPS 或 TPM 超限', action: '指数退避重试；申请提升配额' },
  { status: 502, code: 'upstream_error', desc: '所有候选厂商均调用失败', action: '稍后重试；提供 X-Request-Id 排查' },
  { status: 503, code: 'no_healthy_candidate', desc: '候选模型均处于熔断冷却中', action: '稍后重试（通常 1 分钟内恢复）' },
]

// small tabbed code viewer
const CodeTabs = defineComponent({
  props: { codes: { type: Object as () => Record<string, string>, required: true } },
  setup(props) {
    const tab = ref<string>(Object.keys(props.codes)[0])
    return () => h('div', { class: 'codetabs' }, [
      h('div', { class: 'tabs' }, [
        ...Object.keys(props.codes).map((k) => h('button', { type: 'button', class: { on: tab.value === k }, onClick: () => (tab.value = k) }, k)),
        h('button', { type: 'button', class: 'cp', onClick: () => copy(props.codes[tab.value]) }, '复制'),
      ]),
      h('pre', null, props.codes[tab.value]),
    ])
  },
})

const spec = ref<Spec | null>(null)
const base = computed(() => spec.value?.servers?.[0]?.url ?? `${location.origin}/v1`)
const eps = computed(() => (spec.value ? endpoints(spec.value) : []))
const errorRows = ERRORS

onMounted(async () => {
  const res = await fetch('/openapi.json')
  spec.value = await res.json()
})

const reqContent = (e: Endpoint) => e.op.requestBody?.content['application/json']
const reqSchema = (e: Endpoint) => reqContent(e)?.schema
const okResp = (e: Endpoint) => e.op.responses['200'].content?.['application/json']
const okSchema = (e: Endpoint) => okResp(e)?.schema
const okExample = (e: Endpoint) => okResp(e)?.example
const errorsOf = (e: Endpoint) => Object.fromEntries(Object.entries(e.op.responses).filter(([c]) => c !== '200').map(([c, r]) => [c, resolveResp(spec.value!, r as Resp)]))
const codesFor = (e: Endpoint) => Object.fromEntries(LANGS.map((l: Lang) => [l, sample(l, base.value, e, reqContent(e)?.example)]))
const quickCodes = computed(() => {
  const chat = eps.value.find((e) => e.op.operationId === 'createChatCompletion')
  if (!chat) return {}
  const body = { model: 'deepseek-r1', messages: [{ role: 'user', content: '你好' }] }
  return Object.fromEntries((['Python', 'Node.js', 'cURL'] as Lang[]).map((l) => [l, sample(l, base.value, chat, body)]))
})

// ---- table of contents: scroll + active section --------------------------------------
const active = ref('quickstart')
const bodyRef = ref<HTMLElement>()
function go(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  active.value = id
}
let observer: IntersectionObserver | undefined
onMounted(() => {
  const setup = () => {
    if (!bodyRef.value) return void setTimeout(setup, 100)
    observer = new IntersectionObserver((entries) => {
      const v = entries.filter((x) => x.isIntersecting).sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)[0]
      if (v) active.value = v.target.id
    }, { rootMargin: '-10% 0px -70% 0px' })
    bodyRef.value.querySelectorAll(':scope > section[id]').forEach((el) => observer!.observe(el))
  }
  setup()
})
onBeforeUnmount(() => observer?.disconnect())

// Spec text is trusted (served by our own gateway) but still escaped; only
// `inline code` is turned into markup.
function md(s: string) {
  const esc = s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  return esc.replace(/`([^`]+)`/g, '<code>$1</code>')
}
async function copy(text: string) { await navigator.clipboard.writeText(text); ElMessage.success('已复制') }
function openSpec() { window.open('/openapi.json', '_blank') }

// ---- try it ------------------------------------------------------------------------------
const KEY_STORE = 'llmgw_docs_key'
const tryVisible = ref(false)
const tryEp = ref<Endpoint | null>(null)
const tryKey = ref(sessionStorage.getItem(KEY_STORE) ?? '')
const tryBody = ref('')
const trying = ref(false)
const tryRes = ref<{ status: number; ms: number; requestId: string; body: string } | null>(null)

function saveKey() { sessionStorage.setItem(KEY_STORE, tryKey.value) }
function resetBody() { tryBody.value = JSON.stringify(tryEp.value ? reqContent(tryEp.value)?.example ?? {} : {}, null, 2) }
function openTry(e: Endpoint) { tryEp.value = e; tryRes.value = null; resetBody(); tryVisible.value = true }

async function send() {
  const e = tryEp.value!
  let body: string | undefined
  if (e.method !== 'GET') {
    try { body = JSON.stringify(JSON.parse(tryBody.value)) } catch { ElMessage.error('请求体不是合法的 JSON'); return }
  }
  saveKey()
  trying.value = true
  const t0 = performance.now()
  try {
    const res = await fetch(`/v1${e.path}`, { method: e.method, headers: { Authorization: `Bearer ${tryKey.value.trim()}`, 'Content-Type': 'application/json' }, body })
    const text = await res.text()
    let pretty = text
    try { pretty = JSON.stringify(JSON.parse(text), null, 2) } catch { /* not JSON */ }
    tryRes.value = { status: res.status, ms: Math.round(performance.now() - t0), requestId: res.headers.get('X-Request-Id') ?? '', body: pretty }
  } catch (err) {
    tryRes.value = { status: 0, ms: Math.round(performance.now() - t0), requestId: '', body: String(err) }
  } finally { trying.value = false }
}
</script>

<style scoped>
.docs { display: grid; grid-template-columns: 220px minmax(0, 1fr); gap: 16px; align-items: start; }
@media (max-width: 1000px) { .docs { grid-template-columns: 1fr; } .toc { display: none; } }
.toc { position: sticky; top: 0; display: flex; flex-direction: column; gap: 2px; padding: 12px 8px; background: var(--card); border: 1px solid var(--border); border-radius: var(--radius); }
.tgroup { padding: 8px 10px 4px; font-size: 11px; font-weight: 500; letter-spacing: .06em; color: var(--text-3); }
.toc a { display: flex; align-items: center; gap: 8px; padding: 6px 10px; border-radius: 6px; font-size: 13px; color: var(--text); cursor: pointer; }
.toc a:hover { background: #F3F4F6; }
.toc a.on { background: var(--primary-soft); color: var(--primary); font-weight: 500; }
.body { display: grid; gap: 16px; min-width: 0; }
.body > section { scroll-margin-top: 12px; }
.m { flex: none; min-width: 38px; padding: 1px 5px; border-radius: 4px; font-family: var(--mono); font-size: 10.5px; font-weight: 700; text-align: center; }
.m.post { background: #E8F5EC; color: #15803D; }
.m.get { background: var(--primary-soft); color: #1D4ED8; }
.m.big { font-size: 12px; padding: 3px 8px; }
.ephead { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.path { font-family: var(--mono); font-size: 14px; font-weight: 600; }
.sum { color: var(--text-2); }
.desc { color: var(--text-2); line-height: 1.7; margin: 10px 0 0; }
h5 { margin: 18px 0 8px; font-size: 13px; font-weight: 600; }
.params { border: 1px solid var(--border-soft); border-radius: 8px; }
.type { font-family: var(--mono); font-size: 12px; color: var(--accent); }
:deep(code), code { padding: 1px 6px; border-radius: 4px; background: #F3F4F6; font-family: var(--mono); font-size: 12px; }
.examples { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 12px; }
@media (max-width: 1200px) { .examples { grid-template-columns: 1fr; } }
pre.json, :deep(.codetabs pre) { margin: 0; padding: 12px 14px; border-radius: 8px; background: #0B1220; color: #E2E8F0; font-family: var(--mono); font-size: 12px; line-height: 1.6; overflow: auto; max-height: 420px; }
:deep(.codetabs) { border-radius: 8px; overflow: hidden; }
:deep(.codetabs .tabs) { display: flex; gap: 2px; padding: 6px 8px 0; background: #111A2E; }
:deep(.codetabs .tabs button) { padding: 5px 10px; border: none; border-radius: 6px 6px 0 0; background: transparent; color: #94A3B8; font: inherit; font-size: 12px; cursor: pointer; }
:deep(.codetabs .tabs button.on) { background: #0B1220; color: #fff; }
:deep(.codetabs .tabs .cp) { margin-left: auto; color: #93C5FD; }
:deep(.codetabs pre) { border-radius: 0; }
.errs { display: flex; flex-direction: column; gap: 6px; }
.err { display: flex; gap: 10px; font-size: 12.5px; color: var(--text-2); line-height: 1.5; }
.err b { flex: none; width: 36px; color: var(--danger); }
.steps { margin: 0 0 8px; padding-left: 20px; display: grid; gap: 8px; }
.steps b { display: block; }
.steps span { color: var(--text-2); font-size: 13px; line-height: 1.6; }
.kv { display: flex; align-items: center; gap: 10px; margin: 0 0 12px 20px; font-size: 13px; }
.kv span { color: var(--text-2); }
.bullets { margin: 0; padding-left: 18px; display: grid; gap: 6px; color: var(--text-2); line-height: 1.7; }
.pad { padding: 4px 16px 12px; color: var(--text-2); line-height: 1.7; }
.result { margin-top: 16px; }
.rhead { display: flex; align-items: center; gap: 12px; margin-bottom: 8px; font-size: 12.5px; }
.mono-input :deep(textarea) { font-family: var(--mono); font-size: 12px; }
</style>
