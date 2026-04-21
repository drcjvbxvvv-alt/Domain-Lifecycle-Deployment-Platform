import { defineStore } from 'pinia'
import { ref } from 'vue'
import { costApi } from '@/api/cost'
import type {
  FeeScheduleResponse, CreateFeeScheduleRequest, UpdateFeeScheduleRequest,
  DomainCostResponse, CreateCostRequest, CostSummaryItem,
} from '@/types/cost'
import type { ApiResponse, PaginatedData } from '@/types/common'

type SummaryResponse = { items: CostSummaryItem[]; total: number; group_by: string }

export const useCostStore = defineStore('cost', () => {
  const feeSchedules = ref<FeeScheduleResponse[]>([])
  const domainCosts  = ref<DomainCostResponse[]>([])
  const summary      = ref<CostSummaryItem[]>([])
  const loading      = ref(false)

  // ── Fee Schedules ──────────────────────────────────────────────────────────

  async function fetchFeeSchedules(registrarId?: number) {
    loading.value = true
    try {
      const res = await costApi.listFeeSchedules(registrarId) as unknown as ApiResponse<PaginatedData<FeeScheduleResponse>>
      feeSchedules.value = res.data?.items ?? []
    } finally {
      loading.value = false
    }
  }

  async function createFeeSchedule(data: CreateFeeScheduleRequest) {
    const res = await costApi.createFeeSchedule(data) as unknown as ApiResponse<FeeScheduleResponse>
    feeSchedules.value = [res.data, ...feeSchedules.value]
    return res.data
  }

  async function updateFeeSchedule(id: number, data: UpdateFeeScheduleRequest) {
    const res = await costApi.updateFeeSchedule(id, data) as unknown as ApiResponse<FeeScheduleResponse>
    const idx = feeSchedules.value.findIndex(f => f.id === id)
    if (idx >= 0) feeSchedules.value[idx] = res.data
    return res.data
  }

  async function deleteFeeSchedule(id: number) {
    await costApi.deleteFeeSchedule(id)
    feeSchedules.value = feeSchedules.value.filter(f => f.id !== id)
  }

  // ── Domain Costs ───────────────────────────────────────────────────────────

  async function fetchDomainCosts(domainId: number) {
    loading.value = true
    try {
      const res = await costApi.listDomainCosts(domainId) as unknown as ApiResponse<PaginatedData<DomainCostResponse>>
      domainCosts.value = res.data?.items ?? []
    } finally {
      loading.value = false
    }
  }

  async function createDomainCost(domainId: number, data: CreateCostRequest) {
    const res = await costApi.createDomainCost(domainId, data) as unknown as ApiResponse<DomainCostResponse>
    domainCosts.value = [res.data, ...domainCosts.value]
    return res.data
  }

  // ── Summary ────────────────────────────────────────────────────────────────

  async function fetchSummary(groupBy: 'registrar' | 'tld' = 'registrar') {
    const res = await costApi.getCostSummary(groupBy) as unknown as ApiResponse<SummaryResponse>
    summary.value = res.data?.items ?? []
    return res.data
  }

  return {
    feeSchedules, domainCosts, summary, loading,
    fetchFeeSchedules, createFeeSchedule, updateFeeSchedule, deleteFeeSchedule,
    fetchDomainCosts, createDomainCost,
    fetchSummary,
  }
})
