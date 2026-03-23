package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/log"
	"github.com/stretchr/testify/assert"
)

// setupModerationData creates a User2Mod chat with a message requiring review.
// Returns modUserID, regularUserID, groupID, chatID, messageID, modToken.
func setupModerationData(t *testing.T) (uint64, uint64, uint64, uint64, uint64, string) {
	t.Helper()
	db := database.DBConn
	prefix := uniquePrefix(t.Name())

	groupID := CreateTestGroup(t, prefix)
	modUserID := CreateTestUser(t, prefix+"_mod", "User")
	regularUserID := CreateTestUser(t, prefix+"_user", "User")

	// Make modUserID a moderator on the group
	CreateTestMembership(t, modUserID, groupID, "Moderator")
	// Make regularUserID a member of the group
	CreateTestMembership(t, regularUserID, groupID, "Member")

	// Create User2Mod chat from regularUser to the group
	chatID := CreateTestChatRoom(t, regularUserID, nil, &groupID, "User2Mod")

	// Create a message that requires review
	result := db.Exec(
		"INSERT INTO chat_messages (chatid, userid, message, date, reviewrequired, processingsuccessful) VALUES (?, ?, ?, NOW(), 1, 1)",
		chatID, regularUserID, "Test message needing review")
	if result.Error != nil {
		t.Fatalf("Failed to create review message: %v", result.Error)
	}

	var msgID uint64
	db.Raw("SELECT id FROM chat_messages WHERE chatid = ? ORDER BY id DESC LIMIT 1", chatID).Scan(&msgID)
	if msgID == 0 {
		t.Fatal("Failed to get message ID")
	}

	// Update chat room message counts
	db.Exec("UPDATE chat_rooms SET msginvalid = 1, latestmessage = NOW() WHERE id = ?", chatID)

	// Create session and JWT token for the moderator
	_, modToken := CreateTestSession(t, modUserID)

	return modUserID, regularUserID, groupID, chatID, msgID, modToken
}

// postChatmessages sends a POST to /api/chatmessages with a JSON body.
func postChatmessages(t *testing.T, path string, body map[string]interface{}, token string) *http.Response {
	t.Helper()
	bodyBytes, _ := json.Marshal(body)

	url := path
	if token != "" {
		url += "?jwt=" + token
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := getApp().Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}

	return resp
}

func TestApproveChatMessage(t *testing.T) {
	modUserID, regularUserID, _, chatID, msgID, modToken := setupModerationData(t)
	db := database.DBConn

	resp := postChatmessages(t, "/api/chatmessages", map[string]interface{}{
		"id":     msgID,
		"action": "Approve",
	}, modToken)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.Equal(t, float64(0), body["ret"])

	// Verify message was approved
	var reviewRequired, reviewRejected int
	var reviewedBy uint64
	db.Raw("SELECT reviewrequired, reviewrejected, COALESCE(reviewedby, 0) FROM chat_messages WHERE id = ?", msgID).Row().Scan(&reviewRequired, &reviewRejected, &reviewedBy)
	assert.Equal(t, 0, reviewRequired, "reviewrequired should be 0")
	assert.Equal(t, 0, reviewRejected, "reviewrejected should be 0")
	assert.NotEqual(t, uint64(0), reviewedBy, "reviewedby should be set")

	// Verify chat room message counts updated
	var msgValid, msgInvalid int
	db.Raw("SELECT msgvalid, msginvalid FROM chat_rooms WHERE id = ?", chatID).Row().Scan(&msgValid, &msgInvalid)
	assert.Greater(t, msgValid, 0, "msgvalid should be > 0")

	// Verify mod log entry created for approve action
	var logCount int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE type = ? AND subtype = ? AND byuser = ? AND user = ?",
		log.LOG_TYPE_CHAT, log.LOG_SUBTYPE_APPROVED, modUserID, regularUserID).Scan(&logCount)
	assert.Equal(t, int64(1), logCount, "Approve should create a Chat/Approved log entry")

	// Verify message text was whitelisted (V1 parity: Spam::notSpam).
	var msgText string
	db.Raw("SELECT message FROM chat_messages WHERE id = ?", msgID).Scan(&msgText)
	if msgText != "" {
		var whitelistCount int64
		db.Raw("SELECT COUNT(*) FROM spam_whitelist_subjects WHERE subject = ?", msgText).Scan(&whitelistCount)
		assert.Equal(t, int64(1), whitelistCount, "Approved message text should be whitelisted")
	}
}

