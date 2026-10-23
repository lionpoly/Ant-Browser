import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { test } from 'node:test'
import ts from 'typescript'
import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

const require = createRequire(import.meta.url)
const source = readFileSync(new URL('../src/modules/browser/pages/proxyPool/ProxyPoolImportModal.tsx', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX } }).outputText
const module = { exports: {} }
const box = ({ children }) => React.createElement('div', null, children)
new Function('require', 'exports', 'module', compiled)(name => {
  if (name.endsWith('/components')) return { Button: box, FormItem: box, Input: box, Modal: box, Select: box, Textarea: box }
  if (name === './helpers') return {}
  return require(name)
}, module.exports, module)
const html = renderToStaticMarkup(React.createElement(module.exports.ProxyPoolImportModal, {
  open: true, importMode: 'clash', importUrl: '', importResolvedUrl: '', groups: [], fetchProxyOptions: [],
}))
const activeClass = html.match(/<button[^>]*class="([^"]*shadow-sm[^"]*)"/)[1]
const backgroundToken = activeClass.match(/bg-\[var\((--[\w-]+)\)\]/)[1]
const textToken = activeClass.match(/text-\[var\((--[\w-]+)\)\]/)?.[1]
const luminance = hex => {
  const rgb = hex.match(/[\da-f]{2}/gi).map(x => parseInt(x, 16) / 255).map(x => x <= 0.04045 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4)
  return rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722
}
for (const theme of ['light', 'dark']) {
  test(`selected import tab has readable text in ${theme} theme`, () => {
    const css = readFileSync(new URL(`../src/shared/theme/themes/${theme}.css`, import.meta.url), 'utf8')
    const color = token => css.match(new RegExp(`${token}:\\s*(#[\\da-f]+)`))[1]
    const bg = luminance(color(backgroundToken))
    const fg = luminance(textToken ? color(textToken) : '#ffffff')
    const contrast = (Math.max(bg, fg) + 0.05) / (Math.min(bg, fg) + 0.05)
    assert.ok(contrast >= 4.5, `contrast ${contrast.toFixed(2)} is below 4.5`)
  })
}
