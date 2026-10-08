// Shared vocabulary for the console: model categories, vendor identity, labels.

export interface Option<T = string> { value: T; label: string }

// Model categories of the marketplace filter (模型类型)
export const CATEGORIES: Option[] = [
  { value: 'text', label: '文本生成' },
  { value: 'thinking', label: '深度思考' },
  { value: 'multimodal', label: '全模态' },
  { value: 'vision', label: '图片理解' },
  { value: 'image_gen', label: '图片生成' },
  { value: 'video_gen', label: '视频生成' },
  { value: 'speech_asr', label: '语音识别' },
  { value: 'speech_tts', label: '语音合成' },
  { value: 'embedding', label: '向量模型' },
  { value: 'rerank', label: '重排序' },
  { value: 'code', label: '代码能力' },
  { value: 'extraction', label: '数据抽取' },
  { value: 'doc_parse', label: '文档解析' },
]
export const categoryLabel = (v: string) => CATEGORIES.find((c) => c.value === v)?.label ?? v

// Context-length buckets (上下文长度), in tokens
export const CONTEXT_BUCKETS: (Option & { test: (n: number) => boolean })[] = [
  { value: '', label: '不限', test: () => true },
  { value: 's', label: '16K 以下', test: (n) => n > 0 && n < 16 * 1024 },
  { value: 'm', label: '16K–64K', test: (n) => n >= 16 * 1024 && n < 64 * 1024 },
  { value: 'l', label: '64K–128K', test: (n) => n >= 64 * 1024 && n < 128 * 1024 },
  { value: 'xl', label: '128K 以上', test: (n) => n >= 128 * 1024 },
]

export const fmtContext = (n: number) => (n <= 0 ? '—' : n >= 1024 * 1024 ? `${Math.round(n / 1024 / 1024)}M` : `${Math.round(n / 1024)}K`)

// Categories that need a dedicated playground page
export const PLAYGROUND_FOR: Record<string, string> = {
  vision: '/playground/vision',
  multimodal: '/playground/vision',
  image_gen: '/playground/image',
  video_gen: '/playground/video',
}
export const playgroundPath = (category: string) => PLAYGROUND_FOR[category] ?? '/playground/text'

// Vendor identity: initials on a tinted tile (deliberately not vendor logos).
export const VENDOR_STYLE: Record<string, { text: string; color: string }> = {
  aliyun: { text: '阿', color: '#F97316' },
  bytedance: { text: '字', color: '#2563EB' },
  baidu: { text: '百', color: '#4F46E5' },
  huawei: { text: '华', color: '#DC2626' },
  google: { text: 'G', color: '#0EA5E9' },
  azure: { text: 'Az', color: '#0284C7' },
  deepseek: { text: 'DS', color: '#4D6BFE' },
  kubeai: { text: 'K', color: '#0D9488' },
  zhipu: { text: '智', color: '#3B5BDB' },
  moonshot: { text: 'Ki', color: '#111827' },
  minimax: { text: 'MM', color: '#E11D48' },
  tencent: { text: '腾', color: '#0052D9' },
  kuaishou: { text: '快', color: '#F97316' },
  baichuan: { text: '川', color: '#EA580C' },
  lingyi: { text: '零', color: '#16A34A' },
  xunfei: { text: '讯', color: '#1D4ED8' },
  sensetime: { text: '商', color: '#7C3AED' },
  stepfun: { text: '阶', color: '#0891B2' },
  anthropic: { text: 'A', color: '#B45309' },
  mistral: { text: 'Mi', color: '#EA580C' },
  xai: { text: 'x', color: '#374151' },
  // vendors (厂商) whose code differs from a supplier's
  qwen: { text: '通', color: '#F97316' },
  doubao: { text: '豆', color: '#2563EB' },
  ernie: { text: '文', color: '#4F46E5' },
  openai: { text: 'AI', color: '#10A37F' },
  // suppliers (供应商)
  'aws-bedrock': { text: 'AWS', color: '#F59E0B' },
  openrouter: { text: 'OR', color: '#6366F1' },
}

export const SUPPLIER_TYPE: Record<string, { text: string; tone: 'primary' | 'accent' | 'warning' | 'success' }> = {
  official: { text: '官方直连', tone: 'success' },
  cloud: { text: '云平台', tone: 'primary' },
  reseller: { text: '代理商', tone: 'warning' },
  self_hosted: { text: '自建', tone: 'accent' },
}

export const VENDOR_ROUTING: Record<string, { text: string; hint: string }> = {
  cost_first: { text: '成本优先', hint: '按折后单价从低到高选择供应商' },
  supplier_priority: { text: '供应商优先级', hint: '按供应商优先级从小到大尝试，失败切下一家' },
  supplier_weighted: { text: '供应商权重', hint: '按供应商权重比例分流，失败切下一家' },
}

export const KEY_CATEGORY: Record<string, { text: string; tone: 'primary' | 'accent'; desc: string }> = {
  application: { text: '应用 Key', tone: 'primary', desc: '线上业务系统调用，归属应用，走预算与审批' },
  personal: { text: '个人编码 Key', tone: 'accent', desc: '员工日常编码（IDE / CLI 编码助手），归属个人与部门' },
}

// coding assistants supported for personal keys
export const CODING_TOOLS = ['Claude Code', 'Cursor', 'Cline', 'Continue', 'Codex CLI', 'JetBrains AI', '其他']
// default allowlist offered for personal coding keys
export const DEFAULT_CODING_MODELS = ['claude-sonnet-4', 'deepseek-v3', 'deepseek-r1', 'qwen3-coder-plus', 'kimi-k2', 'doubao-seed-code', 'glm-4.5']
export const vendorStyle = (code: string) => VENDOR_STYLE[code] ?? { text: (code[0] ?? '?').toUpperCase(), color: '#6B7280' }

export const STRATEGY_TEXT = { priority: '优先级', cost_first: '成本优先', round_robin: '加权轮询', supplier_priority: '供应商优先级', supplier_weighted: '供应商权重' } as const
export const STRATEGY_HINT = {
  priority: '按优先级从小到大依次尝试，失败自动切换下一个',
  cost_first: '按折扣后单价从低到高尝试',
  round_robin: '按权重随机分流，失败自动切换',
  supplier_priority: '按「厂商→供应商」中配置的供应商优先级排序',
  supplier_weighted: '按「厂商→供应商」中配置的供应商权重分流',
} as const

// Budget approval routing (mirrors the backend default budget.director_limit).
export const DIRECTOR_LIMIT = 10000
