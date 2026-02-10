package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/chat"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	url2 "net/url"
	"os"
	"testing"
	"time"
)

func TestListChats(t *testing.T) {
	// Create a full test user with chats
	prefix := uniquePrefix("chat")
	userID, token := CreateFullTestUser(t, prefix)

	// Logged out - should return 401
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/chat?includeClosed=true", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Get chats for user
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var chats []chat.ChatRoomListEntry
	json2.Unmarshal(rsp(resp), &chats)

	// Should find chats
	assert.Greater(t, len(chats), 0)

	// Find a chat with a snippet
	found := (uint64)(0)
	for _, c := range chats {
		if len(c.Snippet) > 0 {
			found = c.ID
		}
	}
	assert.Greater(t, found, (uint64)(0), "Should find a chat with a snippet for user %d", userID)

	// Get with since param
	url := "/api/chat?jwt=" + token + "&since=" + url2.QueryEscape(time.Now().Format(time.RFC3339))
	resp, _ = getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Get with search param
	url = "/api/chat?jwt=" + token + "&search=test"
	resp, _ = getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Get the chat
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/"+fmt.Sprint(found)+"?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var c chat.ChatRoomListEntry
	json2.Unmarshal(rsp(resp), &c)
	assert.Equal(t, found, c.ID)

	// Get the messages
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/"+fmt.Sprint(found)+"/message?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var messages []chat.ChatMessage
	json2.Unmarshal(rsp(resp), &messages)
	assert.Equal(t, found, messages[0].Chatid)

	// Get an invalid chat - no auth
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/"+fmt.Sprint(found), nil))
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	// Invalid chat ID format
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/z?jwt="+token, nil))
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// Non-existent chat
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/1?jwt="+token, nil))
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	// Get invalid chat messages - no auth
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/1/message", nil))
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	// Non-existent chat messages
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/1/message?jwt="+token, nil))
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	// Invalid chat ID format for messages
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/z/message?jwt="+token, nil))
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestCreateChatMessage(t *testing.T) {
	// Invalid chat id
	resp, _ := getApp().Test(httptest.NewRequest("POST", "/api/chat/-1/message", nil))
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// Create a mod user with a User2Mod chat for testing
	prefix := uniquePrefix("chatmsg")
	groupID := CreateTestGroup(t, prefix)
	modUserID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modUserID, groupID, "Moderator")
	chatid := CreateTestChatRoom(t, modUserID, nil, &groupID, "User2Mod")
	CreateTestChatMessage(t, chatid, modUserID, "Initial message")
	_, token := CreateTestSession(t, modUserID)

	// Logged out
	resp, _ = getApp().Test(httptest.NewRequest("POST", "/api/chat/"+fmt.Sprint(chatid)+"/message", nil))
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	// Undecodable payload
	request := httptest.NewRequest("POST", "/api/chat/"+fmt.Sprint(chatid)+"/message?jwt="+token, bytes.NewBuffer([]byte("Test")))
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// Invalid payload
	var payload chat.ChatMessage
	s, _ := json2.Marshal(payload)
	b := bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/"+fmt.Sprint(chatid)+"/message?jwt="+token, b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// Valid payload
	chatrsp := struct {
		Id uint64 `json:"id"`
	}{}

	str := "Test basic message"
	payload.Message = str
	s, _ = json2.Marshal(payload)
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/"+fmt.Sprint(chatid)+"/message?jwt="+token, b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &chatrsp)
	assert.Greater(t, chatrsp.Id, (uint64)(1))
}

