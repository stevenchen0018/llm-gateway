<template>
  <el-drawer :model-value="visible" :title="'请求详情'" size="760px" destroy-on-close @update:model-value="(v: boolean) => $emit('update:visible', v)">
    <div v-loading="loading" class="wrap">
      <EmptyState v-if="!loading && !log" :text="notFound ? '没有找到该请求的内容记录' : '加载中'" :hint="notFound ? '可能是请求记录未开启、已超过保留期，或该调用早于记录功能上线' : ''" height="200px" />
      <template v-if="log">
        <dl class="meta">
          <div><dt>时间</dt><dd class="num">{{ fmtTime(log.created_at) }}</dd></div>
          <div><dt>Request ID</dt><dd class="mono rid">{{ log.request_id }}<el-button link :icon="CopyDocument" @click="copy(log.request_id)" /></dd></div>
          <div><dt>API Key</dt><dd>{{ keyName(log.key_id) }}</dd></div>
          <div><dt>接口</dt><dd class="mono">POST {{ log.endpoint }}</dd></div>
          <div><dt>请求模型</dt><dd class="mono">{{ log.model || '—' }}</dd></div>
          <div><dt>命中模型</dt><dd>{{ log.model_id ? modelLabel(log.model_id) : '—' }}<span v-if="log.provider_id" class="muted"> · {{ providerName(log.provider_id) }}</span></dd></div>
          <div><dt>结果</dt><dd><StatusBadge :text="STATUS[log.status].text" :tone="STATUS[log.status].tone" /><span class="mono muted"> HTTP {{ log.http_status }}<template v-if="log.error_code"> · {{ log.error_code }}</template></span></dd></div>
          <div><dt>Token（入 / 出）</dt><dd class="num">{{ fmtNum(log.prompt_tokens) }} / {{ fmtNum(log.completion_tokens) }}</dd></div>
          <div><dt>耗时</dt><dd class="num">{{ fmtDuration(log.latency_ms) }}</dd></div>
          <div><dt>来源 IP</dt><dd class="mono">{{ log.source_ip }}</dd></div>
          <div class="span2"><dt>User-Agent</dt><dd class="mono muted">{{ log.user_agent || '—' }}</dd></div>
        </dl>

        <div v-if="log.filter_hits?.length" class="hits">
          <div class="sec">过滤规则命中</div>
          <div v-for="(h, i) in log.filter_hits" :key="i" class="hit">
            <StatusBadge :text="ACTION[h.action].text" :tone="ACTION[h.action].tone" />
            <b>{{ h.rule }}</b>
            <span class="muted">{{ h.stage === 'input' ? '请求' : '输出' }} · 命中 {{ h.matches }} 处</span>
            <span v-if="h.sample" class="mono sample">「{{ h.sample }}」</span>
          </div>
        </div>

        <el-tabs v-model="tab" class="tabs">
          <el-tab-pane v-if="dialog.length" label="对话视图" name="chat">
            <div class="chat">
              <div v-for="(m, i) in dialog" :key="i" class="msg" :class="m.role">
                <span class="role">{{ ROLE[m.role] ?? m.role }}</span>
                <div class="bubble">{{ m.content }}</div>
              </div>
            </div>
          </el-tab-pane>
          <el-tab-pane label="请求体" name="req"><CodeBlock :text="pretty(log.request_body)" :empty="bodyEmpty" /></el-tab-pane>
          <el-tab-pane label="响应体" name="resp"><CodeBlock :text="pretty(log.response_body)" :empty="bodyEmpty" /></el-tab-pane>
        </el-tabs>
        <div v-if="log.body_truncated" class="hint">内容超过记录上限，已截断保存。</div>
      </template>
    </div>
  </el-drawer>
</template>

<script setup lang="ts">
import { computed, h, ref, watch, type FunctionalComponent } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument } from '@element-plus/icons-vue'
import StatusBadge from './StatusBadge.vue'
import EmptyState from './EmptyState.vue'
import { security } from '../api'
import { useLookups } from '../composables/useLookups'
import { fmtDuration, fmtNum, fmtTime } from '../utils'
import type { RequestLog } from '../types'

const props = defineProps<{ visible: boolean; id?: number; requestId?: string }>()
defineEmits<{ 'update:visible': [v: boolean] }>()

const STATUS = { success: { text: '成功', tone: 'success' }, failed: { text: '失败', tone: 'danger' }, blocked: { text: '已拦截', tone: 'warning' } } as const
const ACTION = { block: { text: '拦截', tone: 'danger' }, mask: { text: '脱敏', tone: 'primary' }, log: { text: '记录', tone: 'info' } } as const
const ROLE: Record<string, string> = { system: '系统', user: '用户', assistant: '模型' }

