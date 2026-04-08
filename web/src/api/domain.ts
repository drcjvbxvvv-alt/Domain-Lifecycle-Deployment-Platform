import { http } from '@/utils/http'
import type { DomainResponse, CreateDomainRequest, DomainStateHistory } from '@/types/domain'
import type { PaginatedData } from '@/types/common'

export const domainApi = {
  list: (params: { project_id?: number; status?: string; cursor?: string; limit?: number }) =>
    http.get<PaginatedData<DomainResponse>>('/domains', { params }),

  get: (id: string) =>
    http.get<DomainResponse>(`/domains/${id}`),

  create: (data: CreateDomainRequest) =>
    http.post<DomainResponse>('/domains', data),

  delete: (id: string) =>
    http.delete(`/domains/${id}`),

  deploy: (id: string) =>
    http.post(`/domains/${id}/deploy`),

  history: (id: string) =>
    http.get<DomainStateHistory[]>(`/domains/${id}/history`),
}
