import { readFileSync } from 'node:fs'
import assert from 'node:assert/strict'
import { test } from 'node:test'
import ts from 'typescript'
const src=readFileSync(new URL('../src/modules/browser/utils/fingerprintSerializer.ts',import.meta.url),'utf8')
const compiled=ts.transpileModule(src,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2020}}).outputText
const {serialize,deserialize,validateFingerprintArgs}=await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
test('CloakBrowser native fields roundtrip without old-kernel filtering',()=>{
 const raw=['--fingerprint=123','--fingerprint-locale=ja-JP','--fingerprint-timezone=Asia/Tokyo','--fingerprint-device-memory=8','--fingerprint-screen-width=1440','--fingerprint-screen-height=900','--fingerprint-gpu-vendor=Intel','--fingerprint-noise=false','--fingerprint-storage-quota=5000','--fingerprint-portable-cookies','--custom-test=keep']
 const result=serialize(deserialize(raw),'cloakbrowser')
 for(const arg of raw) assert.ok(result.includes(arg),arg)
 assert.ok(!result.some(a=>a.startsWith('--timezone=')))
})
test('CloakBrowser translates common locale fields and keeps native validation',()=>{
 const args=serialize({seed:'1',lang:'en-US',timezone:'Europe/London',deviceMemory:'8',noise:'false'},'cloakbrowser')
 assert.ok(args.includes('--fingerprint-locale=en-US'))
 assert.ok(args.includes('--fingerprint-timezone=Europe/London'))
 assert.ok(args.includes('--fingerprint-device-memory=8'))
 assert.equal(validateFingerprintArgs(['--fingerprint=off'],'cloakbrowser').valid,true)
 assert.equal(validateFingerprintArgs(['--fingerprint-device-memory=oops'],'cloakbrowser').valid,false)
})
test('fingerprint-chromium legacy dialect remains default',()=>{
 assert.ok(serialize({timezone:'Asia/Tokyo'}).includes('--timezone=Asia/Tokyo'))
 assert.ok(!serialize({deviceMemory:'8'}).some(a=>a.startsWith('--fingerprint-device-memory=')))
})
test('CloakBrowser locale updates retain native hardware and extension flags', async()=>{
 const {applyLocaleToFingerprintArgs}=await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
 const args=applyLocaleToFingerprintArgs(['--fingerprint-screen-width=1440','--fingerprint-portable-cookies'],'ja-JP','Asia/Tokyo','cloakbrowser')
 assert.ok(args.includes('--fingerprint-screen-width=1440'))
 assert.ok(args.includes('--fingerprint-portable-cookies'))
 assert.ok(args.includes('--fingerprint-locale=ja-JP'))
 assert.equal(validateFingerprintArgs(['--fingerprint-screen-width=1.5'],'cloakbrowser').valid,false)
})
test('legacy profiles preserve raw CloakBrowser-only switches on save',()=>{
 const raw=['--fingerprint-gpu-vendor=Intel','--fingerprint-screen-width=1440','--fingerprint-storage-quota=1000','--fingerprint-noise=false','--fingerprint-webrtc-ip=auto']
 const saved=serialize(deserialize(raw))
 for(const arg of raw) assert.ok(saved.includes(arg),arg)
})
