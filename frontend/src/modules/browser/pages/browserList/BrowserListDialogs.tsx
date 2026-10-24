import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { XCircle } from 'lucide-react'
import { Button, Modal } from '../../../../shared/components'
import { BrowserProfileCopyForm } from '../../components/BrowserProfileCopyForm'
import { KeywordsModal } from '../../components/KeywordsModal'
import { ProfilePackageConflictModal } from '../../../backup/components/ProfilePackageConflictModal'
import type { BrowserProfile, BrowserProfileCopyOptions, BrowserProfilePackageImportAction, BrowserProfilePackageImportPreview } from '../../types'

interface BrowserListDialogsProps {
  proxyErrorModal: boolean
  pendingStartId: string | null
  proxyErrorMsg: string
  onCloseProxyError: () => void
  onStartDirect: () => void
  startingDirect: boolean
  kwModal: { open: boolean; profile: BrowserProfile | null }
  onCloseKeywords: () => void
  onKeywordsSaved: (keywords: string[]) => void
  copyModal: { open: boolean; profile: BrowserProfile | null }
  copyName: string
  copyOptions: BrowserProfileCopyOptions
  onCopyNameChange: (value: string) => void
  onCopyOptionsChange: (value: BrowserProfileCopyOptions) => void
  onCloseCopy: () => void
  onConfirmCopy: () => void
  copyConfirmDisabled: boolean
  copying: boolean
  deleteConfirm: { open: boolean; mode: 'single' | 'batch'; profileName?: string; count: number }
  deleting: boolean
  onCloseDeleteConfirm: () => void
  onConfirmDelete: () => void
  trashModalOpen: boolean
  trashProfiles: BrowserProfile[]
  trashLoading: boolean
  restoringId: string
  permanentlyDeletingId: string
  permanentDeleteConfirm: { open: boolean; profiles: BrowserProfile[] }
  onCloseTrash: () => void
  onRestoreProfiles: (profileIds: string[]) => void
  onOpenPermanentDelete: (profiles: BrowserProfile[]) => void
  onClosePermanentDelete: () => void
  onConfirmPermanentDelete: () => void
  opError: string
  onCloseOpError: () => void
  profileImportPreview: BrowserProfilePackageImportPreview | null
  profileImportBusy: boolean
  onCloseProfileImport: () => void
  onConfirmProfileImport: (actions: BrowserProfilePackageImportAction[]) => void
}

