import { http } from '@/utils/http'
import type { AlertResponse } from '@/types/alert'
import type { PaginatedData } from '@/types/common'

export const alertApi = {
  list: (params: { severity?: string; domain?: string; cursor?: string }) =>
    http.get<PaginatedData<AlertResponse>>('/alerts', { params }),

  ack: (id: string) =>
    http.post(`/alerts/${id}/ack`),
}
