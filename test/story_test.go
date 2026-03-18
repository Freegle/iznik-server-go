package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
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
	// Get non-existent story - should return 404.
	// Use a high ID to avoid collision with fixture data from testenv.php.
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

func TestPutStory(t *testing.T) {
	prefix := uniquePrefix("stwr_put")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{"public":true,"headline":"My Test Headline","story":"This is my test story about freegling."}`
	req := httptest.NewRequest("PUT", "/api/story?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])
	assert.Greater(t, result["id"], float64(0))

	// Verify in DB
	db := database.DBConn
	storyID := uint64(result["id"].(float64))
	var headline string
	var storyText string
	var public int
	var userid uint64
	db.Raw("SELECT headline, story, public, userid FROM users_stories WHERE id = ?", storyID).Row().Scan(&headline, &storyText, &public, &userid)
	assert.Equal(t, "My Test Headline", headline)
	assert.Equal(t, "This is my test story about freegling.", storyText)
	assert.Equal(t, 1, public)
	assert.Equal(t, userID, userid)
}

func TestPutStoryNotLoggedIn(t *testing.T) {
	body := `{"public":true,"headline":"Test","story":"Test story"}`
	req := httptest.NewRequest("PUT", "/api/story", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestPostStoryLike(t *testing.T) {
	prefix := uniquePrefix("stwr_like")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	storyID := CreateTestStory(t, userID, "Like Test "+prefix, "A story to like", true, true)

	body := fmt.Sprintf(`{"id":%d,"action":"Like"}`, storyID)
	req := httptest.NewRequest("POST", "/api/story?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])

	// Verify like exists in DB
	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_stories_likes WHERE storyid = ? AND userid = ?", storyID, userID).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestPostStoryUnlike(t *testing.T) {
	prefix := uniquePrefix("stwr_unlike")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	storyID := CreateTestStory(t, userID, "Unlike Test "+prefix, "A story to unlike", true, true)

	// First like the story
	db := database.DBConn
	db.Exec("INSERT INTO users_stories_likes (storyid, userid) VALUES (?, ?)", storyID, userID)

	// Verify the like exists
	var countBefore int64
	db.Raw("SELECT COUNT(*) FROM users_stories_likes WHERE storyid = ? AND userid = ?", storyID, userID).Scan(&countBefore)
	assert.Equal(t, int64(1), countBefore)

	// Unlike
	body := fmt.Sprintf(`{"id":%d,"action":"Unlike"}`, storyID)
	req := httptest.NewRequest("POST", "/api/story?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify like removed from DB
	var countAfter int64
	db.Raw("SELECT COUNT(*) FROM users_stories_likes WHERE storyid = ? AND userid = ?", storyID, userID).Scan(&countAfter)
	assert.Equal(t, int64(0), countAfter)
}

func TestPatchStory(t *testing.T) {
	prefix := uniquePrefix("stwr_patch")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	storyID := CreateTestStory(t, ownerID, "Patch Test "+prefix, "A story to patch", false, false)

	// Update headline and story text
	body := fmt.Sprintf(`{"id":%d,"headline":"Updated Headline","story":"Updated story text"}`, storyID)
	req := httptest.NewRequest("PATCH", "/api/story?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])

	// Verify updates in DB
	db := database.DBConn
	var headline string
	var storyText string
	db.Raw("SELECT headline, story FROM users_stories WHERE id = ?", storyID).Row().Scan(&headline, &storyText)
	assert.Equal(t, "Updated Headline", headline)
	assert.Equal(t, "Updated story text", storyText)
}

func TestPatchStoryNotMod(t *testing.T) {
	prefix := uniquePrefix("stwr_patchnm")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	// other user is NOT a mod and NOT in the same group
	_, otherToken := CreateTestSession(t, otherID)

	storyID := CreateTestStory(t, ownerID, "NoMod Test "+prefix, "Should not be editable", true, true)

	body := fmt.Sprintf(`{"id":%d,"headline":"Hacked"}`, storyID)
	req := httptest.NewRequest("PATCH", "/api/story?jwt="+otherToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)

	// Verify headline was NOT changed
	db := database.DBConn
	var headline string
	db.Raw("SELECT headline FROM users_stories WHERE id = ?", storyID).Scan(&headline)
	assert.Equal(t, "NoMod Test "+prefix, headline)
}

func TestPatchStoryMakesPublic(t *testing.T) {
	prefix := uniquePrefix("stwr_patchpub")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Set a location on the owner so the newsfeed entry can be created with a position
	db := database.DBConn
	var locationID uint64
	db.Raw("SELECT id FROM locations LIMIT 1").Scan(&locationID)
	if locationID > 0 {
		db.Exec("UPDATE users SET lastlocation = ? WHERE id = ?", locationID, ownerID)
	}

	// Create story that is NOT reviewed and NOT public
	storyID := CreateTestStory(t, ownerID, "Going Public "+prefix, "A story becoming public", false, false)

	// Set both reviewed and public to true in one PATCH
	body := fmt.Sprintf(`{"id":%d,"reviewed":true,"public":true}`, storyID)
	req := httptest.NewRequest("PATCH", "/api/story?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify a newsfeed entry of type 'Story' was created for this story
	var nfCount int64
	db.Raw("SELECT COUNT(*) FROM newsfeed WHERE type = 'Story' AND storyid = ? AND userid = ?", storyID, ownerID).Scan(&nfCount)
	assert.Equal(t, int64(1), nfCount, "Expected a newsfeed entry to be created when story becomes reviewed+public")
}

func TestPatchStoryMakesPublicNotFromNewsfeed(t *testing.T) {
	prefix := uniquePrefix("stwr_patchnf")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Create story that came from newsfeed (fromnewsfeed = 1)
	db := database.DBConn
	db.Exec("INSERT INTO users_stories (userid, headline, story, reviewed, public, date, fromnewsfeed) "+
		"VALUES (?, ?, ?, 0, 0, NOW(), 1)",
		ownerID, "From NF "+prefix, "Originally from newsfeed")

	var storyID uint64
	db.Raw("SELECT id FROM users_stories WHERE userid = ? AND headline = ? ORDER BY id DESC LIMIT 1",
		ownerID, "From NF "+prefix).Scan(&storyID)
	assert.NotZero(t, storyID)

	// Set reviewed and public to true
	body := fmt.Sprintf(`{"id":%d,"reviewed":true,"public":true}`, storyID)
	req := httptest.NewRequest("PATCH", "/api/story?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify NO newsfeed entry was created (since fromnewsfeed is set)
	var nfCount int64
	db.Raw("SELECT COUNT(*) FROM newsfeed WHERE type = 'Story' AND storyid = ? AND userid = ?", storyID, ownerID).Scan(&nfCount)
	assert.Equal(t, int64(0), nfCount, "Should NOT create newsfeed entry for story that came from newsfeed")
}

func TestDeleteStory(t *testing.T) {
	prefix := uniquePrefix("stwr_del")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	storyID := CreateTestStory(t, ownerID, "Delete Test "+prefix, "A story to delete", true, true)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/story/%d?jwt=%s", storyID, modToken), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])

	// Verify deleted from DB
	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_stories WHERE id = ?", storyID).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestDeleteStoryNotMod(t *testing.T) {
	prefix := uniquePrefix("stwr_delnm")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	// other user is NOT a mod and NOT in the same group
	_, otherToken := CreateTestSession(t, otherID)

	storyID := CreateTestStory(t, ownerID, "NodeleteMod Test "+prefix, "Should not be deletable", true, true)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/story/%d?jwt=%s", storyID, otherToken), nil))
	assert.Equal(t, 403, resp.StatusCode)

	// Verify NOT deleted
	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_stories WHERE id = ?", storyID).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestDeleteStoryUnauthorized(t *testing.T) {
	prefix := uniquePrefix("stwr_delua")
	userID := CreateTestUser(t, prefix, "User")
	storyID := CreateTestStory(t, userID, "Unauth Del "+prefix, "Should not be deletable", true, true)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/story/%d", storyID), nil))
	assert.Equal(t, 401, resp.StatusCode)
}

func TestPostStoryLikeNotLoggedIn(t *testing.T) {
	body := `{"id":1,"action":"Like"}`
	req := httptest.NewRequest("POST", "/api/story", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestPatchStoryUnauthorized(t *testing.T) {
	body := `{"id":1,"headline":"Hacked"}`
	req := httptest.NewRequest("PATCH", "/api/story", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}
