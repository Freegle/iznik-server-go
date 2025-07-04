package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/config"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
	"time"
)

func TestConfig(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/config/wibble", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var results []config.ConfigItem
	json2.Unmarshal(rsp(resp), &results)
	assert.Equal(t, len(results), 0)
}

// Helper function to get or create a Support/Admin user
func getSupportUser(t *testing.T) (uint64, string) {
	db := database.DBConn

	// Try to find an existing Support or Admin user
	var userID uint64
	db.Raw("SELECT id FROM users WHERE systemrole IN ('Support', 'Admin') AND deleted IS NULL LIMIT 1").Scan(&userID)

	if userID == 0 {
		// Create a test Support user
		testUser := user.User{
			Firstname:  stringPtr("Test"),
			Lastname:   stringPtr("Support"),
			Systemrole: "Support",
			Lastaccess: time.Now(),
			Added:      time.Now(),
		}
		db.Create(&testUser)
		userID = testUser.ID
	}

	token := getToken(t, userID)
	return userID, token
}

// Helper function to get a regular user
func getRegularUser(t *testing.T) (uint64, string) {
	db := database.DBConn

	// Find a regular user
	var userID uint64
	db.Raw("SELECT id FROM users WHERE systemrole = 'User' AND deleted IS NULL LIMIT 1").Scan(&userID)

	if userID == 0 {
		t.Skip("No regular user found for testing")
	}

	token := getToken(t, userID)
	return userID, token
}

func stringPtr(s string) *string {
	return &s
}

// Test Spam Keywords endpoints

func TestSpamKeywords_Unauthorized(t *testing.T) {
	// Test without authentication
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/config/admin/spam_keywords", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Test with regular user (should be forbidden)
	_, regularToken := getRegularUser(t)
	req := httptest.NewRequest("GET", "/api/config/admin/spam_keywords", nil)
	req.Header.Set("Authorization", regularToken)
	resp, _ = getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestSpamKeywords_List(t *testing.T) {
	_, token := getSupportUser(t)

	req := httptest.NewRequest("GET", "/api/config/admin/spam_keywords", nil)
	req.Header.Set("Authorization", token)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var keywords []config.SpamKeyword
	json2.Unmarshal(rsp(resp), &keywords)
	// Should return an array (might be empty)
	assert.NotNil(t, keywords)
}

func TestSpamKeywords_Create(t *testing.T) {
	_, token := getSupportUser(t)

	// Test valid creation
	keywordReq := config.CreateSpamKeywordRequest{
		Word:   "testspam",
		Action: "Spam",
		Type:   "Literal",
	}

	body, _ := json2.Marshal(keywordReq)
	req := httptest.NewRequest("POST", "/api/config/admin/spam_keywords", bytes.NewReader(body))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 201, resp.StatusCode)

	var keyword config.SpamKeyword
	json2.Unmarshal(rsp(resp), &keyword)
	assert.Equal(t, "testspam", keyword.Word)
	assert.Equal(t, "Spam", keyword.Action)
	assert.Equal(t, "Literal", keyword.Type)
	assert.Greater(t, keyword.ID, uint64(0))

	// Clean up
	db := database.DBConn
	db.Delete(&config.SpamKeyword{}, keyword.ID)
}

func TestSpamKeywords_CreateValidation(t *testing.T) {
	_, token := getSupportUser(t)

	// Test invalid action
	keywordReq := config.CreateSpamKeywordRequest{
		Word:   "testspam",
		Action: "Invalid",
		Type:   "Literal",
	}

	body, _ := json2.Marshal(keywordReq)
	req := httptest.NewRequest("POST", "/api/config/admin/spam_keywords", bytes.NewReader(body))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)

	// Test empty word
	keywordReq = config.CreateSpamKeywordRequest{
		Word:   "",
		Action: "Spam",
		Type:   "Literal",
	}

	body, _ = json2.Marshal(keywordReq)
	req = httptest.NewRequest("POST", "/api/config/spam_keywords", bytes.NewReader(body))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestSpamKeywords_Delete(t *testing.T) {
	_, token := getSupportUser(t)
	db := database.DBConn

	// Create a test keyword
	keyword := config.SpamKeyword{
		Word:   "testdelete",
		Action: "Review",
		Type:   "Literal",
	}
	db.Create(&keyword)

	// Delete it
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/config/admin/spam_keywords/%d", keyword.ID), nil)
	req.Header.Set("Authorization", token)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 204, resp.StatusCode)

	// Verify it's deleted
	var count int64
	db.Model(&config.SpamKeyword{}).Where("id = ?", keyword.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestSpamKeywords_DeleteNotFound(t *testing.T) {
	_, token := getSupportUser(t)

	// Try to delete non-existent keyword
	req := httptest.NewRequest("DELETE", "/api/config/admin/spam_keywords/999999", nil)
	req.Header.Set("Authorization", token)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 404, resp.StatusCode)
}

