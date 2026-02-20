package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListSimulationRuns(t *testing.T) {
	prefix := uniquePrefix("SimList")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/simulation?action=listruns&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "runs")

	// Runs may be empty, just verify the structure.
	runs := result["runs"].([]interface{})
	assert.NotNil(t, runs)
}

func TestListSimulationRunsNotMod(t *testing.T) {
	prefix := uniquePrefix("SimListNM")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/simulation?action=listruns&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestGetSimulationRun(t *testing.T) {
	prefix := uniquePrefix("SimRun")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	// Try to get a non-existent run.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/simulation?action=getrun&runid=999999999&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestGetSimulationMessage(t *testing.T) {
	prefix := uniquePrefix("SimMsg")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	// Try to get a message from non-existent run.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/simulation?runid=999999999&index=0&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	// Should fail because run doesn't exist / no messages.
	assert.Equal(t, float64(2), result["ret"])
}

func TestSimulationUnauthorized(t *testing.T) {
	// No auth token.
	req := httptest.NewRequest("GET", "/api/simulation?action=listruns", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(1), result["ret"])
}

func TestGetSimulationV2Path(t *testing.T) {
	req := httptest.NewRequest("GET", "/apiv2/simulation?action=listruns", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}
