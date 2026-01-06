package test

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthorityMessagesEndpoint(t *testing.T) {
	// Test the authority messages endpoint with a non-existent authority ID
	// This tests the route is set up correctly and returns an empty array
	req := httptest.NewRequest("GET", "/api/authority/999999999/message", nil)

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode, "Should return 200 even for non-existent authority")

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Should return empty array for non-existent authority
	assert.Equal(t, "[]", bodyStr, "Should return empty array for non-existent authority")
}

func TestAuthorityMessagesEndpointWithZeroId(t *testing.T) {
	// Test with default ID (0) which should also return empty array
	req := httptest.NewRequest("GET", "/api/authority/0/message", nil)

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	assert.Equal(t, "[]", bodyStr, "Should return empty array for authority ID 0")
}

func TestAuthorityMessagesEndpointWithInvalidId(t *testing.T) {
	// Test with invalid ID format
	req := httptest.NewRequest("GET", "/api/authority/invalid/message", nil)

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	// Invalid ID should be treated as 0 by Go's strconv
	assert.Equal(t, 200, resp.StatusCode)
}

func TestAuthorityMessagesEndpointV2(t *testing.T) {
	// Test the v2 endpoint as well
	req := httptest.NewRequest("GET", "/apiv2/authority/999999999/message", nil)

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	assert.Equal(t, "[]", bodyStr, "V2 endpoint should also return empty array")
}

func TestAuthoritySingleEndpoint(t *testing.T) {
	// Test the single authority endpoint with a non-existent authority ID
	req := httptest.NewRequest("GET", "/api/authority/999999999", nil)

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode, "Should return 404 for non-existent authority")
}

func TestAuthoritySingleEndpointV2(t *testing.T) {
	// Test the v2 single authority endpoint
	req := httptest.NewRequest("GET", "/apiv2/authority/999999999", nil)

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode, "V2 should return 404 for non-existent authority")
}

func TestAuthoritySingleEndpointInvalidId(t *testing.T) {
	// Test with invalid ID format
	req := httptest.NewRequest("GET", "/api/authority/invalid", nil)

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 400, resp.StatusCode, "Should return 400 for invalid ID")
}

func TestAuthoritySearchEndpoint(t *testing.T) {
	// Test the authority search endpoint with a search term
	req := httptest.NewRequest("GET", "/api/authority?search=London", nil)

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode, "Search should return 200")

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Should return a JSON array (may be empty if no London authorities exist in test DB)
	assert.True(t, bodyStr[0] == '[', "Should return JSON array")
}

func TestAuthoritySearchEndpointV2(t *testing.T) {
	// Test the v2 search endpoint
	req := httptest.NewRequest("GET", "/apiv2/authority?search=Manchester", nil)

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode, "V2 search should return 200")

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	assert.True(t, bodyStr[0] == '[', "V2 should return JSON array")
}

func TestAuthoritySearchEndpointNoSearch(t *testing.T) {
	// Test search endpoint without search parameter
	req := httptest.NewRequest("GET", "/api/authority", nil)

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 400, resp.StatusCode, "Should return 400 when search term is missing")
}

func TestAuthoritySearchEndpointWithLimit(t *testing.T) {
	// Test search endpoint with limit parameter
	req := httptest.NewRequest("GET", "/api/authority?search=Council&limit=5", nil)

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode, "Search with limit should return 200")

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	assert.True(t, bodyStr[0] == '[', "Should return JSON array")
}

func TestAuthoritySearchEndpointEmptyResult(t *testing.T) {
	// Test search endpoint with a term that shouldn't match anything
	req := httptest.NewRequest("GET", "/api/authority?search=XYZNONEXISTENT123", nil)

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode, "Search with no matches should return 200")

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	assert.Equal(t, "[]", bodyStr, "Should return empty array for no matches")
}

func TestAuthoritySingleWithStatsNonExistent(t *testing.T) {
	// Test the single authority endpoint with stats parameter for non-existent authority
	// Should return 404 since authority doesn't exist
	req := httptest.NewRequest("GET", "/api/authority/999999999?stats=true", nil)

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode, "Should return 404 for non-existent authority even with stats")
}

func TestAuthoritySingleWithStatsV2(t *testing.T) {
	// Test the v2 single authority endpoint with stats parameter
	// URL-encode spaces in the date parameters
	req := httptest.NewRequest("GET", "/apiv2/authority/999999999?stats=true&start=30%20days%20ago&end=today", nil)

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode, "V2 should return 404 for non-existent authority with stats")
}

func TestAuthoritySingleWithStatsDateParams(t *testing.T) {
	// Test stats with custom date parameters
	req := httptest.NewRequest("GET", "/api/authority/999999999?stats=1&start=2024-01-01&end=2024-12-31", nil)

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode, "Should return 404 for non-existent authority with date params")
}

func TestAuthoritySingleWithoutStats(t *testing.T) {
	// Test that stats are not included when not requested (for a non-existent authority)
	req := httptest.NewRequest("GET", "/api/authority/999999999?stats=false", nil)

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode, "Should return 404 for non-existent authority")
}
