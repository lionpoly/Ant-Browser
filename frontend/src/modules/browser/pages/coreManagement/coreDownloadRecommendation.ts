import { fetchCloakBrowserVersion } from '../../api/cores'

export const FINGERPRINT_CHROMIUM_RELEASES_URL = 'https://github.com/adryfish/fingerprint-chromium/releases'

export type CoreDownloadSource = 'fingerprint-chromium' | 'cloakbrowser'

export const CORE_DOWNLOAD_SOURCES: Record<CoreDownloadSource, { label: string; releasesUrl: string }> = {
  'fingerprint-chromium': { label: 'fingerprint-chromium', releasesUrl: FINGERPRINT_CHROMIUM_RELEASES_URL },
  cloakbrowser: { label: 'CloakBrowser', releasesUrl: 'https://github.com/CloakHQ/cloakbrowser/releases' },
}

const FINGERPRINT_CHROMIUM_LATEST_RELEASE_API_URL = 'https://api.github.com/repos/adryfish/fingerprint-chromium/releases/latest'

type CoreDownloadPlatform = 'windows' | 'linux' | 'darwin' | ''
type CoreDownloadArch = 'amd64' | 'arm64' | ''

export interface CoreDownloadRuntimeEnvironment {
  platform?: string
  arch?: string
}

export interface CoreDownloadTarget {
  platform: CoreDownloadPlatform
  arch: CoreDownloadArch
  label: string
}

export interface CoreDownloadRecommendation {
  target: CoreDownloadTarget
  releaseTag: string
  publishedAt: string
  assetName: string
  downloadUrl: string
  releasesUrl: string
  namePlaceholder: string
}

interface GitHubReleaseAsset {
  name?: string
  browser_download_url?: string
}

interface GitHubReleaseResponse {
  published_at?: string
  tag_name?: string
  draft?: boolean
  prerelease?: boolean
  html_url?: string
  assets?: GitHubReleaseAsset[]
}

const archiveSuffixes = ['.zip', '.tar.xz', '.txz', '.tar.gz', '.tgz', '.tar.bz2', '.tbz2', '.tar']

function normalizePlatform(value: string | undefined): CoreDownloadPlatform {
  const text = (value || '').trim().toLowerCase()
  if (text.includes('darwin') || text.includes('mac')) return 'darwin'
  if (text.includes('win')) return 'windows'
  if (text.includes('linux')) return 'linux'
  return ''
}

function normalizeArch(value: string | undefined): CoreDownloadArch {
  const text = (value || '').trim().toLowerCase()
  if (text === 'amd64' || text === 'x64' || text === 'x86_64') return 'amd64'
  if (text === 'arm64' || text === 'aarch64') return 'arm64'
  if (hasAny(text, ['arm64', 'aarch64'])) return 'arm64'
  if (hasAny(text, ['amd64', 'x64', 'x86_64', 'win64', 'wow64'])) return 'amd64'
  return ''
}

function inferNavigatorEnvironment(): CoreDownloadRuntimeEnvironment {
  if (typeof navigator === 'undefined') return {}
  const nav = navigator as Navigator & { userAgentData?: { platform?: string; architecture?: string } }
  const platform = nav.userAgentData?.platform || navigator.platform || ''
  const userAgent = navigator.userAgent || ''
  const archSource = `${nav.userAgentData?.architecture || ''} ${navigator.platform || ''} ${userAgent}`
  return { platform, arch: archSource }
}

export function resolveCoreDownloadTarget(env: CoreDownloadRuntimeEnvironment | null | undefined): CoreDownloadTarget {
  const fallback = inferNavigatorEnvironment()
  const platform = normalizePlatform(env?.platform || fallback.platform)
  const arch = normalizeArch(env?.arch || fallback.arch)
  return { platform, arch, label: formatTargetLabel(platform, arch) }
}

function formatTargetLabel(platform: CoreDownloadPlatform, arch: CoreDownloadArch): string {
  const platformLabel = platform === 'windows' ? 'Windows' : platform === 'linux' ? 'Linux' : platform === 'darwin' ? 'macOS' : '未知系统'
  const archLabel = arch || '未知架构'
  return `${platformLabel} / ${archLabel}`
}

function hasSupportedArchiveSuffix(name: string): boolean {
  const lowerName = name.toLowerCase()
  return archiveSuffixes.some(suffix => lowerName.endsWith(suffix))
}

function hasAny(text: string, keywords: string[]): boolean {
  return keywords.some(keyword => text.includes(keyword))
}

function matchesArch(name: string, arch: CoreDownloadArch): boolean {
  if (arch === 'amd64') return hasAny(name, ['amd64', 'x64', 'x86_64'])
  if (arch === 'arm64') return hasAny(name, ['arm64', 'aarch64'])
  return false
}

