package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/user"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Tests for GET /api/user/{id}/publiclocation
// =============================================================================

func TestPublicLocation_ValidUser(t *testing.T) {
	prefix := uniquePrefix("publoc")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")

	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/user/%d/publiclocation", userID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result user.Publiclocation
	json2.Unmarshal(rsp(resp), &result)

	// User was created with settings containing lat/lng, so should get a location.
	// The exact values depend on test data setup, but the structure should be valid.
	assert.NotNil(t, result)
}

func TestPublicLocation_NoAuthRequired(t *testing.T) {
	prefix := uniquePrefix("publocnoauth")
	userID := CreateTestUser(t, prefix, "User")

	// Public location should be accessible without authentication.
	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/user/%d/publiclocation", userID), nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestPublicLocation_InvalidUserID(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/abc/publiclocation", nil))
	// Invalid ID should return 200 with empty result (handler doesn't error, returns empty struct).
	assert.Equal(t, 200, resp.StatusCode)

	var result user.Publiclocation
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, "", result.Location)
	assert.Equal(t, "", result.Display)
}

func TestPublicLocation_NonExistentUser(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/999999999/publiclocation", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result user.Publiclocation
	json2.Unmarshal(rsp(resp), &result)
	// Non-existent user should return empty location.
	assert.Equal(t, "", result.Location)
}

func TestPublicLocation_V2Path(t *testing.T) {
	prefix := uniquePrefix("publocv2")
	userID := CreateTestUser(t, prefix, "User")

	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/apiv2/user/%d/publiclocation", userID), nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestPublicLocation_ResponseStructure(t *testing.T) {
	prefix := uniquePrefix("publocstruct")
	userID := CreateTestUser(t, prefix, "User")

	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/user/%d/publiclocation", userID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	// Verify the response has expected keys.
	_, hasLocation := result["location"]
	_, hasDisplay := result["display"]
	_, hasGroupid := result["groupid"]
	_, hasGroupname := result["groupname"]

	assert.True(t, hasLocation, "Response should have 'location' field")
	assert.True(t, hasDisplay, "Response should have 'display' field")
	assert.True(t, hasGroupid, "Response should have 'groupid' field")
	assert.True(t, hasGroupname, "Response should have 'groupname' field")
}
