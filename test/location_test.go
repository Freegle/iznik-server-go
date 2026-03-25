package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/location"
	"github.com/stretchr/testify/assert"
)

func TestClosest(t *testing.T) {
	l := location.ClosestPostcode(55.957571, -3.205333)
	id := l.ID
	assert.NotNil(t, id)
	name := l.Name
	areaname := l.Areaname
	assert.Greater(t, id, uint64(0))
	assert.Greater(t, len(name), 0)
	assert.Greater(t, len(areaname), 0)

	location := location.FetchSingle(id)
	assert.Equal(t, name, location.Name)
	assert.Equal(t, areaname, location.Areaname)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/"+fmt.Sprint(id), nil))
	assert.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &location)
	assert.Equal(t, location.ID, id)
}

func TestTypeahead(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/typeahead?q=EH3&groupsnear=true&limit=1000", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var locations []location.Location
	json2.Unmarshal(rsp(resp), &locations)
	assert.Greater(t, len(locations), 0)
	assert.Greater(t, len(locations[0].Name), 0)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/location/typeahead?p=EH3", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestLatLng(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/latlng?lat=55.957571&lng=-3.205333", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var location location.Location
	json2.Unmarshal(rsp(resp), &location)
	assert.Equal(t, location.Name, "EH3 6SS")
}

func TestLocation_InvalidID(t *testing.T) {
	// Non-integer location ID
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/notanint", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestLocation_NonExistentID(t *testing.T) {
	// Location ID that doesn't exist - handler returns 404.
	resp, err := getApp().Test(httptest.NewRequest("GET", "/api/location/999999999", nil), 10000)
	assert.NoError(t, err)

	if assert.NotNil(t, resp) {
		assert.Equal(t, 404, resp.StatusCode)
	}
}

func TestTypeahead_MissingQuery(t *testing.T) {
	// No query param at all
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/typeahead", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestLatLngGroupsNearOntn(t *testing.T) {
	// LatLng should return groupsnear with the ontn field
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/latlng?lat=55.957571&lng=-3.205333", nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Parse raw JSON to check ontn field exists
	var raw map[string]json2.RawMessage
	json2.Unmarshal(rsp(resp), &raw)
	assert.Contains(t, string(raw["groupsnear"]), "ontn", "groupsnear should include ontn field")
}

func TestTypeaheadAreaField(t *testing.T) {
	// Typeahead response should include area with lat/lng for postcodes that have an areaid
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/typeahead?q=EH3+6&limit=1", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var locations []location.Location
	json2.Unmarshal(rsp(resp), &locations)
	assert.Greater(t, len(locations), 0)

	if len(locations) > 0 && locations[0].Areaid > 0 {
		assert.NotNil(t, locations[0].Area, "Location with areaid should have area field populated")
		assert.Equal(t, locations[0].Areaid, locations[0].Area.ID)
		assert.NotZero(t, locations[0].Area.Lat, "Area should have lat")
		assert.NotZero(t, locations[0].Area.Lng, "Area should have lng")
	}
}

func TestTypeahead_V2Path(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/location/typeahead?q=EH3&limit=5", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var locations []location.Location
	json2.Unmarshal(rsp(resp), &locations)
	assert.Greater(t, len(locations), 0)
}

func TestAddresses(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/1687412/addresses", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var addresses []location.Address
	json2.Unmarshal(rsp(resp), &addresses)
	assert.Greater(t, len(addresses), 0)
}

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
