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
