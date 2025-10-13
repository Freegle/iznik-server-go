package test

import (
	json2 "encoding/json"
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
	// Get a test user with valid token
	user, token := GetUserWithToken(t)

	// Ensure this user has no gift aid record
	db := database.DBConn
	db.Exec("DELETE FROM giftaid WHERE userid = ?", user.ID)

	// Make authenticated request
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/giftaid?jwt="+token, nil))

	assert.Equal(t, 404, resp.StatusCode)

	var result map[string]string
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, "No Gift Aid declaration found", result["error"])
}

func TestGetGiftAid_Success(t *testing.T) {
	// Get a test user with valid token
	user, token := GetUserWithToken(t)
	db := database.DBConn

	// Create a test gift aid record for this user
	db.Exec("DELETE FROM giftaid WHERE userid = ?", user.ID)
	db.Exec(`INSERT INTO giftaid (userid, period, fullname, homeaddress, postcode, housenameornumber)
		VALUES (?, 'Past4YearsAndFuture', 'Test User Name', '123 Test Street', 'TE1 1ST', '123')`,
		user.ID)

	// Make authenticated request
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/giftaid?jwt="+token, nil))

	assert.Equal(t, 200, resp.StatusCode)

	var result donations.GiftAid
	json2.Unmarshal(rsp(resp), &result)

	// Verify all fields
	assert.Greater(t, result.ID, uint64(0))
	assert.Equal(t, user.ID, result.UserID)
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
	db.Exec("DELETE FROM giftaid WHERE userid = ?", user.ID)
}

func TestGetGiftAid_WithReviewed(t *testing.T) {
	// Get a test user with valid token
	user, token := GetUserWithToken(t)
	db := database.DBConn

	// Create a gift aid record with reviewed timestamp
	db.Exec("DELETE FROM giftaid WHERE userid = ?", user.ID)
	db.Exec(`INSERT INTO giftaid (userid, period, fullname, homeaddress, reviewed)
		VALUES (?, 'Future', 'Test Reviewed User', '456 Review Road', NOW())`,
		user.ID)

	// Make authenticated request
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/giftaid?jwt="+token, nil))

	assert.Equal(t, 200, resp.StatusCode)

	var result donations.GiftAid
	json2.Unmarshal(rsp(resp), &result)

	assert.Greater(t, result.ID, uint64(0))
	assert.Equal(t, user.ID, result.UserID)
	assert.Equal(t, "Future", result.Period)
	assert.NotNil(t, result.Reviewed)

	// Cleanup
	db.Exec("DELETE FROM giftaid WHERE userid = ?", user.ID)
}

func TestGetGiftAid_DeletedRecordNotReturned(t *testing.T) {
	// Get a test user with valid token
	user, token := GetUserWithToken(t)
	db := database.DBConn

	// Create a deleted gift aid record
	db.Exec("DELETE FROM giftaid WHERE userid = ?", user.ID)
	db.Exec(`INSERT INTO giftaid (userid, period, fullname, homeaddress, deleted)
		VALUES (?, 'Declined', 'Test Deleted User', '789 Deleted Drive', NOW())`,
		user.ID)

	// Make authenticated request - should not find the deleted record
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/giftaid?jwt="+token, nil))

	assert.Equal(t, 404, resp.StatusCode)

	var result map[string]string
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, "No Gift Aid declaration found", result["error"])

	// Cleanup
	db.Exec("DELETE FROM giftaid WHERE userid = ?", user.ID)
}

func TestGetGiftAid_AllPeriodTypes(t *testing.T) {
	// Test all valid period enum values
	periods := []string{"This", "Since", "Future", "Declined", "Past4YearsAndFuture"}

	for _, period := range periods {
		t.Run("Period_"+period, func(t *testing.T) {
			user, token := GetUserWithToken(t)
			db := database.DBConn

			// Create gift aid record with specific period
			db.Exec("DELETE FROM giftaid WHERE userid = ?", user.ID)
			db.Exec(`INSERT INTO giftaid (userid, period, fullname, homeaddress)
				VALUES (?, ?, 'Test User', 'Test Address')`,
				user.ID, period)

			// Make authenticated request
			resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/giftaid?jwt="+token, nil))

			assert.Equal(t, 200, resp.StatusCode)

			var result donations.GiftAid
			json2.Unmarshal(rsp(resp), &result)
			assert.Equal(t, period, result.Period)

			// Cleanup
			db.Exec("DELETE FROM giftaid WHERE userid = ?", user.ID)
		})
	}
}
