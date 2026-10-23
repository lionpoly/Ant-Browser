package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/database"
	"path/filepath"
	"testing"
)

func TestDeleteGroupUpdatesLiveProfileCache(t *testing.T) {
	db, err := database.NewDB(filepath.Join(t.TempDir(), "group.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	dao := browser.NewSQLiteGroupDAO(db.GetConn())
	group, err := dao.Create(browser.GroupInput{GroupName: "temporary"})
	if err != nil {
		t.Fatal(err)
	}
	profile := &browser.Profile{ProfileId: "synthetic", GroupId: group.GroupId, Running: true}
	app := &App{browserMgr: &browser.Manager{GroupDAO: dao, Profiles: map[string]*browser.Profile{profile.ProfileId: profile}}}
	if err = app.DeleteGroup(group.GroupId); err != nil {
		t.Fatal(err)
	}
	if profile.GroupId != "" || !profile.Running {
		t.Fatal("live instance cache must move group without stopping instance")
	}
}