func TestApproveChatMessageNotLoggedIn(t *testing.T) {
	_, _, _, _, msgID, _ := setupModerationData(t)

	resp := postChatmessages(t, "/api/chatmessages", map[string]interface{}{
		"id":     msgID,
		"action": "Approve",
	}, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestApproveChatMessageNotModerator(t *testing.T) {
	_, regularUserID, _, _, msgID, _ := setupModerationData(t)

	_, userToken := CreateTestSession(t, regularUserID)

	resp := postChatmessages(t, "/api/chatmessages", map[string]interface{}{
		"id":     msgID,
		"action": "Approve",
	}, userToken)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestApproveAllFutureChatMessage(t *testing.T) {
	_, regularUserID, _, _, msgID, modToken := setupModerationData(t)
	db := database.DBConn

	resp := postChatmessages(t, "/api/chatmessages", map[string]interface{}{
		"id":     msgID,
		"action": "ApproveAllFuture",
	}, modToken)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify message was approved
	var reviewRequired int
	db.Raw("SELECT reviewrequired FROM chat_messages WHERE id = ?", msgID).Scan(&reviewRequired)
	assert.Equal(t, 0, reviewRequired)

	// Verify user's chatmodstatus was set to Unmoderated
	var chatmodstatus string
	db.Raw("SELECT COALESCE(chatmodstatus, '') FROM users WHERE id = ?", regularUserID).Scan(&chatmodstatus)
	assert.Equal(t, "Unmoderated", chatmodstatus)
}

func TestRejectChatMessage(t *testing.T) {
	modUserID, regularUserID, _, chatID, msgID, modToken := setupModerationData(t)
	db := database.DBConn

	resp := postChatmessages(t, "/api/chatmessages", map[string]interface{}{
		"id":     msgID,
		"action": "Reject",
	}, modToken)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify message was rejected
	var reviewRequired, reviewRejected int
	db.Raw("SELECT reviewrequired, reviewrejected FROM chat_messages WHERE id = ?", msgID).Row().Scan(&reviewRequired, &reviewRejected)
	assert.Equal(t, 0, reviewRequired, "reviewrequired should be 0")
	assert.Equal(t, 1, reviewRejected, "reviewrejected should be 1")

	// Verify chat room message counts updated - rejected messages still count as "invalid"
	// (not valid because reviewrejected=1), so msginvalid stays at 1
	var msgInvalid int
	db.Raw("SELECT msginvalid FROM chat_rooms WHERE id = ?", chatID).Scan(&msgInvalid)
	assert.Equal(t, 1, msgInvalid, "msginvalid should be 1 (rejected msgs count as invalid)")

	// Verify mod log entry created for reject action
	var logCount int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE type = ? AND subtype = ? AND byuser = ? AND user = ?",
		log.LOG_TYPE_CHAT, log.LOG_SUBTYPE_REJECTED, modUserID, regularUserID).Scan(&logCount)
	assert.Equal(t, int64(1), logCount, "Reject should create a Chat/Rejected log entry")
}

func TestRejectDuplicatesChatMessage(t *testing.T) {
	_, regularUserID, groupID, _, msgID, modToken := setupModerationData(t)
	db := database.DBConn

	// Create a second chat with a duplicate message
	otherUserID := CreateTestUser(t, uniquePrefix(t.Name())+"_other", "User")
	CreateTestMembership(t, otherUserID, groupID, "Member")
	chatID2 := CreateTestChatRoom(t, otherUserID, nil, &groupID, "User2Mod")

	db.Exec(
		"INSERT INTO chat_messages (chatid, userid, message, date, reviewrequired, processingsuccessful) VALUES (?, ?, ?, NOW(), 1, 1)",
		chatID2, regularUserID, "Test message needing review")

	var dupMsgID uint64
	db.Raw("SELECT id FROM chat_messages WHERE chatid = ? ORDER BY id DESC LIMIT 1", chatID2).Scan(&dupMsgID)

	// Reject the original - should also reject the duplicate
	resp := postChatmessages(t, "/api/chatmessages", map[string]interface{}{
		"id":     msgID,
		"action": "Reject",
	}, modToken)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify duplicate was also rejected
	var reviewRejected int
	db.Raw("SELECT reviewrejected FROM chat_messages WHERE id = ?", dupMsgID).Scan(&reviewRejected)
	assert.Equal(t, 1, reviewRejected, "duplicate message should also be rejected")
}

func TestHoldChatMessage(t *testing.T) {
	modUserID, _, _, _, msgID, modToken := setupModerationData(t)
	db := database.DBConn

	resp := postChatmessages(t, "/api/chatmessages", map[string]interface{}{
		"id":     msgID,
		"action": "Hold",
	}, modToken)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify hold record created
	var heldBy uint64
	db.Raw("SELECT userid FROM chat_messages_held WHERE msgid = ?", msgID).Scan(&heldBy)
	assert.Equal(t, modUserID, heldBy, "message should be held by the moderator")
}

func TestReleaseChatMessage(t *testing.T) {
	_, _, _, _, msgID, modToken := setupModerationData(t)
	db := database.DBConn

	// First hold it
	postChatmessages(t, "/api/chatmessages", map[string]interface{}{
		"id":     msgID,
		"action": "Hold",
	}, modToken)

	// Then release it
	resp := postChatmessages(t, "/api/chatmessages", map[string]interface{}{
		"id":     msgID,
		"action": "Release",
	}, modToken)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify hold record removed
	var heldCount int64
	db.Raw("SELECT COUNT(*) FROM chat_messages_held WHERE msgid = ?", msgID).Scan(&heldCount)
	assert.Equal(t, int64(0), heldCount, "hold record should be removed")
}

func TestApproveHeldByOtherMod(t *testing.T) {
	_, _, groupID, _, msgID, _ := setupModerationData(t)
	db := database.DBConn

	// Create a second moderator and hold the message
	prefix2 := uniquePrefix(t.Name()) + "_mod2"
	mod2ID := CreateTestUser(t, prefix2, "User")
	CreateTestMembership(t, mod2ID, groupID, "Moderator")
	_, mod2Token := CreateTestSession(t, mod2ID)

	// Hold with mod2
	postChatmessages(t, "/api/chatmessages", map[string]interface{}{
		"id":     msgID,
		"action": "Hold",
	}, mod2Token)

	// Create another moderator and try to approve - should fail because held by mod2
	prefix3 := uniquePrefix(t.Name()) + "_mod3"
	mod3ID := CreateTestUser(t, prefix3, "User")
	CreateTestMembership(t, mod3ID, groupID, "Moderator")
	_, mod3Token := CreateTestSession(t, mod3ID)

	resp := postChatmessages(t, "/api/chatmessages", map[string]interface{}{
		"id":     msgID,
		"action": "Approve",
	}, mod3Token)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)

	// Verify message is still pending review
	var reviewRequired int
	db.Raw("SELECT reviewrequired FROM chat_messages WHERE id = ?", msgID).Scan(&reviewRequired)
	assert.Equal(t, 1, reviewRequired, "message should still require review")
}

func TestRedactChatMessage(t *testing.T) {
	_, _, _, _, msgID, modToken := setupModerationData(t)
	db := database.DBConn

	// Update the message to include an email address
	db.Exec("UPDATE chat_messages SET message = ? WHERE id = ?",
		"Please contact me at test@example.com for more info", msgID)

	resp := postChatmessages(t, "/api/chatmessages", map[string]interface{}{
		"id":     msgID,
		"action": "Redact",
	}, modToken)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify email was redacted
	var message string
	db.Raw("SELECT message FROM chat_messages WHERE id = ?", msgID).Scan(&message)
	assert.NotContains(t, message, "test@example.com", "email should be removed")
	assert.Contains(t, message, "(email removed)", "should have placeholder text")
}

func TestRedactNoEmailInMessage(t *testing.T) {
	_, _, _, _, msgID, modToken := setupModerationData(t)
	db := database.DBConn

	db.Exec("UPDATE chat_messages SET message = ? WHERE id = ?",
		"Hello this is a normal message", msgID)

	resp := postChatmessages(t, "/api/chatmessages", map[string]interface{}{
		"id":     msgID,
		"action": "Redact",
	}, modToken)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var message string
	db.Raw("SELECT message FROM chat_messages WHERE id = ?", msgID).Scan(&message)
	assert.Equal(t, "Hello this is a normal message", message)
}

func TestAutoApproveModmailAfterApprove(t *testing.T) {
	_, regularUserID, _, chatID, msgID, modToken := setupModerationData(t)
	db := database.DBConn

	// Create a ModMail message after the review-required message
	db.Exec(
		"INSERT INTO chat_messages (chatid, userid, message, date, type, reviewrequired, processingsuccessful) VALUES (?, ?, ?, DATE_ADD(NOW(), INTERVAL 1 SECOND), ?, 1, 1)",
		chatID, regularUserID, "ModMail after review", "ModMail")

	var modmailID uint64
	db.Raw("SELECT id FROM chat_messages WHERE chatid = ? AND type = 'ModMail' ORDER BY id DESC LIMIT 1", chatID).Scan(&modmailID)

	// Approve the first message
	resp := postChatmessages(t, "/api/chatmessages", map[string]interface{}{
		"id":     msgID,
		"action": "Approve",
	}, modToken)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify modmail was auto-approved
	var reviewRequired int
	db.Raw("SELECT reviewrequired FROM chat_messages WHERE id = ?", modmailID).Scan(&reviewRequired)
	assert.Equal(t, 0, reviewRequired, "ModMail should be auto-approved")
}

func TestInvalidAction(t *testing.T) {
	_, _, _, _, msgID, modToken := setupModerationData(t)

	resp := postChatmessages(t, "/api/chatmessages", map[string]interface{}{
		"id":     msgID,
		"action": "InvalidAction",
	}, modToken)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestV2ChatmessagesModeration(t *testing.T) {
	_, _, _, _, msgID, modToken := setupModerationData(t)

	resp := postChatmessages(t, "/apiv2/chatmessages", map[string]interface{}{
		"id":     msgID,
		"action": "Approve",
	}, modToken)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}


// setupWiderReviewData creates two groups, a mod on group1 with widerchatreview=1,
// and a chat message requiring review between members on group2 (also widerchatreview=1).
// The mod is NOT on group2. Returns mod token and IDs needed for assertions.
func setupWiderReviewData(t *testing.T) (modToken string, modID, group1ID, group2ID, chatMsgID uint64) {
	t.Helper()
	db := database.DBConn
	prefix := uniquePrefix(t.Name())

	// Group 1: mod's group with widerchatreview enabled.
	group1ID = CreateTestGroup(t, prefix+"_g1")
	db.Exec("UPDATE `groups` SET settings = JSON_SET(COALESCE(settings, '{}'), '$.widerchatreview', 1) WHERE id = ?", group1ID)

	// Group 2: separate group with widerchatreview enabled.
	group2ID = CreateTestGroup(t, prefix+"_g2")
	db.Exec("UPDATE `groups` SET settings = JSON_SET(COALESCE(settings, '{}'), '$.widerchatreview', 1) WHERE id = ?", group2ID)

	// Active moderator on group1 only.
	modID = CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, group1ID, "Moderator")
	// Ensure active=1 in membership settings.
	db.Exec("UPDATE memberships SET settings = '{\"active\":1}' WHERE userid = ? AND groupid = ?", modID, group1ID)

	// Two members on group2 (not mod's group).
	user1ID := CreateTestUser(t, prefix+"_user1", "User")
	user2ID := CreateTestUser(t, prefix+"_user2", "User")
	CreateTestMembership(t, user1ID, group2ID, "Member")
	CreateTestMembership(t, user2ID, group2ID, "Member")

	// Another mod on group2 (required for chat moderation).
	mod2ID := CreateTestUser(t, prefix+"_mod2", "Moderator")
	CreateTestMembership(t, mod2ID, group2ID, "Moderator")

	// Create User2User chat between user1 and user2.
	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")

	// Create a message requiring review (not user-reported).
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, reviewrequired, processingsuccessful) "+
		"VALUES (?, ?, 'Wider review test message', NOW(), 1, 1)", chatID, user1ID)
	db.Raw("SELECT id FROM chat_messages WHERE chatid = ? ORDER BY id DESC LIMIT 1", chatID).Scan(&chatMsgID)

	_, modToken = CreateTestSession(t, modID)
	return
}

