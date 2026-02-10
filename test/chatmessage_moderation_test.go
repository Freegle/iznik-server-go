package test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/freegle/iznik-server-go/database"
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
	_, _, _, chatID, msgID, modToken := setupModerationData(t)
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
	_, _, _, chatID, msgID, modToken := setupModerationData(t)
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
