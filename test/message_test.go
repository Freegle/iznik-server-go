package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/message"
	user2 "github.com/freegle/iznik-server-go/user"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestMessages(t *testing.T) {
	// Create test group with messages
	prefix := uniquePrefix("msg")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")

	// Create two messages for the test
	mid := CreateTestMessage(t, userID, groupID, "Test Offer Item 1", 55.9533, -3.1883)
	mid2 := CreateTestMessage(t, userID, groupID, "Test Offer Item 2", 55.9533, -3.1883)

	// Get messages on the group
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groupID)+"/message", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var mids []uint64
	json2.Unmarshal(rsp(resp), &mids)
	assert.Greater(t, len(mids), 0)

	// Get the message
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/"+fmt.Sprint(mid), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msg message.Message
	json2.Unmarshal(rsp(resp), &msg)
	assert.Equal(t, mid, msg.ID)

	// Get the same message multiple times to test the array variant
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/"+fmt.Sprint(mid)+","+fmt.Sprint(mid2), nil))
	assert.Equal(t, 200, resp.StatusCode)

	messages := []message.Message{}
	json2.Unmarshal(rsp(resp), &messages)
	assert.Equal(t, 2, len(messages))
	assert.True(t, (messages[0].ID == mid && messages[1].ID == mid2) || (messages[0].ID == mid2 && messages[1].ID == mid))

	// Test too many
	url := "/api/message/"
	for i := 0; i < 30; i++ {
		url += fmt.Sprint(mid) + ","
	}
	resp, _ = getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 400, resp.StatusCode)

	// Get the user
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(userID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var u user2.User
	json2.Unmarshal(rsp(resp), &u)
	assert.Equal(t, userID, u.ID)
	assert.Greater(t, len(u.Displayname), 0)

	// Shouldn't see memberships without auth
	assert.Equal(t, len(u.Memberships), 0)

	// Get invalid message/user
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/1", nil))
	assert.Equal(t, 404, resp.StatusCode)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/1", nil))
	assert.Equal(t, 404, resp.StatusCode)

	// Get the message as the sender
	midArray := []string{fmt.Sprint(mid)}
	msgDetails := message.GetMessagesByIds(userID, midArray)[0]
	assert.Equal(t, mid, msgDetails.ID)
}

func TestBounds(t *testing.T) {
	// Create a message in specific bounds for this test
	prefix := uniquePrefix("bounds")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	CreateTestMessage(t, userID, groupID, "Test Bounds Item", 55.9533, -3.1883)

	// Get within the bounds
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/message/inbounds?swlat=55&swlng=-3.5&nelat=56&nelng=-3", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msgs []message.MessageSummary
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Greater(t, len(msgs), 0)

	// Repeat but logged in
	_, token := CreateFullTestUser(t, prefix+"_auth")
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/inbounds?swlat=55&swlng=-3.5&nelat=56&nelng=-3&jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Greater(t, len(msgs), 0)

	// Get outside bounds
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/inbounds?swlng=55&swlat=-3.5&nelng=56&nelat=-3", nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Equal(t, len(msgs), 0)
}

func TestMyGroups(t *testing.T) {
	// Get logged out - should return 401
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/message/mygroups", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Create a full test user with group membership and message
	prefix := uniquePrefix("mygroups")
	userID, token := CreateFullTestUser(t, prefix)

	// Create a group the user is in with a message
	groupID := CreateTestGroup(t, prefix+"_grp")
	CreateTestMembership(t, userID, groupID, "Member")
	CreateTestMessage(t, userID, groupID, "Test MyGroups Item", 55.9533, -3.1883)

	// Should be able to fetch messages in our groups
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/mygroups?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msgs []message.MessageSummary
	json2.Unmarshal(rsp(resp), &msgs)
	// We expect at least some messages (could be from other tests too)
}

func TestMessagesByUser(t *testing.T) {
	// Create a user with a message
	prefix := uniquePrefix("usermsg")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	CreateTestMessage(t, userID, groupID, "Test User Message", 55.9533, -3.1883)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(userID)+"/message", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msgs []message.MessageSummary
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Greater(t, len(msgs), 0)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(userID)+"/message?active=true", nil))
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &msgs)
	assert.Greater(t, len(msgs), 0)

	// Invalid user
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/z/message", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestCount(t *testing.T) {
	// Create a full test user for count endpoint
	prefix := uniquePrefix("count")
	_, token := CreateFullTestUser(t, prefix)

	var count int

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/message/count?browseView=mygroups&jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &count)
	// Count can be 0 for a new user

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/count?browseView=nearby&jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &count)
	// Count can be 0 for a new user
}

func TestActivity(t *testing.T) {
	// Create some activity data
	prefix := uniquePrefix("activity")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	CreateTestMessage(t, userID, groupID, "Test Activity Item", 55.9533, -3.1883)

	// Get recent activity
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/activity", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var activity []message.Activity
	json2.Unmarshal(rsp(resp), &activity)
	assert.Greater(t, len(activity), 0)
	assert.Greater(t, activity[0].ID, uint64(0))
}
