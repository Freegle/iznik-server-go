package test

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func TestDeleteNoticeboard(t *testing.T) {
	prefix := uniquePrefix("nb_del")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// We need systemrole to be Moderator for the check
	db := database.DBConn
	db.Exec("UPDATE users SET systemrole = 'Moderator' WHERE id = ?", modID)

	nbID := createTestNoticeboard(t, modID)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/noticeboard/%d?jwt=%s", nbID, modToken), nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Verify deleted
	var count int64
	db.Raw("SELECT COUNT(*) FROM noticeboards WHERE id = ?", nbID).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestDeleteNoticeboardUnauthorized(t *testing.T) {
	prefix := uniquePrefix("nb_delua")
	userID := CreateTestUser(t, prefix, "User")
	nbID := createTestNoticeboard(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/noticeboard/%d", nbID), nil))
	assert.Equal(t, 401, resp.StatusCode)
}

func TestDeleteNoticeboardNotMod(t *testing.T) {
	prefix := uniquePrefix("nb_delnm")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	nbID := createTestNoticeboard(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/noticeboard/%d?jwt=%s", nbID, token), nil))
	assert.Equal(t, 403, resp.StatusCode)

	// Verify NOT deleted
	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM noticeboards WHERE id = ?", nbID).Scan(&count)
	assert.Equal(t, int64(1), count)
}
