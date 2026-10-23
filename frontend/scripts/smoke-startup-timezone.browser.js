const page = await browser.getPage('main')
await page.goto('http://127.0.0.1:5221')
const result = await page.evaluate(async () => {
 const React = (await import('/node_modules/.vite/deps/react.js')).default
 const DOM = (await import('/node_modules/.vite/deps/react-dom_client.js')).default
 const { BrowserListSettingsModal } = await import('/src/modules/browser/components/BrowserListLayout.tsx')
 const { useBrowserListSettings } = await import('/src/modules/browser/pages/browserList/useBrowserListSettings.ts')
 const { createDefaultBrowserSettings } = await import('/src/modules/browser/api/runtime.ts')
 let saved = createDefaultBrowserSettings()
 window.go = { main: { App: {
  GetBrowserSettings: async () => structuredClone(saved),
  SaveBrowserSettings: async value => { saved = structuredClone(value) },
  BrowserCoreList: async () => [],
 } } }
 const host = document.createElement('div');document.body.replaceChildren(host)
 const root = DOM.createRoot(host)
 function Harness() {
  const state = useBrowserListSettings()
  return React.createElement(React.Fragment, null,
   React.createElement('button', {onClick:state.handleOpenSettings}, '打开测试配置'),
   React.createElement(BrowserListSettingsModal, {
    open:state.settingsModalOpen, settings:state.settings, cores:[], savingSettings:state.savingSettings,
    fingerprintText:state.fingerprintText,launchText:state.launchText,startUrlsText:state.startUrlsText,
    onSettingsChange: patch => state.setSettings(previous=>({...previous,...patch})),
    onClose:()=>state.setSettingsModalOpen(false),onSave:state.handleSaveSettings,
    onFingerprintTextChange:state.setFingerprintText,onLaunchTextChange:state.setLaunchText,onStartUrlsTextChange:state.setStartUrlsText,
   }))
 }
 root.render(React.createElement(Harness))
 const wait = async fn => {for(let i=0;i<100;i++){if(fn())return;await new Promise(r=>setTimeout(r,30))}throw Error('UI timeout')}
 const open=()=>[...document.querySelectorAll('button')].find(e=>e.textContent==='打开测试配置')
 const control=()=>document.querySelector('[role="switch"][aria-label="启动前检查并更新时区"]')
 const save=()=>[...document.querySelectorAll('button')].find(e=>e.textContent==='保存')
 await wait(open);open().click();await wait(control)
 if(control().getAttribute('aria-checked')!=='false')throw Error('not default off')
 control().click();await wait(()=>control().getAttribute('aria-checked')==='true')
 save().click();await wait(()=>!control())
 if(!saved.autoTimezoneOnStart)throw Error('not saved')
 open().click();await wait(control)
 if(control().getAttribute('aria-checked')!=='true')throw Error('not restored')
 document.documentElement.setAttribute('data-theme','dark')
 if(!document.body.textContent.includes('检查或保存失败会阻止启动'))throw Error('missing failure explanation')
 return {defaultOff:true,toggleAndSave:true,reopenRestores:true,failureHint:true}
})
console.log(JSON.stringify(result))
await page.shot({type:'jpeg',maxEdge:960,quality:70})
