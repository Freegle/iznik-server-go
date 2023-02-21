package test

import (
	json2 "encoding/json"
	"github.com/freegle/iznik-server-go/isochrone"
	"github.com/freegle/iznik-server-go/message"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestIsochrones(t *testing.T) {
	// Get logged out.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/isochrone", nil))
	assert.Equal(t, 401, resp.StatusCode)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/isochrone/message", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Should be able to get isochrones for user.
	user, token := GetUserWithToken(t)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/isochrone?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var isochrones []isochrone.Isochrones
	json2.Unmarshal(rsp(resp), &isochrones)
	assert.Greater(t, len(isochrones), 0)
	assert.Equal(t, isochrones[0].Userid, user.ID)

	// Should find some messages.
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/isochrone/message?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var msgs []message.MessageSummary
	json2.Unmarshal(rsp(resp), &msgs)
	assert.Greater(t, len(msgs), 0)
}
