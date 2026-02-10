package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func TestPostMessageNotLoggedIn(t *testing.T) {
	body := map[string]interface{}{"id": 1, "action": "Promise"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/message", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestPostMessageNoID(t *testing.T) {
	prefix := uniquePrefix("msgw_noid")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	body := map[string]interface{}{"action": "Promise"}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostMessageUnknownAction(t *testing.T) {
	prefix := uniquePrefix("msgw_unk")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	body := map[string]interface{}{"id": 1, "action": "Bogus"}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostMessagePromise(t *testing.T) {
	prefix := uniquePrefix("msgw_promise")
	db := database.DBConn

	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	_, ownerToken := CreateTestSession(t, ownerID)
	otherID := CreateTestUser(t, prefix+"_other", "User")
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, ownerID, groupID, prefix+" offer item", 52.5, -1.8)

	// Create a chat room between the users for the system message.
	CreateTestChatRoom(t, ownerID, &otherID, nil, "User2User")

	// Promise the item to the other user.
	body := map[string]interface{}{
		"id":     msgID,
		"action": "Promise",
		"userid": otherID,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", ownerToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify promise recorded in DB.
	var count int64
	db.Raw("SELECT COUNT(*) FROM messages_promises WHERE msgid = ? AND userid = ?", msgID, otherID).Scan(&count)
	assert.Equal(t, int64(1), count)

	// Verify chat message created.
	var chatMsgCount int64
	db.Raw("SELECT COUNT(*) FROM chat_messages WHERE refmsgid = ? AND type = 'Promised'", msgID).Scan(&chatMsgCount)
	assert.Equal(t, int64(1), chatMsgCount)
}

func TestPostMessagePromiseNotYourMessage(t *testing.T) {
	prefix := uniquePrefix("msgw_prm_notmine")

	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, otherToken := CreateTestSession(t, otherID)
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, ownerID, groupID, prefix+" offer item", 52.5, -1.8)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "Promise",
		"userid": otherID,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", otherToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestPostMessagePromiseMessageNotFound(t *testing.T) {
	prefix := uniquePrefix("msgw_prm_nf")

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	body := map[string]interface{}{
		"id":     999999999,
		"action": "Promise",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestPostMessageRenege(t *testing.T) {
	prefix := uniquePrefix("msgw_renege")
	db := database.DBConn

	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	_, ownerToken := CreateTestSession(t, ownerID)
	otherID := CreateTestUser(t, prefix+"_other", "User")
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, ownerID, groupID, prefix+" offer item", 52.5, -1.8)

	// Create a chat room and a promise first.
	CreateTestChatRoom(t, ownerID, &otherID, nil, "User2User")
	db.Exec("REPLACE INTO messages_promises (msgid, userid) VALUES (?, ?)", msgID, otherID)

	// Renege on the promise.
	body := map[string]interface{}{
		"id":     msgID,
		"action": "Renege",
		"userid": otherID,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", ownerToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify promise deleted.
	var promiseCount int64
	db.Raw("SELECT COUNT(*) FROM messages_promises WHERE msgid = ? AND userid = ?", msgID, otherID).Scan(&promiseCount)
	assert.Equal(t, int64(0), promiseCount)

	// Verify renege recorded.
	var renegeCount int64
	db.Raw("SELECT COUNT(*) FROM messages_reneged WHERE msgid = ? AND userid = ?", msgID, otherID).Scan(&renegeCount)
	assert.Equal(t, int64(1), renegeCount)

	// Verify chat message created.
	var chatMsgCount int64
	db.Raw("SELECT COUNT(*) FROM chat_messages WHERE refmsgid = ? AND type = 'Reneged'", msgID).Scan(&chatMsgCount)
	assert.Equal(t, int64(1), chatMsgCount)
}

func TestPostMessageOutcomeIntended(t *testing.T) {
	prefix := uniquePrefix("msgw_intended")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, userID, groupID, prefix+" offer item", 52.5, -1.8)

	body := map[string]interface{}{
		"id":      msgID,
		"action":  "OutcomeIntended",
		"outcome": "Taken",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify intended outcome recorded.
	var outcome string
	db.Raw("SELECT outcome FROM messages_outcomes_intended WHERE msgid = ?", msgID).Scan(&outcome)
	assert.Equal(t, "Taken", outcome)
}

func TestPostMessageOutcomeIntendedInvalid(t *testing.T) {
	prefix := uniquePrefix("msgw_int_inv")

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, userID, groupID, prefix+" offer item", 52.5, -1.8)

	body := map[string]interface{}{
		"id":      msgID,
		"action":  "OutcomeIntended",
		"outcome": "Invalid",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostMessageOutcome(t *testing.T) {
	prefix := uniquePrefix("msgw_outcome")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, userID, groupID, prefix+" offer item", 52.5, -1.8)

	happiness := "Happy"
	comment := "Great transaction"
	body := map[string]interface{}{
		"id":        msgID,
		"action":    "Outcome",
		"outcome":   "Taken",
		"happiness": happiness,
		"comment":   comment,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify outcome recorded.
	var dbOutcome string
	var dbHappiness string
	var dbComments string
	db.Raw("SELECT outcome, happiness, comments FROM messages_outcomes WHERE msgid = ?", msgID).Row().Scan(&dbOutcome, &dbHappiness, &dbComments)
	assert.Equal(t, "Taken", dbOutcome)
	assert.Equal(t, "Happy", dbHappiness)
	assert.Equal(t, "Great transaction", dbComments)
}

func TestPostMessageOutcomeDuplicate(t *testing.T) {
	prefix := uniquePrefix("msgw_out_dup")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, userID, groupID, prefix+" offer item", 52.5, -1.8)

	// Insert an existing outcome.
	db.Exec("INSERT INTO messages_outcomes (msgid, outcome) VALUES (?, 'Taken')", msgID)

	body := map[string]interface{}{
		"id":      msgID,
		"action":  "Outcome",
		"outcome": "Withdrawn",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 409, resp.StatusCode)
}

func TestPostMessageOutcomeMessageNotFound(t *testing.T) {
	prefix := uniquePrefix("msgw_out_nf")

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	body := map[string]interface{}{
		"id":      999999999,
		"action":  "Outcome",
		"outcome": "Taken",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestPostMessageAddBy(t *testing.T) {
	prefix := uniquePrefix("msgw_addby")
	db := database.DBConn

	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	_, ownerToken := CreateTestSession(t, ownerID)
	takerID := CreateTestUser(t, prefix+"_taker", "User")
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, ownerID, groupID, prefix+" offer item", 52.5, -1.8)

	// Set initial availability.
	db.Exec("UPDATE messages SET availableinitially = 5, availablenow = 5 WHERE id = ?", msgID)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "AddBy",
		"userid": takerID,
		"count":  2,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", ownerToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify messages_by entry.
	var byCount int
	db.Raw("SELECT count FROM messages_by WHERE msgid = ? AND userid = ?", msgID, takerID).Scan(&byCount)
	assert.Equal(t, 2, byCount)

	// Verify available count reduced.
	var availNow int
	db.Raw("SELECT availablenow FROM messages WHERE id = ?", msgID).Scan(&availNow)
	assert.Equal(t, 3, availNow)
}

func TestPostMessageAddByUpdate(t *testing.T) {
	prefix := uniquePrefix("msgw_addby_upd")
	db := database.DBConn

	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	_, ownerToken := CreateTestSession(t, ownerID)
	takerID := CreateTestUser(t, prefix+"_taker", "User")
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, ownerID, groupID, prefix+" offer item", 52.5, -1.8)

	// Set initial availability and add an existing entry.
	db.Exec("UPDATE messages SET availableinitially = 5, availablenow = 3 WHERE id = ?", msgID)
	db.Exec("INSERT INTO messages_by (userid, msgid, count) VALUES (?, ?, 2)", takerID, msgID)

	// Update the count to 3.
	body := map[string]interface{}{
		"id":     msgID,
		"action": "AddBy",
		"userid": takerID,
		"count":  3,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", ownerToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify updated count.
	var byCount int
	db.Raw("SELECT count FROM messages_by WHERE msgid = ? AND userid = ?", msgID, takerID).Scan(&byCount)
	assert.Equal(t, 3, byCount)

	// Old count was 2, restored to 5, then reduced by 3 = 2.
	var availNow int
	db.Raw("SELECT availablenow FROM messages WHERE id = ?", msgID).Scan(&availNow)
	assert.Equal(t, 2, availNow)
}

func TestPostMessageRemoveBy(t *testing.T) {
	prefix := uniquePrefix("msgw_rmby")
	db := database.DBConn

	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	_, ownerToken := CreateTestSession(t, ownerID)
	takerID := CreateTestUser(t, prefix+"_taker", "User")
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, ownerID, groupID, prefix+" offer item", 52.5, -1.8)

	// Set availability and add an entry.
	db.Exec("UPDATE messages SET availableinitially = 5, availablenow = 3 WHERE id = ?", msgID)
	db.Exec("INSERT INTO messages_by (userid, msgid, count) VALUES (?, ?, 2)", takerID, msgID)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "RemoveBy",
		"userid": takerID,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", ownerToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify entry removed.
	var byCount int64
	db.Raw("SELECT COUNT(*) FROM messages_by WHERE msgid = ? AND userid = ?", msgID, takerID).Scan(&byCount)
	assert.Equal(t, int64(0), byCount)

	// Verify availability restored.
	var availNow int
	db.Raw("SELECT availablenow FROM messages WHERE id = ?", msgID).Scan(&availNow)
	assert.Equal(t, 5, availNow)
}

func TestPostMessageOutcomeTakenOnWanted(t *testing.T) {
	prefix := uniquePrefix("msgw_tak_wnt")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, userID, groupID, prefix+" wanted item", 52.5, -1.8)

	// Change type to Wanted.
	db.Exec("UPDATE messages SET type = 'Wanted' WHERE id = ?", msgID)

	body := map[string]interface{}{
		"id":      msgID,
		"action":  "Outcome",
		"outcome": "Taken",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode, "Taken outcome should be rejected on Wanted message")
}

func TestPostMessageOutcomeReceivedOnOffer(t *testing.T) {
	prefix := uniquePrefix("msgw_rcv_ofr")

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, userID, groupID, prefix+" offer item", 52.5, -1.8)

	// Message is already Offer type from CreateTestMessage.
	body := map[string]interface{}{
		"id":      msgID,
		"action":  "Outcome",
		"outcome": "Received",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode, "Received outcome should be rejected on Offer message")
}

func TestPostMessageAddByNotYourMessage(t *testing.T) {
	prefix := uniquePrefix("msgw_addby_ny")
	db := database.DBConn

	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, otherToken := CreateTestSession(t, otherID)
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, ownerID, groupID, prefix+" offer item", 52.5, -1.8)

	db.Exec("UPDATE messages SET availableinitially = 5, availablenow = 5 WHERE id = ?", msgID)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "AddBy",
		"userid": otherID,
		"count":  1,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", otherToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode, "Non-owner should not be able to AddBy")
}

func TestPostMessageRemoveByNotYourMessage(t *testing.T) {
	prefix := uniquePrefix("msgw_rmby_ny")
	db := database.DBConn

	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, otherToken := CreateTestSession(t, otherID)
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, ownerID, groupID, prefix+" offer item", 52.5, -1.8)

	db.Exec("UPDATE messages SET availableinitially = 5, availablenow = 3 WHERE id = ?", msgID)
	db.Exec("INSERT INTO messages_by (userid, msgid, count) VALUES (?, ?, 2)", otherID, msgID)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "RemoveBy",
		"userid": otherID,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", otherToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode, "Non-owner should not be able to RemoveBy")
}

