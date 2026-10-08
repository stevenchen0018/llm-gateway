<template>
  <div>
    <PageHeader title="模型体验 · 文本生成" desc="多轮对话直连所选模型（不经过路由 / 限流 / 计费），用于快速评估效果与延迟" />
    <div class="layout">
      <Panel title="参数" class="side">
        <el-form label-position="top">
          <el-form-item label="模型"><ModelSelect v-model="modelId" :categories="['text', 'thinking', 'code', 'multimodal', 'extraction']" /></el-form-item>
          <el-form-item label="系统提示词"><el-input v-model="system" type="textarea" :rows="4" resize="none" placeholder="可选：设定角色与回答风格" /></el-form-item>
          <el-form-item :label="`温度 ${temperature.toFixed(1)}`"><el-slider v-model="temperature" :min="0" :max="2" :step="0.1" /></el-form-item>
          <el-form-item label="最大输出 Token"><el-input-number v-model="maxTokens" :min="0" :step="256" controls-position="right" style="width: 100%" /><div class="hint">0 表示使用模型默认值</div></el-form-item>
          <el-button :icon="Delete" :disabled="!messages.length" @click="messages = []">清空对话</el-button>
        </el-form>
      </Panel>

      <Panel class="chat" flush>
        <div ref="scroller" class="msgs">
          <div v-if="!messages.length" class="empty">
            <div class="t">开始一段对话</div>
            <div class="chips"><button v-for="s in samples" :key="s" type="button" @click="send(s)">{{ s }}</button></div>
          </div>
          <div v-for="(m, i) in messages" :key="i" class="msg" :class="m.role">
            <div class="avatar">{{ m.role === 'user' ? '我' : 'AI' }}</div>
            <div class="bubble">
              <div class="text">{{ m.content }}</div>
              <div v-if="m.meta" class="meta num">耗时 {{ fmtDuration(m.meta.latency) }} · 输入 {{ m.meta.prompt }} · 输出 {{ m.meta.completion }} · 合计 {{ m.meta.total }} tokens</div>
            </div>
          </div>
          <div v-if="sending" class="msg assistant"><div class="avatar">AI</div><div class="bubble"><span class="dots"><i /><i /><i /></span></div></div>
        </div>
        <div class="input">
          <el-input v-model="draft" type="textarea" :autosize="{ minRows: 2, maxRows: 6 }" resize="none" placeholder="输入消息，Enter 发送，Shift+Enter 换行" @keydown.enter.exact.prevent="send()" />
          <el-button type="primary" :icon="Promotion" :loading="sending" :disabled="!draft.trim() || !modelId" @click="send()">发送</el-button>
        </div>
      </Panel>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { Delete, Promotion } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import ModelSelect from '../../components/ModelSelect.vue'
import { playground } from '../../api'
import { fmtDuration } from '../../utils'

interface Msg { role: 'user' | 'assistant'; content: string; meta?: { latency: number; prompt: number; completion: number; total: number } }

const samples = ['用三句话介绍大模型网关', '写一个 Go 语言的令牌桶限流器', '解释 TPM 与 RPM 的区别，并举例']
const modelId = ref<number>()
const system = ref('')
const temperature = ref(0.7)
const maxTokens = ref(0)
const messages = ref<Msg[]>([])
const draft = ref('')
const sending = ref(false)
const scroller = ref<HTMLDivElement>()

const toBottom = () => nextTick(() => { if (scroller.value) scroller.value.scrollTop = scroller.value.scrollHeight })

async function send(text?: string) {
  const content = (text ?? draft.value).trim()
  if (!content || !modelId.value || sending.value) return
  messages.value.push({ role: 'user', content })
  draft.value = ''
  sending.value = true
  toBottom()
  try {
    const history = messages.value.map((m) => ({ role: m.role, content: m.content }))
    const res = await playground.chat({
      model_id: modelId.value,
      messages: system.value.trim() ? [{ role: 'system', content: system.value.trim() }, ...history] : history,
      temperature: temperature.value,
      max_tokens: maxTokens.value > 0 ? maxTokens.value : undefined,
    })
    messages.value.push({
      role: 'assistant', content: res.choices?.[0]?.message?.content ?? '',
      meta: { latency: res.latency_ms ?? 0, prompt: res.usage.prompt_tokens, completion: res.usage.completion_tokens, total: res.usage.total_tokens },
    })
  } catch {
    messages.value.pop()
    draft.value = content
  } finally { sending.value = false; toBottom() }
}
</script>

<style scoped>
.layout { display: grid; grid-template-columns: 300px minmax(0, 1fr); gap: 16px; align-items: start; }
@media (max-width: 960px) { .layout { grid-template-columns: 1fr; } }
.chat :deep(.panel-body) { display: flex; flex-direction: column; height: calc(100vh - 210px); min-height: 440px; }
.msgs { flex: 1; overflow-y: auto; padding: 20px; display: flex; flex-direction: column; gap: 18px; }
.empty { margin: auto; text-align: center; }
.empty .t { color: var(--text-2); margin-bottom: 14px; }
.chips { display: flex; flex-wrap: wrap; gap: 8px; justify-content: center; }
.chips button { padding: 6px 12px; border: 1px solid var(--border); border-radius: 999px; background: #fff; font: inherit; font-size: 12.5px; color: var(--text-2); cursor: pointer; }
.chips button:hover { border-color: var(--primary); color: var(--primary); }
.msg { display: flex; gap: 10px; max-width: 86%; }
.msg.user { align-self: flex-end; flex-direction: row-reverse; }
.avatar { flex: none; width: 28px; height: 28px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-size: 11px; font-weight: 600; background: var(--accent-soft); color: var(--accent); }
.msg.user .avatar { background: var(--primary-soft); color: var(--primary); }
.bubble { padding: 10px 14px; border-radius: 10px; background: #F5F6F8; min-width: 0; }
.msg.user .bubble { background: var(--primary); color: #fff; }
.text { white-space: pre-wrap; line-height: 1.7; word-break: break-word; }
.meta { margin-top: 8px; padding-top: 6px; border-top: 1px dashed var(--border); font-size: 11.5px; color: var(--text-3); }
.input { display: flex; gap: 10px; align-items: flex-end; padding: 12px 16px; border-top: 1px solid var(--border-soft); }
.input .el-textarea { flex: 1; }
.dots { display: inline-flex; gap: 4px; padding: 4px 0; }
.dots i { width: 6px; height: 6px; border-radius: 50%; background: #9CA3AF; animation: b 1s infinite ease-in-out; }
.dots i:nth-child(2) { animation-delay: .15s; } .dots i:nth-child(3) { animation-delay: .3s; }
@keyframes b { 0%, 80%, 100% { opacity: .3; transform: translateY(0); } 40% { opacity: 1; transform: translateY(-3px); } }
</style>
