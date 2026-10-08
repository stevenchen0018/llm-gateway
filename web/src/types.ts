// 供应商: a supply channel (official API, cloud platform, reseller, self-hosted)
export type SupplierType = 'official' | 'cloud' | 'reseller' | 'self_hosted'

export interface Provider {
  id: number
  code: string
  name: string
  base_url: string
  auth_type: string
  status: 'active' | 'disabled'
  discount_rate: string
  description: string
  supplier_type: SupplierType
  contact: string
  created_at: string
}

// 厂商: the model maker; supplied by several 供应商
export type VendorRouting = 'cost_first' | 'supplier_priority' | 'supplier_weighted'

export interface Vendor {
  id: number
  code: string
  name: string
  description: string
  website: string
  routing_strategy: VendorRouting
  status: 'active' | 'disabled'
  created_at: string
}

export interface VendorSupplier {
  id: number
  vendor_id: number
  provider_id: number
  priority: number
  weight: number
  status: 'active' | 'disabled'
  remark: string
}

export interface VendorView extends Vendor {
  suppliers: (VendorSupplier & { provider?: Provider; model_count: number })[]
  model_count: number
}

export interface Model {
  id: number
  provider_id: number
  model_key: string
  display_name: string
  type: 'chat' | 'embedding'
  category: string
  context_length: number
  tags: string[]
  description: string
  released_at?: string | null
  input_price_per_1k: string
  output_price_per_1k: string
  tpm_limit: number
  qps_limit: number
  status: 'active' | 'disabled'
  created_by: string
  provider: Provider
  vendor_id?: number | null
  vendor?: Vendor | null
  supply?: VendorSupplier | null
}

export type Strategy = 'priority' | 'cost_first' | 'round_robin' | 'supplier_priority' | 'supplier_weighted'

export interface Route {
  id: number
  name: string
  alias: string
  candidate_model_id: number
  priority: number
  weight: number
  strategy: Strategy
  enabled: boolean
  app_id?: number | null
  api_key_id?: number | null
  source_type: 'custom' | 'vendor'
  source_vendor_id?: number | null
  remark: string
}

export type KeyStatus = 'pending' | 'active' | 'blacklisted'

export interface ApiKey {
  id: number
  key_prefix: string
  name: string
  scenario: string
  owner: string
  owner_email: string
  manager: string
  manager_email: string
  shared_users: string[] | null
  status: KeyStatus
  key_type: 'formal' | 'trial'
  tpm_quota: number
  qps_quota: number
  budget_id?: number | null
  app_id?: number | null
  department_id?: number | null
  expires_at?: string | null
  blacklist_reason: string
  blacklisted_at?: string | null
  ip_whitelist: string[] | null
  category: KeyCategory
  holder_user_id?: number | null
  employee_no: string
  coding_tools: string[] | null
  allowed_models: string[] | null
  created_at: string
}

export type KeyCategory = 'application' | 'personal'

export interface AuditLog {
  id: number
  key_id: number
  action: 'apply' | 'approve' | 'blacklist' | 'restore' | 'update_quota' | 'update_ip_whitelist' | 'update_models' | 'claim' | 'rotate'
  operator: string
  detail: string
  created_at: string
}

export type BudgetApproval = 'pending' | 'approved' | 'rejected'

export interface Budget {
  id: number
  key_id: number
  period: 'monthly' | 'quarterly' | 'none'
  amount: string
  consumed: string
  currency: string
  alert_threshold_pct: number
  status: 'active' | 'exhausted' | 'closed' | 'pending'
  approval_status: BudgetApproval
  approver_level: 'D' | 'CTO' | ''
  approver: string
  applicant: string
  project: string
  reason: string
  reject_reason: string
  approved_at?: string | null
}

export interface Application {
  id: number
  name: string
  description: string
  department_id?: number | null
  owner: string
  owner_email: string
  manager: string
  manager_email: string
  status: string
}

export interface Announcement {
  id: number
  content: string
  level: 'info' | 'warning'
  active: boolean
  created_at: string
}

