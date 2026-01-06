package test

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientLogEndpoint(t *testing.T) {
	// Test the clientlog endpoint with valid log data
	body := `{"logs":[{"timestamp":"2024-01-01T00:00:00Z","level":"info","message":"Test log message","trace_id":"test-trace-123","session_id":"test-session-456","url":"https://example.com","user_agent":"TestAgent/1.0"}]}`

	req := httptest.NewRequest("POST", "/api/clientlog", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	// The endpoint always returns 204 No Content
	assert.Equal(t, 204, resp.StatusCode, "Should return 204 No Content")
}

func TestClientLogEndpointWithEmptyLogs(t *testing.T) {
	// Test with empty logs array
	body := `{"logs":[]}`

	req := httptest.NewRequest("POST", "/api/clientlog", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 204, resp.StatusCode, "Should return 204 even with empty logs")
}

func TestClientLogEndpointWithInvalidJSON(t *testing.T) {
	// Test with invalid JSON - should still return 204 (fire and forget)
	body := `{invalid json}`

	req := httptest.NewRequest("POST", "/api/clientlog", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 204, resp.StatusCode, "Should return 204 even with invalid JSON")
}

func TestClientLogEndpointWithEmptyBody(t *testing.T) {
	// Test with empty body
	req := httptest.NewRequest("POST", "/api/clientlog", nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 204, resp.StatusCode, "Should return 204 even with empty body")
}

func TestClientLogEndpointWithMultipleLogs(t *testing.T) {
	// Test with multiple log entries
	body := `{"logs":[
		{"timestamp":"2024-01-01T00:00:00Z","level":"info","message":"First log","event_type":"page_view","page_name":"home"},
		{"timestamp":"2024-01-01T00:00:01Z","level":"error","message":"Second log","event_type":"error"},
		{"timestamp":"2024-01-01T00:00:02Z","level":"debug","message":"Third log"}
	]}`

	req := httptest.NewRequest("POST", "/api/clientlog", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 204, resp.StatusCode, "Should return 204 with multiple logs")
}

func TestClientLogEndpointWithTraceHeaders(t *testing.T) {
	// Test with trace headers set
	body := `{"logs":[{"timestamp":"2024-01-01T00:00:00Z","level":"info","message":"Log with headers"}]}`

	req := httptest.NewRequest("POST", "/api/clientlog", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-ID", "header-trace-id")
	req.Header.Set("X-Session-ID", "header-session-id")

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 204, resp.StatusCode, "Should return 204 with trace headers")
}

func TestClientLogEndpointWithAuthenticatedUser(t *testing.T) {
	prefix := uniquePrefix("ClientLog")

	// Create a user with a session
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{"logs":[{"timestamp":"2024-01-01T00:00:00Z","level":"info","message":"Authenticated user log"}]}`

	req := httptest.NewRequest("POST", "/api/clientlog?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 204, resp.StatusCode, "Should return 204 for authenticated user")
}

func TestClientLogEndpointV2(t *testing.T) {
	// Test the v2 endpoint
	body := `{"logs":[{"timestamp":"2024-01-01T00:00:00Z","level":"info","message":"V2 log message"}]}`

	req := httptest.NewRequest("POST", "/apiv2/clientlog", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 204, resp.StatusCode, "V2 endpoint should also return 204")
}

func TestClientLogEndpointWithAPIRequestLog(t *testing.T) {
	// Test with API request log event type
	body := `{"logs":[{
		"timestamp":"2024-01-01T00:00:00Z",
		"level":"info",
		"message":"API request completed",
		"event_type":"api_request",
		"method":"GET",
		"path":"/api/message/123",
		"duration_ms":150,
		"status":200
	}]}`

	req := httptest.NewRequest("POST", "/api/clientlog", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getApp().Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 204, resp.StatusCode, "Should handle API request logs")
}
