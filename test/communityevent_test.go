package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/communityevent"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestCommunityEvent(t *testing.T) {
	// Create test data for this test
	prefix := uniquePrefix("event")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	eventID := CreateTestCommunityEvent(t, userID, groupID)

	// Get non-existent event - should return 404
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/communityevents/1", nil))
	assert.Equal(t, 404, resp.StatusCode)

	// Get the event we created
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/communityevent/"+fmt.Sprint(eventID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var event communityevent.CommunityEvent
	json2.Unmarshal(rsp(resp), &event)
	assert.Greater(t, event.ID, uint64(0))
	assert.Greater(t, len(event.Title), 0)
	assert.Greater(t, len(event.Dates), 0)

	// List events requires auth
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/communityevent", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Create a full test user with all relationships for authenticated requests
	_, token := CreateFullTestUser(t, prefix+"_auth")
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/communityevent?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.Greater(t, len(ids), 0)

	// Get events by group
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/communityevent/group/"+fmt.Sprint(groupID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &ids)
	assert.Greater(t, len(ids), 0)
}
