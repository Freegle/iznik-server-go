package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"testing"

	address2 "github.com/freegle/iznik-server-go/address"
	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func TestAddress(t *testing.T) {
	// Get logged out - should return 401
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/address", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Create a full test user with address
	prefix := uniquePrefix("addr")
	userID, token := CreateFullTestUser(t, prefix)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/address?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var addresses []address2.Address
	json2.Unmarshal(rsp(resp), &addresses)
	assert.Greater(t, len(addresses), 0)
	assert.Equal(t, addresses[0].Userid, userID)

	// Get by id
	idstr := strconv.FormatUint(addresses[0].ID, 10)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/address/"+idstr+"?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var address address2.Address
	json2.Unmarshal(rsp(resp), &address)
	assert.Equal(t, address.ID, addresses[0].ID)
	assert.Equal(t, address.Userid, userID)

	// Invalid id
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/address/0?jwt="+token, nil))
	assert.Equal(t, 404, resp.StatusCode)

	// Without token
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/address/"+idstr, nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestAddressModeratorAccess(t *testing.T) {
	// Test that support/admin users can view any address

	// Create a regular user with an address
	prefix1 := uniquePrefix("addr_user1")
	userID1, token1 := CreateFullTestUser(t, prefix1)

	// Get their addresses
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/address?jwt="+token1, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var addresses []address2.Address
	json2.Unmarshal(rsp(resp), &addresses)
	assert.Greater(t, len(addresses), 0)
	addressID := addresses[0].ID
	idstr := strconv.FormatUint(addressID, 10)

	// Create a different regular user - they should NOT be able to see user1's address
	prefix2 := uniquePrefix("addr_user2")
	userID2, token2 := CreateFullTestUser(t, prefix2)
	assert.NotEqual(t, userID1, userID2, "Need different users for test")

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/address/"+idstr+"?jwt="+token2, nil))
	assert.Equal(t, 404, resp.StatusCode, "Regular user should not see another user's address")

	// Create Support user who can access any address
	prefixSupport := uniquePrefix("addr_support")
	supportUserID := CreateTestUser(t, prefixSupport, "Support")
	_, supportToken := CreateTestSession(t, supportUserID)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/address/"+idstr+"?jwt="+supportToken, nil))
	assert.Equal(t, 200, resp.StatusCode, "Support user should be able to see any address")

	var addressSupport address2.Address
	json2.Unmarshal(rsp(resp), &addressSupport)
	assert.Equal(t, addressID, addressSupport.ID)

	// Create Admin user who can access any address
	prefixAdmin := uniquePrefix("addr_admin")
	adminUserID := CreateTestUser(t, prefixAdmin, "Admin")
	_, adminToken := CreateTestSession(t, adminUserID)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/address/"+idstr+"?jwt="+adminToken, nil))
	assert.Equal(t, 200, resp.StatusCode, "Admin user should be able to see any address")

	var addressAdmin address2.Address
	json2.Unmarshal(rsp(resp), &addressAdmin)
	assert.Equal(t, addressID, addressAdmin.ID)
}

func TestAddressCreate(t *testing.T) {
	prefix := uniquePrefix("addr_create")
	userID, token := CreateFullTestUser(t, prefix)

	// Ensure a second PAF address exists for the test (setupLocationTestData only creates one)
	db := database.DBConn
	db.Exec("INSERT IGNORE INTO paf_addresses (id, postcodeid, udprn) VALUES (102367697, 1687412, 50464673)")

	var existingPafID uint64
	db.Raw("SELECT pafid FROM users_addresses WHERE userid = ? LIMIT 1", userID).Scan(&existingPafID)

	var newPafID uint64
	db.Raw("SELECT id FROM paf_addresses WHERE id != ? LIMIT 1", existingPafID).Scan(&newPafID)
	assert.NotZero(t, newPafID, "Need a different PAF address for test")

	body, _ := json2.Marshal(map[string]interface{}{
		"pafid":        newPafID,
		"instructions": "Ring the bell twice",
	})

	req := httptest.NewRequest("POST", "/api/address?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.NotZero(t, result["id"], "Should return new address ID")

	// Verify the address was created in DB
	newID := uint64(result["id"].(float64))
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_addresses WHERE id = ? AND userid = ? AND pafid = ? AND instructions = ?",
		newID, userID, newPafID, "Ring the bell twice").Scan(&count)
	assert.Equal(t, int64(1), count, "Address should exist in DB")
}

