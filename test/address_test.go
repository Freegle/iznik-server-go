package test

import (
	json2 "encoding/json"
	address2 "github.com/freegle/iznik-server-go/address"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
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
}
