package test

import (
	json2 "encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func TestLogo(t *testing.T) {
	// Test GET /api/logo - should return 200 regardless of whether a logo exists
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/logo", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	// Should always have a date field in m-d format
	assert.NotNil(t, result["date"])
	date, ok := result["date"].(string)
	assert.True(t, ok, "date should be a string")
	assert.NotEmpty(t, date)

	// Logo may be nil if no logo exists for today
	// But the structure should still be valid
	_, hasLogo := result["logo"]
	assert.True(t, hasLogo, "response should have logo field (may be nil)")
}

func TestLogoWithStaleJWT(t *testing.T) {
	// A public endpoint should return 200 even if the request includes a JWT
	// for a deleted/expired session. The auth middleware must not override the
	// handler's success response with 401.
	uid := CreateTestUser(t, "stalelogo", "User")
	token := getToken(t, uid)

	// Delete the session to make the JWT stale.
	db := database.DBConn
	db.Exec("DELETE FROM sessions WHERE userid = ?", uid)

	req := httptest.NewRequest("GET", "/api/logo?jwt="+token, nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode, "Public endpoint should not return 401 for stale JWT")
}

func TestAuthEndpointWithStaleJWT(t *testing.T) {
	// An auth-requiring endpoint should still return 401 with a stale JWT.
	uid := CreateTestUser(t, "staleauth", "User")
	token := getToken(t, uid)

	// Delete the session to make the JWT stale.
	db := database.DBConn
	db.Exec("DELETE FROM sessions WHERE userid = ?", uid)

	// POST /newsfeed requires auth (calls WhoAmI).
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, nil)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode, "Auth endpoint should return 401 for stale JWT")
}

func TestLogoV2(t *testing.T) {
	// Test GET /apiv2/logo - should work the same as /api/logo
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/logo", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	// Should always have a date field
	assert.NotNil(t, result["date"])

	// Should have logo field (may be nil)
	_, hasLogo := result["logo"]
	assert.True(t, hasLogo, "response should have logo field (may be nil)")
}