function scoreAsset(asset: GitHubReleaseAsset, target: CoreDownloadTarget): number {
  const name = (asset.name || '').toLowerCase()
  if (!name || !asset.browser_download_url || !hasSupportedArchiveSuffix(name)) return -1
  if (!target.platform || !target.arch) return -1

  let score = 0
  if (target.platform === 'windows') {
    if (!name.includes('windows') || !matchesArch(name, target.arch)) return -1
    score += 100
    if (name.endsWith('.zip')) score += 20
  } else if (target.platform === 'linux') {
    if (!name.includes('linux') || !matchesArch(name, target.arch)) return -1
    score += 100
    if (name.endsWith('.tar.xz')) score += 20
  } else if (target.platform === 'darwin') {
    if (!hasAny(name, ['macos', 'darwin'])) return -1
    if (hasAny(name, ['amd64', 'x64', 'x86_64', 'arm64', 'aarch64']) && !matchesArch(name, target.arch)) return -1
    score += 100
  }

  return score
}

function pickReleaseAsset(assets: GitHubReleaseAsset[], target: CoreDownloadTarget): GitHubReleaseAsset | null {
  return assets
    .map(asset => ({ asset, score: scoreAsset(asset, target) }))
    .filter(item => item.score >= 0)
    .sort((left, right) => right.score - left.score || (left.asset.name || '').localeCompare(right.asset.name || ''))[0]?.asset || null
}

async function fetchReleaseData(url: string, signal?: AbortSignal): Promise<unknown> {
  const response = await fetch(url, {
    headers: { Accept: 'application/vnd.github+json' },
    signal,
  })
  if (!response.ok) throw new Error(`GitHub Releases 请求失败: ${response.status}`)
  return response.json()
}

async function findCloakBrowserRelease(target: CoreDownloadTarget, signal?: AbortSignal): Promise<GitHubReleaseResponse | null> {
  if (!target.platform || !target.arch || (target.platform === 'windows' && target.arch !== 'amd64')) return null
  const arch = target.arch === 'amd64' ? 'x64' : 'arm64'
  const suffix = target.platform === 'windows' ? '.zip' : '.tar.gz'
  const assetName = `cloakbrowser-${target.platform}-${arch}${suffix}`
  // SDK and Pro releases may have no public binary; each platform has its own latest build.
  for (let page = 1; page <= 3; page++) {
    const releases = await fetchReleaseData(`https://api.github.com/repos/CloakHQ/cloakbrowser/releases?per_page=100&page=${page}`, signal) as GitHubReleaseResponse[]
    for (const release of releases) {
      if (release.draft || release.prerelease || !/^chromium-v\d+(?:\.\d+){3,4}$/.test(release.tag_name || '')) continue
      const asset = release.assets?.find(item => item.name === assetName && item.browser_download_url)
      if (asset) return { ...release, assets: [asset] }
    }
    if (releases.length < 100) break
  }
  return null
}

async function findCloakBrowserKeyedRelease(target: CoreDownloadTarget, licenseKey: string, proxyConfig = '', signal?: AbortSignal): Promise<CoreDownloadRecommendation | null> {
  if (!target.platform || !target.arch || (target.platform === 'windows' && target.arch !== 'amd64')) return null
  if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')
  const result = await fetchCloakBrowserVersion(licenseKey, proxyConfig)
  const version = String(result.version || '').trim()
  if (!/^\d+(?:\.\d+){3,4}$/.test(version)) return null
  const platform = `${target.platform}-${target.arch === 'amd64' ? 'x64' : 'arm64'}`
  const archive = `cloakbrowser-${platform}${target.platform === 'windows' ? '.zip' : '.tar.gz'}`
  return {
    target,
    releaseTag: `chromium-v${version}`,
    publishedAt: '',
    assetName: archive,
    downloadUrl: result.downloadUrl,
    releasesUrl: CORE_DOWNLOAD_SOURCES.cloakbrowser.releasesUrl,
    namePlaceholder: `例如: cloakbrowser-${version.split('.')[0]}`,
  }
}

export async function fetchCoreDownloadRecommendation(
  env: CoreDownloadRuntimeEnvironment | null | undefined,
  signal?: AbortSignal,
  source: CoreDownloadSource = 'fingerprint-chromium',
  licenseKey = '',
  proxyConfig = '',
): Promise<CoreDownloadRecommendation | null> {
  const target = resolveCoreDownloadTarget(env)
  if (source === 'cloakbrowser' && licenseKey.trim()) {
    return findCloakBrowserKeyedRelease(target, licenseKey.trim(), proxyConfig, signal)
  }
  const release = source === 'cloakbrowser'
    ? await findCloakBrowserRelease(target, signal)
    : await fetchReleaseData(FINGERPRINT_CHROMIUM_LATEST_RELEASE_API_URL, signal) as GitHubReleaseResponse
  if (!release) return null
  const asset = pickReleaseAsset(release.assets || [], target)
  if (!asset?.name || !asset.browser_download_url) return null

  const releaseTag = (release.tag_name || '').trim()
  const majorVersion = releaseTag.replace(/^(chromium-)?v/, '').split('.')[0] || ''
  const namePrefix = source === 'cloakbrowser' ? 'cloakbrowser' : 'chrome'
  return {
    target,
    releaseTag,
    publishedAt: release.published_at || '',
    assetName: asset.name,
    downloadUrl: asset.browser_download_url,
    releasesUrl: release.html_url || CORE_DOWNLOAD_SOURCES[source].releasesUrl,
    namePlaceholder: `例如: ${namePrefix}-${majorVersion || 'latest'}`,
  }
}
