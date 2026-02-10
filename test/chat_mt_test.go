package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

// Helper to set up a moderator with a group and User2Mod chat containing messages.
func setupModChatData(t *testing.T, prefix string) (modID uint64, userID uint64, groupID uint64, chatID uint64, token string) {
	modID = CreateTestUser(t, prefix+"_mod", "Moderator")
	userID = CreateTestUser(t, prefix+"_user", "User")
	groupID = CreateTestGroup(t, prefix+"_group")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, userID, groupID, "Member")

	chatID = CreateTestChatRoom(t, userID, nil, &groupID, "User2Mod")

	db := database.DBConn
	// Create messages that are visible (processingsuccessful=1).
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Hello from user', NOW(), 1, 0, 0)",
		chatID, userID)
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Another message', NOW(), 1, 0, 0)",
		chatID, userID)
	// Update latestmessage on room.
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", chatID)

	_, token = CreateTestSession(t, modID)
	return
}

func TestUnseenCountMT(t *testing.T) {
	prefix := uniquePrefix("UnseenMT")
	modID, _, _, _, token := setupModChatData(t, prefix)
	_ = modID

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatrooms?count=true&chattypes=User2Mod,Mod2Mod&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "count")
	// Should have at least 2 unseen messages (no roster entry = all unseen).
	assert.GreaterOrEqual(t, result["count"].(float64), float64(2))
}

func TestUnseenCountMTNotLoggedIn(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/chatrooms?count=true&chattypes=User2Mod,Mod2Mod", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(1), result["ret"])
}

func TestUnseenCountMTZeroWhenSeen(t *testing.T) {
	prefix := uniquePrefix("UnseenSeen")
	modID, _, _, chatID, token := setupModChatData(t, prefix)

	db := database.DBConn
	// Mark all as seen by creating/updating roster entry with high lastmsgseen.
	db.Exec("INSERT INTO chat_roster (chatid, userid, lastmsgseen, status, date) VALUES (?, ?, 999999999, 'Online', NOW()) ON DUPLICATE KEY UPDATE lastmsgseen = 999999999",
		chatID, modID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatrooms?count=true&chattypes=User2Mod,Mod2Mod&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, float64(0), result["count"])
}

func TestFetchChatMT(t *testing.T) {
	prefix := uniquePrefix("FetchMT")
	_, _, _, chatID, token := setupModChatData(t, prefix)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatrooms?id=%d&chattypes=User2Mod,Mod2Mod&jwt=%s", chatID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "chatroom")

	chatroom := result["chatroom"].(map[string]interface{})
	assert.Equal(t, float64(chatID), chatroom["id"])
	assert.Equal(t, "User2Mod", chatroom["chattype"])
	assert.Contains(t, chatroom, "unseen")
}

func TestFetchChatMTPermissionDenied(t *testing.T) {
	prefix := uniquePrefix("FetchPerm")
	_, _, _, chatID, _ := setupModChatData(t, prefix)

	// Create a different user who is NOT a moderator of the group.
	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, otherToken := CreateTestSession(t, otherID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatrooms?id=%d&chattypes=User2Mod,Mod2Mod&jwt=%s", chatID, otherToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestListChatsMT(t *testing.T) {
	prefix := uniquePrefix("ListMT")
	_, _, _, _, token := setupModChatData(t, prefix)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/rooms?chattypes=User2Mod,Mod2Mod&summary=true&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "chatrooms")

	chatrooms := result["chatrooms"].([]interface{})
	assert.GreaterOrEqual(t, len(chatrooms), 1)

	first := chatrooms[0].(map[string]interface{})
	assert.Contains(t, first, "id")
	assert.Contains(t, first, "chattype")
	assert.Contains(t, first, "name")
	assert.Contains(t, first, "unseen")
}

