package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDashboardLegacy(t *testing.T) {
	prefix := uniquePrefix("Dashboard")
	_, token := CreateFullTestUser(t, prefix)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/dashboard?jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "dashboard")
	assert.Contains(t, result, "start")
	assert.Contains(t, result, "end")
}

func TestGetDashboardComponents(t *testing.T) {
	prefix := uniquePrefix("DashComp")
	_, token := CreateFullTestUser(t, prefix)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/dashboard?components=RecentCounts,PopularPosts&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "components")

	comps := result["components"].(map[string]interface{})
	assert.Contains(t, comps, "RecentCounts")
	assert.Contains(t, comps, "PopularPosts")
}

func TestGetDashboardRecentCounts(t *testing.T) {
	prefix := uniquePrefix("DashRC")
	_, token := CreateFullTestUser(t, prefix)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/dashboard?components=RecentCounts&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	comps := result["components"].(map[string]interface{})
	rc := comps["RecentCounts"].(map[string]interface{})
	assert.Contains(t, rc, "newmembers")
	assert.Contains(t, rc, "newmessages")
}

func TestGetDashboardModOnlyNotMod(t *testing.T) {
	prefix := uniquePrefix("DashModOnly")
	_, token := CreateFullTestUser(t, prefix)

	// UsersPosting requires moderator - regular user should get nil.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/dashboard?components=UsersPosting&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	comps := result["components"].(map[string]interface{})
	assert.Nil(t, comps["UsersPosting"])
}

func TestGetDashboardTimeSeries(t *testing.T) {
	prefix := uniquePrefix("DashTS")
	_, token := CreateFullTestUser(t, prefix)

	// Activity reads from stats table - may return empty array for test groups.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/dashboard?components=Activity&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	comps := result["components"].(map[string]interface{})
	// Activity should be an array (possibly empty for test data).
	_, ok := comps["Activity"].([]interface{})
	assert.True(t, ok, "Activity should be an array")
}

func TestGetDashboardNoAuth(t *testing.T) {
	// Without auth, should still return success but with limited data.
	req := httptest.NewRequest("GET", "/api/dashboard?components=RecentCounts", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
}

func TestGetDashboardDiscourseTopicsNotMod(t *testing.T) {
	prefix := uniquePrefix("DashDiscNM")
	_, token := CreateFullTestUser(t, prefix)

	// Non-moderator should get nil for DiscourseTopics.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/dashboard?components=DiscourseTopics&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	comps := result["components"].(map[string]interface{})
	assert.Nil(t, comps["DiscourseTopics"])
}

func TestGetDashboardDiscourseTopicsNoConfig(t *testing.T) {
	// A moderator gets nil when DISCOURSE_API/DISCOURSE_APIKEY are not set.
	prefix := uniquePrefix("DashDiscNC")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Moderator")
	_, token := CreateTestSession(t, userID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/dashboard?components=DiscourseTopics&group=%d&jwt=%s", groupID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	comps := result["components"].(map[string]interface{})
	// Without DISCOURSE_API env var, should return nil.
	assert.Nil(t, comps["DiscourseTopics"])
}

func TestGetDashboardV2Path(t *testing.T) {
	req := httptest.NewRequest("GET", "/apiv2/dashboard", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}
