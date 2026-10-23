import { useEffect, useState } from 'react'
import { Button, ConfirmModal, FormItem, Input, Modal, Table, toast } from '../../../shared/components'
import type { TableColumn } from '../../../shared/components/Table'
import type { BrowserGroupInput, BrowserGroupWithCount } from '../types'
import { createGroup, deleteGroup, fetchGroups, updateGroup } from '../api'
import { GroupSelector } from './GroupSelector'

interface Props {
  open: boolean
  onClose: () => void
  onChanged: (groups: BrowserGroupWithCount[], deleted?: BrowserGroupWithCount) => void
}

export function GroupManagerModal({ open, onClose, onChanged }: Props) {
  const [groups, setGroups] = useState<BrowserGroupWithCount[]>([])
  const [form, setForm] = useState<BrowserGroupInput | null>(null)
  const [editingId, setEditingId] = useState('')
  const [deleting, setDeleting] = useState<BrowserGroupWithCount | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!open) return
    let active = true
    setForm(null)
    setBusy(true)
    void fetchGroups().then(items => { if (active) setGroups(items) })
      .catch(() => toast.error('读取分组失败'))
      .finally(() => { if (active) setBusy(false) })
    return () => { active = false }
  }, [open])

  const refresh = async (deleted?: BrowserGroupWithCount) => {
    const items = await fetchGroups()
    setGroups(items)
    onChanged(items, deleted)
  }
  const save = async () => {
    if (!form?.groupName.trim()) { toast.error('请输入分组名称'); return }
    setBusy(true)
    try {
      const input = { ...form, groupName: form.groupName.trim() }
      const saved = editingId ? await updateGroup(editingId, input) : await createGroup(input)
      if (!saved) throw new Error('当前环境不支持分组管理')
      setForm(null)
      await refresh()
      toast.success('分组已保存')
    } catch (error: any) { toast.error(error?.message || '保存分组失败') }
    finally { setBusy(false) }
  }
  const remove = async (group: BrowserGroupWithCount) => {
    setBusy(true)
    try {
      if (!await deleteGroup(group.groupId)) throw new Error('当前环境不支持分组管理')
      await refresh(group)
      toast.success('分组已删除，实例已保留')
    } catch (error: any) { toast.error(error?.message || '删除分组失败') }
    finally { setBusy(false) }
  }
  // Exclude self and all descendants from eligible parents.
  const blocked = new Set(editingId ? [editingId] : [])
  for (let changed = true; changed;) {
    changed = false
    groups.forEach(group => {
      if (blocked.has(group.parentId) && !blocked.has(group.groupId)) { blocked.add(group.groupId); changed = true }
    })
  }
  const columns: TableColumn<BrowserGroupWithCount>[] = [
    { key: 'groupName', title: '分组名称' },
    { key: 'parentId', title: '上级分组', render: value => groups.find(group => group.groupId === value)?.groupName || '顶层' },
    { key: 'instanceCount', title: '实例数' },
    { key: 'actions', title: '操作', align: 'right', render: (_, group) => (
      <div className="flex justify-end gap-2">
        <Button variant="ghost" size="sm" disabled={busy} onClick={() => { setEditingId(group.groupId); setForm({ groupName: group.groupName, parentId: group.parentId, sortOrder: group.sortOrder }) }}>编辑</Button>
        <Button variant="ghost" size="sm" disabled={busy} onClick={() => setDeleting(group)}>删除</Button>
      </div>
    ) },
  ]
  return <>
    <Modal open={open} onClose={() => { if (!busy) onClose() }} closable={!busy} width="720px" title={form ? (editingId ? '编辑分组' : '新建分组') : '分组管理'}
      footer={form ? <>
        <Button variant="secondary" disabled={busy} onClick={() => setForm(null)}>返回分组列表</Button>
        <Button loading={busy} onClick={() => void save()}>保存分组</Button>
      </> : <Button disabled={busy} onClick={() => { setEditingId(''); setForm({ groupName: '', parentId: '', sortOrder: 0 }) }}>新建分组</Button>}>
      {form ? <div className="space-y-4">
        <FormItem label="分组名称" required><Input value={form.groupName} maxLength={100} disabled={busy} onChange={event => setForm({ ...form, groupName: event.target.value })} /></FormItem>
        <FormItem label="上级分组"><GroupSelector groups={groups.filter(group => !blocked.has(group.groupId))} value={form.parentId} onChange={parentId => setForm({ ...form, parentId })} placeholder="顶层分组" /></FormItem>
        <FormItem label="排序" hint="数字越小越靠前"><Input type="number" value={form.sortOrder} disabled={busy} onChange={event => setForm({ ...form, sortOrder: Math.trunc(Number(event.target.value) || 0) })} /></FormItem>
      </div> : <Table columns={columns} data={groups} rowKey="groupId" loading={busy} emptyText="暂无分组，点击“新建分组”开始" />}
    </Modal>
    <ConfirmModal open={!!deleting} onClose={() => setDeleting(null)} onConfirm={() => { if (deleting) void remove(deleting) }} title="删除分组？"
      content={`删除“${deleting?.groupName || ''}”后，子分组和实例会移到${deleting?.parentId ? '上级分组' : '未分组'}，不会删除实例。`} confirmText="删除分组" danger />
  </>
}
