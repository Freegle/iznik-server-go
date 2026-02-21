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

// --- Mod action helpers ---

// createPendingMessage creates a message in Pending collection for mod tests.
func createPendingMessage(t *testing.T, userID uint64, groupID uint64, prefix string) uint64 {
	db := database.DBConn

	var locationID uint64
	db.Raw("SELECT id FROM locations LIMIT 1").Scan(&locationID)

	db.Exec("INSERT INTO messages (fromuser, subject, textbody, type, locationid, arrival, date) VALUES (?, ?, 'Test body', 'Offer', ?, NOW(), NOW())",
		userID, prefix+" pending offer", locationID)

	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? AND subject = ? ORDER BY id DESC LIMIT 1",
		userID, prefix+" pending offer").Scan(&msgID)

	if msgID == 0 {
		t.Fatalf("ERROR: Pending message was created but ID not found")
	}

	db.Exec("INSERT INTO messages_groups (msgid, groupid, arrival, collection, autoreposts) VALUES (?, ?, NOW(), 'Pending', 0)",
		msgID, groupID)

	return msgID
}

// --- Test: Approve ---

func TestPostMessageApprove(t *testing.T) {
	prefix := uniquePrefix("msgmod_appr")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "Approve",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", modToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify collection changed to Approved.
	var collection string
	db.Raw("SELECT collection FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&collection)
	assert.Equal(t, "Approved", collection)

	// Verify approvedby set.
	var approvedby uint64
	db.Raw("SELECT COALESCE(approvedby, 0) FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&approvedby)
	assert.Equal(t, modID, approvedby)

	// Verify heldby cleared.
	var heldby *uint64
	db.Raw("SELECT heldby FROM messages WHERE id = ?", msgID).Scan(&heldby)
	assert.Nil(t, heldby)

	// Verify background task queued.
	var taskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = 'email_message_approved' AND data LIKE ?",
		fmt.Sprintf("%%\"msgid\": %d%%", msgID)).Scan(&taskCount)
	assert.Equal(t, int64(1), taskCount)
}

func TestPostMessageApproveNotMod(t *testing.T) {
	prefix := uniquePrefix("msgmod_appr_nm")

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	regularID := CreateTestUser(t, prefix+"_regular", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, regularID, groupID, "Member")
	_, regularToken := CreateTestSession(t, regularID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "Approve",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", regularToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

// --- Test: Reject ---

func TestPostMessageReject(t *testing.T) {
	prefix := uniquePrefix("msgmod_rej")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)

	body := map[string]interface{}{
		"id":      msgID,
		"action":  "Reject",
		"subject": "Rejection reason",
		"body":    "Please fix your post",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", modToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify pending message_groups entry removed.
	var mgCount int64
	db.Raw("SELECT COUNT(*) FROM messages_groups WHERE msgid = ? AND collection = 'Pending'", msgID).Scan(&mgCount)
	assert.Equal(t, int64(0), mgCount)

	// Verify background task queued.
	var taskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = 'email_message_rejected' AND data LIKE ?",
		fmt.Sprintf("%%\"msgid\": %d%%", msgID)).Scan(&taskCount)
	assert.Equal(t, int64(1), taskCount)
}

// --- Test: Delete (mod action) ---

func TestPostMessageDelete(t *testing.T) {
	prefix := uniquePrefix("msgmod_del")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	msgID := CreateTestMessage(t, posterID, groupID, prefix+" offer item", 52.5, -1.8)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "Delete",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", modToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify messages_groups marked as deleted.
	var mgDeleted int
	db.Raw("SELECT deleted FROM messages_groups WHERE msgid = ?", msgID).Scan(&mgDeleted)
	assert.Equal(t, 1, mgDeleted)

	// Verify message marked as deleted.
	var deleted *string
	db.Raw("SELECT deleted FROM messages WHERE id = ?", msgID).Scan(&deleted)
	assert.NotNil(t, deleted)
}

// --- Test: Spam ---

func TestPostMessageSpam(t *testing.T) {
	prefix := uniquePrefix("msgmod_spam")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "Spam",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", modToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify recorded as spam in messages_spamham.
	var spamham string
	db.Raw("SELECT spamham FROM messages_spamham WHERE msgid = ?", msgID).Scan(&spamham)
	assert.Equal(t, "Spam", spamham)

	// Verify message marked as deleted (spam calls delete in PHP).
	var deleted *string
	db.Raw("SELECT deleted FROM messages WHERE id = ?", msgID).Scan(&deleted)
	assert.NotNil(t, deleted)
}

// --- Test: Hold ---

func TestPostMessageHold(t *testing.T) {
	prefix := uniquePrefix("msgmod_hold")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "Hold",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", modToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify heldby set to mod.
	var heldby uint64
	db.Raw("SELECT COALESCE(heldby, 0) FROM messages WHERE id = ?", msgID).Scan(&heldby)
	assert.Equal(t, modID, heldby)
}

// --- Test: Release ---

func TestPostMessageRelease(t *testing.T) {
	prefix := uniquePrefix("msgmod_rel")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)

	// First hold the message.
	db.Exec("UPDATE messages SET heldby = ? WHERE id = ?", modID, msgID)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "Release",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", modToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify heldby cleared.
	var heldby *uint64
	db.Raw("SELECT heldby FROM messages WHERE id = ?", msgID).Scan(&heldby)
	assert.Nil(t, heldby)
}

// --- Test: ApproveEdits ---

func TestPostMessageApproveEdits(t *testing.T) {
	prefix := uniquePrefix("msgmod_aped")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	msgID := CreateTestMessage(t, posterID, groupID, prefix+" offer item", 52.5, -1.8)

	// Mark as edited.
	db.Exec("UPDATE messages SET editedby = ? WHERE id = ?", posterID, msgID)

	// Create a pending edit.
	newSubject := prefix + " updated subject"
	newText := "Updated body text"
	db.Exec("INSERT INTO messages_edits (msgid, byuser, oldsubject, newsubject, oldtext, newtext, reviewrequired) VALUES (?, ?, ?, ?, 'Old text', ?, 1)",
		msgID, posterID, prefix+" offer item", newSubject, newText)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "ApproveEdits",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", modToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify editedby cleared.
	var editedby *uint64
	db.Raw("SELECT editedby FROM messages WHERE id = ?", msgID).Scan(&editedby)
	assert.Nil(t, editedby)

	// Verify subject and textbody updated.
	var subject, textbody string
	db.Raw("SELECT subject, COALESCE(textbody, '') FROM messages WHERE id = ?", msgID).Row().Scan(&subject, &textbody)
	assert.Equal(t, newSubject, subject)
	assert.Equal(t, newText, textbody)

	// Verify edit marked as approved.
	var approvedCount int64
	db.Raw("SELECT COUNT(*) FROM messages_edits WHERE msgid = ? AND approvedat IS NOT NULL", msgID).Scan(&approvedCount)
	assert.Equal(t, int64(1), approvedCount)
}

// --- Test: RevertEdits ---

func TestPostMessageRevertEdits(t *testing.T) {
	prefix := uniquePrefix("msgmod_rved")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	msgID := CreateTestMessage(t, posterID, groupID, prefix+" offer item", 52.5, -1.8)

	// Mark as edited.
	db.Exec("UPDATE messages SET editedby = ? WHERE id = ?", posterID, msgID)

	// Create a pending edit.
	db.Exec("INSERT INTO messages_edits (msgid, byuser, oldsubject, newsubject, oldtext, newtext, reviewrequired) VALUES (?, ?, ?, ?, 'Old text', 'New text', 1)",
		msgID, posterID, prefix+" offer item", prefix+" changed subject")

	body := map[string]interface{}{
		"id":     msgID,
		"action": "RevertEdits",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", modToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify editedby cleared.
	var editedby *uint64
	db.Raw("SELECT editedby FROM messages WHERE id = ?", msgID).Scan(&editedby)
	assert.Nil(t, editedby)

	// Verify subject NOT changed (reverted, not applied).
	var subject string
	db.Raw("SELECT subject FROM messages WHERE id = ?", msgID).Scan(&subject)
	assert.Equal(t, prefix+" offer item", subject)

	// Verify edit marked as reverted.
	var revertedCount int64
	db.Raw("SELECT COUNT(*) FROM messages_edits WHERE msgid = ? AND revertedat IS NOT NULL", msgID).Scan(&revertedCount)
	assert.Equal(t, int64(1), revertedCount)
}

// --- Test: PartnerConsent ---

func TestPostMessagePartnerConsent(t *testing.T) {
	prefix := uniquePrefix("msgmod_pc")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	msgID := CreateTestMessage(t, posterID, groupID, prefix+" offer item", 52.5, -1.8)

	// Create a partner in partners_keys.
	partnerName := prefix + "_partner"
	db.Exec("INSERT INTO partners_keys (partner, `key`) VALUES (?, ?)", partnerName, "testkey_"+prefix)

	body := map[string]interface{}{
		"id":      msgID,
		"action":  "PartnerConsent",
		"partner": partnerName,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", modToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify partner consent recorded in partners_messages.
	var pmCount int64
	db.Raw("SELECT COUNT(*) FROM partners_messages WHERE msgid = ?", msgID).Scan(&pmCount)
	assert.Equal(t, int64(1), pmCount)
}

// --- Test: Reply ---

func TestPostMessageReply(t *testing.T) {
	prefix := uniquePrefix("msgmod_repl")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)

	body := map[string]interface{}{
		"id":      msgID,
		"action":  "Reply",
		"subject": "Quick note",
		"body":    "Please update your listing",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", modToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify background task queued.
	var taskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = 'email_message_reply' AND data LIKE ?",
		fmt.Sprintf("%%\"msgid\": %d%%", msgID)).Scan(&taskCount)
	assert.Equal(t, int64(1), taskCount)
}

// --- Test: JoinAndPost ---

func TestPostMessageJoinAndPost(t *testing.T) {
	prefix := uniquePrefix("msgmod_jap")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	// User is NOT a member yet.

	body := map[string]interface{}{
		"id":       0, // JoinAndPost needs id=0 bypass -- wait, PostMessage requires id != 0
		"action":   "JoinAndPost",
		"groupid":  groupID,
		"type":     "Offer",
		"item":     "Test chair",
		"textbody": "A nice chair for free",
	}

	// PostMessage requires id != 0. Let's use a placeholder ID.
	// Actually, looking at the code, JoinAndPost creates a NEW message, so the req.ID
	// isn't used for an existing message. But PostMessage checks `req.ID == 0` and returns 400.
	// We need to pass some non-zero id. The handler doesn't use req.ID.
	body["id"] = 1 // Dummy ID -- JoinAndPost doesn't use req.ID to look up a message.

	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.NotNil(t, result["id"])

	newMsgID := uint64(result["id"].(float64))

	// Verify user joined the group.
	var memberCount int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ?", userID, groupID).Scan(&memberCount)
	assert.Equal(t, int64(1), memberCount)

	// Verify message created.
	var msgSubject string
	db.Raw("SELECT subject FROM messages WHERE id = ?", newMsgID).Scan(&msgSubject)
	assert.Equal(t, "Offer: Test chair", msgSubject)

	// Verify message added to group.
	var mgCount int64
	db.Raw("SELECT COUNT(*) FROM messages_groups WHERE msgid = ? AND groupid = ?", newMsgID, groupID).Scan(&mgCount)
	assert.Equal(t, int64(1), mgCount)
}

// --- Test: PatchMessage ---

func TestPatchMessage(t *testing.T) {
	prefix := uniquePrefix("msgmod_patch")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	CreateTestMembership(t, ownerID, groupID, "Member")
	_, ownerToken := CreateTestSession(t, ownerID)

	msgID := createPendingMessage(t, ownerID, groupID, prefix)

	body := map[string]interface{}{
		"id":      msgID,
		"subject": "Updated Subject",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PATCH", "/api/message?jwt="+ownerToken, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify subject was updated.
	var subject string
	db.Raw("SELECT subject FROM messages WHERE id = ?", msgID).Scan(&subject)
	assert.Equal(t, "Updated Subject", subject)

	// Owner edit should create a review record.
	var editCount int64
	db.Raw("SELECT COUNT(*) FROM messages_edits WHERE msgid = ? AND byuser = ?", msgID, ownerID).Scan(&editCount)
	assert.Equal(t, int64(1), editCount)
}

func TestPatchMessageAsMod(t *testing.T) {
	prefix := uniquePrefix("msgmod_patchmod")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)

	body := map[string]interface{}{
		"id":      msgID,
		"subject": "Mod Updated Subject",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PATCH", "/api/message?jwt="+modToken, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Mod edits should NOT create review record.
	var editCount int64
	db.Raw("SELECT COUNT(*) FROM messages_edits WHERE msgid = ? AND byuser = ?", msgID, modID).Scan(&editCount)
	assert.Equal(t, int64(0), editCount)
}

// --- Test: DELETE /message/:id ---

func TestDeleteMessageOwner(t *testing.T) {
	prefix := uniquePrefix("msgmod_delown")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	CreateTestMembership(t, ownerID, groupID, "Member")
	_, ownerToken := CreateTestSession(t, ownerID)

	msgID := createPendingMessage(t, ownerID, groupID, prefix)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/message/%d?jwt=%s", msgID, ownerToken), nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify message is soft-deleted.
	var deleted *string
	db.Raw("SELECT deleted FROM messages WHERE id = ?", msgID).Scan(&deleted)
	assert.NotNil(t, deleted, "Message should be soft-deleted")
}

func TestDeleteMessageMod(t *testing.T) {
	prefix := uniquePrefix("msgmod_delmod")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/message/%d?jwt=%s", msgID, modToken), nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var deleted *string
	db.Raw("SELECT deleted FROM messages WHERE id = ?", msgID).Scan(&deleted)
	assert.NotNil(t, deleted, "Message should be soft-deleted by mod")
}

func TestDeleteMessageNotOwnerNotMod(t *testing.T) {
	prefix := uniquePrefix("msgmod_delfail")

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	_, otherToken := CreateTestSession(t, otherID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/message/%d?jwt=%s", msgID, otherToken), nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

// --- Test: PUT /message ---

func TestPutMessage(t *testing.T) {
	prefix := uniquePrefix("msgmod_put")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	body := map[string]interface{}{
		"groupid":  groupID,
		"type":     "Offer",
		"subject":  prefix + " Test Offer",
		"textbody": "A test offer message",
		"item":     "Test Item",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/message?jwt="+token, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Greater(t, result["id"], float64(0))

	// Verify the message was created.
	newID := uint64(result["id"].(float64))
	var subject string
	db.Raw("SELECT subject FROM messages WHERE id = ?", newID).Scan(&subject)
	assert.Equal(t, prefix+" Test Offer", subject)
}

func TestPutMessageNotMember(t *testing.T) {
	prefix := uniquePrefix("msgmod_putnm")

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	// NOT a member of the group.
	_, token := CreateTestSession(t, userID)

	body := map[string]interface{}{
		"groupid":  groupID,
		"type":     "Offer",
		"subject":  "Should fail",
		"textbody": "Not a member",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/message?jwt="+token, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestPutMessageInvalidType(t *testing.T) {
	prefix := uniquePrefix("msgmod_putbad")

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	body := map[string]interface{}{
		"groupid":  groupID,
		"type":     "Invalid",
		"subject":  "Bad type",
		"textbody": "Invalid type",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/message?jwt="+token, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

// --- Test: System Admin can act as mod ---

func TestPostMessageApproveAsAdmin(t *testing.T) {
	prefix := uniquePrefix("msgmod_appr_adm")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	CreateTestMembership(t, posterID, groupID, "Member")
	// Admin does NOT need to be a member of the group.
	_, adminToken := CreateTestSession(t, adminID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "Approve",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", adminToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify collection changed to Approved.
	var collection string
	db.Raw("SELECT collection FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&collection)
	assert.Equal(t, "Approved", collection)
}
