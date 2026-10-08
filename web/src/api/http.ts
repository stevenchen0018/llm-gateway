import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '../router'

export const TOKEN_KEY = 'llmgw_token'
export const SCOPE_KEY = 'llmgw_dept_scope'

// Super admins can narrow the whole console to one department; the backend
// ignores this for department users (they are always confined to their own).
export function getDeptScope(): number | undefined {
  const v = Number(localStorage.getItem(SCOPE_KEY))
  return v > 0 ? v : undefined
}

// Endpoints whose results are about the whole platform, not a tenant slice.
const UNSCOPED = ['/admin/v1/departments', '/admin/v1/users', '/admin/v1/providers', '/admin/v1/models', '/admin/v1/announcements', '/admin/v1/auth/', '/admin/v1/my-models']

const http = axios.create({ baseURL: '', timeout: 90_000 })

http.interceptors.request.use((cfg) => {
  const token = localStorage.getItem(TOKEN_KEY)
  if (token) cfg.headers.Authorization = `Bearer ${token}`
  const scope = getDeptScope()
  const url = cfg.url ?? ''
  if (scope && (cfg.method ?? 'get').toLowerCase() === 'get' && url.startsWith('/admin/v1/') && !UNSCOPED.some((p) => url.startsWith(p))) {
    cfg.params = { department_id: scope, ...(cfg.params ?? {}) }
  }
  return cfg
})

// Admin envelope: success => {data}, failure => {error:{message}}.
http.interceptors.response.use(
  (res) => res.data?.data ?? null,
  (err) => {
    const status = err.response?.status
    const message = err.response?.data?.error?.message || err.message || '请求失败'
    if (status === 401 && router.currentRoute.value.path !== '/login') {
      localStorage.removeItem(TOKEN_KEY)
      router.replace({ path: '/login', query: { redirect: router.currentRoute.value.fullPath } })
    } else {
      ElMessage.error(message)
    }
    return Promise.reject(err)
  },
)

export default http