export interface UsageSummary {
  bucket: string
  group_key: string
  request_count: number
  failed_count: number
  total_tokens: number
  cost: string
  avg_latency_ms: number
}

export interface AlertEvent {
  id: number
  type: 'quota' | 'budget' | 'failover' | 'report' | 'security'
  ref_id: number
  message: string
  level: 'info' | 'warning' | 'critical'
  notified_at?: string | null
  created_at: string
}

export interface ChatResult {
  id: string
  model: string
  choices: { message: { role: string; content: string } }[]
  usage: { prompt_tokens: number; completion_tokens: number; total_tokens: number }
  latency_ms?: number
}

export interface ImageResult {
  model: string
  images: string[]
  latency_ms: number
}

// ---- monitoring --------------------------------------------------------------

export interface SeriesPoint {
  ts: string
  requests: number
  failed: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  latency_sum_ms: number
  cost: string
}

export interface Series {
  group_key: string
  points: SeriesPoint[]
}

export interface SeriesResult {
  step_seconds: number
  from: string
  to: string
  series: Series[]
}

export interface RankRow {
  group_key: string
  requests: number
  failed: number
  failure_rate: number
  avg_latency_ms: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  cost: number
}

export interface CapacityRow {
  id: number
  name: string
  sub: string
  status: string
  tpm_limit: number
  qps_limit: number
  peak_rpm: number
  peak_tpm: number
  last_rpm: number
  last_tpm: number
  tpm_util: number
  qps_util: number
}

export interface CapacityView {
  window_minutes: number
  keys: CapacityRow[]
  models: CapacityRow[]
  departments: CapacityRow[]
}

// ---- cost ------------------------------------------------------------------------

export interface OverviewRow {
  group_key: string
  requests: number
  tokens: number
  cost: number
  list_cost: number
  saved: number
  price_per_m: number
  share_of_cost: number
}

export interface ModelPrice {
  model_id: number
  name: string
  provider: string
  category: string
  list_per_m: number
  effective_per_m: number
  observed: boolean
  tokens: number
  cost: number
  discount: number
  vs_avg_pct: number
  rank: number
}

export interface CategoryBoard {
  category: string
  avg_per_m: number
  min_per_m: number
  max_per_m: number
  tokens: number
  cost: number
  models: ModelPrice[]
}

export interface Suggestion {
  from_model_id: number
  from_name: string
  from_provider: string
  to_model_id: number
  to_name: string
  to_provider: string
  category: string
  from_per_m: number
  to_per_m: number
  saving_pct: number
  tokens: number
  saving_in_range: number
  saving_monthly: number
}

export interface DigestItem {
  owner: string
  owner_email: string
  manager: string
  manager_email: string
  apps: string[]
  keys: number
  top_key: string
  requests: number
  tokens: number
  cost: number
  prev_cost: number
  change_pct: number
}

export interface UsageLog {
  id: number
  request_id: string
  key_id: number
  model_id: number
  provider_id: number
  alias: string
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  cost: string
  list_cost: string
  latency_ms: number
  status: 'success' | 'failed'
  error_code?: string
  source_ip: string
  created_at: string
}

// ---- routing ----------------------------------------------------------------------

export interface RouteTrace {
  requested_name: string
  vendor: string
  pin_kind: 'supplier' | 'vendor' | ''
  model_name: string
  matched: Model[]
  scope: 'key' | 'global' | 'direct'
  policies: Route[] | null
  strategy: Strategy
  candidates: { route: Route; model: Model }[] | null
}

// ---- identity & tenancy --------------------------------------------------------

export type Role = 'super_admin' | 'dept_admin' | 'viewer'

export interface Department {
  id: number
  code: string
  name: string
  description: string
  leader: string
  tpm_quota: number
  qps_quota: number
  status: 'active' | 'disabled'
  created_at: string
}

export interface DepartmentView extends Department {
  users: number
  apps: number
  keys: number
  requests_30d: number
  tokens_30d: number
  cost_30d: number
}

