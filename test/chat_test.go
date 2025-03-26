package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/chat"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"iznik-server-go/database"
	user2 "iznik-server-go/user"
	"net/http/httptest"
	url2 "net/url"
	"os"
	"testing"
	"time"
)

func TestListChats(t *testing.T) {
	_, token := GetUserWithToken(t)

	// Logged out
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/chat?includeClosed=true", nil))
	assert.Equal(t, 401, resp.StatusCode)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var chats []chat.ChatRoomListEntry
	json2.Unmarshal(rsp(resp), &chats)

	// Should find a chat with a name.
	assert.Greater(t, len(chats), 0)
	assert.Greater(t, len(chats[0].Name), 0)
	assert.Greater(t, len(chats[0].Icon), 0)

	// At least one should have a snippet.
	found := (uint64)(0)

	for _, chat := range chats {
		if len(chat.Snippet) > 0 {
			found = chat.ID
		}
	}
	assert.Greater(t, found, (uint64)(0))

	// Get with since param.
	url := "/api/chat?jwt=" + token + "&since=" + url2.QueryEscape(time.Now().Format(time.RFC3339))
	resp, _ = getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Get with search param.
	url = "/api/chat?jwt=" + token + "&search=test"
	resp, _ = getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Get the chat.
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/"+fmt.Sprint(found)+"?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var c chat.ChatRoomListEntry
	json2.Unmarshal(rsp(resp), &c)
	assert.Equal(t, found, c.ID)

	// Get the messages.
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/"+fmt.Sprint(found)+"/message?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var messages []chat.ChatMessage
	json2.Unmarshal(rsp(resp), &messages)
	assert.Equal(t, found, messages[0].Chatid)

	// Get an invalid chat
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/"+fmt.Sprint(found), nil))
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/z?jwt="+token, nil))
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/1?jwt="+token, nil))
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	// Get invalid chat messages
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/1/message", nil))
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/1/message?jwt="+token, nil))
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/chat/z/message?jwt="+token, nil))
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestCreateChatMessage(t *testing.T) {
	// Invalid chat id
	resp, _ := getApp().Test(httptest.NewRequest("POST", "/api/chat/-1/message", nil))
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// Find a chat id between a mod and group mods.  That means if we run this on the live system the potential
	// confusion is limited.
	chatid, _, token := GetChatFromModToGroup(t)

	// Logged out
	resp, _ = getApp().Test(httptest.NewRequest("POST", "/api/chat/"+fmt.Sprint(chatid)+"/message", nil))
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	// Undecodable payload.
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
	m := GetMessage(t)

	var payload chat.ChatMessageLovejunk

	payload.Refmsgid = &m.ID
	firstname := "Test"
	payload.Firstname = &firstname
	lastname := "User"
	payload.Lastname = &lastname

	// Without ljuserid
	s, _ := json2.Marshal(payload)
	b := bytes.NewBuffer(s)
	request := httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// Without partnerkey
	ljuserid := uint64(time.Now().UnixNano())
	payload.Ljuserid = &ljuserid
	s, _ = json2.Marshal(payload)
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// With invalid partnerkey
	payload.Partnerkey = "invalid"
	s, _ = json2.Marshal(payload)
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	// With valid partnerkey but no message
	payload.Partnerkey = os.Getenv("LOVEJUNK_PARTNER_KEY")
	s, _ = json2.Marshal(payload)
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// Valid
	payload.Message = "Test message"
	loc := "EH3 6SS"
	payload.PostcodePrefix = &loc
	s, _ = json2.Marshal(payload)
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var ret chat.ChatMessageLovejunkResponse
	json2.Unmarshal(rsp(resp), &ret)
	assert.Greater(t, ret.Id, (uint64)(0))
	assert.Greater(t, ret.Chatid, (uint64)(0))

	// Initial reply.
	payload.Message = "Test initial reply"
	payload.Initialreply = true
	offerid := uint64(123)
	payload.Offerid = &offerid
	s, _ = json2.Marshal(payload)
	b = bytes.NewBuffer(s)
	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &ret)
	assert.Greater(t, ret.Id, (uint64)(0))
	assert.Greater(t, ret.Chatid, (uint64)(0))

	// Find the user with ljuserid
	db := database.DBConn
	var user user2.User
	db.Where("ljuserid = ?", ljuserid).First(&user)
	assert.Equal(t, user.Firstname, firstname)
	assert.Equal(t, user.Lastname, lastname)

	// Ban the LJ user on the group.
	groupid := m.MessageGroups[0].Groupid
	db.Raw("INSERT INTO users_banned (userid, groupid) VALUES (?, ?)", user.ID, groupid)

	request = httptest.NewRequest("POST", "/api/chat/lovejunk", b)
	request.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(request)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}
