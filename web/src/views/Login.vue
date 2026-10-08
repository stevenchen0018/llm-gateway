<template>
  <div class="login">
    <section class="brand-pane">
      <div class="brand"><Logo :size="30" /><b>LLM Gateway</b></div>
      <div class="pitch">
        <h1>统一管理企业大模型的<br />流量、成本与稳定性</h1>
        <ul>
          <li><i /><div><b>统一接入</b><span>OpenAI 兼容 API，屏蔽多厂商差异</span></div></li>
          <li><i /><div><b>智能调度</b><span>按成本与优先级路由，分钟级容灾切换</span></div></li>
          <li><i /><div><b>成本治理</b><span>Key 级 TPM/QPS 限流与预算预警闭环</span></div></li>
        </ul>
      </div>
      <div class="copy">AI Infrastructure · Gateway Console</div>
    </section>

    <section class="form-pane">
      <form class="box" @submit.prevent="submit">
        <h2>登录控制台</h2>
        <p class="muted">使用管理员账号访问网关管理后台</p>
        <label>用户名<el-input v-model="form.username" size="large" placeholder="请输入用户名" autocomplete="username" /></label>
        <label>密码<el-input v-model="form.password" type="password" show-password size="large" placeholder="请输入密码" autocomplete="current-password" /></label>
        <el-button type="primary" size="large" native-type="submit" :loading="loading" style="width: 100%; margin-top: 8px">登录</el-button>
      </form>
    </section>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { auth } from '../api'
import { TOKEN_KEY } from '../api/http'
import { setMe, setScope } from '../composables/useAuth'
import Logo from '../components/Logo.vue'

const router = useRouter()
const route = useRoute()
const form = reactive({ username: '', password: '' })
const loading = ref(false)

async function submit() {
  if (!form.username || !form.password) return
  loading.value = true
  try {
    const res = await auth.login(form.username, form.password)
    localStorage.setItem(TOKEN_KEY, res.token)
    setMe(res.user)
    setScope(undefined)
    router.replace((route.query.redirect as string) || '/')
  } finally { loading.value = false }
}
</script>

<style scoped>
.login { height: 100%; display: grid; grid-template-columns: minmax(360px, 5fr) 6fr; background: var(--card); }
@media (max-width: 860px) { .login { grid-template-columns: 1fr; } .brand-pane { display: none !important; } }
.brand-pane { background: var(--sidebar); color: var(--sidebar-text); padding: 32px 48px; display: flex; flex-direction: column; justify-content: space-between; }
.brand { display: flex; align-items: center; gap: 10px; color: #fff; font-size: 16px; }
.pitch h1 { color: #fff; font-size: 26px; line-height: 38px; font-weight: 600; letter-spacing: -.01em; }
ul { list-style: none; padding: 0; margin: 32px 0 0; display: grid; gap: 18px; }
li { display: flex; gap: 12px; }
li i { width: 6px; height: 6px; margin-top: 8px; border-radius: 50%; background: var(--accent); flex: none; }
li b { display: block; color: #fff; font-weight: 500; }
li span { display: block; color: #94A3B8; font-size: 13px; margin-top: 2px; }
.copy { font-size: 12px; color: #64748B; }
.form-pane { display: flex; align-items: center; justify-content: center; padding: 24px; }
.box { width: 360px; }
h2 { font-size: 22px; font-weight: 600; letter-spacing: -.01em; }
.box > p { margin: 4px 0 28px; }
label { display: block; font-size: 12px; font-weight: 500; color: var(--text-2); margin-bottom: 16px; }
label .el-input { display: flex; width: 100%; margin-top: 6px; }
</style>
