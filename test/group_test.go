package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
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

func TestListGroupsWithSupport(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("grpsupp")

	// Create an Admin user so we can pass the systemrole check.
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	// Create a group and set support-specific fields on it.
	groupID := CreateTestGroup(t, prefix)
	db.Exec("UPDATE `groups` SET founded = '2020-01-15', lastmoderated = NOW(), lastmodactive = NOW(), "+
		"lastautoapprove = NOW(), activeownercount = 2, activemodcount = 5, "+
		"backupmodsactive = 1, backupownersactive = 1, "+
		"affiliationconfirmed = '2025-06-01', affiliationconfirmedby = ?, "+
		"publish = 1, onhere = 1 WHERE id = ?", adminID, groupID)

	// Insert a log entry for autoapproved and approved so the counts are non-zero.
	db.Exec("INSERT INTO logs (timestamp, groupid, type, subtype) VALUES (NOW(), ?, 'Message', 'Autoapproved')", groupID)
	db.Exec("INSERT INTO logs (timestamp, groupid, type, subtype) VALUES (NOW(), ?, 'Message', 'Approved')", groupID)
	db.Exec("INSERT INTO logs (timestamp, groupid, type, subtype) VALUES (NOW(), ?, 'Message', 'Approved')", groupID)

	// Request with support=true and admin JWT.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group?support=true&jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Decode into raw JSON to check for support-specific keys.
	var rawGroups []map[string]interface{}
	json2.Unmarshal(rsp(resp), &rawGroups)
	assert.Greater(t, len(rawGroups), 0)

	// Find the group we created.
	var found map[string]interface{}
	for _, g := range rawGroups {
		if uint64(g["id"].(float64)) == groupID {
			found = g
			break
		}
	}
	assert.NotNil(t, found, "Created group should appear in support listing")

	// Verify support-only fields are present.
	assert.Contains(t, found, "founded")
	assert.Contains(t, found, "lastmoderated")
	assert.Contains(t, found, "lastmodactive")
	assert.Contains(t, found, "lastautoapprove")
	assert.Contains(t, found, "activeownercount")
	assert.Contains(t, found, "activemodcount")
	assert.Contains(t, found, "backupmodsactive")
	assert.Contains(t, found, "backupownersactive")
	assert.Contains(t, found, "affiliationconfirmed")
	assert.Contains(t, found, "affiliationconfirmedby")
	assert.Contains(t, found, "recentautoapproves")
	assert.Contains(t, found, "recentmanualapproves")
	assert.Contains(t, found, "recentautoapprovespercent")

	// Verify the approve counts are sensible.
	assert.Equal(t, float64(1), found["recentautoapproves"])
	// Manual = total Approved (2) - auto (1) = 1
	assert.Equal(t, float64(1), found["recentmanualapproves"])
	// Percent = 100*1/(1+1) = 50
	assert.Equal(t, float64(50), found["recentautoapprovespercent"])

	// Verify that activeownercount is returned correctly.
	assert.Equal(t, float64(2), found["activeownercount"])
	assert.Equal(t, float64(5), found["activemodcount"])

	// Now verify that support=true as a regular User does NOT return support fields.
	regularID := CreateTestUser(t, prefix+"_regular", "User")
	_, regularToken := CreateTestSession(t, regularID)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/group?support=true&jwt="+regularToken, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var regularGroups []map[string]interface{}
	json2.Unmarshal(rsp(resp), &regularGroups)
	assert.Greater(t, len(regularGroups), 0)

	// Find any group and verify support fields are NOT present.
	if len(regularGroups) > 0 {
		assert.NotContains(t, regularGroups[0], "founded")
		assert.NotContains(t, regularGroups[0], "lastmoderated")
		assert.NotContains(t, regularGroups[0], "activeownercount")
		assert.NotContains(t, regularGroups[0], "recentautoapproves")
	}

	// Cleanup test log entries.
	db.Exec("DELETE FROM logs WHERE groupid = ? AND type = 'Message' AND subtype IN ('Autoapproved', 'Approved')", groupID)
}

