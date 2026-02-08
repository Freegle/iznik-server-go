package test

import (
	"bytes"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestNewsfeedLove(t *testing.T) {
	prefix := uniquePrefix("nfwr_love")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	nfID := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Test love "+prefix)

	body := fmt.Sprintf(`{"id":%d,"action":"Love"}`, nfID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify like exists
	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM newsfeed_likes WHERE newsfeedid = ? AND userid = ?", nfID, userID).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestNewsfeedUnlove(t *testing.T) {
	prefix := uniquePrefix("nfwr_unlove")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	nfID := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Test unlove "+prefix)

	// Love first
	db := database.DBConn
	db.Exec("INSERT IGNORE INTO newsfeed_likes (newsfeedid, userid) VALUES (?, ?)", nfID, userID)

	body := fmt.Sprintf(`{"id":%d,"action":"Unlove"}`, nfID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify like removed
	var count int64
	db.Raw("SELECT COUNT(*) FROM newsfeed_likes WHERE newsfeedid = ? AND userid = ?", nfID, userID).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestNewsfeedSeen(t *testing.T) {
	prefix := uniquePrefix("nfwr_seen")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	nfID := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Test seen "+prefix)

	body := fmt.Sprintf(`{"id":%d,"action":"Seen"}`, nfID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify seen record
	db := database.DBConn
	var seenID uint64
	db.Raw("SELECT newsfeedid FROM newsfeed_users WHERE userid = ?", userID).Scan(&seenID)
	assert.Equal(t, nfID, seenID)
}

func TestNewsfeedFollow(t *testing.T) {
	prefix := uniquePrefix("nfwr_follow")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	nfID := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Test follow "+prefix)

	// Unfollow first
	db := database.DBConn
	db.Exec("REPLACE INTO newsfeed_unfollow (userid, newsfeedid) VALUES (?, ?)", userID, nfID)

	body := fmt.Sprintf(`{"id":%d,"action":"Follow"}`, nfID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify unfollow removed
	var count int64
	db.Raw("SELECT COUNT(*) FROM newsfeed_unfollow WHERE userid = ? AND newsfeedid = ?", userID, nfID).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestNewsfeedUnfollow(t *testing.T) {
	prefix := uniquePrefix("nfwr_unfollow")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	nfID := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Test unfollow "+prefix)

	body := fmt.Sprintf(`{"id":%d,"action":"Unfollow"}`, nfID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify unfollow record
	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM newsfeed_unfollow WHERE userid = ? AND newsfeedid = ?", userID, nfID).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestNewsfeedReport(t *testing.T) {
	prefix := uniquePrefix("nfwr_report")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	nfID := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Test report "+prefix)

	body := fmt.Sprintf(`{"id":%d,"action":"Report","reason":"Inappropriate content"}`, nfID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify report and reviewrequired flag
	db := database.DBConn
	var reviewRequired int
	db.Raw("SELECT reviewrequired FROM newsfeed WHERE id = ?", nfID).Scan(&reviewRequired)
	assert.Equal(t, 1, reviewRequired)

	var reportCount int64
	db.Raw("SELECT COUNT(*) FROM newsfeed_reports WHERE newsfeedid = ? AND userid = ?", nfID, userID).Scan(&reportCount)
	assert.Equal(t, int64(1), reportCount)
}

func TestNewsfeedHide(t *testing.T) {
	prefix := uniquePrefix("nfwr_hide")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Moderator")
	_, token := CreateTestSession(t, userID)
	nfID := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Test hide "+prefix)

	body := fmt.Sprintf(`{"id":%d,"action":"Hide"}`, nfID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify hidden
	db := database.DBConn
	var hiddenby *uint64
	db.Raw("SELECT hiddenby FROM newsfeed WHERE id = ?", nfID).Scan(&hiddenby)
	assert.NotNil(t, hiddenby)
}

func TestNewsfeedUnhide(t *testing.T) {
	prefix := uniquePrefix("nfwr_unhide")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Moderator")
	_, token := CreateTestSession(t, userID)
	nfID := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Test unhide "+prefix)

	// Hide first
	db := database.DBConn
	db.Exec("UPDATE newsfeed SET hidden = NOW(), hiddenby = ? WHERE id = ?", userID, nfID)

	body := fmt.Sprintf(`{"id":%d,"action":"Unhide"}`, nfID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify unhidden
	var hiddenby *uint64
	db.Raw("SELECT hiddenby FROM newsfeed WHERE id = ?", nfID).Scan(&hiddenby)
	assert.Nil(t, hiddenby)
}

func TestNewsfeedEdit(t *testing.T) {
	prefix := uniquePrefix("nfwr_edit")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	nfID := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Test edit "+prefix)

	body := fmt.Sprintf(`{"id":%d,"message":"Updated message %s"}`, nfID, prefix)
	req := httptest.NewRequest("PATCH", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify update
	db := database.DBConn
	var message string
	db.Raw("SELECT message FROM newsfeed WHERE id = ?", nfID).Scan(&message)
	assert.Equal(t, "Updated message "+prefix, message)
}

func TestNewsfeedEditUnauthorized(t *testing.T) {
	body := `{"id":1,"message":"Hacked"}`
	req := httptest.NewRequest("PATCH", "/api/newsfeed", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestNewsfeedEditNonOwner(t *testing.T) {
	prefix := uniquePrefix("nfwr_edno")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, otherToken := CreateTestSession(t, otherID)
	nfID := CreateTestNewsfeed(t, ownerID, 52.2, -0.1, "Test edit noown "+prefix)

	body := fmt.Sprintf(`{"id":%d,"message":"Hacked"}`, nfID)
	req := httptest.NewRequest("PATCH", "/api/newsfeed?jwt="+otherToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestNewsfeedDelete(t *testing.T) {
	prefix := uniquePrefix("nfwr_del")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	nfID := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Test delete "+prefix)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/newsfeed/%d?jwt=%s", nfID, token), nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Verify soft-deleted
	db := database.DBConn
	var deleted *string
	db.Raw("SELECT deleted FROM newsfeed WHERE id = ?", nfID).Scan(&deleted)
	assert.NotNil(t, deleted)
}

func TestNewsfeedDeleteUnauthorized(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("DELETE", "/api/newsfeed/1", nil))
	assert.Equal(t, 401, resp.StatusCode)
}

func TestNewsfeedDeleteNonOwner(t *testing.T) {
	prefix := uniquePrefix("nfwr_dno")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, otherToken := CreateTestSession(t, otherID)
	nfID := CreateTestNewsfeed(t, ownerID, 52.2, -0.1, "Test del noown "+prefix)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/newsfeed/%d?jwt=%s", nfID, otherToken), nil))
	assert.Equal(t, 403, resp.StatusCode)
}

func TestNewsfeedPostUnauthorized(t *testing.T) {
	body := `{"action":"Love","id":1}`
	req := httptest.NewRequest("POST", "/api/newsfeed", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}
