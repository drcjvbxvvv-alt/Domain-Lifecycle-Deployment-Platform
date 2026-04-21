import { http } from '@/utils/http'
import type { ExpiryDashboardData } from '@/types/expiry'

export const expiryApi = {
  dashboard: () =>
    http.get<ExpiryDashboardData>('/dashboard/expiry'),
}
