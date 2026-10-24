import { useState, type Dispatch, type SetStateAction } from 'react'
import { Button, FormItem, Input, Modal, toast } from '../../../../shared/components'
import { BrowserOpenURL } from '../../../../wailsjs/runtime/runtime'
import type { BrowserProxy } from '../../types'
import { saveCloakBrowserLicenseKey } from '../../api/cores'
import type { CoreDownloadForm, CoreDownloadProgress } from '../coreManagement.types'
import { CORE_DOWNLOAD_SOURCES } from './coreDownloadRecommendation'
import type { CoreDownloadRecommendation, CoreDownloadSource } from './coreDownloadRecommendation'

interface CoreDownloadModalProps {
  open: boolean
  form: CoreDownloadForm
  progress: CoreDownloadProgress | null
  recommendation: CoreDownloadRecommendation | null
  recommendationLoading: boolean
  recommendationError: string
  proxies: BrowserProxy[]
  setForm: Dispatch<SetStateAction<CoreDownloadForm>>
  setProgress: Dispatch<SetStateAction<CoreDownloadProgress | null>>
  onSourceChange: (source: CoreDownloadSource) => void
  onRefreshRecommendation: () => void
  onClose: () => void
  onStart: () => void
}

export function CoreDownloadModal({
  open,
  form,
  progress,
  recommendation,
  recommendationLoading,
  recommendationError,
  proxies,
  setForm,
  setProgress,
  onSourceChange,
  onRefreshRecommendation,
  onClose,
  onStart,
}: CoreDownloadModalProps) {
  const [savingLicense, setSavingLicense] = useState(false)
  const downloading = progress !== null && progress.phase !== 'error'
  const isRedownload = form.mode === 'redownload'

  const handleSaveLicense = async () => {
    setSavingLicense(true)
    try {
      await saveCloakBrowserLicenseKey(form.licenseKey || '')
      toast.success('启动授权已保存，可直接启动已下载的内核，无需重新下载')
    } catch (error) {
      toast.error(String(error))
    } finally {
      setSavingLicense(false)
    }
  }

  const handleClose = () => {
    if (progress && progress.phase !== 'done' && progress.phase !== 'error') {
      toast.warning('正在下载中，请稍候...')
      return
    }
    onClose()
    setProgress(null)
  }

  return (
    <Modal
      open={open}
      onClose={handleClose}
      title={isRedownload ? '重新下载内核' : '下载内核'}
      width="480px"
      footer={
        <>
          <Button variant="secondary" onClick={handleClose} disabled={downloading}>取消</Button>
          <Button onClick={onStart} loading={downloading}>{isRedownload ? '开始重新下载' : '开始下载'}</Button>
        </>
      }
    >
      <div className="space-y-4">
        <FormItem label="内核来源">
          <select
            aria-label="内核来源"
            value={form.source}
            onChange={e => onSourceChange(e.target.value as CoreDownloadSource)}
            disabled={progress !== null}
            className="w-full h-9 px-3 rounded-md border border-[var(--color-border-default)] bg-[var(--color-bg-primary)] text-[var(--color-text-primary)] text-sm focus:outline-none focus:ring-1 focus:ring-[var(--color-accent)]"
          >
            {Object.entries(CORE_DOWNLOAD_SOURCES).map(([value, source]) => (
              <option key={value} value={value}>{source.label}</option>
            ))}
          </select>
          {form.source === 'cloakbrowser' && (
            <p className="text-xs text-[var(--color-text-muted)] mt-1">不填写 Key 时使用 GitHub 公开构建；填写 CloakBrowser Key 后自动获取当前平台的最新构建。</p>
          )}
        </FormItem>

        {form.source === 'cloakbrowser' && (
          <FormItem label="CloakBrowser Key（可选）">
            <Input
              type="password"
              value={form.licenseKey || ''}
              onChange={e => setForm(prev => ({ ...prev, licenseKey: e.target.value, url: '' }))}
              placeholder="cb_xxxxxxxxx"
              disabled={progress !== null}
              autoComplete="off"
            />
            <Button variant="secondary" onClick={handleSaveLicense} loading={savingLicense} disabled={!form.licenseKey?.trim() || progress !== null} className="mt-2">保存启动授权</Button>
            <p className="text-xs text-[var(--color-text-muted)] mt-1">下载成功或点击保存时，Key 会写入本机 ~/.cloakbrowser/license.key，由本机 CloakBrowser 共用。保存会替换已有授权；单独保存仅写入本机，不验证授权有效性。</p>
          </FormItem>
        )}

        <FormItem label="内核名称" required>
          <Input
            value={form.name}
            onChange={e => setForm(prev => ({ ...prev, name: e.target.value }))}
            placeholder={recommendation?.namePlaceholder || (form.source === 'cloakbrowser' ? '例如: cloakbrowser-latest' : '例如: chrome-latest')}
            disabled={progress !== null || isRedownload}
          />
          {!isRedownload && (
            <p className="text-xs text-[var(--color-text-muted)] mt-1">该名称将同时作为数据存放的子文件夹名。</p>
          )}
        </FormItem>

        {isRedownload && (
          <div className="rounded-lg border border-[var(--color-warning)]/40 bg-[var(--color-warning)]/10 p-3 text-xs leading-5 text-[var(--color-warning)]">
            请核对来源后点击“使用推荐”或自行填写地址。重新下载会在校验新压缩包可用后替换当前内核目录；替换失败会自动恢复旧目录。正在使用该内核的实例请先停止。
          </div>
        )}

        <FormItem label="下载地址" required>
          <Input
            value={form.url}
            onChange={e => setForm(prev => ({ ...prev, url: e.target.value }))}
            placeholder={recommendationLoading ? '正在匹配当前环境...' : 'https://github.com/.../chromium.zip'}
            disabled={progress !== null}
          />
          <div className="text-xs text-[var(--color-text-muted)] mt-2 flex items-center justify-between gap-3 bg-[var(--color-bg-muted)] p-2 rounded">
            <span className="min-w-0 truncate">
              {recommendation
                ? `已按 ${recommendation.target.label} 推荐：${recommendation.assetName}`
                : recommendationLoading
                  ? '正在按当前环境获取推荐地址...'
                  : recommendationError || '未获取到当前环境推荐地址'}
            </span>
            {recommendation && (
              <button
                type="button"
                onClick={() => setForm(prev => ({ ...prev, url: recommendation.downloadUrl }))}
                className="shrink-0 text-[var(--color-accent)] hover:underline cursor-pointer font-medium"
                disabled={progress !== null}
              >
                使用推荐
              </button>
            )}
            <button
              type="button"
              onClick={() => BrowserOpenURL(recommendation?.releasesUrl || CORE_DOWNLOAD_SOURCES[form.source].releasesUrl)}
              className="shrink-0 text-[var(--color-accent)] hover:underline cursor-pointer font-medium"
            >
              打开 Releases
            </button>
            {!recommendationLoading && !recommendation && (
              <button
                type="button"
                onClick={onRefreshRecommendation}
                className="shrink-0 text-[var(--color-accent)] hover:underline cursor-pointer font-medium"
              >
                重试
              </button>
            )}
          </div>
        </FormItem>

        {recommendation && (
          <p className="text-xs text-[var(--color-text-muted)]">
            版本：{recommendation.releaseTag} · 发布：{recommendation.publishedAt.slice(0, 10) || '未知'}
          </p>
        )}

        <FormItem label="下载代理设置">
          <select
            value={form.proxyMode}
            onChange={e => {
              const mode = e.target.value
              setForm(prev => ({
                ...prev,
                proxyMode: mode,
                proxyId: mode === 'custom' && proxies.length > 0 ? proxies[0].proxyId : '',
              }))
            }}
            className="w-full h-9 px-3 rounded-md border border-[var(--color-border-default)] bg-[var(--color-bg-primary)] text-[var(--color-text-primary)] text-sm focus:outline-none focus:ring-1 focus:ring-[var(--color-accent)] focus:border-[var(--color-accent)]"
            disabled={progress !== null}
          >
            <option value="system">跟随系统全局代理</option>
            <option value="direct">直连模式 (不使用代理)</option>
            {proxies.length > 0 && <option value="custom">指定应用代理配置...</option>}
          </select>
        </FormItem>

        {form.proxyMode === 'custom' && (
          <FormItem label="选择代理池节点" required>
            <select
              value={form.proxyId}
              onChange={e => setForm(prev => ({ ...prev, proxyId: e.target.value }))}
              className="w-full h-9 px-3 rounded-md border border-[var(--color-border-default)] bg-[var(--color-bg-primary)] text-[var(--color-text-primary)] text-sm focus:outline-none focus:ring-1 focus:ring-[var(--color-accent)] focus:border-[var(--color-accent)]"
              disabled={progress !== null}
            >
              {proxies.map(proxy => (
                <option key={proxy.proxyId} value={proxy.proxyId}>
                  {proxy.proxyName} ({proxy.proxyConfig})
                </option>
              ))}
            </select>
          </FormItem>
        )}

        {progress && (
          <div className="mt-4 p-4 border border-[var(--color-border-default)] rounded-lg bg-[var(--color-bg-secondary)]">
            <div className="flex justify-between text-sm mb-2">
              <span className="font-medium text-[var(--color-text-primary)]">{progress.message}</span>
              <span className="text-[var(--color-text-muted)]">{progress.progress}%</span>
            </div>
            <div className="w-full bg-[var(--color-bg-surface)] rounded-full h-2 overflow-hidden border border-[var(--color-border-muted)]">
              <div
                className="bg-[var(--color-accent)] h-2 rounded-full transition-all duration-300"
                style={{ width: `${Math.max(0, Math.min(100, progress.progress))}%` }}
              />
            </div>
          </div>
        )}
      </div>
    </Modal>
  )
}
