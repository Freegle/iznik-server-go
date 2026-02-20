package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func TestConvertToStory(t *testing.T) {
	prefix := uniquePrefix("nf_c2s")
	// Create a moderator user
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Create a newsfeed entry by another user
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	nfID := CreateTestNewsfeed(t, posterID, 52.2, -0.1, "My freegle story "+prefix)

	body := fmt.Sprintf(`{"id":%d,"action":"ConvertToStory"}`, nfID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Parse response and verify story ID is returned
	var result map[string]interface{}
	json.Unmarshal(rsp(resp), &result)
	storyID := result["id"]
	assert.NotNil(t, storyID, "Response should contain story id")
	assert.NotZero(t, storyID)

	// Verify the story was created in the database
	db := database.DBConn
	var storyText string
	var storyUserID uint64
	var fromNewsfeed int
	db.Raw("SELECT story, userid, fromnewsfeed FROM users_stories WHERE id = ?", uint64(storyID.(float64))).Row().Scan(&storyText, &storyUserID, &fromNewsfeed)
	assert.Equal(t, "My freegle story "+prefix, storyText)
	assert.Equal(t, posterID, storyUserID)
	assert.Equal(t, 1, fromNewsfeed)
}

func TestConvertToStoryNotMod(t *testing.T) {
	prefix := uniquePrefix("nf_c2s_nm")
	// Create a regular (non-mod) user
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	nfID := CreateTestNewsfeed(t, userID, 52.2, -0.1, "Regular user story "+prefix)

	body := fmt.Sprintf(`{"id":%d,"action":"ConvertToStory"}`, nfID)
	req := httptest.NewRequest("POST", "/api/newsfeed?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestConvertToStoryUnauthorized(t *testing.T) {
	body := `{"id":1,"action":"ConvertToStory"}`
	req := httptest.NewRequest("POST", "/api/newsfeed", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}