func TestGetGroupWithShowmods(t *testing.T) {
	prefix := uniquePrefix("grpshowmods")

	groupID := CreateTestGroup(t, prefix)

	// Create a moderator with showmod setting defaulting to true.
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")

	// Request with showmods=true.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groupID)+"?showmods=true", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var grp group.Group
	json2.Unmarshal(rsp(resp), &grp)
	assert.Equal(t, grp.ID, groupID)

	// Should have at least one volunteer/showmod.
	assert.Greater(t, len(grp.GroupVolunteers), 0)

	// Verify the mod appears in the showmods list.
	foundMod := false
	for _, v := range grp.GroupVolunteers {
		if v.ID == modID {
			foundMod = true
			break
		}
	}
	assert.True(t, foundMod, "Moderator should appear in showmods")

	// Without showmods=true (default behavior) should also include showmods for backward compatibility.
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groupID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var grpDefault group.Group
	json2.Unmarshal(rsp(resp), &grpDefault)
	assert.Greater(t, len(grpDefault.GroupVolunteers), 0)
}

func TestGetGroupWithSponsors(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("grpsponsor")

	groupID := CreateTestGroup(t, prefix)

	// Insert a visible, currently-active sponsor.
	db.Exec("INSERT INTO groups_sponsorship (groupid, name, linkurl, imageurl, tagline, startdate, enddate, contactname, contactemail, amount, visible) "+
		"VALUES (?, 'Test Sponsor', 'https://example.com', 'https://example.com/img.png', 'Great sponsor', "+
		"DATE_SUB(NOW(), INTERVAL 30 DAY), DATE_ADD(NOW(), INTERVAL 30 DAY), 'Contact', 'contact@example.com', 100, 1)", groupID)

	// Insert an expired sponsor (should NOT appear with sponsors=true).
	db.Exec("INSERT INTO groups_sponsorship (groupid, name, linkurl, imageurl, tagline, startdate, enddate, contactname, contactemail, amount, visible) "+
		"VALUES (?, 'Expired Sponsor', 'https://expired.com', 'https://expired.com/img.png', 'Old sponsor', "+
		"DATE_SUB(NOW(), INTERVAL 90 DAY), DATE_SUB(NOW(), INTERVAL 1 DAY), 'Old Contact', 'old@example.com', 50, 1)", groupID)

	// Insert an invisible sponsor (should NOT appear with sponsors=true).
	db.Exec("INSERT INTO groups_sponsorship (groupid, name, linkurl, imageurl, tagline, startdate, enddate, contactname, contactemail, amount, visible) "+
		"VALUES (?, 'Hidden Sponsor', 'https://hidden.com', 'https://hidden.com/img.png', 'Secret sponsor', "+
		"DATE_SUB(NOW(), INTERVAL 30 DAY), DATE_ADD(NOW(), INTERVAL 30 DAY), 'Hidden', 'hidden@example.com', 200, 0)", groupID)

	// Request with sponsors=true (filtered by date/visible).
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groupID)+"?sponsors=true", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var grp group.Group
	json2.Unmarshal(rsp(resp), &grp)
	assert.Equal(t, grp.ID, groupID)

	// Should have exactly 1 active+visible sponsor.
	assert.Equal(t, 1, len(grp.GroupSponsors))
	if len(grp.GroupSponsors) > 0 {
		assert.Equal(t, "Test Sponsor", grp.GroupSponsors[0].Name)
		assert.Equal(t, "https://example.com", grp.GroupSponsors[0].Linkurl)
		assert.Equal(t, "Great sponsor", grp.GroupSponsors[0].Tagline)
	}

	// Without sponsors=true (default behavior), all sponsors should be loaded (GORM Preload, no filtering).
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groupID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var grpDefault group.Group
	json2.Unmarshal(rsp(resp), &grpDefault)

	// Default (unfiltered) should include all 3 sponsors for this group.
	assert.Equal(t, 3, len(grpDefault.GroupSponsors))

	// Cleanup.
	db.Exec("DELETE FROM groups_sponsorship WHERE groupid = ?", groupID)
}

func TestGetGroupWithPolygon(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("grppoly")

	groupID := CreateTestGroup(t, prefix)

	// Set polygon data on the group.
	poly := "POLYGON((-0.1 51.5, -0.1 51.6, 0.0 51.6, 0.0 51.5, -0.1 51.5))"
	db.Exec("UPDATE `groups` SET poly = ? WHERE id = ?", poly, groupID)

	// Without polygon param - should not include poly fields.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groupID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var grpNoPoly group.Group
	json2.Unmarshal(rsp(resp), &grpNoPoly)
	assert.Nil(t, grpNoPoly.Poly)

	// With polygon=true - should include poly fields.
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groupID)+"?polygon=true", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var grpPoly group.Group
	json2.Unmarshal(rsp(resp), &grpPoly)
	assert.NotNil(t, grpPoly.Poly)
	assert.Equal(t, poly, *grpPoly.Poly)
}
