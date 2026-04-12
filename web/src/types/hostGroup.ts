export interface HostGroupResponse {
  id:                    number
  uuid:                  string
  project_id:            number
  name:                  string
  description?:          string
  region?:               string
  max_concurrency:       number  // 0 = unlimited
  reload_batch_size:     number
  reload_batch_wait_secs: number
  created_at:            string
  updated_at:            string
}

export interface UpdateConcurrencyRequest {
  max_concurrency:       number
  reload_batch_size:     number
  reload_batch_wait_secs: number
}
