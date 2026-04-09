package test

import (
	"bytes"
	json2 "encoding/json"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/log"
	"github.com/freegle/iznik-server-go/message"
	"github.com/freegle/iznik-server-go/queue"
	user2 "github.com/freegle/iznik-server-go/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessages(t *testing.T) {
	// Create test group with messages
	prefix := uniquePrefix("msg")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")

	// Create two messages for the test
	mid := CreateTestMessage(t, userID, groupID, "Test Offer Item 1", 55.9533, -3.1883)
	mid2 := CreateTestMessage(t, userID, groupID, "Test Offer Item 2", 55.9533, -3.1883)

	// Get messages on the group
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groupID)+"/message", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var mids []uint64
	json2.Unmarshal(rsp(resp), &mids)
	assert.Greater(t, len(mids), 0)

	// Get the message
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/"+fmt.Sprint(mid), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msg message.Message
	json2.Unmarshal(rsp(resp), &msg)
	assert.Equal(t, mid, msg.ID)

	// Get the same message multiple times to test the array variant
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/"+fmt.Sprint(mid)+","+fmt.Sprint(mid2), nil))
	assert.Equal(t, 200, resp.StatusCode)

	messages := []message.Message{}
	json2.Unmarshal(rsp(resp), &messages)
	assert.Equal(t, 2, len(messages))
	assert.True(t, (messages[0].ID == mid && messages[1].ID == mid2) || (messages[0].ID == mid2 && messages[1].ID == mid))

	// Test too many
	url := "/api/message/"
	for i := 0; i < 30; i++ {
		url += fmt.Sprint(mid) + ","
	}
	resp, _ = getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 400, resp.StatusCode)

	// Get the user
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(userID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var u user2.User
	json2.Unmarshal(rsp(resp), &u)
	assert.Equal(t, userID, u.ID)
	assert.Greater(t, len(u.Displayname), 0)

	// Shouldn't see memberships without auth
	assert.Equal(t, len(u.Memberships), 0)

	// Get invalid message/user - use very high IDs guaranteed not to exist
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/999999999", nil))
	assert.Equal(t, 404, resp.StatusCode)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/999999999", nil))
	assert.Equal(t, 404, resp.StatusCode)

	// Get the message as the sender
	midArray := []string{fmt.Sprint(mid)}
	msgDetails := message.GetMessagesByIds(userID, midArray)[0]
	assert.Equal(t, mid, msgDetails.ID)
}

func TestBounds(t *testing.T) {
	// Create a message in specific bounds for this test
	prefix := uniquePrefix("bounds")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	CreateTestMessage(t, userID, groupID, "Test Bounds Item", 55.9533, -3.1883)

	// Get within the bounds
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/message/inbounds?swlat=55&swlng=-3.5&nelat=56&nelng=-3", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msgs []message.MessageSummary
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Greater(t, len(msgs), 0)

	// Repeat but logged in
	_, token := CreateFullTestUser(t, prefix+"_auth")
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/inbounds?swlat=55&swlng=-3.5&nelat=56&nelng=-3&jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Greater(t, len(msgs), 0)

	// Get outside bounds
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/inbounds?swlng=55&swlat=-3.5&nelng=56&nelat=-3", nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Equal(t, len(msgs), 0)
}

func TestMyGroups(t *testing.T) {
	// Get logged out - should return 401
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/message/mygroups", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Create a full test user with group membership and message
	prefix := uniquePrefix("mygroups")
	userID, token := CreateFullTestUser(t, prefix)

	// Create a group the user is in with a message
	groupID := CreateTestGroup(t, prefix+"_grp")
	CreateTestMembership(t, userID, groupID, "Member")
	CreateTestMessage(t, userID, groupID, "Test MyGroups Item", 55.9533, -3.1883)

	// Should be able to fetch messages in our groups
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/mygroups?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msgs []message.MessageSummary
	json2.Unmarshal(rsp(resp), &msgs)
	// We expect at least some messages (could be from other tests too)
}

func TestMessagesByUser(t *testing.T) {
	// Create a user with a message
	prefix := uniquePrefix("usermsg")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	CreateTestMessage(t, userID, groupID, "Test User Message", 55.9533, -3.1883)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(userID)+"/message", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msgs []message.MessageSummary
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Greater(t, len(msgs), 0)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(userID)+"/message?active=true", nil))
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &msgs)
	assert.Greater(t, len(msgs), 0)

	// Invalid user
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/z/message", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestActiveQueryExcludesExpiredMessages(t *testing.T) {
	prefix := uniquePrefix("expire")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	// Recent message (1 day old) — should appear in active.
	recentID := CreateTestMessageWithArrival(t, userID, groupID, "OFFER: Fresh Sofa", 55.9533, -3.1883, 1)

	// Old message (200 days old, well past default 90-day Offer expiry) — should NOT appear in active.
	oldID := CreateTestMessageWithArrival(t, userID, groupID, "OFFER: Ancient Chair", 55.9533, -3.1883, 200)

	// Active query should include recent, exclude old.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(userID)+"/message?active=true&jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msgs []message.MessageSummary
	json2.Unmarshal(rsp(resp), &msgs)

	foundRecent := false
	foundOld := false
	for _, m := range msgs {
		if m.ID == recentID {
			foundRecent = true
		}
		if m.ID == oldID {
			foundOld = true
		}
	}
	assert.True(t, foundRecent, "Recent message should appear in active query")
	assert.False(t, foundOld, "Expired message should be excluded from active query")

	// Non-active query should return both, with old marked as hasoutcome=true.
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(userID)+"/message?active=false&jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &msgs)

	for _, m := range msgs {
		if m.ID == recentID {
			assert.False(t, m.Hasoutcome, "Recent message should not have hasoutcome set")
		}
		if m.ID == oldID {
			assert.True(t, m.Hasoutcome, "Expired message should have hasoutcome=true in non-active query")
		}
	}
}

func TestExpiredMessageKeptAliveByPromise(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("exprms")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	promiserID := CreateTestUser(t, prefix, "Promiser")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	// Old message (200 days) with a promise — should still appear active.
	msgID := CreateTestMessageWithArrival(t, userID, groupID, "OFFER: Promised Table", 55.9533, -3.1883, 200)
	db.Exec("INSERT INTO messages_promises (msgid, userid) VALUES (?, ?)", msgID, promiserID)
	t.Cleanup(func() {
		db.Exec("DELETE FROM messages_promises WHERE msgid = ?", msgID)
	})

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(userID)+"/message?active=true&jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msgs []message.MessageSummary
	json2.Unmarshal(rsp(resp), &msgs)

	found := false
	for _, m := range msgs {
		if m.ID == msgID {
			found = true
		}
	}
	assert.True(t, found, "Expired message with promise should still appear in active query")
}

func TestRejectedMessageInActiveQuery(t *testing.T) {
	// Rejected messages should appear in the active query for own messages
	// so users can see them on My Posts and edit/resend them.
	prefix := uniquePrefix("rjctmsg")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	db := database.DBConn

	// Create a message and set it to Rejected (no spatial index entry).
	msgID := CreateTestMessage(t, userID, groupID, "OFFER: Rejected Chair", 55.9533, -3.1883)
	db.Exec("UPDATE messages_groups SET collection = 'Rejected' WHERE msgid = ?", msgID)
	db.Exec("DELETE FROM messages_spatial WHERE msgid = ?", msgID)

	// Active query for own user should include the rejected message.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(userID)+"/message?active=true&jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msgs []message.MessageSummary
	json2.Unmarshal(rsp(resp), &msgs)

	found := false
	for _, m := range msgs {
		if m.ID == msgID {
			found = true
			break
		}
	}
	assert.True(t, found, "Rejected message should appear in active query for own user")
}

func TestCount(t *testing.T) {
	// Create a full test user for count endpoint
	prefix := uniquePrefix("count")
	_, token := CreateFullTestUser(t, prefix)

	var count int

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/message/count?browseView=mygroups&jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &count)
	// Count can be 0 for a new user

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/count?browseView=nearby&jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &count)
	// Count can be 0 for a new user
}

func TestActivity(t *testing.T) {
	// Create some activity data
	prefix := uniquePrefix("activity")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	CreateTestMessage(t, userID, groupID, "Test Activity Item", 55.9533, -3.1883)

	// Get recent activity
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/activity", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var activity []message.Activity
	json2.Unmarshal(rsp(resp), &activity)
	assert.Greater(t, len(activity), 0)
	assert.Greater(t, activity[0].ID, uint64(0))
}

func TestMessageUnseenStatus(t *testing.T) {
	// Test that messages are correctly marked as unseen/seen based on messages_likes View entries
	prefix := uniquePrefix("unseen")
	groupID := CreateTestGroup(t, prefix)

	// Create message owner
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	CreateTestMembership(t, ownerID, groupID, "Member")

	// Create a viewer who will mark the message as seen
	viewerID := CreateTestUser(t, prefix+"_viewer", "User")
	CreateTestMembership(t, viewerID, groupID, "Member")
	_, viewerToken := CreateTestSession(t, viewerID)

	// Create a message
	msgID := CreateTestMessage(t, ownerID, groupID, "Test Unseen Item", 55.9533, -3.1883)

	// Get owner's messages as viewer - should show unseen=true (no View record exists)
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(ownerID)+"/message?jwt="+viewerToken, nil))
	assert.Equal(t, 200, resp.StatusCode)

	type MessageWithUnseen struct {
		ID     uint64 `json:"id"`
		Unseen bool   `json:"unseen"`
	}

	var msgs []MessageWithUnseen
	json2.Unmarshal(rsp(resp), &msgs)

	// Find our message
	var foundMsg *MessageWithUnseen
	for i, m := range msgs {
		if m.ID == msgID {
			foundMsg = &msgs[i]
			break
		}
	}
	assert.NotNil(t, foundMsg, "Message should be found in user's messages")
	assert.True(t, foundMsg.Unseen, "Message should be unseen before viewing")

	// Mark the message as viewed by the viewer
	MarkMessageAsViewed(t, viewerID, msgID)

	// Get owner's messages again as viewer - should now show unseen=false
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(ownerID)+"/message?jwt="+viewerToken, nil))
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &msgs)

	// Find our message again
	foundMsg = nil
	for i, m := range msgs {
		if m.ID == msgID {
			foundMsg = &msgs[i]
			break
		}
	}
	assert.NotNil(t, foundMsg, "Message should still be found in user's messages")
	assert.False(t, foundMsg.Unseen, "Message should be seen after viewing")
}

// =============================================================================
// Additional auth & error tests for partial-coverage endpoints
// =============================================================================

func TestGroupMessages_WithAuth(t *testing.T) {
	// Test that authenticated user sees their own pending messages in group
	prefix := uniquePrefix("grpmsgauth")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	// Create a message (will be approved in test setup)
	CreateTestMessage(t, userID, groupID, "Test Auth Group Msg", 55.9533, -3.1883)

	// With auth - should include own messages
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groupID)+"/message?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var mids []uint64
	json2.Unmarshal(rsp(resp), &mids)
	assert.Greater(t, len(mids), 0)
}

func TestGroupMessages_InvalidGroupID(t *testing.T) {
	// Non-integer group ID should return empty array (handler parses 0)
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/notanint/message", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var mids []uint64
	json2.Unmarshal(rsp(resp), &mids)
	assert.Equal(t, 0, len(mids))
}

func TestBounds_MissingParams(t *testing.T) {
	// Missing all required bounds params - should return empty (defaults to 0,0,0,0)
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/message/inbounds", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msgs []message.MessageSummary
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Equal(t, 0, len(msgs))
}

func TestBounds_PartialParams(t *testing.T) {
	// Only some bounds params provided
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/message/inbounds?swlat=55", nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestActivity_V2Path(t *testing.T) {
	// Verify v2 path works
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/activity", nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestMessagesByUser_NonExistentUser(t *testing.T) {
	// User ID that doesn't exist should return 200 with empty array
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/999999999/message", nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestMessageWithoutGroupNotAccessible(t *testing.T) {
	// Test that messages without an entry in messages_groups cannot be fetched via the public API
	// This prevents internal messages (like chat messages) from being exposed publicly
	prefix := uniquePrefix("nogroup")

	// Create a user
	userID := CreateTestUser(t, prefix, "User")

	// Create a message WITHOUT a messages_groups entry
	msgID := CreateTestMessageWithoutGroup(t, userID, "Private Chat Message")

	// Try to fetch the message - should return 404 since it has no group association
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/message/"+fmt.Sprint(msgID), nil))
	assert.Equal(t, 404, resp.StatusCode, "Message without messages_groups entry should not be accessible")
}

