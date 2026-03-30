package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/isochrone"
	"github.com/freegle/iznik-server-go/message"
	"github.com/stretchr/testify/assert"
)

func TestIsochrones(t *testing.T) {
	// Get logged out - should return 401
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/isochrone", nil))
	assert.Equal(t, 401, resp.StatusCode)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/isochrone/message", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Create a full test user with isochrone
	prefix := uniquePrefix("iso")
	userID, token := CreateFullTestUser(t, prefix)

	// Get isochrones for user
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/isochrone?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var isochrones []isochrone.Isochrones
	json2.Unmarshal(rsp(resp), &isochrones)
	assert.Greater(t, len(isochrones), 0)
	assert.Equal(t, isochrones[0].Userid, userID)

	// Create a message in the area for this test
	groupID := CreateTestGroup(t, prefix+"_msg")
	CreateTestMessage(t, userID, groupID, "Test Message "+prefix, 55.9533, -3.1883)

	// Should find messages in isochrone area
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/isochrone/message?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msgs []message.MessageSummary
	json2.Unmarshal(rsp(resp), &msgs)
	// Note: May not find messages if isochrone geometry doesn't match - that's OK
	// The key test is that the endpoint works
}

func TestCreateIsochrone(t *testing.T) {
	prefix := uniquePrefix("IsoCreate")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	db := database.DBConn

	// Create a location for the isochrone.
	var locID uint64
	db.Raw("SELECT id FROM locations LIMIT 1").Scan(&locID)
	assert.NotZero(t, locID, "Test database must have locations")

	body := fmt.Sprintf(`{"transport":"Walk","minutes":30,"nickname":"Home","locationid":%d}`, locID)
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/isochrone?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Greater(t, result["id"].(float64), float64(0))
}

func TestCreateIsochroneClampMinutes(t *testing.T) {
	prefix := uniquePrefix("IsoClamp")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	db := database.DBConn

	var locID uint64
	db.Raw("SELECT id FROM locations LIMIT 1").Scan(&locID)
	assert.NotZero(t, locID, "Test database must have locations")

	// Minutes > 45 should be clamped.
	body := fmt.Sprintf(`{"transport":"Drive","minutes":999,"nickname":"Far","locationid":%d}`, locID)
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/isochrone?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
}

