import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'Login',
      component: () => import('@/views/LoginView.vue'),
      meta: { requiresAuth: false },
    },

    // ── Main layout ──────────────────────────────────────────
    {
      path: '/',
      component: () => import('@/views/layouts/MainLayout.vue'),
      meta: { requiresAuth: true },
      children: [
        {
          path: '',
          name: 'Dashboard',
          component: () => import('@/views/DashboardView.vue'),
          meta: { title: 'Dashboard', minRole: 'viewer' },
        },

        // Domains
        {
          path: 'domains',
          name: 'DomainList',
          component: () => import('@/views/domains/DomainList.vue'),
          meta: { title: '域名列表', minRole: 'viewer' },
        },
        {
          path: 'domains/:id',
          name: 'DomainDetail',
          component: () => import('@/views/domains/DomainDetail.vue'),
          meta: { title: '域名詳情', minRole: 'viewer' },
        },

        // Projects
        {
          path: 'projects',
          name: 'ProjectList',
          component: () => import('@/views/projects/ProjectList.vue'),
          meta: { title: '專案管理', minRole: 'viewer' },
        },

        // Releases
        {
          path: 'releases',
          name: 'ReleaseList',
          component: () => import('@/views/releases/ReleaseList.vue'),
          meta: { title: '發布管理', minRole: 'viewer' },
        },

        // Alerts
        {
          path: 'alerts',
          name: 'AlertList',
          component: () => import('@/views/AlertList.vue'),
          meta: { title: '告警記錄', minRole: 'viewer' },
        },

        // Pool
        {
          path: 'pool',
          name: 'PoolList',
          component: () => import('@/views/pool/PoolList.vue'),
          meta: { title: '備用域名池', minRole: 'operator' },
        },

        // Servers
        {
          path: 'servers',
          name: 'ServerList',
          component: () => import('@/views/servers/ServerList.vue'),
          meta: { title: '伺服器管理', minRole: 'admin' },
        },

        // Settings
        {
          path: 'settings/users',
          name: 'UserList',
          component: () => import('@/views/settings/UserList.vue'),
          meta: { title: '使用者管理', minRole: 'admin' },
        },
      ],
    },

    // 404
    {
      path: '/:pathMatch(.*)*',
      name: 'NotFound',
      component: () => import('@/views/NotFound.vue'),
    },
  ],
})

// Navigation guard
router.beforeEach((to) => {
  const auth = useAuthStore()

  if (to.meta.requiresAuth !== false && !auth.isLoggedIn) {
    return { name: 'Login', query: { redirect: to.fullPath } }
  }

  if (to.name === 'Login' && auth.isLoggedIn) {
    return { name: 'Dashboard' }
  }
})

export default router
