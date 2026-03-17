package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	// Verify mod log entry created.
	var logCount int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE type = 'Message' AND subtype = 'Approved' AND msgid = ? AND byuser = ?",
		msgID, modID).Scan(&logCount)
	assert.Equal(t, int64(1), logCount, "Approve should create a mod log entry")
}

func TestPostMessageApproveWithStdMsg(t *testing.T) {
	prefix := uniquePrefix("msgmod_appr_std")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, posterID, groupID, "Member")
	_, modToken := CreateTestSession(t, modID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)

	body := map[string]interface{}{
		"id":       msgID,
		"action":   "Approve",
		"groupid":  groupID,
		"subject":  "Welcome to Freegle!",
		"body":     "Thanks for your post.",
		"stdmsgid": 42,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", modToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify background task includes stdmsg fields.
	var taskData string
	db.Raw("SELECT data FROM background_tasks WHERE task_type = 'email_message_approved' AND data LIKE ? ORDER BY id DESC LIMIT 1",
		fmt.Sprintf("%%\"msgid\": %d%%", msgID)).Scan(&taskData)
	assert.Contains(t, taskData, "Welcome to Freegle!", "Task should include subject")
	assert.Contains(t, taskData, "Thanks for your post.", "Task should include body")
	assert.Contains(t, taskData, "42", "Task should include stdmsgid")

	// Verify mod log includes stdmsgid and text.
	var logText string
	var logStdmsgid *uint64
	db.Raw("SELECT text, stdmsgid FROM logs WHERE type = 'Message' AND subtype = 'Approved' AND msgid = ? ORDER BY id DESC LIMIT 1",
		msgID).Row().Scan(&logText, &logStdmsgid)
	assert.Equal(t, "Welcome to Freegle!", logText, "Log text should be the subject, not the body")
	assert.NotEqual(t, "Thanks for your post.", logText, "Log text must NOT be the body")
	assert.NotNil(t, logStdmsgid, "Log should contain stdmsgid")
	assert.Equal(t, uint64(42), *logStdmsgid)
}

func TestPostMessageRejectCreatesLog(t *testing.T) {
	prefix := uniquePrefix("msgmod_rej_log")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, posterID, groupID, "Member")
	_, modToken := CreateTestSession(t, modID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)

	body := map[string]interface{}{
		"id":      msgID,
		"action":  "Reject",
		"groupid": groupID,
		"subject": "Sorry",
		"body":    "Not suitable for this group.",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", modToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify mod log entry.
	var logCount int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE type = 'Message' AND subtype = 'Rejected' AND msgid = ? AND byuser = ?",
		msgID, modID).Scan(&logCount)
	assert.Equal(t, int64(1), logCount, "Reject should create a mod log entry")

	// Verify task includes groupid.
	var taskData string
	db.Raw("SELECT data FROM background_tasks WHERE task_type = 'email_message_rejected' AND data LIKE ? ORDER BY id DESC LIMIT 1",
		fmt.Sprintf("%%\"msgid\": %d%%", msgID)).Scan(&taskData)
	assert.Contains(t, taskData, fmt.Sprintf("\"groupid\": %d", groupID), "Task should include groupid")

	// V1 behavior: reject with subject moves to Rejected collection (not deleted).
	var collection string
	db.Raw("SELECT collection FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&collection)
	assert.Equal(t, "Rejected", collection, "Reject with stdmsg should move to Rejected collection")
}

