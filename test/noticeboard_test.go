package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/stretchr/testify/assert"
)

func TestPostNoticeboardCreate(t *testing.T) {
	prefix := uniquePrefix("nb_create")
	db := database.DBConn
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"lat":         51.5074,
		"lng":         -0.1278,
		"name":        "Test Noticeboard",
		"description": "A test board",
		"active":      true,
	})
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/noticeboard?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.NotNil(t, result["id"])

	// Verify in DB
	id := uint64(result["id"].(float64))
	var name string
	var addedby uint64
	db.Raw("SELECT name, COALESCE(addedby, 0) FROM noticeboards WHERE id = ?", id).Row().Scan(&name, &addedby)
	assert.Equal(t, "Test Noticeboard", name)
	assert.Equal(t, userID, addedby)
}

func TestPostNoticeboardCreateNoLatLng(t *testing.T) {
	prefix := uniquePrefix("nb_nolatlng")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "Test Board",
	})
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/noticeboard?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostNoticeboardActionRefreshed(t *testing.T) {
	prefix := uniquePrefix("nb_refresh")
	db := database.DBConn
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	// Create a noticeboard first
	nbID := createTestNoticeboard(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"id":     nbID,
		"action": "Refreshed",
	})
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/noticeboard?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify check record created
	var checkCount int64
	db.Raw("SELECT COUNT(*) FROM noticeboards_checks WHERE noticeboardid = ? AND refreshed = 1", nbID).Scan(&checkCount)
	assert.Equal(t, int64(1), checkCount)

	// Verify active set to 1
	var active int
	db.Raw("SELECT active FROM noticeboards WHERE id = ?", nbID).Scan(&active)
	assert.Equal(t, 1, active)
}

func TestPostNoticeboardActionDeclined(t *testing.T) {
	prefix := uniquePrefix("nb_decline")
	db := database.DBConn
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	nbID := createTestNoticeboard(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"id":     nbID,
		"action": "Declined",
	})
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/noticeboard?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var checkCount int64
	db.Raw("SELECT COUNT(*) FROM noticeboards_checks WHERE noticeboardid = ? AND declined = 1", nbID).Scan(&checkCount)
	assert.Equal(t, int64(1), checkCount)
}

func TestPostNoticeboardActionInactive(t *testing.T) {
	prefix := uniquePrefix("nb_inactive")
	db := database.DBConn
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	nbID := createTestNoticeboard(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"id":     nbID,
		"action": "Inactive",
	})
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/noticeboard?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify active set to 0
	var active int
	db.Raw("SELECT active FROM noticeboards WHERE id = ?", nbID).Scan(&active)
	assert.Equal(t, 0, active)
}

func TestPostNoticeboardActionComments(t *testing.T) {
	prefix := uniquePrefix("nb_comments")
	db := database.DBConn
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	nbID := createTestNoticeboard(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"id":       nbID,
		"action":   "Comments",
		"comments": "Poster looks great",
	})
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/noticeboard?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var comments string
	db.Raw("SELECT comments FROM noticeboards_checks WHERE noticeboardid = ? ORDER BY id DESC LIMIT 1", nbID).Scan(&comments)
	assert.Equal(t, "Poster looks great", comments)
}

