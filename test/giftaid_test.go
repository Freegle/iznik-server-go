package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/donations"
	"github.com/stretchr/testify/assert"
)

func TestGetGiftAid_NotLoggedIn(t *testing.T) {
	// Test without authentication - should return 401
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/giftaid", nil))
	assert.Equal(t, 401, resp.StatusCode)

	var result map[string]string
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, "Not logged in", result["error"])
}

func TestGetGiftAid_NoRecord(t *testing.T) {
	// Create a test user
	prefix := uniquePrefix("giftaidno")
	userID, token := CreateFullTestUser(t, prefix)

	// Ensure this user has no gift aid record
	db := database.DBConn
	db.Exec("DELETE FROM giftaid WHERE userid = ?", userID)

	// Make authenticated request
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/giftaid?jwt="+token, nil))

	assert.Equal(t, 404, resp.StatusCode)

	var result map[string]string
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, "No Gift Aid declaration found", result["error"])
}

func TestGetGiftAid_Success(t *testing.T) {
	// Create a test user
	prefix := uniquePrefix("giftaidok")
	userID, token := CreateFullTestUser(t, prefix)
	db := database.DBConn

	// Create a test gift aid record for this user
	db.Exec("DELETE FROM giftaid WHERE userid = ?", userID)
	db.Exec(`INSERT INTO giftaid (userid, period, fullname, homeaddress, postcode, housenameornumber)
		VALUES (?, 'Past4YearsAndFuture', 'Test User Name', '123 Test Street', 'TE1 1ST', '123')`,
		userID)

	// Make authenticated request
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/giftaid?jwt="+token, nil))

	assert.Equal(t, 200, resp.StatusCode)

	var result donations.GiftAid
	json2.Unmarshal(rsp(resp), &result)

	// Verify all fields
	assert.Greater(t, result.ID, uint64(0))
	assert.Equal(t, userID, result.UserID)
	assert.Equal(t, "Past4YearsAndFuture", result.Period)
	assert.Equal(t, "Test User Name", result.Fullname)
	assert.Equal(t, "123 Test Street", result.Homeaddress)
	assert.NotNil(t, result.Postcode)
	assert.Equal(t, "TE1 1ST", *result.Postcode)
	assert.NotNil(t, result.Housenameornumber)
	assert.Equal(t, "123", *result.Housenameornumber)
	assert.Nil(t, result.Deleted)
	assert.Nil(t, result.Reviewed)

	// Cleanup
	db.Exec("DELETE FROM giftaid WHERE userid = ?", userID)
}

func TestGetGiftAid_WithReviewed(t *testing.T) {
	// Create a test user
	prefix := uniquePrefix("giftaidrev")
	userID, token := CreateFullTestUser(t, prefix)
	db := database.DBConn

	// Create a gift aid record with reviewed timestamp
	db.Exec("DELETE FROM giftaid WHERE userid = ?", userID)
	db.Exec(`INSERT INTO giftaid (userid, period, fullname, homeaddress, reviewed)
		VALUES (?, 'Future', 'Test Reviewed User', '456 Review Road', NOW())`,
		userID)

	// Make authenticated request
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/giftaid?jwt="+token, nil))

	assert.Equal(t, 200, resp.StatusCode)

	var result donations.GiftAid
	json2.Unmarshal(rsp(resp), &result)

	assert.Greater(t, result.ID, uint64(0))
	assert.Equal(t, userID, result.UserID)
	assert.Equal(t, "Future", result.Period)
	assert.NotNil(t, result.Reviewed)

	// Cleanup
	db.Exec("DELETE FROM giftaid WHERE userid = ?", userID)
}

func TestGetGiftAid_DeletedRecordNotReturned(t *testing.T) {
	// Create a test user
	prefix := uniquePrefix("giftaiddel")
	userID, token := CreateFullTestUser(t, prefix)
	db := database.DBConn

	// Create a deleted gift aid record
	db.Exec("DELETE FROM giftaid WHERE userid = ?", userID)
	db.Exec(`INSERT INTO giftaid (userid, period, fullname, homeaddress, deleted)
		VALUES (?, 'Declined', 'Test Deleted User', '789 Deleted Drive', NOW())`,
		userID)

	// Make authenticated request - should not find the deleted record
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/giftaid?jwt="+token, nil))

	assert.Equal(t, 404, resp.StatusCode)

	var result map[string]string
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, "No Gift Aid declaration found", result["error"])

	// Cleanup
	db.Exec("DELETE FROM giftaid WHERE userid = ?", userID)
}

