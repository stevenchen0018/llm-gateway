import http, { TOKEN_KEY, getDeptScope } from './http'
import type {
  AdminUser, AlertEvent, Vendor, VendorSupplier, VendorView, Announcement, DepartmentView, Me, Application, ApiKey, AuditLog, Budget, CapacityView, CategoryBoard, ChatResult,
  BudgetSummary, DigestItem, ImportResult, TransferEntity, TransferJob, FilterRule, PageParams, PageResult, FilterTestResult, GatewaySettings, ImageResult, RequestLog, VideoTask, Model, OverviewRow, Provider, RankRow, Route, RouteTrace, SeriesResult, Suggestion,
  UsageLog, UsageSummary,
} from '../types'

// Go serializes nil slices as null; normalise list responses to [].
const list = async <T>(p: Promise<unknown>): Promise<T[]> => ((await p) as T[] | null) ?? []
// paged list endpoints: same URL as the plain list, called with page/page_size
const paged = async <T>(p: Promise<unknown>): Promise<PageResult<T>> => {
  const r = (await p) as PageResult<T>
  return { ...r, items: r.items ?? [] }
}
const one = <T>(p: Promise<unknown>) => p as Promise<T>

export interface Filter {
  from?: string
  to?: string
  key_id?: number
  model_id?: number
  provider_id?: number
  app_id?: number
  category?: string
  vendor_id?: number
  key_category?: string
}

// drop undefined so they don't become "undefined" query strings
const clean = <T extends object>(o: T) => Object.fromEntries(Object.entries(o).filter(([, v]) => v !== undefined && v !== '' && v !== null))

export const auth = {
  login: (username: string, password: string) =>
    one<{ token: string; expires_at: string; user: Me }>(http.post('/admin/v1/auth/login', { username, password })),
  me: () => one<Me>(http.get('/admin/v1/auth/me')),
  changePassword: (old_password: string, new_password: string) => http.put('/admin/v1/auth/password', { old_password, new_password }),
}

export const tenancy = {
  departments: () => list<DepartmentView>(http.get('/admin/v1/departments')),
  departmentsPage: (p: PageParams) => paged<DepartmentView>(http.get('/admin/v1/departments', { params: clean(p) })),
  usersPage: (p: PageParams) => paged<AdminUser>(http.get('/admin/v1/users', { params: clean(p) })),
  createDepartment: (b: object) => http.post('/admin/v1/departments', b),
  updateDepartment: (id: number, b: object) => http.put(`/admin/v1/departments/${id}`, b),
  deleteDepartment: (id: number) => http.delete(`/admin/v1/departments/${id}`),
  users: () => list<AdminUser>(http.get('/admin/v1/users')),
  createUser: (b: object) => one<AdminUser>(http.post('/admin/v1/users', b)),
  updateUser: (id: number, b: object) => one<AdminUser>(http.put(`/admin/v1/users/${id}`, b)),
  resetPassword: (id: number, password: string) => http.put(`/admin/v1/users/${id}/password`, { password }),
}

export const providers = {
  list: () => list<Provider>(http.get('/admin/v1/providers')),
  page: (p: PageParams) => paged<Provider>(http.get('/admin/v1/providers', { params: clean(p) })),
  create: (b: object) => one<Provider>(http.post('/admin/v1/providers', b)),
  update: (id: number, b: object) => one<Provider>(http.put(`/admin/v1/providers/${id}`, b)),
}

export const vendors = {
  list: () => list<VendorView>(http.get('/admin/v1/vendors')),
  page: (p: PageParams) => paged<VendorView>(http.get('/admin/v1/vendors', { params: clean(p) })),
  create: (b: object) => one<Vendor>(http.post('/admin/v1/vendors', b)),
  update: (id: number, b: object) => one<Vendor>(http.put(`/admin/v1/vendors/${id}`, b)),
  remove: (id: number) => http.delete(`/admin/v1/vendors/${id}`),
  addSupplier: (vendorId: number, b: object) => one<VendorSupplier>(http.post(`/admin/v1/vendors/${vendorId}/suppliers`, b)),
  updateSupplier: (id: number, b: object) => one<VendorSupplier>(http.put(`/admin/v1/vendor-suppliers/${id}`, b)),
  removeSupplier: (id: number) => http.delete(`/admin/v1/vendor-suppliers/${id}`),
}

