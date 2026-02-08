package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestStory(t *testing.T) {
	// Get logged out.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story/1", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestListStory(t *testing.T) {
	// Get logged out.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
}

func TestGroupStory(t *testing.T) {
	// Get logged out.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story/group/0", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
}

func TestListStoryReviewedFilter(t *testing.T) {
	prefix := uniquePrefix("story_reviewed")
	userID := CreateTestUser(t, prefix, "User")

	// Create one reviewed+public story and one unreviewed story
	reviewedID := CreateTestStory(t, userID, "Reviewed Story "+prefix, "A reviewed story", true, true)
	unreviewedID := CreateTestStory(t, userID, "Unreviewed Story "+prefix, "An unreviewed story", false, true)

	// Default list (no params) should return only reviewed stories
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story?limit=1000", nil))
	assert.Equal(t, 200, resp.StatusCode)
	var defaultIDs []uint64
	json2.Unmarshal(rsp(resp), &defaultIDs)
	assert.Contains(t, defaultIDs, reviewedID, "Default list should include reviewed stories")
	assert.NotContains(t, defaultIDs, unreviewedID, "Default list should exclude unreviewed stories")

	// With reviewed=0 should return only unreviewed stories
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/story?reviewed=0&limit=1000", nil))
	assert.Equal(t, 200, resp.StatusCode)
	var unreviewedIDs []uint64
	json2.Unmarshal(rsp(resp), &unreviewedIDs)
	assert.Contains(t, unreviewedIDs, unreviewedID, "reviewed=0 should include unreviewed stories")
	assert.NotContains(t, unreviewedIDs, reviewedID, "reviewed=0 should exclude reviewed stories")

	// With reviewed=1 should return only reviewed stories (explicit)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/story?reviewed=1&limit=1000", nil))
	assert.Equal(t, 200, resp.StatusCode)
	var reviewedIDs []uint64
	json2.Unmarshal(rsp(resp), &reviewedIDs)
	assert.Contains(t, reviewedIDs, reviewedID, "reviewed=1 should include reviewed stories")
	assert.NotContains(t, reviewedIDs, unreviewedID, "reviewed=1 should exclude unreviewed stories")
}

func TestListStoryPublicFilter(t *testing.T) {
	prefix := uniquePrefix("story_public")
	userID := CreateTestUser(t, prefix, "User")

	// Create one public and one non-public story (both reviewed)
	publicID := CreateTestStory(t, userID, "Public Story "+prefix, "A public story", true, true)
	privateID := CreateTestStory(t, userID, "Private Story "+prefix, "A private story", true, false)

	// Default list should return only public stories
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story?limit=1000", nil))
	assert.Equal(t, 200, resp.StatusCode)
	var defaultIDs []uint64
	json2.Unmarshal(rsp(resp), &defaultIDs)
	assert.Contains(t, defaultIDs, publicID, "Default list should include public stories")
	assert.NotContains(t, defaultIDs, privateID, "Default list should exclude private stories")

	// With public=0 should return non-public stories
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/story?public=0&limit=1000", nil))
	assert.Equal(t, 200, resp.StatusCode)
	var privateIDs []uint64
	json2.Unmarshal(rsp(resp), &privateIDs)
	assert.Contains(t, privateIDs, privateID, "public=0 should include private stories")
	assert.NotContains(t, privateIDs, publicID, "public=0 should exclude public stories")
}

func TestListStoryGroupReviewedFilter(t *testing.T) {
	prefix := uniquePrefix("story_group_rev")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")

	// Create reviewed and unreviewed stories for this group member
	reviewedID := CreateTestStory(t, userID, "Group Reviewed "+prefix, "reviewed", true, true)
	unreviewedID := CreateTestStory(t, userID, "Group Unreviewed "+prefix, "unreviewed", false, true)

	// Default group list should return only reviewed
	url := fmt.Sprintf("/api/story/group/%d?limit=1000", groupID)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var defaultIDs []uint64
	json2.Unmarshal(rsp(resp), &defaultIDs)
	assert.Contains(t, defaultIDs, reviewedID)
	assert.NotContains(t, defaultIDs, unreviewedID)

	// reviewed=0 should return unreviewed
	url = fmt.Sprintf("/api/story/group/%d?reviewed=0&limit=1000", groupID)
	resp, _ = getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var unreviewedIDs []uint64
	json2.Unmarshal(rsp(resp), &unreviewedIDs)
	assert.Contains(t, unreviewedIDs, unreviewedID)
	assert.NotContains(t, unreviewedIDs, reviewedID)
}
