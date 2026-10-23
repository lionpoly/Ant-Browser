// Run with Vite at :5218 and bb browser-automation:
// bb browser-automation run SESSION --script-file ABSOLUTE_PATH --script-host HOST --json
// All Wails calls and release requests below are synthetic; no binary is downloaded.
const page = await browser.getPage('main')
await page.goto('http://localhost:5218')
const result = await page.evaluate(async () => {
  const check = (condition, message) => { if (!condition) throw new Error(message) }
  const wait = async (condition, message) => {
    for (let i = 0; i < 200; i++) {
      if (condition()) return
      await new Promise(resolve => setTimeout(resolve, 10))
    }
    throw new Error(`Timed out: ${message}`)
  }
  const calls = []
  window.runtime = {
    EventsOnMultiple: () => () => {}, EventsOff: () => {},
    Environment: async () => ({ platform: 'darwin', arch: 'arm64' }),
  }
  const core = { coreId: 'fixture', coreName: 'cloakbrowser-fixture', corePath: '/synthetic/Chromium.app', isDefault: false }
  window.go = { main: { App: new Proxy({}, { get: (_, key) => async (...args) => {
    if (key === 'GetBrowserSettings') return {
      userDataRoot: 'data', defaultFingerprintArgs: [], defaultLaunchArgs: [], defaultStartUrls: [],
      lightStartEnabled: true, restoreLastSession: false, startReadyTimeoutMs: 3000,
      startStableWindowMs: 1200, defaultConnectorType: 'xray',
    }
    if (key === 'BrowserCoreList') return [core]
    if (key === 'BrowserCoreValidate') return { valid: true, message: 'synthetic' }
    if (key === 'BrowserCoreDownload' || key === 'BrowserCoreRedownload') calls.push({ key, args })
    return []
  } }) } }
  const React = await import('/node_modules/.vite/deps/react.js')
  const DOM = await import('/node_modules/.vite/deps/react-dom_client.js')
  const { CoreManagementPage } = await import('/src/modules/browser/pages/CoreManagementPage.tsx')
  document.getElementById('root')?.remove()
  const mount = document.createElement('div')
  document.body.append(mount)
  const root = DOM.default.createRoot(mount)
  const pending = []
  const realFetch = window.fetch
  window.fetch = (url, options) => {
    if (!String(url).startsWith('https://api.github.com/')) return realFetch(url, options)
    return new Promise(resolve => pending.push({ url: String(url), signal: options.signal, resolve }))
  }
  const settle = (request, cloak = false) => {
    const asset = cloak ? 'cloakbrowser-darwin-arm64.tar.gz' : 'chromium-macos-arm64.zip'
    const publisher = cloak ? 'CloakHQ/cloakbrowser' : 'adryfish/fingerprint-chromium'
    const release = {
      tag_name: 'chromium-v145.0.7632.109.2', published_at: '2026-03-04T00:09:02Z',
      assets: [{ name: asset, browser_download_url: `https://github.com/${publisher}/releases/download/chromium-v145.0.7632.109.2/${asset}` }],
    }
    request.resolve({ ok: true, json: async () => cloak ? [release] : release })
  }
  const button = text => [...document.querySelectorAll('button')].find(el => el.textContent.trim() === text)
  const source = () => document.querySelector('select[aria-label="内核来源"]')
  const url = () => [...document.querySelectorAll('input')].find(el => el.placeholder.startsWith('https://') || el.placeholder === '正在匹配当前环境...')
  const input = (element, value) => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(element, value)
    element.dispatchEvent(new Event('input', { bubbles: true }))
  }
  try {
    root.render(React.default.createElement(CoreManagementPage))
    await wait(() => button('重新下载'), 'core list')
    button('下载内核').click()
    await wait(() => pending.length === 1, 'first request')
    // Resolve during the source-change event; promise continuations run after
    // the handler, potentially before passive effect cleanup.
    const old = pending[0]
    const onChange = () => settle(old)
    document.addEventListener('change', onChange, { capture: true, once: true })
    source().value = 'cloakbrowser'
    source().dispatchEvent(new Event('change', { bubbles: true }))
    await wait(() => pending.length === 2, 'new source request')
    check(old.signal.aborted, 'old request must abort')
    check(url().value === '', 'stale source must not populate URL')
    settle(pending[1], true)
    await wait(() => url().value.includes('CloakHQ'), 'CloakBrowser autofill')
    button('取消').click()
    await wait(() => !source(), 'modal closed')

    button('重新下载').click()
    await wait(() => pending.length === 3, 'redownload request')
    settle(pending[2])
    await wait(() => button('使用推荐'), 'redownload recommendation')
    check(url().value === '', 'redownload must require explicit URL choice')
    button('使用推荐').click()
    await wait(() => url().value.includes('fingerprint-chromium'), 'explicit recommendation')
    button('取消').click()
    await wait(() => !source(), 'redownload closed')

    button('下载内核').click()
    await wait(() => pending.length === 4, 'cancel pending request')
    button('取消').click()
    await wait(() => !source(), 'pending modal closed')
    check(pending[3].signal.aborted, 'closing modal must abort pending request')
    settle(pending[3])
    button('下载内核').click()
    await wait(() => pending.length === 5, 'manual download request')
    check(url().value === '', 'closed request must not populate reopened modal')
    input(document.querySelector('input[placeholder^="例如:"]'), 'manual-core')
    input(url(), 'https://example.com/manual.zip')
    await wait(() => url().value === 'https://example.com/manual.zip', 'manual URL')
    button('开始下载').click()
    await wait(() => calls.length === 1, 'download invoked')
    settle(pending[4])
    await wait(() => button('使用推荐'), 'late result')
    check(url().value === 'https://example.com/manual.zip', 'late result must preserve download URL')
    check(calls[0].args[1] === 'https://example.com/manual.zip', 'backend receives manual URL')
    check(source().disabled, 'source locked during download')
    return { passed: ['source switch boundary', 'abort', 'matching autofill', 'redownload explicit choice', 'close and reopen with pending request', 'manual URL and download lock'] }
  } finally {
    root.unmount()
    window.fetch = realFetch
    mount.remove()
  }
})
console.log(JSON.stringify(result))