func TestPostMessagePromiseCreatesChat(t *testing.T) {
	// H1: Promise should create a chat room if none exists between the users.
	prefix := uniquePrefix("msgw_prm_cc")
	db := database.DBConn

	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	_, ownerToken := CreateTestSession(t, ownerID)
	otherID := CreateTestUser(t, prefix+"_other", "User")
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, ownerID, groupID, prefix+" offer item", 52.5, -1.8)

	// Verify no chat room exists between these users.
	var chatCount int64
	db.Raw("SELECT COUNT(*) FROM chat_rooms WHERE (user1 = ? AND user2 = ?) OR (user1 = ? AND user2 = ?)",
		ownerID, otherID, otherID, ownerID).Scan(&chatCount)
	assert.Equal(t, int64(0), chatCount)

	// Promise the item - should create a chat room.
	body := map[string]interface{}{
		"id":     msgID,
		"action": "Promise",
		"userid": otherID,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", ownerToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify chat room was created.
	db.Raw("SELECT COUNT(*) FROM chat_rooms WHERE (user1 = ? AND user2 = ?) OR (user1 = ? AND user2 = ?)",
		ownerID, otherID, otherID, ownerID).Scan(&chatCount)
	assert.Equal(t, int64(1), chatCount)

	// Verify chat message was created.
	var chatMsgCount int64
	db.Raw("SELECT COUNT(*) FROM chat_messages WHERE refmsgid = ? AND type = 'Promised'", msgID).Scan(&chatMsgCount)
	assert.Equal(t, int64(1), chatMsgCount)
}

