package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/job"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestJobs(t *testing.T) {
	// Create a job at specific coordinates for this test
	lat := 52.5833189
	lng := -2.0455619
	jobID := CreateTestJob(t, lat, lng)

	// Query for jobs near those coordinates
	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/job?lat=%f&lng=%f", lat, lng), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var jobs []job.Job
	json2.Unmarshal(rsp(resp), &jobs)
	assert.Greater(t, len(jobs), 0)

	// Get the specific job we created
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/job/"+fmt.Sprint(jobID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Non-existent job should return 404
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/job/0", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestJobs_InvalidID(t *testing.T) {
	// Non-integer job ID
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/job/notanint", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestJobs_WithoutCoords(t *testing.T) {
	// No lat/lng params - should still return 200 (defaults to 0,0)
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/job", nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestJobs_V2Path(t *testing.T) {
	lat := 52.5833189
	lng := -2.0455619
	CreateTestJob(t, lat, lng)

	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/apiv2/job?lat=%f&lng=%f", lat, lng), nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestJobClick(t *testing.T) {
	// Create a job for this test
	jobID := CreateTestJob(t, 52.5833189, -2.0455619)

	// Record a click without authentication (anonymous user)
	resp, _ := getApp().Test(httptest.NewRequest("POST", fmt.Sprintf("/api/job?id=%d&link=http://example.com/job", jobID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])

	// Test with missing job ID - still returns success (matches PHP behavior)
	resp, _ = getApp().Test(httptest.NewRequest("POST", "/api/job", nil))
	assert.Equal(t, 200, resp.StatusCode)
}
