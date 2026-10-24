import { useEffect, useRef, useState } from 'react'
import { toast } from '../../../../shared/components'
import type { BrowserProxy } from '../../types'
import { fetchClashImportFromURL, fetchWebshareProxies, getProxyImportSources, saveProxyImportSource, deleteProxyImportSource, type ProxyImportSource } from '../../api'
import {
  CHAIN_QUICK_IMPORT_TEMPLATE,
  DIRECT_QUICK_IMPORT_TEMPLATE,
  INITIAL_CHAIN_IMPORT_FORM,
  INITIAL_DIRECT_IMPORT_FORM,
  buildChainImportCandidate,
  buildDirectImportCandidate,
  buildDirectImportCandidatesFromText,
  buildImportCandidatesFromClash,
  buildImportPreview,
  createExistingProxyPicker,
  nextProxyID,
  parseChainImportJSON,
  parseClashImportText,
  parseDirectImportText,
  resolveImportSourceID,
  type ChainImportForm,
  type DirectImportForm,
  type ProxyDisplayInfo,
  type ProxyImportMode,
} from './helpers'
import { appendSourceIgnoredProxyNames } from './storage'
import { buildWebshareImportCandidates } from './helpers.webshare'
import { mergeImportedProxies } from '../../utils/proxyIdentity'

interface UseProxyImportFlowOptions {
  proxies: BrowserProxy[]
  globalAutoRefreshEnabled: boolean
  globalRefreshInterval: number
  saveProxies: (list: BrowserProxy[]) => Promise<void>
}

function createInitialChainImportForm(): ChainImportForm {
  return {
    ...INITIAL_CHAIN_IMPORT_FORM,
    first: { ...INITIAL_CHAIN_IMPORT_FORM.first },
    second: { ...INITIAL_CHAIN_IMPORT_FORM.second },
  }
}

