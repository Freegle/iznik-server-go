package test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/message"
	"github.com/stretchr/testify/assert"
)

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
	assert.Equal(t, 400, resp.StatusCode)
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

// =============================================================================
// Message fetchMT with messagehistory=true Tests
// =============================================================================

func TestGetMessageWithHistory(t *testing.T) {
	prefix := uniquePrefix("msghist")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	msgID := CreateTestMessage(t, posterID, groupID, prefix+" History Test Item", 55.9533, -3.1883)

	// Hold the message so we can verify heldby info.
	db.Exec("UPDATE messages SET heldby = ? WHERE id = ?", modID, msgID)

	// Fetch with messagehistory=true as moderator.
	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/message/%d?messagehistory=true&jwt=%s", msgID, modToken), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result message.MessageWithHistory
	json.NewDecoder(resp.Body).Decode(&result)

	assert.Equal(t, msgID, result.ID)
	assert.NotNil(t, result.MessageHistoryData, "Should have messagehistory data for moderator")

	// Verify groups.
	assert.Greater(t, len(result.MessageHistoryData.Groups), 0, "Should have group info")
	foundGroup := false
	for _, g := range result.MessageHistoryData.Groups {
		if g.Groupid == groupID {
			foundGroup = true
			break
		}
	}
	assert.True(t, foundGroup, "Should find the test group in history")

	// Verify poster emails.
	assert.Greater(t, len(result.MessageHistoryData.PosterEmails), 0, "Should have poster emails")

	// Verify recent posts.
	assert.Greater(t, len(result.MessageHistoryData.RecentPosts), 0, "Should have recent posts")

	// Verify held-by info.
	assert.NotNil(t, result.MessageHistoryData.HeldBy, "Should have heldby info")
	assert.Equal(t, modID, result.MessageHistoryData.HeldBy.ID)
}

func TestGetMessageWithHistoryNotMod(t *testing.T) {
	prefix := uniquePrefix("msghist_nomod")

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	regularID := CreateTestUser(t, prefix+"_regular", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, regularID, groupID, "Member")
	_, regularToken := CreateTestSession(t, regularID)

	msgID := CreateTestMessage(t, posterID, groupID, prefix+" No Mod History", 55.9533, -3.1883)

	// Fetch with messagehistory=true as regular member - should succeed but WITHOUT history.
	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/message/%d?messagehistory=true&jwt=%s", msgID, regularToken), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	// Should have message data but NO history.
	assert.NotNil(t, result["id"])
	assert.Nil(t, result["messagehistory"], "Non-mod should not see message history")
}

func TestGetMessageWithHistoryNoAuth(t *testing.T) {
	prefix := uniquePrefix("msghist_noauth")

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")

	msgID := CreateTestMessage(t, posterID, groupID, prefix+" No Auth History", 55.9533, -3.1883)

	// Fetch with messagehistory=true but no auth - should return message without history.
	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/message/%d?messagehistory=true", msgID), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.NotNil(t, result["id"])
	assert.Nil(t, result["messagehistory"], "Unauthenticated should not see message history")
}

func TestGetMessageWithHistoryAdmin(t *testing.T) {
	prefix := uniquePrefix("msghist_admin")

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	CreateTestMembership(t, posterID, groupID, "Member")
	// Admin is NOT a member of the group.
	_, adminToken := CreateTestSession(t, adminID)

	msgID := CreateTestMessage(t, posterID, groupID, prefix+" Admin History Test", 55.9533, -3.1883)

	// Admin should see history even without group membership.
	resp, err := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/message/%d?messagehistory=true&jwt=%s", msgID, adminToken), nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result message.MessageWithHistory
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, msgID, result.ID)
	assert.NotNil(t, result.MessageHistoryData, "Admin should see messagehistory")
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