const { keyName, modelLabel, providerName } = useLookups()
const log = ref<RequestLog | null>(null)
const loading = ref(false)
const notFound = ref(false)
const tab = ref('chat')

watch(() => [props.visible, props.id, props.requestId] as const, async ([v, id, rid]) => {
  if (!v) return
  log.value = null; notFound.value = false; loading.value = true
  try {
    log.value = id ? await security.requestLog(id) : await security.requestLogByRequestId(rid!)
    tab.value = dialog.value.length ? 'chat' : 'req'
  } catch { notFound.value = true } finally { loading.value = false }
}, { immediate: true })

const bodyEmpty = '未保存请求/响应内容（记录设置中关闭了「保存内容」）'

function parse(s?: string): any { try { return s ? JSON.parse(s) : null } catch { return null } }
function pretty(s?: string) {
  const j = parse(s)
  return j ? JSON.stringify(j, null, 2) : (s ?? '')
}
const text = (c: unknown): string => typeof c === 'string' ? c : Array.isArray(c) ? c.map((p: any) => p?.text ?? (p?.type === 'image_url' ? '[图片]' : '')).join(' ') : ''

// chat requests render as a conversation: the messages sent plus the answer
const dialog = computed(() => {
  const req = parse(log.value?.request_body), resp = parse(log.value?.response_body)
  const out: { role: string; content: string }[] = []
  if (Array.isArray(req?.messages)) for (const m of req.messages) out.push({ role: m.role, content: text(m.content) })
  else if (typeof req?.prompt === 'string') out.push({ role: 'user', content: req.prompt })
  if (!out.length) return out
  const answer = resp?.choices?.[0]?.message?.content ?? resp?.choices?.[0]?.text
  if (answer) out.push({ role: 'assistant', content: answer })
  else if (resp?.error) out.push({ role: 'error', content: `${resp.error.code ?? ''} ${resp.error.message ?? ''}`.trim() })
  return out
})

async function copy(s: string) { await navigator.clipboard.writeText(s); ElMessage.success('已复制') }

const CodeBlock: FunctionalComponent<{ text: string; empty: string }> = (p) =>
  p.text
    ? h('div', { class: 'code' }, [h('button', { class: 'cp', type: 'button', onClick: () => copy(p.text) }, '复制'), h('pre', p.text)])
    : h('div', { class: 'hint', style: 'padding: 12px 0' }, p.empty)
</script>

<style scoped>
.wrap { min-height: 200px; }
.meta { display: grid; grid-template-columns: 1fr 1fr; gap: 10px 20px; margin: 0 0 16px; }
.meta div { min-width: 0; }
.meta .span2 { grid-column: 1 / -1; }
.meta dt { font-size: 12px; color: var(--text-2); margin-bottom: 2px; }
.meta dd { margin: 0; font-size: 13px; overflow-wrap: anywhere; text-align: left; }
.rid { display: flex; align-items: center; gap: 4px; font-size: 12px; }
.sec { font-size: 12px; color: var(--text-2); margin-bottom: 6px; }
.hits { margin-bottom: 12px; padding: 10px 12px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: #FFFBEB; }
.hit { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; font-size: 13px; padding: 3px 0; }
.sample { font-size: 12px; color: #92400E; }
.chat { display: grid; gap: 10px; }
.msg { display: grid; gap: 4px; }
.role { font-size: 11.5px; color: var(--text-3); }
.bubble { white-space: pre-wrap; word-break: break-word; font-size: 13px; line-height: 1.6; padding: 8px 12px; border-radius: 8px; background: #F3F4F6; }
.msg.system .bubble { background: #F9FAFB; color: var(--text-2); border: 1px dashed var(--border); }
.msg.user .bubble { background: var(--primary-soft); }
.msg.assistant .bubble { background: #fff; border: 1px solid var(--border); }
.msg.error .bubble { background: var(--danger-soft); color: #B91C1C; }
:deep(.code) { position: relative; }
:deep(.code pre) { margin: 0; max-height: 460px; overflow: auto; padding: 12px 14px; background: #0F172A; color: #E2E8F0; border-radius: 8px; font: 12px/1.6 var(--mono); white-space: pre-wrap; word-break: break-all; }
:deep(.code .cp) { position: absolute; top: 8px; right: 10px; border: 1px solid #334155; background: #1E293B; color: #CBD5E1; border-radius: 4px; font-size: 11.5px; padding: 1px 8px; cursor: pointer; }
</style>
