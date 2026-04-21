export interface TagResponse {
  id:            number
  name:          string
  color:         string | null
  domain_count?: number
}

export interface CreateTagRequest {
  name:   string
  color?: string | null
}

export interface UpdateTagRequest {
  name:   string
  color?: string | null
}

export type BulkAction = 'update' | 'add_tags' | 'remove_tags'

export interface BulkActionRequest {
  domain_ids:            number[]
  action:                BulkAction
  tag_ids?:              number[]
  registrar_account_id?: number | null
  dns_provider_id?:      number | null
  auto_renew?:           boolean | null
}