func TestMessageModOnlyFields(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("msg_modfields")

	// Create group, regular user, and mod user.
	groupID := CreateTestGroup(t, prefix)
	regularUserID := CreateTestUser(t, prefix+"_reg", "User")
	modUserID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, regularUserID, groupID, "Member")
	CreateTestMembership(t, modUserID, groupID, "Moderator")
	_, regularToken := CreateTestSession(t, regularUserID)
	_, modToken := CreateTestSession(t, modUserID)

	// Create a message with source/fromip/fromcountry set.
	msgID := CreateTestMessage(t, regularUserID, groupID, "Test Mod Fields Item", 55.9533, -3.1883)
	db.Exec("UPDATE messages SET source = 'Platform', sourceheader = 'Freegle App', fromaddr = 'test@users.ilovefreegle.org', fromip = '1.2.3.4', fromcountry = 'GB' WHERE id = ?", msgID)

	// Fetch as mod — should see source/fromip/fromcountry.
	resp, err := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/message/%d?jwt=%s", msgID, modToken), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var modMsg message.Message
	json2.Unmarshal(rsp(resp), &modMsg)
	assert.NotNil(t, modMsg.Source, "Mod should see source")
	assert.Equal(t, "Platform", *modMsg.Source)
	assert.NotNil(t, modMsg.Fromip, "Mod should see fromip")
	assert.Equal(t, "1.2.3.4", *modMsg.Fromip)
	assert.NotNil(t, modMsg.Fromcountry, "Mod should see fromcountry")

	// Fetch as regular user — should NOT see source/fromip/fromcountry.
	resp, err = getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/message/%d?jwt=%s", msgID, regularToken), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var regMsg message.Message
	json2.Unmarshal(rsp(resp), &regMsg)
	assert.Nil(t, regMsg.Source, "Regular user should NOT see source")
	assert.Nil(t, regMsg.Fromip, "Regular user should NOT see fromip")
	assert.Nil(t, regMsg.Fromcountry, "Regular user should NOT see fromcountry")
	assert.Nil(t, regMsg.Fromaddr, "Regular user should NOT see fromaddr")

	// Fetch without auth — should NOT see source/fromip/fromcountry.
	resp, err = getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/message/%d", msgID), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var anonMsg message.Message
	json2.Unmarshal(rsp(resp), &anonMsg)
	assert.Nil(t, anonMsg.Source, "Anonymous user should NOT see source")
	assert.Nil(t, anonMsg.Fromip, "Anonymous user should NOT see fromip")
	assert.Nil(t, anonMsg.Fromcountry, "Anonymous user should NOT see fromcountry")
}

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

	// Log creation and push notifications are now handled by the batch processor
	// (not synchronously in the Go API), so no log or push_notify_group_mods assertions here.
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

	// Log creation is now handled by the batch processor (not synchronously in the Go API).
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

	// Verify task includes groupid.
	var taskData string
	db.Raw("SELECT data FROM background_tasks WHERE task_type = 'email_message_rejected' AND data LIKE ? ORDER BY id DESC LIMIT 1",
		fmt.Sprintf("%%\"msgid\": %d%%", msgID)).Scan(&taskData)
	assert.Contains(t, taskData, fmt.Sprintf("\"groupid\": %d", groupID), "Task should include groupid")

	// V1 behavior: reject with subject moves to Rejected collection (not deleted).
	var collection string
	db.Raw("SELECT collection FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&collection)
	assert.Equal(t, "Rejected", collection, "Reject with stdmsg should move to Rejected collection")

	// Log creation is now handled by the batch processor (not synchronously in the Go API).
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

	// Push notifications are now queued by the batch processor, not synchronously by the Go API.
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

	// Verify background task queued (for log+push in batch processor).
	var taskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = 'email_message_rejected' AND data LIKE ?",
		fmt.Sprintf("%%\"msgid\": %d%%", msgID)).Scan(&taskCount)
	assert.Equal(t, int64(1), taskCount, "Delete should queue background task for logging and push")
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

	// Verify push_notify_group_mods background task was queued.
	var pushTaskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = ? AND processed_at IS NULL AND data LIKE ?",
		queue.TaskPushNotifyGroupMods, fmt.Sprintf("%%group_id%%%d%%", groupID)).Scan(&pushTaskCount)
	assert.Equal(t, int64(1), pushTaskCount, "Hold should queue push_notify_group_mods task")
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

	// Verify push_notify_group_mods background task was queued.
	var pushTaskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = ? AND processed_at IS NULL AND data LIKE ?",
		queue.TaskPushNotifyGroupMods, fmt.Sprintf("%%group_id%%%d%%", groupID)).Scan(&pushTaskCount)
	assert.Equal(t, int64(1), pushTaskCount, "Release should queue push_notify_group_mods task")
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

	// Verify edit marked as approved with reviewrequired = 0.
	var approvedCount int64
	db.Raw("SELECT COUNT(*) FROM messages_edits WHERE msgid = ? AND approvedat IS NOT NULL AND reviewrequired = 0", msgID).Scan(&approvedCount)
	assert.Equal(t, int64(1), approvedCount)

	// Verify it no longer appears in the V1-style count query (which only checks reviewrequired).
	var pendingEditCount int64
	db.Raw("SELECT COUNT(*) FROM messages_edits WHERE msgid = ? AND reviewrequired = 1", msgID).Scan(&pendingEditCount)
	assert.Equal(t, int64(0), pendingEditCount, "Approved edit should not appear in V1 count query")
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

	// Verify edit marked as reverted with reviewrequired = 0.
	var revertedCount int64
	db.Raw("SELECT COUNT(*) FROM messages_edits WHERE msgid = ? AND revertedat IS NOT NULL AND reviewrequired = 0", msgID).Scan(&revertedCount)
	assert.Equal(t, int64(1), revertedCount)

	// Verify it no longer appears in the V1-style count query.
	var pendingEditCount int64
	db.Raw("SELECT COUNT(*) FROM messages_edits WHERE msgid = ? AND reviewrequired = 1", msgID).Scan(&pendingEditCount)
	assert.Equal(t, int64(0), pendingEditCount, "Reverted edit should not appear in V1 count query")
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

	// Verify message added to group as Pending.
	var mgCount int64
	db.Raw("SELECT COUNT(*) FROM messages_groups WHERE msgid = ? AND groupid = ? AND collection = 'Pending'", msgID, groupID).Scan(&mgCount)
	assert.Equal(t, int64(1), mgCount)

	// Verify draft was cleaned up.
	var draftCount int64
	db.Raw("SELECT COUNT(*) FROM messages_drafts WHERE msgid = ?", msgID).Scan(&draftCount)
	assert.Equal(t, int64(0), draftCount)

	// Verify membership join was logged.
	var logCount int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE type = 'Group' AND subtype = 'Joined' AND user = ? AND groupid = ?", userID, groupID).Scan(&logCount)
	assert.Equal(t, int64(1), logCount, "JoinAndPost should create a Joined log entry")
}