func TestPostNoticeboardActionNoID(t *testing.T) {
	prefix := uniquePrefix("nb_actionoid")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"action": "Refreshed",
	})
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/noticeboard?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPatchNoticeboard(t *testing.T) {
	prefix := uniquePrefix("nb_patch")
	db := database.DBConn
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	nbID := createTestNoticeboard(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"id":          nbID,
		"name":        "Updated Board",
		"description": "New description",
		"active":      false,
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/noticeboard?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var name, description string
	var active int
	db.Raw("SELECT name, COALESCE(description, ''), active FROM noticeboards WHERE id = ?", nbID).Row().Scan(&name, &description, &active)
	assert.Equal(t, "Updated Board", name)
	assert.Equal(t, "New description", description)
	assert.Equal(t, 0, active)
}

func TestPatchNoticeboardNoID(t *testing.T) {
	prefix := uniquePrefix("nb_patchnoid")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "No ID Board",
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/noticeboard?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPatchNoticeboardNotFound(t *testing.T) {
	prefix := uniquePrefix("nb_patchnf")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"id":   99999999,
		"name": "Ghost Board",
	})
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/noticeboard?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestPostNoticeboardNotLoggedIn(t *testing.T) {
	body, _ := json.Marshal(map[string]interface{}{
		"lat":  51.5074,
		"lng":  -0.1278,
		"name": "Anonymous Board",
	})
	req := httptest.NewRequest("POST", "/api/noticeboard", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	// Handler doesn't require auth explicitly - addedby will be 0 (anonymous).
	// This tests that the endpoint doesn't crash without auth.
	assert.Equal(t, 200, resp.StatusCode)
}

func TestPostNoticeboardInvalidAction(t *testing.T) {
	prefix := uniquePrefix("nb_invact")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	nbID := createTestNoticeboard(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"id":     nbID,
		"action": "BogusAction",
	})
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/noticeboard?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostNoticeboardEmptyBody(t *testing.T) {
	prefix := uniquePrefix("nb_empty")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	req := httptest.NewRequest("POST", fmt.Sprintf("/api/noticeboard?jwt=%s", token), bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	// Empty body with no action and no lat/lng should fail
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostNoticeboardInvalidJSON(t *testing.T) {
	prefix := uniquePrefix("nb_badjson")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	req := httptest.NewRequest("POST", fmt.Sprintf("/api/noticeboard?jwt=%s", token), bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostNoticeboardActionNonExistentBoard(t *testing.T) {
	prefix := uniquePrefix("nb_ghost")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"id":     99999999,
		"action": "Refreshed",
	})
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/noticeboard?jwt=%s", token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	// Handler inserts check record regardless - this tests it doesn't crash
	assert.Equal(t, 200, resp.StatusCode)
}

func TestPatchNoticeboardInvalidJSON(t *testing.T) {
	prefix := uniquePrefix("nb_patchjson")
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/noticeboard?jwt=%s", token), bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestGetNoticeboardSingle(t *testing.T) {
	prefix := uniquePrefix("nb_getsingle")
	db := database.DBConn
	userID := CreateTestUser(t, prefix+"_user", "User")

	nbID := createTestNoticeboard(t, userID)

	// Add a check record
	db.Exec("INSERT INTO noticeboards_checks (noticeboardid, userid, checkedat, refreshed, inactive) VALUES (?, ?, NOW(), 1, 0)", nbID, userID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/noticeboard/%d", nbID), nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	// Verify flat V2 format: addedby is a user ID number, not a nested object
	assert.Equal(t, float64(nbID), result["id"])
	assert.Equal(t, float64(userID), result["addedby"])
	assert.Equal(t, "Test Board", result["name"])
	assert.Equal(t, true, result["active"])

	// Verify checks are flat with userid as ID
	checks := result["checks"].([]interface{})
	assert.GreaterOrEqual(t, len(checks), 1)
	check := checks[0].(map[string]interface{})
	assert.Equal(t, float64(userID), check["userid"])
	// No nested user object
	assert.Nil(t, check["user"])
}

func TestGetNoticeboardSingleNotFound(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/noticeboard/99999999", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestGetNoticeboardSingleInvalidID(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/noticeboard/abc", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestGetNoticeboardSingleWithPhoto(t *testing.T) {
	prefix := uniquePrefix("nb_getphoto")
	db := database.DBConn
	userID := CreateTestUser(t, prefix+"_user", "User")

	nbID := createTestNoticeboard(t, userID)

	// Insert a photo record
	db.Exec("INSERT INTO noticeboards_images (noticeboardid, contenttype) VALUES (?, 'image/jpeg')", nbID)
	var photoID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&photoID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/noticeboard/%d", nbID), nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	photo := result["photo"].(map[string]interface{})
	assert.Equal(t, float64(photoID), photo["id"])
	assert.Contains(t, photo["path"], "bimg_")
	assert.Contains(t, photo["paththumb"], "tbimg_")
}

func TestGetNoticeboardSingleEmptyChecks(t *testing.T) {
	prefix := uniquePrefix("nb_getnochk")
	userID := CreateTestUser(t, prefix+"_user", "User")

	nbID := createTestNoticeboard(t, userID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/noticeboard/%d", nbID), nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	// Empty checks should be an empty array, not null
	checks := result["checks"].([]interface{})
	assert.Equal(t, 0, len(checks))
}

func TestGetNoticeboardList(t *testing.T) {
	prefix := uniquePrefix("nb_getlist")
	userID := CreateTestUser(t, prefix+"_user", "User")

	createTestNoticeboard(t, userID)

	req := httptest.NewRequest("GET", "/api/noticeboard", nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	noticeboards := result["noticeboards"].([]interface{})
	assert.GreaterOrEqual(t, len(noticeboards), 1)

	// List items should have flat fields
	nb := noticeboards[0].(map[string]interface{})
	assert.NotNil(t, nb["id"])
	assert.NotNil(t, nb["name"])
	assert.NotNil(t, nb["lat"])
	assert.NotNil(t, nb["lng"])
}

func TestGetNoticeboardListEmpty(t *testing.T) {
	// Get list - even if empty, should return array not null
	req := httptest.NewRequest("GET", "/api/noticeboard", nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	// noticeboards should be an array (possibly empty, possibly with test data)
	assert.NotNil(t, result["noticeboards"])
}

// Helper to create a test noticeboard
func createTestNoticeboard(t *testing.T, addedby uint64) uint64 {
	db := database.DBConn

	result := db.Exec(
		fmt.Sprintf("INSERT INTO noticeboards (name, lat, lng, position, added, addedby, active, lastcheckedat) "+
			"VALUES ('Test Board', 51.5074, -0.1278, ST_GeomFromText('POINT(-0.1278 51.5074)', %d), NOW(), ?, 1, NOW())", utils.SRID),
		addedby)

	if result.Error != nil {
		t.Fatalf("Failed to create test noticeboard: %v", result.Error)
	}

	var id uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)
	return id
}
