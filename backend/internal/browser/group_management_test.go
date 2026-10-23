package browser

import (
	"ant-chrome/backend/internal/database"
	"path/filepath"
	"testing"
)

func TestGroupDeletePreservesInstancesAndRollsBackFailures(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rollback"}[fail], func(t *testing.T) {
			db, err := database.NewDB(filepath.Join(t.TempDir(), "groups.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			conn := db.GetConn()
			for _, query := range []string{
				`CREATE TABLE browser_groups(group_id TEXT PRIMARY KEY,group_name TEXT,parent_id TEXT,sort_order INTEGER,created_at TEXT,updated_at TEXT)`,
				`CREATE TABLE browser_profiles(profile_id TEXT PRIMARY KEY,group_id TEXT)`,
			} {
				if _, err = conn.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			dao := NewSQLiteGroupDAO(conn)
			parent, _ := dao.Create(GroupInput{GroupName: "parent"})
			group, _ := dao.Create(GroupInput{GroupName: "group", ParentId: parent.GroupId})
			child, _ := dao.Create(GroupInput{GroupName: "child", ParentId: group.GroupId})
			if _, err = conn.Exec(`INSERT INTO browser_profiles VALUES('instance',?)`, group.GroupId); err != nil {
				t.Fatal(err)
			}
			if fail {
				if _, err = conn.Exec(`CREATE TRIGGER fail_delete BEFORE DELETE ON browser_groups BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
					t.Fatal(err)
				}
			}
			err = dao.Delete(group.GroupId)
			if (err != nil) != fail {
				t.Fatalf("unexpected deletion result: %v", err)
			}
			expected := parent.GroupId
			if fail {
				expected = group.GroupId
			}
			restored, err := dao.GetById(child.GroupId)
			if err != nil || restored.ParentId != expected {
				t.Fatal("child moved despite failed deletion")
			}
			var instanceGroup string
			if err = conn.QueryRow(`SELECT group_id FROM browser_profiles WHERE profile_id='instance'`).Scan(&instanceGroup); err != nil || instanceGroup != expected {
				t.Fatal("instance lost or assigned incorrectly")
			}
		})
	}
}
