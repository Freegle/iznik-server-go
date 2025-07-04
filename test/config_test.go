package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/config"
	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestConfig(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/config/wibble", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var results []config.ConfigItem
	json2.Unmarshal(rsp(resp), &results)
	assert.Equal(t, len(results), 0)
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
	_, regularToken := GetUserWithToken(t, "User")
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/config/admin/spam_keywords?jwt="+regularToken, nil))
	assert.Equal(t, 403, resp.StatusCode)
}

func TestSpamKeywords_List(t *testing.T) {
	_, token := GetUserWithToken(t, "Support")

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/config/admin/spam_keywords?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var keywords []config.SpamKeyword
	json2.Unmarshal(rsp(resp), &keywords)
	// Should return an array (might be empty)
	assert.NotNil(t, keywords)
}

func TestSpamKeywords_Create(t *testing.T) {
	_, token := GetUserWithToken(t, "Support")

	// Test valid creation
	keywordReq := config.CreateSpamKeywordRequest{
		Word:   "testspam",
		Action: "Spam",
		Type:   "Literal",
	}

	body, _ := json2.Marshal(keywordReq)
	req := httptest.NewRequest("POST", "/api/config/admin/spam_keywords?jwt="+token, bytes.NewReader(body))
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
	_, token := GetUserWithToken(t, "Support")

	// Test invalid action
	keywordReq := config.CreateSpamKeywordRequest{
		Word:   "testspam",
		Action: "Invalid",
		Type:   "Literal",
	}

	body, _ := json2.Marshal(keywordReq)
	req := httptest.NewRequest("POST", "/api/config/admin/spam_keywords?jwt="+token, bytes.NewReader(body))
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
	req = httptest.NewRequest("POST", "/api/config/admin/spam_keywords?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestSpamKeywords_Delete(t *testing.T) {
	_, token := GetUserWithToken(t, "Support")
	db := database.DBConn

	// Create a test keyword
	keyword := config.SpamKeyword{
		Word:   "testdelete",
		Action: "Review",
		Type:   "Literal",
	}
	db.Create(&keyword)

	// Delete it
	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/config/admin/spam_keywords/%d?jwt=%s", keyword.ID, token), nil))
	assert.Equal(t, 204, resp.StatusCode)

	// Verify it's deleted
	var count int64
	db.Model(&config.SpamKeyword{}).Where("id = ?", keyword.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestSpamKeywords_DeleteNotFound(t *testing.T) {
	_, token := GetUserWithToken(t, "Support")

	// Try to delete non-existent keyword
	resp, _ := getApp().Test(httptest.NewRequest("DELETE", "/api/config/admin/spam_keywords/999999?jwt="+token, nil))
	assert.Equal(t, 404, resp.StatusCode)
}

// Test Worry Words endpoints

func TestWorryWords_Unauthorized(t *testing.T) {
	// Test without authentication
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/config/admin/worry_words", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Test with regular user (should be forbidden)
	_, regularToken := GetUserWithToken(t, "User")
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/config/admin/worry_words?jwt="+regularToken, nil))
	assert.Equal(t, 403, resp.StatusCode)
}

func TestWorryWords_List(t *testing.T) {
	_, token := GetUserWithToken(t, "Support")

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/config/admin/worry_words?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var words []config.WorryWord
	json2.Unmarshal(rsp(resp), &words)
	// Should return an array (might be empty)
	assert.NotNil(t, words)
}

func TestWorryWords_Create(t *testing.T) {
	_, token := GetUserWithToken(t, "Support")

	// Test valid creation
	wordReq := config.CreateWorryWordRequest{
		Keyword: "testworry",
	}

	body, _ := json2.Marshal(wordReq)
	req := httptest.NewRequest("POST", "/api/config/admin/worry_words?jwt="+token, bytes.NewReader(body))
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
	_, token := GetUserWithToken(t, "Support")

	// Test empty word
	wordReq := config.CreateWorryWordRequest{
		Keyword: "",
	}

	body, _ := json2.Marshal(wordReq)
	req := httptest.NewRequest("POST", "/api/config/admin/worry_words?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)

	// Test whitespace-only word
	wordReq = config.CreateWorryWordRequest{
		Keyword: "   ",
	}

	body, _ = json2.Marshal(wordReq)
	req = httptest.NewRequest("POST", "/api/config/admin/worry_words?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestWorryWords_Delete(t *testing.T) {
	_, token := GetUserWithToken(t, "Support")
	db := database.DBConn

	// Create a test word
	word := config.WorryWord{
		Keyword: "testdeleteworry",
	}
	db.Create(&word)

	// Delete it
	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/config/admin/worry_words/%d?jwt=%s", word.ID, token), nil))
	assert.Equal(t, 204, resp.StatusCode)

	// Verify it's deleted
	var count int64
	db.Model(&config.WorryWord{}).Where("id = ?", word.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestWorryWords_DeleteNotFound(t *testing.T) {
	_, token := GetUserWithToken(t, "Support")

	// Try to delete non-existent word
	resp, _ := getApp().Test(httptest.NewRequest("DELETE", "/api/config/admin/worry_words/999999?jwt="+token, nil))
	assert.Equal(t, 404, resp.StatusCode)
}

// Integration tests

func TestSpamKeywords_Integration(t *testing.T) {
	_, token := GetUserWithToken(t, "Support")

	// Create a keyword with exclude pattern
	keywordReq := config.CreateSpamKeywordRequest{
		Word:    "integration",
		Exclude: stringPtr("test"),
		Action:  "Review",
		Type:    "Regex",
	}

	body, _ := json2.Marshal(keywordReq)
	req := httptest.NewRequest("POST", "/api/config/admin/spam_keywords?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 201, resp.StatusCode)

	var keyword config.SpamKeyword
	json2.Unmarshal(rsp(resp), &keyword)

	// List keywords and verify it's included
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/config/admin/spam_keywords?jwt="+token, nil))
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
