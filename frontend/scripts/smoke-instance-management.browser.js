// Synthetic-only UI verification. Requires isolated Vite on :5218.
const page = await browser.getPage('main')
await page.goto('http://localhost:5218')
const result = await page.evaluate(async () => {
  const wait = async (condition, message) => {
    for (let i = 0; i < 160; i++) { if (condition()) return; await new Promise(resolve => setTimeout(resolve, 30)) }
    throw new Error(`Timeout: ${message}`)
  }
  const check = (condition, message) => { if (!condition) throw new Error(message) }
  const button = text => [...document.querySelectorAll('button')].find(el => el.textContent.trim() === text)
  const input = (el, value) => {
    Object.getOwnPropertyDescriptor(el.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype, 'value').set.call(el, value)
    el.dispatchEvent(new Event('input', { bubbles: true }))
  }
  const select = (el, value) => { el.value = value; el.dispatchEvent(new Event('change', { bubbles: true })) }
  const sources = [
    { id: 'c1', kind: 'clash', name: '订阅一', value: 'https://first.example.test' },
    { id: 'c2', kind: 'clash', name: '订阅二', value: 'https://second.example.test' },
    { id: 'w1', kind: 'webshare', name: '账号一', value: 'synthetic-key-one' },
    { id: 'w2', kind: 'webshare', name: '账号二', value: 'synthetic-key-two' },
  ]
  let groups = [{ groupId: 'g1', groupName: '父分组', parentId: '', sortOrder: 0, instanceCount: 1 }]
  let profile = { profileId: 'p', profileName: '测试实例', running: false, debugReady: false, proxyId: '__direct__', proxyConfig: 'direct://', fingerprintArgs: [], launchArgs: [], tags: [] }
  let cookies = [{ name: 'session', value: 'synthetic-cookie', domain: '.example.test', path: '/', expires: -1, httpOnly: true, secure: true, sameSite: 'Lax' }]
  const calls = []
  window.go = { main: { App: new Proxy({}, { get: (_, key) => async (...args) => {
    calls.push(key)
    if (key === 'BrowserProxyGetImportSources') return { sources: [...sources], selectedClashId: 'c1', selectedWebshareId: 'w1' }
    if (key === 'BrowserProxySaveImportSource') {
      const [id, kind, name, value] = args
      const record = { id: id || 'new', kind, name, value }
      const i = sources.findIndex(item => item.id === record.id)
      if (i < 0) sources.push(record); else sources[i] = record
      return record
    }
    if (key === 'BrowserProxyDeleteImportSource') { sources.splice(sources.findIndex(item => item.id === args[0]), 1); return }
    if (key === 'ListGroups') return [...groups]
    if (key === 'CreateGroup') { const group = { ...args[0], groupId: 'new-group', instanceCount: 0 }; groups.push(group); return group }
    if (key === 'UpdateGroup') { const i = groups.findIndex(group => group.groupId === args[0]); groups[i] = { ...groups[i], ...args[1] }; return groups[i] }
    if (key === 'DeleteGroup') { groups = groups.filter(group => group.groupId !== args[0]); return }
    if (key === 'BrowserProfileList') return [profile]
    if (key === 'BrowserInstanceStart') { profile = { ...profile, running: true, debugReady: true }; return profile }
    if (key === 'BrowserGetCookies') return cookies
    if (key === 'BrowserImportCookies') { cookies = JSON.parse(args[1]); return cookies.length }
    if (key === 'BrowserClearCookies') { cookies = []; return }
    return []
  } }) } }
  const React = (await import('/node_modules/.vite/deps/react.js')).default
  const DOM = (await import('/node_modules/.vite/deps/react-dom_client.js')).default
  const { useProxyImportFlow } = await import('/src/modules/browser/pages/proxyPool/useProxyImportFlow.ts')
  const { ProxyPoolImportModal } = await import('/src/modules/browser/pages/proxyPool/ProxyPoolImportModal.tsx')
  const { GroupManagerModal } = await import('/src/modules/browser/components/GroupManagerModal.tsx')
  const { ProfileCookiesModal } = await import('/src/modules/browser/components/ProfileCookiesModal.tsx')
  document.getElementById('root')?.remove()
  document.documentElement.setAttribute('data-theme', 'dark')
  const mount = document.createElement('div'); document.body.append(mount)
  const root = DOM.createRoot(mount)
  const noop = () => {}
  function ImportHarness() {
    const flow = useProxyImportFlow({ proxies: [], globalAutoRefreshEnabled: false, globalRefreshInterval: 60, saveProxies: async () => {} })
    return React.createElement(ProxyPoolImportModal, {
      ...flow, open: true, groups: [], fetchProxyOptions: [], onClose: noop,
      onImportModeChange: flow.handleImportModeChange, onImportUrlChange: flow.handleImportUrlChange,
      onWebshareApiKeyChange: flow.setWebshareApiKey, onSelectImportSource: flow.handleSelectImportSource,
      onImportSourceNameChange: flow.setImportSourceName, onSaveImportSource: flow.handleSaveImportSource,
      onForgetImportSource: flow.handleForgetImportSource,
    })
  }
  root.render(React.createElement(ImportHarness))
  await wait(() => document.querySelector('input[placeholder="订阅 URL"]')?.value.includes('first'), 'saved subscription')
  select(document.querySelector('select'), 'c2')
  await wait(() => document.querySelector('input[placeholder="订阅 URL"]').value.includes('second'), 'second subscription')
  ;[...document.querySelectorAll('button')].find(el => el.textContent.startsWith('Webshare API')).click()
  await wait(() => document.querySelector('input[type="password"]')?.value === 'synthetic-key-one', 'first key')
  select(document.querySelector('select'), 'w2')
  await wait(() => document.querySelector('input[type="password"]').value === 'synthetic-key-two', 'second key')
  select(document.querySelector('select'), '')
  await wait(() => document.querySelector('input[type="password"]').value === '', 'new account')
  input(document.querySelector('input[placeholder="例如：业务账号 A"]'), '账号三')
  input(document.querySelector('input[type="password"]'), 'synthetic-key-three')
  button('保存来源').click()
  await wait(() => sources.length === 5, 'source saved')
  await wait(() => button('删除来源') && !button('删除来源').disabled, 'delete ready')
  button('删除来源').click()
  await wait(() => sources.length === 4, 'source removed independently')
  check(sources.some(item => item.id === 'c2') && sources.some(item => item.id === 'w2'), 'other sources retained')

  root.render(React.createElement(GroupManagerModal, { open: true, onClose: noop, onChanged: noop }))
  await wait(() => document.body.textContent.includes('父分组') && button('新建分组') && !button('新建分组').disabled, 'group manager')
  button('新建分组').click()
  await wait(() => button('保存分组'), 'new group form')
  input(document.querySelector('input[maxlength="100"]'), '子分组')
  select(document.querySelector('select'), 'g1')
  button('保存分组').click()
  await wait(() => groups.length === 2 && button('编辑'), 'group saved')
  check(groups[1].parentId === 'g1', 'parent retained')
  ;[...document.querySelectorAll('button')].filter(el => el.textContent === '编辑')[1].click()
  await wait(() => button('保存分组'), 'edit group')
  input(document.querySelector('input[maxlength="100"]'), '子分组改名')
  button('保存分组').click()
  await wait(() => document.body.textContent.includes('子分组改名') && button('删除'), 'group renamed')
  ;[...document.querySelectorAll('button')].filter(el => el.textContent === '删除')[1].click()
  await wait(() => button('删除分组'), 'delete confirmation')
  button('删除分组').click()
  await wait(() => groups.length === 1, 'group deleted')

  root.render(React.createElement(ProfileCookiesModal, { open: true, profileId: 'p', onClose: noop }))
  await wait(() => button('启动实例'), 'stopped cookies entry')
  button('启动实例').click()
  await wait(() => button('导入'), 'running cookies manager')
  await wait(() => document.body.textContent.includes('session'), 'cookie list')
  button('导入').click()
  await wait(() => document.querySelector('textarea'), 'cookie import form')
  input(document.querySelector('textarea'), JSON.stringify([{ name: 'imported', value: 'synthetic', domain: 'example.test', path: '/', expires: -1, httpOnly: false, secure: false }]))
  button('导入 Cookie').click()
  await wait(() => document.body.textContent.includes('imported') && !document.querySelector('textarea'), 'cookie imported and refreshed')
  button('清除全部').click()
  await wait(() => button('确认清除'), 'clear confirmation')
  button('确认清除').click()
  await wait(() => cookies.length === 0 && document.body.textContent.includes('暂无数据'), 'cookies cleared')
  window.__managementSmokeRoot = root
  return { passed: ['multiple subscription selection', 'multiple account selection', 'named source save and independent deletion', 'group create/parent/rename/delete', 'stopped instance launch', 'cookie import/refresh/clear'], calls: calls.filter(key => /ImportSource|Group|Cookies/.test(key)) }
})
console.log(JSON.stringify(result))
await page.shot({ type: 'jpeg', maxEdge: 1200, quality: 75 })
