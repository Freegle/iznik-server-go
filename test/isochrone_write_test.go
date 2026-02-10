package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func TestCreateIsochrone(t *testing.T) {
	prefix := uniquePrefix("IsoCreate")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	db := database.DBConn

	// Create a location for the isochrone.
	var locID uint64
	db.Raw("SELECT id FROM locations LIMIT 1").Scan(&locID)
	if locID == 0 {
		t.Skip("No locations in test database")
	}

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
	if locID == 0 {
		t.Skip("No locations in test database")
	}

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
	assert.Equal(t, 200, resp.StatusCode)

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
	var locID uint64
	db.Raw("SELECT id FROM locations LIMIT 1").Scan(&locID)
	if locID == 0 {
		t.Skip("No locations in test database")
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

func TestIsochroneWriteV2Path(t *testing.T) {
	req := httptest.NewRequest("DELETE", "/apiv2/isochrone?id=0", nil)
	resp, _ := getApp().Test(req)
	// Should get 401 (not logged in) rather than 404 (route not found).
	assert.Equal(t, 401, resp.StatusCode)
}
