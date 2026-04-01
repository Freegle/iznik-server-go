package test

import (
	"bytes"
	json2 "encoding/json"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/group"
	flog "github.com/freegle/iznik-server-go/log"
	"github.com/freegle/iznik-server-go/log"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	// Get an invalid group - use very high ID guaranteed not to exist
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/group/999999999", nil))
	assert.Equal(t, 404, resp.StatusCode)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/group/notanint", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestGetGroup_NonFreegleType(t *testing.T) {
	// Groups with non-Freegle types should still be fetchable by ID.
	// V1 returns any group by ID regardless of type.
	prefix := uniquePrefix("nonfreegle")
	db := database.DBConn
	name := fmt.Sprintf("TestGroup_%s", prefix)

	result := db.Exec(fmt.Sprintf("INSERT INTO `groups` (nameshort, namefull, type, onhere, polyindex, lat, lng) "+
		"VALUES (?, ?, 'Other', 1, ST_GeomFromText('POINT(-3.1883 55.9533)', %d), 55.9533, -3.1883)", utils.SRID),
		name, "Non-Freegle "+prefix)
	require.NoError(t, result.Error)

	var groupID uint64
	db.Raw("SELECT id FROM `groups` WHERE nameshort = ? ORDER BY id DESC LIMIT 1", name).Scan(&groupID)
	require.Greater(t, groupID, uint64(0))

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groupID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var grp group.Group
	json2.Unmarshal(rsp(resp), &grp)
	assert.Equal(t, groupID, grp.ID)
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

func TestGetGroup_BatchFetch(t *testing.T) {
	prefix := uniquePrefix("grpbatch")
	group1ID := CreateTestGroup(t, prefix + "_g1")
	group2ID := CreateTestGroup(t, prefix + "_g2")
	group3ID := CreateTestGroup(t, prefix + "_g3")

	// Fetch multiple groups in one request.
	url := fmt.Sprintf("/api/group/%d,%d,%d", group1ID, group2ID, group3ID)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var groups []group.Group
	json2.Unmarshal(rsp(resp), &groups)
	assert.Equal(t, 3, len(groups), "Should return 3 groups")

	// Verify order matches request order.
	assert.Equal(t, group1ID, groups[0].ID)
	assert.Equal(t, group2ID, groups[1].ID)
	assert.Equal(t, group3ID, groups[2].ID)

	// Each group should have basic fields populated.
	for _, g := range groups {
		assert.Greater(t, len(g.Nameshort), 0)
		assert.Greater(t, len(g.Namedisplay), 0)
	}
}

func TestGetGroup_BatchFetchWithAuth(t *testing.T) {
	prefix := uniquePrefix("grpbatchauth")
	group1ID := CreateTestGroup(t, prefix + "_g1")
	group2ID := CreateTestGroup(t, prefix + "_g2")
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, group1ID, "Member")
	// Not a member of group2.
	_, token := CreateTestSession(t, userID)

	url := fmt.Sprintf("/api/group/%d,%d?jwt=%s", group1ID, group2ID, token)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var groups []group.Group
	json2.Unmarshal(rsp(resp), &groups)
	assert.Equal(t, 2, len(groups))
	assert.Equal(t, "Member", groups[0].Myrole)
	assert.Equal(t, "Non-member", groups[1].Myrole)
}

func TestGetGroup_BatchFetchSkipsInvalid(t *testing.T) {
	prefix := uniquePrefix("grpbatchinv")
	groupID := CreateTestGroup(t, prefix)

	// Mix valid and invalid IDs.
	url := fmt.Sprintf("/api/group/%d,999999999", groupID)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var groups []group.Group
	json2.Unmarshal(rsp(resp), &groups)
	assert.Equal(t, 1, len(groups), "Should return only the valid group")
	assert.Equal(t, groupID, groups[0].ID)
}

func TestGetGroup_SingleIdStillReturnsObject(t *testing.T) {
	// Single ID should still return a single object (not array) for backwards compat.
	prefix := uniquePrefix("grpsingle")
	groupID := CreateTestGroup(t, prefix)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groupID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var grp group.Group
	json2.Unmarshal(rsp(resp), &grp)
	assert.Equal(t, groupID, grp.ID)
}