func TestPostMessageOutcomeTakenWithUserRecordsBy(t *testing.T) {
	// H3: Outcome Taken/Received with userid should insert into messages_by.
	prefix := uniquePrefix("msgw_out_by")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	takerID := CreateTestUser(t, prefix+"_taker", "User")
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, userID, groupID, prefix+" offer item", 52.5, -1.8)

	// Set availability.
	db.Exec("UPDATE messages SET availableinitially = 3, availablenow = 3 WHERE id = ?", msgID)

	body := map[string]interface{}{
		"id":      msgID,
		"action":  "Outcome",
		"outcome": "Taken",
		"userid":  takerID,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify messages_by entry created with availablenow count.
	var byCount int
	db.Raw("SELECT count FROM messages_by WHERE msgid = ? AND userid = ?", msgID, takerID).Scan(&byCount)
	assert.Equal(t, 3, byCount, "messages_by should record availablenow count for the taker")
}

func TestPostMessageWithdrawnPending(t *testing.T) {
	// H4: Withdrawn on a pending message should delete it entirely.
	prefix := uniquePrefix("msgw_wdr_pnd")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, userID, groupID, prefix+" offer item", 52.5, -1.8)

	// Set the message as Pending on the group.
	db.Exec("UPDATE messages_groups SET collection = 'Pending' WHERE msgid = ? AND groupid = ?", msgID, groupID)

	body := map[string]interface{}{
		"id":      msgID,
		"action":  "Outcome",
		"outcome": "Withdrawn",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, true, result["deleted"], "Pending message should be deleted, not marked")

	// Verify message was deleted.
	var msgCount int64
	db.Raw("SELECT COUNT(*) FROM messages WHERE id = ?", msgID).Scan(&msgCount)
	assert.Equal(t, int64(0), msgCount, "Message should be deleted from messages table")
}

