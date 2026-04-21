export type CostType = 'registration' | 'renewal' | 'transfer' | 'privacy' | 'other'

export interface FeeScheduleResponse {
  id:               number
  registrar_id:     number
  tld:              string
  registration_fee: number
  renewal_fee:      number
  transfer_fee:     number
  privacy_fee:      number
  currency:         string
  created_at:       string
  updated_at:       string
}

export interface CreateFeeScheduleRequest {
  registrar_id:      number
  tld:               string
  registration_fee?: number
  renewal_fee?:      number
  transfer_fee?:     number
  privacy_fee?:      number
  currency:          string
}

export interface UpdateFeeScheduleRequest {
  registration_fee: number
  renewal_fee:      number
  transfer_fee:     number
  privacy_fee:      number
  currency:         string
}

export interface DomainCostResponse {
  id:           number
  domain_id:    number
  cost_type:    CostType
  amount:       number
  currency:     string
  period_start: string | null
  period_end:   string | null
  paid_at:      string | null
  notes:        string | null
  created_at:   string
}

export interface CreateCostRequest {
  cost_type:     CostType
  amount:        number
  currency:      string
  period_start?: string | null
  period_end?:   string | null
  paid_at?:      string | null
  notes?:        string | null
}

export interface CostSummaryItem {
  group_key:  string
  total_cost: number
  currency:   string
  count:      number
}
