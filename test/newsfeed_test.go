package test

import (
	"bytes"
	json2 "encoding/json"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	newsfeed2 "github.com/freegle/iznik-server-go/newsfeed"
	"github.com/freegle/iznik-server-go/newsfeed"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/stretchr/testify/assert"
)

func TestFeed(t *testing.T) {
	// Get logged out - should return 401
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/newsfeed", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Create a full test user with newsfeed entry
	prefix := uniquePrefix("feed")
	userID, token := CreateFullTestUser(t, prefix)

	// Create a newsfeed entry for this user
	lat := 55.9533
	lng := -3.1883
	message := fmt.Sprintf("Test newsfeed message %s", prefix)
	newsfeedID := CreateTestNewsfeed(t, userID, lat, lng, message)

	// Should be able to get feed for a user
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/newsfeed?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var newsfeed []newsfeed2.Newsfeed
	json2.Unmarshal(rsp(resp), &newsfeed)
	assert.Greater(t, len(newsfeed), 0)

	// Get with distance
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/newsfeed?distance=10000&jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &newsfeed)
	assert.Greater(t, len(newsfeed), 0)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/newsfeed?distance=0&jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &newsfeed)
	assert.Greater(t, len(newsfeed), 0)

	// Get the specific newsfeed entry we created
	id := strconv.FormatUint(newsfeedID, 10)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/newsfeed/"+id, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var single newsfeed2.Newsfeed
	json2.Unmarshal(rsp(resp), &single)
	assert.Greater(t, single.ID, uint64(0))

	// Non-existent newsfeed should return 404
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/newsfeed/-1", nil))
	assert.Equal(t, 404, resp.StatusCode)

	// Get count - requires auth
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/newsfeedcount", nil))
	assert.Equal(t, 401, resp.StatusCode)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/newsfeedcount?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestNewsfeed_InvalidID(t *testing.T) {
	// Non-integer ID
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/newsfeed/notanint", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestNewsfeed_SingleWithAuth(t *testing.T) {
	// Single newsfeed with auth should also work
	prefix := uniquePrefix("feedsingleauth")
	userID, token := CreateFullTestUser(t, prefix)
	lat := 55.9533
	lng := -3.1883
	newsfeedID := CreateTestNewsfeed(t, userID, lat, lng, "Test single auth "+prefix)

	id := strconv.FormatUint(newsfeedID, 10)
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/newsfeed/"+id+"?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var single newsfeed2.Newsfeed
	json2.Unmarshal(rsp(resp), &single)
	assert.Equal(t, newsfeedID, single.ID)
}

func TestNewsfeed_V2Path(t *testing.T) {
	prefix := uniquePrefix("feedv2")
	_, token := CreateFullTestUser(t, prefix)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/newsfeed?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
}

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

func TestNewsfeedSeenHigherIDGuard(t *testing.T) {
	prefix := uniquePrefix("nfwr_seeng")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	nfID1 := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Test seen guard 1 "+prefix)
	nfID2 := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Test seen guard 2 "+prefix)

	// Ensure nfID2 > nfID1
	assert.Greater(t, nfID2, nfID1)

	// Mark higher ID as seen first
	body := fmt.Sprintf(`{"id":%d,"action":"Seen"}`, nfID2)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	db := database.DBConn
	var seenID uint64
	db.Raw("SELECT newsfeedid FROM newsfeed_users WHERE userid = ?", userID).Scan(&seenID)
	assert.Equal(t, nfID2, seenID)

	// Now try to mark lower ID as seen - should NOT overwrite
	body = fmt.Sprintf(`{"id":%d,"action":"Seen"}`, nfID1)
	req = httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify seen record still points to higher ID
	db.Raw("SELECT newsfeedid FROM newsfeed_users WHERE userid = ?", userID).Scan(&seenID)
	assert.Equal(t, nfID2, seenID, "Lower ID should not overwrite higher seen ID")
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

	// Verify background task was queued for email_chitchat_report
	var taskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = 'email_chitchat_report' AND processed_at IS NULL AND data LIKE ?",
		fmt.Sprintf("%%\"newsfeed_id\": %d%%", nfID)).Scan(&taskCount)
	assert.Equal(t, int64(1), taskCount, "Expected email_chitchat_report task to be queued")
}

