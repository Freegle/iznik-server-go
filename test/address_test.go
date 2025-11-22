package test

import (
	json2 "encoding/json"
	address2 "github.com/freegle/iznik-server-go/address"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"strconv"
	"testing"
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
