import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

const source = readFileSync(new URL('../src/modules/browser/pages/proxyPool/useProxyImportFlow.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS } }).outputText
const savedSources = {
  sources: [
    { id: 'c1', kind: 'clash', name: '订阅一', value: 'https://saved.test' },
    { id: 'w1', kind: 'webshare', name: '账号一', value: 'saved-key' },
    { id: 'w2', kind: 'webshare', name: '账号二', value: 'other-key' },
  ], selectedClashId: 'c1', selectedWebshareId: 'w1',
}
function setup(load = async () => savedSources) {
  const slots = [], effects = [], saves = [], errors = []
  let index = 0
  const react = {
    useState(initial) {
      const i = index++
      if (!(i in slots)) slots[i] = typeof initial === 'function' ? initial() : initial
      return [slots[i], value => { slots[i] = typeof value === 'function' ? value(slots[i]) : value }]
    },
    useRef(initial) { const [value] = react.useState({ current: initial }); return value },
    useEffect(fn) { const i = index++; if (!(i in slots)) { slots[i] = true; effects.push(fn) } },
  }
  const helpers = { INITIAL_CHAIN_IMPORT_FORM: { first: { server: '', port: '' }, second: { server: '', port: '' } }, INITIAL_DIRECT_IMPORT_FORM: { server: '', port: '' },
    buildImportPreview: x => x, nextProxyID: () => 'new', resolveImportSourceID: () => 'source' }
  const api = { getProxyImportSources: load,
    saveProxyImportSource: async (id, kind, name, value) => { saves.push([id, kind, name, value]); return { id: id || 'new-source', kind, name, value } },
    deleteProxyImportSource: async id => saves.push(['delete', id]),
    fetchWebshareProxies: async () => ({ results: [{}] }), fetchClashImportFromURL: async () => ({ content: 'proxies: []' }) }
  const module = { exports: {} }
  new Function('require', 'exports', 'module', compiled)(name => {
    if (name === 'react') return react
    if (name.endsWith('/api')) return api
    if (name.endsWith('/components')) return { toast: { error: x => errors.push(x), success() {} } }
    if (name === './helpers') return helpers
    if (name === './helpers.webshare') return { buildWebshareImportCandidates: () => [{ proxyName: 'test', proxyConfig: 'http://example.test:80' }] }
    return {}
  }, module.exports, module)
  const render = () => { index = 0; return module.exports.useProxyImportFlow({ proxies: [], saveProxies: async () => {}, globalAutoRefreshEnabled: false, globalRefreshInterval: 60 }) }
  render(); effects.forEach(fn => fn())
  return { render, saves, api, errors }
}
const flush = () => new Promise(resolve => setImmediate(resolve))

test('remembers both sources across tab switches and successful imports', async () => {
  const app = setup(); await flush()
  let flow = app.render()
  assert.equal(flow.importUrl, 'https://saved.test')
  assert.equal(flow.webshareApiKey, 'saved-key')
  flow.handleImportModeChange('webshare')
  flow = app.render()
  assert.equal(flow.importUrl, 'https://saved.test')
  await flow.handleFetchWebshare()
  flow = app.render()
  await flow.handleConfirmImport()
  flow = app.render()
  assert.equal(flow.webshareApiKey, 'saved-key')
  assert.equal(flow.importUrl, 'https://saved.test')
  assert.deepEqual(app.saves, [['w1', 'webshare', '账号一', 'saved-key']])
  flow.handleImportModeChange('clash'); flow = app.render()
  await flow.handleFetchImportURL()
  assert.deepEqual(app.saves.at(-1), ['c1', 'clash', '订阅一', 'https://saved.test'])
})

test('late restore never overwrites edits; forgetting preserves the other source', async () => {
  let finish
  const app = setup(() => new Promise(resolve => { finish = resolve }))
  app.render().setWebshareApiKey('edited-key')
  finish(savedSources); await flush()
  let flow = app.render()
  assert.equal(flow.webshareApiKey, 'edited-key')
  flow.handleImportModeChange('webshare'); flow = app.render()
  flow.handleSelectImportSource('w1'); flow = app.render()
  await flow.handleForgetImportSource(); flow = app.render()
  assert.equal(flow.webshareApiKey, '')
  assert.equal(flow.importUrl, 'https://saved.test')
  assert.deepEqual(app.saves, [['delete', 'w1']])
})

test('save failures are visible and do not block a successful fetch', async () => {
  const app = setup(); await flush()
  app.api.saveProxyImportSource = async () => { throw new Error('disk unavailable') }
  await app.render().handleFetchWebshare()
  assert.equal(app.render().previewModalOpen, true)
  assert.ok(app.errors.some(x => x.includes('保存')))
  await app.render().handleForgetImportSource()
  assert.equal(app.render().importUrl, 'https://saved.test')
})

test('switches accounts independently and saves a new named source', async () => {
  const app = setup(); await flush()
  let flow = app.render()
  flow.handleImportModeChange('webshare'); flow = app.render()
  flow.handleSelectImportSource('w2'); flow = app.render()
  assert.equal(flow.webshareApiKey, 'other-key')
  assert.equal(flow.importSourceName, '账号二')
  flow.handleSelectImportSource(''); flow = app.render()
  assert.equal(flow.webshareApiKey, '')
  flow.setImportSourceName('新账号'); flow.setWebshareApiKey('new-key'); flow = app.render()
  await flow.handleSaveImportSource(); flow = app.render()
  assert.equal(flow.importSources.length, 4)
  assert.equal(flow.selectedImportSourceId, 'new-source')
  assert.deepEqual(app.saves.at(-1), ['', 'webshare', '新账号', 'new-key'])
  assert.equal(flow.importUrl, 'https://saved.test')
})
