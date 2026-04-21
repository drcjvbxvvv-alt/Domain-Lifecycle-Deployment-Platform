import { defineStore } from 'pinia'
import { ref } from 'vue'
import { tagApi } from '@/api/tag'
import type { TagResponse, CreateTagRequest, UpdateTagRequest, BulkActionRequest } from '@/types/tag'
import type { ApiResponse, PaginatedData } from '@/types/common'

export const useTagStore = defineStore('tag', () => {
  const tags        = ref<TagResponse[]>([])
  const domainTags  = ref<TagResponse[]>([])
  const loading     = ref(false)

  async function fetchList() {
    loading.value = true
    try {
      const res = await tagApi.list() as unknown as ApiResponse<PaginatedData<TagResponse>>
      tags.value = res.data?.items ?? []
    } finally {
      loading.value = false
    }
  }

  async function create(data: CreateTagRequest) {
    const res = await tagApi.create(data) as unknown as ApiResponse<TagResponse>
    tags.value = [...tags.value, res.data]
    return res.data
  }

  async function update(id: number, data: UpdateTagRequest) {
    const res = await tagApi.update(id, data) as unknown as ApiResponse<TagResponse>
    const idx = tags.value.findIndex(t => t.id === id)
    if (idx >= 0) tags.value[idx] = res.data
    return res.data
  }

  async function deleteTag(id: number) {
    await tagApi.delete(id)
    tags.value = tags.value.filter(t => t.id !== id)
  }

  // Domain tags
  async function fetchDomainTags(domainId: number) {
    const res = await tagApi.getDomainTags(domainId) as unknown as ApiResponse<TagResponse[]>
    domainTags.value = res.data ?? []
  }

  async function setDomainTags(domainId: number, tagIds: number[]) {
    await tagApi.setDomainTags(domainId, tagIds)
    await fetchDomainTags(domainId)
  }

  // Bulk
  async function bulk(data: BulkActionRequest) {
    const res = await tagApi.bulk(data) as unknown as ApiResponse<{ affected: number }>
    return res.data?.affected ?? 0
  }

  return {
    tags, domainTags, loading,
    fetchList, create, update, deleteTag,
    fetchDomainTags, setDomainTags, bulk,
  }
})
