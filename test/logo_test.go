package test

import (
	json2 "encoding/json"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
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
