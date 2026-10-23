import type { CookieInfo } from '../types'
import { getBindings, getGoApp } from './runtime'

export async function fetchBrowserCookies(profileId: string): Promise<CookieInfo[]> {
  const bindings: any = (await getBindings()) || getGoApp()
  if (bindings?.BrowserGetCookies) {
    return (await bindings.BrowserGetCookies(profileId)) || []
  }
  throw new Error('当前环境未连接 Cookie 管理服务')
}

export async function clearBrowserCookies(profileId: string): Promise<boolean> {
  const bindings: any = (await getBindings()) || getGoApp()
  if (bindings?.BrowserClearCookies) {
    await bindings.BrowserClearCookies(profileId)
    return true
  }
  throw new Error('当前环境未连接 Cookie 管理服务')
}

export async function exportBrowserCookies(profileId: string): Promise<string> {
  const bindings: any = (await getBindings()) || getGoApp()
  if (bindings?.BrowserExportCookies) {
    return (await bindings.BrowserExportCookies(profileId)) || ''
  }
  throw new Error('当前环境未连接 Cookie 管理服务')
}

export async function importBrowserCookies(profileId: string, content: string): Promise<number> {
  const bindings: any = (await getBindings()) || getGoApp()
  if (!bindings?.BrowserImportCookies) throw new Error('当前环境不支持 Cookie 导入')
  return await bindings.BrowserImportCookies(profileId, content)
}
