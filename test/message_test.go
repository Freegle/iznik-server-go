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

	// Should show as having posted a message, i.e. an offer or a wanted.
	assert.Greater(t, u.Info.Offers+u.Info.Wanteds, uint64(0))

	// Shouldn't see memberships.
	assert.Equal(t, len(u.Memberships), 0)

	// Get invalid message/user.
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/message/1", nil))
	assert.Equal(t, 404, resp.StatusCode)
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/user/1", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestBounds(t *testing.T) {
	app := fiber.New()
	database.InitDatabase()
	router.SetupRoutes(app)

	// Get within the bounds set up on the test group.
	resp, _ := app.Test(httptest.NewRequest("GET", "/api/message/inbounds?swlat=55&swlng=-3.5&nelat=56&nelng=-3", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msgs []message.MessagesSpatial
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Greater(t, len(msgs), 0)

	// Get outside.
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/message/inbounds?swlng=55&swlat=-3.5&nelng=56&nelat=-3", nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Equal(t, len(msgs), 0)
}

func TestMyGroups(t *testing.T) {
	app := fiber.New()
	database.InitDatabase()
	router.SetupRoutes(app)

	// Get logged out.
	resp, _ := app.Test(httptest.NewRequest("GET", "/api/message/mygroups", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Should be able to fetch messages in our groups.
	_, token := GetUserWithToken()

	resp, _ = app.Test(httptest.NewRequest("GET", "/api/message/mygroups?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msgs []message.MessagesSpatial
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Greater(t, len(msgs), 0)
}
