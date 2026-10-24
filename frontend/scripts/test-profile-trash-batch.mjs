import { readFileSync } from 'node:fs'
import assert from 'node:assert/strict'
import { test } from 'node:test'
import ts from 'typescript'
const source = readFileSync(new URL('../src/modules/browser/pages/browserList/profileTrashBatch.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2020 } }).outputText
const { runProfileTrashBatch } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
test('batch continues after failure and reports only successful IDs as completed', async () => {
  const calls = []
  const result = await runProfileTrashBatch(['a', 'b', 'c'], async id => {
    calls.push(id)
    if (id === 'b') throw new Error('synthetic failure')
  })
  assert.deepEqual(calls, ['a', 'b', 'c'])
  assert.deepEqual(result, { succeededIds: ['a', 'c'], failedIds: ['b'] })
})
test('each selected instance is attempted once, sequentially, including retry after all failures', async () => {
  let active = 0
  const calls = []
  const result = await runProfileTrashBatch(['a', 'a', 'b'], async id => {
    assert.equal(++active, 1)
    calls.push(id)
    await Promise.resolve()
    active--
    throw new Error('synthetic failure')
  })
  assert.deepEqual(calls, ['a', 'b'])
  assert.deepEqual(result, { succeededIds: [], failedIds: ['a', 'b'] })
})
test('empty selection never calls destructive API', async () => {
  assert.deepEqual(await runProfileTrashBatch([], () => { throw new Error('must not call') }), { succeededIds: [], failedIds: [] })
})
