package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/group"
	"github.com/freegle/iznik-server-go/message"
	"github.com/freegle/iznik-server-go/router"
	user2 "github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestMessages(t *testing.T) {
	app := fiber.New()
	database.InitDatabase()
	router.SetupRoutes(app)

	// Get a group id.
	resp, _ := app.Test(httptest.NewRequest("GET", "/api/group", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var groups []group.GroupEntry
	json2.Unmarshal(rsp(resp), &groups)

	gid := groups[0].ID

	// Get messages on the group.
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(gid)+"/message", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var mids []uint64
	json2.Unmarshal(rsp(resp), &mids)

	assert.Greater(t, mids[0], uint64(0))
	mid := mids[0]

	// Get the message
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/message/"+fmt.Sprint(mid), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var message message.Message
	json2.Unmarshal(rsp(resp), &message)
	assert.Equal(t, mid, message.ID)

	uid := message.FromuserObj.ID
	assert.Greater(t, uid, uint64(0))

	// Get the user.
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(uid), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var u user2.User
	json2.Unmarshal(rsp(resp), &u)
	assert.Equal(t, uid, u.ID)
	assert.Greater(t, len(u.Displayname), 0)

	// Shouldn't see memberships.
	assert.Equal(t, len(u.Memberships), 0)
}