func TestPostMessageRejectNoSubjectDeletes(t *testing.T) {
	prefix := uniquePrefix("msgmod_rej_del")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, posterID, groupID, "Member")
	_, modToken := CreateTestSession(t, modID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)

	body := map[string]interface{}{
		"id":      msgID,
		"action":  "Reject",
		"groupid": groupID,
		// No subject or body — plain delete.
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", modToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// V1 behavior: reject without subject deletes (sets deleted=1), not Rejected collection.
	var deleted int
	db.Raw("SELECT COALESCE(deleted, 0) FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&deleted)
	assert.Equal(t, 1, deleted, "Reject without stdmsg should mark as deleted")
}

func TestPostMessageApproveMarksHam(t *testing.T) {
	prefix := uniquePrefix("msgmod_appr_ham")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, posterID, groupID, "Member")
	_, modToken := CreateTestSession(t, modID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)

	// Set spamtype on message to simulate it being flagged.
	db.Exec("UPDATE messages SET spamtype = 'Spam' WHERE id = ?", msgID)

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

	// Verify message marked as Ham (matching V1 notSpam behavior).
	var spamham string
	db.Raw("SELECT spamham FROM messages_spamham WHERE msgid = ?", msgID).Scan(&spamham)
	assert.Equal(t, "Ham", spamham, "Approve should mark spam-flagged message as Ham")
}

func TestPostMessageApproveNoSpamham(t *testing.T) {
	prefix := uniquePrefix("msgmod_appr_nosh")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, posterID, groupID, "Member")
	_, modToken := CreateTestSession(t, modID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)
	// Don't set spamtype — message was not flagged as spam.

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

	// No spamham entry should be created for non-spam messages.
	var count int64
	db.Raw("SELECT COUNT(*) FROM messages_spamham WHERE msgid = ?", msgID).Scan(&count)
	assert.Equal(t, int64(0), count, "Non-spam message should not create spamham entry")
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

	// Verify messages_groups row was deleted.
	var mgCount int64
	db.Raw("SELECT COUNT(*) FROM messages_groups WHERE msgid = ?", msgID).Scan(&mgCount)
	assert.Equal(t, int64(0), mgCount)

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

	// Create a test partner.
	partnerName := prefix + "_partner"
	db.Exec("INSERT INTO partners_keys (partner, `key`) VALUES (?, ?)", partnerName, prefix+"_key")
	defer db.Exec("DELETE FROM partners_keys WHERE partner = ?", partnerName)

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

	// Verify partners_messages record created.
	var pmCount int64
	db.Raw("SELECT COUNT(*) FROM partners_messages WHERE msgid = ?", msgID).Scan(&pmCount)
	assert.Equal(t, int64(1), pmCount)
	defer db.Exec("DELETE FROM partners_messages WHERE msgid = ?", msgID)
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

	// Step 1: Create a draft message and store it in messages_drafts.
	// JoinAndPost submits an existing draft (matching the client PUT→POST flow).
	db.Exec("INSERT INTO messages (fromuser, type, subject, textbody, arrival, date, source) VALUES (?, 'Offer', 'Offer: Test chair', 'A nice chair for free', NOW(), NOW(), 'Platform')",
		userID)
	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", userID).Scan(&msgID)
	require.NotZero(t, msgID, "Failed to create test message")
	db.Exec("INSERT INTO messages_drafts (msgid, groupid, userid) VALUES (?, ?, ?)", msgID, groupID, userID)

	// Step 2: Call JoinAndPost to submit the draft.
	body := map[string]interface{}{
		"id":     msgID,
		"action": "JoinAndPost",
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
	assert.Equal(t, float64(0), result["ret"])
	assert.NotNil(t, result["id"])
	assert.Equal(t, float64(msgID), result["id"])

	// Verify user joined the group.
	var memberCount int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ?", userID, groupID).Scan(&memberCount)
	assert.Equal(t, int64(1), memberCount)

	// Verify message added to group as Approved.
	var mgCount int64
	db.Raw("SELECT COUNT(*) FROM messages_groups WHERE msgid = ? AND groupid = ? AND collection = 'Approved'", msgID, groupID).Scan(&mgCount)
	assert.Equal(t, int64(1), mgCount)

	// Verify draft was cleaned up.
	var draftCount int64
	db.Raw("SELECT COUNT(*) FROM messages_drafts WHERE msgid = ?", msgID).Scan(&draftCount)
	assert.Equal(t, int64(0), draftCount)
}

