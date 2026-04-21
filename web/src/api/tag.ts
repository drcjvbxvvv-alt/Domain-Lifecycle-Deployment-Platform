import { http } from '@/utils/http'
import type { TagResponse, CreateTagRequest, UpdateTagRequest, BulkActionRequest } from '@/types/tag'
import type { PaginatedData } from '@/types/common'

export const tagApi = {
  // Tag CRUD
  list: () =>
    http.get<PaginatedData<TagResponse>>('/tags'),

  create: (data: CreateTagRequest) =>
    http.post<TagResponse>('/tags', data),

  update: (id: number, data: UpdateTagRequest) =>
    http.put<TagResponse>(`/tags/${id}`, data),

  delete: (id: number) =>
    http.delete(`/tags/${id}`),

  // Domain tags
  getDomainTags: (domainId: number) =>
    http.get<TagResponse[]>(`/domains/${domainId}/tags`),

  setDomainTags: (domainId: number, tagIds: number[]) =>
    http.put(`/domains/${domainId}/tags`, { tag_ids: tagIds }),

  // Bulk operations
  bulk: (data: BulkActionRequest) =>
    http.post<{ affected: number }>('/domains/bulk', data),

  // CSV export — returns a blob URL
  exportUrl: (params?: Record<string, string>) => {
    const base = '/api/v1/domains/export'
    const qs = params ? '?' + new URLSearchParams(params).toString() : ''
    return base + qs
  },
}
