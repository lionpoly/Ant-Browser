import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

const source = readFileSync(new URL('../src/modules/browser/pages/proxyPool/helpers.webshare.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS } }).outputText
const module = { exports: {} }
new Function('exports', 'module', compiled)(module.exports, module)
const { buildWebshareImportCandidates } = module.exports

test('Webshare records become authenticated HTTP proxy candidates', () => {
  const [candidate] = buildWebshareImportCandidates([{
    id: 'd-1', username: 'user@example.com', password: 'p@ss', proxy_address: '1.2.3.4', port: 8168,
    valid: true, country_code: 'US', city_name: 'New York',
  }])
  assert.equal(candidate.proxyName, 'Webshare US / New York - 1.2.3.4:8168')
  assert.equal(candidate.proxyConfig, 'http://user%40example.com:p%40ss@1.2.3.4:8168')
})

test('invalid or out of range Webshare records are excluded', () => {
  assert.deepEqual(buildWebshareImportCandidates([
    { proxy_address: '', port: 1, valid: true },
    { proxy_address: '1.2.3.4', port: 0, valid: true },
    { proxy_address: '1.2.3.4', port: 90000, valid: true },
    { proxy_address: '1.2.3.4', port: 80, valid: false },
  ]), [])
})
