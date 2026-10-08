<template>
  <div>
    <PageHeader title="模型体验 · 图片生成" desc="输入描述词生成图片，对比不同文生图模型的效果与耗时" />
    <div class="layout">
      <Panel title="生成参数">
        <el-form label-position="top">
          <el-form-item label="模型"><ModelSelect v-model="modelId" :categories="['image_gen']" /></el-form-item>
          <el-form-item label="描述词">
            <el-input v-model="prompt" type="textarea" :rows="5" resize="none" placeholder="描述你想生成的画面，如：星空下的未来城市，赛博朋克风格" />
            <div class="chips"><button v-for="s in samples" :key="s" type="button" @click="prompt = s">{{ s }}</button></div>
          </el-form-item>
          <el-form-item label="图片尺寸">
            <el-radio-group v-model="size"><el-radio-button v-for="s in sizes" :key="s" :value="s">{{ s }}</el-radio-button></el-radio-group>
          </el-form-item>
          <el-button type="primary" :loading="loading" :disabled="!modelId || !prompt.trim()" @click="run">生成图片</el-button>
        </el-form>
      </Panel>

      <Panel title="生成结果">
        <EmptyState v-if="!history.length && !loading" text="生成的图片将显示在这里" height="300px" />
        <div v-else v-loading="loading" class="gallery">
          <figure v-for="(h, i) in history" :key="h.id" :class="{ first: i === 0 }">
            <img :src="h.url" :alt="h.prompt" />
            <figcaption>
              <div class="p">{{ h.prompt }}</div>
              <div class="m num muted">{{ h.model }} · {{ h.size }} · {{ fmtDuration(h.latency) }}</div>
              <a :href="h.url" :download="`generated-${h.id}.${h.url.startsWith('data:image/svg') ? 'svg' : 'png'}`">下载</a>
            </figcaption>
          </figure>
        </div>
      </Panel>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import EmptyState from '../../components/EmptyState.vue'
import ModelSelect from '../../components/ModelSelect.vue'
import { playground } from '../../api'
import { useLookups } from '../../composables/useLookups'
import { fmtDuration } from '../../utils'

const { modelName } = useLookups()
const samples = ['星空下的未来城市，赛博朋克风格', '水墨风格的江南水乡', '一只戴着宇航员头盔的柴犬，扁平插画']
const sizes = ['512x512', '1024x1024', '1280x720']
const modelId = ref<number>()
const prompt = ref('')
const size = ref('1024x1024')
const loading = ref(false)
const history = ref<{ id: number; url: string; prompt: string; model: string; size: string; latency: number }[]>([])
let seq = 0

async function run() {
  loading.value = true
  try {
    const res = await playground.image({ model_id: modelId.value!, prompt: prompt.value.trim(), size: size.value })
    for (const url of res.images) history.value.unshift({ id: ++seq, url, prompt: prompt.value.trim(), model: modelName(modelId.value), size: size.value, latency: res.latency_ms })
  } finally { loading.value = false }
}
</script>

<style scoped>
.layout { display: grid; grid-template-columns: minmax(0, 4fr) minmax(0, 7fr); gap: 16px; align-items: start; }
@media (max-width: 960px) { .layout { grid-template-columns: 1fr; } }
.chips { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; }
.chips button { padding: 3px 10px; border: 1px solid var(--border); border-radius: 999px; background: #fff; font: inherit; font-size: 12px; color: var(--text-2); cursor: pointer; text-align: left; }
.chips button:hover { border-color: var(--primary); color: var(--primary); }
.gallery { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 14px; min-height: 120px; }
figure { margin: 0; border: 1px solid var(--border); border-radius: 10px; overflow: hidden; background: #fff; }
figure.first { grid-column: span 2; }
img { display: block; width: 100%; aspect-ratio: 1 / 1; object-fit: contain; background: #F3F4F6; }
figure.first img { aspect-ratio: 4 / 3; }
figcaption { padding: 10px 12px; font-size: 12.5px; }
.p { line-height: 1.5; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.m { font-size: 11.5px; margin: 4px 0; }
</style>
