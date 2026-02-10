package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func createTestSearch(t *testing.T, userID uint64, term string) uint64 {
	db := database.DBConn
	result := db.Exec("INSERT INTO users_searches (userid, term, maxmsg, deleted) VALUES (?, ?, 999999, 0)", userID, term)
	assert.NoError(t, result.Error)

	var id uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)
	return id
}

func TestDeleteUserSearch(t *testing.T) {
	prefix := uniquePrefix("DeleteSearch")
	userID, token := CreateFullTestUser(t, prefix)

	// Create a search to delete.
	searchID := createTestSearch(t, userID, prefix+"_testsearch")

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/usersearch?id=%d&jwt=%s", searchID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify it is soft-deleted.
	var deleted int
	database.DBConn.Raw("SELECT deleted FROM users_searches WHERE id = ?", searchID).Scan(&deleted)
	assert.Equal(t, 1, deleted)
}

func TestDeleteUserSearchNotLoggedIn(t *testing.T) {
	req := httptest.NewRequest("DELETE", "/api/usersearch?id=1", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(1), result["ret"])
	assert.Equal(t, "Not logged in", result["status"])
}

func TestDeleteUserSearchWrongUser(t *testing.T) {
	prefix := uniquePrefix("DeleteSearchWrong")

	// Create a user who owns the search.
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	searchID := createTestSearch(t, ownerID, prefix+"_search")

	// Create a different user who tries to delete it.
	prefix2 := uniquePrefix("DeleteSearchWrong2")
	_, token := CreateFullTestUser(t, prefix2)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/usersearch?id=%d&jwt=%s", searchID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
	assert.Equal(t, "Permission denied", result["status"])
}

func TestDeleteUserSearchV2Path(t *testing.T) {
	prefix := uniquePrefix("DeleteSearchV2")
	userID, token := CreateFullTestUser(t, prefix)

	searchID := createTestSearch(t, userID, prefix+"_search")

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/apiv2/usersearch?id=%d&jwt=%s", searchID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
}
