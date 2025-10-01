package test

import (
	json2 "encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDonations(t *testing.T) {
	// Test without groupid - should return default target and current month's raised amount
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/donations", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	// Should have target and raised fields
	assert.Contains(t, result, "target")
	assert.Contains(t, result, "raised")

	// Target should be the default (2000 unless DONATION_TARGET env var is set)
	target, ok := result["target"].(float64)
	assert.True(t, ok, "target should be a number")
	assert.Greater(t, target, float64(0), "target should be positive")

	// Raised should be a non-negative number
	raised, ok := result["raised"].(float64)
	assert.True(t, ok, "raised should be a number")
	assert.GreaterOrEqual(t, raised, float64(0), "raised should be >= 0")
}

func TestGetDonationsWithGroupID(t *testing.T) {
	// Test with a valid group ID
	// Note: This will return 0 for raised if no donations exist for this group
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/donations?groupid=1", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	assert.Contains(t, result, "target")
	assert.Contains(t, result, "raised")

	// Raised should be a valid number
	raised, ok := result["raised"].(float64)
	assert.True(t, ok, "raised should be a number")
	assert.GreaterOrEqual(t, raised, float64(0), "raised should be >= 0")
}

func TestGetDonationsInvalidGroupID(t *testing.T) {
	// Test with non-existent group ID - should fall back to default target
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/donations?groupid=999999", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	assert.Contains(t, result, "target")
	assert.Contains(t, result, "raised")

	// Should fall back to default target when group not found
	target, ok := result["target"].(float64)
	assert.True(t, ok, "target should be a number")
	assert.Greater(t, target, float64(0), "target should be positive (falls back to default)")

	// Raised should be 0 for non-existent group
	assert.Equal(t, float64(0), result["raised"])
}