export const models = {
  list: () => list<Model>(http.get('/admin/v1/models')),
  create: (b: object) => one<Model>(http.post('/admin/v1/models', b)),
  update: (id: number, b: object) => one<Model>(http.put(`/admin/v1/models/${id}`, b)),
  try: (id: number, prompt: string) => one<ChatResult>(http.post(`/admin/v1/models/${id}/try`, { prompt })),
  mine: () => list<Model>(http.get('/admin/v1/my-models')),
  minePage: (p: PageParams) => paged<Model>(http.get('/admin/v1/my-models', { params: clean(p) })),
  addMine: (model_id: number) => http.post('/admin/v1/my-models', { model_id }),
  removeMine: (id: number) => http.delete(`/admin/v1/my-models/${id}`),
}

export const playground = {
  chat: (b: { model_id: number; messages: { role: string; content: string }[]; temperature?: number; max_tokens?: number }) =>
    one<ChatResult>(http.post('/admin/v1/playground/chat', b)),
  vision: (b: { model_id: number; prompt: string; image: string }) => one<ChatResult>(http.post('/admin/v1/playground/vision', b)),
  image: (b: { model_id: number; prompt: string; size?: string }) => one<ImageResult>(http.post('/admin/v1/playground/image', b)),
  submitVideo: (b: { model_id: number; prompt: string; size: string; seconds: number; image?: string }) =>
    one<VideoTask>(http.post('/admin/v1/playground/video', b)),
  video: (modelId: number, taskId: string) => one<VideoTask>(http.get(`/admin/v1/playground/video/${encodeURIComponent(taskId)}`, { params: { model_id: modelId } })),
  // vendor files that need credentials are fetched through the gateway as a blob
  videoBlobUrl: async (modelId: number, taskId: string) => {
    const token = localStorage.getItem('llmgw_token')
    const res = await fetch(`/admin/v1/playground/video/${encodeURIComponent(taskId)}/content?model_id=${modelId}`, { headers: { Authorization: `Bearer ${token}` } })
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    return URL.createObjectURL(await res.blob())
  },
}

export const routes = {
  list: () => list<Route>(http.get('/admin/v1/routing-policies')),
  create: (b: object) => one<Route>(http.post('/admin/v1/routing-policies', b)),
  update: (id: number, b: object) => one<Route>(http.put(`/admin/v1/routing-policies/${id}`, b)),
  remove: (id: number) => http.delete(`/admin/v1/routing-policies/${id}`),
  simulate: (b: { api_key_id: number; model: string }) => one<{ trace: RouteTrace; resolved: boolean }>(http.post('/admin/v1/routing/simulate', b)),
}

export const keys = {
  list: () => list<ApiKey>(http.get('/admin/v1/keys')),
  apply: (b: object) => one<ApiKey>(http.post('/admin/v1/keys', b)),
  approve: (id: number) => one<{ secret?: string; claim_required?: boolean }>(http.put(`/admin/v1/keys/${id}/approve`)),
  blacklist: (id: number, reason: string) => http.put(`/admin/v1/keys/${id}/blacklist`, { reason }),
  restore: (id: number) => http.put(`/admin/v1/keys/${id}/restore`),
  search: (p: { q?: string; status?: string; key_type?: string; category?: string; app_id?: number; ids?: string; page?: number; page_size?: number }) =>
    paged<ApiKey>(http.get('/admin/v1/keys/search', { params: clean(p) })),
  mine: (p: PageParams) => paged<ApiKey>(http.get('/admin/v1/keys/mine', { params: clean(p) })),
  claim: (id: number) => one<{ secret: string }>(http.post(`/admin/v1/keys/${id}/claim`)),
  rotate: (id: number) => one<{ secret?: string; claim_required?: boolean }>(http.post(`/admin/v1/keys/${id}/rotate`)),
  allowedModels: (id: number, models: string[]) => one<ApiKey>(http.put(`/admin/v1/keys/${id}/models`, { models })),
  ipWhitelist: (id: number, ips: string[]) => one<ApiKey>(http.put(`/admin/v1/keys/${id}/ip-whitelist`, { ips })),
  quota: (id: number, tpm_quota: number, qps_quota: number) => one<ApiKey>(http.put(`/admin/v1/keys/${id}/quota`, { tpm_quota, qps_quota })),
  history: (id: number) => list<AuditLog>(http.get(`/admin/v1/keys/${id}/history`)),
  auditLogs: (limit = 200) => list<AuditLog>(http.get('/admin/v1/audit-logs', { params: { limit } })),
  auditPage: (p: PageParams) => paged<AuditLog>(http.get('/admin/v1/audit-logs', { params: clean(p) })),
}

