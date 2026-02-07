package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Tests for GET /api/user/{id}/search
// =============================================================================

func TestUserSearch_Unauthorized(t *testing.T) {
	// Without auth should return 404 (handler returns "User not found" when myid is 0).
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/1/search", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestUserSearch_OwnSearches(t *testing.T) {
	prefix := uniquePrefix("usrsearch")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	db := database.DBConn

	// Create some test search records.
	db.Exec("INSERT INTO users_searches (userid, term, deleted, date) VALUES (?, 'sofa', 0, NOW())", userID)
	db.Exec("INSERT INTO users_searches (userid, term, deleted, date) VALUES (?, 'table', 0, NOW())", userID)
	db.Exec("INSERT INTO users_searches (userid, term, deleted, date) VALUES (?, 'chair', 0, NOW())", userID)
	defer db.Exec("DELETE FROM users_searches WHERE userid = ?", userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/user/%d/search?jwt=%s", userID, token), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var searches []user.Search
	json2.Unmarshal(rsp(resp), &searches)
	assert.GreaterOrEqual(t, len(searches), 3)

	// Verify searches belong to the correct user.
	for _, s := range searches {
		assert.Equal(t, userID, s.Userid)
	}
}

func TestUserSearch_OtherUserSearches(t *testing.T) {
	prefix := uniquePrefix("usrsearchother")
	userID := CreateTestUser(t, prefix, "User")
	otherUserID := CreateTestUser(t, prefix+"_other", "User")
	_, token := CreateTestSession(t, userID)

	// Trying to access another user's searches should return 404.
	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/user/%d/search?jwt=%s", otherUserID, token), nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestUserSearch_DeletedSearchesExcluded(t *testing.T) {
	prefix := uniquePrefix("usrsearchdel")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	db := database.DBConn

	// Create a mix of deleted and non-deleted searches.
	db.Exec("INSERT INTO users_searches (userid, term, deleted, date) VALUES (?, 'active_search', 0, NOW())", userID)
	db.Exec("INSERT INTO users_searches (userid, term, deleted, date) VALUES (?, 'deleted_search', 1, NOW())", userID)
	defer db.Exec("DELETE FROM users_searches WHERE userid = ?", userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/user/%d/search?jwt=%s", userID, token), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var searches []user.Search
	json2.Unmarshal(rsp(resp), &searches)

	// Verify no deleted searches are included.
	for _, s := range searches {
		assert.NotEqual(t, "deleted_search", s.Term)
	}
}

func TestUserSearch_DeduplicatesTerms(t *testing.T) {
	prefix := uniquePrefix("usrsearchdedup")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	db := database.DBConn

	// Create multiple searches with the same term.
	db.Exec("INSERT INTO users_searches (userid, term, deleted, date) VALUES (?, 'duplicate_term', 0, DATE_SUB(NOW(), INTERVAL 1 HOUR))", userID)
	db.Exec("INSERT INTO users_searches (userid, term, deleted, date) VALUES (?, 'duplicate_term', 0, NOW())", userID)
	db.Exec("INSERT INTO users_searches (userid, term, deleted, date) VALUES (?, 'unique_term', 0, NOW())", userID)
	defer db.Exec("DELETE FROM users_searches WHERE userid = ?", userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/user/%d/search?jwt=%s", userID, token), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var searches []user.Search
	json2.Unmarshal(rsp(resp), &searches)

	// Count occurrences of "duplicate_term" - should be 1 due to GROUP BY.
	dupCount := 0
	for _, s := range searches {
		if s.Term == "duplicate_term" {
			dupCount++
		}
	}
	assert.Equal(t, 1, dupCount, "Duplicate terms should be deduplicated")
}

func TestUserSearch_LimitedTo10(t *testing.T) {
	prefix := uniquePrefix("usrsearchlimit")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	db := database.DBConn

	// Create more than 10 unique search terms.
	for i := 0; i < 15; i++ {
		db.Exec("INSERT INTO users_searches (userid, term, deleted, date) VALUES (?, ?, 0, NOW())",
			userID, fmt.Sprintf("term_%d_%s", i, prefix))
	}
	defer db.Exec("DELETE FROM users_searches WHERE userid = ?", userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/user/%d/search?jwt=%s", userID, token), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var searches []user.Search
	json2.Unmarshal(rsp(resp), &searches)

	// Should be limited to 10 results.
	assert.LessOrEqual(t, len(searches), 10)
}

func TestUserSearch_InvalidUserID(t *testing.T) {
	prefix := uniquePrefix("usrsearchinval")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/abc/search?jwt="+token, nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestUserSearch_V2Path(t *testing.T) {
	prefix := uniquePrefix("usrsearchv2")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/apiv2/user/%d/search?jwt=%s", userID, token), nil))
	assert.Equal(t, 200, resp.StatusCode)
}
