import { computed, reactive } from 'vue'
import { auth } from '../api'
import { SCOPE_KEY, TOKEN_KEY, getDeptScope } from '../api/http'
import type { Me, Role } from '../types'

const state = reactive({ me: null as Me | null, scope: getDeptScope() as number | undefined })

export const ROLE_TEXT: Record<Role, string> = { super_admin: '超级管理员', dept_admin: '部门管理员', viewer: '只读成员' }
export const ROLE_TONE: Record<Role, 'accent' | 'primary' | 'info'> = { super_admin: 'accent', dept_admin: 'primary', viewer: 'info' }

export async function loadMe() {
  state.me = await auth.me()
  if (!state.me.permissions.super) setScope(undefined) // a stale super-admin scope must not leak
  return state.me
}

export function setMe(me: Me) { state.me = me }

export function clearAuth() {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(SCOPE_KEY)
  state.me = null
  state.scope = undefined
}

export function setScope(id: number | undefined) {
  state.scope = id
  if (id) localStorage.setItem(SCOPE_KEY, String(id))
  else localStorage.removeItem(SCOPE_KEY)
}

export function useAuth() {
  const perms = computed(() => state.me?.permissions ?? { super: false, write: false, manage_users: false, manage_depts: false, platform: false, view_prompts: false, security: false })
  return {
    state,
    me: computed(() => state.me),
    isSuper: computed(() => perms.value.super),
    canWrite: computed(() => perms.value.write),
    canPlatform: computed(() => perms.value.platform),
    canViewPrompts: computed(() => perms.value.view_prompts),
    // a department user's own department, or the super admin's selected scope
    effectiveDept: computed(() => (perms.value.super ? state.scope : state.me?.department_id ?? undefined)),
  }
}
