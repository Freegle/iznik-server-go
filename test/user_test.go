package test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
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

func TestUserComments(t *testing.T) {
	t.Run("Moderator with modtools=true sees comments", func(t *testing.T) {
		db := database.DBConn
		prefix := uniquePrefix("comments_mod")

		// Create a moderator user with session.
		modID := CreateTestUser(t, prefix+"_mod", "Moderator")
		_, modToken := CreateTestSession(t, modID)

		// Create a target user.
		targetID := CreateTestUser(t, prefix+"_target", "User")

		// Create a group and membership so the mod can see comments.
		groupID := CreateTestGroup(t, prefix)
		CreateTestMembership(t, modID, groupID, "Moderator")

		// Insert a comment on the target user.
		db.Exec("INSERT INTO users_comments (userid, groupid, byuserid, user1, date) VALUES (?, ?, ?, 'Test note', NOW())",
			targetID, groupID, modID)

		// Fetch user with modtools=true.
		url := fmt.Sprintf("/api/user/%d?modtools=true&jwt=%s", targetID, modToken)
		resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var user user2.User
		err = json.NewDecoder(resp.Body).Decode(&user)
		assert.NoError(t, err)
		assert.Equal(t, targetID, user.ID)
		assert.NotNil(t, user.Comments)
		assert.Equal(t, 1, len(user.Comments))
		assert.Equal(t, "Test note", *user.Comments[0].User1)
		assert.NotNil(t, user.Comments[0].Byuser)
		assert.Equal(t, modID, user.Comments[0].Byuser.ID)
	})

	t.Run("Moderator without modtools param gets no comments", func(t *testing.T) {
		prefix := uniquePrefix("comments_nomt")
		db := database.DBConn

		modID := CreateTestUser(t, prefix+"_mod", "Moderator")
		_, modToken := CreateTestSession(t, modID)
		targetID := CreateTestUser(t, prefix+"_target", "User")
		groupID := CreateTestGroup(t, prefix)

		db.Exec("INSERT INTO users_comments (userid, groupid, byuserid, user1, date) VALUES (?, ?, ?, 'Hidden note', NOW())",
			targetID, groupID, modID)

		// Fetch WITHOUT modtools param.
		url := fmt.Sprintf("/api/user/%d?jwt=%s", targetID, modToken)
		resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var user user2.User
		err = json.NewDecoder(resp.Body).Decode(&user)
		assert.NoError(t, err)
		assert.Nil(t, user.Comments)
	})

	t.Run("Non-moderator with modtools=true gets no comments", func(t *testing.T) {
		prefix := uniquePrefix("comments_nonmod")
		db := database.DBConn

		userID := CreateTestUser(t, prefix+"_user", "User")
		_, userToken := CreateTestSession(t, userID)
		targetID := CreateTestUser(t, prefix+"_target", "User")

		db.Exec("INSERT INTO users_comments (userid, user1, date) VALUES (?, 'Secret note', NOW())", targetID)

		url := fmt.Sprintf("/api/user/%d?modtools=true&jwt=%s", targetID, userToken)
		resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var user user2.User
		err = json.NewDecoder(resp.Body).Decode(&user)
		assert.NoError(t, err)
		assert.Nil(t, user.Comments)
	})

	t.Run("Batch fetch with modtools=true includes comments", func(t *testing.T) {
		prefix := uniquePrefix("comments_batch")
		db := database.DBConn

		modID := CreateTestUser(t, prefix+"_mod", "Moderator")
		_, modToken := CreateTestSession(t, modID)
		target1 := CreateTestUser(t, prefix+"_t1", "User")
		target2 := CreateTestUser(t, prefix+"_t2", "User")
		groupID := CreateTestGroup(t, prefix)

		db.Exec("INSERT INTO users_comments (userid, groupid, byuserid, user1, date) VALUES (?, ?, ?, 'Note on t1', NOW())",
			target1, groupID, modID)
		db.Exec("INSERT INTO users_comments (userid, groupid, byuserid, user1, date) VALUES (?, ?, ?, 'Note on t2', NOW())",
			target2, groupID, modID)

		url := fmt.Sprintf("/api/user/%d,%d?modtools=true&jwt=%s", target1, target2, modToken)
		resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var users []user2.User
		err = json.NewDecoder(resp.Body).Decode(&users)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(users))

		for _, u := range users {
			assert.NotNil(t, u.Comments, "User %d should have comments", u.ID)
			assert.Equal(t, 1, len(u.Comments))
		}
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