func TestJoinAndPostSavesDeadline(t *testing.T) {
	prefix := uniquePrefix("msgmod_jap_dl")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	// Create a draft message.
	db.Exec("INSERT INTO messages (fromuser, type, subject, textbody, arrival, date, source) VALUES (?, 'Offer', 'Offer: Deadline test', 'Item with deadline', NOW(), NOW(), 'Platform')",
		userID)
	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", userID).Scan(&msgID)
	require.NotZero(t, msgID)
	db.Exec("INSERT INTO messages_drafts (msgid, groupid, userid) VALUES (?, ?, ?)", msgID, groupID, userID)

	// JoinAndPost with deadline.
	body := map[string]interface{}{
		"id":       msgID,
		"action":   "JoinAndPost",
		"deadline": "2026-07-15",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/message?jwt=%s", token), bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify deadline was saved.
	var deadline *string
	db.Raw("SELECT DATE_FORMAT(deadline, '%Y-%m-%d') FROM messages WHERE id = ?", msgID).Scan(&deadline)
	assert.NotNil(t, deadline, "Deadline should be saved during JoinAndPost")
	assert.Equal(t, "2026-07-15", *deadline)
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

// TestJoinAndPostModeratedUserGoesToPending verifies that when a user has
// ourPostingStatus='MODERATED', their message goes to Pending instead of Approved.
func TestJoinAndPostModeratedUserGoesToPending(t *testing.T) {
	prefix := uniquePrefix("msgmod_jap_mod")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	// Pre-create membership with MODERATED posting status.
	CreateTestMembership(t, userID, groupID, "Member")
	db.Exec("UPDATE memberships SET ourPostingStatus = 'MODERATED' WHERE userid = ? AND groupid = ?", userID, groupID)

	// Create a draft message.
	db.Exec("INSERT INTO messages (fromuser, type, subject, textbody, arrival, date, source) VALUES (?, 'Offer', 'Offer: Moderated chair', 'A chair', NOW(), NOW(), 'Platform')", userID)
	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", userID).Scan(&msgID)
	require.NotZero(t, msgID)
	db.Exec("INSERT INTO messages_drafts (msgid, groupid, userid) VALUES (?, ?, ?)", msgID, groupID, userID)

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
	assert.Equal(t, float64(0), result["ret"])

	// Message should be in Pending, not Approved.
	var collection string
	db.Raw("SELECT collection FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&collection)
	assert.Equal(t, "Pending", collection, "MODERATED user's message should go to Pending")
}

// TestJoinAndPostBannedUserReturns403 verifies that a banned user cannot post.
func TestJoinAndPostBannedUserReturns403(t *testing.T) {
	prefix := uniquePrefix("msgmod_jap_ban")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	// Create a Banned membership.
	db.Exec("INSERT INTO memberships (userid, groupid, role, collection) VALUES (?, ?, 'Member', 'Banned')", userID, groupID)

	// Create a draft message.
	db.Exec("INSERT INTO messages (fromuser, type, subject, textbody, arrival, date, source) VALUES (?, 'Offer', 'Offer: Banned chair', 'A chair', NOW(), NOW(), 'Platform')", userID)
	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", userID).Scan(&msgID)
	require.NotZero(t, msgID)
	db.Exec("INSERT INTO messages_drafts (msgid, groupid, userid) VALUES (?, ?, ?)", msgID, groupID, userID)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "JoinAndPost",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/message?jwt=%s", token), bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode, "Banned user should get 403")
}

// TestJoinAndPostProhibitedUserReturns403 verifies that a PROHIBITED user cannot post.
func TestJoinAndPostProhibitedUserReturns403(t *testing.T) {
	prefix := uniquePrefix("msgmod_jap_proh")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	// Create membership with PROHIBITED posting status.
	CreateTestMembership(t, userID, groupID, "Member")
	db.Exec("UPDATE memberships SET ourPostingStatus = 'PROHIBITED' WHERE userid = ? AND groupid = ?", userID, groupID)

	// Create a draft message.
	db.Exec("INSERT INTO messages (fromuser, type, subject, textbody, arrival, date, source) VALUES (?, 'Offer', 'Offer: Prohibited chair', 'A chair', NOW(), NOW(), 'Platform')", userID)
	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", userID).Scan(&msgID)
	require.NotZero(t, msgID)
	db.Exec("INSERT INTO messages_drafts (msgid, groupid, userid) VALUES (?, ?, ?)", msgID, groupID, userID)

	body := map[string]interface{}{
		"id":     msgID,
		"action": "JoinAndPost",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/message?jwt=%s", token), bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode, "PROHIBITED user should get 403")
}

// TestJoinAndPostGroupDefaultModerated verifies that when a group has
// defaultpostingstatus=MODERATED and user has no explicit posting status,
// the message goes to Pending.
func TestJoinAndPostGroupDefaultModerated(t *testing.T) {
	prefix := uniquePrefix("msgmod_jap_gmod")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	// Set the group's default posting status to MODERATED.
	db.Exec("UPDATE `groups` SET settings = JSON_SET(COALESCE(settings, '{}'), '$.defaultpostingstatus', 'MODERATED') WHERE id = ?", groupID)

	// User is NOT a member yet (JoinAndPost will create the membership).

	// Create a draft message.
	db.Exec("INSERT INTO messages (fromuser, type, subject, textbody, arrival, date, source) VALUES (?, 'Offer', 'Offer: GroupMod chair', 'A chair', NOW(), NOW(), 'Platform')", userID)
	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", userID).Scan(&msgID)
	require.NotZero(t, msgID)
	db.Exec("INSERT INTO messages_drafts (msgid, groupid, userid) VALUES (?, ?, ?)", msgID, groupID, userID)

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
	assert.Equal(t, float64(0), result["ret"])

	// Message should be in Pending because group default is MODERATED.
	var collection string
	db.Raw("SELECT collection FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&collection)
	assert.Equal(t, "Pending", collection, "Group default MODERATED should send message to Pending")
}

// TestJoinAndPostForcePendingOverridesApproved verifies that forcepending=true
// sends an otherwise-approved message to Pending.
func TestJoinAndPostForcePendingOverridesApproved(t *testing.T) {
	prefix := uniquePrefix("msgmod_jap_fp")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	// User has UNMODERATED posting status — would normally go to Approved.
	CreateTestMembership(t, userID, groupID, "Member")

	// Create a draft message.
	db.Exec("INSERT INTO messages (fromuser, type, subject, textbody, arrival, date, source) VALUES (?, 'Offer', 'Offer: Forced pending sofa', 'A sofa', NOW(), NOW(), 'Platform')", userID)
	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", userID).Scan(&msgID)
	require.NotZero(t, msgID)
	db.Exec("INSERT INTO messages_drafts (msgid, groupid, userid) VALUES (?, ?, ?)", msgID, groupID, userID)

	body := map[string]interface{}{
		"id":           msgID,
		"action":       "JoinAndPost",
		"forcepending": true,
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/message?jwt=%s", token), bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Message should be in Pending despite user being unmoderated.
	var collection string
	db.Raw("SELECT collection FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&collection)
	assert.Equal(t, "Pending", collection, "forcepending=true should send message to Pending")
}

// TestJoinAndPostForcePendingFalseDoesNotOverride verifies that forcepending=false
// does not bypass moderation — a MODERATED user still goes to Pending.
func TestJoinAndPostForcePendingFalseDoesNotOverride(t *testing.T) {
	prefix := uniquePrefix("msgmod_jap_fpf")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	// User is explicitly MODERATED.
	CreateTestMembership(t, userID, groupID, "Member")
	db.Exec("UPDATE memberships SET ourPostingStatus = 'MODERATED' WHERE userid = ? AND groupid = ?", userID, groupID)

	// Create a draft message.
	db.Exec("INSERT INTO messages (fromuser, type, subject, textbody, arrival, date, source) VALUES (?, 'Offer', 'Offer: Still pending desk', 'A desk', NOW(), NOW(), 'Platform')", userID)
	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", userID).Scan(&msgID)
	require.NotZero(t, msgID)
	db.Exec("INSERT INTO messages_drafts (msgid, groupid, userid) VALUES (?, ?, ?)", msgID, groupID, userID)

	body := map[string]interface{}{
		"id":           msgID,
		"action":       "JoinAndPost",
		"forcepending": false,
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/message?jwt=%s", token), bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Message should still be Pending — forcepending=false cannot override moderation.
	var collection string
	db.Raw("SELECT collection FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&collection)
	assert.Equal(t, "Pending", collection, "forcepending=false must not override MODERATED status")
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

func TestGetMessageReturnsEditsForMod(t *testing.T) {
	prefix := uniquePrefix("msg_get_edits")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")

	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Non-mod user (systemrole User, group role Member)
	otherID := CreateTestUser(t, prefix+"_other", "User")
	CreateTestMembership(t, otherID, groupID, "Member")
	_, otherToken := CreateTestSession(t, otherID)

	msgID := CreateTestMessage(t, posterID, groupID, prefix+" item", 52.5, -1.8)

	// Create a pending edit with oldtext and newtext.
	db.Exec("INSERT INTO messages_edits (msgid, byuser, oldtext, newtext, reviewrequired, timestamp) VALUES (?, ?, 'Old body text', 'New body text', 1, NOW())",
		msgID, posterID)

	// Fetch as mod — should see edits with oldtext/newtext.
	resp, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/message/%d?jwt=%s", msgID, modToken), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msg map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&msg)

	edits, hasEdits := msg["edits"]
	assert.True(t, hasEdits, "Mod should see edits field")

	editList := edits.([]interface{})
	assert.Equal(t, 1, len(editList), "Should have 1 pending edit")

	edit := editList[0].(map[string]interface{})
	assert.Equal(t, "Old body text", edit["oldtext"])
	assert.Equal(t, "New body text", edit["newtext"])

	// Fetch as non-mod — should NOT see edits.
	resp2, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/message/%d?jwt=%s", msgID, otherToken), nil))
	assert.Equal(t, 200, resp2.StatusCode)

	var msg2 map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&msg2)

	_, hasEdits2 := msg2["edits"]
	assert.False(t, hasEdits2, "Non-mod should NOT see edits field")

	// Cleanup
	db.Exec("DELETE FROM messages_edits WHERE msgid = ?", msgID)
}

func TestGetMessageReturnsLocationForMod(t *testing.T) {
	prefix := uniquePrefix("msg_get_loc")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")

	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	msgID := CreateTestMessage(t, posterID, groupID, prefix+" item", 52.5, -1.8)

	// Create a location and assign it to the message.
	db.Exec("INSERT INTO locations (name, type, lat, lng) VALUES (?, 'Postcode', 52.5, -1.8)", prefix+"_PC")
	var locID uint64
	db.Raw("SELECT id FROM locations WHERE name = ? ORDER BY id DESC LIMIT 1", prefix+"_PC").Scan(&locID)
	assert.Greater(t, locID, uint64(0), "Location should be created")
	db.Exec("UPDATE messages SET locationid = ? WHERE id = ?", locID, msgID)

	// Fetch as mod — location should have correct lat/lng from the location record.
	resp, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/message/%d?jwt=%s", msgID, modToken), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msg map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&msg)

	loc, hasLoc := msg["location"]
	assert.True(t, hasLoc, "Mod should see location")

	locMap := loc.(map[string]interface{})
	assert.NotEqual(t, float64(0), locMap["lat"], "Location lat should not be 0")
	assert.NotEqual(t, float64(0), locMap["lng"], "Location lng should not be 0")
	assert.InDelta(t, 52.5, locMap["lat"].(float64), 0.01, "Location lat should match")
	assert.InDelta(t, -1.8, locMap["lng"].(float64), 0.01, "Location lng should match")

	// Fetch as non-mod — should NOT see precise location (privacy).
	otherID := CreateTestUser(t, prefix+"_other", "User")
	CreateTestMembership(t, otherID, groupID, "Member")
	_, otherToken := CreateTestSession(t, otherID)

	resp2, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/message/%d?jwt=%s", msgID, otherToken), nil))
	assert.Equal(t, 200, resp2.StatusCode)

	var msg2 map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&msg2)

	_, hasLoc2 := msg2["location"]
	assert.False(t, hasLoc2, "Non-mod should NOT see precise location")
}

func TestPatchMessageRejectedToPending(t *testing.T) {
	prefix := uniquePrefix("msgmod_patchrej")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	CreateTestMembership(t, ownerID, groupID, "Member")
	_, ownerToken := CreateTestSession(t, ownerID)

	// Create a message in Rejected collection (simulates a mod-rejected message).
	msgID := createPendingMessage(t, ownerID, groupID, prefix)
	db.Exec("UPDATE messages_groups SET collection = 'Rejected', rejectedat = NOW() WHERE msgid = ? AND groupid = ?", msgID, groupID)

	// Verify it's Rejected before the PATCH.
	var collBefore string
	db.Raw("SELECT collection FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&collBefore)
	require.Equal(t, "Rejected", collBefore, "Setup: message should be Rejected")

	body := map[string]interface{}{
		"id":      msgID,
		"subject": "Edited After Rejection",
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

	// Issue 1: After PATCH on a rejected message, collection should become Pending.
	var collAfter string
	db.Raw("SELECT collection FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&collAfter)
	assert.Equal(t, "Pending", collAfter, "Editing a rejected message should move it back to Pending")
}

func TestPatchMessageLogEntry(t *testing.T) {
	prefix := uniquePrefix("msgmod_patchlog")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	CreateTestMembership(t, ownerID, groupID, "Member")
	_, ownerToken := CreateTestSession(t, ownerID)

	msgID := createPendingMessage(t, ownerID, groupID, prefix)

	body := map[string]interface{}{
		"id":      msgID,
		"subject": "Log Entry Test Subject",
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

	// Issue 2: After PATCH, a log entry should exist with type='Message', subtype='Edit'.
	var logCount int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE type = ? AND subtype = ? AND msgid = ? AND byuser = ?",
		log.LOG_TYPE_MESSAGE, log.LOG_SUBTYPE_EDIT, msgID, ownerID).Scan(&logCount)
	assert.Equal(t, int64(1), logCount, "PATCH should create a log entry with type='Message', subtype='Edit'")
}

// --- Test: DELETE /message/:id ---

func TestPatchMessageLocationName(t *testing.T) {
	prefix := uniquePrefix("msgmod_patchloc")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	msgID := CreateTestMessage(t, userID, groupID, prefix+" Test Item", 53.0, -1.0)

	// Find a location name to use.
	var locName string
	var locID uint64
	db.Raw("SELECT id, name FROM locations WHERE name LIKE '% %' LIMIT 1").Row().Scan(&locID, &locName)
	if locID == 0 {
		t.Fatal("No locations in test database")
	}

	// PATCH with location name (not locationid) — should resolve to locationid.
	body, _ := json.Marshal(map[string]interface{}{
		"id":       msgID,
		"subject":  prefix + " Edited Subject",
		"location": locName,
	})
	req := httptest.NewRequest("PATCH", "/api/message?jwt="+modToken, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify subject was updated.
	var subject string
	db.Raw("SELECT subject FROM messages WHERE id = ?", msgID).Scan(&subject)
	assert.Equal(t, prefix+" Edited Subject", subject)

	// Verify locationid was set from the location name.
	var msgLocID uint64
	db.Raw("SELECT COALESCE(locationid, 0) FROM messages WHERE id = ?", msgID).Scan(&msgLocID)
	assert.Equal(t, locID, msgLocID, "locationid should be resolved from location name")
}

func TestPatchMessageExtendDeadlineClearsExpiredOutcome(t *testing.T) {
	prefix := uniquePrefix("msgpatch_extend")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	CreateTestMembership(t, ownerID, groupID, "Member")
	_, ownerToken := CreateTestSession(t, ownerID)

	msgID := CreateTestMessage(t, ownerID, groupID, prefix+" Test Item", 53.0, -1.0)

	// Simulate batch job: set a past deadline and insert an Expired outcome.
	db.Exec("UPDATE messages SET deadline = '2026-01-01' WHERE id = ?", msgID)
	db.Exec("INSERT INTO messages_outcomes (msgid, outcome, comments, timestamp) VALUES (?, 'Expired', 'Reached deadline', NOW())", msgID)
	// Simulate an in-progress intended outcome (e.g. user started marking post Taken).
	db.Exec("INSERT INTO messages_outcomes_intended (msgid, outcome) VALUES (?, 'Taken') ON DUPLICATE KEY UPDATE outcome = VALUES(outcome)", msgID)

	// Confirm message currently has an Expired outcome.
	var outcomeCount int64
	db.Raw("SELECT COUNT(*) FROM messages_outcomes WHERE msgid = ? AND outcome = 'Expired'", msgID).Scan(&outcomeCount)
	assert.Equal(t, int64(1), outcomeCount, "message should have Expired outcome before patch")
	var intendedCount int64
	db.Raw("SELECT COUNT(*) FROM messages_outcomes_intended WHERE msgid = ?", msgID).Scan(&intendedCount)
	assert.Equal(t, int64(1), intendedCount, "intended outcome should exist before patch")

	// PATCH with a future deadline — should clear only the Expired outcome.
	futureDeadline := "2027-01-01"
	body, _ := json.Marshal(map[string]interface{}{
		"id":       msgID,
		"deadline": futureDeadline,
	})
	req := httptest.NewRequest("PATCH", "/api/message?jwt="+ownerToken, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Expired outcome should be cleared so the post becomes active again.
	db.Raw("SELECT COUNT(*) FROM messages_outcomes WHERE msgid = ? AND outcome = 'Expired'", msgID).Scan(&outcomeCount)
	assert.Equal(t, int64(0), outcomeCount, "Expired outcome should be cleared after patching with future deadline")
	// In-progress intended outcome must NOT be cleared — it is unrelated to deadline extension.
	db.Raw("SELECT COUNT(*) FROM messages_outcomes_intended WHERE msgid = ?", msgID).Scan(&intendedCount)
	assert.Equal(t, int64(1), intendedCount, "intended outcome should be preserved after deadline extension")
}

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

func TestPutMessageRecordsFromIP(t *testing.T) {
	prefix := uniquePrefix("msgput_ip")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	body := map[string]interface{}{
		"groupid":  groupID,
		"type":     "Offer",
		"subject":  prefix + " IP Test",
		"textbody": "Testing fromip",
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
	newID := uint64(result["id"].(float64))

	// Verify fromip was recorded.
	var fromip *string
	db.Raw("SELECT fromip FROM messages WHERE id = ?", newID).Scan(&fromip)
	assert.NotNil(t, fromip, "fromip should be recorded")
}

// TestPutMessageAvailableNowSetsInitially verifies: sending only
// availablenow sets both availableinitially and availablenow to that value.
func TestPutMessageAvailableNowSetsInitially(t *testing.T) {
	prefix := uniquePrefix("msgput_avail")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	body := map[string]interface{}{
		"groupid":      groupID,
		"type":         "Offer",
		"item":         "Chairs",
		"textbody":     "Some chairs",
		"availablenow": 6,
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/message?jwt="+token, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	require.Equal(t, float64(0), result["ret"])
	newID := uint64(result["id"].(float64))

	var availInit, availNow int
	db.Raw("SELECT availableinitially, availablenow FROM messages WHERE id = ?", newID).Row().Scan(&availInit, &availNow)
	assert.Equal(t, 6, availInit, "availableinitially should mirror availablenow when not explicitly set")
	assert.Equal(t, 6, availNow)
}

func TestPutMessageSetsLatLngFromLocation(t *testing.T) {
	prefix := uniquePrefix("msgput_loc")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	// Find a location with non-zero lat/lng.
	var locID uint64
	var locLat, locLng float64
	db.Raw("SELECT id, lat, lng FROM locations WHERE lat != 0 AND lng != 0 LIMIT 1").Row().Scan(&locID, &locLat, &locLng)
	if locID == 0 {
		t.Fatal("No locations with non-zero lat/lng in test database")
	}

	body := map[string]interface{}{
		"groupid":    groupID,
		"type":       "Offer",
		"subject":    prefix + " Located Offer",
		"textbody":   "A test offer with location",
		"item":       "Located Item",
		"locationid": locID,
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/message?jwt="+token, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	newID := uint64(result["id"].(float64))

	// Verify lat/lng were set from the location.
	var msgLat, msgLng float64
	db.Raw("SELECT lat, lng FROM messages WHERE id = ?", newID).Row().Scan(&msgLat, &msgLng)
	assert.InDelta(t, locLat, msgLat, 0.001, "message lat should match location lat")
	assert.InDelta(t, locLng, msgLng, 0.001, "message lng should match location lng")

	// Verify locationid was set.
	var msgLocID uint64
	db.Raw("SELECT COALESCE(locationid, 0) FROM messages WHERE id = ?", newID).Scan(&msgLocID)
	assert.Equal(t, locID, msgLocID, "message locationid should be set")

	// Draft should NOT be in messages_spatial.
	var spatialCount int64
	db.Raw("SELECT COUNT(*) FROM messages_spatial WHERE msgid = ?", newID).Scan(&spatialCount)
	assert.Equal(t, int64(0), spatialCount, "draft should not be in messages_spatial")

	// Now submit via JoinAndPost — spatial index should be populated.
	postBody, _ := json.Marshal(map[string]interface{}{
		"id":     newID,
		"email":  fmt.Sprintf("%s@test.com", prefix+"_user"),
		"action": "JoinAndPost",
	})
	postReq := httptest.NewRequest("POST", "/api/message?jwt="+token, bytes.NewBuffer(postBody))
	postReq.Header.Set("Content-Type", "application/json")
	postResp, postErr := getApp().Test(postReq)
	assert.NoError(t, postErr)
	assert.Equal(t, 200, postResp.StatusCode)

	// Now messages_spatial should have the entry.
	db.Raw("SELECT COUNT(*) FROM messages_spatial WHERE msgid = ?", newID).Scan(&spatialCount)
	assert.Equal(t, int64(1), spatialCount, "submitted message should be in messages_spatial")

	// Verify spatial coords match.
	var spatialLat, spatialLng float64
	db.Raw("SELECT ST_Y(point), ST_X(point) FROM messages_spatial WHERE msgid = ?", newID).Row().Scan(&spatialLat, &spatialLng)
	assert.InDelta(t, locLat, spatialLat, 0.001, "spatial lat should match location")
	assert.InDelta(t, locLng, spatialLng, 0.001, "spatial lng should match location")
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

	// Verify heldby is set to the mod before moving to Pending).
	var heldby uint64
	db.Raw("SELECT COALESCE(heldby, 0) FROM messages WHERE id = ?", msgID).Scan(&heldby)
	assert.Equal(t, modID, heldby, "BackToPending should set heldby to the mod")

	// Verify a log entry was created.
	var logCount int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE type = ? AND subtype = ? AND msgid = ? AND byuser = ?",
		log.LOG_TYPE_MESSAGE, log.LOG_SUBTYPE_HOLD, msgID, modID).Scan(&logCount)
	assert.Equal(t, int64(1), logCount, "BackToPending should create a Hold log entry")

	// Verify push_notify_group_mods background task was queued.
	var pushTaskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = ? AND processed_at IS NULL AND data LIKE ?",
		queue.TaskPushNotifyGroupMods, fmt.Sprintf("%%group_id%%%d%%", groupID)).Scan(&pushTaskCount)
	assert.GreaterOrEqual(t, pushTaskCount, int64(1), "BackToPending should queue push_notify_group_mods task")
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

// TestPostMessageOutcomeIntendedRepost verifies that "Repost" is a valid
// intended outcome. The frontend sends this when a user clicks the repost
// link from the notification email.
func TestPostMessageOutcomeIntendedRepost(t *testing.T) {
	prefix := uniquePrefix("msgw_int_rep")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, userID, groupID, prefix+" offer item", 52.5, -1.8)

	body := map[string]interface{}{
		"id":      msgID,
		"action":  "OutcomeIntended",
		"outcome": "Repost",
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
	assert.Equal(t, "Repost", outcome)
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

func TestPostMessageAddBySomeoneElse(t *testing.T) {
	// AddBy with no userid means "someone else" (not a known Freegle user).
	prefix := uniquePrefix("msgw_addby_else")
	db := database.DBConn

	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	_, ownerToken := CreateTestSession(t, ownerID)
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, ownerID, groupID, prefix+" offer item", 52.5, -1.8)

	db.Exec("UPDATE messages SET availableinitially = 3, availablenow = 3 WHERE id = ?", msgID)

	// AddBy with no userid — represents "someone else".
	body := map[string]interface{}{
		"id":     msgID,
		"action": "AddBy",
		"count":  1,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/message?jwt=%s", ownerToken)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify messages_by entry with userid=NULL.
	var byCount int
	db.Raw("SELECT count FROM messages_by WHERE msgid = ? AND userid IS NULL", msgID).Scan(&byCount)
	assert.Equal(t, 1, byCount)

	// Verify available count reduced.
	var availNow int
	db.Raw("SELECT availablenow FROM messages WHERE id = ?", msgID).Scan(&availNow)
	assert.Equal(t, 2, availNow)
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

// =============================================================================
// Message List Tests (GET /messages)
// =============================================================================

func TestListMessagesApproved(t *testing.T) {
	prefix := uniquePrefix("lstmsg_apr")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	CreateTestMessage(t, userID, groupID, prefix+" Offer Sofa", 55.9533, -3.1883)

	// List approved messages for the group - public access, no auth required.
	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/messages?groupid=%d&collection=Approved", groupID), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result message.ListMessagesResponse
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Greater(t, len(result.Messages), 0)

	// Verify the message has expected fields.
	found := false
	for _, m := range result.Messages {
		if m.Fromuser == userID {
			found = true
			assert.Greater(t, m.ID, uint64(0))
			assert.NotEmpty(t, m.Subject)
			assert.NotEmpty(t, m.Type)
			assert.Greater(t, len(m.Groups), 0)
			break
		}
	}
	assert.True(t, found, "Should find the created message in the list")
}

func TestListMessagesPending(t *testing.T) {
	prefix := uniquePrefix("lstmsg_pend")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Create a pending message.
	var locationID uint64
	db.Raw("SELECT id FROM locations LIMIT 1").Scan(&locationID)
	db.Exec("INSERT INTO messages (fromuser, subject, textbody, type, locationid, arrival, date) VALUES (?, ?, 'Test body', 'Offer', ?, NOW(), NOW())",
		posterID, prefix+" pending item", locationID)

	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? AND subject = ? ORDER BY id DESC LIMIT 1",
		posterID, prefix+" pending item").Scan(&msgID)
	assert.Greater(t, msgID, uint64(0))

	db.Exec("INSERT INTO messages_groups (msgid, groupid, arrival, collection, autoreposts) VALUES (?, ?, NOW(), 'Pending', 0)",
		msgID, groupID)

	// Mod should be able to see pending messages.
	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/messages?groupid=%d&collection=Pending&jwt=%s", groupID, modToken), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result message.ListMessagesResponse
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Greater(t, len(result.Messages), 0)

	// Verify we find the pending message.
	found := false
	for _, m := range result.Messages {
		if m.ID == msgID {
			found = true
			break
		}
	}
	assert.True(t, found, "Mod should see the pending message")
}

func TestListMessagesMT_DeletedMessageNotReturned(t *testing.T) {
	// Regression test: messages with messages.deleted set should not appear in
	// /api/modtools/messages even when messages_groups.collection = 'Pending'.
	// V1 filters with messages.deleted IS NULL; the Go API was missing this check.
	prefix := uniquePrefix("lstmt_del")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Create a message that is marked deleted but still has a Pending entry in messages_groups.
	db.Exec("INSERT INTO messages (fromuser, subject, textbody, type, arrival, date, deleted) VALUES (?, ?, 'Test body', 'Offer', NOW(), NOW(), NOW())",
		posterID, prefix+" deleted pending item")
	var deletedMsgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? AND subject = ? ORDER BY id DESC LIMIT 1",
		posterID, prefix+" deleted pending item").Scan(&deletedMsgID)
	assert.Greater(t, deletedMsgID, uint64(0))
	db.Exec("INSERT INTO messages_groups (msgid, groupid, arrival, collection, autoreposts) VALUES (?, ?, NOW(), 'Pending', 0)",
		deletedMsgID, groupID)

	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/modtools/messages?groupid=%d&collection=Pending&jwt=%s", groupID, modToken), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	msgs, _ := body["messages"].([]interface{})
	for _, id := range msgs {
		assert.NotEqual(t, float64(deletedMsgID), id, "Deleted message should not appear in modtools pending list")
	}

	// Clean up.
	db.Exec("DELETE FROM messages_groups WHERE msgid = ?", deletedMsgID)
	db.Exec("DELETE FROM messages WHERE id = ?", deletedMsgID)
}

func TestListMessagesMT_LimboUserMessageNotReturned(t *testing.T) {
	// Regression test: messages from limbo (soft-deleted) users should not appear in
	// /api/modtools/messages. The user's messages.deleted may be NULL (limbo only sets
	// users.deleted), but the list query should filter on users.deleted IS NULL.
	prefix := uniquePrefix("lstmt_limbo")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Create a pending message from the poster.
	msgID := CreateTestMessage(t, posterID, groupID, prefix+" limbo test item", 52.0, -1.0)
	db.Exec("UPDATE messages_groups SET collection = 'Pending' WHERE msgid = ?", msgID)

	// Verify it appears in listing before limbo.
	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/modtools/messages?groupid=%d&collection=Pending&jwt=%s", groupID, modToken), nil))
	assert.NoError(t, err)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	msgs, _ := body["messages"].([]interface{})
	found := false
	for _, id := range msgs {
		if id == float64(msgID) {
			found = true
		}
	}
	assert.True(t, found, "Message should appear before user is limbo'd")

	// Limbo the poster (soft-delete).
	db.Exec("UPDATE users SET deleted = NOW() WHERE id = ?", posterID)

	// Verify it no longer appears in listing.
	resp2, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/modtools/messages?groupid=%d&collection=Pending&jwt=%s", groupID, modToken), nil))
	assert.NoError(t, err)
	var body2 map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&body2)
	msgs2, _ := body2["messages"].([]interface{})
	for _, id := range msgs2 {
		assert.NotEqual(t, float64(msgID), id, "Limbo user's message should not appear in modtools pending list")
	}

	// Clean up.
	db.Exec("DELETE FROM messages_groups WHERE msgid = ?", msgID)
	db.Exec("DELETE FROM messages WHERE id = ?", msgID)
	db.Exec("UPDATE users SET deleted = NULL WHERE id = ?", posterID)
}

func TestListMessagesPendingUnauthorized(t *testing.T) {
	prefix := uniquePrefix("lstmsg_pend_unauth")

	groupID := CreateTestGroup(t, prefix)
	regularID := CreateTestUser(t, prefix+"_regular", "User")
	CreateTestMembership(t, regularID, groupID, "Member")
	_, regularToken := CreateTestSession(t, regularID)

	// Regular member should NOT be able to see pending messages.
	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/messages?groupid=%d&collection=Pending&jwt=%s", groupID, regularToken), nil))
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestListMessagesPendingNotLoggedIn(t *testing.T) {
	prefix := uniquePrefix("lstmsg_pend_nolog")
	groupID := CreateTestGroup(t, prefix)

	// Not logged in should not see pending messages.
	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/messages?groupid=%d&collection=Pending", groupID), nil))
	assert.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestListMessagesWithContext(t *testing.T) {
	prefix := uniquePrefix("lstmsg_ctx")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")

	// Create 5 messages with different arrival times.
	for i := 0; i < 5; i++ {
		subject := fmt.Sprintf("%s Offer Item %d", prefix, i)
		CreateTestMessageWithArrival(t, userID, groupID, subject, 55.9533, -3.1883, 5-i)
	}

	// Also verify messages exist.
	var count int64
	db.Raw("SELECT COUNT(*) FROM messages_groups mg INNER JOIN messages m ON m.id = mg.msgid WHERE mg.groupid = ? AND mg.collection = 'Approved' AND mg.deleted = 0 AND m.fromuser IS NOT NULL", groupID).Scan(&count)
	assert.GreaterOrEqual(t, count, int64(5))

	// First page - get 3 messages.
	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/messages?groupid=%d&collection=Approved&limit=3", groupID), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var page1 message.ListMessagesResponse
	json.NewDecoder(resp.Body).Decode(&page1)
	assert.Equal(t, 3, len(page1.Messages))
	assert.NotNil(t, page1.Context, "Should have pagination context when more messages exist")

	// Second page using the context.
	ctxJSON, _ := json.Marshal(page1.Context)
	resp, err = getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/messages?groupid=%d&collection=Approved&limit=3&context=%s", groupID, url.QueryEscape(string(ctxJSON))), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var page2 message.ListMessagesResponse
	json.NewDecoder(resp.Body).Decode(&page2)
	assert.Greater(t, len(page2.Messages), 0, "Second page should have messages")

	// Verify no overlap between pages.
	page1IDs := map[uint64]bool{}
	for _, m := range page1.Messages {
		page1IDs[m.ID] = true
	}
	for _, m := range page2.Messages {
		assert.False(t, page1IDs[m.ID], "Pages should not overlap: message %d found in both pages", m.ID)
	}
}

func TestListMessagesSearch(t *testing.T) {
	prefix := uniquePrefix("lstmsg_srch")

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")

	// Create messages with specific subjects for search.
	CreateTestMessage(t, userID, groupID, prefix+" Offer Vintage Armchair", 55.9533, -3.1883)
	CreateTestMessage(t, userID, groupID, prefix+" Offer Kitchen Table", 55.9533, -3.1883)

	// Search by subject (searchall).
	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/messages?groupid=%d&collection=Approved&subaction=searchall&search=Armchair", groupID), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result message.ListMessagesResponse
	json.NewDecoder(resp.Body).Decode(&result)

	// We should find the armchair message but not the table.
	foundArmchair := false
	foundTable := false
	for _, m := range result.Messages {
		if m.ID > 0 && m.Fromuser == userID {
			if containsSubstring(m.Subject, "Armchair") {
				foundArmchair = true
			}
			if containsSubstring(m.Subject, "Table") {
				foundTable = true
			}
		}
	}
	assert.True(t, foundArmchair, "Should find armchair message")
	assert.False(t, foundTable, "Should NOT find table message when searching for armchair")
}

func TestListMessagesSearchByID(t *testing.T) {
	prefix := uniquePrefix("lstmsg_srchid")

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")

	msgID := CreateTestMessage(t, userID, groupID, prefix+" Offer Search By ID Test", 55.9533, -3.1883)

	// Search by numeric message ID.
	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/messages?groupid=%d&collection=Approved&subaction=searchall&search=%d", groupID, msgID), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result message.ListMessagesResponse
	json.NewDecoder(resp.Body).Decode(&result)

	found := false
	for _, m := range result.Messages {
		if m.ID == msgID {
			found = true
		}
	}
	assert.True(t, found, "Should find message by its numeric ID")
}

func TestListMessagesSearchMemb(t *testing.T) {
	prefix := uniquePrefix("lstmsg_srchmb")

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_searchuser", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	CreateTestMessage(t, userID, groupID, prefix+" Offer Bicycle", 55.9533, -3.1883)

	// Search by member name (searchmemb) - requires mod access for non-approved.
	// But also works on Approved collection.
	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/messages?groupid=%d&collection=Approved&subaction=searchmemb&search=%s&jwt=%s",
			groupID, url.QueryEscape(prefix+"_searchuser"), modToken), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result message.ListMessagesResponse
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Greater(t, len(result.Messages), 0, "Should find messages by member name")
}

func TestListMessagesSearchMembByID(t *testing.T) {
	prefix := uniquePrefix("lstmsg_srchmid")

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	CreateTestMessage(t, userID, groupID, prefix+" Offer SearchByMemberID", 55.9533, -3.1883)

	// Search by numeric user ID (searchmemb with a number).
	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/messages?groupid=%d&collection=Approved&subaction=searchmemb&search=%d&jwt=%s",
			groupID, userID, modToken), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result message.ListMessagesResponse
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Greater(t, len(result.Messages), 0, "Should find messages by numeric member ID")
}

func TestListMessagesInvalidCollection(t *testing.T) {
	prefix := uniquePrefix("lstmsg_badcoll")
	groupID := CreateTestGroup(t, prefix)

	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/messages?groupid=%d&collection=Invalid", groupID), nil))
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestListMessagesNoGroupID(t *testing.T) {
	resp, err := getApp().Test(httptest.NewRequest("GET",
		"/api/messages?collection=Approved", nil))
	assert.NoError(t, err)
	// No groupid returns empty list (graceful degradation).
	assert.Equal(t, 200, resp.StatusCode)
}

func TestListMessagesWithLimit(t *testing.T) {
	prefix := uniquePrefix("lstmsg_lim")

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")

	// Create 3 messages.
	CreateTestMessage(t, userID, groupID, prefix+" Item 1", 55.9533, -3.1883)
	CreateTestMessage(t, userID, groupID, prefix+" Item 2", 55.9533, -3.1883)
	CreateTestMessage(t, userID, groupID, prefix+" Item 3", 55.9533, -3.1883)

	// Request with limit of 2.
	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/messages?groupid=%d&collection=Approved&limit=2", groupID), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result message.ListMessagesResponse
	json.NewDecoder(resp.Body).Decode(&result)
	assert.LessOrEqual(t, len(result.Messages), 2)
}

func TestListMessagesV2Path(t *testing.T) {
	prefix := uniquePrefix("lstmsg_v2")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	CreateTestMessage(t, userID, groupID, prefix+" V2 Item", 55.9533, -3.1883)

	// Verify the v2 path works.
	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/apiv2/messages?groupid=%d&collection=Approved", groupID), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestListMessagesAdminCanSeePending(t *testing.T) {
	prefix := uniquePrefix("lstmsg_admin")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	CreateTestMembership(t, posterID, groupID, "Member")
	// Admin is NOT a member of the group.
	_, adminToken := CreateTestSession(t, adminID)

	// Create pending message.
	var locationID uint64
	db.Raw("SELECT id FROM locations LIMIT 1").Scan(&locationID)
	db.Exec("INSERT INTO messages (fromuser, subject, textbody, type, locationid, arrival, date) VALUES (?, ?, 'Test body', 'Offer', ?, NOW(), NOW())",
		posterID, prefix+" admin pending", locationID)
	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? AND subject = ? ORDER BY id DESC LIMIT 1",
		posterID, prefix+" admin pending").Scan(&msgID)
	db.Exec("INSERT INTO messages_groups (msgid, groupid, arrival, collection, autoreposts) VALUES (?, ?, NOW(), 'Pending', 0)",
		msgID, groupID)

	// Admin should see pending.
	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/messages?groupid=%d&collection=Pending&jwt=%s", groupID, adminToken), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result message.ListMessagesResponse
	json.NewDecoder(resp.Body).Decode(&result)
	found := false
	for _, m := range result.Messages {
		if m.ID == msgID {
			found = true
			break
		}
	}
	assert.True(t, found, "Admin should see pending message")
}

func TestGetMessageWithoutHistory(t *testing.T) {
	// Verify that regular GET /message/:id still works without messagehistory param.
	prefix := uniquePrefix("msgnohist")

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")

	msgID := CreateTestMessage(t, posterID, groupID, prefix+" Normal Message", 55.9533, -3.1883)

	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/message/%d", msgID), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result message.Message
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, msgID, result.ID)
}

func TestGetMultipleMessagesStillWorks(t *testing.T) {
	// Verify that GET /message/id1,id2 still works with the new handler.
	prefix := uniquePrefix("msgmulti")

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")

	mid1 := CreateTestMessage(t, posterID, groupID, prefix+" Multi 1", 55.9533, -3.1883)
	mid2 := CreateTestMessage(t, posterID, groupID, prefix+" Multi 2", 55.9533, -3.1883)

	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/message/%d,%d", mid1, mid2), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var messages []message.Message
	json.NewDecoder(resp.Body).Decode(&messages)
	assert.Equal(t, 2, len(messages))
}

// =============================================================================
// Helper functions
// =============================================================================

func containsSubstring(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func TestMessagesMarkSeen(t *testing.T) {
	prefix := uniquePrefix("markseen")
	groupID := CreateTestGroup(t, prefix)

	// Create message owner and viewer
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	CreateTestMembership(t, ownerID, groupID, "Member")

	viewerID := CreateTestUser(t, prefix+"_viewer", "User")
	CreateTestMembership(t, viewerID, groupID, "Member")
	_, viewerToken := CreateTestSession(t, viewerID)

	// Create messages
	msgID1 := CreateTestMessage(t, ownerID, groupID, "Test Item 1", 55.9533, -3.1883)
	msgID2 := CreateTestMessage(t, ownerID, groupID, "Test Item 2", 55.9533, -3.1883)

	// Mark both messages as seen via POST
	body := fmt.Sprintf(`{"ids": [%d, %d]}`, msgID1, msgID2)
	req := httptest.NewRequest("POST", "/api/messages/markseen?jwt="+viewerToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, true, result["success"])

	// Verify messages are now marked as seen by checking the user message endpoint
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(ownerID)+"/message?jwt="+viewerToken, nil))
	assert.Equal(t, 200, resp.StatusCode)

	type MessageWithUnseen struct {
		ID     uint64 `json:"id"`
		Unseen bool   `json:"unseen"`
	}

	var msgs []MessageWithUnseen
	json2.Unmarshal(rsp(resp), &msgs)

	for _, m := range msgs {
		if m.ID == msgID1 || m.ID == msgID2 {
			assert.False(t, m.Unseen, "Message %d should be seen after MarkSeen", m.ID)
		}
	}
}

func TestMessagesMarkSeenUnauthorized(t *testing.T) {
	// Test without token - should fail
	body := `{"ids": [1]}`
	req := httptest.NewRequest("POST", "/api/messages/markseen", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestMessagesMarkSeenEmptyIds(t *testing.T) {
	prefix := uniquePrefix("markseen_empty")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Test with empty IDs array
	body := `{"ids": []}`
	req := httptest.NewRequest("POST", "/api/messages/markseen?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestMessagesMarkSeenInvalidBody(t *testing.T) {
	prefix := uniquePrefix("markseen_invalid")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Test with missing IDs field
	body := `{}`
	req := httptest.NewRequest("POST", "/api/messages/markseen?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestMessagesMarkSeenInvalidJSON(t *testing.T) {
	prefix := uniquePrefix("markseen_json")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	req := httptest.NewRequest("POST", "/api/messages/markseen?jwt="+token, bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestMessagesMarkSeenNonExistentIDs(t *testing.T) {
	// Marking non-existent message IDs should succeed (inserts orphaned View records
	// but doesn't crash). This matches PHP behaviour.
	prefix := uniquePrefix("markseen_ghost")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{"ids": [999999998, 999999999]}`
	req := httptest.NewRequest("POST", "/api/messages/markseen?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestMessagesMarkSeenIdempotent(t *testing.T) {
	// Marking the same message as seen twice should succeed (ON DUPLICATE KEY UPDATE)
	prefix := uniquePrefix("markseen_idem")
	groupID := CreateTestGroup(t, prefix)

	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	CreateTestMembership(t, ownerID, groupID, "Member")

	viewerID := CreateTestUser(t, prefix+"_viewer", "User")
	CreateTestMembership(t, viewerID, groupID, "Member")
	_, viewerToken := CreateTestSession(t, viewerID)

	msgID := CreateTestMessage(t, ownerID, groupID, "Test Idempotent", 55.9533, -3.1883)

	body := fmt.Sprintf(`{"ids": [%d]}`, msgID)

	// First mark
	req := httptest.NewRequest("POST", "/api/messages/markseen?jwt="+viewerToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Second mark (should also succeed)
	req = httptest.NewRequest("POST", "/api/messages/markseen?jwt="+viewerToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}

// --- Tests: Subject reconstruction from item + location ---

func TestPatchMessageReconstructsSubjectFromItemLocation(t *testing.T) {
	// Bug #209: When a mod edits item/location in ModTools, the Go PATCH handler
	// should reconstruct the subject using area + vague postcode.
	prefix := uniquePrefix("msgmod_subj_recon")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, posterID, groupID, "Member")
	_, modToken := CreateTestSession(t, modID)

	// Create a message.
	msgID := CreateTestMessage(t, posterID, groupID, "OFFER: Old item (Old Location)", 52.5, -1.8)

	// Create an area location.
	db.Exec("INSERT INTO locations (name, type, lat, lng) VALUES (?, 'Other', 52.5, -1.8)", prefix+"_Village")
	var areaID uint64
	db.Raw("SELECT id FROM locations WHERE name = ? ORDER BY id DESC LIMIT 1", prefix+"_Village").Scan(&areaID)
	require.NotZero(t, areaID)

	// Create a postcode location with areaid pointing to the area.
	db.Exec("INSERT INTO locations (name, type, lat, lng, areaid) VALUES (?, 'Postcode', 52.5, -1.8, ?)", prefix+"_CB22 3AA", areaID)
	var pcID uint64
	db.Raw("SELECT id FROM locations WHERE name = ? ORDER BY id DESC LIMIT 1", prefix+"_CB22 3AA").Scan(&pcID)
	require.NotZero(t, pcID)

	// Assign the postcode location to the message.
	db.Exec("UPDATE messages SET locationid = ? WHERE id = ?", pcID, msgID)

	// Create an item for the message.
	db.Exec("INSERT INTO items (name) VALUES (?)", prefix+"_Kitchen table")
	var itemID uint64
	db.Raw("SELECT id FROM items WHERE name = ? ORDER BY id DESC LIMIT 1", prefix+"_Kitchen table").Scan(&itemID)
	require.NotZero(t, itemID)
	db.Exec("DELETE FROM messages_items WHERE msgid = ?", msgID)
	db.Exec("INSERT INTO messages_items (msgid, itemid) VALUES (?, ?)", msgID, itemID)

	// PATCH with a new item and location name.
	body, _ := json.Marshal(map[string]interface{}{
		"id":       msgID,
		"item":     prefix + "_Dining table",
		"location": prefix + "_CB22 3AA",
	})
	req := httptest.NewRequest("PATCH", "/api/message?jwt="+modToken, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify subject was reconstructed with area + vague postcode.
	var subject string
	db.Raw("SELECT subject FROM messages WHERE id = ?", msgID).Scan(&subject)

	// Should be "OFFER: <item> (<area> <vague_pc>)" — area name + outward code only.
	assert.Contains(t, subject, prefix+"_Dining table")
	assert.Contains(t, subject, prefix+"_Village")
	assert.Contains(t, subject, prefix+"_CB22")
	// Should NOT contain the full postcode (inward code).
	assert.NotContains(t, subject, "3AA")
}

func TestPatchMessageItemCaseCorrection(t *testing.T) {
	// Bug: Discourse #9518 post #18 — "Correct Case" standard message lowercases the
	// subject visually but the Go API finds the existing item via case-insensitive MySQL
	// lookup and reconstructs the subject using the original capitalized DB name, so the
	// lowercase edit never saves.
	prefix := uniquePrefix("msgmod_case")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, posterID, groupID, "Member")
	_, modToken := CreateTestSession(t, modID)

	// Create an area and postcode location.
	db.Exec("INSERT INTO locations (name, type, lat, lng) VALUES (?, 'Other', 52.5, -1.8)", prefix+"_Town")
	var areaID uint64
	db.Raw("SELECT id FROM locations WHERE name = ? ORDER BY id DESC LIMIT 1", prefix+"_Town").Scan(&areaID)
	require.NotZero(t, areaID)
	db.Exec("INSERT INTO locations (name, type, lat, lng, areaid) VALUES (?, 'Postcode', 52.5, -1.8, ?)", prefix+"_CB22 3AA", areaID)
	var pcID uint64
	db.Raw("SELECT id FROM locations WHERE name = ? ORDER BY id DESC LIMIT 1", prefix+"_CB22 3AA").Scan(&pcID)
	require.NotZero(t, pcID)

	// Create a message with the item in UPPERCASE.
	msgID := CreateTestMessage(t, posterID, groupID, "OFFER: KITCHEN TABLE (Location)", 52.5, -1.8)
	db.Exec("UPDATE messages SET locationid = ? WHERE id = ?", pcID, msgID)

	// Create item with UPPERCASE name.
	db.Exec("INSERT INTO items (name) VALUES (?)", prefix+"_KITCHEN TABLE")
	var itemID uint64
	db.Raw("SELECT id FROM items WHERE name = ? ORDER BY id DESC LIMIT 1", prefix+"_KITCHEN TABLE").Scan(&itemID)
	require.NotZero(t, itemID)
	db.Exec("DELETE FROM messages_items WHERE msgid = ?", msgID)
	db.Exec("INSERT INTO messages_items (msgid, itemid) VALUES (?, ?)", msgID, itemID)

	// PATCH with lowercase item name (simulating "Correct Case" action).
	body, _ := json.Marshal(map[string]interface{}{
		"id":   msgID,
		"item": prefix + "_kitchen table",
	})
	req := httptest.NewRequest("PATCH", "/api/message?jwt="+modToken, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// The subject should use the lowercase item name the moderator submitted.
	var subject string
	db.Raw("SELECT subject FROM messages WHERE id = ?", msgID).Scan(&subject)
	assert.Contains(t, subject, prefix+"_kitchen table", "subject should contain the submitted lowercase item name")
	assert.NotContains(t, subject, prefix+"_KITCHEN TABLE", "subject should NOT contain the old uppercase name")

	// The items table canonical name must NOT be changed — it is shared across all
	// messages using this item. Modifying it from a single message edit would cause
	// flip-flopping if different mods use different casings.
	var storedName string
	db.Raw("SELECT name FROM items WHERE id = ?", itemID).Scan(&storedName)
	assert.Equal(t, prefix+"_KITCHEN TABLE", storedName, "items canonical name should NOT be mutated by a message edit")
}

func TestPatchMessageTypeChangeCreatesEditRecord(t *testing.T) {
	// When a message owner changes the type (e.g. Offer→Wanted), the edit record should
	// capture oldtype/newtype and the reconstructed subject change.
	prefix := uniquePrefix("msgmod_typeedit")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	CreateTestMembership(t, ownerID, groupID, "Member")
	_, ownerToken := CreateTestSession(t, ownerID)

	// Create a message via createPendingMessage (handles required DB columns).
	msgID := createPendingMessage(t, ownerID, groupID, prefix)

	// Create area + postcode locations and item for subject reconstruction.
	db.Exec("INSERT INTO locations (name, type, lat, lng) VALUES (?, 'Other', 52.5, -1.8)", prefix+"_Area")
	var areaID uint64
	db.Raw("SELECT id FROM locations WHERE name = ? ORDER BY id DESC LIMIT 1", prefix+"_Area").Scan(&areaID)
	db.Exec("INSERT INTO locations (name, type, lat, lng, areaid) VALUES (?, 'Postcode', 52.5, -1.8, ?)", prefix+"_B25 8FF", areaID)
	var pcID uint64
	db.Raw("SELECT id FROM locations WHERE name = ? ORDER BY id DESC LIMIT 1", prefix+"_B25 8FF").Scan(&pcID)
	db.Exec("UPDATE messages SET locationid = ? WHERE id = ?", pcID, msgID)
	db.Exec("INSERT INTO items (name) VALUES (?)", prefix+"_Widget")
	var itemID uint64
	db.Raw("SELECT id FROM items WHERE name = ? ORDER BY id DESC LIMIT 1", prefix+"_Widget").Scan(&itemID)
	db.Exec("DELETE FROM messages_items WHERE msgid = ?", msgID)
	db.Exec("INSERT INTO messages_items (msgid, itemid) VALUES (?, ?)", msgID, itemID)

	// Set the subject to match what reconstruction would produce for type=Offer.
	db.Exec("UPDATE messages SET subject = ?, type = 'Offer' WHERE id = ?",
		"OFFER: "+prefix+"_Widget ("+prefix+"_Area "+prefix+"_B25)", msgID)

	// Owner changes type to Wanted (no explicit subject in request).
	body, _ := json.Marshal(map[string]interface{}{
		"id":   msgID,
		"type": "Wanted",
	})
	req := httptest.NewRequest("PATCH", "/api/message?jwt="+ownerToken, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify edit record was created with correct oldtype/newtype.
	var editID uint64
	var oldType, newType, oldSubject, newSubject *string
	db.Raw("SELECT id, oldtype, newtype, oldsubject, newsubject FROM messages_edits WHERE msgid = ? AND byuser = ? ORDER BY id DESC LIMIT 1",
		msgID, ownerID).Row().Scan(&editID, &oldType, &newType, &oldSubject, &newSubject)
	assert.NotZero(t, editID, "Edit record should be created for type change")
	assert.NotNil(t, oldType)
	assert.NotNil(t, newType)
	assert.Equal(t, "Offer", *oldType)
	assert.Equal(t, "Wanted", *newType)

	// Verify the reconstructed subject is captured.
	assert.NotNil(t, oldSubject)
	assert.NotNil(t, newSubject)
	assert.Contains(t, *oldSubject, "OFFER")
	assert.Contains(t, *newSubject, "WANTED")
}

func TestPatchMessageTypeChangeModNoEditRecord(t *testing.T) {
	// Mod type changes should NOT create an edit record.
	prefix := uniquePrefix("msgmod_typemod")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	msgID := createPendingMessage(t, posterID, groupID, prefix)

	body, _ := json.Marshal(map[string]interface{}{
		"id":   msgID,
		"type": "Wanted",
	})
	req := httptest.NewRequest("PATCH", "/api/message?jwt="+modToken, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var editCount int64
	db.Raw("SELECT COUNT(*) FROM messages_edits WHERE msgid = ? AND byuser = ?", msgID, modID).Scan(&editCount)
	assert.Equal(t, int64(0), editCount, "Mod type change should not create edit record")
}

// --- Tests: RejectToDraft / BackToDraft ---

func TestRejectToDraftOwner(t *testing.T) {
	prefix := uniquePrefix("msg_r2d_own")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	// Create an approved message.
	msgID := CreateTestMessage(t, userID, groupID, prefix+" item", 52.5, -1.8)

	// Verify it's in messages_groups.
	var mgCount int64
	db.Raw("SELECT COUNT(*) FROM messages_groups WHERE msgid = ?", msgID).Scan(&mgCount)
	require.Equal(t, int64(1), mgCount, "Message should be in messages_groups")

	// Call RejectToDraft as the message owner.
	body, _ := json.Marshal(map[string]interface{}{
		"id":     msgID,
		"action": "RejectToDraft",
	})
	req := httptest.NewRequest("POST", "/api/message?jwt="+token, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Offer", result["messagetype"])

	// Verify message is now in messages_drafts.
	var draftCount int64
	db.Raw("SELECT COUNT(*) FROM messages_drafts WHERE msgid = ?", msgID).Scan(&draftCount)
	assert.Equal(t, int64(1), draftCount, "Message should be in messages_drafts")

	// Verify message is no longer in messages_groups.
	db.Raw("SELECT COUNT(*) FROM messages_groups WHERE msgid = ?", msgID).Scan(&mgCount)
	assert.Equal(t, int64(0), mgCount, "Message should be removed from messages_groups")

	// Verify repost log entry was created.
	var logCount int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE msgid = ? AND type = 'Message' AND subtype = 'Repost'", msgID).Scan(&logCount)
	assert.Equal(t, int64(1), logCount, "Repost log entry should exist")
}

func TestRejectToDraftClearsExpiredDeadline(t *testing.T) {
	prefix := uniquePrefix("msg_r2d_dl")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	msgID := CreateTestMessage(t, userID, groupID, prefix+" item", 52.5, -1.8)

	// Set an expired deadline.
	db.Exec("UPDATE messages SET deadline = '2020-01-01' WHERE id = ?", msgID)

	// Call RejectToDraft.
	body, _ := json.Marshal(map[string]interface{}{
		"id":     msgID,
		"action": "RejectToDraft",
	})
	req := httptest.NewRequest("POST", "/api/message?jwt="+token, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify deadline was cleared.
	var deadline *string
	db.Raw("SELECT deadline FROM messages WHERE id = ?", msgID).Scan(&deadline)
	assert.Nil(t, deadline, "Expired deadline should be cleared")
}

func TestRejectToDraftKeepsFutureDeadline(t *testing.T) {
	prefix := uniquePrefix("msg_r2d_futuredl")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	msgID := CreateTestMessage(t, userID, groupID, prefix+" item", 52.5, -1.8)

	// Set a future deadline — should be preserved.
	futureDeadline := time.Now().AddDate(0, 0, 30).Format("2006-01-02")
	db.Exec("UPDATE messages SET deadline = ? WHERE id = ?", futureDeadline, msgID)

	body, _ := json.Marshal(map[string]interface{}{
		"id":     msgID,
		"action": "RejectToDraft",
	})
	req := httptest.NewRequest("POST", "/api/message?jwt="+token, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var deadline *string
	db.Raw("SELECT DATE_FORMAT(deadline, '%Y-%m-%d') FROM messages WHERE id = ?", msgID).Scan(&deadline)
	require.NotNil(t, deadline, "Future deadline should be preserved")
	assert.Equal(t, futureDeadline, *deadline, "Future deadline value should be unchanged")
}

func TestRejectToDraftForbiddenForOtherUser(t *testing.T) {
	prefix := uniquePrefix("msg_r2d_forbid")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	CreateTestMembership(t, ownerID, groupID, "Member")

	otherID := CreateTestUser(t, prefix+"_other", "User")
	CreateTestMembership(t, otherID, groupID, "Member")
	_, otherToken := CreateTestSession(t, otherID)

	msgID := CreateTestMessage(t, ownerID, groupID, prefix+" item", 52.5, -1.8)

	// Another user (not owner, not mod) should be forbidden.
	body, _ := json.Marshal(map[string]interface{}{
		"id":     msgID,
		"action": "RejectToDraft",
	})
	req := httptest.NewRequest("POST", "/api/message?jwt="+otherToken, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)

	// Verify message is still in messages_groups.
	var mgCount int64
	db.Raw("SELECT COUNT(*) FROM messages_groups WHERE msgid = ?", msgID).Scan(&mgCount)
	assert.Equal(t, int64(1), mgCount, "Message should still be in messages_groups")
}

func TestBackToDraftAlias(t *testing.T) {
	prefix := uniquePrefix("msg_b2d_alias")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	msgID := CreateTestMessage(t, userID, groupID, prefix+" item", 52.5, -1.8)

	// BackToDraft should work the same as RejectToDraft.
	body, _ := json.Marshal(map[string]interface{}{
		"id":     msgID,
		"action": "BackToDraft",
	})
	req := httptest.NewRequest("POST", "/api/message?jwt="+token, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify message is now a draft.
	var draftCount int64
	db.Raw("SELECT COUNT(*) FROM messages_drafts WHERE msgid = ?", msgID).Scan(&draftCount)
	assert.Equal(t, int64(1), draftCount, "Message should be in messages_drafts")
}

func TestRejectToDraftFullRepostFlow(t *testing.T) {
	// This tests the complete repost flow: RejectToDraft → PATCH → JoinAndPost.
	prefix := uniquePrefix("msg_r2d_flow")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	// Create an approved message with an old arrival.
	msgID := CreateTestMessage(t, userID, groupID, prefix+" item", 52.5, -1.8)
	db.Exec("UPDATE messages_groups SET arrival = DATE_SUB(NOW(), INTERVAL 30 DAY) WHERE msgid = ?", msgID)

	// Step 1: RejectToDraft.
	body1, _ := json.Marshal(map[string]interface{}{
		"id":     msgID,
		"action": "RejectToDraft",
	})
	req := httptest.NewRequest("POST", "/api/message?jwt="+token, bytes.NewBuffer(body1))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Step 2: PATCH to update the message (simulating client edit).
	patchBody, _ := json.Marshal(map[string]interface{}{
		"id":       msgID,
		"textbody": "Updated description for repost",
	})
	req = httptest.NewRequest("PATCH", "/api/message?jwt="+token, bytes.NewBuffer(patchBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Step 3: JoinAndPost to resubmit.
	body3, _ := json.Marshal(map[string]interface{}{
		"id":     msgID,
		"action": "JoinAndPost",
	})
	req = httptest.NewRequest("POST", "/api/message?jwt="+token, bytes.NewBuffer(body3))
	req.Header.Set("Content-Type", "application/json")
	resp, err = getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, float64(msgID), result["id"])

	// Verify message is back in messages_groups as Pending.
	var collection string
	db.Raw("SELECT collection FROM messages_groups WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&collection)
	assert.Equal(t, "Pending", collection)

	// Verify draft was cleaned up.
	var draftCount int64
	db.Raw("SELECT COUNT(*) FROM messages_drafts WHERE msgid = ?", msgID).Scan(&draftCount)
	assert.Equal(t, int64(0), draftCount, "Draft should be cleaned up after resubmit")

	// Verify text was updated.
	var textbody string
	db.Raw("SELECT textbody FROM messages WHERE id = ?", msgID).Scan(&textbody)
	assert.Equal(t, "Updated description for repost", textbody)
}

func TestRejectToDraftClearsOutcome(t *testing.T) {
	// When a message is moved back to draft for reposting, any existing outcome
	// (e.g. "Withdrawn") must be cleared so the reposted message starts fresh.
	prefix := uniquePrefix("r2d_outcome")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	msgID := CreateTestMessage(t, userID, groupID, prefix+" item", 52.5, -1.8)

	// Set a previous outcome.
	db.Exec("INSERT INTO messages_outcomes (msgid, outcome) VALUES (?, 'Withdrawn')", msgID)
	db.Exec("INSERT INTO messages_outcomes_intended (msgid, outcome) VALUES (?, 'Repost') ON DUPLICATE KEY UPDATE outcome = VALUES(outcome)", msgID)

	var outcomeCount int64
	db.Raw("SELECT COUNT(*) FROM messages_outcomes WHERE msgid = ?", msgID).Scan(&outcomeCount)
	require.Equal(t, int64(1), outcomeCount, "Outcome should exist before RejectToDraft")

	// Call RejectToDraft.
	body, _ := json.Marshal(map[string]interface{}{
		"id":     msgID,
		"action": "RejectToDraft",
	})
	req := httptest.NewRequest("POST", "/api/message?jwt="+token, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Outcome should be cleared.
	db.Raw("SELECT COUNT(*) FROM messages_outcomes WHERE msgid = ?", msgID).Scan(&outcomeCount)
	assert.Equal(t, int64(0), outcomeCount, "Outcome should be cleared after RejectToDraft")

	var intendedCount int64
	db.Raw("SELECT COUNT(*) FROM messages_outcomes_intended WHERE msgid = ?", msgID).Scan(&intendedCount)
	assert.Equal(t, int64(0), intendedCount, "Intended outcome should be cleared after RejectToDraft")
}

func TestRejectToDraftResetsAvailablenow(t *testing.T) {
	// When a message is moved back to draft, availablenow is reset to
	// availableinitially and messages_by is cleared.
	prefix := uniquePrefix("r2d_avail")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	CreateTestMembership(t, otherID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	msgID := CreateTestMessage(t, userID, groupID, prefix+" item", 52.5, -1.8)

	// Simulate the item being promised: set availablenow=0 and add a messages_by row.
	db.Exec("UPDATE messages SET availableinitially = 2, availablenow = 0 WHERE id = ?", msgID)
	db.Exec("INSERT INTO messages_by (msgid, userid, count) VALUES (?, ?, 2)", msgID, otherID)

	// Call RejectToDraft.
	body, _ := json.Marshal(map[string]interface{}{
		"id":     msgID,
		"action": "RejectToDraft",
	})
	req := httptest.NewRequest("POST", "/api/message?jwt="+token, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// availablenow should be reset to availableinitially.
	var availnow int
	db.Raw("SELECT availablenow FROM messages WHERE id = ?", msgID).Scan(&availnow)
	assert.Equal(t, 2, availnow, "availablenow should be reset to availableinitially")

	// messages_by should be cleared.
	var byCount int64
	db.Raw("SELECT COUNT(*) FROM messages_by WHERE msgid = ?", msgID).Scan(&byCount)
	assert.Equal(t, int64(0), byCount, "messages_by should be cleared after RejectToDraft")
}

func TestJoinAndPostClearsOutcomeAndRecordsPosting(t *testing.T) {
	// When a message is submitted via JoinAndPost, any stale outcome is cleared
	// and a messages_postings row is inserted (V1 parity).
	prefix := uniquePrefix("jap_outcome")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	msgID := CreateTestMessage(t, userID, groupID, prefix+" item", 52.5, -1.8)

	// Manually put the message into draft state (bypassing RejectToDraft).
	db.Exec("INSERT IGNORE INTO messages_drafts (msgid, groupid, userid) VALUES (?, ?, ?)", msgID, groupID, userID)
	db.Exec("DELETE FROM messages_groups WHERE msgid = ?", msgID)

	// Add a stale outcome that should be cleared on resubmit.
	db.Exec("INSERT INTO messages_outcomes (msgid, outcome) VALUES (?, 'Withdrawn')", msgID)

	// Call JoinAndPost.
	body, _ := json.Marshal(map[string]interface{}{
		"id":     msgID,
		"action": "JoinAndPost",
	})
	req := httptest.NewRequest("POST", "/api/message?jwt="+token, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Outcome should be cleared.
	var outcomeCount int64
	db.Raw("SELECT COUNT(*) FROM messages_outcomes WHERE msgid = ?", msgID).Scan(&outcomeCount)
	assert.Equal(t, int64(0), outcomeCount, "Outcome should be cleared by JoinAndPost")

	// messages_postings row should be inserted.
	var postingCount int64
	db.Raw("SELECT COUNT(*) FROM messages_postings WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&postingCount)
	assert.Equal(t, int64(1), postingCount, "messages_postings row should be inserted by JoinAndPost")
}

func TestGetMessageItemLocationForMod(t *testing.T) {
	// When a mod views a message posted by another user, the API should
	// return item and location data (needed for the structured edit UI).
	prefix := uniquePrefix("MsgItemLoc")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, userID, groupID, "Member")
	_, modToken := CreateTestSession(t, modID)

	// Create a message from the user with a location and item
	var locationID uint64
	db.Exec("INSERT INTO locations (name, type, lat, lng) VALUES (?, 'Postcode', 51.5, -0.1)",
		"PW_"+prefix+"_PC")
	db.Raw("SELECT id FROM locations WHERE name = ? ORDER BY id DESC LIMIT 1", "PW_"+prefix+"_PC").Scan(&locationID)
	assert.NotZero(t, locationID, "Location should be created")
	defer db.Exec("DELETE FROM locations WHERE id = ?", locationID)

	var msgID uint64
	db.Exec("INSERT INTO messages (fromuser, subject, textbody, type, source, locationid, sourceheader) VALUES (?, ?, ?, ?, 'Platform', ?, 'Platform')",
		userID, "OFFER: Test Item ("+prefix+")", "Test body", "Offer", locationID)
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", userID).Scan(&msgID)
	defer db.Exec("DELETE FROM messages WHERE id = ?", msgID)

	// Add to group
	db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, arrival) VALUES (?, ?, 'Approved', NOW())", msgID, groupID)
	defer db.Exec("DELETE FROM messages_groups WHERE msgid = ?", msgID)

	// Create an item for the message
	var itemID uint64
	db.Exec("INSERT INTO items (name) VALUES (?)", "Test Item")
	db.Raw("SELECT id FROM items WHERE name = 'Test Item' ORDER BY id DESC LIMIT 1").Scan(&itemID)
	db.Exec("INSERT INTO messages_items (msgid, itemid) VALUES (?, ?)", msgID, itemID)
	defer db.Exec("DELETE FROM messages_items WHERE msgid = ?", msgID)
	defer db.Exec("DELETE FROM items WHERE id = ?", itemID)

	// Fetch the message as the mod (not the owner)
	req := httptest.NewRequest("GET", fmt.Sprintf("/apiv2/message/%d?jwt=%s", msgID, modToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	// Item should be present for mod viewing another user's message
	assert.NotNil(t, result["item"], "Item should be returned for mod viewing message")
	if result["item"] != nil {
		itemData := result["item"].(map[string]interface{})
		assert.Equal(t, "Test Item", itemData["name"])
	}

	// Location should be present for mod (precise postcode visible to mods)
	assert.NotNil(t, result["location"], "Location should be returned for mod viewing message")
}

func TestGetMessageWorryWords(t *testing.T) {
	prefix := uniquePrefix("msg_worry")
	db := database.DBConn

	// Create group and users.
	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")

	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	otherID := CreateTestUser(t, prefix+"_other", "User")
	CreateTestMembership(t, otherID, groupID, "Member")
	_, otherToken := CreateTestSession(t, otherID)

	// Insert a global worry word (use a simple word without special chars,
	// matching real worry words like "cocaine", "heroin" etc.).
	worryKeyword := "dangertest" + fmt.Sprintf("%d", time.Now().UnixNano()%100000)
	db.Exec("INSERT INTO worrywords (keyword, type) VALUES (?, 'Regulated')", worryKeyword)
	defer db.Exec("DELETE FROM worrywords WHERE keyword = ?", worryKeyword)

	// Create a message whose subject contains the worry word.
	msgID := CreateTestMessage(t, posterID, groupID, "OFFER: "+worryKeyword+" near town", 52.5, -1.8)

	// 1. Fetch as mod — should see worry matches.
	resp, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/message/%d?jwt=%s", msgID, modToken), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msg map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&msg)

	worry, hasWorry := msg["worry"]
	assert.True(t, hasWorry, "Mod should see worry field")
	worryList := worry.([]interface{})
	assert.GreaterOrEqual(t, len(worryList), 1, "Should have at least 1 worry match")

	wm := worryList[0].(map[string]interface{})
	assert.Equal(t, worryKeyword, wm["word"])
	ww := wm["worryword"].(map[string]interface{})
	assert.Equal(t, worryKeyword, ww["keyword"])
	assert.Equal(t, "Regulated", ww["type"])

	// 2. Fetch as non-mod — should NOT see worry.
	resp2, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/message/%d?jwt=%s", msgID, otherToken), nil))
	assert.Equal(t, 200, resp2.StatusCode)

	var msg2 map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&msg2)

	_, hasWorry2 := msg2["worry"]
	assert.False(t, hasWorry2, "Non-mod should NOT see worry field")

	// 3. Test worry word in textbody (not subject).
	worryKeyword2 := "bodytest" + fmt.Sprintf("%d", time.Now().UnixNano()%100000)
	db.Exec("INSERT INTO worrywords (keyword, type) VALUES (?, 'Medicine')", worryKeyword2)
	defer db.Exec("DELETE FROM worrywords WHERE keyword = ?", worryKeyword2)

	// Create message with clean subject but worry word in body.
	msgID2 := CreateTestMessage(t, posterID, groupID, "OFFER: harmless item", 52.5, -1.8)
	db.Exec("UPDATE messages SET textbody = ? WHERE id = ?",
		"This is a test body containing "+worryKeyword2+" in the text.", msgID2)

	resp3, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/message/%d?jwt=%s", msgID2, modToken), nil))
	assert.Equal(t, 200, resp3.StatusCode)

	var msg3 map[string]interface{}
	json.NewDecoder(resp3.Body).Decode(&msg3)

	worry3, hasWorry3 := msg3["worry"]
	assert.True(t, hasWorry3, "Mod should see worry for body match")
	worryList3 := worry3.([]interface{})
	assert.GreaterOrEqual(t, len(worryList3), 1)
	wm3 := worryList3[0].(map[string]interface{})
	assert.Equal(t, worryKeyword2, wm3["word"])
	ww3 := wm3["worryword"].(map[string]interface{})
	assert.Equal(t, "Medicine", ww3["type"])

	// 4. Test group-specific worry words via group settings.
	groupWorry := "grouptest" + fmt.Sprintf("%d", time.Now().UnixNano()%100000)
	db.Exec("UPDATE `groups` SET settings = JSON_SET(COALESCE(settings, '{}'), '$.spammers', JSON_OBJECT('worrywords', ?)) WHERE id = ?",
		groupWorry, groupID)
	defer db.Exec("UPDATE `groups` SET settings = JSON_REMOVE(settings, '$.spammers') WHERE id = ?", groupID)

	msgID3 := CreateTestMessage(t, posterID, groupID, "OFFER: "+groupWorry+" here", 52.5, -1.8)

	resp4, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/message/%d?jwt=%s", msgID3, modToken), nil))
	assert.Equal(t, 200, resp4.StatusCode)

	var msg4 map[string]interface{}
	json.NewDecoder(resp4.Body).Decode(&msg4)

	worry4, hasWorry4 := msg4["worry"]
	assert.True(t, hasWorry4, "Mod should see group-specific worry match")
	worryList4 := worry4.([]interface{})
	assert.GreaterOrEqual(t, len(worryList4), 1)
	wm4 := worryList4[0].(map[string]interface{})
	assert.Equal(t, strings.ToLower(groupWorry), wm4["word"])
	ww4 := wm4["worryword"].(map[string]interface{})
	assert.Equal(t, "Review", ww4["type"])

	// 5. Test Allowed words are excluded.
	allowedWord := "allowtest" + fmt.Sprintf("%d", time.Now().UnixNano()%100000)
	db.Exec("INSERT INTO worrywords (keyword, type) VALUES (?, 'Allowed')", allowedWord)
	db.Exec("INSERT INTO worrywords (keyword, type) VALUES (?, 'Regulated')", allowedWord+"x")
	defer db.Exec("DELETE FROM worrywords WHERE keyword IN (?, ?)", allowedWord, allowedWord+"x")

	// Create message with just the allowed word — should NOT trigger worry.
	msgID4 := CreateTestMessage(t, posterID, groupID, "OFFER: "+allowedWord+" only", 52.5, -1.8)
	db.Exec("UPDATE messages SET textbody = ? WHERE id = ?", "Just "+allowedWord+" nothing else", msgID4)

	resp5, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/message/%d?jwt=%s", msgID4, modToken), nil))
	assert.Equal(t, 200, resp5.StatusCode)

	var msg5 map[string]interface{}
	json.NewDecoder(resp5.Body).Decode(&msg5)

	// The allowed word itself should not appear as a worry match.
	if worry5, hasWorry5 := msg5["worry"]; hasWorry5 {
		worryList5 := worry5.([]interface{})
		for _, w := range worryList5 {
			wm5 := w.(map[string]interface{})
			assert.NotEqual(t, allowedWord, wm5["word"], "Allowed word should not be a worry match")
		}
	}

	// 6. Test case-insensitive matching.
	msgID5 := CreateTestMessage(t, posterID, groupID, "OFFER: "+strings.ToUpper(worryKeyword)+" HERE", 52.5, -1.8)

	resp6, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/message/%d?jwt=%s", msgID5, modToken), nil))
	assert.Equal(t, 200, resp6.StatusCode)

	var msg6 map[string]interface{}
	json.NewDecoder(resp6.Body).Decode(&msg6)

	_, hasWorry6 := msg6["worry"]
	assert.True(t, hasWorry6, "Case-insensitive match should trigger worry")
}

func TestPatchMessageDeadline(t *testing.T) {
	prefix := uniquePrefix("msgmod_deadline")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	CreateTestMembership(t, ownerID, groupID, "Member")
	_, ownerToken := CreateTestSession(t, ownerID)

	msgID := createPendingMessage(t, ownerID, groupID, prefix)

	// Set a deadline.
	body := map[string]interface{}{
		"id":       msgID,
		"deadline": "2026-06-01",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PATCH", "/api/message?jwt="+ownerToken, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify deadline was saved.
	var deadline *string
	db.Raw("SELECT DATE_FORMAT(deadline, '%Y-%m-%d') FROM messages WHERE id = ?", msgID).Scan(&deadline)
	assert.NotNil(t, deadline)
	assert.Equal(t, "2026-06-01", *deadline)

	// Clear the deadline by setting to empty string.
	body2 := map[string]interface{}{
		"id":       msgID,
		"deadline": "",
	}
	bodyBytes2, _ := json.Marshal(body2)
	req2 := httptest.NewRequest("PATCH", "/api/message?jwt="+ownerToken, bytes.NewBuffer(bodyBytes2))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err2 := getApp().Test(req2)
	assert.NoError(t, err2)
	assert.Equal(t, 200, resp2.StatusCode)

	// Verify deadline is now NULL.
	var deadline2 *string
	db.Raw("SELECT deadline FROM messages WHERE id = ?", msgID).Scan(&deadline2)
	assert.Nil(t, deadline2, "Deadline should be NULL after clearing")
}

func TestGetMessageWorryWordsGroupMod(t *testing.T) {
	// Verify that a group-level moderator (systemrole=User, membership role=Moderator)
	// can see worry words. This tests the fix where worry words are shown
	// to any group mod, not just system-level mods.
	prefix := uniquePrefix("msg_worry_grpmod")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")

	// Create a group-level mod: systemrole='User' but membership role='Moderator'.
	groupModID := CreateTestUser(t, prefix+"_grpmod", "User")
	CreateTestMembership(t, groupModID, groupID, "Moderator")
	_, groupModToken := CreateTestSession(t, groupModID)

	// Insert a unique worry word.
	worryKeyword := "grpmodworry" + fmt.Sprintf("%d", time.Now().UnixNano()%100000)
	db.Exec("INSERT INTO worrywords (keyword, type) VALUES (?, 'Regulated')", worryKeyword)
	defer db.Exec("DELETE FROM worrywords WHERE keyword = ?", worryKeyword)

	// Create message containing the worry word.
	msgID := CreateTestMessage(t, posterID, groupID, "OFFER: "+worryKeyword+" near here", 52.5, -1.8)

	// Fetch as group-level mod — should see worry words.
	resp, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/message/%d?jwt=%s", msgID, groupModToken), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msg map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&msg)

	worry, hasWorry := msg["worry"]
	assert.True(t, hasWorry, "Group-level mod (systemrole=User) should see worry field")
	worryList := worry.([]interface{})
	assert.GreaterOrEqual(t, len(worryList), 1, "Should have at least 1 worry match")

	wm := worryList[0].(map[string]interface{})
	assert.Equal(t, worryKeyword, wm["word"])
	ww := wm["worryword"].(map[string]interface{})
	assert.Equal(t, worryKeyword, ww["keyword"])
	assert.Equal(t, "Regulated", ww["type"])
}

// TestMessagePostWritesHistory verifies that the JoinAndPost submit path writes a messages_history row.
// V1 parity: Message::save() does INSERT IGNORE INTO messages_history when a message is posted.
func TestMessagePostWritesHistory(t *testing.T) {
	prefix := uniquePrefix("msg_hist")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	// Create a draft message.
	db.Exec("INSERT INTO messages (fromuser, type, subject, textbody, arrival, date, source) VALUES (?, 'Offer', 'Offer: History test chair', 'A free chair', NOW(), NOW(), 'Platform')", userID)
	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", userID).Scan(&msgID)
	require.NotZero(t, msgID, "Failed to create draft message")
	db.Exec("INSERT INTO messages_drafts (msgid, groupid, userid) VALUES (?, ?, ?)", msgID, groupID, userID)

	// Submit via JoinAndPost.
	body, _ := json.Marshal(map[string]interface{}{
		"id":     msgID,
		"action": "JoinAndPost",
	})
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/message?jwt=%s", token), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify a messages_history row was created for this message+group.
	var histCount int64
	db.Raw("SELECT COUNT(*) FROM messages_history WHERE msgid = ? AND groupid = ? AND source = 'Platform' AND fromuser = ?",
		msgID, groupID, userID).Scan(&histCount)
	assert.Equal(t, int64(1), histCount, "JoinAndPost should insert a messages_history row")

	// Verify the subject was recorded.
	var histSubject *string
	db.Raw("SELECT subject FROM messages_history WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&histSubject)
	assert.NotNil(t, histSubject)
	assert.Equal(t, "Offer: History test chair", *histSubject)

	// Verify fromip was recorded in messages_history.
	var histFromip *string
	db.Raw("SELECT fromip FROM messages_history WHERE msgid = ? AND groupid = ?", msgID, groupID).Scan(&histFromip)
	assert.NotNil(t, histFromip, "JoinAndPost should record fromip in messages_history")
}

// TestMessageEditRecordsAllColumns verifies that the PATCH /message edit path records
// olditems, newitems, oldimages, newimages, oldlocation, newlocation in messages_edits.
// V1 parity: Message::save() inserts all 15 columns into messages_edits.
func TestMessageEditRecordsAllColumns(t *testing.T) {
	prefix := uniquePrefix("msg_edit_full")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	CreateTestMembership(t, ownerID, groupID, "Member")
	_, ownerToken := CreateTestSession(t, ownerID)

	// Create a message with an item and a known locationid.
	db.Exec("INSERT INTO locations (name, type, lat, lng) VALUES (?, 'Other', 52.5, -1.8)", prefix+"_Area")
	var areaID uint64
	db.Raw("SELECT id FROM locations WHERE name = ? ORDER BY id DESC LIMIT 1", prefix+"_Area").Scan(&areaID)
	db.Exec("INSERT INTO locations (name, type, lat, lng, areaid) VALUES (?, 'Postcode', 52.5, -1.8, ?)", prefix+"_B25 8FF", areaID)
	var pcID uint64
	db.Raw("SELECT id FROM locations WHERE name = ? ORDER BY id DESC LIMIT 1", prefix+"_B25 8FF").Scan(&pcID)

	db.Exec("INSERT INTO items (name) VALUES (?)", prefix+"_Sofa")
	var itemID uint64
	db.Raw("SELECT id FROM items WHERE name = ? ORDER BY id DESC LIMIT 1", prefix+"_Sofa").Scan(&itemID)
	require.NotZero(t, itemID)

	msgID := createPendingMessage(t, ownerID, groupID, prefix)
	db.Exec("UPDATE messages SET locationid = ? WHERE id = ?", pcID, msgID)
	db.Exec("INSERT IGNORE INTO messages_items (msgid, itemid) VALUES (?, ?)", msgID, itemID)

	// Create a new item to edit to (so items change).
	db.Exec("INSERT INTO items (name) VALUES (?)", prefix+"_Chair")
	var newItemID uint64
	db.Raw("SELECT id FROM items WHERE name = ? ORDER BY id DESC LIMIT 1", prefix+"_Chair").Scan(&newItemID)
	require.NotZero(t, newItemID)

	// Create a new location to edit to.
	db.Exec("INSERT INTO locations (name, type, lat, lng) VALUES (?, 'Other', 53.0, -2.0)", prefix+"_NewArea")
	var newAreaID uint64
	db.Raw("SELECT id FROM locations WHERE name = ? ORDER BY id DESC LIMIT 1", prefix+"_NewArea").Scan(&newAreaID)
	db.Exec("INSERT INTO locations (name, type, lat, lng, areaid) VALUES (?, 'Postcode', 53.0, -2.0, ?)", prefix+"_M1 1AA", newAreaID)
	var newPcID uint64
	db.Raw("SELECT id FROM locations WHERE name = ? ORDER BY id DESC LIMIT 1", prefix+"_M1 1AA").Scan(&newPcID)

	// PATCH the message: change item and location.
	body, _ := json.Marshal(map[string]interface{}{
		"id":         msgID,
		"item":       prefix + "_Chair",
		"locationid": newPcID,
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/message?jwt=%s", ownerToken), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify edit record was created.
	var editID uint64
	var oldItems, newItems, oldLocation, newLocation *string
	row := db.Raw("SELECT id, olditems, newitems, CAST(oldlocation AS CHAR), CAST(newlocation AS CHAR) FROM messages_edits WHERE msgid = ? AND byuser = ? ORDER BY id DESC LIMIT 1",
		msgID, ownerID).Row()
	err = row.Scan(&editID, &oldItems, &newItems, &oldLocation, &newLocation)
	assert.NoError(t, err)
	assert.NotZero(t, editID, "Edit record should be created")

	// Verify items were recorded.
	assert.NotNil(t, oldItems, "olditems should be set")
	assert.NotNil(t, newItems, "newitems should be set")
	assert.Contains(t, *oldItems, fmt.Sprintf("%d", itemID), "olditems should contain original item ID")
	assert.Contains(t, *newItems, fmt.Sprintf("%d", newItemID), "newitems should contain new item ID")

	// Verify location was recorded.
	assert.NotNil(t, oldLocation, "oldlocation should be set")
	assert.NotNil(t, newLocation, "newlocation should be set")
	assert.Equal(t, fmt.Sprintf("%d", pcID), *oldLocation, "oldlocation should be original postcode location ID")
	assert.Equal(t, fmt.Sprintf("%d", newPcID), *newLocation, "newlocation should be new postcode location ID")
}

// TestMessageAiDeclinedWritesTable is a gap documentation test.
// V1 line 3999: INSERT IGNORE INTO messages_ai_declined (msgid) when AI check declines a message.
// Go V2 has NO AI service integration in the web post path — this functionality is entirely absent.
// This test verifies the table exists and is writable (documents the gap without requiring AI).
func TestMessageAiDeclinedWritesTable(t *testing.T) {
	prefix := uniquePrefix("msg_ai_dec")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	msgID := CreateTestMessage(t, userID, groupID, prefix+" AI declined item", 55.9533, -3.1883)

	// Directly insert into messages_ai_declined to verify the table is writable.
	// In V1, this is done by the AI spam check when it declines a message.
	// In V2 Go, there is no AI check integration — this is a known parity gap.
	result := db.Exec("INSERT IGNORE INTO messages_ai_declined (msgid) VALUES (?)", msgID)
	assert.NoError(t, result.Error, "messages_ai_declined table should accept inserts")

	// Verify the row was written.
	var count int64
	db.Raw("SELECT COUNT(*) FROM messages_ai_declined WHERE msgid = ?", msgID).Scan(&count)
	assert.Equal(t, int64(1), count, "messages_ai_declined should have a row for the message")

	// NOTE: Gap — Go V2 never calls an AI service for web-posted messages.
	// V1 checks messages against an AI model and inserts into messages_ai_declined when declined.
	// The Go implementation would need to call an AI service and insert here when the AI declines.
}

func TestGetMessagePostings(t *testing.T) {
	prefix := uniquePrefix("MsgPostings")
	db := database.DBConn

	modID := CreateTestUser(t, prefix+"_mod", "User")
	_, modToken := CreateTestSession(t, modID)
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")

	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")

	msgID := CreateTestMessage(t, userID, groupID, "OFFER: Test Postings Item", 55.9533, -3.1883)

	// Add a posting record.
	db.Exec("INSERT INTO messages_postings (msgid, groupid, date) VALUES (?, ?, NOW() - INTERVAL 2 DAY)", msgID, groupID)

	// Mod fetches the message — should include postings.
	url := fmt.Sprintf("/api/message/%d?jwt=%s", msgID, modToken)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var msg map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&msg)

	postings, ok := msg["postings"].([]interface{})
	assert.True(t, ok, "postings should be an array")
	assert.GreaterOrEqual(t, len(postings), 1, "should have at least one posting")

	posting := postings[0].(map[string]interface{})
	assert.Equal(t, float64(msgID), posting["msgid"])
	assert.Equal(t, float64(groupID), posting["groupid"])
	assert.NotEmpty(t, posting["date"])
	assert.NotEmpty(t, posting["namedisplay"])
}
