package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/chat"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	url2 "net/url"
	"testing"
	"time"
)

func TestListChats(t *testing.T) {
	_, token := GetUserWithToken(t)

	// Logged out
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/chat", nil))
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