func TestWiderReviewEligibility(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("WiderElig")

	// Group without widerchatreview.
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, groupID, "Moderator")
	db.Exec("UPDATE memberships SET settings = '{\"active\":1}' WHERE userid = ? AND groupid = ?", modID, groupID)

	// Not eligible without the setting.
	_, token := CreateTestSession(t, modID)
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?jwt=%s", token), nil)
	resp, _ := getApp().Test(req, -1)
	assert.Equal(t, 200, resp.StatusCode)

	// Enable widerchatreview on the group.
	db.Exec("UPDATE `groups` SET settings = JSON_SET(COALESCE(settings, '{}'), '$.widerchatreview', 1) WHERE id = ?", groupID)

	// Now the endpoint should still return 200 (just includes wider review).
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?jwt=%s", token), nil)
	resp, _ = getApp().Test(req, -1)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestWiderReviewShowsMessages(t *testing.T) {
	modToken, _, _, _, chatMsgID := setupWiderReviewData(t)

	// The mod on group1 should see the message from group2 via wider review.
	// Use limit=1000 to ensure our test message isn't cut off by the default limit
	// when there are many existing review messages in the database.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?jwt=%s&limit=1000", modToken), nil)
	resp, _ := getApp().Test(req, -1)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	msgs, ok := result["chatmessages"].([]interface{})
	assert.True(t, ok, "chatmessages should be an array")

	// Find our message in the results.
	found := false
	for _, m := range msgs {
		msg := m.(map[string]interface{})
		if uint64(msg["id"].(float64)) == chatMsgID {
			found = true
			assert.Equal(t, true, msg["widerchatreview"], "message should be marked as wider chat review")
			break
		}
	}
	assert.True(t, found, "wider review message should appear in review queue")
}