export function useProxyImportFlow({
  proxies,
  globalAutoRefreshEnabled,
  globalRefreshInterval,
  saveProxies,
}: UseProxyImportFlowOptions) {
  const [importModalOpen, setImportModalOpen] = useState(false)
  const [importMode, setImportMode] = useState<ProxyImportMode>('clash')
  const [importUrl, setImportUrl] = useState('')
  const [importFetchProxyId, setImportFetchProxyId] = useState('')
  const [importResolvedUrl, setImportResolvedUrl] = useState('')
  const [importText, setImportText] = useState('')
  const [importDnsServers, setImportDnsServers] = useState('')
  const [importNamePrefix, setImportNamePrefix] = useState('')
  const [importGroupName, setImportGroupName] = useState('')
  const [webshareApiKey, updateWebshareApiKey] = useState('')
  const [fetchingWebshare, setFetchingWebshare] = useState(false)
  const [chainImportText, setChainImportText] = useState('')
  const [directImportText, setDirectImportText] = useState('')
  const [chainImportForm, setChainImportForm] = useState<ChainImportForm>(() => createInitialChainImportForm())
  const [directImportForm, setDirectImportForm] = useState<DirectImportForm>(() => ({ ...INITIAL_DIRECT_IMPORT_FORM }))
  const [previewModalOpen, setPreviewModalOpen] = useState(false)
  const [previewList, setPreviewList] = useState<ProxyDisplayInfo[]>([])
  const [removedPreviewProxyNames, setRemovedPreviewProxyNames] = useState<string[]>([])
  const [importing, setImporting] = useState(false)
  const [fetchingImportUrl, setFetchingImportUrl] = useState(false)

  const editedSources = useRef({ clashUrl: false, webshareApiKey: false })
  const [importSources, setImportSources] = useState<ProxyImportSource[]>([])
  const [selectedSources, setSelectedSources] = useState({ clash: '', webshare: '' })
  const [sourceNames, setSourceNames] = useState({ clash: '', webshare: '' })
  const [loadingSources, setLoadingSources] = useState(true)
  const [savingSource, setSavingSource] = useState(false)
  const sourceKind = importMode === 'webshare' ? 'webshare' : 'clash'
  const selectedImportSourceId = selectedSources[sourceKind]
  const importSourceName = sourceNames[sourceKind]

  useEffect(() => {
    let active = true
    void getProxyImportSources().then(saved => {
      if (!active) return
      setImportSources(saved.sources || [])
      const clash = saved.sources.find(item => item.id === saved.selectedClashId)
      const webshare = saved.sources.find(item => item.id === saved.selectedWebshareId)
      if (!editedSources.current.clashUrl && clash) {
        setImportUrl(clash.value)
        setSelectedSources(prev => ({ ...prev, clash: clash.id }))
        setSourceNames(prev => ({ ...prev, clash: clash.name }))
      }
      if (!editedSources.current.webshareApiKey && webshare) {
        updateWebshareApiKey(webshare.value)
        setSelectedSources(prev => ({ ...prev, webshare: webshare.id }))
        setSourceNames(prev => ({ ...prev, webshare: webshare.name }))
      }
    }).catch(() => {
      if (active) toast.error('读取已保存的导入来源失败')
    }).finally(() => { if (active) setLoadingSources(false) })
    return () => { active = false }
  }, [])

  const setWebshareApiKey = (value: string) => {
    editedSources.current.webshareApiKey = true
    updateWebshareApiKey(value)
  }

  const setImportSourceName = (name: string) => {
    editedSources.current[sourceKind === 'clash' ? 'clashUrl' : 'webshareApiKey'] = true
    setSourceNames(prev => ({ ...prev, [sourceKind]: name }))
  }

  const handleSelectImportSource = (id: string) => {
    const source = importSources.find(item => item.id === id && item.kind === sourceKind)
    editedSources.current[sourceKind === 'clash' ? 'clashUrl' : 'webshareApiKey'] = true
    setSelectedSources(prev => ({ ...prev, [sourceKind]: source?.id || '' }))
    setSourceNames(prev => ({ ...prev, [sourceKind]: source?.name || '' }))
    if (sourceKind === 'webshare') updateWebshareApiKey(source?.value || '')
    else { setImportUrl(source?.value || ''); setImportResolvedUrl(''); setImportText('') }
  }

  const rememberSource = async (field: 'clashUrl' | 'webshareApiKey', value: string) => {
    const kind = field === 'clashUrl' ? 'clash' : 'webshare'
    setSavingSource(true)
    try {
      const saved = await saveProxyImportSource(selectedSources[kind], kind, sourceNames[kind], value)
      setImportSources(prev => [...prev.filter(item => item.id !== saved.id), saved])
      setSelectedSources(prev => ({ ...prev, [kind]: saved.id }))
      setSourceNames(prev => ({ ...prev, [kind]: saved.name }))
      return true
    } catch {
      toast.error('保存导入来源失败，请检查是否重复或重试')
      return false
    } finally {
      setSavingSource(false)
    }
  }

  const handleSaveImportSource = async () => {
    const value = sourceKind === 'clash' ? importUrl.trim() : webshareApiKey.trim()
    if (!value) { toast.error('请先输入订阅 URL 或 API key'); return }
    if (await rememberSource(sourceKind === 'clash' ? 'clashUrl' : 'webshareApiKey', value)) toast.success('导入来源已保存')
  }

  const handleForgetImportSource = async () => {
    editedSources.current[sourceKind === 'clash' ? 'clashUrl' : 'webshareApiKey'] = true
    setSavingSource(true)
    try {
      if (selectedImportSourceId) await deleteProxyImportSource(selectedImportSourceId)
      setImportSources(prev => prev.filter(item => item.id !== selectedImportSourceId))
      setSelectedSources(prev => ({ ...prev, [sourceKind]: '' }))
      setSourceNames(prev => ({ ...prev, [sourceKind]: '' }))
      if (sourceKind === 'webshare') updateWebshareApiKey('')
      else { setImportUrl(''); setImportResolvedUrl('') }
      toast.success('已删除该导入来源')
    } catch {
      toast.error('删除导入来源失败，请重试')
    } finally {
      setSavingSource(false)
    }
  }

  const handleRemovePreviewProxy = (proxyId: string) => {
    const target = previewList.find(item => item.proxyId === proxyId)
    if (target) {
      setRemovedPreviewProxyNames(prev => prev.includes(target.proxyName) ? prev : [...prev, target.proxyName])
    }
    setPreviewList(prev => prev.filter(item => item.proxyId !== proxyId))
  }

  const updateChainImportHop = (hop: 'first' | 'second', field: keyof ChainImportForm['first'], value: string) => {
    setChainImportForm(prev => ({
      ...prev,
      [hop]: {
        ...prev[hop],
        [field]: value,
      },
    }))
  }

  const handleImportModeChange = (nextMode: ProxyImportMode) => {
    setImportMode(nextMode)
    setImportResolvedUrl('')
    if (nextMode !== 'clash') {
      setImportFetchProxyId('')
      setImportDnsServers('')
    }
  }

  const handleFillChainTemplate = () => {
    setChainImportText(CHAIN_QUICK_IMPORT_TEMPLATE)
  }

  const handleFillDirectTemplate = () => {
    setDirectImportText(DIRECT_QUICK_IMPORT_TEMPLATE)
  }

  const handleCopyChainTemplate = async () => {
    try {
      if (!navigator?.clipboard?.writeText) throw new Error('当前环境不支持剪贴板')
      await navigator.clipboard.writeText(CHAIN_QUICK_IMPORT_TEMPLATE)
      toast.success('JSON 模板已复制')
    } catch (error: any) {
      toast.error(error?.message || '复制模板失败')
    }
  }

  const handleCopyDirectTemplate = async () => {
    try {
      if (!navigator?.clipboard?.writeText) throw new Error('当前环境不支持剪贴板')
      await navigator.clipboard.writeText(DIRECT_QUICK_IMPORT_TEMPLATE)
      toast.success('JSON 模板已复制')
    } catch (error: any) {
      toast.error(error?.message || '复制模板失败')
    }
  }

  const handleApplyChainJSON = () => {
    try {
      const { form, groupName } = parseChainImportJSON(chainImportText)
      setChainImportForm(form)
      setImportGroupName(groupName)
      toast.success('JSON 已应用')
    } catch (error: any) {
      toast.error(error?.message || 'JSON 应用失败')
    }
  }

  const handleApplyDirectText = () => {
    try {
      const { form, groupName } = parseDirectImportText(directImportText)
      setDirectImportForm(form)
      if (groupName) setImportGroupName(groupName)
      setDirectImportText('')
      toast.success('文本已应用')
    } catch (error: any) {
      toast.error(error?.message || '文本应用失败')
    }
  }

  const handleImportUrlChange = (nextValue: string) => {
    editedSources.current.clashUrl = true
    setImportUrl(nextValue)
    if (importResolvedUrl.trim() && nextValue.trim() !== importResolvedUrl.trim()) {
      setImportResolvedUrl('')
    }
  }

  const handleFetchImportURL = async () => {
    const targetURL = importUrl.trim()
    if (!targetURL) {
      toast.error('请输入订阅 URL')
      return
    }

    setFetchingImportUrl(true)
    try {
      const result = await fetchClashImportFromURL(targetURL, importFetchProxyId)
      const content = (result?.content || '').trim()
      if (!content) throw new Error('订阅内容为空')

      await rememberSource('clashUrl', targetURL)
      setImportResolvedUrl((result?.url || targetURL).trim())
      setImportText(content)

      if (!importDnsServers.trim() && typeof result?.dnsServers === 'string' && result.dnsServers.trim()) {
        setImportDnsServers(result.dnsServers.trim())
      }
      if (!importGroupName.trim() && typeof result?.suggestedGroup === 'string' && result.suggestedGroup.trim()) {
        setImportGroupName(result.suggestedGroup.trim())
      }

      toast.success(`URL 获取成功，检测到 ${Math.max(0, Number(result?.proxyCount || 0))} 个代理`)
    } catch (error: any) {
      setImportResolvedUrl('')
      toast.error(error?.message || 'URL 获取失败')
    } finally {
      setFetchingImportUrl(false)
    }
  }

  const handleFetchWebshare = async () => {
    const apiKey = webshareApiKey.trim()
    if (!apiKey) {
      toast.error('请输入 Webshare API key')
      return
    }
    setFetchingWebshare(true)
    try {
      const result = await fetchWebshareProxies(apiKey)
      const candidates = buildWebshareImportCandidates(result.results || [])
      if (!candidates.length) throw new Error('Webshare 未返回可导入的有效代理')
      await rememberSource('webshareApiKey', apiKey)
      setImportMode('webshare')
      setRemovedPreviewProxyNames([])
      setPreviewList(buildImportPreview(candidates, importGroupName.trim() || 'Webshare'))
      setImportModalOpen(false)
      setPreviewModalOpen(true)
      toast.success(`已获取 ${candidates.length} 个 Webshare 代理`)
    } catch (error: any) {
      toast.error(error?.message || '获取 Webshare 代理失败')
    } finally {
      setFetchingWebshare(false)
    }
  }

  const handleParseImport = () => {
    try {
      const prefix = importNamePrefix.trim()
      let candidates
      let previewGroupName = importGroupName.trim()
      if (importMode === 'clash') {
        candidates = buildImportCandidatesFromClash(parseClashImportText(importText), prefix)
      } else if (importMode === 'direct') {
        if (directImportText.trim()) {
          const parsed = buildDirectImportCandidatesFromText(directImportText)
          candidates = parsed.candidates
          if (!previewGroupName) previewGroupName = parsed.defaultGroupName
        } else {
          candidates = [buildDirectImportCandidate(directImportForm)]
        }
      } else {
        candidates = [buildChainImportCandidate(chainImportForm)]
      }
      if (!candidates.length) {
        toast.error('未解析到可导入代理')
        return
      }
      const preview = buildImportPreview(candidates, previewGroupName)
      setRemovedPreviewProxyNames([])
      setPreviewList(preview)
      setImportModalOpen(false)
      setPreviewModalOpen(true)
    } catch (error: any) {
      toast.error(`解析失败: ${error?.message || '未知错误'}`)
    }
  }

  const handleConfirmImport = async () => {
    if (previewList.length === 0) {
      toast.error('请至少保留 1 个代理后再导入')
      return
    }
    setImporting(true)
    try {
      const sourceURL = importMode === 'clash' ? importResolvedUrl.trim() : ''
      const isURLImport = !!sourceURL
      const sourceNamePrefix = importMode === 'clash' ? importNamePrefix.trim() : ''
      const sourceID = isURLImport ? resolveImportSourceID(proxies, sourceURL, sourceNamePrefix) : ''
      const sourceAutoRefresh = isURLImport ? globalAutoRefreshEnabled : false
      const sourceRefreshIntervalM = sourceAutoRefresh ? globalRefreshInterval : 0
      const sourceLastRefreshAt = isURLImport ? new Date().toISOString() : ''
      const oldSourceProxies = isURLImport
        ? proxies.filter(item => (item.sourceId || '').trim() === sourceID)
        : proxies
      const pickExisting = createExistingProxyPicker(oldSourceProxies)

      const newProxies: BrowserProxy[] = previewList.map((p) => {
        const existingProxy = pickExisting(p.proxyName, p.proxyConfig)
        return {
        proxyId: existingProxy?.proxyId || nextProxyID(),
        proxyName: p.proxyName,
        proxyConfig: p.proxyConfig,
        preferredKernel: existingProxy?.preferredKernel || undefined,
        dnsServers: importMode === 'clash' ? importDnsServers.trim() || undefined : undefined,
        groupName: p.groupName.trim() || undefined,
        sourceId: sourceID || undefined,
        sourceUrl: sourceURL || undefined,
        sourceNamePrefix: sourceNamePrefix || undefined,
        sourceAutoRefresh,
        sourceRefreshIntervalM,
        sourceLastRefreshAt: sourceLastRefreshAt || undefined,
        }
      })
      const allProxies = isURLImport
        ? mergeImportedProxies(proxies.filter(item => (item.sourceId || '').trim() !== sourceID), newProxies)
        : mergeImportedProxies(proxies, newProxies)
      await saveProxies(allProxies)
      if (isURLImport && removedPreviewProxyNames.length > 0) {
        appendSourceIgnoredProxyNames(sourceID, removedPreviewProxyNames)
      }
      setPreviewModalOpen(false)
      setImportFetchProxyId('')
      setImportResolvedUrl('')
      setImportText('')
      setImportDnsServers('')
      setImportNamePrefix('')
      setImportGroupName('')
      setChainImportText('')
      setDirectImportText('')
      setChainImportForm(createInitialChainImportForm())
      setDirectImportForm({ ...INITIAL_DIRECT_IMPORT_FORM })
      setPreviewList([])
      setRemovedPreviewProxyNames([])
      toast.success(`成功导入 ${newProxies.length} 个代理`)
    } catch (error: any) {
      toast.error(error?.message || '导入失败')
    } finally {
      setImporting(false)
    }
  }

  const canParseImport = importMode === 'clash'
    ? !!importText.trim()
    : importMode === 'direct'
      ? !!directImportText.trim() || (!!directImportForm.server.trim() && !!directImportForm.port.trim())
      : !!chainImportForm.first.server.trim()
        && !!chainImportForm.first.port.trim()
        && !!chainImportForm.second.server.trim()
        && !!chainImportForm.second.port.trim()

  return {
    importModalOpen, setImportModalOpen, importMode, importUrl, importFetchProxyId, importResolvedUrl, importText,
    importDnsServers, importNamePrefix, importGroupName, webshareApiKey, fetchingWebshare, chainImportText, directImportText,
    chainImportForm, directImportForm, previewModalOpen, setPreviewModalOpen, previewList, removedPreviewProxyNames,
    importing, fetchingImportUrl, canParseImport, setImportText, setImportDnsServers,
    setImportNamePrefix, setImportGroupName, setWebshareApiKey, setImportFetchProxyId, setChainImportText, setDirectImportText,
    setChainImportForm, setDirectImportForm, handleRemovePreviewProxy, updateChainImportHop,
    handleImportModeChange, handleFillChainTemplate, handleFillDirectTemplate, handleCopyChainTemplate,
    handleCopyDirectTemplate, handleApplyChainJSON, handleApplyDirectText, handleImportUrlChange,
    handleFetchImportURL, handleFetchWebshare, handleParseImport, handleConfirmImport, handleForgetImportSource,
    importSources, selectedImportSourceId, importSourceName, savingSource, loadingSources,
    handleSelectImportSource, setImportSourceName, handleSaveImportSource,
  }
}
