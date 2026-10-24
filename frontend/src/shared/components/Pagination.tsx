import { ChevronLeft, ChevronRight } from 'lucide-react'

export function Pagination({ page, pageSize, total, onPageChange, onPageSizeChange }: { page: number; pageSize: number; total: number; onPageChange: (page: number) => void; onPageSizeChange: (size: number) => void }) {
  const pageCount = Math.max(1, Math.ceil(total / pageSize))
  const current = Math.min(page, pageCount)
  if (total === 0) return null
  return <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--color-border-muted)] px-4 py-3 text-xs text-[var(--color-text-muted)]">
    <span>共 {total} 条，第 {current} / {pageCount} 页</span>
    <div className="flex items-center gap-2">
      <label className="flex items-center gap-1.5">每页
        <select value={pageSize} onChange={e => onPageSizeChange(Number(e.target.value))} className="rounded border border-[var(--color-border-default)] bg-[var(--color-bg-surface)] px-2 py-1 text-xs text-[var(--color-text-primary)]">
          {[20, 50, 100].map(size => <option key={size} value={size}>{size}</option>)}
        </select>条
      </label>
      <button type="button" aria-label="上一页" disabled={current <= 1} onClick={() => onPageChange(current - 1)} className="rounded p-1.5 hover:bg-[var(--color-bg-muted)] disabled:opacity-40"><ChevronLeft className="h-3.5 w-3.5" /></button>
      <button type="button" aria-label="下一页" disabled={current >= pageCount} onClick={() => onPageChange(current + 1)} className="rounded p-1.5 hover:bg-[var(--color-bg-muted)] disabled:opacity-40"><ChevronRight className="h-3.5 w-3.5" /></button>
    </div>
  </div>
}