export interface AdminUser {
  id: number
  username: string
  display_name: string
  email: string
  role: Role
  department_id?: number | null
  status: 'active' | 'disabled'
  last_login_at?: string | null
  created_at: string
}

export interface Me extends AdminUser {
  department?: Department | null
  permissions: { super: boolean; write: boolean; manage_users: boolean; manage_depts: boolean; platform: boolean; view_prompts: boolean; security: boolean; apply_personal: boolean }
}

// ---- video generation ------------------------------------------------------------

export interface VideoTask {
  id: string
  model: string
  status: 'queued' | 'running' | 'succeeded' | 'failed'
  progress: number
  video_url?: string
  content_proxied?: boolean
  error?: string
  size: string
  seconds: number
  created_at: string
}

// ---- security & compliance ---------------------------------------------------------

export interface GatewaySettings {
  request_log: { enabled: boolean; capture_body: boolean; max_body_kb: number; mask_sensitive: boolean; retention_days: number }
  content_filter: { enabled: boolean; check_output: boolean; block_message: string }
}

export type FilterAction = 'block' | 'mask' | 'log'
export type FilterStage = 'input' | 'output' | 'both'

export interface FilterRule {
  id: number
  name: string
  description: string
  match_type: 'keyword' | 'regex'
  pattern: string
  action: FilterAction
  replacement: string
  stage: FilterStage
  department_id?: number | null
  priority: number
  enabled: boolean
  hit_count: number
  last_hit_at?: string | null
  created_by: string
  created_at: string
  updated_at: string
}

export interface FilterHit {
  rule_id: number
  rule: string
  action: FilterAction
  stage: 'input' | 'output'
  matches: number
  sample?: string
}

export interface FilterTestResult {
  hits: FilterHit[] | null
  blocked: boolean
  block_by?: string
  output: string
}

export interface RequestLog {
  id: number
  request_id: string
  key_id: number
  endpoint: string
  model: string
  model_id?: number | null
  provider_id?: number | null
  status: 'success' | 'failed' | 'blocked'
  http_status: number
  error_code: string
  prompt_preview: string
  request_body?: string
  response_body?: string
  body_truncated: boolean
  filter_hits: FilterHit[] | null
  prompt_tokens: number
  completion_tokens: number
  latency_ms: number
  source_ip: string
  user_agent: string
  created_at: string
}

// ---- pagination ---------------------------------------------------------------------

// Every admin list endpoint returns this envelope when called with page/page_size.
export interface PageResult<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface PageParams {
  page: number
  page_size: number
  q?: string
  [filter: string]: string | number | boolean | undefined
}

export interface BudgetSummary {
  total: number
  pending: number
  pending_d: number
  pending_cto: number
  approved: number
  rejected: number
  amount: number
  consumed: number
  attention: number
  open_key_ids: number[]
}

// ---- bulk import / export ---------------------------------------------------------------

export interface TransferColumn {
  key: string
  title: string
  required: boolean
  desc: string
  example: string
  options?: string[]
  import_only?: boolean
  export_only?: boolean
}

export interface TransferEntity {
  name: string
  title: string
  group: string
  desc: string
  can_export: boolean
  can_import: boolean
  columns: TransferColumn[]
}

export interface ImportError { row: number; column: string; message: string }

export interface ImportResult {
  total: number
  valid: number
  invalid: number
  create: number
  update: number
  skip: number
  created: number
  updated: number
  skipped: number
  failed: number
  committed: boolean
  message?: string
  errors: ImportError[]
  errors_truncated?: boolean
  preview: { row: number; op: 'create' | 'update' | 'skip'; summary: string }[]
}

export interface TransferJob {
  id: number
  direction: 'import' | 'export'
  entity: string
  entity_title: string
  format: string
  file_name: string
  operator: string
  status: 'success' | 'partial' | 'failed'
  total: number
  created: number
  updated: number
  skipped: number
  failed: number
  message: string
  errors: ImportError[]
  created_at: string
}
