import { defineStore } from 'pinia'
import { ref } from 'vue'
import { releaseApi } from '@/api/release'
import type { ReleaseResponse, ReleaseShardResponse, ReleaseStateHistoryEntry } from '@/types/release'
import type { ApiResponse, PaginatedData } from '@/types/common'

export const useReleaseStore = defineStore('release', () => {
  const releases = ref<ReleaseResponse[]>([])
  const total    = ref(0)
  const loading  = ref(false)
  const current  = ref<ReleaseResponse | null>(null)
  const shards   = ref<ReleaseShardResponse[]>([])
  const history  = ref<ReleaseStateHistoryEntry[]>([])

  async function fetchByProject(projectId: number, params?: { cursor?: string }) {
    loading.value = true
    try {
      const res = await releaseApi.list({ project_id: projectId, ...params }) as unknown as ApiResponse<PaginatedData<ReleaseResponse>>
      releases.value = res.data?.items ?? []
      total.value    = res.data?.total ?? 0
    } finally {
      loading.value = false
    }
  }

  async function fetchOne(id: string) {
    loading.value = true
    try {
      const res = await releaseApi.get(id) as unknown as ApiResponse<ReleaseResponse>
      current.value = res.data
    } finally {
      loading.value = false
    }
  }

  async function fetchShards(id: string) {
    const res = await releaseApi.shards(id) as unknown as ApiResponse<ReleaseShardResponse[]>
    shards.value = res.data ?? []
  }

  async function fetchHistory(id: string) {
    const res = await releaseApi.history(id) as unknown as ApiResponse<{ items: ReleaseStateHistoryEntry[] }>
    history.value = res.data?.items ?? []
  }

  return { releases, total, loading, current, shards, history, fetchByProject, fetchOne, fetchShards, fetchHistory }
})