func TestWiderReviewExcludesHeldMessages(t *testing.T) {
	modToken, _, _, _, chatMsgID := setupWiderReviewData(t)
	db := database.DBConn

	// Hold the message.
	holdUserID := CreateTestUser(t, uniquePrefix("WiderHold")+"_holder", "Moderator")
	db.Exec("INSERT INTO chat_messages_held (msgid, userid) VALUES (?, ?)", chatMsgID, holdUserID)

	// Held messages should NOT appear in wider review.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?jwt=%s", modToken), nil)
	resp, _ := getApp().Test(req, -1)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	msgs := result["chatmessages"].([]interface{})
	for _, m := range msgs {
		msg := m.(map[string]interface{})
		if uint64(msg["id"].(float64)) == chatMsgID {
			t.Error("held message should NOT appear in wider chat review")
		}
	}
}

func TestWiderReviewExcludesUserReported(t *testing.T) {
	modToken, _, _, _, chatMsgID := setupWiderReviewData(t)
	db := database.DBConn

	// Set reportreason to 'User' (user-reported spam).
	db.Exec("UPDATE chat_messages SET reportreason = 'User' WHERE id = ?", chatMsgID)

	// User-reported messages should NOT appear in wider review.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?jwt=%s", modToken), nil)
	resp, _ := getApp().Test(req, -1)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	msgs := result["chatmessages"].([]interface{})
	for _, m := range msgs {
		msg := m.(map[string]interface{})
		if uint64(msg["id"].(float64)) == chatMsgID {
			t.Error("user-reported message should NOT appear in wider chat review")
		}
	}
}

