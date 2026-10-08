<template>
  <el-dialog :model-value="visible" :title="`接入说明 · ${model?.display_name ?? ''}`" width="720px" destroy-on-close @update:model-value="$emit('update:visible', $event)">
    <template v-if="model">
      <div v-if="model.category === 'image_gen'" class="note">图片生成模型暂通过控制台「模型体验 · 图片生成」使用，网关统一 API 目前开放对话、补全与向量化接口。</div>
      <div class="kv">
        <div><span class="k">接入地址</span><code>{{ baseUrl }}</code><el-button link type="primary" size="small" @click="copy(baseUrl)">复制</el-button></div>
        <div><span class="k">模型名称</span><code>{{ model.model_key }}</code><el-button link type="primary" size="small" @click="copy(model.model_key)">复制</el-button></div>
        <div><span class="k">指定供应商</span><code>{{ model.provider?.code }}/{{ model.display_name }}</code><span class="muted">同一模型由多家供应商提供时，可用「供应商编码/模型」固定调用渠道；不指定则按调度策略与厂商的供应商选择自动路由</span></div>
      </div>
      <el-tabs v-model="tab" class="tabs">
        <el-tab-pane label="cURL" name="curl" /><el-tab-pane label="Python" name="py" /><el-tab-pane label="Node.js" name="js" />
      </el-tabs>
      <div class="code">
        <el-button class="cp" size="small" :icon="CopyDocument" @click="copy(snippet)">复制</el-button>
        <pre>{{ snippet }}</pre>
      </div>
      <p class="hint">API Key 请在「API Key · Key 申请」中申请并审批；请求与响应完全兼容 OpenAI 协议，业务无需关心各厂商入参出参差异。调度策略会按 Key + 模型自动路由并容灾。</p>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument } from '@element-plus/icons-vue'
import type { Model } from '../../types'

const props = defineProps<{ visible: boolean; model: Model | null }>()
defineEmits<{ 'update:visible': [v: boolean] }>()

const tab = ref('curl')
const baseUrl = computed(() => `${location.origin}/v1`)
const isEmbedding = computed(() => props.model?.type === 'embedding')

const snippet = computed(() => {
  const m = props.model
  if (!m) return ''
  const model = m.model_key
  if (isEmbedding.value) {
    if (tab.value === 'py') return `from openai import OpenAI\n\nclient = OpenAI(base_url="${baseUrl.value}", api_key="sk-...")\nresp = client.embeddings.create(model="${model}", input=["你好，世界"])\nprint(len(resp.data[0].embedding))`
    if (tab.value === 'js') return `import OpenAI from "openai";\n\nconst client = new OpenAI({ baseURL: "${baseUrl.value}", apiKey: "sk-..." });\nconst resp = await client.embeddings.create({ model: "${model}", input: ["你好，世界"] });\nconsole.log(resp.data[0].embedding.length);`
    return `curl ${baseUrl.value}/embeddings \\\n  -H "Authorization: Bearer sk-..." \\\n  -H "Content-Type: application/json" \\\n  -d '{"model": "${model}", "input": ["你好，世界"]}'`
  }
  if (tab.value === 'py') return `from openai import OpenAI\n\nclient = OpenAI(base_url="${baseUrl.value}", api_key="sk-...")\nresp = client.chat.completions.create(\n    model="${model}",\n    messages=[{"role": "user", "content": "你好"}],\n)\nprint(resp.choices[0].message.content)`
  if (tab.value === 'js') return `import OpenAI from "openai";\n\nconst client = new OpenAI({ baseURL: "${baseUrl.value}", apiKey: "sk-..." });\nconst resp = await client.chat.completions.create({\n  model: "${model}",\n  messages: [{ role: "user", content: "你好" }],\n});\nconsole.log(resp.choices[0].message.content);`
  return `curl ${baseUrl.value}/chat/completions \\\n  -H "Authorization: Bearer sk-..." \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "${model}",\n    "messages": [{"role": "user", "content": "你好"}]\n  }'`
})

async function copy(text: string) {
  await navigator.clipboard.writeText(text)
  ElMessage.success('已复制')
}
</script>

<style scoped>
.note { margin-bottom: 12px; padding: 8px 12px; border-radius: 6px; background: var(--warning-soft); color: #B45309; font-size: 12.5px; }
.kv { display: grid; gap: 8px; margin-bottom: 4px; }
.kv > div { display: flex; align-items: center; gap: 10px; font-size: 13px; }
.k { flex: none; width: 60px; color: var(--text-2); }
code { padding: 2px 8px; border-radius: 4px; background: #F3F4F6; font-family: var(--mono); font-size: 12px; }
.tabs { margin-top: 8px; }
.code { position: relative; }
.cp { position: absolute; top: 8px; right: 8px; }
pre { margin: 0; padding: 14px 16px; border-radius: 8px; background: #0B1220; color: #E2E8F0; font-family: var(--mono); font-size: 12.5px; line-height: 1.65; overflow-x: auto; }
.hint { margin-top: 12px; }
</style>