func TestPostMessageWithdrawnApproved(t *testing.T) {
	// Withdrawn on an approved message should record the outcome normally (not delete).
	prefix := uniquePrefix("msgw_wdr_app")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, userID, groupID, prefix+" offer item", 52.5, -1.8)

	// Message is already Approved by default from CreateTestMessage.
	body := map[string]interface{}{
		"id":      msgID,
		"action":  "Outcome",
		"outcome": "Withdrawn",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify outcome was recorded (not deleted).
	var dbOutcome string
	db.Raw("SELECT outcome FROM messages_outcomes WHERE msgid = ?", msgID).Scan(&dbOutcome)
	assert.Equal(t, "Withdrawn", dbOutcome)

	// Verify message still exists.
	var msgCount int64
	db.Raw("SELECT COUNT(*) FROM messages WHERE id = ?", msgID).Scan(&msgCount)
	assert.Equal(t, int64(1), msgCount, "Approved message should NOT be deleted")
}

func TestPostMessageView(t *testing.T) {
	prefix := uniquePrefix("msgw_view")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, userID, groupID, prefix+" offer item", 52.5, -1.8)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "View",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify view recorded.
	var viewCount int64
	db.Raw("SELECT COUNT(*) FROM messages_likes WHERE msgid = ? AND userid = ? AND type = 'View'", msgID, userID).Scan(&viewCount)
	assert.Equal(t, int64(1), viewCount)
}

func TestPostMessageViewDedup(t *testing.T) {
	prefix := uniquePrefix("msgw_view_dup")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, userID, groupID, prefix+" offer item", 52.5, -1.8)

	// Insert a recent view.
	db.Exec("INSERT INTO messages_likes (msgid, userid, type) VALUES (?, ?, 'View')", msgID, userID)

	// View again - should be de-duplicated (count stays at 1).
	body := map[string]interface{}{
		"id":     msgID,
		"action": "View",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Should still be just 1 view (de-duplicated within 30 min).
	var viewCount int
	db.Raw("SELECT count FROM messages_likes WHERE msgid = ? AND userid = ? AND type = 'View'", msgID, userID).Scan(&viewCount)
	assert.Equal(t, 1, viewCount)
}

// --- Adversarial tests ---

func TestPostMessageAddByNegativeCount(t *testing.T) {
	// Negative count should not corrupt availablenow.
	prefix := uniquePrefix("msgw_addby_neg")
	db := database.DBConn

	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	_, ownerToken := CreateTestSession(t, ownerID)
	takerID := CreateTestUser(t, prefix+"_taker", "User")
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, ownerID, groupID, prefix+" offer item", 52.5, -1.8)

	db.Exec("UPDATE messages SET availableinitially = 5, availablenow = 5 WHERE id = ?", msgID)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "AddBy",
		"userid": takerID,
		"count":  -3,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", ownerToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// availablenow should not exceed availableinitially (LEAST guard protects).
	var availNow int
	db.Raw("SELECT availablenow FROM messages WHERE id = ?", msgID).Scan(&availNow)
	assert.LessOrEqual(t, availNow, 5, "availablenow should not exceed availableinitially")
}

