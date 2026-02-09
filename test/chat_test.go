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
