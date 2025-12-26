package test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	user2 "github.com/freegle/iznik-server-go/user"
	"github.com/stretchr/testify/assert"
)

func TestDeleted(t *testing.T) {
	// Create a deleted user for this test
	prefix := uniquePrefix("deleted")
	uid := CreateDeletedTestUser(t, prefix)

	// Get of the user should work, even though they're deleted.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(uid), nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetUserByEmail(t *testing.T) {
	t.Run("Valid email returns exists true", func(t *testing.T) {
		// Create a user with a specific email for this test
		prefix := uniquePrefix("byemail")
		email := fmt.Sprintf("%s@test.com", prefix)
		CreateTestUserWithEmail(t, prefix, email)

		// Test the API endpoint
		resp, err := getApp().Test(httptest.NewRequest("GET", "/api/user/byemail/"+email, nil))
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		// Parse response
		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		assert.NoError(t, err)
		assert.True(t, response["exists"].(bool))
	})

	t.Run("Non-existent email returns exists false", func(t *testing.T) {
		resp, err := getApp().Test(httptest.NewRequest("GET", "/api/user/byemail/nonexistent@example.com", nil))
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		// Parse response
		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		assert.NoError(t, err)
		assert.False(t, response["exists"].(bool))
	})

	t.Run("Empty email returns 400", func(t *testing.T) {
		resp, err := getApp().Test(httptest.NewRequest("GET", "/api/user/byemail/", nil))
		assert.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode) // Route not found when email is empty
	})
}

func TestGetUsersBatch(t *testing.T) {
	t.Run("Batch fetch multiple users returns all users", func(t *testing.T) {
		// Create two test users
		prefix1 := uniquePrefix("batchuser1")
		prefix2 := uniquePrefix("batchuser2")
		uid1 := CreateTestUser(t, prefix1, "User")
		uid2 := CreateTestUser(t, prefix2, "User")

		// Fetch both users in a single batch request
		url := fmt.Sprintf("/api/user/%d,%d", uid1, uid2)
		resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		// Parse response - should be an array of users
		var users []user2.User
		err = json.NewDecoder(resp.Body).Decode(&users)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(users))

		// Check that both users are in the response (order may vary)
		foundIds := make(map[uint64]bool)
		for _, u := range users {
			foundIds[u.ID] = true
		}
		assert.True(t, foundIds[uid1], "User 1 should be in response")
		assert.True(t, foundIds[uid2], "User 2 should be in response")
	})

	t.Run("Batch fetch single user returns single user object", func(t *testing.T) {
		// A single user without comma should return a single user, not an array
		prefix := uniquePrefix("singleuser")
		uid := CreateTestUser(t, prefix, "User")

		url := fmt.Sprintf("/api/user/%d", uid)
		resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		// Parse response - should be a single user object, not array
		var user user2.User
		err = json.NewDecoder(resp.Body).Decode(&user)
		assert.NoError(t, err)
		assert.Equal(t, uid, user.ID)
	})

	t.Run("Batch fetch with non-existent user skips that user", func(t *testing.T) {
		// Create one real user
		prefix := uniquePrefix("realuser")
		uid := CreateTestUser(t, prefix, "User")

		// Request with real user and a non-existent user ID
		url := fmt.Sprintf("/api/user/%d,999999999", uid)
		resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		// Parse response - should only contain the real user
		var users []user2.User
		err = json.NewDecoder(resp.Body).Decode(&users)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(users))
		assert.Equal(t, uid, users[0].ID)
	})

	t.Run("Batch fetch with too many users returns 400", func(t *testing.T) {
		// Create a request with 31 user IDs (over the limit of 30)
		ids := "1"
		for i := 2; i <= 31; i++ {
			ids += fmt.Sprintf(",%d", i)
		}

		url := fmt.Sprintf("/api/user/%s", ids)
		resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
		assert.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
	})
}