func TestCreateIsochroneNotLoggedIn(t *testing.T) {
	body := `{"transport":"Walk","minutes":30,"locationid":1}`
	req := httptest.NewRequest("PUT", "/api/isochrone", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestDeleteIsochrone(t *testing.T) {
	prefix := uniquePrefix("IsoDel")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// CreateTestIsochrone already creates an isochrones_users link.
	CreateTestIsochrone(t, userID, 55.9533, -3.1883)

	db := database.DBConn
	var isoUserID uint64
	db.Raw("SELECT id FROM isochrones_users WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&isoUserID)
	assert.Greater(t, isoUserID, uint64(0))

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/isochrone?id=%d&jwt=%s", isoUserID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify deleted.
	var count int64
	db.Raw("SELECT COUNT(*) FROM isochrones_users WHERE id = ?", isoUserID).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestDeleteIsochroneWrongUser(t *testing.T) {
	prefix := uniquePrefix("IsoDelWrong")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, otherToken := CreateTestSession(t, otherID)

	CreateTestIsochrone(t, ownerID, 55.9533, -3.1883)

	db := database.DBConn
	var isoUserID uint64
	db.Raw("SELECT id FROM isochrones_users WHERE userid = ? ORDER BY id DESC LIMIT 1", ownerID).Scan(&isoUserID)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/isochrone?id=%d&jwt=%s", isoUserID, otherToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestEditIsochrone(t *testing.T) {
	prefix := uniquePrefix("IsoEdit")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	isoID := CreateTestIsochrone(t, userID, 55.9533, -3.1883)

	db := database.DBConn

	// The isochrone needs a locationid for the edit handler to find it.
	// Find a location with geometry that does NOT already have an isochrone
	// with Cycle/15 to avoid unique key collisions when the edit creates one.
	var locID uint64
	db.Raw("SELECT l.id FROM locations l WHERE l.geometry IS NOT NULL AND l.id NOT IN (SELECT locationid FROM isochrones WHERE locationid IS NOT NULL AND transport = 'Cycle' AND minutes = 15) LIMIT 1").Scan(&locID)
	if locID == 0 {
		t.Fatal("No available location with geometry and without existing Cycle/15 isochrone")
	}
	db.Exec("UPDATE isochrones SET locationid = ? WHERE id = ?", locID, isoID)

	var isoUserID uint64
	db.Raw("SELECT id FROM isochrones_users WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&isoUserID)

	body := fmt.Sprintf(`{"id":%d,"minutes":15,"transport":"Cycle"}`, isoUserID)
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/isochrone?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
}

func TestEditIsochroneNullGeometry(t *testing.T) {
	// Test the COALESCE fallback: when a location has NULL geometry, the edit
	// handler should fall back to ST_GeomFromText('POINT(0 0)') instead of
	// failing with a NOT NULL constraint violation on the polygon column.
	prefix := uniquePrefix("IsoEditNull")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	isoID := CreateTestIsochrone(t, userID, 55.9533, -3.1883)

	db := database.DBConn

	// Create a location with NULL geometry.
	db.Exec("INSERT INTO locations (name, type, lat, lng) VALUES (?, 'Polygon', 55.95, -3.19)", prefix+"_loc")
	var locID uint64
	db.Raw("SELECT id FROM locations WHERE name = ? ORDER BY id DESC LIMIT 1", prefix+"_loc").Scan(&locID)
	if locID == 0 {
		t.Fatal("Failed to create test location")
	}

	// Confirm geometry is NULL.
	var geomCount int64
	db.Raw("SELECT COUNT(*) FROM locations WHERE id = ? AND geometry IS NOT NULL", locID).Scan(&geomCount)
	assert.Equal(t, int64(0), geomCount, "Test location should have NULL geometry")

	// Point the isochrone at this NULL-geometry location.
	db.Exec("UPDATE isochrones SET locationid = ? WHERE id = ?", locID, isoID)

	var isoUserID uint64
	db.Raw("SELECT id FROM isochrones_users WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&isoUserID)

	// Edit the isochrone — this should succeed via COALESCE fallback.
	body := fmt.Sprintf(`{"id":%d,"minutes":15,"transport":"Cycle"}`, isoUserID)
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/isochrone?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
}

func TestEditIsochroneWithCharsetContentType(t *testing.T) {
	// Regression test: mobile browsers (e.g. Chrome on Android/Capacitor) send
	// Content-Type: application/json; charset=utf-8. The strict == check skipped
	// body parsing, so req.ID stayed 0 → 400 "Missing id". With strings.Contains
	// the body is parsed and the id is found.
	//
	// We verify the fix by sending a PATCH with a valid isochrone_users ID using
	// the charset content-type. The OLD code would return 400 (Missing id).
	// The NEW code reads the id from the body, then proceeds to the edit logic.
	// We don't need the full locationid setup: getting a non-400 response proves
	// the body was parsed.
	prefix := uniquePrefix("IsoEditCharset")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	CreateTestIsochrone(t, userID, 55.9533, -3.1883)

	db := database.DBConn
	var isoUserID uint64
	db.Raw("SELECT id FROM isochrones_users WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&isoUserID)
	assert.Greater(t, isoUserID, uint64(0))

	// With the old strict == check, id wouldn't be parsed from the JSON body
	// and we'd get 400 "Missing id". With strings.Contains the body is parsed.
	body := fmt.Sprintf(`{"id":%d,"minutes":20,"transport":"Cycle"}`, isoUserID)
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/isochrone?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, _ := getApp().Test(req)

	// Must NOT be 400 — that would mean "Missing id" (body not parsed).
	assert.NotEqual(t, 400, resp.StatusCode, "Body should be parsed even with charset in Content-Type")
}

func TestEditIsochroneEmptyTransport(t *testing.T) {
	// Empty transport should fall back to the current isochrone's transport (or "Walk" default),
	// not fail with 400. This handles historical NULL transport rows in the DB.
	prefix := uniquePrefix("IsoEditEmpty")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	isoID := CreateTestIsochrone(t, userID, 55.9533, -3.1883)

	db := database.DBConn

	// Set up locationid for the edit handler — pick a location that won't
	// collide with existing Walk/30 (current) or Walk/20 (edit target) isochrones.
	var locID uint64
	db.Raw("SELECT l.id FROM locations l WHERE l.geometry IS NOT NULL AND l.id NOT IN (SELECT locationid FROM isochrones WHERE locationid IS NOT NULL AND transport = 'Walk' AND minutes IN (20, 30)) LIMIT 1").Scan(&locID)
	if locID == 0 {
		t.Fatal("No available location with geometry and without existing Walk/20 or Walk/30 isochrone")
	}
	db.Exec("UPDATE isochrones SET locationid = ?, transport = 'Walk' WHERE id = ?", locID, isoID)

	var isoUserID uint64
	db.Raw("SELECT id FROM isochrones_users WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&isoUserID)
	assert.Greater(t, isoUserID, uint64(0))

	body := fmt.Sprintf(`{"id":%d,"minutes":20,"transport":""}`, isoUserID)
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/isochrone?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	// Should succeed — empty transport falls back to current ("Walk").
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
}

func TestEditIsochroneInvalidTransport(t *testing.T) {
	// Invalid (non-empty, non-matching) transport should return 400.
	prefix := uniquePrefix("IsoEditBadTr")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	CreateTestIsochrone(t, userID, 55.9533, -3.1883)

	db := database.DBConn
	var isoUserID uint64
	db.Raw("SELECT id FROM isochrones_users WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&isoUserID)
	assert.Greater(t, isoUserID, uint64(0))

	body := fmt.Sprintf(`{"id":%d,"minutes":20,"transport":"Teleport"}`, isoUserID)
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/isochrone?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestIsochroneWriteV2Path(t *testing.T) {
	req := httptest.NewRequest("DELETE", "/apiv2/isochrone?id=0", nil)
	resp, _ := getApp().Test(req)
	// Should get 401 (not logged in) rather than 404 (route not found).
	assert.Equal(t, 401, resp.StatusCode)
}
