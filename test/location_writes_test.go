package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func TestCreateLocation(t *testing.T) {
	prefix := uniquePrefix("locwr_create")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	body := fmt.Sprintf(`{"name":"Test Location %s","polygon":"POLYGON((-3.21 55.94, -3.21 55.97, -3.18 55.97, -3.18 55.94, -3.21 55.94))"}`, prefix)
	req := httptest.NewRequest("PUT", "/api/locations?jwt="+adminToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Greater(t, result["id"], float64(0))

	// Cleanup
	db := database.DBConn
	db.Exec("DELETE FROM locations WHERE id = ?", int(result["id"].(float64)))
}

func TestCreateLocationNotAdmin(t *testing.T) {
	prefix := uniquePrefix("locwr_notadm")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{"name":"Test Location","polygon":"POLYGON((-3.21 55.94, -3.21 55.97, -3.18 55.97, -3.18 55.94, -3.21 55.94))"}`
	req := httptest.NewRequest("PUT", "/api/locations?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestCreateLocationNotLoggedIn(t *testing.T) {
	body := `{"name":"Test Location","polygon":"POLYGON((-3.21 55.94, -3.21 55.97, -3.18 55.97, -3.18 55.94, -3.21 55.94))"}`
	req := httptest.NewRequest("PUT", "/api/locations", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestUpdateLocation(t *testing.T) {
	prefix := uniquePrefix("locwr_upd")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	// Create a location first.
	db := database.DBConn
	db.Exec("INSERT INTO locations (name, type, canon, popularity) VALUES (?, 'Polygon', ?, 0)",
		"UpdateTest "+prefix, "updatetest "+prefix)
	var locID uint64
	db.Raw("SELECT id FROM locations WHERE name = ? ORDER BY id DESC LIMIT 1", "UpdateTest "+prefix).Scan(&locID)
	assert.Greater(t, locID, uint64(0))

	// Update polygon.
	newPolygon := "POLYGON((-3.22 55.93, -3.22 55.98, -3.17 55.98, -3.17 55.93, -3.22 55.93))"
	body := fmt.Sprintf(`{"id":%d,"polygon":"%s"}`, locID, newPolygon)
	req := httptest.NewRequest("PATCH", "/api/locations?jwt="+adminToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, true, result["success"])

	// Update name.
	newName := "Updated " + prefix
	body = fmt.Sprintf(`{"id":%d,"name":"%s"}`, locID, newName)
	req = httptest.NewRequest("PATCH", "/api/locations?jwt="+adminToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify name was updated.
	var name string
	db.Raw("SELECT name FROM locations WHERE id = ?", locID).Scan(&name)
	assert.Equal(t, newName, name)

	// Verify canon was set to lowercase.
	var canon string
	db.Raw("SELECT canon FROM locations WHERE id = ?", locID).Scan(&canon)
	assert.Equal(t, "updated "+prefix, canon)

	// Cleanup
	db.Exec("DELETE FROM locations WHERE id = ?", locID)
}

func TestUpdateLocationNotAdmin(t *testing.T) {
	prefix := uniquePrefix("locwr_updna")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{"id":1,"name":"Hacked"}`
	req := httptest.NewRequest("PATCH", "/api/locations?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestExcludeLocation(t *testing.T) {
	prefix := uniquePrefix("locwr_excl")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Create a test location.
	db := database.DBConn
	db.Exec("INSERT INTO locations (name, type, canon, popularity) VALUES (?, 'Polygon', ?, 0)",
		"ExclTest "+prefix, "excltest "+prefix)
	var locID uint64
	db.Raw("SELECT id FROM locations WHERE name = ? ORDER BY id DESC LIMIT 1", "ExclTest "+prefix).Scan(&locID)
	assert.Greater(t, locID, uint64(0))

	body := fmt.Sprintf(`{"id":%d,"groupid":%d,"action":"Exclude"}`, locID, groupID)
	req := httptest.NewRequest("POST", "/api/locations?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify exclusion was created.
	var count int64
	db.Raw("SELECT COUNT(*) FROM locations_excluded WHERE locationid = ? AND groupid = ?", locID, groupID).Scan(&count)
	assert.Equal(t, int64(1), count)

	// Cleanup
	db.Exec("DELETE FROM locations_excluded WHERE locationid = ? AND groupid = ?", locID, groupID)
	db.Exec("DELETE FROM locations WHERE id = ?", locID)
}

func TestExcludeLocationNotMod(t *testing.T) {
	prefix := uniquePrefix("locwr_exclnm")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	body := fmt.Sprintf(`{"id":1,"groupid":%d,"action":"Exclude"}`, groupID)
	req := httptest.NewRequest("POST", "/api/locations?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestConvertKML(t *testing.T) {
	prefix := uniquePrefix("locwr_kml")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	kml := `<?xml version="1.0" encoding="UTF-8"?>
<kml xmlns="http://www.opengis.net/kml/2.2">
<Document>
<Placemark>
<Polygon>
<outerBoundaryIs>
<LinearRing>
<coordinates>-0.1,51.5,0 -0.1,51.6,0 0.0,51.6,0 0.0,51.5,0 -0.1,51.5,0</coordinates>
</LinearRing>
</outerBoundaryIs>
</Polygon>
</Placemark>
</Document>
</kml>`

	body, _ := json2.Marshal(map[string]interface{}{
		"action": "kml",
		"kml":    kml,
	})
	req := httptest.NewRequest("POST", "/api/locations/kml?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])
	assert.Contains(t, result["wkt"], "POLYGON")
	assert.Contains(t, result["wkt"], "-0.1 51.5")
}

func TestConvertKMLNotLoggedIn(t *testing.T) {
	body := `{"action":"kml","kml":"<kml/>"}`
	req := httptest.NewRequest("POST", "/api/locations/kml", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestConvertKMLInvalidXML(t *testing.T) {
	prefix := uniquePrefix("locwr_kmlbad")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body, _ := json2.Marshal(map[string]interface{}{
		"action": "kml",
		"kml":    "not valid xml at all",
	})
	req := httptest.NewRequest("POST", "/api/locations/kml?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestConvertKMLEmptyKML(t *testing.T) {
	prefix := uniquePrefix("locwr_kmlempty")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body, _ := json2.Marshal(map[string]interface{}{
		"action": "kml",
		"kml":    "",
	})
	req := httptest.NewRequest("POST", "/api/locations/kml?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}
