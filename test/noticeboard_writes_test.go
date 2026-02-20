package test

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

// createTestNoticeboard creates a noticeboard for testing and returns its ID
func createTestNoticeboard(t *testing.T, name string) uint64 {
	db := database.DBConn

	result := db.Exec("INSERT INTO noticeboards (name, lat, lng, active, added) VALUES (?, 55.9533, -3.1883, 1, NOW())", name)
	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create noticeboard: %v", result.Error)
	}

	var id uint64
	db.Raw("SELECT id FROM noticeboards WHERE name = ? ORDER BY id DESC LIMIT 1", name).Scan(&id)

	if id == 0 {
		t.Fatalf("ERROR: Noticeboard was created but ID not found")
	}

	return id
}

func TestDeleteNoticeboard(t *testing.T) {
	prefix := uniquePrefix("nb_del")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// We need systemrole to be Moderator for the check
	db := database.DBConn
	db.Exec("UPDATE users SET systemrole = 'Moderator' WHERE id = ?", modID)

	nbID := createTestNoticeboard(t, "Test NB "+prefix)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/noticeboard/%d?jwt=%s", nbID, modToken), nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Verify deleted
	var count int64
	db.Raw("SELECT COUNT(*) FROM noticeboards WHERE id = ?", nbID).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestDeleteNoticeboardUnauthorized(t *testing.T) {
	prefix := uniquePrefix("nb_delua")
	nbID := createTestNoticeboard(t, "Test NB "+prefix)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/noticeboard/%d", nbID), nil))
	assert.Equal(t, 401, resp.StatusCode)
}

func TestDeleteNoticeboardNotMod(t *testing.T) {
	prefix := uniquePrefix("nb_delnm")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	nbID := createTestNoticeboard(t, "Test NB "+prefix)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/noticeboard/%d?jwt=%s", nbID, token), nil))
	assert.Equal(t, 403, resp.StatusCode)

	// Verify NOT deleted
	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM noticeboards WHERE id = ?", nbID).Scan(&count)
	assert.Equal(t, int64(1), count)
}