func TestWiderReviewNotEligibleWithoutSetting(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("WiderNoSet")

	// Group WITHOUT widerchatreview.
	group1ID := CreateTestGroup(t, prefix+"_g1")
	// Group with widerchatreview and a review message.
	group2ID := CreateTestGroup(t, prefix+"_g2")
	db.Exec("UPDATE `groups` SET settings = JSON_SET(COALESCE(settings, '{}'), '$.widerchatreview', 1) WHERE id = ?", group2ID)

	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, group1ID, "Moderator")
	db.Exec("UPDATE memberships SET settings = '{\"active\":1}' WHERE userid = ? AND groupid = ?", modID, group1ID)

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	CreateTestMembership(t, user1ID, group2ID, "Member")
	CreateTestMembership(t, user2ID, group2ID, "Member")

	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, reviewrequired, processingsuccessful) "+
		"VALUES (?, ?, 'Should not see this', NOW(), 1, 1)", chatID, user1ID)

	_, modToken := CreateTestSession(t, modID)

	// Mod's group doesn't have widerchatreview → should NOT see wider review messages.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?jwt=%s", modToken), nil)
	resp, _ := getApp().Test(req, -1)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	msgs := result["chatmessages"].([]interface{})
	assert.Equal(t, 0, len(msgs), "should not see wider review messages without widerchatreview on mod's group")
}

