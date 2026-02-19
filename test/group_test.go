package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/group"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestListGroups(t *testing.T) {
	// Create test groups with moderators
	prefix1 := uniquePrefix("group1")
	prefix2 := uniquePrefix("group2")

	groupID1 := CreateTestGroup(t, prefix1)
	groupID2 := CreateTestGroup(t, prefix2)

	// Create a moderator for the first group (GroupVolunteers returns Moderator/Owner members)
	userID := CreateTestUser(t, prefix1, "Moderator")
	CreateTestMembership(t, userID, groupID1, "Moderator")

	// List groups
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var groups []group.GroupEntry
	json2.Unmarshal(rsp(resp), &groups)

	assert.Greater(t, len(groups), 1)
	assert.Greater(t, groups[0].ID, uint64(0))
	assert.Greater(t, len(groups[0].Nameshort), 0)

	// Get the first group we created
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groupID1), nil))
	assert.Equal(t, 200, resp.StatusCode)
	var grp group.Group
	json2.Unmarshal(rsp(resp), &grp)

	assert.Greater(t, grp.ID, uint64(0))
	assert.Equal(t, grp.Showjoin, 0)

	// Check that it has volunteers
	assert.Greater(t, len(grp.GroupVolunteers), 0)

	// Get the second group
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groupID2), nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &grp)

	assert.Equal(t, grp.ID, groupID2)

	// Get an invalid group
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/group/1", nil))
	assert.Equal(t, 404, resp.StatusCode)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/group/notanint", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestListGroups_WithAuth(t *testing.T) {
	// Auth should not change group listing behavior (public endpoint)
	prefix := uniquePrefix("grpauth")
	_, token := CreateFullTestUser(t, prefix)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var groups []group.GroupEntry
	json2.Unmarshal(rsp(resp), &groups)
	assert.Greater(t, len(groups), 0)
}

func TestGetGroup_WithAuth(t *testing.T) {
	// Auth may include additional data for own group (e.g. showjoin)
	prefix := uniquePrefix("grpgetauth")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groupID)+"?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var grp group.Group
	json2.Unmarshal(rsp(resp), &grp)
	assert.Equal(t, grp.ID, groupID)
}

func TestListGroups_V2Path(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/group", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var groups []group.GroupEntry
	json2.Unmarshal(rsp(resp), &groups)
	assert.Greater(t, len(groups), 0)
}