export const budgets = {
  list: () => list<Budget>(http.get('/admin/v1/budgets')),
  page: (p: PageParams) => paged<Budget>(http.get('/admin/v1/budgets', { params: clean(p) })),
  summary: () => one<BudgetSummary>(http.get('/admin/v1/budgets/summary')),
  create: (b: object) => one<Budget>(http.post('/admin/v1/budgets', b)),
  approve: (id: number) => one<Budget>(http.put(`/admin/v1/budgets/${id}/approve`)),
  reject: (id: number, reason: string) => one<Budget>(http.put(`/admin/v1/budgets/${id}/reject`, { reason })),
}

export const platform = {
  applications: () => list<Application>(http.get('/admin/v1/applications')),
  applicationsPage: (p: PageParams) => paged<Application>(http.get('/admin/v1/applications', { params: clean(p) })),
  createApplication: (b: object) => one<Application>(http.post('/admin/v1/applications', b)),
  updateApplication: (id: number, b: object) => one<Application>(http.put(`/admin/v1/applications/${id}`, b)),
  announcementsPage: (p: PageParams) => paged<Announcement>(http.get('/admin/v1/announcements', { params: clean(p) })),
  announcements: (activeOnly = false) => list<Announcement>(http.get('/admin/v1/announcements', { params: activeOnly ? { active: true } : {} })),
  createAnnouncement: (b: object) => one<Announcement>(http.post('/admin/v1/announcements', b)),
  updateAnnouncement: (id: number, b: object) => one<Announcement>(http.put(`/admin/v1/announcements/${id}`, b)),
  deleteAnnouncement: (id: number) => http.delete(`/admin/v1/announcements/${id}`),
}

export const monitor = {
  series: (p: Filter & { group_by: string; step?: number }) => one<SeriesResult>(http.get('/admin/v1/monitor/timeseries', { params: clean(p) })),
  ranking: (p: Filter & { group_by: string }) => list<RankRow>(http.get('/admin/v1/monitor/ranking', { params: clean(p) })),
  capacity: (window_minutes = 60) => one<CapacityView>(http.get('/admin/v1/capacity', { params: { window_minutes } })),
}

export const cost = {
  overview: (p: Filter & { group_by: string }) => list<OverviewRow>(http.get('/admin/v1/cost/overview', { params: clean(p) })),
  benchmark: (p: Filter) => list<CategoryBoard>(http.get('/admin/v1/cost/benchmark', { params: clean(p) })),
  suggestions: (p: Filter) => list<Suggestion>(http.get('/admin/v1/cost/suggestions', { params: clean(p) })),
  digest: (days = 7) => list<DigestItem>(http.get('/admin/v1/cost/digest', { params: { days } })),
  sendDigest: (days = 7) => one<{ sent: number }>(http.post('/admin/v1/cost/digest/send', null, { params: { days } })),
}

export const dashboard = {
  usage: (params: { from: string; to: string; group_by: string }) =>
    list<UsageSummary>(http.get('/admin/v1/dashboard/usage', { params })),
  alerts: (limit = 50) => list<AlertEvent>(http.get('/admin/v1/alerts', { params: { limit } })),
  alertsPage: (p: PageParams) => paged<AlertEvent>(http.get('/admin/v1/alerts', { params: clean(p) })),
  logs: (p: { from?: string; to?: string; key_id?: number; model_id?: number; status?: string; page: number; page_size: number }) =>
    paged<UsageLog>(http.get('/admin/v1/logs', { params: clean(p) })),
}

