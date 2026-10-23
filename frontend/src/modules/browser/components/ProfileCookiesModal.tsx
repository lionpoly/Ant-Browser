import { useEffect, useState } from 'react'
import { Button, Modal, toast } from '../../../shared/components'
import type { BrowserProfile } from '../types'
import { fetchBrowserProfiles, startBrowserInstance } from '../api'
import { warmupProfileProxyBeforeStart } from '../utils/proxyWarmup'
import { CookieManagerCard } from './CookieManagerCard'

export function ProfileCookiesModal({ open, profileId, onClose }: { open: boolean; profileId: string; onClose: () => void }) {
  const [profile, setProfile] = useState<BrowserProfile | null>(null)
  const [starting, setStarting] = useState(false)
  const [loadError, setLoadError] = useState('')

  useEffect(() => {
    if (!open || !profileId) return
    let active = true
    let inFlight = false
    setProfile(null)
    setLoadError('')
    const refresh = async () => {
      if (inFlight) return
      inFlight = true
      try {
        const list = await fetchBrowserProfiles()
        if (active) {
          setProfile(list.find(item => item.profileId === profileId) || null)
          setLoadError(list.some(item => item.profileId === profileId) ? '' : '实例不存在，请返回列表刷新')
        }
      } catch { if (active) setLoadError('读取实例状态失败，请稍后重试') }
      finally { inFlight = false }
    }
    void refresh()
    const timer = window.setInterval(() => void refresh(), 3000)
    return () => { active = false; window.clearInterval(timer) }
  }, [open, profileId])

  const start = async () => {
    if (!profile) return
    setStarting(true)
    try {
      await warmupProfileProxyBeforeStart(profile)
      const current = await startBrowserInstance(profileId)
      if (current) setProfile(current)
    } catch (error: any) { toast.error(error?.message || '启动实例失败') }
    finally { setStarting(false) }
  }

  return <Modal open={open} onClose={onClose} title={`Cookies 管理${profile ? ` · ${profile.profileName}` : ''}`} width="1000px">
    {loadError && <p className="text-sm text-[var(--color-error)]">{loadError}</p>}
    {open && profile?.profileId === profileId ? <div className="space-y-3">
      {!profile.running && <div className="flex items-center justify-between gap-3">
        <span className="text-sm text-[var(--color-text-muted)]">启动已保存的实例后，可以查看、导入、导出及清除 Cookies。</span>
        <Button onClick={() => void start()} loading={starting}>启动实例</Button>
      </div>}
      <CookieManagerCard key={profileId} profileId={profileId} profileName={profile.profileName} running={profile.running} ready={profile.running && profile.debugReady && !loadError} />
    </div> : !loadError && <p className="text-sm text-[var(--color-text-muted)]">正在读取实例状态…</p>}
  </Modal>
}
