import { http } from '@/utils/http'
import type { ProjectResponse, CreateProjectRequest, PrefixRuleResponse } from '@/types/project'
import type { PaginatedData } from '@/types/common'

export const projectApi = {
  list: () =>
    http.get<PaginatedData<ProjectResponse>>('/projects'),

  get: (id: string) =>
    http.get<ProjectResponse>(`/projects/${id}`),

  create: (data: CreateProjectRequest) =>
    http.post<ProjectResponse>('/projects', data),

  update: (id: string, data: Partial<CreateProjectRequest>) =>
    http.put<ProjectResponse>(`/projects/${id}`, data),

  delete: (id: string) =>
    http.delete(`/projects/${id}`),

  listPrefixRules: (id: string) =>
    http.get<PrefixRuleResponse[]>(`/projects/${id}/prefix-rules`),
}