// Test Worry Words endpoints

func TestWorryWords_Unauthorized(t *testing.T) {
	// Test without authentication
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/config/admin/worry_words", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Test with regular user (should be forbidden)
	_, regularToken := getRegularUser(t)
	req := httptest.NewRequest("GET", "/api/config/admin/worry_words", nil)
	req.Header.Set("Authorization", regularToken)
	resp, _ = getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestWorryWords_List(t *testing.T) {
	_, token := getSupportUser(t)

	req := httptest.NewRequest("GET", "/api/config/admin/worry_words", nil)
	req.Header.Set("Authorization", token)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var words []config.WorryWord
	json2.Unmarshal(rsp(resp), &words)
	// Should return an array (might be empty)
	assert.NotNil(t, words)
}

func TestWorryWords_Create(t *testing.T) {
	_, token := getSupportUser(t)

	// Test valid creation
	wordReq := config.CreateWorryWordRequest{
		Keyword: "testworry",
	}

	body, _ := json2.Marshal(wordReq)
	req := httptest.NewRequest("POST", "/api/config/admin/worry_words", bytes.NewReader(body))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 201, resp.StatusCode)

	var word config.WorryWord
	json2.Unmarshal(rsp(resp), &word)
	assert.Equal(t, "testworry", word.Keyword)
	assert.Greater(t, word.ID, uint64(0))

	// Clean up
	db := database.DBConn
	db.Delete(&config.WorryWord{}, word.ID)
}

func TestWorryWords_CreateValidation(t *testing.T) {
	_, token := getSupportUser(t)

	// Test empty word
	wordReq := config.CreateWorryWordRequest{
		Keyword: "",
	}

	body, _ := json2.Marshal(wordReq)
	req := httptest.NewRequest("POST", "/api/config/admin/worry_words", bytes.NewReader(body))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)

	// Test whitespace-only word
	wordReq = config.CreateWorryWordRequest{
		Keyword: "   ",
	}

	body, _ = json2.Marshal(wordReq)
	req = httptest.NewRequest("POST", "/api/config/admin/worry_words", bytes.NewReader(body))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestWorryWords_Delete(t *testing.T) {
	_, token := getSupportUser(t)
	db := database.DBConn

	// Create a test word
	word := config.WorryWord{
		Keyword: "testdeleteworry",
	}
	db.Create(&word)

	// Delete it
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/config/admin/worry_words/%d", word.ID), nil)
	req.Header.Set("Authorization", token)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 204, resp.StatusCode)

	// Verify it's deleted
	var count int64
	db.Model(&config.WorryWord{}).Where("id = ?", word.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestWorryWords_DeleteNotFound(t *testing.T) {
	_, token := getSupportUser(t)

	// Try to delete non-existent word
	req := httptest.NewRequest("DELETE", "/api/config/admin/worry_words/999999", nil)
	req.Header.Set("Authorization", token)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 404, resp.StatusCode)
}

// Integration tests

func TestSpamKeywords_Integration(t *testing.T) {
	_, token := getSupportUser(t)

	// Create a keyword with exclude pattern
	keywordReq := config.CreateSpamKeywordRequest{
		Word:    "integration",
		Exclude: stringPtr("test"),
		Action:  "Review",
		Type:    "Regex",
	}

	body, _ := json2.Marshal(keywordReq)
	req := httptest.NewRequest("POST", "/api/config/admin/spam_keywords", bytes.NewReader(body))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 201, resp.StatusCode)

	var keyword config.SpamKeyword
	json2.Unmarshal(rsp(resp), &keyword)

	// List keywords and verify it's included
	req = httptest.NewRequest("GET", "/api/config/admin/spam_keywords", nil)
	req.Header.Set("Authorization", token)
	resp, _ = getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var keywords []config.SpamKeyword
	json2.Unmarshal(rsp(resp), &keywords)

	found := false
	for _, k := range keywords {
		if k.ID == keyword.ID {
			found = true
			assert.Equal(t, "integration", k.Word)
			assert.Equal(t, "test", *k.Exclude)
			assert.Equal(t, "Review", k.Action)
			assert.Equal(t, "Regex", k.Type)
			break
		}
	}
	assert.True(t, found, "Created keyword should be in the list")

	// Clean up
	db := database.DBConn
	db.Delete(&config.SpamKeyword{}, keyword.ID)
}
