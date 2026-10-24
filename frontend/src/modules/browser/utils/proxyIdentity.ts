import { parseProxyInfo } from '../pages/proxyPool/helpers.clash'

/** Stable dedupe key for imports: normalized endpoint host/IP plus port. */
export function proxyEndpointKey(proxyConfig: string): string {
  const info = parseProxyInfo(proxyConfig)
  const server = info.server.trim().toLowerCase()
  const port = Number(info.port || 0)
  if (!server || server === '-' || !Number.isInteger(port) || port <= 0) return ''
  return `${server}:${port}`
}

export function mergeImportedProxies(existing: import('../types').BrowserProxy[], incoming: import('../types').BrowserProxy[]) {
  const result = [...existing]
  const indexByEndpoint = new Map<string, number>()
  result.forEach((proxy, index) => {
    const key = proxyEndpointKey(proxy.proxyConfig)
    if (key && !indexByEndpoint.has(key)) indexByEndpoint.set(key, index)
  })
  incoming.forEach(proxy => {
    const key = proxyEndpointKey(proxy.proxyConfig)
    const index = key ? indexByEndpoint.get(key) : undefined
    if (index === undefined) {
      indexByEndpoint.set(key || `__no-key-${result.length}`, result.length)
      result.push(proxy)
    } else {
      result[index] = { ...result[index], ...proxy, proxyId: result[index].proxyId }
    }
  })
  return result
}