export const security = {
  settings: () => one<GatewaySettings>(http.get('/admin/v1/security/settings')),
  updateSettings: (b: GatewaySettings) => one<GatewaySettings>(http.put('/admin/v1/security/settings', b)),
  rules: () => list<FilterRule>(http.get('/admin/v1/security/filter-rules')),
  rulesPage: (p: PageParams) => paged<FilterRule>(http.get('/admin/v1/security/filter-rules', { params: clean(p) })),
  createRule: (b: object) => one<FilterRule>(http.post('/admin/v1/security/filter-rules', b)),
  updateRule: (id: number, b: object) => one<FilterRule>(http.put(`/admin/v1/security/filter-rules/${id}`, b)),
  deleteRule: (id: number) => http.delete(`/admin/v1/security/filter-rules/${id}`),
  test: (b: { text: string; department_id?: number; rule?: object }) => one<FilterTestResult>(http.post('/admin/v1/security/filter-test', b)),
  requestLogs: (p: { from?: string; to?: string; key_id?: number; model_id?: number; status?: string; q?: string; request_id?: string; filter_hit?: boolean; page: number; page_size: number }) =>
    paged<RequestLog>(http.get('/admin/v1/request-logs', { params: clean(p) })),
  requestLog: (id: number) => one<RequestLog>(http.get(`/admin/v1/request-logs/${id}`)),
  requestLogByRequestId: (request_id: string) => one<RequestLog>(http.get('/admin/v1/request-logs/by-request', { params: { request_id } })),
}

// ---- bulk import / export ---------------------------------------------------------------

// fetch a file with the console's auth + department scope and hand it to the browser
async function download(path: string, params: Record<string, unknown> = {}) {
  const q = new URLSearchParams()
  const scope = getDeptScope()
  if (scope && params.department_id === undefined) q.set('department_id', String(scope))
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== null && v !== '') q.set(k, String(v))
  const res = await fetch(`${path}?${q}`, { headers: { Authorization: `Bearer ${localStorage.getItem(TOKEN_KEY) ?? ''}` } })
  if (!res.ok) {
    const msg = await res.json().then((j) => j?.error?.message).catch(() => '')
    throw new Error(msg || `下载失败（HTTP ${res.status}）`)
  }
  const cd = res.headers.get('Content-Disposition') ?? ''
  const m = /filename\*=UTF-8''([^;]+)/i.exec(cd)
  const name = m ? decodeURIComponent(m[1]) : 'export'
  const url = URL.createObjectURL(await res.blob())
  const a = document.createElement('a')
  a.href = url; a.download = name; document.body.appendChild(a); a.click(); a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 2000)
  return { name, rows: Number(res.headers.get('X-Export-Rows') ?? 0), truncated: res.headers.get('X-Export-Truncated') === '1' }
}

export const transfer = {
  entities: () => list<TransferEntity>(http.get('/admin/v1/transfer/entities')),
  jobs: (p: PageParams) => paged<TransferJob>(http.get('/admin/v1/transfer/jobs', { params: clean(p) })),
  template: (entity: string, format: 'xlsx' | 'csv' = 'xlsx') => download(`/admin/v1/transfer/${entity}/template`, { format }),
  export: (entity: string, format: 'xlsx' | 'csv', filters: Record<string, unknown> = {}) => download(`/admin/v1/transfer/${entity}/export`, { ...filters, format }),
  import: (entity: string, file: File, opts: { mode: 'validate' | 'commit'; on_conflict: string; skip_invalid: boolean }) => {
    const fd = new FormData()
    fd.append('file', file); fd.append('mode', opts.mode); fd.append('on_conflict', opts.on_conflict); fd.append('skip_invalid', String(opts.skip_invalid))
    return one<ImportResult>(http.post(`/admin/v1/transfer/${entity}/import`, fd, { timeout: 180_000 }))
  },
}
