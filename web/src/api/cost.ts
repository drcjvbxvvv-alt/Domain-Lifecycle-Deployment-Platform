import { http } from '@/utils/http'
import type {
  FeeScheduleResponse, CreateFeeScheduleRequest, UpdateFeeScheduleRequest,
  DomainCostResponse, CreateCostRequest, CostSummaryItem,
} from '@/types/cost'
import type { PaginatedData } from '@/types/common'

type SummaryResponse = { items: CostSummaryItem[]; total: number; group_by: string }

export const costApi = {
  // Fee schedules
  listFeeSchedules: (registrarId?: number) =>
    http.get<PaginatedData<FeeScheduleResponse>>('/fee-schedules', {
      params: registrarId ? { registrar_id: registrarId } : {},
    }),

  createFeeSchedule: (data: CreateFeeScheduleRequest) =>
    http.post<FeeScheduleResponse>('/fee-schedules', data),

  updateFeeSchedule: (id: number, data: UpdateFeeScheduleRequest) =>
    http.put<FeeScheduleResponse>(`/fee-schedules/${id}`, data),

  deleteFeeSchedule: (id: number) =>
    http.delete(`/fee-schedules/${id}`),

  // Domain cost records
  listDomainCosts: (domainId: number) =>
    http.get<PaginatedData<DomainCostResponse>>(`/domains/${domainId}/costs`),

  createDomainCost: (domainId: number, data: CreateCostRequest) =>
    http.post<DomainCostResponse>(`/domains/${domainId}/costs`, data),

  // Aggregate summary
  getCostSummary: (groupBy: 'registrar' | 'tld' = 'registrar') =>
    http.get<SummaryResponse>('/costs/summary', { params: { group_by: groupBy } }),
}
