import type { BrowserCore, BrowserCoreExtended, BrowserCoreInput, BrowserCoreValidateResult } from '../types'
import { getBindings, getMockCores, setMockCores } from './runtime'

export interface BrowserCloakBrowserVersionResult {
  version: string
  platform: string
  downloadUrl: string
  message: string
}

export async function saveCloakBrowserLicenseKey(licenseKey: string): Promise<void> {
  const bindings: any = await getBindings()
  if (!bindings?.BrowserCloakBrowserLicenseSave) {
    throw new Error('当前应用版本不支持保存 CloakBrowser 启动授权')
  }
  await bindings.BrowserCloakBrowserLicenseSave(licenseKey)
}

export async function fetchCloakBrowserVersion(licenseKey: string, proxyConfig = ''): Promise<BrowserCloakBrowserVersionResult> {
  const bindings: any = await getBindings()
  if (!bindings?.BrowserCloakBrowserVersion) {
    throw new Error('当前应用版本不支持 CloakBrowser Key 查询')
  }
  const result = await bindings.BrowserCloakBrowserVersion({ licenseKey, proxyConfig })
  if (!result?.version || !result?.downloadUrl) {
    throw new Error(result?.message || 'CloakBrowser 没有返回可用版本')
  }
  return result
}

export async function fetchBrowserCores(): Promise<BrowserCore[]> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserCoreList) {
    return (await bindings.BrowserCoreList()) || []
  }
  return getMockCores()
}

export async function saveBrowserCore(input: BrowserCoreInput): Promise<boolean> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserCoreSave) {
    await bindings.BrowserCoreSave(input)
    return true
  }

  const nextCores = [...getMockCores()]
  const index = nextCores.findIndex((core) => core.coreId === input.coreId)
  if (index >= 0) {
    nextCores[index] = input
  } else {
    nextCores.push({ ...input, coreId: input.coreId || `core-${Date.now()}` })
  }
  setMockCores(nextCores)
  return true
}

export async function deleteBrowserCore(coreId: string): Promise<boolean> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserCoreDelete) {
    await bindings.BrowserCoreDelete(coreId)
    return true
  }
  setMockCores(getMockCores().filter((core) => core.coreId !== coreId))
  return true
}

export async function setDefaultBrowserCore(coreId: string): Promise<boolean> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserCoreSetDefault) {
    await bindings.BrowserCoreSetDefault(coreId)
    return true
  }
  setMockCores(getMockCores().map((core) => ({ ...core, isDefault: core.coreId === coreId })))
  return true
}

export async function validateBrowserCorePath(corePath: string): Promise<BrowserCoreValidateResult> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserCoreValidate) {
    return (await bindings.BrowserCoreValidate(corePath)) || { valid: false, message: '验证失败' }
  }
  return { valid: true, message: '路径有效（模拟）' }
}

export async function fetchCoreExtendedInfo(): Promise<BrowserCoreExtended[]> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserCoreExtendedInfo) {
    return (await bindings.BrowserCoreExtendedInfo()) || []
  }
  return []
}

export async function scanBrowserCores(): Promise<BrowserCore[]> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserCoreScan) {
    return (await bindings.BrowserCoreScan()) || []
  }
  return getMockCores()
}

export async function importLocalBrowserCore(): Promise<BrowserCore | null> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserCoreImportLocal) {
    return (await bindings.BrowserCoreImportLocal()) || null
  }
  return null
}

export async function BrowserCoreDownload(coreName: string, url: string, proxyConfig?: string, licenseKey?: string): Promise<boolean> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserCoreDownload) {
    await bindings.BrowserCoreDownload(coreName, url, proxyConfig || '', licenseKey || '')
    return true
  }
  return true
}

export async function redownloadBrowserCore(coreId: string, url: string, proxyConfig?: string, licenseKey?: string): Promise<boolean> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserCoreRedownload) {
    await bindings.BrowserCoreRedownload(coreId, url, proxyConfig || '', licenseKey || '')
    return true
  }
  return true
}

export async function openCorePath(corePath: string): Promise<boolean> {
  const bindings: any = await getBindings()
  if (bindings?.OpenCorePath) {
    await bindings.OpenCorePath(corePath)
    return true
  }
  return false
}