export function BrowserListDialogs({
  proxyErrorModal,
  pendingStartId,
  proxyErrorMsg,
  onCloseProxyError,
  onStartDirect,
  startingDirect,
  kwModal,
  onCloseKeywords,
  onKeywordsSaved,
  copyModal,
  copyName,
  copyOptions,
  onCopyNameChange,
  onCopyOptionsChange,
  onCloseCopy,
  onConfirmCopy,
  copyConfirmDisabled,
  copying,
  deleteConfirm,
  deleting,
  onCloseDeleteConfirm,
  onConfirmDelete,
  trashModalOpen,
  trashProfiles,
  trashLoading,
  restoringId,
  permanentlyDeletingId,
  permanentDeleteConfirm,
  onCloseTrash,
  onRestoreProfiles,
  onOpenPermanentDelete,
  onClosePermanentDelete,
  onConfirmPermanentDelete,
  opError,
  onCloseOpError,
  profileImportPreview,
  profileImportBusy,
  onCloseProfileImport,
  onConfirmProfileImport,
}: BrowserListDialogsProps) {
  const [selectedTrashIds, setSelectedTrashIds] = useState<Set<string>>(new Set())
  const trashBusy = !!restoringId || !!permanentlyDeletingId
  const selectedTrash = trashProfiles.filter(profile => selectedTrashIds.has(profile.profileId))
  const allTrashSelected = trashProfiles.length > 0 && selectedTrash.length === trashProfiles.length
  useEffect(() => {
    setSelectedTrashIds(previous => trashModalOpen
      ? new Set(trashProfiles.filter(profile => previous.has(profile.profileId)).map(profile => profile.profileId))
      : new Set())
  }, [trashProfiles, trashModalOpen])
  const toggleTrash = (id: string) => setSelectedTrashIds(previous => {
    const next = new Set(previous)
    next.has(id) ? next.delete(id) : next.add(id)
    return next
  })
  const formatTime = (value?: string) => {
    if (!value) return '-'
    const date = new Date(value)
    return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString('zh-CN')
  }

  const formatExpireTime = (value?: string) => {
    if (!value) return '-'
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return '-'
    date.setDate(date.getDate() + 3)
    return date.toLocaleString('zh-CN')
  }

  return (
    <>
      <Modal
        open={proxyErrorModal}
        onClose={onCloseProxyError}
        title="代理链路不可用"
        width="420px"
        footer={
          <>
            <Button variant="secondary" onClick={onCloseProxyError} disabled={startingDirect}>取消</Button>
            {pendingStartId && (
              <Button variant="secondary" onClick={onStartDirect} loading={startingDirect}>
                直连启动
              </Button>
            )}
            {pendingStartId && (
              <Link to={`/browser/edit/${pendingStartId}`}>
                <Button onClick={onCloseProxyError} disabled={startingDirect}>去修改代理</Button>
              </Link>
            )}
          </>
        }
      >
        <div className="space-y-3">
          <div className="flex items-start gap-3 p-3 rounded-lg bg-[var(--color-bg-secondary)]">
            <XCircle className="w-5 h-5 text-red-500 mt-0.5 shrink-0" />
            <p className="text-sm text-[var(--color-text-primary)]">{proxyErrorMsg}</p>
          </div>
          <p className="text-sm text-[var(--color-text-muted)]">请前往编辑页面重新选择可用链路；如果是订阅导入，先刷新订阅并确认该节点仍存在。</p>
        </div>
      </Modal>

      {kwModal.profile && (
        <KeywordsModal
          open={kwModal.open}
          profileId={kwModal.profile.profileId}
          profileName={kwModal.profile.profileName}
          initialKeywords={kwModal.profile.keywords || []}
          onClose={onCloseKeywords}
          onSaved={onKeywordsSaved}
        />
      )}

      <Modal
        open={copyModal.open}
        onClose={onCloseCopy}
        title="复制实例"
        width="720px"
        footer={
          <>
            <Button variant="secondary" onClick={onCloseCopy}>取消</Button>
            <Button onClick={onConfirmCopy} loading={copying} disabled={copyConfirmDisabled}>确认复制</Button>
          </>
        }
      >
        <BrowserProfileCopyForm
          sourceName={copyModal.profile?.profileName}
          copyName={copyName}
          copyOptions={copyOptions}
          onCopyNameChange={onCopyNameChange}
          onCopyOptionsChange={onCopyOptionsChange}
          autoFocusName
        />
      </Modal>

      <Modal
        open={deleteConfirm.open}
        onClose={onCloseDeleteConfirm}
        title="删除实例"
        width="400px"
        footer={
          <>
            <Button variant="secondary" onClick={onCloseDeleteConfirm} disabled={deleting}>取消</Button>
            <Button variant="danger" onClick={onConfirmDelete} loading={deleting}>确定删除</Button>
          </>
        }
      >
        <div className="text-sm text-[var(--color-text-secondary)]">
          {deleteConfirm.mode === 'batch'
            ? `确定将选中的 ${deleteConfirm.count} 个实例移入回收站？3 天内可恢复。`
            : `确定将实例「${deleteConfirm.profileName || '未命名实例'}」移入回收站？3 天内可恢复。`}
        </div>
      </Modal>

      <Modal
        open={trashModalOpen}
        onClose={onCloseTrash}
        title="实例回收站"
        width="720px"
        closable={!trashBusy}
        footer={<Button variant="secondary" onClick={onCloseTrash} disabled={trashBusy}>关闭</Button>}
      >
        {trashLoading ? (
          <div className="py-10 text-center text-sm text-[var(--color-text-muted)]">加载中...</div>
        ) : trashProfiles.length === 0 ? (
          <div className="py-10 text-center text-sm text-[var(--color-text-muted)]">回收站为空</div>
        ) : (
          <div className="space-y-3">
            <div className="flex flex-wrap items-center gap-2">
              <label className="flex items-center gap-2 text-sm text-[var(--color-text-secondary)]">
                <input type="checkbox" aria-label="全选回收站实例" checked={allTrashSelected} disabled={trashBusy}
                  ref={element => { if (element) element.indeterminate = selectedTrash.length > 0 && !allTrashSelected }}
                  onChange={() => setSelectedTrashIds(allTrashSelected ? new Set() : new Set(trashProfiles.map(profile => profile.profileId)))}
                  className="h-4 w-4 accent-[var(--color-accent)]" />
                全选
              </label>
              <span className="mr-auto text-xs text-[var(--color-text-muted)]">已选 {selectedTrash.length} / {trashProfiles.length}</span>
              <Button size="sm" variant="secondary" disabled={!selectedTrash.length || trashBusy} onClick={() => setSelectedTrashIds(new Set())}>取消选择</Button>
              <Button size="sm" disabled={!selectedTrash.length || trashBusy} loading={restoringId === '__batch__'} onClick={() => onRestoreProfiles(selectedTrash.map(profile => profile.profileId))}>恢复所选</Button>
              <Button size="sm" variant="danger" disabled={!selectedTrash.length || trashBusy} onClick={() => onOpenPermanentDelete(selectedTrash)}>彻底删除所选</Button>
            </div>
            <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[var(--color-border-default)] text-left text-xs text-[var(--color-text-muted)]">
                  <th className="w-8"><span className="sr-only">选择</span></th>
                  <th className="py-2 pr-3 font-medium">实例</th>
                  <th className="py-2 pr-3 font-medium">删除时间</th>
                  <th className="py-2 pr-3 font-medium">自动清理</th>
                  <th className="py-2 text-right font-medium">操作</th>
                </tr>
              </thead>
              <tbody>
                {trashProfiles.map((profile) => (
                  <tr key={profile.profileId} className="border-b border-[var(--color-border-muted)] last:border-0">
                    <td className="pr-3"><input type="checkbox" aria-label={`选择 ${profile.profileName || '未命名实例'}`} checked={selectedTrashIds.has(profile.profileId)} disabled={trashBusy} onChange={() => toggleTrash(profile.profileId)} className="h-4 w-4 accent-[var(--color-accent)]" /></td>
                    <td className="py-3 pr-3 text-[var(--color-text-primary)]">{profile.profileName || '未命名实例'}</td>
                    <td className="py-3 pr-3 text-[var(--color-text-secondary)]">{formatTime(profile.deletedAt)}</td>
                    <td className="py-3 pr-3 text-[var(--color-text-secondary)]">{formatExpireTime(profile.deletedAt)}</td>
                    <td className="py-3 text-right">
                      <div className="flex justify-end gap-2">
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() => onRestoreProfiles([profile.profileId])}
                          loading={restoringId === profile.profileId}
                          disabled={trashBusy}
                        >
                          恢复
                        </Button>
                        <Button
                          size="sm"
                          variant="danger"
                          onClick={() => onOpenPermanentDelete([profile])}
                          loading={permanentlyDeletingId === profile.profileId}
                          disabled={trashBusy}
                        >
                          彻底删除
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            </div>
          </div>
        )}
      </Modal>

      <Modal
        open={permanentDeleteConfirm.open}
        onClose={onClosePermanentDelete}
        title="彻底删除实例"
        closable={!trashBusy}
        width="420px"
        footer={
          <>
            <Button variant="secondary" onClick={onClosePermanentDelete} disabled={trashBusy}>取消</Button>
            <Button variant="danger" onClick={onConfirmPermanentDelete} loading={!!permanentlyDeletingId}>彻底删除</Button>
          </>
        }
      >
        <div className="space-y-2 text-sm text-[var(--color-text-secondary)]">
          <p>{permanentDeleteConfirm.profiles.length === 1
            ? `确定彻底删除实例「${permanentDeleteConfirm.profiles[0].profileName || '未命名实例'}」？`
            : `确定彻底删除选中的 ${permanentDeleteConfirm.profiles.length} 个实例？`}</p>
          <p className="text-red-500">这会删除配置、浏览器用户数据、快照、快捷码和插件绑定，删除后不可恢复。</p>
        </div>
      </Modal>

      <ProfilePackageConflictModal
        preview={profileImportPreview}
        busy={profileImportBusy}
        onClose={onCloseProfileImport}
        onConfirm={onConfirmProfileImport}
      />

      <Modal
        open={!!opError}
        onClose={onCloseOpError}
        title="操作失败"
        width="420px"
        footer={<Button onClick={onCloseOpError}>知道了</Button>}
      >
        <div className="text-[var(--color-text-secondary)] whitespace-pre-line">{opError}</div>
      </Modal>
    </>
  )
}
