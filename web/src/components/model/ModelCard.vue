<template>
  <article class="card" :class="{ off: model.status !== 'active' }">
    <header>
      <VendorLogo :code="model.vendor?.code ?? model.provider?.code ?? ''" :size="36" />
      <div class="ttl">
        <h4 :title="model.display_name">{{ model.display_name }}</h4>
        <span class="muted">{{ model.vendor?.name ?? '—' }}<span class="sep">·</span>{{ model.provider?.name }} 提供</span>
      </div>
      <span v-if="model.status !== 'active'" class="ribbon off">已下架</span>
      <span v-else-if="isNew" class="ribbon new">新品</span>
      <span v-else-if="isSelfHosted" class="ribbon self">自建</span>
    </header>

    <div class="tags">
      <span class="tag cat">{{ categoryLabel(model.category) }}</span>
      <span v-if="(supplierCount ?? 1) > 1" class="tag multi" :title="`共 ${supplierCount} 家供应商提供该模型`">{{ supplierCount }} 家供应商</span>
      <span v-for="t in extraTags.slice(0, 3)" :key="t" class="tag">{{ t }}</span>
      <span v-if="extraTags.length > 3" class="tag more">+{{ extraTags.length - 3 }}</span>
    </div>

    <p class="desc">{{ model.description || '暂无描述' }}</p>

    <dl class="meta">
      <div><dt>上下文</dt><dd class="num">{{ fmtContext(model.context_length) }}</dd></div>
      <div><dt>输入 / 输出 每 1K</dt><dd class="num">{{ fmtMoney(model.input_price_per_1k) }} / {{ fmtMoney(model.output_price_per_1k) }}</dd></div>
    </dl>

    <footer>
      <button type="button" @click="$emit('detail', model)"><el-icon><View /></el-icon>查看详情</button>
      <button type="button" @click="$emit('guide', model)"><el-icon><Document /></el-icon>接入说明</button>
      <button type="button" class="primary" @click="$emit('try', model)"><el-icon><VideoPlay /></el-icon>立即体验</button>
      <el-tooltip :content="mine ? '已在我的模型，点击移除' : '加入我的模型'" placement="top">
        <button type="button" class="star" :class="{ on: mine }" @click="$emit('toggle-mine', model)"><el-icon><StarFilled v-if="mine" /><Star v-else /></el-icon></button>
      </el-tooltip>
    </footer>
    <div class="date faint">发布于 {{ model.released_at ? model.released_at.slice(0, 10) : '—' }}</div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import VendorLogo from '../VendorLogo.vue'
import { categoryLabel, fmtContext } from '../../constants'
import { fmtMoney } from '../../utils'
import type { Model } from '../../types'

const props = defineProps<{ model: Model; mine?: boolean; supplierCount?: number }>()
defineEmits<{ detail: [m: Model]; guide: [m: Model]; try: [m: Model]; 'toggle-mine': [m: Model] }>()

// the category chip already says e.g. 深度思考; don't repeat it as a tag
const extraTags = computed(() => props.model.tags.filter((t) => t !== categoryLabel(props.model.category)))
const isNew = computed(() => !!props.model.released_at && Date.now() - new Date(props.model.released_at).getTime() < 45 * 86400e3)
const isSelfHosted = computed(() => props.model.provider?.supplier_type === 'self_hosted')
</script>

<style scoped>
.sep { margin: 0 4px; color: var(--text-3); }
.tag.multi { background: var(--accent-soft); color: var(--accent); }
.card { position: relative; display: flex; flex-direction: column; gap: 10px; padding: 14px 14px 10px; background: var(--card); border: 1px solid var(--border); border-radius: var(--radius); transition: border-color .15s, box-shadow .15s; min-width: 0; }
.card:hover { border-color: #C7D2FE; box-shadow: 0 2px 10px rgba(37, 99, 235, .07); }
.card.off { opacity: .62; }
header { display: flex; align-items: center; gap: 10px; }
.ttl { min-width: 0; flex: 1; }
h4 { margin: 0; font-size: 14px; line-height: 20px; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.ttl .muted { font-size: 12px; }
.ribbon { flex: none; padding: 1px 8px; border-radius: 999px; font-size: 11px; font-weight: 500; }
.ribbon.new { background: #FFF1E6; color: #C2410C; }
.ribbon.self { background: #E6F7F4; color: #0F766E; }
.ribbon.off { background: #F1F2F4; color: #6B7280; }
.tags { display: flex; flex-wrap: wrap; gap: 4px; min-height: 22px; }
.tag { padding: 0 7px; height: 20px; line-height: 20px; border-radius: 4px; background: #F3F4F6; color: var(--text-2); font-size: 11px; }
.tag.cat { background: var(--primary-soft); color: #1D4ED8; }
.tag.more { background: transparent; color: var(--text-3); padding: 0 2px; }
.desc { margin: 0; color: var(--text-2); font-size: 12.5px; line-height: 1.6; display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; min-height: 60px; }
.meta { display: grid; gap: 4px; margin: 0; padding: 8px 0; border-top: 1px dashed var(--border); border-bottom: 1px dashed var(--border); }
.meta div { display: flex; justify-content: space-between; font-size: 12px; }
dt { color: var(--text-2); }
dd { margin: 0; color: var(--text); }
footer { display: flex; align-items: center; gap: 2px; }
footer button { display: inline-flex; align-items: center; gap: 4px; height: 28px; padding: 0 8px; border: none; background: none; border-radius: 6px; font: inherit; font-size: 12.5px; color: var(--text-2); cursor: pointer; }
footer button:hover { background: #F3F4F6; color: var(--text); }
footer button.primary { color: var(--primary); font-weight: 500; }
footer button.primary:hover { background: var(--primary-soft); }
footer .star { margin-left: auto; padding: 0 6px; }
footer .star.on { color: #F59E0B; }
.date { font-size: 11px; }
</style>
