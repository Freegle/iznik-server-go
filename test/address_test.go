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
	// Get logged out.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/address", nil))
	assert.Equal(t, 401, resp.StatusCode)

	user, token := GetUserWithToken(t)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/address?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var addresses []address2.Address
	json2.Unmarshal(rsp(resp), &addresses)
	assert.Greater(t, len(addresses), 0)
	assert.Equal(t, addresses[0].Userid, user.ID)

	// Get by id
	idstr := strconv.FormatUint(addresses[0].ID, 10)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/address/"+idstr+"?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var address address2.Address
	json2.Unmarshal(rsp(resp), &address)
	assert.Equal(t, address.ID, addresses[0].ID)
	assert.Equal(t, address.Userid, user.ID)

	// Invalid id.
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/address/0?jwt="+token, nil))
	assert.Equal(t, 404, resp.StatusCode)

	// Without token
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/address/"+idstr, nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestAddressModeratorAccess(t *testing.T) {
	// Test that moderators can view addresses in chats for groups they moderate,
	// and that support/admin users can view any address

	// Get a regular user with an address
	user1, token1 := GetUserWithToken(t)

	// Get their addresses
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/address?jwt="+token1, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var addresses []address2.Address
	json2.Unmarshal(rsp(resp), &addresses)
	assert.Greater(t, len(addresses), 0)
	addressID := addresses[0].ID
	idstr := strconv.FormatUint(addressID, 10)

	// Get a different regular user - they should NOT be able to see user1's address
	user2, token2 := GetUserWithToken(t, []uint64{user1.ID})
	assert.NotEqual(t, user1.ID, user2.ID, "Need different users for test")

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/address/"+idstr+"?jwt="+token2, nil))
	assert.Equal(t, 404, resp.StatusCode, "Regular user should not see another user's address")

	// Test Support user can access any address
	_, supportToken := GetUserWithToken(t, "Support")
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/address/"+idstr+"?jwt="+supportToken, nil))
	assert.Equal(t, 200, resp.StatusCode, "Support user should be able to see any address")

	var addressSupport address2.Address
	json2.Unmarshal(rsp(resp), &addressSupport)
	assert.Equal(t, addressID, addressSupport.ID)

	// Test Admin user can access any address
	_, adminToken := GetUserWithToken(t, "Admin")
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/address/"+idstr+"?jwt="+adminToken, nil))
	assert.Equal(t, 200, resp.StatusCode, "Admin user should be able to see any address")

	var addressAdmin address2.Address
	json2.Unmarshal(rsp(resp), &addressAdmin)
	assert.Equal(t, addressID, addressAdmin.ID)
}
