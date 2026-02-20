package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/communityevent"
	"github.com/freegle/iznik-server-go/database"
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

func TestCommunityEvent_InvalidID(t *testing.T) {
	// Non-integer ID should return 404
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/communityevent/notanint", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestCommunityEvent_InvalidGroupID(t *testing.T) {
	// Non-existent group should return empty array
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/communityevent/group/999999999", nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestCommunityEvent_V2Path(t *testing.T) {
	// Verify v2 paths work
	prefix := uniquePrefix("eventv2")
	_, token := CreateFullTestUser(t, prefix)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/communityevent?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestCommunityEvent_PendingList(t *testing.T) {
	prefix := uniquePrefix("eventpend")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	// Create a regular user who creates a pending event
	creatorID := CreateTestUser(t, prefix+"_creator", "User")
	CreateTestMembership(t, creatorID, groupID, "Member")

	// Create a pending event directly (not using helper which creates non-pending)
	db.Exec("INSERT INTO communityevents (userid, title, description, pending, deleted) VALUES (?, 'Pending Event', 'Pending description', 1, 0)", creatorID)
	var pendingID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND pending = 1 ORDER BY id DESC LIMIT 1", creatorID).Scan(&pendingID)
	assert.Greater(t, pendingID, uint64(0))
	db.Exec("INSERT INTO communityevents_groups (eventid, groupid) VALUES (?, ?)", pendingID, groupID)

	// Create a moderator user for the same group
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Moderator should see pending events
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/communityevent?pending=true&jwt="+modToken, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.Contains(t, ids, pendingID)

	// Regular member should NOT see pending events (they're not a mod)
	memberID := CreateTestUser(t, prefix+"_member", "User")
	CreateTestMembership(t, memberID, groupID, "Member")
	_, memberToken := CreateTestSession(t, memberID)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/communityevent?pending=true&jwt="+memberToken, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var memberIds []uint64
	json2.Unmarshal(rsp(resp), &memberIds)
	assert.NotContains(t, memberIds, pendingID)

	// Verify single fetch returns pending field
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/communityevent/"+fmt.Sprint(pendingID), nil))
	assert.Equal(t, 200, resp.StatusCode)
	var event communityevent.CommunityEvent
	json2.Unmarshal(rsp(resp), &event)
	assert.True(t, event.Pending)
}

func TestCommunityEvent_PendingListAdmin(t *testing.T) {
	prefix := uniquePrefix("eventadm")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	// Create a pending event
	creatorID := CreateTestUser(t, prefix+"_creator", "User")
	CreateTestMembership(t, creatorID, groupID, "Member")
	db.Exec("INSERT INTO communityevents (userid, title, description, pending, deleted) VALUES (?, 'Admin Pending Event', 'Admin test', 1, 0)", creatorID)
	var pendingID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND pending = 1 ORDER BY id DESC LIMIT 1", creatorID).Scan(&pendingID)
	db.Exec("INSERT INTO communityevents_groups (eventid, groupid) VALUES (?, ?)", pendingID, groupID)

	// Admin should see all pending events
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/communityevent?pending=true&jwt="+adminToken, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.Contains(t, ids, pendingID)
}