// TestJoinAndPostNewUserPassword verifies that when a new user (no password)
// posts via JoinAndPost, the generated password can be used to log in.
func TestJoinAndPostNewUserPassword(t *testing.T) {
	prefix := uniquePrefix("msgmod_jap_pw")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)

	// Create a user WITHOUT a password (simulates findOrCreateUserForDraft creating a bare user).
	email := prefix + "_new@test.com"
	userID := CreateTestUserWithEmail(t, prefix+"_new", email)
	_, token := CreateTestSession(t, userID)

	// Ensure user has NO Native login (no password).
	db.Exec("DELETE FROM users_logins WHERE userid = ? AND type = 'Native'", userID)

	// Create a draft message.
	db.Exec("INSERT INTO messages (fromuser, type, subject, textbody, arrival, date, source) VALUES (?, 'Offer', 'Offer: Test table', 'A free table', NOW(), NOW(), 'Platform')", userID)
	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", userID).Scan(&msgID)
	require.NotZero(t, msgID)
	db.Exec("INSERT INTO messages_drafts (msgid, groupid, userid) VALUES (?, ?, ?)", msgID, groupID, userID)

	// Call JoinAndPost.
	body := map[string]interface{}{
		"id":     msgID,
		"action": "JoinAndPost",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/message?jwt=%s", token), bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, true, result["newuser"])
	assert.NotEmpty(t, result["newpassword"])

	newPassword := result["newpassword"].(string)

	// Verify the generated password works for login via POST /session.
	loginBody := map[string]interface{}{
		"email":    email,
		"password": newPassword,
	}
	loginBytes, _ := json.Marshal(loginBody)
	loginReq := httptest.NewRequest("POST", "/api/session", bytes.NewBuffer(loginBytes))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := getApp().Test(loginReq)
	require.NoError(t, err)
	assert.Equal(t, 200, loginResp.StatusCode, "Login with generated password should succeed")

	var loginResult map[string]interface{}
	json.NewDecoder(loginResp.Body).Decode(&loginResult)
	assert.NotEmpty(t, loginResult["jwt"], "Login should return a JWT")
	assert.NotNil(t, loginResult["persistent"], "Login should return persistent token")
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

func TestPutMessageNotMemberDraft(t *testing.T) {
	prefix := uniquePrefix("msgmod_putnm")

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	// NOT a member of the group — but drafts don't require membership.
	_, token := CreateTestSession(t, userID)

	body := map[string]interface{}{
		"groupid":  groupID,
		"type":     "Offer",
		"subject":  "Draft by non-member",
		"textbody": "Should succeed as draft",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/message?jwt="+token, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestPutMessageNotMemberNonDraft(t *testing.T) {
	prefix := uniquePrefix("msgmod_putnmd")

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	// NOT a member — non-Draft collection should be rejected.
	_, token := CreateTestSession(t, userID)

	body := map[string]interface{}{
		"groupid":    groupID,
		"type":       "Offer",
		"subject":    "Should fail",
		"textbody":   "Not a member",
		"collection": "Pending",
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

func TestPutMessageExistingEmailNoJWT(t *testing.T) {
	// Security test: PutMessage with an existing user's email must NOT return a JWT.
	// Knowing an email address must not grant authentication.
	prefix := uniquePrefix("msgmod_nojwt")

	// Create a user with a known email.
	email := prefix + "@test.com"
	existingUID := CreateTestUserWithEmail(t, prefix+"_existing", email)
	assert.Greater(t, existingUID, uint64(0))

	groupID := CreateTestGroup(t, prefix)

	// Unauthenticated PUT with that user's email.
	body := map[string]interface{}{
		"type":    "Offer",
		"subject": "Test offer",
		"item":    "Test item",
		"email":   email,
		"groupid": groupID,
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/message", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// CRITICAL: The response must NOT contain a JWT or persistent session.
	_, hasJWT := result["jwt"]
	assert.False(t, hasJWT, "Response must not contain JWT for existing user email")
	_, hasPersistent := result["persistent"]
	assert.False(t, hasPersistent, "Response must not contain persistent session for existing user email")
}

func TestPutMessageNewEmailGetsJWT(t *testing.T) {
	// For a brand-new email, PutMessage should create a user and return a JWT.
	prefix := uniquePrefix("msgmod_newjwt")

	groupID := CreateTestGroup(t, prefix)
	email := prefix + "_brand_new@test.com"

	body := map[string]interface{}{
		"type":    "Offer",
		"subject": "Test offer",
		"item":    "Test item",
		"email":   email,
		"groupid": groupID,
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/message", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// New user SHOULD get a JWT.
	_, hasJWT := result["jwt"]
	assert.True(t, hasJWT, "Response should contain JWT for new user")
}

// --- Test: BackToPending ---

func TestPostMessageBackToPending(t *testing.T) {
	prefix := uniquePrefix("msgmod_btp")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Create and approve a message first.
	msgID := createPendingMessage(t, posterID, groupID, prefix)
	db.Exec("UPDATE messages_groups SET collection = 'Approved', approvedby = ?, approvedat = NOW() WHERE msgid = ?",
		modID, msgID)

	// Verify it's Approved.
	var collection string
	db.Raw("SELECT collection FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&collection)
	assert.Equal(t, "Approved", collection)

	// Now send BackToPending.
	body := map[string]interface{}{
		"id":     msgID,
		"action": "BackToPending",
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

	// Verify collection changed back to Pending.
	db.Raw("SELECT collection FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&collection)
	assert.Equal(t, "Pending", collection)

	// Verify approvedby cleared.
	var approvedby *uint64
	db.Raw("SELECT approvedby FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&approvedby)
	assert.Nil(t, approvedby)
}

func TestPostMessageBackToPendingNotMod(t *testing.T) {
	prefix := uniquePrefix("msgmod_btp_nm")

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	regularID := CreateTestUser(t, prefix+"_regular", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, regularID, groupID, "Member")
	_, regularToken := CreateTestSession(t, regularID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)
	db := database.DBConn
	db.Exec("UPDATE messages_groups SET collection = 'Approved' WHERE msgid = ?", msgID)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "BackToPending",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", regularToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)

	// Verify collection unchanged.
	var collection string
	db.Raw("SELECT collection FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&collection)
	assert.Equal(t, "Approved", collection)
}

// TestApproveCrossPostOnlyAffectsOneGroup verifies that approving a cross-posted message
// with a specific groupid only approves for that group, leaving other groups Pending.
func TestApproveCrossPostOnlyAffectsOneGroup(t *testing.T) {
	prefix := uniquePrefix("msgmod_xpost")
	db := database.DBConn

	group1ID := CreateTestGroup(t, prefix+"_g1")
	group2ID := CreateTestGroup(t, prefix+"_g2")
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, group1ID, "Member")
	CreateTestMembership(t, posterID, group2ID, "Member")
	CreateTestMembership(t, modID, group1ID, "Moderator")
	CreateTestMembership(t, modID, group2ID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Create message pending on both groups (cross-post).
	msgID := createPendingMessage(t, posterID, group1ID, prefix)
	db.Exec("INSERT INTO messages_groups (msgid, groupid, arrival, collection, autoreposts) VALUES (?, ?, NOW(), 'Pending', 0)",
		msgID, group2ID)

	// Approve only for group1.
	body := map[string]interface{}{
		"id":      msgID,
		"action":  "Approve",
		"groupid": group1ID,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", modToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Group1 should be Approved.
	var collection1 string
	db.Raw("SELECT collection FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, group1ID).Scan(&collection1)
	assert.Equal(t, "Approved", collection1)

	// Group2 should still be Pending.
	var collection2 string
	db.Raw("SELECT collection FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, group2ID).Scan(&collection2)
	assert.Equal(t, "Pending", collection2)
}
