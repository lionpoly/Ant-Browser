import { useEffect, useMemo, useState } from 'react'
import { Download, RefreshCw, Trash2, Upload } from 'lucide-react'
import { Badge, Button, Card, Input, Modal, Table, Textarea, toast } from '../../../shared/components'
import type { TableColumn } from '../../../shared/components/Table'
import type { CookieInfo } from '../types'
import { clearBrowserCookies, exportBrowserCookies, fetchBrowserCookies, importBrowserCookies } from '../api'

interface Props {
  profileId: string
  profileName: string
  running: boolean
  ready: boolean
}

const formatExpires = (expires: number) => {
  if (expires <= 0) return 'Session'
  return new Date(expires * 1000).toLocaleString('zh-CN')
}

export function CookieManagerCard({ profileId, profileName, running, ready }: Props) {
  const [cookies, setCookies] = useState<CookieInfo[]>([])
  const [filterDomain, setFilterDomain] = useState('')
  const [loading, setLoading] = useState(false)
  const [clearing, setClearing] = useState(false)
  const [showConfirm, setShowConfirm] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [importText, setImportText] = useState('')
  const [importing, setImporting] = useState(false)

  const closeImport = () => { if (!importing) { setImportOpen(false); setImportText('') } }
  const handleImport = async () => {
    setImporting(true)
    try {
      const count = await importBrowserCookies(profileId, importText)
      setImportOpen(false)
      setImportText('')
      toast.success(`已导入 ${count} 条 Cookie`)
      await loadCookies()
    } catch (error: any) { toast.error(error?.message || '导入 Cookie 失败') }
    finally { setImporting(false) }
  }


  const loadCookies = async () => {
    if (!ready) return
    setLoading(true)
    try {
      const list = await fetchBrowserCookies(profileId)
      setCookies(list)
    } catch {
      toast.error('获取 Cookie 失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (ready) loadCookies()
    else setCookies([])
  }, [profileId, ready])

  const filteredCookies = useMemo(() => {
    if (!filterDomain.trim()) return cookies
    const kw = filterDomain.toLowerCase()
    return cookies.filter(c => c.domain.toLowerCase().includes(kw))
  }, [cookies, filterDomain])

  const handleClear = async () => {
    setClearing(true)
    try {
      await clearBrowserCookies(profileId)
      setCookies([])
      toast.success('Cookie 已清除')
    } catch {
      toast.error('清除 Cookie 失败')
    } finally {
      setClearing(false)
      setShowConfirm(false)
    }
  }

  const handleExport = async () => {
    try {
      const content = await exportBrowserCookies(profileId)
      const date = new Date().toISOString().slice(0, 10)
      const filename = `cookies_${profileName}_${date}.txt`
      const blob = new Blob([content], { type: 'text/plain' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = filename
      a.click()
      URL.revokeObjectURL(url)
      toast.success('Cookie 已导出')
    } catch {
      toast.error('导出 Cookie 失败')
    }
  }

  const columns: TableColumn<CookieInfo>[] = [
    { key: 'domain', title: '域名', render: v => <span className="font-mono text-xs">{v}</span> },
    { key: 'name', title: '名称', render: v => <span className="font-mono text-xs">{v}</span> },
    {
      key: 'value',
      title: '值',
      render: v => (
        <span className="font-mono text-xs max-w-[120px] truncate block" title={v as string}>{v}</span>
      ),
    },
    { key: 'expires', title: '过期时间', render: v => formatExpires(v as number) },
    {
      key: 'httpOnly',
      title: 'HttpOnly',
      render: v => <Badge variant={v ? 'success' : 'default'}>{v ? '是' : '否'}</Badge>,
    },
    {
      key: 'secure',
      title: 'Secure',
      render: v => <Badge variant={v ? 'success' : 'default'}>{v ? '是' : '否'}</Badge>,
    },
  ]

  const subtitle = !running
    ? '实例未运行，无法管理 Cookie'
    : !ready
      ? '实例运行中，等待调试接口就绪后可管理 Cookie'
      : `共 ${cookies.length} 条${filterDomain ? `，已过滤 ${filteredCookies.length} 条` : ''}`

  return (
    <>
    <Card title="Cookie 管理" subtitle={subtitle}>
      {!running ? (
        <p className="text-sm text-[var(--color-text-muted)] py-4 text-center">
          请先启动实例以查看 Cookie
        </p>
      ) : !ready ? (
        <p className="text-sm text-[var(--color-text-muted)] py-4 text-center">
          浏览器已启动，正在等待调试接口就绪
        </p>
      ) : (
        <div className="space-y-3">
          <div className="flex flex-col sm:flex-row gap-2 items-start sm:items-center justify-between">
            <Input
              placeholder="按域名过滤..."
              value={filterDomain}
              onChange={e => setFilterDomain(e.target.value)}
              className="w-full sm:w-64"
            />
            <div className="flex gap-2 flex-shrink-0">
              <Button size="sm" variant="ghost" onClick={loadCookies} disabled={loading || clearing || importing}>
                <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
                刷新
              </Button>
              <Button size="sm" variant="secondary" onClick={() => setImportOpen(true)} disabled={loading || clearing || importing}>
                <Upload className="w-4 h-4" />导入
              </Button>
              <Button size="sm" variant="ghost" onClick={handleExport} disabled={loading || clearing || importing}>
                <Download className="w-4 h-4" />
                导出 Netscape
              </Button>
              <Button size="sm" variant="secondary" onClick={() => setShowConfirm(true)} disabled={clearing || loading || importing}>
                <Trash2 className="w-4 h-4" />
                清除全部
              </Button>
            </div>
          </div>

          {showConfirm && (
            <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-elevated)] p-4 flex items-center justify-between gap-4">
              <span className="text-sm text-[var(--color-text-secondary)]">
                确认清除该实例的所有 Cookie？此操作不可撤销。
              </span>
              <div className="flex gap-2 flex-shrink-0">
                <Button size="sm" variant="ghost" onClick={() => setShowConfirm(false)}>取消</Button>
                <Button size="sm" onClick={handleClear} disabled={clearing}>确认清除</Button>
              </div>
            </div>
          )}

          <Table columns={columns} data={filteredCookies} rowKey={cookie => JSON.stringify([cookie.domain, cookie.path, cookie.name])} loading={loading} />
        </div>
      )}
    </Card>
    <Modal open={importOpen} onClose={closeImport} closable={!importing} title="导入 Cookie" width="640px" footer={<>
      <Button variant="secondary" onClick={closeImport} disabled={importing}>取消</Button>
      <Button onClick={() => void handleImport()} loading={importing} disabled={!ready || !importText.trim()}>导入 Cookie</Button>
    </>}>
      <div className="space-y-3">
        <p className="text-sm text-[var(--color-text-secondary)]">支持 Cookie JSON 数组或 Netscape 文本，同域名、路径和名称的 Cookie 将被覆盖。操作立即生效。</p>
        <input type="file" accept=".json,.txt" disabled={importing} aria-label="选择 Cookie 文件" onChange={async event => {
          const file = event.target.files?.[0]
          event.target.value = ''
          if (!file) return
          if (file.size > 2 * 1024 * 1024) { toast.error('Cookie 文件不能超过 2 MB'); return }
          try { setImportText(await file.text()) } catch { toast.error('读取 Cookie 文件失败') }
        }} />
        <Textarea value={importText} onChange={event => setImportText(event.target.value)} rows={12} maxLength={2 * 1024 * 1024} disabled={importing} placeholder="粘贴 Cookie JSON 或 Netscape 文本" />
      </div>
    </Modal>
    </>
  )
}