func TestAddressCreateUnauthorized(t *testing.T) {
	body, _ := json2.Marshal(map[string]interface{}{
		"pafid": 1,
	})

	req := httptest.NewRequest("POST", "/api/address", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestAddressCreateDuplicatePafid(t *testing.T) {
	prefix := uniquePrefix("addr_dup")
	userID, token := CreateFullTestUser(t, prefix)

	// Get the pafid of the existing address (created by CreateFullTestUser)
	db := database.DBConn
	var existingPafID uint64
	db.Raw("SELECT pafid FROM users_addresses WHERE userid = ? LIMIT 1", userID).Scan(&existingPafID)
	assert.NotZero(t, existingPafID, "Should have existing address")

	// Try to create an address with the same pafid - should return existing address ID (REPLACE behavior)
	body, _ := json2.Marshal(map[string]interface{}{
		"pafid":        existingPafID,
		"instructions": "Updated instructions",
	})

	req := httptest.NewRequest("POST", "/api/address?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.NotZero(t, result["id"], "Should return address ID")
}

func TestAddressCreateMissingPafid(t *testing.T) {
	prefix := uniquePrefix("addr_nopaf")
	_, token := CreateFullTestUser(t, prefix)

	body, _ := json2.Marshal(map[string]interface{}{
		"instructions": "No pafid provided",
	})

	req := httptest.NewRequest("POST", "/api/address?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestAddressUpdate(t *testing.T) {
	prefix := uniquePrefix("addr_update")
	userID, token := CreateFullTestUser(t, prefix)

	// Get the existing address
	db := database.DBConn
	var addressID uint64
	db.Raw("SELECT id FROM users_addresses WHERE userid = ? LIMIT 1", userID).Scan(&addressID)
	assert.NotZero(t, addressID, "Should have existing address")

	body, _ := json2.Marshal(map[string]interface{}{
		"id":           addressID,
		"instructions": "Leave at the back door",
		"lat":          55.9533,
		"lng":          -3.1883,
	})

	req := httptest.NewRequest("PATCH", "/api/address?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req, 5000)
	if !assert.NotNil(t, resp, "expected response for address update") {
		return
	}
	assert.Equal(t, 200, resp.StatusCode)

	// Verify the update
	var instructions string
	db.Raw("SELECT instructions FROM users_addresses WHERE id = ?", addressID).Scan(&instructions)
	assert.Equal(t, "Leave at the back door", instructions)
}

func TestAddressUpdateUnauthorized(t *testing.T) {
	body, _ := json2.Marshal(map[string]interface{}{
		"id":           1,
		"instructions": "Should not work",
	})

	req := httptest.NewRequest("PATCH", "/api/address", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestAddressUpdateOtherUser(t *testing.T) {
	prefix1 := uniquePrefix("addr_upd1")
	userID1, _ := CreateFullTestUser(t, prefix1)

	prefix2 := uniquePrefix("addr_upd2")
	_, token2 := CreateFullTestUser(t, prefix2)

	// Get user1's address
	db := database.DBConn
	var addressID uint64
	db.Raw("SELECT id FROM users_addresses WHERE userid = ? LIMIT 1", userID1).Scan(&addressID)
	assert.NotZero(t, addressID, "Should have address for user1")

	// Try to update user1's address with user2's token
	body, _ := json2.Marshal(map[string]interface{}{
		"id":           addressID,
		"instructions": "Hacked!",
	})

	req := httptest.NewRequest("PATCH", "/api/address?jwt="+token2, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode, "Should not be able to update another user's address")
}

func TestAddressDelete(t *testing.T) {
	prefix := uniquePrefix("addr_delete")
	userID, token := CreateFullTestUser(t, prefix)

	// Get the existing address
	db := database.DBConn
	var addressID uint64
	db.Raw("SELECT id FROM users_addresses WHERE userid = ? LIMIT 1", userID).Scan(&addressID)
	assert.NotZero(t, addressID, "Should have existing address")

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/address/%d?jwt=%s", addressID, token), nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Verify deletion
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_addresses WHERE id = ?", addressID).Scan(&count)
	assert.Equal(t, int64(0), count, "Address should be deleted")
}

func TestAddressDeleteUnauthorized(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("DELETE", "/api/address/1", nil))
	assert.Equal(t, 401, resp.StatusCode)
}

func TestAddressDeleteOtherUser(t *testing.T) {
	prefix1 := uniquePrefix("addr_del1")
	userID1, _ := CreateFullTestUser(t, prefix1)

	prefix2 := uniquePrefix("addr_del2")
	_, token2 := CreateFullTestUser(t, prefix2)

	// Get user1's address
	db := database.DBConn
	var addressID uint64
	db.Raw("SELECT id FROM users_addresses WHERE userid = ? LIMIT 1", userID1).Scan(&addressID)
	assert.NotZero(t, addressID, "Should have address for user1")

	// Try to delete user1's address with user2's token
	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/address/%d?jwt=%s", addressID, token2), nil))
	assert.Equal(t, 403, resp.StatusCode, "Should not be able to delete another user's address")

	// Verify address still exists
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_addresses WHERE id = ?", addressID).Scan(&count)
	assert.Equal(t, int64(1), count, "Address should still exist")
}

func TestAddressDeleteNonexistent(t *testing.T) {
	prefix := uniquePrefix("addr_delnf")
	_, token := CreateFullTestUser(t, prefix)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/address/999999999?jwt=%s", token), nil))
	assert.Equal(t, 404, resp.StatusCode, "Should return 404 for nonexistent address")
}

func TestAddressUpdateWithLat(t *testing.T) {
	prefix := uniquePrefix("addr_updll")
	userID, token := CreateFullTestUser(t, prefix)

	// Get existing address
	db := database.DBConn
	var addressID uint64
	db.Raw("SELECT id FROM users_addresses WHERE userid = ? LIMIT 1", userID).Scan(&addressID)

	// Update just lat/lng
	body, _ := json2.Marshal(map[string]interface{}{
		"id":  addressID,
		"lat": 51.5074,
		"lng": -0.1278,
	})

	req := httptest.NewRequest("PATCH", "/api/address?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify the lat/lng was updated
	var addr address2.Address
	db.Raw("SELECT lat, lng FROM users_addresses WHERE id = ?", addressID).Scan(&addr)
	assert.InDelta(t, 51.5074, addr.Lat, 0.001)
	assert.InDelta(t, -0.1278, addr.Lng, 0.001)
}
