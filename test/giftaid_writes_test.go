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

func TestSetGiftAid(t *testing.T) {
	prefix := uniquePrefix("ga_set")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{"period":"This","fullname":"Test User","homeaddress":"123 Test Street, Edinburgh"}`
	req := httptest.NewRequest("POST", "/api/giftaid?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Greater(t, result["id"], float64(0))

	// Verify in DB
	db := database.DBConn
	var period string
	db.Raw("SELECT period FROM giftaid WHERE userid = ?", userID).Scan(&period)
	assert.Equal(t, "This", period)
}

func TestSetGiftAidDeclined(t *testing.T) {
	prefix := uniquePrefix("ga_dec")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Declined period should work without fullname/homeaddress
	body := `{"period":"Declined"}`
	req := httptest.NewRequest("POST", "/api/giftaid?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Greater(t, result["id"], float64(0))

	// Verify in DB
	db := database.DBConn
	var period string
	db.Raw("SELECT period FROM giftaid WHERE userid = ?", userID).Scan(&period)
	assert.Equal(t, "Declined", period)
}

func TestSetGiftAidUnauthorized(t *testing.T) {
	body := `{"period":"This","fullname":"Test","homeaddress":"123 Street"}`
	req := httptest.NewRequest("POST", "/api/giftaid", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestSetGiftAidMissingParams(t *testing.T) {
	prefix := uniquePrefix("ga_miss")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Missing period
	body := `{"fullname":"Test","homeaddress":"123 Street"}`
	req := httptest.NewRequest("POST", "/api/giftaid?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)

	// Non-Declined period with missing fullname
	body = `{"period":"This","homeaddress":"123 Street"}`
	req = httptest.NewRequest("POST", "/api/giftaid?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)

	// Non-Declined period with missing homeaddress
	body = `{"period":"This","fullname":"Test"}`
	req = httptest.NewRequest("POST", "/api/giftaid?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestEditGiftAid(t *testing.T) {
	prefix := uniquePrefix("ga_edit")
	adminID := CreateTestUser(t, prefix, "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	// Create a giftaid record first
	userID := CreateTestUser(t, prefix+"_user", "User")
	db := database.DBConn
	db.Exec("INSERT INTO giftaid (userid, period, fullname, homeaddress) VALUES (?, 'This', 'Original Name', '123 Original St')", userID)

	var giftaidID uint64
	db.Raw("SELECT id FROM giftaid WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&giftaidID)
	assert.NotZero(t, giftaidID)

	// Admin edits the record
	body := fmt.Sprintf(`{"id":%d,"fullname":"Updated Name","postcode":"EH1 1AA"}`, giftaidID)
	req := httptest.NewRequest("PATCH", "/api/giftaid?jwt="+adminToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify in DB
	var fullname string
	var postcode *string
	db.Raw("SELECT fullname, postcode FROM giftaid WHERE id = ?", giftaidID).Row().Scan(&fullname, &postcode)
	assert.Equal(t, "Updated Name", fullname)
	assert.NotNil(t, postcode)
	assert.Equal(t, "EH1 1AA", *postcode)
}

func TestEditGiftAidNotAdmin(t *testing.T) {
	prefix := uniquePrefix("ga_editna")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{"id":1,"fullname":"Hacked"}`
	req := httptest.NewRequest("PATCH", "/api/giftaid?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestDeleteGiftAid(t *testing.T) {
	prefix := uniquePrefix("ga_del")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Create a giftaid record first
	db := database.DBConn
	db.Exec("INSERT INTO giftaid (userid, period, fullname, homeaddress) VALUES (?, 'This', 'Test User', '123 Test St')", userID)

	// Delete it
	req := httptest.NewRequest("DELETE", "/api/giftaid?jwt="+token, nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify soft-deleted: period should be Declined and deleted should be set
	var period string
	var deleted *string
	db.Raw("SELECT period, deleted FROM giftaid WHERE userid = ?", userID).Row().Scan(&period, &deleted)
	assert.Equal(t, "Declined", period)
	assert.NotNil(t, deleted)
}

func TestDeleteGiftAidUnauthorized(t *testing.T) {
	req := httptest.NewRequest("DELETE", "/api/giftaid", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestListGiftAid(t *testing.T) {
	prefix := uniquePrefix("ga_list")
	adminID := CreateTestUser(t, prefix, "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	// Create a giftaid record that needs review (not declined, not reviewed, not deleted)
	userID := CreateTestUser(t, prefix+"_user", "User")
	db := database.DBConn
	db.Exec("INSERT INTO giftaid (userid, period, fullname, homeaddress) VALUES (?, 'This', 'List Test User', '456 List St')", userID)

	// Admin lists all
	req := httptest.NewRequest("GET", "/api/giftaid?all=true&jwt="+adminToken, nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	giftaids, ok := result["giftaids"].([]interface{})
	assert.True(t, ok)
	assert.Greater(t, len(giftaids), 0)
}

func TestListGiftAidNotAdmin(t *testing.T) {
	prefix := uniquePrefix("ga_listna")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	req := httptest.NewRequest("GET", "/api/giftaid?all=true&jwt="+token, nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestSearchGiftAid(t *testing.T) {
	prefix := uniquePrefix("ga_search")
	adminID := CreateTestUser(t, prefix, "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	// Create a giftaid record to search for
	userID := CreateTestUser(t, prefix+"_user", "User")
	db := database.DBConn
	searchableName := "UniqueSearchName_" + prefix
	db.Exec("INSERT INTO giftaid (userid, period, fullname, homeaddress) VALUES (?, 'This', ?, '789 Search St')", userID, searchableName)

	// Admin searches
	req := httptest.NewRequest("GET", "/api/giftaid?search="+searchableName+"&jwt="+adminToken, nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	giftaids, ok := result["giftaids"].([]interface{})
	assert.True(t, ok)
	assert.Greater(t, len(giftaids), 0)
}
