package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	volunteering2 "github.com/freegle/iznik-server-go/volunteering"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestVolunteering(t *testing.T) {
	// Create test data for this test
	prefix := uniquePrefix("vol")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	volunteeringID := CreateTestVolunteering(t, userID, groupID)

	// Get non-existent volunteering - should return 404
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/volunteering/1", nil))
	assert.Equal(t, 404, resp.StatusCode)

	// Get the volunteering we created
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/volunteering/"+fmt.Sprint(volunteeringID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var volunteering volunteering2.Volunteering
	json2.Unmarshal(rsp(resp), &volunteering)
	assert.Greater(t, volunteering.ID, uint64(0))
	assert.Greater(t, len(volunteering.Title), 0)
	assert.Greater(t, len(volunteering.Dates), 0)

	// List volunteering requires auth
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/volunteering", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Create a full test user with all relationships for authenticated requests
	_, token := CreateFullTestUser(t, prefix+"_auth")
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/volunteering?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.Greater(t, len(ids), 0)

	// Get volunteering by group
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/volunteering/group/"+fmt.Sprint(groupID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &ids)
	assert.Greater(t, len(ids), 0)
}

func TestVolunteering_InvalidID(t *testing.T) {
	// Non-integer ID should return 404
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/volunteering/notanint", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestVolunteering_InvalidGroupID(t *testing.T) {
	// Non-existent group should return empty array
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/volunteering/group/999999999", nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestVolunteering_V2Path(t *testing.T) {
	// Verify v2 paths work
	prefix := uniquePrefix("volv2")
	_, token := CreateFullTestUser(t, prefix)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/volunteering?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestVolunteering_PendingList(t *testing.T) {
	prefix := uniquePrefix("volpend")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	// Create a regular user who creates a pending volunteering
	creatorID := CreateTestUser(t, prefix+"_creator", "User")
	CreateTestMembership(t, creatorID, groupID, "Member")

	db.Exec("INSERT INTO volunteering (userid, title, description, pending, deleted, expired) VALUES (?, 'Pending Vol', 'Pending desc', 1, 0, 0)", creatorID)
	var pendingID uint64
	db.Raw("SELECT id FROM volunteering WHERE userid = ? AND pending = 1 ORDER BY id DESC LIMIT 1", creatorID).Scan(&pendingID)
	assert.Greater(t, pendingID, uint64(0))
	db.Exec("INSERT INTO volunteering_groups (volunteeringid, groupid) VALUES (?, ?)", pendingID, groupID)

	// Create a moderator user for the same group
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Moderator should see pending volunteering
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/volunteering?pending=true&jwt="+modToken, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.Contains(t, ids, pendingID)

	// Regular member should NOT see pending volunteering
	memberID := CreateTestUser(t, prefix+"_member", "User")
	CreateTestMembership(t, memberID, groupID, "Member")
	_, memberToken := CreateTestSession(t, memberID)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/volunteering?pending=true&jwt="+memberToken, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var memberIds []uint64
	json2.Unmarshal(rsp(resp), &memberIds)
	assert.NotContains(t, memberIds, pendingID)

	// Verify single fetch returns pending field
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/volunteering/"+fmt.Sprint(pendingID), nil))
	assert.Equal(t, 200, resp.StatusCode)
	var vol volunteering2.Volunteering
	json2.Unmarshal(rsp(resp), &vol)
	assert.True(t, vol.Pending)
}

func TestVolunteering_PendingListAdmin(t *testing.T) {
	prefix := uniquePrefix("voladm")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	// Create a pending volunteering
	creatorID := CreateTestUser(t, prefix+"_creator", "User")
	CreateTestMembership(t, creatorID, groupID, "Member")
	db.Exec("INSERT INTO volunteering (userid, title, description, pending, deleted, expired) VALUES (?, 'Admin Pending Vol', 'Admin test', 1, 0, 0)", creatorID)
	var pendingID uint64
	db.Raw("SELECT id FROM volunteering WHERE userid = ? AND pending = 1 ORDER BY id DESC LIMIT 1", creatorID).Scan(&pendingID)
	db.Exec("INSERT INTO volunteering_groups (volunteeringid, groupid) VALUES (?, ?)", pendingID, groupID)

	// Admin should see all pending volunteering
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/volunteering?pending=true&jwt="+adminToken, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.Contains(t, ids, pendingID)
}