func TestCreateChatMessageLoveJunk(t *testing.T) {
	// Create test data for LoveJunk integration test
	prefix := uniquePrefix("lovejunk")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")

	// Create a message with spaces in subject (required for LoveJunk)
	msgID := CreateTestMessage(t, userID, groupID, "Test Offer Item", 55.9533, -3.1883)

	var payload chat.ChatMessageLovejunk

	payload.Refmsgid = &msgID
	firstname := "Test"
	payload.Firstname = &firstname
	lastname := "User"
	payload.Lastname = &lastname

	// Use longer timeout for LoveJunk tests - DB lookups can be slow under CI load.
	timeout := 5000

	// Without ljuserid
	s, _ := json2.Marshal(payload)
	b := bytes.NewBuffer(s)
	request := httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request, timeout)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// Without partnerkey
	ljuserid := uint64(time.Now().UnixNano())
	payload.Ljuserid = &ljuserid
	s, _ = json2.Marshal(payload)
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request, timeout)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// With invalid partnerkey
	payload.Partnerkey = "invalid"
	s, _ = json2.Marshal(payload)
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request, timeout)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	// Remaining tests require a valid LOVEJUNK_PARTNER_KEY env var.
	partnerKey := os.Getenv("LOVEJUNK_PARTNER_KEY")
	if partnerKey == "" {
		t.Log("LOVEJUNK_PARTNER_KEY not set, skipping integration tests")
		return
	}

	// With valid partnerkey but no message
	payload.Partnerkey = partnerKey
	s, _ = json2.Marshal(payload)
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request, timeout)
	if !assert.NotNil(t, resp, "expected response for valid partnerkey with no message") {
		return
	}
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// Valid
	payload.Message = "Test message"
	loc := "EH3 6SS"
	payload.PostcodePrefix = &loc
	s, _ = json2.Marshal(payload)
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request, timeout)
	if !assert.NotNil(t, resp, "expected response for valid LoveJunk request") {
		return
	}
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var ret chat.ChatMessageLovejunkResponse
	json2.Unmarshal(rsp(resp), &ret)
	assert.Greater(t, ret.Id, (uint64)(0))
	assert.Greater(t, ret.Chatid, (uint64)(0))

	// Initial reply
	payload.Message = "Test initial reply"
	payload.Initialreply = true
	offerid := uint64(123)
	payload.Offerid = &offerid
	s, _ = json2.Marshal(payload)
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request, timeout)
	if !assert.NotNil(t, resp, "expected response for initial reply") {
		return
	}
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &ret)
	assert.Greater(t, ret.Id, (uint64)(0))
	assert.Greater(t, ret.Chatid, (uint64)(0))
	assert.Greater(t, ret.Userid, (uint64)(0))

	// Fake a ban of the LJ user on the group
	var ban user.UserBanned
	ban.Userid = ret.Userid
	ban.Groupid = groupID
	ban.Byuser = ret.Userid
	db := database.DBConn
	db.Create(&ban)

	// Shouldn't be able to reply to a message on this group
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request, timeout)
	if !assert.NotNil(t, resp, "expected response for banned user") {
		return
	}
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestUserBanned(t *testing.T) {
	db := database.DBConn

	// Create test user and group for ban test
	prefix := uniquePrefix("banned")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")

	// Create a ban
	var ban user.UserBanned
	ban.Userid = userID
	ban.Groupid = groupID
	ban.Byuser = userID
	db.Create(&ban)
}

