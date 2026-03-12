package test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

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
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?jwt=%s", modToken), nil)
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