func TestPostMessageAddByHugeCount(t *testing.T) {
	// Very large count should not make availablenow negative (GREATEST guard protects).
	prefix := uniquePrefix("msgw_addby_huge")
	db := database.DBConn

	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	_, ownerToken := CreateTestSession(t, ownerID)
	takerID := CreateTestUser(t, prefix+"_taker", "User")
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, ownerID, groupID, prefix+" offer item", 52.5, -1.8)

	db.Exec("UPDATE messages SET availableinitially = 2, availablenow = 2 WHERE id = ?", msgID)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "AddBy",
		"userid": takerID,
		"count":  99999,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", ownerToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// availablenow should be 0, not negative (GREATEST(0) guard).
	var availNow int
	db.Raw("SELECT availablenow FROM messages WHERE id = ?", msgID).Scan(&availNow)
	assert.GreaterOrEqual(t, availNow, 0, "availablenow should never go negative")
}

func TestPostMessagePromiseToSelfNoUserid(t *testing.T) {
	// Promise without userid should promise to self (no chat message).
	prefix := uniquePrefix("msgw_prm_self")
	db := database.DBConn

	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	_, ownerToken := CreateTestSession(t, ownerID)
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, ownerID, groupID, prefix+" offer item", 52.5, -1.8)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "Promise",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", ownerToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Promise should be recorded with self as userid.
	var count int64
	db.Raw("SELECT COUNT(*) FROM messages_promises WHERE msgid = ? AND userid = ?", msgID, ownerID).Scan(&count)
	assert.Equal(t, int64(1), count)

	// No chat message should be created (promising to self).
	var chatMsgCount int64
	db.Raw("SELECT COUNT(*) FROM chat_messages WHERE refmsgid = ? AND type = 'Promised'", msgID).Scan(&chatMsgCount)
	assert.Equal(t, int64(0), chatMsgCount)
}

func TestPostMessageDoublePromise(t *testing.T) {
	// Double Promise should be idempotent (REPLACE INTO).
	prefix := uniquePrefix("msgw_prm_dbl")
	db := database.DBConn

	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	_, ownerToken := CreateTestSession(t, ownerID)
	otherID := CreateTestUser(t, prefix+"_other", "User")
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, ownerID, groupID, prefix+" offer item", 52.5, -1.8)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "Promise",
		"userid": otherID,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", ownerToken)

	// First promise.
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Second promise (same user, same message) - should not error.
	bodyBytes, _ = json.Marshal(body)
	req = httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err = getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Still only one promise record (REPLACE INTO is idempotent).
	var count int64
	db.Raw("SELECT COUNT(*) FROM messages_promises WHERE msgid = ? AND userid = ?", msgID, otherID).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestPostMessageRenegeWithoutPromise(t *testing.T) {
	// Renege when no promise exists should succeed without error.
	prefix := uniquePrefix("msgw_rng_nop")

	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	_, ownerToken := CreateTestSession(t, ownerID)
	otherID := CreateTestUser(t, prefix+"_other", "User")
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, ownerID, groupID, prefix+" offer item", 52.5, -1.8)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "Renege",
		"userid": otherID,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", ownerToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode, "Renege without existing promise should succeed gracefully")
}

func TestPostMessageOutcomeNoHappiness(t *testing.T) {
	// Outcome without happiness should succeed (happiness is optional).
	prefix := uniquePrefix("msgw_out_noh")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, userID, groupID, prefix+" offer item", 52.5, -1.8)

	body := map[string]interface{}{
		"id":      msgID,
		"action":  "Outcome",
		"outcome": "Taken",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify outcome recorded without happiness.
	var dbOutcome string
	db.Raw("SELECT outcome FROM messages_outcomes WHERE msgid = ?", msgID).Scan(&dbOutcome)
	assert.Equal(t, "Taken", dbOutcome)
}

func TestPostMessageEmptyBody(t *testing.T) {
	prefix := uniquePrefix("msgw_empty")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	// Empty JSON body - should return 400 (missing id).
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostMessageInvalidJSON(t *testing.T) {
	prefix := uniquePrefix("msgw_badjson")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}