func TestNewsfeedHide(t *testing.T) {
	prefix := uniquePrefix("nfwr_hide")
	userID := CreateTestUser(t, prefix, "Admin")
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
	userID := CreateTestUser(t, prefix, "Admin")
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

func TestNewsfeedHidePermissionDenied(t *testing.T) {
	prefix := uniquePrefix("nfwr_hdeny")
	// Regular Moderator should NOT be able to hide - requires Admin/Support or ChitChat team
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Moderator")
	_, token := CreateTestSession(t, userID)
	nfID := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Test hide deny "+prefix)

	body := fmt.Sprintf(`{"id":%d,"action":"Hide"}`, nfID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestNewsfeedHideChitChatTeam(t *testing.T) {
	prefix := uniquePrefix("nfwr_hteam")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	nfID := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Test hide team "+prefix)

	// Add user to ChitChat Moderation team
	db := database.DBConn
	var teamID uint64
	db.Raw("SELECT id FROM teams WHERE name = 'ChitChat Moderation'").Scan(&teamID)
	if teamID == 0 {
		db.Exec("INSERT INTO teams (name) VALUES ('ChitChat Moderation')")
		db.Raw("SELECT LAST_INSERT_ID()").Scan(&teamID)
	}
	db.Exec("INSERT INTO teams_members (teamid, userid) VALUES (?, ?)", teamID, userID)

	body := fmt.Sprintf(`{"id":%d,"action":"Hide"}`, nfID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify hidden
	var hiddenby *uint64
	db.Raw("SELECT hiddenby FROM newsfeed WHERE id = ?", nfID).Scan(&hiddenby)
	assert.NotNil(t, hiddenby)
}

func TestNewsfeedCreatePost(t *testing.T) {
	prefix := uniquePrefix("nfwr_creat")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// User needs a location for post creation
	db := database.DBConn
	db.Exec("INSERT INTO locations (name, lat, lng, type) VALUES (?, 52.2, -0.1, 'Polygon')", "TestLoc_"+prefix)
	var locID uint64
	db.Raw("SELECT id FROM locations WHERE name = ? ORDER BY id DESC LIMIT 1", "TestLoc_"+prefix).Scan(&locID)
	if locID > 0 {
		db.Exec("UPDATE users SET lastlocation = ? WHERE id = ?", locID, userID)
	}

	body := fmt.Sprintf(`{"message":"Hello chitchat %s"}`, prefix)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify the post was created
	var nfID uint64
	db.Raw("SELECT id FROM newsfeed WHERE userid = ? AND message = ? ORDER BY id DESC LIMIT 1",
		userID, "Hello chitchat "+prefix).Scan(&nfID)
	assert.NotZero(t, nfID)
}

func TestNewsfeedCreateReply(t *testing.T) {
	prefix := uniquePrefix("nfwr_reply")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	nfID := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Thread head "+prefix)

	body := fmt.Sprintf(`{"message":"Reply to thread %s","replyto":%d}`, prefix, nfID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify reply was created
	db := database.DBConn
	var replyID uint64
	db.Raw("SELECT id FROM newsfeed WHERE replyto = ? AND userid = ? ORDER BY id DESC LIMIT 1",
		nfID, userID).Scan(&replyID)
	assert.NotZero(t, replyID)
}

func TestNewsfeedReplyNotifiesThreadContributors(t *testing.T) {
	prefix := uniquePrefix("nfwr_rnotif")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	replierID := CreateTestUser(t, prefix+"_replier", "User")
	_, replierToken := CreateTestSession(t, replierID)

	// Owner creates a thread
	nfID := CreateTestNewsfeed(t, ownerID, 52.2, -0.1, "Thread head "+prefix)

	// Replier posts a reply to the thread
	body := fmt.Sprintf(`{"message":"Reply to thread %s","replyto":%d}`, prefix, nfID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+replierToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Owner should receive a CommentOnYourPost notification
	db := database.DBConn
	var notifCount int64
	db.Raw("SELECT COUNT(*) FROM users_notifications WHERE fromuser = ? AND touser = ? AND type = 'CommentOnYourPost' AND newsfeedid = ?",
		replierID, ownerID, nfID).Scan(&notifCount)
	assert.Equal(t, int64(1), notifCount, "owner should receive CommentOnYourPost notification")
}

func TestNewsfeedLoveNotification(t *testing.T) {
	prefix := uniquePrefix("nfwr_loven")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	loverID := CreateTestUser(t, prefix+"_lover", "User")
	_, loverToken := CreateTestSession(t, loverID)
	nfID := CreateTestNewsfeed(t, ownerID, 52.2, -0.1, "Test love notif "+prefix)

	body := fmt.Sprintf(`{"id":%d,"action":"Love"}`, nfID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+loverToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify notification was created for the post owner
	db := database.DBConn
	var notifCount int64
	db.Raw("SELECT COUNT(*) FROM users_notifications WHERE fromuser = ? AND touser = ? AND type = 'LovedPost' AND newsfeedid = ?",
		loverID, ownerID, nfID).Scan(&notifCount)
	assert.Equal(t, int64(1), notifCount)
}

func TestNewsfeedLoveCommentNotification(t *testing.T) {
	prefix := uniquePrefix("nfwr_lovecn")
	postOwnerID := CreateTestUser(t, prefix+"_post", "User")
	replyOwnerID := CreateTestUser(t, prefix+"_reply", "User")
	loverID := CreateTestUser(t, prefix+"_lover", "User")
	_, loverToken := CreateTestSession(t, loverID)

	// Create a top-level post and a reply to it
	nfID := CreateTestNewsfeed(t, postOwnerID, 52.2, -0.1, "Thread head "+prefix)
	db := database.DBConn
	db.Exec(fmt.Sprintf("INSERT INTO newsfeed (userid, message, type, replyto, timestamp, deleted, reviewrequired, position, hidden, pinned) "+
		"VALUES (?, ?, 'Message', ?, NOW(), NULL, 0, ST_GeomFromText(?, %d), NULL, 0)", utils.SRID),
		replyOwnerID, "Reply "+prefix, nfID, fmt.Sprintf("POINT(%f %f)", -0.1, 52.2))
	var replyID uint64
	db.Raw("SELECT id FROM newsfeed WHERE replyto = ? AND userid = ? ORDER BY id DESC LIMIT 1",
		nfID, replyOwnerID).Scan(&replyID)
	assert.NotZero(t, replyID)

	// Love the reply
	body := fmt.Sprintf(`{"id":%d,"action":"Love"}`, replyID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+loverToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify notification type is LovedComment (not LovedPost)
	var notifCount int64
	db.Raw("SELECT COUNT(*) FROM users_notifications WHERE fromuser = ? AND touser = ? AND type = 'LovedComment' AND newsfeedid = ?",
		loverID, replyOwnerID, replyID).Scan(&notifCount)
	assert.Equal(t, int64(1), notifCount)

	// Verify NO LovedPost notification was created
	var postNotifCount int64
	db.Raw("SELECT COUNT(*) FROM users_notifications WHERE fromuser = ? AND touser = ? AND type = 'LovedPost' AND newsfeedid = ?",
		loverID, replyOwnerID, replyID).Scan(&postNotifCount)
	assert.Equal(t, int64(0), postNotifCount)
}

func TestNewsfeedUnknownAction(t *testing.T) {
	prefix := uniquePrefix("nfwr_unkact")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{"id":1,"action":"InvalidAction"}`
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

// --- Adversarial tests ---

func TestNewsfeedSeenZeroID(t *testing.T) {
	// Seen with ID=0 should not corrupt newsfeed_users table.
	prefix := uniquePrefix("nfwr_seen0")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{"id":0,"action":"Seen"}`
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Should not have created a record with newsfeedid=0.
	var count int64
	db := database.DBConn
	db.Raw("SELECT COUNT(*) FROM newsfeed_users WHERE userid = ? AND newsfeedid = 0", userID).Scan(&count)
	assert.Equal(t, int64(0), count, "Seen with ID=0 should not create a record")
}

func TestNewsfeedDoubleLove(t *testing.T) {
	// Love twice should be idempotent (ON DUPLICATE KEY).
	prefix := uniquePrefix("nfwr_dblluv")
	db := database.DBConn
	userID, token := CreateFullTestUser(t, prefix)
	nfID := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Test double love "+prefix)

	body := fmt.Sprintf(`{"id":%d,"action":"Love"}`, nfID)

	// First Love.
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Second Love - should succeed without error.
	req = httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Should still be exactly one love.
	var loveCount int64
	db.Raw("SELECT COUNT(*) FROM newsfeed_likes WHERE newsfeedid = ? AND userid = ?", nfID, userID).Scan(&loveCount)
	assert.Equal(t, int64(1), loveCount)
}

func TestNewsfeedEmptyBody(t *testing.T) {
	prefix := uniquePrefix("nfwr_empty")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	// Empty body has no action - should return 400.
	assert.Equal(t, 400, resp.StatusCode)
}

func TestNewsfeedLoveNonExistent(t *testing.T) {
	// Love a non-existent newsfeed item should handle gracefully.
	prefix := uniquePrefix("nfwr_luv_ne")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{"id":999999999,"action":"Love"}`
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	// Should succeed or return 404, but not crash.
	assert.Contains(t, []int{200, 404}, resp.StatusCode)
}

func TestConvertToStory(t *testing.T) {
	prefix := uniquePrefix("nf_c2s")
	// Create a moderator user
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Create a newsfeed entry by another user
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	nfID := CreateTestNewsfeed(t, posterID, 52.2, -0.1, "My freegle story "+prefix)

	body := fmt.Sprintf(`{"id":%d,"action":"ConvertToStory"}`, nfID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Parse response and verify story ID is returned
	var result map[string]interface{}
	json.Unmarshal(rsp(resp), &result)
	storyID := result["id"]
	assert.NotNil(t, storyID, "Response should contain story id")
	assert.NotZero(t, storyID)

	// Verify the story was created in the database
	db := database.DBConn
	var storyText string
	var storyUserID uint64
	var fromNewsfeed int
	db.Raw("SELECT story, userid, fromnewsfeed FROM users_stories WHERE id = ?", uint64(storyID.(float64))).Row().Scan(&storyText, &storyUserID, &fromNewsfeed)
	assert.Equal(t, "My freegle story "+prefix, storyText)
	assert.Equal(t, posterID, storyUserID)
	assert.Equal(t, 1, fromNewsfeed)
}

func TestConvertToStoryNotMod(t *testing.T) {
	prefix := uniquePrefix("nf_c2s_nm")
	// Create a regular (non-mod) user
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	nfID := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Regular user story "+prefix)

	body := fmt.Sprintf(`{"id":%d,"action":"ConvertToStory"}`, nfID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestConvertToStoryUnauthorized(t *testing.T) {
	body := `{"id":1,"action":"ConvertToStory"}`
	req := httptest.NewRequest("POST", "/api/newsfeed", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestCreateNewsfeedEntryForCommunityEvent(t *testing.T) {
	prefix := uniquePrefix("nfcr_event")
	db := database.DBConn

	// Create a test user and group with known locations.
	userID := CreateTestUser(t, prefix+"_user", "User")
	groupID := CreateTestGroup(t, prefix+"_group")

	// Set group location and name.
	db.Exec("UPDATE `groups` SET lat = 52.2, lng = -0.1, nameshort = ? WHERE id = ?", prefix+"_group", groupID)

	// Clear any existing newsfeed entries for this user to avoid duplicate protection interference.
	db.Exec("DELETE FROM newsfeed WHERE userid = ?", userID)

	// Create a test community event.
	db.Exec("INSERT INTO communityevents (userid, title, description, location, pending, deleted) VALUES (?, ?, 'Test event', 'Test location', 0, 0)",
		userID, "Test Event "+prefix)
	var eventID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND title = ? ORDER BY id DESC LIMIT 1",
		userID, "Test Event "+prefix).Scan(&eventID)
	assert.NotZero(t, eventID)

	// Create newsfeed entry.
	nfID, err := newsfeed.CreateNewsfeedEntry(
		newsfeed.TypeCommunityEvent,
		userID,
		groupID,
		&eventID,
		nil,
	)

	assert.NoError(t, err)
	assert.NotZero(t, nfID)

	// Verify the newsfeed entry.
	type NfEntry struct {
		Type     string  `json:"type"`
		Userid   uint64  `json:"userid"`
		Groupid  uint64  `json:"groupid"`
		Eventid  *uint64 `json:"eventid"`
		Location *string `json:"location"`
		Hidden   *string `json:"hidden"`
	}
	var entry NfEntry
	db.Raw("SELECT type, userid, groupid, eventid, location, hidden FROM newsfeed WHERE id = ?", nfID).Scan(&entry)

	assert.Equal(t, newsfeed.TypeCommunityEvent, entry.Type)
	assert.Equal(t, userID, entry.Userid)
	assert.Equal(t, groupID, entry.Groupid)
	assert.NotNil(t, entry.Eventid)
	assert.Equal(t, eventID, *entry.Eventid)

	// Verify location was set from group name.
	assert.NotNil(t, entry.Location)
	assert.Equal(t, prefix+"_group", *entry.Location)

	// Verify hidden is NULL for non-suppressed user.
	assert.Nil(t, entry.Hidden)

	// Clean up.
	db.Exec("DELETE FROM newsfeed WHERE id = ?", nfID)
	db.Exec("DELETE FROM communityevents WHERE id = ?", eventID)
}

func TestCreateNewsfeedEntryForVolunteering(t *testing.T) {
	prefix := uniquePrefix("nfcr_vol")
	db := database.DBConn

	// Create a test user and group with known locations.
	userID := CreateTestUser(t, prefix+"_user", "User")
	groupID := CreateTestGroup(t, prefix+"_group")

	// Set group location.
	db.Exec("UPDATE `groups` SET lat = 53.5, lng = -1.5 WHERE id = ?", groupID)

	// Clear any existing newsfeed entries for this user to avoid duplicate protection interference.
	db.Exec("DELETE FROM newsfeed WHERE userid = ?", userID)

	// Create a test volunteering opportunity.
	db.Exec("INSERT INTO volunteering (userid, title, description, location, pending, expired, deleted) VALUES (?, ?, 'Help needed', 'Test location', 0, 0, 0)",
		userID, "Test Volunteering "+prefix)
	var volID uint64
	db.Raw("SELECT id FROM volunteering WHERE userid = ? AND title = ? ORDER BY id DESC LIMIT 1",
		userID, "Test Volunteering "+prefix).Scan(&volID)
	assert.NotZero(t, volID)

	// Create newsfeed entry.
	nfID, err := newsfeed.CreateNewsfeedEntry(
		newsfeed.TypeVolunteerOpportunity,
		userID,
		groupID,
		nil,
		&volID,
	)

	assert.NoError(t, err)
	assert.NotZero(t, nfID)

	// Verify the newsfeed entry.
	type NfEntry struct {
		Type           string  `json:"type"`
		Userid         uint64  `json:"userid"`
		Groupid        uint64  `json:"groupid"`
		Volunteeringid *uint64 `json:"volunteeringid"`
	}
	var entry NfEntry
	db.Raw("SELECT type, userid, groupid, volunteeringid FROM newsfeed WHERE id = ?", nfID).Scan(&entry)

	assert.Equal(t, newsfeed.TypeVolunteerOpportunity, entry.Type)
	assert.Equal(t, userID, entry.Userid)
	assert.Equal(t, groupID, entry.Groupid)
	assert.NotNil(t, entry.Volunteeringid)
	assert.Equal(t, volID, *entry.Volunteeringid)

	// Clean up.
	db.Exec("DELETE FROM newsfeed WHERE id = ?", nfID)
	db.Exec("DELETE FROM volunteering WHERE id = ?", volID)
}

func TestCreateNewsfeedEntryNoLocation(t *testing.T) {
	prefix := uniquePrefix("nfcr_noloc")
	db := database.DBConn

	// Create user and group with NO location.
	userID := CreateTestUser(t, prefix+"_user", "User")
	groupID := CreateTestGroup(t, prefix+"_group")

	// Ensure no location is set.
	db.Exec("UPDATE `groups` SET lat = NULL, lng = NULL WHERE id = ?", groupID)
	db.Exec("UPDATE users SET lastlocation = NULL WHERE id = ?", userID)

	// Create a real community event (needed for FK constraint).
	db.Exec("INSERT INTO communityevents (userid, title, description, location, pending, deleted) VALUES (?, ?, 'Test', 'Test', 0, 0)",
		userID, "NoLoc Event "+prefix)
	var eventID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND title = ? ORDER BY id DESC LIMIT 1",
		userID, "NoLoc Event "+prefix).Scan(&eventID)
	assert.NotZero(t, eventID)

	nfID, err := newsfeed.CreateNewsfeedEntry(
		newsfeed.TypeCommunityEvent,
		userID,
		groupID,
		&eventID,
		nil,
	)

	// Should return 0 without error - can't create without location.
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), nfID)

	// Clean up.
	db.Exec("DELETE FROM communityevents WHERE id = ?", eventID)
}

func TestCreateNewsfeedEntrySuppressedUser(t *testing.T) {
	prefix := uniquePrefix("nfcr_supp")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	groupID := CreateTestGroup(t, prefix+"_group")
	db.Exec("UPDATE `groups` SET lat = 51.5, lng = -0.1 WHERE id = ?", groupID)

	// Mark user as suppressed.
	db.Exec("UPDATE users SET newsfeedmodstatus = 'Suppressed' WHERE id = ?", userID)

	// Clear any existing newsfeed entries.
	db.Exec("DELETE FROM newsfeed WHERE userid = ?", userID)

	// Create a real community event for FK constraint.
	db.Exec("INSERT INTO communityevents (userid, title, description, location, pending, deleted) VALUES (?, ?, 'Test', 'Test', 0, 0)",
		userID, "Suppressed Event "+prefix)
	var eventID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND title = ? ORDER BY id DESC LIMIT 1",
		userID, "Suppressed Event "+prefix).Scan(&eventID)
	assert.NotZero(t, eventID)

	nfID, err := newsfeed.CreateNewsfeedEntry(
		newsfeed.TypeCommunityEvent,
		userID,
		groupID,
		&eventID,
		nil,
	)

	assert.NoError(t, err)
	assert.NotZero(t, nfID)

	// Verify hidden is set (not NULL).
	var hidden *string
	db.Raw("SELECT hidden FROM newsfeed WHERE id = ?", nfID).Scan(&hidden)
	assert.NotNil(t, hidden, "Suppressed user's newsfeed entry should have hidden set")

	// Clean up.
	db.Exec("DELETE FROM newsfeed WHERE id = ?", nfID)
	db.Exec("DELETE FROM communityevents WHERE id = ?", eventID)
}

func TestCreateNewsfeedEntrySpammerUser(t *testing.T) {
	prefix := uniquePrefix("nfcr_spam")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	groupID := CreateTestGroup(t, prefix+"_group")
	db.Exec("UPDATE `groups` SET lat = 51.5, lng = -0.1 WHERE id = ?", groupID)

	// Add user to spam list.
	db.Exec("INSERT INTO spam_users (userid, collection, reason) VALUES (?, 'Spammer', 'Test spammer')", userID)

	// Clear any existing newsfeed entries.
	db.Exec("DELETE FROM newsfeed WHERE userid = ?", userID)

	// Create a real community event for FK constraint.
	db.Exec("INSERT INTO communityevents (userid, title, description, location, pending, deleted) VALUES (?, ?, 'Test', 'Test', 0, 0)",
		userID, "Spam Event "+prefix)
	var eventID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND title = ? ORDER BY id DESC LIMIT 1",
		userID, "Spam Event "+prefix).Scan(&eventID)
	assert.NotZero(t, eventID)

	nfID, err := newsfeed.CreateNewsfeedEntry(
		newsfeed.TypeCommunityEvent,
		userID,
		groupID,
		&eventID,
		nil,
	)

	assert.NoError(t, err)
	assert.NotZero(t, nfID)

	// Verify hidden is set (not NULL).
	var hidden *string
	db.Raw("SELECT hidden FROM newsfeed WHERE id = ?", nfID).Scan(&hidden)
	assert.NotNil(t, hidden, "Spammer user's newsfeed entry should have hidden set")

	// Clean up.
	db.Exec("DELETE FROM newsfeed WHERE id = ?", nfID)
	db.Exec("DELETE FROM communityevents WHERE id = ?", eventID)
	db.Exec("DELETE FROM spam_users WHERE userid = ?", userID)
}

func TestCreateNewsfeedEntryDuplicateProtection(t *testing.T) {
	prefix := uniquePrefix("nfcr_dup")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	groupID := CreateTestGroup(t, prefix+"_group")
	db.Exec("UPDATE `groups` SET lat = 51.5, lng = -0.1 WHERE id = ?", groupID)

	// Clear any existing newsfeed entries.
	db.Exec("DELETE FROM newsfeed WHERE userid = ?", userID)

	// Create a real community event for FK constraint.
	db.Exec("INSERT INTO communityevents (userid, title, description, location, pending, deleted) VALUES (?, ?, 'Test', 'Test', 0, 0)",
		userID, "Dup Event "+prefix)
	var eventID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND title = ? ORDER BY id DESC LIMIT 1",
		userID, "Dup Event "+prefix).Scan(&eventID)
	assert.NotZero(t, eventID)

	// Create a real volunteering opportunity for FK constraint.
	db.Exec("INSERT INTO volunteering (userid, title, description, location, pending, expired, deleted) VALUES (?, ?, 'Test', 'Test', 0, 0, 0)",
		userID, "Dup Vol "+prefix)
	var volID uint64
	db.Raw("SELECT id FROM volunteering WHERE userid = ? AND title = ? ORDER BY id DESC LIMIT 1",
		userID, "Dup Vol "+prefix).Scan(&volID)
	assert.NotZero(t, volID)

	// First call should succeed.
	nfID1, err := newsfeed.CreateNewsfeedEntry(
		newsfeed.TypeCommunityEvent,
		userID,
		groupID,
		&eventID,
		nil,
	)
	assert.NoError(t, err)
	assert.NotZero(t, nfID1)

	// Second call with same type should be skipped (duplicate protection).
	nfID2, err := newsfeed.CreateNewsfeedEntry(
		newsfeed.TypeCommunityEvent,
		userID,
		groupID,
		&eventID,
		nil,
	)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), nfID2, "Duplicate entry should be skipped")

	// Third call with DIFFERENT type should succeed.
	nfID3, err := newsfeed.CreateNewsfeedEntry(
		newsfeed.TypeVolunteerOpportunity,
		userID,
		groupID,
		nil,
		&volID,
	)
	assert.NoError(t, err)
	assert.NotZero(t, nfID3, "Different type should not be considered duplicate")

	// Clean up.
	db.Exec("DELETE FROM newsfeed WHERE userid = ?", userID)
	db.Exec("DELETE FROM communityevents WHERE id = ?", eventID)
	db.Exec("DELETE FROM volunteering WHERE id = ?", volID)
}
