import type { ReleaseStatus } from './common'

export interface ReleaseResponse {
  uuid:             string
  project_id:       number
  title:            string | null
  status:           ReleaseStatus
  total_domains:    number
  shard_size:       number
  canary_threshold: number
  created_at:       string
  updated_at:       string
}

export interface ShardResponse {
  id:            number
  shard_index:   number
  status:        string
  domain_count:  number
  success_count: number
  fail_count:    number
  started_at:    string | null
  completed_at:  string | null
}
