import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test, afterEach } from 'node:test'
import ts from 'typescript'

const source = readFileSync(new URL('../src/modules/browser/pages/coreManagement/coreDownloadRecommendation.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2020 } }).outputText
const { fetchCoreDownloadRecommendation, resolveCoreDownloadTarget } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
const originalFetch = globalThis.fetch
afterEach(() => { globalThis.fetch = originalFetch })
const officialFixture = JSON.parse(readFileSync(new URL('./fixtures/cloakbrowser-public-releases.json', import.meta.url), 'utf8'))
const asset = name => ({ name, browser_download_url: `https://github.com/CloakHQ/cloakbrowser/releases/download/chromium-v146.0.0.1/${name}` })
const release = (tag, names, extra = {}) => ({ tag_name: tag, assets: names.map(asset), ...extra })
const mock = body => { globalThis.fetch = async () => ({ ok: true, json: async () => body }) }

test('Go runtime platforms resolve accurately, including darwin', () => {
  assert.equal(resolveCoreDownloadTarget({ platform: 'darwin', arch: 'arm64' }).platform, 'darwin')
})

test('unsupported runtime architectures stay unresolved', () => {
  assert.equal(resolveCoreDownloadTarget({ platform: 'linux', arch: '386' }).arch, '')
  assert.equal(resolveCoreDownloadTarget({ platform: 'darwin', arch: 'arm' }).arch, '')
})

test('default source retains fingerprint-chromium latest recommendation', async () => {
  let requested
  globalThis.fetch = async url => { requested = url; return { ok: true, json: async () => release('148.0.1', ['chromium-windows-x64.zip']) } }
  const result = await fetchCoreDownloadRecommendation({ platform: 'windows', arch: 'amd64' })
  assert.match(requested, /adryfish\/fingerprint-chromium\/releases\/latest$/)
  assert.equal(result.assetName, 'chromium-windows-x64.zip')
  assert.equal(result.namePlaceholder, '例如: chrome-148')
})

for (const [platform, arch, filename] of [
  ['windows', 'amd64', 'cloakbrowser-windows-x64.zip'],
  ['linux', 'amd64', 'cloakbrowser-linux-x64.tar.gz'],
  ['linux', 'arm64', 'cloakbrowser-linux-arm64.tar.gz'],
  ['darwin', 'amd64', 'cloakbrowser-darwin-x64.tar.gz'],
  ['darwin', 'arm64', 'cloakbrowser-darwin-arm64.tar.gz'],
]) {
  test(`CloakBrowser recommends public ${platform}/${arch} binary beyond SDK and Pro releases`, async () => {
    mock([
      release('v0.6.3', [filename]),
      release('chromium-v155.0.0.1-pro', ['SHA256SUMS']),
      release('chromium-v148.0.0.1', [filename], { prerelease: true }),
      release('chromium-v147.0.0.1', [filename], { draft: true }),
      release('chromium-v146.0.0.1', [filename]),
    ])
    const result = await fetchCoreDownloadRecommendation({ platform, arch }, undefined, 'cloakbrowser')
    assert.equal(result?.assetName, filename)
    assert.equal(result.releaseTag, 'chromium-v146.0.0.1')
    assert.equal(result.namePlaceholder, '例如: cloakbrowser-146')
    assert.match(result.releasesUrl, /CloakHQ\/cloakbrowser\/releases/)
  })
}

test('CloakBrowser never substitutes a different architecture or platform', async () => {
  mock([release('chromium-v146.0.0.1', ['cloakbrowser-linux-x64.tar.gz', 'cloakbrowser-windows-x64.zip'])])
  assert.equal(await fetchCoreDownloadRecommendation({ platform: 'linux', arch: 'arm64' }, undefined, 'cloakbrowser'), null)
  assert.equal(await fetchCoreDownloadRecommendation({ platform: 'windows', arch: 'arm64' }, undefined, 'cloakbrowser'), null)
})

test('CloakBrowser matches captured official release metadata', async () => {
  mock(officialFixture)
  const result = await fetchCoreDownloadRecommendation({ platform: 'darwin', arch: 'arm64' }, undefined, 'cloakbrowser')
  assert.equal(result?.releaseTag, 'chromium-v145.0.7632.109.2')
  assert.equal(result?.assetName, 'cloakbrowser-darwin-arm64.tar.gz')
  assert.match(result?.downloadUrl || '', /CloakBrowser\/releases\/download\/chromium-v145\.0\.7632\.109\.2/)
})

test('CloakBrowser follows release pages until a matching public archive is found', async () => {
  const urls = []
  globalThis.fetch = async url => {
    urls.push(url)
    return { ok: true, json: async () => urls.length === 1
      ? Array.from({ length: 100 }, () => release('chromium-v155.0.0.1-pro', []))
      : [release('chromium-v145.0.0.1', ['cloakbrowser-darwin-arm64.tar.gz'])] }
  }
  const result = await fetchCoreDownloadRecommendation({ platform: 'darwin', arch: 'arm64' }, undefined, 'cloakbrowser')
  assert.equal(result?.assetName, 'cloakbrowser-darwin-arm64.tar.gz')
  assert.equal(urls.length, 2)
  assert.match(urls[1], /page=2/)
})

test('HTTP failures and aborts propagate instead of recommending another source', async () => {
  globalThis.fetch = async () => ({ ok: false, status: 403 })
  await assert.rejects(fetchCoreDownloadRecommendation({ platform: 'linux', arch: 'amd64' }, undefined, 'cloakbrowser'), /403/)
  const controller = new AbortController()
  controller.abort()
  globalThis.fetch = async (_url, options) => { options.signal.throwIfAborted() }
  await assert.rejects(fetchCoreDownloadRecommendation({ platform: 'linux', arch: 'amd64' }, controller.signal, 'cloakbrowser'), { name: 'AbortError' })
})
