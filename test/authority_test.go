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
