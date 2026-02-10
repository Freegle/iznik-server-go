package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

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
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
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
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
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
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
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
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
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
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
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
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify settings and rules stored
	var storedSettings, storedRules string
	db.Raw("SELECT COALESCE(settings, ''), COALESCE(rules, '') FROM `groups` WHERE id = ?", groupID).Row().Scan(&storedSettings, &storedRules)
	assert.NotEmpty(t, storedSettings)
	assert.NotEmpty(t, storedRules)
	assert.Contains(t, storedSettings, "duplicates")
	assert.Contains(t, storedRules, "offer")
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
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var tagline string
	db.Raw("SELECT COALESCE(tagline, '') FROM `groups` WHERE id = ?", groupID).Scan(&tagline)
	assert.Equal(t, "Support set this", tagline)
}
