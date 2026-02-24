package test

import (
	json2 "encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetStatus(t *testing.T) {
	// Write a test status file.
	statusJSON := `{"ret":0,"status":"OK","version":"1.0"}`
	err := os.WriteFile("/tmp/iznik.status", []byte(statusJSON), 0644)
	assert.NoError(t, err)
	defer os.Remove("/tmp/iznik.status")

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/status", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "OK", result["status"])
	assert.Equal(t, "1.0", result["version"])
}

func TestGetStatusMissing(t *testing.T) {
	// Ensure no status file exists.
	os.Remove("/tmp/iznik.status")

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/status", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(1), result["ret"])
	assert.Equal(t, "Cannot access status file", result["status"])
}

func TestGetStatusV2Path(t *testing.T) {
	statusJSON := `{"ret":0,"status":"OK"}`
	err := os.WriteFile("/tmp/iznik.status", []byte(statusJSON), 0644)
	assert.NoError(t, err)
	defer os.Remove("/tmp/iznik.status")

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/status", nil))
	assert.Equal(t, 200, resp.StatusCode)
}
