package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func createTestUnreviewedStory(t *testing.T, userID uint64) uint64 {
	db := database.DBConn
	db.Exec("INSERT INTO users_stories (userid, headline, story, public, reviewed) VALUES (?, 'Test Headline', 'Test Story Body', 0, 0)", userID)
	var id uint64
	db.Raw("SELECT id FROM users_stories WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&id)
	assert.Greater(t, id, uint64(0))
	return id
}

func TestStoryCreate(t *testing.T) {
	prefix := uniquePrefix("storywr_create")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{"headline":"My Freegle Story","story":"I gave away a sofa and it was great!","public":true}`
	req := httptest.NewRequest("PUT", "/api/story?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Greater(t, result["id"], float64(0))

	// Verify in DB
	db := database.DBConn
	var headline string
	db.Raw("SELECT headline FROM users_stories WHERE id = ?", uint64(result["id"].(float64))).Scan(&headline)
	assert.Equal(t, "My Freegle Story", headline)
}

func TestStoryCreateUnauthorized(t *testing.T) {
	body := `{"headline":"Test","story":"Test story"}`
	req := httptest.NewRequest("PUT", "/api/story", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestStoryUpdate(t *testing.T) {
	prefix := uniquePrefix("storywr_upd")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	storyID := createTestUnreviewedStory(t, userID)

	// Owner updates headline
	headline := "Updated Headline"
	body := fmt.Sprintf(`{"id":%d,"headline":"%s"}`, storyID, headline)
	req := httptest.NewRequest("PATCH", "/api/story?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify
	db := database.DBConn
	var dbHeadline string
	db.Raw("SELECT headline FROM users_stories WHERE id = ?", storyID).Scan(&dbHeadline)
	assert.Equal(t, "Updated Headline", dbHeadline)
}

func TestStoryUpdateUnauthorized(t *testing.T) {
	body := `{"id":1,"headline":"Hacked"}`
	req := httptest.NewRequest("PATCH", "/api/story", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestStoryUpdateNonOwner(t *testing.T) {
	prefix := uniquePrefix("storywr_noown")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, otherToken := CreateTestSession(t, otherID)
	storyID := createTestUnreviewedStory(t, ownerID)

	body := fmt.Sprintf(`{"id":%d,"headline":"Hacked"}`, storyID)
	req := httptest.NewRequest("PATCH", "/api/story?jwt="+otherToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestStoryUpdateByModerator(t *testing.T) {
	prefix := uniquePrefix("storywr_mod")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)
	storyID := createTestUnreviewedStory(t, ownerID)

	// Moderator reviews and approves for publicity
	body := fmt.Sprintf(`{"id":%d,"reviewed":1,"public":true}`, storyID)
	req := httptest.NewRequest("PATCH", "/api/story?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify reviewed and public
	db := database.DBConn
	var reviewed, public int
	db.Raw("SELECT reviewed, public FROM users_stories WHERE id = ?", storyID).Row().Scan(&reviewed, &public)
	assert.Equal(t, 1, reviewed)
	assert.Equal(t, 1, public)
}

func TestStoryUpdateByAdmin(t *testing.T) {
	prefix := uniquePrefix("storywr_admin")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)
	storyID := createTestUnreviewedStory(t, ownerID)

	body := fmt.Sprintf(`{"id":%d,"reviewed":1,"public":true}`, storyID)
	req := httptest.NewRequest("PATCH", "/api/story?jwt="+adminToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestStoryUpdateNewsletterReview(t *testing.T) {
	prefix := uniquePrefix("storywr_nl")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)
	storyID := createTestUnreviewedStory(t, ownerID)

	// Approve for newsletter
	body := fmt.Sprintf(`{"id":%d,"newsletterreviewed":1,"newsletter":1}`, storyID)
	req := httptest.NewRequest("PATCH", "/api/story?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	db := database.DBConn
	var nlReviewed, nl int
	db.Raw("SELECT newsletterreviewed, newsletter FROM users_stories WHERE id = ?", storyID).Row().Scan(&nlReviewed, &nl)
	assert.Equal(t, 1, nlReviewed)
	assert.Equal(t, 1, nl)
}

func TestStoryUpdateMissingID(t *testing.T) {
	prefix := uniquePrefix("storywr_noid")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{"headline":"No ID"}`
	req := httptest.NewRequest("PATCH", "/api/story?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestStoryLike(t *testing.T) {
	prefix := uniquePrefix("storywr_like")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	storyID := createTestUnreviewedStory(t, userID)

	body := fmt.Sprintf(`{"id":%d}`, storyID)
	req := httptest.NewRequest("POST", "/api/story/like?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify like in DB
	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_stories_likes WHERE storyid = ? AND userid = ?", storyID, userID).Scan(&count)
	assert.Equal(t, int64(1), count)

	// Like again - should be idempotent (INSERT IGNORE)
	req2 := httptest.NewRequest("POST", "/api/story/like?jwt="+token, bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req2)
	assert.Equal(t, 200, resp.StatusCode)

	db.Raw("SELECT COUNT(*) FROM users_stories_likes WHERE storyid = ? AND userid = ?", storyID, userID).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestStoryLikeUnauthorized(t *testing.T) {
	body := `{"id":1}`
	req := httptest.NewRequest("POST", "/api/story/like", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestStoryLikeMissingID(t *testing.T) {
	prefix := uniquePrefix("storywr_likenoid")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{}`
	req := httptest.NewRequest("POST", "/api/story/like?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestStoryUnlike(t *testing.T) {
	prefix := uniquePrefix("storywr_unlike")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	storyID := createTestUnreviewedStory(t, userID)

	// Like first
	db := database.DBConn
	db.Exec("INSERT INTO users_stories_likes (storyid, userid) VALUES (?, ?)", storyID, userID)

	// Then unlike
	body := fmt.Sprintf(`{"id":%d}`, storyID)
	req := httptest.NewRequest("POST", "/api/story/unlike?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var count int64
	db.Raw("SELECT COUNT(*) FROM users_stories_likes WHERE storyid = ? AND userid = ?", storyID, userID).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestStoryUnlikeUnauthorized(t *testing.T) {
	body := `{"id":1}`
	req := httptest.NewRequest("POST", "/api/story/unlike", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestStoryDelete(t *testing.T) {
	prefix := uniquePrefix("storywr_del")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	storyID := createTestUnreviewedStory(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/story/%d?jwt=%s", storyID, token), nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Verify deleted from DB
	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_stories WHERE id = ?", storyID).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestStoryDeleteUnauthorized(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("DELETE", "/api/story/1", nil))
	assert.Equal(t, 401, resp.StatusCode)
}

func TestStoryDeleteNonOwner(t *testing.T) {
	prefix := uniquePrefix("storywr_dno")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, otherToken := CreateTestSession(t, otherID)
	storyID := createTestUnreviewedStory(t, ownerID)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/story/%d?jwt=%s", storyID, otherToken), nil))
	assert.Equal(t, 403, resp.StatusCode)
}

func TestStoryDeleteByModerator(t *testing.T) {
	prefix := uniquePrefix("storywr_dmod")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)
	storyID := createTestUnreviewedStory(t, ownerID)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/story/%d?jwt=%s", storyID, modToken), nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestStoryDeleteInvalidID(t *testing.T) {
	prefix := uniquePrefix("storywr_dinv")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", "/api/story/notanumber?jwt="+token, nil))
	assert.Equal(t, 400, resp.StatusCode)
}

func TestStoryV2Path(t *testing.T) {
	prefix := uniquePrefix("storywr_v2")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Create via v2 path
	body := `{"headline":"V2 Story","story":"Testing V2 path","public":false}`
	req := httptest.NewRequest("PUT", "/apiv2/story?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Greater(t, result["id"], float64(0))
}
