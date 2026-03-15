package test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

// Tests for the support tools endpoints (GET /api/user/:id/*).
// All endpoints require the caller to be a moderator of a group the target belongs to.

func TestUserChatrooms_ModCanSee(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supChat")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, modID)

	// Create a chat room where target is user1.
	db.Exec("INSERT INTO chat_rooms (user1, user2, chattype, latestmessage) VALUES (?, ?, 'User2User', NOW())",
		targetID, modID)

	url := fmt.Sprintf("/api/user/%d/chatrooms?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	body := rsp(resp)
	assert.Equal(t, 200, resp.StatusCode, "Response: %s", string(body))

	var rooms []map[string]interface{}
	json.Unmarshal(body, &rooms)
	assert.GreaterOrEqual(t, len(rooms), 1, "Expected at least 1 chat room, got %d. Response: %s", len(rooms), string(body))
	if len(rooms) > 0 {
		assert.Equal(t, "User2User", rooms[0]["chattype"])
	}
}

func TestUserChatrooms_NonModForbidden(t *testing.T) {
	prefix := uniquePrefix("supChatForbid")
	groupID := CreateTestGroup(t, prefix)
	callerID := CreateTestUser(t, prefix+"_caller", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, callerID, groupID, "Member")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, callerID)

	url := fmt.Sprintf("/api/user/%d/chatrooms?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 403, resp.StatusCode)
}

func TestUserEmailHistory(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supEmail")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, modID)

	db.Exec("INSERT INTO logs_emails (userid, `from`, `to`, subject, status) VALUES (?, 'noreply@test.com', 'user@test.com', 'Test Subject', 'Sent')",
		targetID)

	url := fmt.Sprintf("/api/user/%d/emailhistory?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var emails []map[string]interface{}
	json.Unmarshal(rsp(resp), &emails)
	assert.GreaterOrEqual(t, len(emails), 1)
	assert.Equal(t, "Test Subject", emails[0]["subject"])
}

func TestUserBans(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supBans")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, modID)

	db.Exec("INSERT INTO users_banned (userid, groupid, byuser) VALUES (?, ?, ?)",
		targetID, groupID, modID)

	url := fmt.Sprintf("/api/user/%d/bans?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var bans []map[string]interface{}
	json.Unmarshal(rsp(resp), &bans)
	assert.GreaterOrEqual(t, len(bans), 1)
	assert.Equal(t, float64(groupID), bans[0]["groupid"])
}

func TestUserNewsfeed(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supNews")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, modID)

	db.Exec("INSERT INTO newsfeed (userid, type, message, position) VALUES (?, 'Message', 'Test chitchat post', ST_GeomFromText('POINT(0 0)', 3857))",
		targetID)

	url := fmt.Sprintf("/api/user/%d/newsfeed?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var posts []map[string]interface{}
	json.Unmarshal(rsp(resp), &posts)
	assert.GreaterOrEqual(t, len(posts), 1)
	assert.Equal(t, "Test chitchat post", posts[0]["message"])
}

func TestUserApplied(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supApplied")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, modID)

	db.Exec("INSERT INTO memberships_history (userid, groupid, collection, added) VALUES (?, ?, 'Approved', NOW())",
		targetID, groupID)

	url := fmt.Sprintf("/api/user/%d/applied?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var applied []map[string]interface{}
	json.Unmarshal(rsp(resp), &applied)
	assert.GreaterOrEqual(t, len(applied), 1)
	assert.Equal(t, float64(groupID), applied[0]["groupid"])
}

func TestUserMembershipHistory(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supMemHist")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, modID)

	db.Exec("INSERT INTO memberships_history (userid, groupid, collection, added) VALUES (?, ?, 'Approved', '2025-01-01')",
		targetID, groupID)

	url := fmt.Sprintf("/api/user/%d/membershiphistory?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var history []map[string]interface{}
	json.Unmarshal(rsp(resp), &history)
	assert.GreaterOrEqual(t, len(history), 1)
}

func TestUserLogins(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supLogins")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, modID)

	db.Exec("INSERT INTO users_logins (userid, type, uid) VALUES (?, 'Native', ?)",
		targetID, fmt.Sprintf("%s@test.com", prefix))

	url := fmt.Sprintf("/api/user/%d/logins?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var logins []map[string]interface{}
	json.Unmarshal(rsp(resp), &logins)
	assert.GreaterOrEqual(t, len(logins), 1)
	assert.Equal(t, "Native", logins[0]["type"])
}

func TestUserFetchMT_ReturnsModFields(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supFetchMT")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, modID)

	db.Exec("UPDATE users SET chatmodstatus = 'Fully', newsfeedmodstatus = 'Suppressed' WHERE id = ?", targetID)

	url := fmt.Sprintf("/api/user/fetchmt?id=%d&modtools=true&jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var raw map[string]interface{}
	json.Unmarshal(rsp(resp), &raw)
	assert.Equal(t, "Fully", raw["chatmodstatus"])
	assert.Equal(t, "Suppressed", raw["newsfeedmodstatus"])
}

func TestSpammers_FilterByUserid(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supSpamUid")
	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Create an admin to access spammers endpoint.
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	db.Exec("INSERT INTO spam_users (userid, collection, reason, added) VALUES (?, 'Spammer', 'Test reason', NOW())",
		targetID)

	url := fmt.Sprintf("/apiv2/modtools/spammers?userid=%d&jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.Unmarshal(rsp(resp), &result)
	spammers := result["spammers"].([]interface{})
	assert.GreaterOrEqual(t, len(spammers), 1)
	first := spammers[0].(map[string]interface{})
	assert.Equal(t, float64(targetID), first["userid"])
	assert.Equal(t, "Test reason", first["reason"])
}

func TestUserFetchMT_HidesModFieldsFromNonMod(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supHideMod")
	groupID := CreateTestGroup(t, prefix)
	callerID := CreateTestUser(t, prefix+"_caller", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, callerID, groupID, "Member")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, callerID)

	db.Exec("UPDATE users SET chatmodstatus = 'Fully', newsfeedmodstatus = 'Suppressed' WHERE id = ?", targetID)

	// Non-mod fetching another user — mod-only fields should be hidden.
	url := fmt.Sprintf("/api/user/%d?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var raw map[string]interface{}
	json.Unmarshal(rsp(resp), &raw)
	assert.Nil(t, raw["chatmodstatus"], "chatmodstatus should be hidden from non-mods")
	assert.Nil(t, raw["newsfeedmodstatus"], "newsfeedmodstatus should be hidden from non-mods")
	assert.Nil(t, raw["tnuserid"], "tnuserid should be hidden from non-mods")
}

func TestSupportEndpoints_AllReturn403ForNonMod(t *testing.T) {
	prefix := uniquePrefix("supAll403")
	groupID := CreateTestGroup(t, prefix)
	callerID := CreateTestUser(t, prefix+"_caller", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, callerID, groupID, "Member")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, callerID)

	endpoints := []string{
		"chatrooms", "emailhistory", "bans", "newsfeed",
		"applied", "membershiphistory", "logins",
	}

	for _, ep := range endpoints {
		url := fmt.Sprintf("/api/user/%d/%s?jwt=%s", targetID, ep, token)
		resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
		assert.Equal(t, 403, resp.StatusCode, "Endpoint %s should return 403 for non-mod", ep)
	}
}

// Ensure time import is used.
var _ = time.Now