func TestReviewReasonEnrichment(t *testing.T) {
	_, _, _, _, _ = setupWiderReviewData(t)
	db := database.DBConn
	prefix := uniquePrefix(t.Name())

	// Create a group and membership for the mod so they can see the messages.
	groupID := CreateTestGroup(t, prefix+"_enrich")
	modID := CreateTestUser(t, prefix+"_emod", "Moderator")
	CreateTestMembership(t, modID, groupID, "Moderator")
	regularUserID := CreateTestUser(t, prefix+"_euser", "User")
	CreateTestMembership(t, regularUserID, groupID, "Member")
	_, modToken2 := CreateTestSession(t, modID)

	chatID := CreateTestChatRoom(t, regularUserID, nil, &groupID, "User2Mod")

	tests := []struct {
		name     string
		message  string
		reason   string
		expected string
	}{
		{"money_dollar", "I can sell this for $50", "Spam", "Money"},
		{"money_pound", "It costs £20 to deliver", "Spam", "Money"},
		{"email", "Contact me at scammer@evil.com", "Spam", "Email"},
		{"link", "Visit http://suspicious-site.xyz/malware for details", "Spam", "Link"},
		{"script", "Hello <script>alert('xss')</script> world", "Spam", "Script"},
		{"url_removed", "Check out (URL removed) for info", "Spam", "Link"},
		{"non_spam_reason", "Normal message", "Fully", "Fully"},
		{"no_reason", "Normal message", "", ""},
		{"freegle_email_excluded", "Email noreply@ilovefreegle.org for info", "Spam", "Spam"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var reportreason *string
			if tc.reason != "" {
				reportreason = &tc.reason
			}

			db.Exec(
				"INSERT INTO chat_messages (chatid, userid, message, date, reviewrequired, reportreason, processingsuccessful) VALUES (?, ?, ?, NOW(), 1, ?, 1)",
				chatID, regularUserID, tc.message, reportreason)

			var msgID uint64
			db.Raw("SELECT id FROM chat_messages WHERE chatid = ? ORDER BY id DESC LIMIT 1", chatID).Scan(&msgID)

			req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?jwt=%s", modToken2), nil)
			resp, _ := getApp().Test(req, -1)
			assert.Equal(t, 200, resp.StatusCode)

			var result map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&result)

			msgs := result["chatmessages"].([]interface{})
			found := false
			for _, m := range msgs {
				msg := m.(map[string]interface{})
				if uint64(msg["id"].(float64)) == msgID {
					found = true
					if tc.expected == "" {
						// Empty/nil reason should result in empty string or nil.
						rr, ok := msg["reviewreason"]
						if ok && rr != nil {
							assert.Equal(t, "", rr, "expected empty reviewreason for test %s", tc.name)
						}
					} else {
						assert.Equal(t, tc.expected, msg["reviewreason"], "wrong reviewreason for test %s", tc.name)
					}
					break
				}
			}
			assert.True(t, found, "message %d should appear in review queue for test %s", msgID, tc.name)

			// Clean up for next iteration.
			db.Exec("DELETE FROM chat_messages WHERE id = ?", msgID)
		})
	}
}

func TestWiderReviewWorkCounts(t *testing.T) {
	modToken, _, group1ID, group2ID, _ := setupWiderReviewData(t)

	// Check work counts via groupwork endpoint.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/group/work?jwt=%s", modToken), nil)
	resp, _ := getApp().Test(req, -1)
	assert.Equal(t, 200, resp.StatusCode)

	var workItems []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&workItems)

	// Should have work items. The wider review message should create a chatreviewother count
	// on group2 (the wider review group) as a new entry.
	foundGroup2 := false
	for _, w := range workItems {
		gid := uint64(w["groupid"].(float64))
		if gid == group2ID {
			foundGroup2 = true
			assert.Greater(t, w["chatreviewother"].(float64), float64(0),
				"group2 should have chatreviewother count from wider review")
		}
		if gid == group1ID {
			// Group1 is the mod's own group — wider review may or may not show here.
			// The important thing is group2 has the count.
		}
	}
	assert.True(t, foundGroup2, "group2 should appear in work counts from wider review")
}
