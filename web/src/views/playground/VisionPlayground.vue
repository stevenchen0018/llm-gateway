<template>
  <div>
    <PageHeader title="模型体验 · 图片理解" desc="上传图片并提问，体验视觉模型的图像描述、OCR、图表解读能力" />
    <div class="layout">
      <Panel title="输入">
        <el-form label-position="top">
          <el-form-item label="模型"><ModelSelect v-model="modelId" :categories="['vision', 'multimodal']" /></el-form-item>
          <el-form-item label="图片">
            <el-upload class="up" drag :auto-upload="false" :show-file-list="false" accept="image/*" :on-change="onFile">
              <img v-if="image" :src="image" alt="预览" class="preview" />
              <div v-else class="ph"><el-icon :size="30"><UploadFilled /></el-icon><div>拖拽图片到此处，或点击上传</div><div class="hint">支持 PNG / JPG / WebP / SVG，≤ 4MB</div></div>
            </el-upload>
            <div class="row"><el-button size="small" @click="useSample">使用示例图</el-button><el-button v-if="image" size="small" @click="image = ''">移除</el-button></div>
          </el-form-item>
          <el-form-item label="问题">
            <el-input v-model="prompt" type="textarea" :rows="3" resize="none" placeholder="关于这张图片，你想了解什么？" />
            <div class="chips"><button v-for="s in samples" :key="s" type="button" @click="prompt = s">{{ s }}</button></div>
          </el-form-item>
          <el-button type="primary" :loading="loading" :disabled="!modelId || !image || !prompt.trim()" @click="run">开始分析</el-button>
        </el-form>
      </Panel>

      <Panel title="结果">
        <EmptyState v-if="!result && !loading" text="分析结果将显示在这里" height="260px" />
        <div v-else v-loading="loading" style="min-height: 120px">
          <div v-if="result" class="answer">{{ result.choices?.[0]?.message?.content }}</div>
          <div v-if="result" class="meta num">耗时 {{ fmtDuration(result.latency_ms ?? 0) }} · 输入 {{ result.usage.prompt_tokens }} · 输出 {{ result.usage.completion_tokens }} · 合计 {{ result.usage.total_tokens }} tokens</div>
        </div>
      </Panel>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage, type UploadFile } from 'element-plus'
import { UploadFilled } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import EmptyState from '../../components/EmptyState.vue'
import ModelSelect from '../../components/ModelSelect.vue'
import { playground } from '../../api'
import { fmtDuration } from '../../utils'
import type { ChatResult } from '../../types'

const samples = ['详细描述这张图片的内容', '提取图片中的所有文字', '这张图表反映了什么趋势？']
const modelId = ref<number>()
const image = ref('')
const prompt = ref('')
const loading = ref(false)
const result = ref<ChatResult | null>(null)

function onFile(file: UploadFile) {
  const raw = file.raw
  if (!raw) return
  if (raw.size > 4 * 1024 * 1024) { ElMessage.warning('图片不能超过 4MB'); return }
  const reader = new FileReader()
  reader.onload = () => { image.value = String(reader.result) }
  reader.readAsDataURL(raw)
}

function useSample() {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="480" height="300"><rect width="480" height="300" fill="#F7F8FA"/><text x="24" y="40" font-size="18" font-family="sans-serif" fill="#111827">近 7 日调用量</text>${[60, 92, 130, 118, 150, 96, 72].map((h, i) => `<rect x="${40 + i * 60}" y="${260 - h}" width="34" height="${h}" rx="4" fill="#2563EB"/>`).join('')}<line x1="30" y1="260" x2="450" y2="260" stroke="#D1D5DB"/></svg>`
  // btoa only handles Latin-1; go through UTF-8 so the Chinese caption encodes
  image.value = 'data:image/svg+xml;base64,' + btoa(unescape(encodeURIComponent(svg)))
  if (!prompt.value) prompt.value = samples[2]
}

async function run() {
  loading.value = true
  result.value = null
  try { result.value = await playground.vision({ model_id: modelId.value!, prompt: prompt.value.trim(), image: image.value }) } finally { loading.value = false }
}
</script>

<style scoped>
.layout { display: grid; grid-template-columns: minmax(0, 5fr) minmax(0, 6fr); gap: 16px; align-items: start; }
@media (max-width: 960px) { .layout { grid-template-columns: 1fr; } }
.up { width: 100%; }
.up :deep(.el-upload-dragger) { padding: 12px; min-height: 180px; display: flex; align-items: center; justify-content: center; border-radius: 8px; }
.ph { color: var(--text-2); display: grid; gap: 6px; justify-items: center; }
.preview { max-width: 100%; max-height: 260px; border-radius: 6px; }
.row { display: flex; gap: 8px; margin-top: 8px; }
.chips { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; }
.chips button { padding: 3px 10px; border: 1px solid var(--border); border-radius: 999px; background: #fff; font: inherit; font-size: 12px; color: var(--text-2); cursor: pointer; }
.chips button:hover { border-color: var(--primary); color: var(--primary); }
.answer { white-space: pre-wrap; line-height: 1.8; }
.meta { margin-top: 14px; padding-top: 10px; border-top: 1px dashed var(--border); font-size: 12px; color: var(--text-3); }
</style>
