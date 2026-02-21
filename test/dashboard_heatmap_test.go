package test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDashboardHeatmap(t *testing.T) {
	// Heatmap does not require auth - it returns location data for public display.
	req := httptest.NewRequest("GET", "/api/dashboard?heatmap=true", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])

	// Verify the heatmap key is present (may be empty array in test DB)
	_, hasHeatmap := result["heatmap"]
	assert.True(t, hasHeatmap, "Response should contain heatmap key")
}

func TestDashboardHeatmapWithData(t *testing.T) {
	// Create some spatial data, then verify it appears in the heatmap.
	prefix := uniquePrefix("DashHM")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")

	// CreateTestMessage inserts into messages_spatial with successful=1 and arrival=NOW()
	CreateTestMessage(t, userID, groupID, "Heatmap test "+prefix, 52.2, -0.1)

	req := httptest.NewRequest("GET", "/api/dashboard?heatmap=true", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Heatmap should be an array with at least one entry
	heatmap, ok := result["heatmap"].([]interface{})
	assert.True(t, ok, "heatmap should be an array")
	assert.Greater(t, len(heatmap), 0, "heatmap should have at least one entry from test data")

	// Verify each entry has lat and lng
	if len(heatmap) > 0 {
		point := heatmap[0].(map[string]interface{})
		_, hasLat := point["lat"]
		_, hasLng := point["lng"]
		assert.True(t, hasLat, "heatmap point should have lat")
		assert.True(t, hasLng, "heatmap point should have lng")
	}
}