func TestGetGroup_WelcomemailModtoolsOnly(t *testing.T) {
	prefix := uniquePrefix("grpwelcome")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Set a welcomemail on the group.
	db.Exec("UPDATE `groups` SET welcomemail = 'Welcome to our group!' WHERE id = ?", groupID)

	// Without modtools flag: welcomemail should be empty.
	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/group/%d?jwt=%s", groupID, modToken), nil))
	assert.Equal(t, 200, resp.StatusCode)
	var grp map[string]interface{}
	json2.Unmarshal(rsp(resp), &grp)
	assert.Empty(t, grp["welcomemail"], "welcomemail should be empty without modtools flag")

	// With modtools=true: welcomemail should be returned.
	resp, _ = getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/group/%d?modtools=true&jwt=%s", groupID, modToken), nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &grp)
	assert.Equal(t, "Welcome to our group!", grp["welcomemail"], "welcomemail should be returned with modtools=true")
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
	db.Exec("INSERT INTO logs (timestamp, groupid, type, subtype) VALUES (NOW(), ?, ?, ?)", groupID, flog.LOG_TYPE_MESSAGE, flog.LOG_SUBTYPE_AUTO_APPROVED)
	db.Exec("INSERT INTO logs (timestamp, groupid, type, subtype) VALUES (NOW(), ?, ?, ?)", groupID, flog.LOG_TYPE_MESSAGE, flog.LOG_SUBTYPE_APPROVED)
	db.Exec("INSERT INTO logs (timestamp, groupid, type, subtype) VALUES (NOW(), ?, ?, ?)", groupID, flog.LOG_TYPE_MESSAGE, flog.LOG_SUBTYPE_APPROVED)

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

func TestGetGroupReturnsBboxAndType(t *testing.T) {
	prefix := uniquePrefix("grpbbox")
	groupID := CreateTestGroup(t, prefix)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groupID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var grp group.Group
	json2.Unmarshal(rsp(resp), &grp)

	// Group should return bbox (computed from polyindex) and type.
	assert.Equal(t, "Freegle", grp.Type)
	assert.NotEmpty(t, grp.Bbox, "bbox should be populated from polyindex")
}

func TestGetGroupReturnsMicrovolunteering(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("grpmicro")
	groupID := CreateTestGroup(t, prefix)

	db.Exec("UPDATE `groups` SET microvolunteering = 1, microvolunteeringoptions = ? WHERE id = ?",
		`{"approvedmessages":true,"wordmatch":true}`, groupID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groupID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var raw map[string]interface{}
	json2.Unmarshal(rsp(resp), &raw)

	assert.Equal(t, float64(1), raw["microvolunteering"])
	opts, ok := raw["microvolunteeringoptions"].(map[string]interface{})
	assert.True(t, ok, "microvolunteeringoptions should be a JSON object")
	assert.Equal(t, true, opts["approvedmessages"])
	assert.Equal(t, true, opts["wordmatch"])
}

func TestPatchGroupNotLoggedIn(t *testing.T) {
	prefix := uniquePrefix("grpw_noauth")
	groupID := CreateTestGroup(t, prefix)

	body, _ := json.Marshal(map[string]interface{}{
		"id":      groupID,
		"tagline": "New tagline",
	})
	req := httptest.NewRequest("PATCH", "/api/group", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestPatchGroupNotModOrOwner(t *testing.T) {
	prefix := uniquePrefix("grpw_nomem")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	// Member only, not mod
	CreateTestMembership(t, userID, groupID, "Member")

	body, _ := json.Marshal(map[string]interface{}{
		"id":      groupID,
		"tagline": "New tagline",
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/group?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestPatchGroupNoID(t *testing.T) {
	prefix := uniquePrefix("grpw_noid")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"tagline": "New tagline",
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/group?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPatchGroupGroupNotFound(t *testing.T) {
	prefix := uniquePrefix("grpw_notfound")
	userID := CreateTestUser(t, prefix+"_user", "Admin")
	_, token := CreateTestSession(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"id":      99999999,
		"tagline": "New tagline",
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/group?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestPatchGroupConfirmAffiliation(t *testing.T) {
	prefix := uniquePrefix("grpw_affil")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_owner", "User")
	_, token := CreateTestSession(t, userID)

	// Need to be owner
	CreateTestMembership(t, userID, groupID, "Owner")

	body, _ := json.Marshal(map[string]interface{}{
		"id":                    groupID,
		"affiliationconfirmed": "2026-02-09T12:00:00Z",
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/group?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 10000)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify field was set
	var confirmed string
	var confirmedBy uint64
	db.Raw("SELECT COALESCE(affiliationconfirmed, ''), COALESCE(affiliationconfirmedby, 0) FROM `groups` WHERE id = ?", groupID).Row().Scan(&confirmed, &confirmedBy)
	assert.NotEmpty(t, confirmed)
	assert.Equal(t, userID, confirmedBy)
}

func TestPatchGroupModSettableFields(t *testing.T) {
	prefix := uniquePrefix("grpw_mod")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_mod", "User")
	_, token := CreateTestSession(t, userID)
	CreateTestMembership(t, userID, groupID, "Moderator")

	body, _ := json.Marshal(map[string]interface{}{
		"id":      groupID,
		"tagline": "Test Tagline",
		"region":  "London",
		"onhere":  1,
		"ontn":    1,
		"publish": 1,
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/group?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 10000)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify fields
	var tagline, region string
	var onhere, ontn, publish int
	db.Raw("SELECT COALESCE(tagline, ''), COALESCE(region, ''), onhere, ontn, publish FROM `groups` WHERE id = ?", groupID).Row().Scan(&tagline, &region, &onhere, &ontn, &publish)
	assert.Equal(t, "Test Tagline", tagline)
	assert.Equal(t, "London", region)
	assert.Equal(t, 1, onhere)
	assert.Equal(t, 1, ontn)
	assert.Equal(t, 1, publish)
}

func TestPatchGroupAdminOnlyFields(t *testing.T) {
	prefix := uniquePrefix("grpw_admin")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	// Admin user - not a member of the group
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	newShort := fmt.Sprintf("NS%d", groupID)
	body, _ := json.Marshal(map[string]interface{}{
		"id":              groupID,
		"lat":             51.5074,
		"lng":             -0.1278,
		"nameshort":       newShort,
		"licenserequired": 0,
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/group?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 10000)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify fields
	var lat, lng float64
	var nameshort string
	var licenserequired int
	db.Raw("SELECT COALESCE(lat, 0), COALESCE(lng, 0), nameshort, COALESCE(licenserequired, 0) FROM `groups` WHERE id = ?", groupID).Row().Scan(&lat, &lng, &nameshort, &licenserequired)
	assert.InDelta(t, 51.5074, lat, 0.001)
	assert.InDelta(t, -0.1278, lng, 0.001)
	assert.Equal(t, newShort, nameshort)
	assert.Equal(t, 0, licenserequired)
}

func TestPatchGroupAdminOnlyFieldsDeniedForMod(t *testing.T) {
	prefix := uniquePrefix("grpw_admindeny")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_modonly", "User")
	_, token := CreateTestSession(t, userID)
	CreateTestMembership(t, userID, groupID, "Moderator")

	// Get current lat
	var origLat float64
	db.Raw("SELECT COALESCE(lat, 0) FROM `groups` WHERE id = ?", groupID).Scan(&origLat)

	body, _ := json.Marshal(map[string]interface{}{
		"id":  groupID,
		"lat": 99.0,
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/group?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 10000)
	require.NoError(t, err)
	// Should succeed overall but lat should NOT be changed (admin-only field silently ignored)
	assert.Equal(t, 200, resp.StatusCode)

	var newLat float64
	db.Raw("SELECT COALESCE(lat, 0) FROM `groups` WHERE id = ?", groupID).Scan(&newLat)
	assert.Equal(t, origLat, newLat)
}

func TestPatchGroupPolygon(t *testing.T) {
	prefix := uniquePrefix("grpw_poly")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	adminID := CreateTestUser(t, prefix+"_support", "Support")
	_, token := CreateTestSession(t, adminID)

	poly := "POLYGON((-0.1 51.5, -0.1 51.6, 0.0 51.6, 0.0 51.5, -0.1 51.5))"
	body, _ := json.Marshal(map[string]interface{}{
		"id":   groupID,
		"poly": poly,
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/group?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 10000)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify polygon was set
	var storedPoly string
	db.Raw("SELECT COALESCE(poly, '') FROM `groups` WHERE id = ?", groupID).Scan(&storedPoly)
	assert.Equal(t, poly, storedPoly)
}

func TestPatchGroupSettingsAndRules(t *testing.T) {
	prefix := uniquePrefix("grpw_setrules")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_mod", "User")
	_, token := CreateTestSession(t, userID)
	CreateTestMembership(t, userID, groupID, "Moderator")

	settings := map[string]interface{}{
		"duplicates":   7,
		"reposts":      map[string]int{"offer": 3, "wanted": 7},
		"moderated":    0,
		"close":        map[string]int{"offer": 30, "wanted": 14},
	}
	rules := map[string]interface{}{
		"offer": "Rules for offers",
		"wanted": "Rules for wanted",
	}

	body, _ := json.Marshal(map[string]interface{}{
		"id":       groupID,
		"settings": settings,
		"rules":    rules,
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/group?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 10000)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify settings and rules stored
	var storedSettings, storedRules string
	db.Raw("SELECT COALESCE(settings, ''), COALESCE(rules, '') FROM `groups` WHERE id = ?", groupID).Row().Scan(&storedSettings, &storedRules)
	assert.NotEmpty(t, storedSettings)
	assert.NotEmpty(t, storedRules)
	assert.Contains(t, storedSettings, "duplicates")
	assert.Contains(t, storedRules, "offer")
}

func TestPatchGroupSettingsLogsAuditEntry(t *testing.T) {
	prefix := uniquePrefix("grpw_setlog")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_mod", "User")
	_, token := CreateTestSession(t, userID)
	CreateTestMembership(t, userID, groupID, "Moderator")

	// Clean up any pre-existing log entries for this group
	db.Exec("DELETE FROM logs WHERE groupid = ? AND type = ? AND subtype = ?", groupID, log.LOG_TYPE_GROUP, log.LOG_SUBTYPE_EDIT)

	settings := map[string]interface{}{
		"duplicates": 7,
	}
	body, _ := json.Marshal(map[string]interface{}{
		"id":       groupID,
		"settings": settings,
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/group?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 10000)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify that a log entry was created with type='Group', subtype='Edit'
	var logCount int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE type = ? AND subtype = ? AND groupid = ? AND byuser = ? AND text = 'Settings'",
		log.LOG_TYPE_GROUP, log.LOG_SUBTYPE_EDIT,
		groupID, userID).Scan(&logCount)
	assert.Equal(t, int64(1), logCount, "Settings change should create a Group/Edit log entry with text='Settings'")
}

func TestPatchGroupRulesLogsAuditEntry(t *testing.T) {
	prefix := uniquePrefix("grpw_rulelog")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_mod", "User")
	_, token := CreateTestSession(t, userID)
	CreateTestMembership(t, userID, groupID, "Moderator")

	// Clean up any pre-existing log entries for this group
	db.Exec("DELETE FROM logs WHERE groupid = ? AND type = ? AND subtype = ?", groupID, log.LOG_TYPE_GROUP, log.LOG_SUBTYPE_EDIT)

	rules := map[string]interface{}{
		"offer": "Rules for offers",
	}
	body, _ := json.Marshal(map[string]interface{}{
		"id":    groupID,
		"rules": rules,
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/group?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 10000)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify that a log entry was created with type='Group', subtype='Edit'
	var logCount int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE type = ? AND subtype = ? AND groupid = ? AND byuser = ? AND text = 'Rules'",
		log.LOG_TYPE_GROUP, log.LOG_SUBTYPE_EDIT, groupID, userID).Scan(&logCount)
	assert.Equal(t, int64(1), logCount, "Rules change should create a Group/Edit log entry with text='Rules'")
}

func TestPatchGroupSettingsAndRulesLogsBothEntries(t *testing.T) {
	prefix := uniquePrefix("grpw_bothlog")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_mod", "User")
	_, token := CreateTestSession(t, userID)
	CreateTestMembership(t, userID, groupID, "Moderator")

	// Clean up any pre-existing log entries for this group
	db.Exec("DELETE FROM logs WHERE groupid = ? AND type = ? AND subtype = ?", groupID, log.LOG_TYPE_GROUP, log.LOG_SUBTYPE_EDIT)

	body, _ := json.Marshal(map[string]interface{}{
		"id":       groupID,
		"settings": map[string]interface{}{"duplicates": 7},
		"rules":    map[string]interface{}{"offer": "New rules"},
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/group?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 10000)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Both a Settings and Rules log entry should exist
	var settingsLogCount, rulesLogCount int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE type = ? AND subtype = ? AND groupid = ? AND byuser = ? AND text = 'Settings'",
		log.LOG_TYPE_GROUP, log.LOG_SUBTYPE_EDIT,
		groupID, userID).Scan(&settingsLogCount)
	db.Raw("SELECT COUNT(*) FROM logs WHERE type = ? AND subtype = ? AND groupid = ? AND byuser = ? AND text = 'Rules'",
		log.LOG_TYPE_GROUP, log.LOG_SUBTYPE_EDIT, groupID, userID).Scan(&rulesLogCount)
	assert.Equal(t, int64(1), settingsLogCount, "Should have Settings log entry")
	assert.Equal(t, int64(1), rulesLogCount, "Should have Rules log entry")
}

func TestPatchGroupInvalidPolyReturnsError(t *testing.T) {
	prefix := uniquePrefix("grpw_badpoly")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	// Self-intersecting polygon (bowtie shape) — invalid geometry
	invalidPoly := "POLYGON((0 0, 10 10, 10 0, 0 10, 0 0))"
	body, _ := json.Marshal(map[string]interface{}{
		"id":   groupID,
		"poly": invalidPoly,
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/group?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 10000)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode, "Invalid poly geometry should return 400")

	// Verify the poly was NOT stored
	var storedPoly string
	db.Raw("SELECT COALESCE(poly, '') FROM `groups` WHERE id = ?", groupID).Scan(&storedPoly)
	assert.Empty(t, storedPoly, "Invalid poly should not be stored in the database")
}

func TestPatchGroupInvalidPolyofficialReturnsError(t *testing.T) {
	prefix := uniquePrefix("grpw_badpolyoff")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	// Self-intersecting polygon (bowtie shape) — invalid geometry
	invalidPoly := "POLYGON((0 0, 10 10, 10 0, 0 10, 0 0))"
	body, _ := json.Marshal(map[string]interface{}{
		"id":           groupID,
		"polyofficial": invalidPoly,
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/group?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 10000)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode, "Invalid polyofficial geometry should return 400")

	// Verify the polyofficial was NOT stored
	var storedPoly string
	db.Raw("SELECT COALESCE(polyofficial, '') FROM `groups` WHERE id = ?", groupID).Scan(&storedPoly)
	assert.Empty(t, storedPoly, "Invalid polyofficial should not be stored in the database")
}

func TestPatchGroupInvalidPolyNotWKT(t *testing.T) {
	prefix := uniquePrefix("grpw_notwkt")
	groupID := CreateTestGroup(t, prefix)

	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	// Completely invalid WKT — not parseable at all
	body, _ := json.Marshal(map[string]interface{}{
		"id":   groupID,
		"poly": "this is not a polygon",
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/group?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 10000)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode, "Non-WKT poly should return 400")
}

func TestPatchGroupValidPolyStillAccepted(t *testing.T) {
	prefix := uniquePrefix("grpw_goodpoly")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	// Valid polygon
	validPoly := "POLYGON((-0.1 51.5, -0.1 51.6, 0.0 51.6, 0.0 51.5, -0.1 51.5))"
	body, _ := json.Marshal(map[string]interface{}{
		"id":   groupID,
		"poly": validPoly,
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/group?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 10000)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode, "Valid poly should still be accepted")

	// Verify the poly was stored
	var storedPoly string
	db.Raw("SELECT COALESCE(poly, '') FROM `groups` WHERE id = ?", groupID).Scan(&storedPoly)
	assert.Equal(t, validPoly, storedPoly)
}

func TestPatchGroupSupportCanPatchWithoutMembership(t *testing.T) {
	prefix := uniquePrefix("grpw_support")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	// Support user, no membership in the group
	supportID := CreateTestUser(t, prefix+"_support", "Support")
	_, token := CreateTestSession(t, supportID)

	body, _ := json.Marshal(map[string]interface{}{
		"id":      groupID,
		"tagline": "Support set this",
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/group?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 10000)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var tagline string
	db.Raw("SELECT COALESCE(tagline, '') FROM `groups` WHERE id = ?", groupID).Scan(&tagline)
	assert.Equal(t, "Support set this", tagline)
}
