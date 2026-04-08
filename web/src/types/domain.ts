import type { DomainStatus } from './common'

export interface SubdomainResponse {
  uuid:           string
  prefix:         string
  fqdn:           string
  dns_provider:   string
  cdn_provider:   string
  nginx_template: string
  dns_record_id:  string | null
  cdn_domain_id:  string | null
  ssl_expiry:     string | null
  created_at:     string
  updated_at:     string
}

export interface DomainResponse {
  uuid:        string
  domain:      string
  status:      DomainStatus
  project_id:  number
  conf_path:   string | null
  subdomains?: SubdomainResponse[]
  created_at:  string
  updated_at:  string
}

export interface CreateDomainRequest {
  domain:     string
  project_id: number
  prefixes:   string[]
}

export interface DomainStateHistory {
  id:          number
  from_status: DomainStatus
  to_status:   DomainStatus
  reason:      string | null
  triggered_by: string
  changed_at:  string
}
