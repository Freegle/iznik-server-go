package test

import (
	json2 "encoding/json"
	"github.com/freegle/iznik-server-go/chat"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/router"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestListChats(t *testing.T) {
	app := fiber.New()
	database.InitDatabase()
	router.SetupRoutes(app)

	_, token := GetUserWithToken(t)

	// Logged out
	resp, _ := app.Test(httptest.NewRequest("GET", "/api/chat", nil))
	assert.Equal(t, 401, resp.StatusCode)

	resp, _ = app.Test(httptest.NewRequest("GET", "/api/chat?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var chats []chat.ChatRoomListEntry
	json2.Unmarshal(rsp(resp), &chats)
	//fmt.Printf("Chats %+v", chats)

	// Should find a chat with a name.
	assert.Greater(t, len(chats), 0)
	assert.Greater(t, len(chats[0].Name), 0)
	assert.Greater(t, len(chats[0].Icon), 0)
	assert.Greater(t, len(chats[0].Snippet), 0)
}
