import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { http } from '@/utils/http'

interface LoginResponse {
  token: string
  user: { uuid: string; email: string; role: string; display_name: string }
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string | null>(localStorage.getItem('token'))
  const user  = ref<LoginResponse['user'] | null>(
    JSON.parse(localStorage.getItem('user') ?? 'null'),
  )

  const isLoggedIn = computed(() => !!token.value)
  const role       = computed(() => user.value?.role ?? '')

  async function login(email: string, password: string) {
    const res = (await http.post('/auth/login', { email, password })) as LoginResponse
    token.value = res.token
    user.value  = res.user
    localStorage.setItem('token', res.token)
    localStorage.setItem('user', JSON.stringify(res.user))
  }

  function logout() {
    token.value = null
    user.value  = null
    localStorage.removeItem('token')
    localStorage.removeItem('user')
  }

  return { token, user, isLoggedIn, role, login, logout }
})
