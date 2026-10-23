import type { WebshareProxyRecord } from '../../api/proxies'
import type { ImportCandidate } from './helpers.types'

export function buildWebshareImportCandidates(records: WebshareProxyRecord[]): ImportCandidate[] {
  return records
    .filter(item => item.valid !== false && item.proxy_address?.trim() && Number(item.port) > 0 && Number(item.port) <= 65535)
    .map((item, index) => {
      const address = item.proxy_address.trim()
      const port = Number(item.port)
      const location = [item.country_code, item.city_name].filter(Boolean).join(' / ')
      const label = location ? `Webshare ${location} - ${address}:${port}` : `Webshare ${address}:${port}`
      const auth = item.username
        ? `${encodeURIComponent(item.username)}${item.password ? `:${encodeURIComponent(item.password)}` : ''}@`
        : ''
      return {
        proxyName: label || `Webshare 代理 ${index + 1}`,
        proxyConfig: `http://${auth}${address}:${port}`,
      }
    })
}
