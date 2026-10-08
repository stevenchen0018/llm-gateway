import { createRouter, createWebHistory } from 'vue-router'
import { TOKEN_KEY } from './api/http'
import { clearAuth, loadMe, useAuth } from './composables/useAuth'

// perm: which permission (from /auth/me) is needed to open the page
type Perm = 'super' | 'write' | 'manage_users' | 'manage_depts' | 'view_prompts'
const route = (path: string, view: () => Promise<unknown>, title: string, icon: string, perm?: Perm) => ({
  path, component: view as () => Promise<never>, meta: { title, icon, perm },
})

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('./views/Login.vue'), meta: { public: true } },
    {
      path: '/',
      component: () => import('./layout/AppLayout.vue'),
      redirect: '/dashboard',
      children: [
        route('dashboard', () => import('./views/Dashboard.vue'), '总览大盘', 'DataAnalysis'),
        route('market', () => import('./views/Market.vue'), '模型市场', 'Shop'),
        route('my-models', () => import('./views/MyModels.vue'), '我的模型', 'Collection'),
        route('routing', () => import('./views/Routing.vue'), '模型调度', 'Guide'),
        route('playground/text', () => import('./views/playground/TextPlayground.vue'), '文本生成', 'ChatDotRound'),
        route('playground/vision', () => import('./views/playground/VisionPlayground.vue'), '图片理解', 'Picture'),
        route('playground/image', () => import('./views/playground/ImagePlayground.vue'), '图片生成', 'MagicStick'),
        route('playground/video', () => import('./views/playground/VideoPlayground.vue'), '视频生成', 'VideoCamera'),
        route('monitor/model', () => import('./views/monitor/ModelMonitor.vue'), '模型监控', 'Monitor'),
        route('monitor/key', () => import('./views/monitor/KeyMonitor.vue'), 'Key 监控', 'TrendCharts'),
        route('capacity', () => import('./views/monitor/Capacity.vue'), '容量管理', 'Odometer'),
        route('alerts', () => import('./views/Alerts.vue'), '告警中心', 'Bell'),
        route('logs', () => import('./views/monitor/Logs.vue'), '调用日志', 'Document'),
        route('security/filter', () => import('./views/security/ContentFilter.vue'), '提示词过滤', 'Filter'),
        route('security/requests', () => import('./views/security/RequestLogs.vue'), '请求记录', 'ChatLineSquare', 'view_prompts'),
        route('cost', () => import('./views/cost/Cost.vue'), '成本大盘', 'Coin'),
        route('benchmark', () => import('./views/cost/Benchmark.vue'), '均价赛马', 'Trophy'),
        route('budgets', () => import('./views/Budgets.vue'), '预算管理', 'Wallet'),
        route('keys/mine', () => import('./views/keys/MyKeys.vue'), '我的 Key', 'Postcard'),
        route('keys/apply', () => import('./views/keys/KeyApply.vue'), 'Key 申请', 'DocumentAdd'),
        route('keys', () => import('./views/keys/Keys.vue'), 'Key 管理', 'Key'),
        route('blacklist', () => import('./views/keys/Blacklist.vue'), '黑名单', 'CircleClose'),
        route('docs', () => import('./views/docs/ApiDocs.vue'), 'API 文档', 'Reading'),
        route('departments', () => import('./views/platform/Departments.vue'), '部门管理', 'OfficeBuilding', 'manage_depts'),
        route('users', () => import('./views/platform/Users.vue'), '用户管理', 'User', 'manage_users'),
        route('applications', () => import('./views/platform/Applications.vue'), '应用管理', 'Grid'),
        route('vendors', () => import('./views/platform/Vendors.vue'), '厂商管理', 'Management'),
        route('providers', () => import('./views/platform/Providers.vue'), '供应商管理', 'Connection'),
        route('announcements', () => import('./views/platform/Announcements.vue'), '公告管理', 'Notification', 'super'),
        route('audit', () => import('./views/platform/AuditLogs.vue'), '审计日志', 'Tickets'),
        route('data', () => import('./views/platform/DataTransfer.vue'), '数据导入导出', 'Files'),
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

router.beforeEach(async (to) => {
  if (to.meta.public) return
  if (!localStorage.getItem(TOKEN_KEY)) return { path: '/login', query: { redirect: to.fullPath } }
  const { state } = useAuth()
  if (!state.me) {
    try { await loadMe() } catch { clearAuth(); return { path: '/login', query: { redirect: to.fullPath } } }
  }
  const perm = to.meta.perm as keyof NonNullable<typeof state.me>['permissions'] | undefined
  if (perm && !state.me!.permissions[perm]) return { path: '/dashboard' }
})

export default router
