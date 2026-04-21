<script setup lang="ts">
import { onMounted, ref, h } from 'vue'
import type { DataTableColumns } from 'naive-ui'
import type { VNodeChild } from 'vue'
import {
  NButton, NModal, NCard, NForm, NFormItem, NInput,
  NSpace, NPopconfirm, NTag, useMessage,
} from 'naive-ui'
import { AppTable, PageHeader } from '@/components'
import { useTagStore } from '@/stores/tag'
import type { TagResponse } from '@/types/tag'

const store   = useTagStore()
const message = useMessage()

// ── Create ────────────────────────────────────────────────────────────────────
const showCreate = ref(false)
const creating   = ref(false)
const createForm = ref({ name: '', color: '#3b82f6' })

async function handleCreate() {
  if (!createForm.value.name.trim()) { message.warning('名稱不可為空'); return }
  creating.value = true
  try {
    await store.create({ name: createForm.value.name.trim(), color: createForm.value.color || null })
    message.success('標籤已建立')
    showCreate.value = false
    createForm.value = { name: '', color: '#3b82f6' }
  } catch (e: any) {
    message.error(e?.response?.data?.message || '建立失敗')
  } finally {
    creating.value = false
  }
}

// ── Edit ──────────────────────────────────────────────────────────────────────
const showEdit = ref(false)
const editing  = ref(false)
const editId   = ref(0)
const editForm = ref({ name: '', color: '' })

function openEdit(row: TagResponse) {
  editId.value = row.id
  editForm.value = { name: row.name, color: row.color ?? '' }
  showEdit.value = true
}

async function handleEdit() {
  if (!editForm.value.name.trim()) { message.warning('名稱不可為空'); return }
  editing.value = true
  try {
    await store.update(editId.value, { name: editForm.value.name.trim(), color: editForm.value.color || null })
    message.success('標籤已更新')
    showEdit.value = false
  } catch (e: any) {
    message.error(e?.response?.data?.message || '更新失敗')
  } finally {
    editing.value = false
  }
}

async function handleDelete(id: number) {
  try {
    await store.deleteTag(id)
    message.success('已刪除')
  } catch (e: any) {
    message.error(e?.response?.data?.message || '刪除失敗')
  }
}

// ── Table ─────────────────────────────────────────────────────────────────────
const columns: DataTableColumns<TagResponse> = [
  { title: '標籤', key: 'name', minWidth: 160,
    render: (row): VNodeChild => h(NTag, {
      bordered: false,
      style: row.color ? `background: ${row.color}20; color: ${row.color}; border: 1px solid ${row.color}40` : undefined,
      size: 'small',
    }, { default: () => row.name }),
  },
  { title: '色碼', key: 'color', width: 100,
    render: (row): VNodeChild => row.color
      ? h('span', { style: 'display:flex; align-items:center; gap:6px' }, [
          h('span', { style: `width:14px; height:14px; border-radius:3px; background:${row.color}; display:inline-block` }),
          row.color,
        ])
      : '-',
  },
  { title: '域名數', key: 'domain_count', width: 90 },
  {
    title: '操作', key: 'actions', width: 120, fixed: 'right',
    render: (row): VNodeChild => h(NSpace, {}, {
      default: () => [
        h(NButton, { size: 'small', quaternary: true, type: 'primary', onClick: () => openEdit(row) }, { default: () => '編輯' }),
        h(NPopconfirm, { onPositiveClick: () => handleDelete(row.id) }, {
          trigger: () => h(NButton, { size: 'small', quaternary: true, type: 'error' }, { default: () => '刪除' }),
          default: () => `確認刪除「${row.name}」？`,
        }),
      ],
    }),
  },
]

onMounted(() => store.fetchList())
</script>

<template>
  <div class="list-page">
    <PageHeader title="標籤管理" subtitle="管理域名分類標籤">
      <template #actions>
        <NButton type="primary" @click="showCreate = true">新增標籤</NButton>
      </template>
    </PageHeader>

    <AppTable
      :columns="columns"
      :data="store.tags"
      :loading="store.loading"
      :row-key="(r: TagResponse) => r.id"
    />

    <!-- Create -->
    <NModal v-model:show="showCreate" :mask-closable="!creating">
      <NCard title="新增標籤" :bordered="false" style="width: 420px">
        <NForm label-placement="left" label-width="80px">
          <NFormItem label="名稱" required>
            <NInput v-model:value="createForm.name" placeholder="例：production" />
          </NFormItem>
          <NFormItem label="顏色">
            <NInput v-model:value="createForm.color" placeholder="#RRGGBB" />
          </NFormItem>
        </NForm>
        <template #action>
          <div style="display:flex; justify-content:flex-end; gap:8px">
            <NButton :disabled="creating" @click="showCreate = false">取消</NButton>
            <NButton type="primary" :loading="creating" @click="handleCreate">建立</NButton>
          </div>
        </template>
      </NCard>
    </NModal>

    <!-- Edit -->
    <NModal v-model:show="showEdit" :mask-closable="!editing">
      <NCard title="編輯標籤" :bordered="false" style="width: 420px">
        <NForm label-placement="left" label-width="80px">
          <NFormItem label="名稱" required>
            <NInput v-model:value="editForm.name" />
          </NFormItem>
          <NFormItem label="顏色">
            <NInput v-model:value="editForm.color" placeholder="#RRGGBB" />
          </NFormItem>
        </NForm>
        <template #action>
          <div style="display:flex; justify-content:flex-end; gap:8px">
            <NButton :disabled="editing" @click="showEdit = false">取消</NButton>
            <NButton type="primary" :loading="editing" @click="handleEdit">儲存</NButton>
          </div>
        </template>
      </NCard>
    </NModal>
  </div>
</template>

<style scoped>
.list-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}
</style>
