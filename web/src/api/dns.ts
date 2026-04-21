import { http } from '@/utils/http'
import type { ApiResponse } from '@/types/common'
import type { DNSLookupResult, PropagationResult } from '@/types/dns'

export const dnsApi = {
  /** Look up DNS records for a domain in the database by its ID. */
  lookupByDomain(domainId: number): Promise<ApiResponse<DNSLookupResult>> {
    return http.get(`/domains/${domainId}/dns-records`)
  },

  /** Look up DNS records for any arbitrary FQDN. */
  lookupByFQDN(fqdn: string): Promise<ApiResponse<DNSLookupResult>> {
    return http.get('/dns/lookup', { params: { fqdn } })
  },

  /** Check DNS propagation for a domain across multiple resolvers. */
  propagationByDomain(domainId: number, types?: string): Promise<ApiResponse<PropagationResult>> {
    return http.get(`/domains/${domainId}/dns-propagation`, { params: types ? { types } : {} })
  },

  /** Check DNS propagation for any arbitrary FQDN. */
  propagationByFQDN(fqdn: string, types?: string): Promise<ApiResponse<PropagationResult>> {
    return http.get('/dns/propagation', { params: { fqdn, ...(types ? { types } : {}) } })
  },
}