func TestListChatsMTMod2Mod(t *testing.T) {
	prefix := uniquePrefix("ListM2M")
	mod1ID := CreateTestUser(t, prefix+"_mod1", "Moderator")
	mod2ID := CreateTestUser(t, prefix+"_mod2", "Moderator")
	groupID := CreateTestGroup(t, prefix+"_group")
	CreateTestMembership(t, mod1ID, groupID, "Moderator")
	CreateTestMembership(t, mod2ID, groupID, "Moderator")

	chatID := CreateTestChatRoom(t, mod1ID, &mod2ID, &groupID, "Mod2Mod")

	db := database.DBConn
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Mod message', NOW(), 1, 0, 0)",
		chatID, mod1ID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", chatID)

	_, token := CreateTestSession(t, mod2ID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/rooms?chattypes=User2Mod,Mod2Mod&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	chatrooms := result["chatrooms"].([]interface{})
	// Should contain the Mod2Mod chat.
	found := false
	for _, cr := range chatrooms {
		room := cr.(map[string]interface{})
		if room["id"].(float64) == float64(chatID) {
			found = true
			assert.Equal(t, "Mod2Mod", room["chattype"])
			break
		}
	}
	assert.True(t, found, "Should find the Mod2Mod chat in the list")
}

func TestListChatsMTEmpty(t *testing.T) {
	prefix := uniquePrefix("ListEmpty")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	// User has no moderator memberships, so should get empty list.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/rooms?chattypes=User2Mod,Mod2Mod&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	chatrooms := result["chatrooms"].([]interface{})
	assert.Equal(t, 0, len(chatrooms))
}

func TestListChatsMTSearch(t *testing.T) {
	prefix := uniquePrefix("ListSearch")
	_, _, _, _, token := setupModChatData(t, prefix)

	// Search for message content.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/rooms?chattypes=User2Mod,Mod2Mod&search=Hello&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "chatrooms")
}

func TestReviewChatMessages(t *testing.T) {
	prefix := uniquePrefix("ReviewMsgs")
	modID, userID, groupID, _, token := setupModChatData(t, prefix)
	_ = modID

	// Create a User2User chat between users where one is in the mod's group.
	user2ID := CreateTestUser(t, prefix+"_user2", "User")
	u2uChatID := CreateTestChatRoom(t, userID, &user2ID, nil, "User2User")

	db := database.DBConn
	// Create a message pending review.
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Spam message', NOW(), 1, 1, 0)",
		u2uChatID, user2ID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", u2uChatID)

	// Also create a User2Mod review message.
	u2mChatID := CreateTestChatRoom(t, user2ID, nil, &groupID, "User2Mod")
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, processingsuccessful, reviewrequired, reviewrejected) VALUES (?, ?, 'Review this', NOW(), 1, 1, 0)",
		u2mChatID, user2ID)
	db.Exec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", u2mChatID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?limit=10&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "chatmessages")
	assert.Contains(t, result, "context")

	msgs := result["chatmessages"].([]interface{})
	assert.GreaterOrEqual(t, len(msgs), 1)

	// Each message should have a chatroom embedded.
	first := msgs[0].(map[string]interface{})
	assert.Contains(t, first, "chatroom")
	chatroom := first["chatroom"].(map[string]interface{})
	assert.Contains(t, chatroom, "chattype")
}

func TestReviewChatMessagesNotModerator(t *testing.T) {
	prefix := uniquePrefix("ReviewNonMod")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	// Non-moderator should get empty list.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	msgs := result["chatmessages"].([]interface{})
	assert.Equal(t, 0, len(msgs))
}

func TestChatMessagesForRoom(t *testing.T) {
	prefix := uniquePrefix("RoomMsgs")
	_, _, _, chatID, token := setupModChatData(t, prefix)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?roomid=%d&jwt=%s", chatID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "chatmessages")

	msgs := result["chatmessages"].([]interface{})
	assert.GreaterOrEqual(t, len(msgs), 2)
}

func TestChatMessagesForRoomPermissionDenied(t *testing.T) {
	prefix := uniquePrefix("RoomPerm")
	_, _, _, chatID, _ := setupModChatData(t, prefix)

	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, otherToken := CreateTestSession(t, otherID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chatmessages?roomid=%d&jwt=%s", chatID, otherToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestGetChatRoomsMTV2Path(t *testing.T) {
	req := httptest.NewRequest("GET", "/apiv2/chatrooms", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestListChatRoomsMTV2Path(t *testing.T) {
	req := httptest.NewRequest("GET", "/apiv2/chat/rooms", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestReviewChatMessageV2Path(t *testing.T) {
	req := httptest.NewRequest("GET", "/apiv2/chatmessages", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}
