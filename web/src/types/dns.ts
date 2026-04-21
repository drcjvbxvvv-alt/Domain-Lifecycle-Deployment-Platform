// Types for live DNS record lookup — mirrors Go's dnsquery.LookupResult.

export type DNSRecordType = 'A' | 'AAAA' | 'CNAME' | 'MX' | 'TXT' | 'NS'

export interface DNSRecord {
  type: DNSRecordType
  name: string
  value: string
  priority?: number  // MX only
  ttl?: number
}

export interface DNSLookupResult {
  fqdn: string
  records: DNSRecord[]
  queried_at: string
  error?: string
}