func TestGetGiftAid_AllPeriodTypes(t *testing.T) {
	// Test all valid period enum values
	periods := []string{"This", "Since", "Future", "Declined", "Past4YearsAndFuture"}

	for _, period := range periods {
		t.Run("Period_"+period, func(t *testing.T) {
			prefix := uniquePrefix("giftaidperiod")
			userID, token := CreateFullTestUser(t, prefix)
			db := database.DBConn

			// Create gift aid record with specific period
			db.Exec("DELETE FROM giftaid WHERE userid = ?", userID)
			db.Exec(`INSERT INTO giftaid (userid, period, fullname, homeaddress)
				VALUES (?, ?, 'Test User', 'Test Address')`,
				userID, period)

			// Make authenticated request
			resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/giftaid?jwt="+token, nil))

			assert.Equal(t, 200, resp.StatusCode)

			var result donations.GiftAid
			json2.Unmarshal(rsp(resp), &result)
			assert.Equal(t, period, result.Period)

			// Cleanup
			db.Exec("DELETE FROM giftaid WHERE userid = ?", userID)
		})
	}
}

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

	// Non-Declined period with missing fullname and no firstname/lastname
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

	// Non-Declined period with firstname but missing lastname (incomplete pair)
	body = `{"period":"This","firstname":"Jon","homeaddress":"123 Street"}`
	req = httptest.NewRequest("POST", "/api/giftaid?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestSetGiftAidWithFirstnameLastname(t *testing.T) {
	prefix := uniquePrefix("ga_fnln")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	db := database.DBConn
	db.Exec("DELETE FROM giftaid WHERE userid = ?", userID)

	// Submit with firstname+lastname instead of fullname
	body := `{"period":"Past4YearsAndFuture","firstname":"Budi","lastname":"Santoso","homeaddress":"1 Test Rd, London, SW1A 1AA"}`
	req := httptest.NewRequest("POST", "/api/giftaid?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify firstname, lastname, and derived fullname stored in DB
	var firstname, lastname, fullname *string
	db.Raw("SELECT firstname, lastname, fullname FROM giftaid WHERE userid = ?", userID).
		Row().Scan(&firstname, &lastname, &fullname)

	assert.NotNil(t, firstname)
	assert.Equal(t, "Budi", *firstname)
	assert.NotNil(t, lastname)
	assert.Equal(t, "Santoso", *lastname)
	assert.NotNil(t, fullname)
	assert.Equal(t, "Budi Santoso", *fullname)

	// Verify GET returns the fields
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/giftaid?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result donations.GiftAid
	json2.Unmarshal(rsp(resp), &result)
	assert.NotNil(t, result.Firstname)
	assert.Equal(t, "Budi", *result.Firstname)
	assert.NotNil(t, result.Lastname)
	assert.Equal(t, "Santoso", *result.Lastname)

	db.Exec("DELETE FROM giftaid WHERE userid = ?", userID)
}

func TestSetGiftAidFirstnameLastnameStoredAlongsideFullname(t *testing.T) {
	prefix := uniquePrefix("ga_both")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	db := database.DBConn
	db.Exec("DELETE FROM giftaid WHERE userid = ?", userID)

	// Submit with all three fields; fullname takes precedence in storage
	body := `{"period":"Future","fullname":"John Smith","firstname":"John","lastname":"Smith","homeaddress":"2 High St"}`
	req := httptest.NewRequest("POST", "/api/giftaid?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var firstname, lastname, fullname *string
	db.Raw("SELECT firstname, lastname, fullname FROM giftaid WHERE userid = ?", userID).
		Row().Scan(&firstname, &lastname, &fullname)

	assert.Equal(t, "John Smith", *fullname)
	assert.NotNil(t, firstname)
	assert.Equal(t, "John", *firstname)
	assert.NotNil(t, lastname)
	assert.Equal(t, "Smith", *lastname)

	db.Exec("DELETE FROM giftaid WHERE userid = ?", userID)
}

func TestEditGiftAidFirstnameLastname(t *testing.T) {
	prefix := uniquePrefix("ga_editfnln")
	adminID := CreateTestUser(t, prefix, "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	userID := CreateTestUser(t, prefix+"_user", "User")
	db := database.DBConn
	db.Exec("INSERT INTO giftaid (userid, period, fullname, homeaddress) VALUES (?, 'This', 'Old Name', '1 Road')", userID)

	var giftaidID uint64
	db.Raw("SELECT id FROM giftaid WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&giftaidID)

	// Admin sets firstname+lastname
	body := fmt.Sprintf(`{"id":%d,"firstname":"Nguyen","lastname":"Van An"}`, giftaidID)
	req := httptest.NewRequest("PATCH", "/api/giftaid?jwt="+adminToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var firstname, lastname *string
	db.Raw("SELECT firstname, lastname FROM giftaid WHERE id = ?", giftaidID).Row().Scan(&firstname, &lastname)
	assert.NotNil(t, firstname)
	assert.Equal(t, "Nguyen", *firstname)
	assert.NotNil(t, lastname)
	assert.Equal(t, "Van An", *lastname)

	// fullname should remain unchanged (admin only updated firstname/lastname)
	var fullname string
	db.Raw("SELECT fullname FROM giftaid WHERE id = ?", giftaidID).Scan(&fullname)
	assert.Equal(t, "Old Name", fullname)
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

func TestEditGiftAidClearField(t *testing.T) {
	prefix := uniquePrefix("ga_clear")
	adminID := CreateTestUser(t, prefix, "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	// Create a giftaid record with a populated fullname.
	userID := CreateTestUser(t, prefix+"_user", "User")
	db := database.DBConn
	db.Exec("INSERT INTO giftaid (userid, period, fullname, homeaddress, postcode) VALUES (?, 'This', 'Has Name', '123 Street', 'EH1 1AA')", userID)

	var giftaidID uint64
	db.Raw("SELECT id FROM giftaid WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&giftaidID)
	assert.NotZero(t, giftaidID)

	// Clear the fullname by sending empty string (now possible with *string pointer).
	body := fmt.Sprintf(`{"id":%d,"fullname":""}`, giftaidID)
	req := httptest.NewRequest("PATCH", "/api/giftaid?jwt="+adminToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify fullname was cleared.
	var fullname string
	db.Raw("SELECT fullname FROM giftaid WHERE id = ?", giftaidID).Scan(&fullname)
	assert.Equal(t, "", fullname, "Sending empty string should clear the field")

	// Verify postcode was NOT cleared (we didn't send it).
	var postcode string
	db.Raw("SELECT COALESCE(postcode, '') FROM giftaid WHERE id = ?", giftaidID).Scan(&postcode)
	assert.Equal(t, "EH1 1AA", postcode, "Unmentioned field should not be modified")
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
