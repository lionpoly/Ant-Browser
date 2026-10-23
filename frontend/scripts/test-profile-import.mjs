import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'

// Execute the page's actual async handlers with controlled API promises.
const source = readFileSync(new URL('../src/modules/browser/pages/BrowserListPage.tsx', import.meta.url), 'utf8')
const ast = ts.createSourceFile('BrowserListPage.tsx', source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)
const handlers = new Map()
function visit(node) {
  if (ts.isVariableDeclaration(node) && ['handleImportProfiles', 'executeProfileImport'].includes(node.name.getText(ast))) {
    handlers.set(node.name.getText(ast), node.initializer.getText(ast))
  }
  ts.forEachChild(node, visit)
}
visit(ast)
const compiled = ts.transpileModule(
  [...handlers].map(([name, body]) => `const ${name} = ${body}`).join('\n')
    + '\n;({ handleImportProfiles, executeProfileImport })',
  { compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS } },
).outputText

const preview = { zipPath: '/synthetic/import.xlsx', profiles: [] }
function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
function setup(prepare, restore = async () => ({ importedCount: 2 })) {
  const state = { busy: false, preview: null, errors: [], refreshes: 0 }
  const context = {
    get profilePackageBusy() { return state.busy },
    get profileImportPreview() { return state.preview },
    setProfilePackageBusy: value => { state.busy = value },
    setProfileImportPreview: value => { state.preview = value },
    prepareBrowserProfilePackageImport: prepare,
    importBrowserProfilePackageWithOptions: restore,
    setSelectedIds: () => {},
    loadProfiles: async () => { state.refreshes++ },
    toast: { error: message => state.errors.push(message), success() {}, warning() {} },
  }
  return { state, ...runInNewContext(compiled, context) }
}

test('preview completion unlocks confirmation; restore locks only while executing', async () => {
  const preparation = deferred()
  const restoration = deferred()
  const calls = []
  const flow = setup(() => preparation.promise, (...args) => { calls.push(args); return restoration.promise })
  const pending = flow.handleImportProfiles()
  assert.equal(flow.state.busy, true)
  preparation.resolve(preview)
  await pending
  assert.equal(flow.state.preview, preview)
  assert.equal(flow.state.busy, false, 'confirmation, cancel and close must be usable')
  const actions = [{ sourceIndex: 0, mode: 'new' }]
  const importing = flow.executeProfileImport(preview.zipPath, actions, true)
  assert.equal(flow.state.busy, true, 'actual restore must prevent concurrent operations')
  assert.equal(flow.state.preview, null)
  assert.equal(calls[0][0], preview.zipPath)
  assert.equal(calls[0][2], true)
  assert.equal(calls[0][3], actions)
  restoration.resolve({ importedCount: 2 })
  await importing
  assert.equal(flow.state.busy, false)
  assert.equal(flow.state.refreshes, 1)
})

test('preparation cancellation and failure release busy state', async () => {
  for (const prepare of [async () => ({ cancelled: true }), async () => { throw new Error('prepare failed') }]) {
    const flow = setup(prepare)
    await flow.handleImportProfiles()
    assert.equal(flow.state.busy, false)
    assert.equal(flow.state.preview, null)
  }
})

test('restore failure and cancellation release busy state', async () => {
  for (const restore of [async () => ({ cancelled: true }), async () => { throw new Error('restore failed') }]) {
    const flow = setup(async () => preview, restore)
    await flow.handleImportProfiles()
    await flow.executeProfileImport(preview.zipPath, [], true)
    assert.equal(flow.state.busy, false)
    assert.equal(flow.state.preview, null)
    assert.equal(flow.state.refreshes, 0)
  }
})
