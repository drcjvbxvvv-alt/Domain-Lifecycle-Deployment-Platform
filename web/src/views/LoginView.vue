<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  NForm, NFormItem, NInput, NButton,
  useMessage, type FormInst, type FormRules,
} from 'naive-ui'
import { useAuthStore } from '@/stores/auth'

const router  = useRouter()
const route   = useRoute()
const message = useMessage()
const auth    = useAuthStore()

// ── Form state ───────────────────────────────────────────────────────────────
const formRef = ref<FormInst | null>(null)
const loading = ref(false)
const showPwd = ref(false)

const model = ref({ email: '', password: '' })

const rules: FormRules = {
  email: [
    { required: true, message: '請輸入 Email',    trigger: 'blur' },
    { type: 'email',  message: 'Email 格式不正確', trigger: 'blur' },
  ],
  password: [
    { required: true, message: '請輸入密碼',    trigger: 'blur' },
    { min: 6,         message: '密碼至少 6 位', trigger: 'blur' },
  ],
}

// ── Error state ───────────────────────────────────────────────────────────────
const errorMsg = ref('')
const hasError = computed(() => errorMsg.value.length > 0)
function clearError() { errorMsg.value = '' }

// ── Submit ────────────────────────────────────────────────────────────────────
async function handleSubmit() {
  try { await formRef.value?.validate() } catch { return }

  loading.value  = true
  errorMsg.value = ''

  try {
    await auth.login(model.value.email, model.value.password)
    const redirect = (route.query.redirect as string) ?? '/'
    await router.push(redirect)
    message.success('登入成功')
  } catch (err: unknown) {
    const status = (err as { response?: { status?: number } })?.response?.status
    errorMsg.value = status === 401
      ? '帳號或密碼錯誤，請重新確認'
      : '登入失敗，請稍後再試'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-root">

    <!-- Background decoration -->
    <div class="bg-grid" />
    <div class="bg-glow bg-glow--tl" />
    <div class="bg-glow bg-glow--br" />

    <!-- Card -->
    <div class="login-card">

      <!-- Brand -->
      <div class="brand">
        <img src="/logo.svg" width="64" height="64" alt="Domain Platform logo" class="brand__logo" />
        <div class="brand__text">
          <h1 class="brand__title">Domain Platform</h1>
          <p class="brand__subtitle">域名全生命週期管理平台</p>
        </div>
      </div>

      <div class="divider" />

      <!-- Form -->
      <NForm
        ref="formRef"
        :model="model"
        :rules="rules"
        :show-label="false"
        size="large"
        @keydown.enter="handleSubmit"
      >
        <NFormItem path="email">
          <NInput
            v-model:value="model.email"
            placeholder="Email 帳號"
            :input-props="{ type: 'email', autocomplete: 'email' }"
            :status="hasError ? 'error' : undefined"
            @input="clearError"
          >
            <template #prefix>
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" class="field-icon">
                <path d="M20 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V6c0-1.1-.9-2-2-2z"
                      stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/>
                <path d="M22 6l-10 7L2 6"
                      stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/>
              </svg>
            </template>
          </NInput>
        </NFormItem>

        <NFormItem path="password">
          <NInput
            v-model:value="model.password"
            :type="showPwd ? 'text' : 'password'"
            placeholder="密碼"
            :input-props="{ autocomplete: 'current-password' }"
            :status="hasError ? 'error' : undefined"
            @input="clearError"
          >
            <template #prefix>
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" class="field-icon">
                <rect x="3" y="11" width="18" height="11" rx="2"
                      stroke="currentColor" stroke-width="1.8"/>
                <path d="M7 11V7a5 5 0 0110 0v4"
                      stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/>
              </svg>
            </template>
            <template #suffix>
              <span class="eye-toggle" @click="showPwd = !showPwd">
                <svg v-if="showPwd" width="15" height="15" viewBox="0 0 24 24" fill="none">
                  <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"
                        stroke="currentColor" stroke-width="1.8"/>
                  <circle cx="12" cy="12" r="3" stroke="currentColor" stroke-width="1.8"/>
                </svg>
                <svg v-else width="15" height="15" viewBox="0 0 24 24" fill="none">
                  <path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24M1 1l22 22"
                        stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/>
                </svg>
              </span>
            </template>
          </NInput>
        </NFormItem>
      </NForm>

      <!-- Error -->
      <Transition name="err">
        <div v-if="hasError" class="error-box">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" style="flex-shrink:0">
            <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="2"/>
            <path d="M12 8v4M12 16h.01" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
          </svg>
          {{ errorMsg }}
        </div>
      </Transition>

      <!-- Login button -->
      <NButton
        type="primary"
        size="large"
        block
        :loading="loading"
        class="submit-btn"
        @click="handleSubmit"
      >
        登入
      </NButton>

      <!-- Footer -->
      <p class="card-footer">
        Domain Lifecycle Management Platform &nbsp;·&nbsp; v0.1.0
      </p>
    </div>

  </div>
</template>

<style scoped>
/* ── Root ─────────────────────────────────────────────────────────────────── */
.login-root {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  background-color: var(--bg-page);
  overflow: hidden;
}

/* Dot-grid background */
.bg-grid {
  position: absolute;
  inset: 0;
  background-image: radial-gradient(circle, #334155 1px, transparent 1px);
  background-size: 28px 28px;
  opacity: 0.35;
  pointer-events: none;
}

/* Ambient colour glow */
.bg-glow {
  position: absolute;
  border-radius: 50%;
  filter: blur(88px);
  pointer-events: none;
}
.bg-glow--tl {
  width: 520px;
  height: 520px;
  top:  -180px;
  left: -180px;
  background: radial-gradient(circle, rgba(56, 189, 248, 0.13) 0%, transparent 70%);
}
.bg-glow--br {
  width: 420px;
  height: 420px;
  bottom: -130px;
  right:  -130px;
  background: radial-gradient(circle, rgba(192, 132, 252, 0.10) 0%, transparent 70%);
}

/* ── Card ─────────────────────────────────────────────────────────────────── */
.login-card {
  position: relative;
  z-index: 1;
  width: 400px;
  padding: 40px 36px 32px;
  background-color: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: 14px;
  box-shadow:
    0 0 0 1px rgba(56, 189, 248, 0.05),
    0 8px 32px rgba(0, 0, 0, 0.40),
    0 32px 64px rgba(0, 0, 0, 0.30);
}

/* ── Brand ────────────────────────────────────────────────────────────────── */
.brand {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 28px;
}

.brand__logo {
  flex-shrink: 0;
  border-radius: 14px;
}

.brand__text {
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.brand__title {
  font-size: 20px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: -0.3px;
  line-height: 1.2;
}

.brand__subtitle {
  font-size: 12px;
  color: var(--text-muted);
  letter-spacing: 0.2px;
}

/* ── Divider ──────────────────────────────────────────────────────────────── */
.divider {
  height: 1px;
  background: linear-gradient(to right, transparent, var(--border) 30%, var(--border) 70%, transparent);
  margin-bottom: 28px;
}

/* ── Field icons ──────────────────────────────────────────────────────────── */
.field-icon  { color: var(--text-muted); }

.eye-toggle {
  display: flex;
  align-items: center;
  color: var(--text-muted);
  cursor: pointer;
  transition: color 0.15s;
}
.eye-toggle:hover { color: var(--text-secondary); }

/* ── Error box ────────────────────────────────────────────────────────────── */
.error-box {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 10px 12px;
  margin-bottom: 14px;
  background-color: rgba(239, 68, 68, 0.10);
  border: 1px solid rgba(239, 68, 68, 0.28);
  border-radius: 6px;
  font-size: 13px;
  color: #f87171;
  line-height: 1.5;
}

/* ── Button ───────────────────────────────────────────────────────────────── */
.submit-btn {
  margin-top: 6px;
  height: 44px !important;
  font-size: 15px !important;
  font-weight: 600 !important;
  letter-spacing: 0.5px;
}

/* ── Footer ───────────────────────────────────────────────────────────────── */
.card-footer {
  margin-top: 22px;
  text-align: center;
  font-size: 11px;
  color: var(--text-muted);
  letter-spacing: 0.3px;
}

/* ── Error transition ─────────────────────────────────────────────────────── */
.err-enter-active, .err-leave-active { transition: all 0.2s ease; }
.err-enter-from,  .err-leave-to     { opacity: 0; transform: translateY(-6px); }
</style>
