import { http } from '@/utils/http'
import type { ApiResponse } from '@/types/common'
import type { DNSLookupResult } from '@/types/dns'

export const dnsApi = {
  /** Look up DNS records for a domain in the database by its ID. */
  lookupByDomain(domainId: number): Promise<ApiResponse<DNSLookupResult>> {
    return http.get(`/domains/${domainId}/dns-records`)
  },

  /** Look up DNS records for any arbitrary FQDN. */
  lookupByFQDN(fqdn: string): Promise<ApiResponse<DNSLookupResult>> {
    return http.get('/dns/lookup', { params: { fqdn } })
  },
}
