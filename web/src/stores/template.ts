import { defineStore } from 'pinia'
import { ref } from 'vue'
import { templateApi } from '@/api/template'
import type { TemplateResponse, TemplateVersionResponse } from '@/types/template'
import type { ApiResponse, PaginatedData } from '@/types/common'

export const useTemplateStore = defineStore('template', () => {
  const templates = ref<TemplateResponse[]>([])
  const total     = ref(0)
  const loading   = ref(false)
  const current   = ref<TemplateResponse | null>(null)
  const versions  = ref<TemplateVersionResponse[]>([])

  async function fetchByProject(projectId: number | string) {
    loading.value = true
    try {
      const res = await templateApi.listByProject(projectId) as unknown as ApiResponse<PaginatedData<TemplateResponse>>
      templates.value = res.data?.items ?? []
      total.value     = res.data?.total ?? 0
    } finally {
      loading.value = false
    }
  }

  async function fetchOne(id: number | string) {
    loading.value = true
    try {
      const res = await templateApi.get(id) as unknown as ApiResponse<TemplateResponse>
      current.value = res.data
    } finally {
      loading.value = false
    }
  }

  async function fetchVersions(id: number | string) {
    const res = await templateApi.listVersions(id) as unknown as ApiResponse<{ items: TemplateVersionResponse[] }>
    versions.value = res.data?.items ?? []
  }

  return { templates, total, loading, current, versions, fetchByProject, fetchOne, fetchVersions }
})
