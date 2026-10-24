import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Copy, HeartPulse, MoreHorizontal, RefreshCw, Trash2 } from 'lucide-react'
import { Button, toast } from '../../../../shared/components'
import { ClipboardSetText } from '../../../../wailsjs/runtime/runtime'

interface Props {
  open: boolean
  onToggle: () => void
  onClose: () => void
  proxyConfig: string
  builtin: boolean
  hasSource: boolean
  refreshing: boolean
  checking: boolean
  onRefresh: () => void
  onCheck: () => void
  onDelete: () => void
}

export function ProxyMoreActions({ open, onToggle, onClose, proxyConfig, builtin, hasSource, refreshing, checking, onRefresh, onCheck, onDelete }: Props) {
  const triggerRef = useRef<HTMLDivElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const [position, setPosition] = useState({ top: 0, left: 0 })
  useEffect(() => {
    if (!open) return
    const update = () => {
      const rect = triggerRef.current?.getBoundingClientRect()
      if (!rect) return
      const height = menuRef.current?.offsetHeight || 176
      setPosition({
        left: Math.max(8, Math.min(rect.right - 176, window.innerWidth - 184)),
        top: rect.bottom + 8 + height > window.innerHeight ? Math.max(8, rect.top - height - 8) : rect.bottom + 8,
      })
    }
    const outside = (event: MouseEvent) => {
      if (!triggerRef.current?.contains(event.target as Node) && !menuRef.current?.contains(event.target as Node)) onClose()
    }
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { onClose(); triggerRef.current?.querySelector('button')?.focus() }
    }
    update()
    document.addEventListener('mousedown', outside)
    document.addEventListener('keydown', escape)
    window.addEventListener('resize', update)
    window.addEventListener('scroll', update, true)
    return () => {
      document.removeEventListener('mousedown', outside)
      document.removeEventListener('keydown', escape)
      window.removeEventListener('resize', update)
      window.removeEventListener('scroll', update, true)
    }
  }, [open, onClose])

  const run = (action: () => void) => { onClose(); action() }
  const copy = async () => {
    try {
      if ((window as any).runtime?.ClipboardSetText) {
        if (!await ClipboardSetText(proxyConfig)) throw new Error('clipboard unavailable')
      } else {
        await navigator.clipboard.writeText(proxyConfig)
      }
      toast.success('代理配置已复制到剪切板')
    } catch {
      toast.error('复制失败，请检查剪切板权限后重试')
    }
  }
  const itemClass = 'flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-xs text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-muted)] hover:text-[var(--color-text-primary)] disabled:opacity-40 disabled:cursor-not-allowed'
  return <>
    <div ref={triggerRef} className="inline-flex">
      <Button size="sm" variant="ghost" title="更多" className="px-2" aria-expanded={open} onClick={onToggle}>
        <MoreHorizontal className="h-3.5 w-3.5" />更多
      </Button>
    </div>
    {open && createPortal(
      <div ref={menuRef} className="fixed z-[9999] w-44 rounded-xl border border-[var(--color-border-default)] bg-[var(--color-bg-surface)] p-1.5 shadow-xl" style={position} onClick={event => event.stopPropagation()}>
        {hasSource && <button type="button" className={itemClass} disabled={refreshing} onClick={() => run(onRefresh)}><RefreshCw className={`h-3.5 w-3.5 ${refreshing ? 'animate-spin' : ''}`} />刷新订阅</button>}
        <button type="button" className={itemClass} disabled={checking || proxyConfig === 'direct://'} onClick={() => run(onCheck)}><HeartPulse className="h-3.5 w-3.5" />IP 健康</button>
        <button type="button" className={itemClass} onClick={() => run(() => { void copy() })}><Copy className="h-3.5 w-3.5" />复制到剪切板</button>
        <button type="button" className={`${itemClass} text-[var(--color-error)]`} disabled={builtin} title={builtin ? '内置代理不可删除' : undefined} onClick={() => run(onDelete)}><Trash2 className="h-3.5 w-3.5 text-[var(--color-error)]" />删除</button>
      </div>, document.body,
    )}
  </>
}
