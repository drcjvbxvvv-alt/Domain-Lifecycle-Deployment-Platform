import { defineStore } from 'pinia'
import { ref } from 'vue'
import { hostGroupApi } from '@/api/hostGroup'
import type { HostGroupResponse, UpdateConcurrencyRequest } from '@/types/hostGroup'
import type { ApiResponse } from '@/types/common'

export const useHostGroupStore = defineStore('hostGroup', () => {
  const hostGroups = ref<HostGroupResponse[]>([])
  const loading    = ref(false)

  async function fetchList() {
    loading.value = true
    try {
      const res = await hostGroupApi.list() as unknown as ApiResponse<HostGroupResponse[]>
      hostGroups.value = res.data ?? []
    } finally {
      loading.value = false
    }
  }

  async function updateConcurrency(id: number, data: UpdateConcurrencyRequest) {
    const res = await hostGroupApi.updateConcurrency(id, data) as unknown as ApiResponse<HostGroupResponse>
    const updated = res.data
    if (updated) {
      const idx = hostGroups.value.findIndex(h => h.id === id)
      if (idx !== -1) hostGroups.value[idx] = updated
    }
    return updated
  }

  return { hostGroups, loading, fetchList, updateConcurrency }
})
