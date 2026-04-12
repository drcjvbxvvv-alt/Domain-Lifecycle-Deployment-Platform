import { http } from '@/utils/http'
import type { HostGroupResponse, UpdateConcurrencyRequest } from '@/types/hostGroup'

export const hostGroupApi = {
  list: () =>
    http.get<HostGroupResponse[]>('/host-groups'),

  get: (id: number) =>
    http.get<HostGroupResponse>(`/host-groups/${id}`),

  updateConcurrency: (id: number, data: UpdateConcurrencyRequest) =>
    http.put<HostGroupResponse>(`/host-groups/${id}`, data),
}
