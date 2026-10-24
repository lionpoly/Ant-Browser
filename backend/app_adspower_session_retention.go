package backend

import (
	"ant-chrome/backend/internal/browser"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Older AdsPower imports inherited the global default (off), which causes
// Chromium to discard session cookies and tabs on the next startup. Upgrade
// only imported profiles still in follow mode; explicit user choices win.
// Called with browserMgr.Mutex held, before session files can be removed.
func (a *App) ensureAdsPowerSessionRetention(profile *BrowserProfile, userDataDir string) error {
	if browser.NormalizeRestoreLastSessionMode(profile.RestoreLastSession) != browser.RestoreLastSessionFollow {
		return nil
	}
	info, err := os.Stat(filepath.Join(userDataDir, adsPowerMetadataPath))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("实例启动已阻止：无法确认导入实例的会话保留设置，请检查导入元数据")
	}
	previousMode, previousUpdated := profile.RestoreLastSession, profile.UpdatedAt
	previousProfiles := a.config.Browser.Profiles
	profile.RestoreLastSession = browser.RestoreLastSessionEnabled
	profile.UpdatedAt = time.Now().Format(time.RFC3339)
	if a.browserMgr.ProfileDAO != nil {
		err = a.browserMgr.ProfileDAO.Upsert(profile)
	} else {
		err = a.browserMgr.SaveProfiles()
	}
	if err != nil {
		profile.RestoreLastSession, profile.UpdatedAt = previousMode, previousUpdated
		a.config.Browser.Profiles = previousProfiles
		return fmt.Errorf("实例启动已阻止：导入实例的会话保留设置保存失败，请检查存储权限后重试")
	}
	return nil
}