func TestPostChatRoomNotLoggedIn(t *testing.T) {
	// POST without auth should return 401
	payload := map[string]interface{}{"id": 1, "action": "Nudge"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms", bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestPostChatRoomNudge(t *testing.T) {
	prefix := uniquePrefix("nudge")

	// Create two users and a User2User chat
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	CreateTestChatMessage(t, chatid, user1ID, "Hello")
	_, token := CreateTestSession(t, user1ID)

	// Nudge
	payload := map[string]interface{}{"id": chatid, "action": "Nudge"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Greater(t, result["id"], float64(0))

	// Nudge again - should return existing nudge ID (not create a new one)
	s, _ = json2.Marshal(payload)
	request = httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result2 map[string]interface{}
	json2.Unmarshal(rsp(resp), &result2)
	assert.Equal(t, float64(0), result2["ret"])
	assert.Equal(t, result["id"], result2["id"])
}

func TestPostChatRoomNudgeNotMember(t *testing.T) {
	prefix := uniquePrefix("nudgenm")

	// Create chat between user1 and user2, but try to nudge as user3
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	user3ID := CreateTestUser(t, prefix+"_u3", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token3 := CreateTestSession(t, user3ID)

	payload := map[string]interface{}{"id": chatid, "action": "Nudge"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token3, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestPostChatRoomTyping(t *testing.T) {
	prefix := uniquePrefix("typing")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token := CreateTestSession(t, user1ID)

	// Create roster entry first (typing updates existing roster entry)
	db := database.DBConn
	db.Exec("INSERT INTO chat_roster (chatid, userid, status) VALUES (?, ?, 'Online')", chatid, user1ID)

	// Create a recent unmailed chat message to verify date bump
	msgid := CreateTestChatMessage(t, chatid, user1ID, "Recent typing msg "+prefix)
	// Set date to 10 seconds ago and mailedtoall = 0 (within the 30s window)
	db.Exec("UPDATE chat_messages SET date = DATE_SUB(NOW(), INTERVAL 10 SECOND), mailedtoall = 0 WHERE id = ?", msgid)

	var dateBefore string
	db.Raw("SELECT date FROM chat_messages WHERE id = ?", msgid).Scan(&dateBefore)

	payload := map[string]interface{}{"id": chatid, "action": "Typing"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify date was bumped (should be newer than before)
	var dateAfter string
	db.Raw("SELECT date FROM chat_messages WHERE id = ?", msgid).Scan(&dateAfter)
	assert.NotEqual(t, dateBefore, dateAfter, "Typing should bump recent unmailed message dates")
}

func TestPostChatRoomRosterUpdate(t *testing.T) {
	prefix := uniquePrefix("roster")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	msgid := CreateTestChatMessage(t, chatid, user2ID, "Hello from user2")
	_, token := CreateTestSession(t, user1ID)

	// Mark as read (roster update with lastmsgseen)
	payload := map[string]interface{}{"id": chatid, "lastmsgseen": msgid}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.NotNil(t, result["roster"])
	// Unseen should be 0 since we just marked the message as seen
	assert.Equal(t, float64(0), result["unseen"])
}

func TestPostChatRoomHideChat(t *testing.T) {
	prefix := uniquePrefix("hide")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token := CreateTestSession(t, user1ID)

	// Hide chat (status=Closed)
	payload := map[string]interface{}{"id": chatid, "status": "Closed"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify roster entry has Closed status
	roster := result["roster"].([]interface{})
	found := false
	for _, r := range roster {
		entry := r.(map[string]interface{})
		if uint64(entry["userid"].(float64)) == user1ID {
			assert.Equal(t, "Closed", entry["status"])
			found = true
		}
	}
	assert.True(t, found, "Should find user in roster")
}

func TestPostChatRoomBlockChat(t *testing.T) {
	prefix := uniquePrefix("block")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token := CreateTestSession(t, user1ID)

	// Block chat
	payload := map[string]interface{}{"id": chatid, "status": "Blocked"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Now try to close (should remain Blocked since BLOCKED takes precedence)
	payload = map[string]interface{}{"id": chatid, "status": "Closed"}
	s, _ = json2.Marshal(payload)
	request = httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	roster := result["roster"].([]interface{})
	for _, r := range roster {
		entry := r.(map[string]interface{})
		if uint64(entry["userid"].(float64)) == user1ID {
			// Should still be Blocked, not Closed
			assert.Equal(t, "Blocked", entry["status"])
		}
	}
}

func TestPostChatRoomRosterUpdateNonMember(t *testing.T) {
	prefix := uniquePrefix("rosternm")

	// Create chat between user1 and user2
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	user3ID := CreateTestUser(t, prefix+"_u3", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token3 := CreateTestSession(t, user3ID)

	// User3 tries to update roster for a chat they're not in - should fail
	payload := map[string]interface{}{"id": chatid}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token3, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestPostChatRoomUnhide(t *testing.T) {
	prefix := uniquePrefix("unhide")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token := CreateTestSession(t, user1ID)

	// Hide chat first
	payload := map[string]interface{}{"id": chatid, "status": "Closed"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Unhide chat (status=Online)
	payload = map[string]interface{}{"id": chatid, "status": "Online"}
	s, _ = json2.Marshal(payload)
	request = httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	roster := result["roster"].([]interface{})
	for _, r := range roster {
		entry := r.(map[string]interface{})
		if uint64(entry["userid"].(float64)) == user1ID {
			assert.Equal(t, "Online", entry["status"])
		}
	}
}

// --- Adversarial tests ---

func TestPostChatRoomDoubleNudge(t *testing.T) {
	// Second nudge should be idempotent (returns existing nudge ID).
	prefix := uniquePrefix("nudge_dbl")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	CreateTestChatMessage(t, chatid, user2ID, "Hello")
	_, token := CreateTestSession(t, user1ID)

	payload := map[string]interface{}{"id": chatid, "action": "Nudge"}
	s, _ := json2.Marshal(payload)

	// First nudge.
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	var r1 map[string]interface{}
	json2.Unmarshal(rsp(resp), &r1)
	firstID := r1["id"]

	// Second nudge - should return same ID (not create a new one).
	s, _ = json2.Marshal(payload)
	request = httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	var r2 map[string]interface{}
	json2.Unmarshal(rsp(resp), &r2)
	assert.Equal(t, firstID, r2["id"], "Double nudge should return the same ID")
}

func TestPostChatRoomHideAlreadyHidden(t *testing.T) {
	// Hiding an already-hidden chat should be idempotent.
	prefix := uniquePrefix("hide_dbl")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token := CreateTestSession(t, user1ID)

	payload := map[string]interface{}{"id": chatid, "status": "Closed"}
	s, _ := json2.Marshal(payload)

	// First hide.
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Second hide - should succeed without error.
	s, _ = json2.Marshal(payload)
	request = httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestPostChatRoomBlockThenClose(t *testing.T) {
	// Block then Close should NOT downgrade to Closed (Block takes precedence).
	prefix := uniquePrefix("blk_cls")
	db := database.DBConn

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token := CreateTestSession(t, user1ID)

	// Block.
	payload := map[string]interface{}{"id": chatid, "status": "Blocked"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Try to Close (should NOT override Blocked).
	payload = map[string]interface{}{"id": chatid, "status": "Closed"}
	s, _ = json2.Marshal(payload)
	request = httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Status should still be Blocked.
	var status string
	db.Raw("SELECT status FROM chat_roster WHERE chatid = ? AND userid = ?", chatid, user1ID).Scan(&status)
	assert.Equal(t, "Blocked", status, "Closed should not override Blocked")
}

func TestPostChatRoomNonExistentChat(t *testing.T) {
	prefix := uniquePrefix("chat_ne")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	// Roster update on non-existent chat.
	payload := map[string]interface{}{"id": 999999999, "status": "Online"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)

	// Should return ret=2 (not visible), not crash.
	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestPostChatRoomEmptyBody(t *testing.T) {
	prefix := uniquePrefix("chat_empty")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer([]byte("{}")))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)

	// Empty body with no action and no id should return 400.
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestPostChatRoomTypingNonExistentChat(t *testing.T) {
	prefix := uniquePrefix("type_ne")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	// Typing on non-existent chat should succeed (just does UPDATE with 0 rows affected).
	payload := map[string]interface{}{"id": 999999999, "action": "Typing"}
	s, _ := json2.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}
