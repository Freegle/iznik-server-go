package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/group"
	"github.com/freegle/iznik-server-go/message"
	user2 "github.com/freegle/iznik-server-go/user"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestMessages(t *testing.T) {
	// Get a group id.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var groups []group.GroupEntry
	json2.Unmarshal(rsp(resp), &groups)

	gid := groups[0].ID

	// Get messages on the group.
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(gid)+"/message", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var mids []uint64
	json2.Unmarshal(rsp(resp), &mids)

	assert.Greater(t, mids[0], uint64(1))
	mid := mids[0]
	mid2 := mids[1]

	// Get the message
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/"+fmt.Sprint(mid), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msg message.Message
	json2.Unmarshal(rsp(resp), &msg)
	assert.Equal(t, mid, msg.ID)

	uid := msg.Fromuser
	assert.Greater(t, uid, uint64(0))

	// Get the same message multiple times to test the array variant.
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/"+fmt.Sprint(mid)+","+fmt.Sprint(mid2), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var messages []message.Message
	json2.Unmarshal(rsp(resp), &messages)
	assert.Equal(t, 2, len(messages))
	assert.True(t, (messages[0].ID == mid && messages[1].ID == mid2) || (messages[0].ID == mid2 && messages[1].ID == mid))

	// Test too many.
	url := "/api/message/"
	// add mid 30 times
	for i := 0; i < 30; i++ {
		url += fmt.Sprint(mid) + ","
	}
	resp, _ = getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 400, resp.StatusCode)

	// Get the user.
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(uid), nil))
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
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/1", nil))
	assert.Equal(t, 404, resp.StatusCode)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/1", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestBounds(t *testing.T) {
	// Get within the bounds set up on the test group.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/message/inbounds?swlat=55&swlng=-3.5&nelat=56&nelng=-3", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msgs []message.MessageSummary
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Greater(t, len(msgs), 0)

	// Repeat but logged in.
	_, token := GetUserWithToken(t)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/inbounds?swlat=55&swlng=-3.5&nelat=56&nelng=-3?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Greater(t, len(msgs), 0)

	// Get outside.
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/inbounds?swlng=55&swlat=-3.5&nelng=56&nelat=-3", nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Equal(t, len(msgs), 0)
}

func TestMyGroups(t *testing.T) {
	// Get logged out.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/message/mygroups", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Should be able to fetch messages in our groups.
	_, token := GetUserWithToken(t)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/mygroups?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msgs []message.MessageSummary
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Greater(t, len(msgs), 0)
}

func TestMessagesByUser(t *testing.T) {
	// Find a user with a message.
	uid := GetUserWithMessage(t)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(uid)+"/message", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msgs []message.MessageSummary
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Greater(t, len(msgs), 0)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(uid)+"/message?active=true", nil))
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &msgs)
	assert.Greater(t, len(msgs), 0)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/z/message", nil))
	assert.Equal(t, 404, resp.StatusCode)
}
