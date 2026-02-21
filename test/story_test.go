package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func createTestStory(t *testing.T, userID uint64) uint64 {
	db := database.DBConn

	result := db.Exec("INSERT INTO users_stories (userid, date, public, headline, story, reviewed) "+
		"VALUES (?, NOW(), 1, 'Test Headline', 'Test story text', 1)", userID)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create story: %v", result.Error)
	}

	var storyID uint64
	db.Raw("SELECT id FROM users_stories WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&storyID)

	if storyID == 0 {
		t.Fatalf("ERROR: Story was created but ID not found")
	}

	return storyID
}

func TestStory(t *testing.T) {
	// Get non-existent story - should return 404
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story/999999999", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestStory_ValidStory(t *testing.T) {
	prefix := uniquePrefix("storyval")
	userID := CreateTestUser(t, prefix, "User")
	storyID := createTestStory(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story/"+fmt.Sprint(storyID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	type StoryResponse struct {
		ID       uint64 `json:"id"`
		Headline string `json:"headline"`
	}
	var story StoryResponse
	json2.Unmarshal(rsp(resp), &story)
	assert.Equal(t, storyID, story.ID)
	assert.Equal(t, "Test Headline", story.Headline)
}

func TestStory_InvalidID(t *testing.T) {
	// Non-integer ID
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story/notanint", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestListStory(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
}

func TestGroupStory(t *testing.T) {
	// Group 0 - should return empty
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story/group/0", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
}

func TestGroupStory_WithData(t *testing.T) {
	// Story is linked to group via user's membership, not a separate table
	prefix := uniquePrefix("storygrp")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	createTestStory(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story/group/"+fmt.Sprint(groupID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.Greater(t, len(ids), 0)
}

func TestStory_V2Path(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/story", nil))
	assert.Equal(t, 200, resp.StatusCode)
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

func TestListStoryNewsletterReviewedFilter(t *testing.T) {
	prefix := uniquePrefix("story_nlrev")
	userID := CreateTestUser(t, prefix, "User")
	db := database.DBConn

	// Create two reviewed+public stories
	nlReviewedID := CreateTestStory(t, userID, "NL Reviewed "+prefix, "newsletter reviewed story", true, true)
	nlNotReviewedID := CreateTestStory(t, userID, "NL Not Reviewed "+prefix, "not newsletter reviewed", true, true)

	// Mark one as newsletter-reviewed
	db.Exec("UPDATE users_stories SET newsletterreviewed = 1 WHERE id = ?", nlReviewedID)

	// Default list (no newsletterreviewed param) should return both
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story?limit=1000", nil))
	assert.Equal(t, 200, resp.StatusCode)
	var allIDs []uint64
	json2.Unmarshal(rsp(resp), &allIDs)
	assert.Contains(t, allIDs, nlReviewedID, "Default should include newsletter-reviewed")
	assert.Contains(t, allIDs, nlNotReviewedID, "Default should include non-newsletter-reviewed")

	// newsletterreviewed=1 should return only newsletter-reviewed stories
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/story?newsletterreviewed=1&limit=1000", nil))
	assert.Equal(t, 200, resp.StatusCode)
	var nlRevIDs []uint64
	json2.Unmarshal(rsp(resp), &nlRevIDs)
	assert.Contains(t, nlRevIDs, nlReviewedID, "newsletterreviewed=1 should include newsletter-reviewed")
	assert.NotContains(t, nlRevIDs, nlNotReviewedID, "newsletterreviewed=1 should exclude non-newsletter-reviewed")

	// newsletterreviewed=0 should return only non-newsletter-reviewed stories
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/story?newsletterreviewed=0&limit=1000", nil))
	assert.Equal(t, 200, resp.StatusCode)
	var nlNotRevIDs []uint64
	json2.Unmarshal(rsp(resp), &nlNotRevIDs)
	assert.Contains(t, nlNotRevIDs, nlNotReviewedID, "newsletterreviewed=0 should include non-newsletter-reviewed")
	assert.NotContains(t, nlNotRevIDs, nlReviewedID, "newsletterreviewed=0 should exclude newsletter-reviewed")
}
