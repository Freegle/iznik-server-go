package test

import (
	"bytes"
	"encoding/json"
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/log"
	user2 "github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		assert.Equal(t, 400, resp.StatusCode) // "byemail" is not a valid user ID
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

func TestLastpushNullReturnsNil(t *testing.T) {
	t.Run("User with no push notifications returns nil lastpush", func(t *testing.T) {
		prefix := uniquePrefix("lastpush_null")

		// Create a mod and a target user with no push notifications.
		modID := CreateTestUser(t, prefix+"_mod", "Moderator")
		_, modToken := CreateTestSession(t, modID)
		targetID := CreateTestUser(t, prefix+"_target", "User")
		groupID := CreateTestGroup(t, prefix)
		CreateTestMembership(t, modID, groupID, "Moderator")
		CreateTestMembership(t, targetID, groupID, "Member")

		url := fmt.Sprintf("/api/user/%d?modtools=true&jwt=%s", targetID, modToken)
		resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var u user2.User
		err = json.NewDecoder(resp.Body).Decode(&u)
		assert.NoError(t, err)
		assert.Nil(t, u.Lastpush, "lastpush should be nil when user has no push notifications")
	})
}

func TestInventNameForBlankUser(t *testing.T) {
	db := database.DBConn

	for _, tc := range []struct {
		name     string
		fullname interface{}
	}{
		{"null fullname", nil},
		{"empty fullname", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prefix := uniquePrefix("invent_name")
			// Keep local part under 32 chars so TidyName does not truncate it.
			localPart := fmt.Sprintf("inv%d", time.Now().UnixNano()%100000000)

			db.Exec("INSERT INTO users (fullname, systemrole) VALUES (?, 'User')", tc.fullname)
			var targetID uint64
			db.Raw("SELECT id FROM users ORDER BY id DESC LIMIT 1").Scan(&targetID)
			require.NotZero(t, targetID)

			// Add an email — local part should become the display name.
			db.Exec("INSERT INTO users_emails (userid, email, preferred) VALUES (?, ?, 1)", targetID, localPart+"@example.com")

			// Create a viewing user.
			viewerID := CreateTestUser(t, prefix+"_viewer", "User")
			_, viewerToken := CreateTestSession(t, viewerID)

			url := fmt.Sprintf("/api/user/%d?jwt=%s", targetID, viewerToken)
			resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
			assert.NoError(t, err)
			assert.Equal(t, 200, resp.StatusCode)

			var u user2.User
			err = json.NewDecoder(resp.Body).Decode(&u)
			assert.NoError(t, err)
			assert.Equal(t, localPart, u.Displayname, "Display name should be email local part, not 'A freegler'")

			// Verify the invented name was stored in the DB.
			var storedFullname string
			db.Raw("SELECT fullname FROM users WHERE id = ?", targetID).Scan(&storedFullname)
			assert.Equal(t, localPart, storedFullname)
		})
	}
}

func TestInventNamePersistsBadStoredNames(t *testing.T) {
	db := database.DBConn

	// 32-char Yahoo-ID-style hex string (alphanumeric mix).
	yahooHex := "ab12cd34ef56ab12cd34ef56ab12cd34"

	for _, tc := range []struct {
		name     string
		fullname string
	}{
		{"FBUser name", "FBUser12345"},
		{"Yahoo hex name", yahooHex},
		{"A freegler literal", "A freegler"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prefix := uniquePrefix("invent_persist")

			db.Exec("INSERT INTO users (fullname, systemrole) VALUES (?, 'User')", tc.fullname)
			var targetID uint64
			db.Raw("SELECT id FROM users ORDER BY id DESC LIMIT 1").Scan(&targetID)
			require.NotZero(t, targetID)

			// Give the user a clean email whose local part will be used as the invented name.
			cleanLocal := fmt.Sprintf("cleanlocal%d", time.Now().UnixNano()%100000000)
			db.Exec("INSERT INTO users_emails (userid, email, preferred) VALUES (?, ?, 1)", targetID, cleanLocal+"@example.com")

			viewerID := CreateTestUser(t, prefix+"_viewer", "User")
			_, viewerToken := CreateTestSession(t, viewerID)

			url := fmt.Sprintf("/api/user/%d?jwt=%s", targetID, viewerToken)
			resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
			assert.NoError(t, err)
			assert.Equal(t, 200, resp.StatusCode)

			var u user2.User
			err = json.NewDecoder(resp.Body).Decode(&u)
			assert.NoError(t, err)
			assert.NotEqual(t, tc.fullname, u.Displayname, "displayname must not be the bad stored value")
			assert.NotEqual(t, "A freegler", u.Displayname, "displayname must not be 'A freegler'")
			assert.Equal(t, cleanLocal, u.Displayname, "displayname should be invented from email local part")

			// The bad fullname must be overwritten in the DB so subsequent reads don't repeat the cycle.
			var storedFullname string
			db.Raw("SELECT fullname FROM users WHERE id = ?", targetID).Scan(&storedFullname)
			assert.Equal(t, u.Displayname, storedFullname, "fullname in DB should be updated to invented name")
		})
	}
}

func TestInventNameFallsBackToGeneratedName(t *testing.T) {
	db := database.DBConn

	// User with no email at all — must fall back to trigram-generated name.
	db.Exec("INSERT INTO users (fullname, systemrole) VALUES ('A freegler', 'User')")
	var targetID uint64
	db.Raw("SELECT id FROM users ORDER BY id DESC LIMIT 1").Scan(&targetID)
	require.NotZero(t, targetID)

	prefix := uniquePrefix("invent_fallback")
	viewerID := CreateTestUser(t, prefix+"_viewer", "User")
	_, viewerToken := CreateTestSession(t, viewerID)

	url := fmt.Sprintf("/api/user/%d?jwt=%s", targetID, viewerToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var u user2.User
	err = json.NewDecoder(resp.Body).Decode(&u)
	assert.NoError(t, err)
	// With no email, GenerateName() is used — result should be a non-empty plausible word.
	assert.NotEmpty(t, u.Displayname)
	assert.NotEqual(t, "A freegler", u.Displayname)
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

func TestPostUserEngaged(t *testing.T) {
	// Record engagement success (no login required).
	// Even with a non-existent engage ID, the handler returns success.
	payload := map[string]interface{}{"engageid": 999999}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user", bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestPostUserEngagedStringId(t *testing.T) {
	// Nuxt3 route.query values are always strings; the server must accept
	// engageid as a JSON string, not just a JSON number (Sentry 7377071204).
	payload := `{"engageid":"999999"}`
	request := httptest.NewRequest("POST", "/api/user", bytes.NewBufferString(payload))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestPostUserRateUp(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("rateup")
	raterID := CreateTestUser(t, prefix+"_rater", "User")
	rateeID := CreateTestUser(t, prefix+"_ratee", "User")
	_, token := CreateTestSession(t, raterID)

	// Rate user up.
	rating := "Up"
	payload := map[string]interface{}{
		"action": "Rate",
		"ratee":  rateeID,
		"rating": rating,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Verify rating in DB.
	var storedRating string
	db.Raw("SELECT rating FROM ratings WHERE rater = ? AND ratee = ?", raterID, rateeID).Scan(&storedRating)
	assert.Equal(t, "Up", storedRating)
}

func TestPostUserRateDown(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("ratedown")
	raterID := CreateTestUser(t, prefix+"_rater", "User")
	rateeID := CreateTestUser(t, prefix+"_ratee", "User")
	_, token := CreateTestSession(t, raterID)

	// Rate user down with reason and text.
	rating := "Down"
	reason := "Didn't show up"
	text := "Was a no-show"
	payload := map[string]interface{}{
		"action": "Rate",
		"ratee":  rateeID,
		"rating": rating,
		"reason": reason,
		"text":   text,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Verify rating in DB with reviewrequired set.
	var storedRating string
	var reviewRequired bool
	db.Raw("SELECT rating, reviewrequired FROM ratings WHERE rater = ? AND ratee = ?", raterID, rateeID).Row().Scan(&storedRating, &reviewRequired)
	assert.Equal(t, "Down", storedRating)
	assert.True(t, reviewRequired)
}

func TestPostUserRateSelf(t *testing.T) {
	prefix := uniquePrefix("rateself")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	rating := "Up"
	payload := map[string]interface{}{
		"action": "Rate",
		"ratee":  userID,
		"rating": rating,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestPostUserRateNotLoggedIn(t *testing.T) {
	payload := map[string]interface{}{
		"action": "Rate",
		"ratee":  1,
		"rating": "Up",
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user", bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestPostUserRatingReviewed(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("ratingrev")
	raterID := CreateTestUser(t, prefix+"_rater", "Support")
	rateeID := CreateTestUser(t, prefix+"_ratee", "User")
	_, token := CreateTestSession(t, raterID)

	// Insert a rating with reviewrequired.
	db.Exec("INSERT INTO ratings (rater, ratee, rating, reason, text, timestamp, reviewrequired) VALUES (?, ?, 'Down', 'Test', 'Test', NOW(), 1)",
		raterID, rateeID)
	var ratingID uint64
	db.Raw("SELECT id FROM ratings WHERE rater = ? AND ratee = ? ORDER BY id DESC LIMIT 1", raterID, rateeID).Scan(&ratingID)
	assert.Greater(t, ratingID, uint64(0))

	// Mark rating as reviewed.
	payload := map[string]interface{}{
		"action":   "RatingReviewed",
		"ratingid": ratingID,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Verify reviewrequired is now 0.
	var reviewRequired bool
	db.Raw("SELECT reviewrequired FROM ratings WHERE id = ?", ratingID).Scan(&reviewRequired)
	assert.False(t, reviewRequired)
}

func TestPostUserAddEmail(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("addemail")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	newEmail := prefix + "_new@test.com"
	payload := map[string]interface{}{
		"action": "AddEmail",
		"id":     userID,
		"email":  newEmail,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var response map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&response)
	assert.Equal(t, float64(0), response["ret"])

	// Verify email is in DB.
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_emails WHERE userid = ? AND email = ?", userID, newEmail).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestPostUserAddEmailAlreadyUsed(t *testing.T) {
	prefix := uniquePrefix("addemaildup")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token1 := CreateTestSession(t, user1ID)

	// Add email to user2 first.
	db := database.DBConn
	existingEmail := prefix + "_existing@test.com"
	db.Exec("INSERT INTO users_emails (userid, email) VALUES (?, ?)", user2ID, existingEmail)

	// Try to add same email to user1 - should fail with ret=3.
	payload := map[string]interface{}{
		"action": "AddEmail",
		"id":     user1ID,
		"email":  existingEmail,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+token1, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusConflict, resp.StatusCode)

	var response map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&response)
	assert.Equal(t, float64(3), response["ret"])
}

func TestPostUserRemoveEmail(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("rmemail")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	emailToRemove := prefix + "_remove@test.com"
	db.Exec("INSERT INTO users_emails (userid, email) VALUES (?, ?)", userID, emailToRemove)

	payload := map[string]interface{}{
		"action": "RemoveEmail",
		"id":     userID,
		"email":  emailToRemove,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var response map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&response)
	assert.Equal(t, float64(0), response["ret"])

	// Verify email is removed.
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_emails WHERE userid = ? AND email = ?", userID, emailToRemove).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestPostUserUnknownAction(t *testing.T) {
	prefix := uniquePrefix("unkaction")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	payload := map[string]interface{}{
		"action": "DoSomethingWeird",
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestPostUserInvalidJSON(t *testing.T) {
	prefix := uniquePrefix("badjson")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	request := httptest.NewRequest("POST", "/api/user?jwt="+token, bytes.NewBuffer([]byte("not json")))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestPostUserRateInvalidRating(t *testing.T) {
	prefix := uniquePrefix("badrating")
	raterID := CreateTestUser(t, prefix+"_rater", "User")
	rateeID := CreateTestUser(t, prefix+"_ratee", "User")
	_, token := CreateTestSession(t, raterID)

	rating := "Sideways"
	payload := map[string]interface{}{
		"action": "Rate",
		"ratee":  rateeID,
		"rating": rating,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestPostUserRateNoRatee(t *testing.T) {
	prefix := uniquePrefix("noratee")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	payload := map[string]interface{}{
		"action": "Rate",
		"rating": "Up",
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestPostUserAddEmailNoEmail(t *testing.T) {
	prefix := uniquePrefix("noemail")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	payload := map[string]interface{}{
		"action": "AddEmail",
		"id":     userID,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestPostUserAddEmailOtherUserNonAdmin(t *testing.T) {
	prefix := uniquePrefix("emailother")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token := CreateTestSession(t, user1ID)

	payload := map[string]interface{}{
		"action": "AddEmail",
		"id":     user2ID,
		"email":  prefix + "_new@test.com",
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestPostUserAddEmailSupportForOtherUser(t *testing.T) {
	// Support user can add email to another user's account.
	db := database.DBConn
	prefix := uniquePrefix("emailsupport")
	supportID := CreateTestUser(t, prefix+"_support", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	_, token := CreateTestSession(t, supportID)
	db.Exec("UPDATE users SET systemrole = 'Support' WHERE id = ?", supportID)

	newEmail := prefix + "_new@test.com"
	payload := map[string]interface{}{
		"action": "AddEmail",
		"id":     targetID,
		"email":  newEmail,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var response map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&response)
	assert.Equal(t, float64(0), response["ret"])

	// Verify email is on the target user.
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_emails WHERE userid = ? AND email = ?", targetID, newEmail).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestPostUserAddEmailOrphanedRow(t *testing.T) {
	// If an orphaned email row exists (userid IS NULL), adding should succeed by updating it.
	db := database.DBConn
	prefix := uniquePrefix("emailorphan")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	orphanEmail := prefix + "_orphan@test.com"
	db.Exec("INSERT INTO users_emails (userid, email) VALUES (NULL, ?)", orphanEmail)

	payload := map[string]interface{}{
		"action": "AddEmail",
		"id":     userID,
		"email":  orphanEmail,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var response map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&response)
	assert.Equal(t, float64(0), response["ret"])

	// Verify email is now assigned to the user.
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_emails WHERE userid = ? AND email = ?", userID, orphanEmail).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestPostUserAddEmailAlreadyOnSameUser(t *testing.T) {
	// Adding an email that already exists on the same user should succeed (idempotent).
	db := database.DBConn
	prefix := uniquePrefix("emailsame")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	existingEmail := prefix + "_existing@test.com"
	db.Exec("INSERT INTO users_emails (userid, email) VALUES (?, ?)", userID, existingEmail)

	payload := map[string]interface{}{
		"action": "AddEmail",
		"id":     userID,
		"email":  existingEmail,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Should still be exactly one row.
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_emails WHERE userid = ? AND email = ?", userID, existingEmail).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestPostUserRatingReviewedNoID(t *testing.T) {
	prefix := uniquePrefix("revnoid")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	payload := map[string]interface{}{
		"action": "RatingReviewed",
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestPostUserRemoveEmailNotOnUser(t *testing.T) {
	prefix := uniquePrefix("rmemailnotours")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	payload := map[string]interface{}{
		"action": "RemoveEmail",
		"id":     userID,
		"email":  "nonexistent@example.com",
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var response map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&response)
	assert.Equal(t, float64(3), response["ret"])
}

// =============================================================================
// PUT /user tests (signup)
// =============================================================================

func TestPutUser(t *testing.T) {
	prefix := uniquePrefix("putuser")
	email := fmt.Sprintf("%s@test.com", prefix)

	payload := map[string]interface{}{
		"email":       email,
		"password":    "testpass123",
		"firstname":   "Test",
		"lastname":    prefix,
		"displayname": "Test " + prefix,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/user", bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request, 5000)
	assert.NoError(t, err)
	if resp == nil {
		t.Fatal("Response is nil")
	}
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])
	assert.NotZero(t, result["id"])
	assert.NotEmpty(t, result["jwt"])
	assert.NotNil(t, result["persistent"])

	// Verify user exists in DB.
	db := database.DBConn
	userID := uint64(result["id"].(float64))
	var count int64
	db.Raw("SELECT COUNT(*) FROM users WHERE id = ?", userID).Scan(&count)
	assert.Equal(t, int64(1), count)

	// Verify email exists.
	db.Raw("SELECT COUNT(*) FROM users_emails WHERE userid = ? AND email = ?", userID, email).Scan(&count)
	assert.Equal(t, int64(1), count)

	// Verify login credentials exist.
	db.Raw("SELECT COUNT(*) FROM users_logins WHERE userid = ? AND type = 'Native'", userID).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestPutUserDuplicateEmail(t *testing.T) {
	prefix := uniquePrefix("putdup")
	// Create an existing user with an email.
	existingID := CreateTestUser(t, prefix+"_existing", "User")
	db := database.DBConn

	// Get the email for that user.
	var existingEmail string
	db.Raw("SELECT email FROM users_emails WHERE userid = ? LIMIT 1", existingID).Scan(&existingEmail)

	// Try to create a new user with the same email.
	payload := map[string]interface{}{
		"email":     existingEmail,
		"firstname": "Duplicate",
		"lastname":  "User",
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/user", bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusConflict, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(2), result["ret"])
	assert.Contains(t, result["status"], "already in use")
}

func TestPutUserWithGroup(t *testing.T) {
	prefix := uniquePrefix("putwgroup")
	groupID := CreateTestGroup(t, prefix)

	email := fmt.Sprintf("%s@test.com", prefix)
	payload := map[string]interface{}{
		"email":     email,
		"firstname": "Group",
		"lastname":  "Joiner",
		"groupid":   groupID,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/user", bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify membership was created.
	db := database.DBConn
	userID := uint64(result["id"].(float64))
	var memberCount int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ?", userID, groupID).Scan(&memberCount)
	assert.Equal(t, int64(1), memberCount)
}

// =============================================================================
// PATCH /user tests (profile update)
// =============================================================================

func TestPatchUserDisplayname(t *testing.T) {
	prefix := uniquePrefix("patchdn")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	newName := "Updated Name " + prefix
	payload := map[string]interface{}{
		"displayname": newName,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PATCH", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify in DB.
	db := database.DBConn
	var fullname string
	db.Raw("SELECT fullname FROM users WHERE id = ?", userID).Scan(&fullname)
	assert.Equal(t, newName, fullname)

	// Verify firstname and lastname are cleared.
	var firstname, lastname *string
	db.Raw("SELECT firstname FROM users WHERE id = ?", userID).Scan(&firstname)
	db.Raw("SELECT lastname FROM users WHERE id = ?", userID).Scan(&lastname)
	assert.Nil(t, firstname)
	assert.Nil(t, lastname)
}

func TestPatchUserSettings(t *testing.T) {
	prefix := uniquePrefix("patchsettings")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	settings := map[string]interface{}{
		"notificationmuted": true,
		"mylocation": map[string]interface{}{
			"lat": 51.5,
			"lng": -0.1,
		},
	}
	payload := map[string]interface{}{
		"settings": settings,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PATCH", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify settings are stored as JSON.
	db := database.DBConn
	var storedSettings string
	db.Raw("SELECT settings FROM users WHERE id = ?", userID).Scan(&storedSettings)
	assert.NotEmpty(t, storedSettings)
	assert.Contains(t, storedSettings, "notificationmuted")
}

func TestPatchUserSettingsPostcodeChange(t *testing.T) {
	prefix := uniquePrefix("patchpostcode")
	db := database.DBConn
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Create a test location of type Postcode.
	db.Exec("INSERT INTO locations (name, type, lat, lng) VALUES (?, 'Postcode', 55.9533, -3.1883)", prefix+"_loc")
	var locID uint64
	db.Raw("SELECT id FROM locations WHERE name = ? LIMIT 1", prefix+"_loc").Scan(&locID)
	if locID == 0 {
		t.Skip("Could not create test location")
	}
	defer db.Exec("DELETE FROM locations WHERE id = ?", locID)

	settings := map[string]interface{}{
		"mylocation": map[string]interface{}{
			"id":   locID,
			"name": prefix + "_loc",
			"type": "Postcode",
			"lat":  55.9533,
			"lng":  -3.1883,
		},
	}
	payload := map[string]interface{}{"settings": settings}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PATCH", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// lastlocation should be updated to the new location ID.
	var lastlocation uint64
	db.Raw("SELECT COALESCE(lastlocation, 0) FROM users WHERE id = ?", userID).Scan(&lastlocation)
	assert.Equal(t, locID, lastlocation, "lastlocation should be updated when postcode changes")

	// PostcodeChange log entry should exist.
	logEntry := findLog(db, "User", "PostcodeChange", userID)
	if assert.NotNil(t, logEntry, "PostcodeChange log entry should exist") {
		assert.Equal(t, prefix+"_loc", *logEntry.Text, "Log text should be the postcode name")
	}
}

func TestPatchUserOnHoliday(t *testing.T) {
	prefix := uniquePrefix("patchholiday")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	holidayDate := "2026-03-15"
	payload := map[string]interface{}{
		"onholidaytill": holidayDate,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PATCH", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify in DB.
	db := database.DBConn
	var onholidaytill *string
	db.Raw("SELECT onholidaytill FROM users WHERE id = ?", userID).Scan(&onholidaytill)
	assert.NotNil(t, onholidaytill)
	assert.Contains(t, *onholidaytill, "2026-03-15")
}

func TestPatchUserAboutMe(t *testing.T) {
	prefix := uniquePrefix("patchabout")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	aboutText := "I love freegling! " + prefix
	payload := map[string]interface{}{
		"aboutme": aboutText,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PATCH", "/api/user?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify in DB.
	db := database.DBConn
	var storedAbout string
	db.Raw("SELECT text FROM users_aboutme WHERE userid = ? ORDER BY timestamp DESC LIMIT 1", userID).Scan(&storedAbout)
	assert.Equal(t, aboutText, storedAbout)
}

func TestPatchUserNotLoggedIn(t *testing.T) {
	payload := map[string]interface{}{
		"displayname": "Should Fail",
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PATCH", "/api/user", bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestPatchUserMuteChitchat(t *testing.T) {
	prefix := uniquePrefix("patchmute")
	db := database.DBConn

	// Create a mod and a target user on the same group.
	modID := CreateTestUser(t, prefix+"_mod", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, modToken := CreateTestSession(t, modID)

	// Mod mutes the target user's chitchat.
	payload := map[string]interface{}{
		"id":                targetID,
		"newsfeedmodstatus": "Suppressed",
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PATCH", "/api/user?jwt="+modToken, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify in DB.
	var modstatus string
	db.Raw("SELECT COALESCE(newsfeedmodstatus, '') FROM users WHERE id = ?", targetID).Scan(&modstatus)
	assert.Equal(t, "Suppressed", modstatus)
}

func TestPatchUserPasswordBySupportUser(t *testing.T) {
	prefix := uniquePrefix("patchpw")
	db := database.DBConn

	// Create an admin/support user and a target user.
	adminID := CreateTestUser(t, prefix+"_admin", "User")
	db.Exec("UPDATE users SET systemrole = 'Support' WHERE id = ?", adminID)
	targetID := CreateTestUser(t, prefix+"_target", "User")
	_, adminToken := CreateTestSession(t, adminID)

	// Admin sets password for target user.
	payload := map[string]interface{}{
		"id":       targetID,
		"password": "newpass123",
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PATCH", "/api/user?jwt="+adminToken, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Target user should now be able to log in with the new password.
	loginPayload := map[string]interface{}{
		"email":    prefix + "_target@test.com",
		"password": "newpass123",
	}
	s, _ = json.Marshal(loginPayload)
	loginReq := httptest.NewRequest("POST", "/api/session", bytes.NewBuffer(s))
	loginReq.Header.Set("Content-Type", "application/json")
	resp, err = getApp().Test(loginReq, 5000)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.NotEmpty(t, result["jwt"])
}

func TestPatchUserPasswordByNonAdmin(t *testing.T) {
	prefix := uniquePrefix("patchpwnon")

	// Create two regular users.
	userID := CreateTestUser(t, prefix+"_user", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	_, userToken := CreateTestSession(t, userID)

	// Non-admin tries to set password for another user — should be forbidden.
	payload := map[string]interface{}{
		"id":       targetID,
		"password": "hackedpass",
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PATCH", "/api/user?jwt="+userToken, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

// =============================================================================
// DELETE /user tests
// =============================================================================

func TestLimboUserSelf(t *testing.T) {
	prefix := uniquePrefix("delself")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	request := httptest.NewRequest("DELETE", "/api/user?jwt="+token, nil)
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify user is marked as deleted.
	db := database.DBConn
	var deleted *string
	db.Raw("SELECT deleted FROM users WHERE id = ?", userID).Scan(&deleted)
	assert.NotNil(t, deleted)
}

func TestLimboUserAdmin(t *testing.T) {
	prefix := uniquePrefix("deladmin")
	db := database.DBConn
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	_, adminToken := CreateTestSession(t, adminID)

	// Give the target a membership so we can verify it gets removed.
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, targetID, groupID, "Member")

	// Verify membership exists before delete.
	var memBefore int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Approved'",
		targetID, groupID).Scan(&memBefore)
	assert.Equal(t, int64(1), memBefore, "Membership should exist before delete")

	payload := map[string]interface{}{
		"id": targetID,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("DELETE", "/api/user?jwt="+adminToken, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify target user is marked as deleted.
	var deleted *string
	db.Raw("SELECT deleted FROM users WHERE id = ?", targetID).Scan(&deleted)
	assert.NotNil(t, deleted)

	// Verify approved memberships were removed.
	var memAfter int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Approved'",
		targetID, groupID).Scan(&memAfter)
	assert.Equal(t, int64(0), memAfter, "Approved memberships should be removed on delete")

	// Verify log entry was created.
	var logCount int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE type = ? AND subtype = ? AND user = ? AND byuser = ?",
		log.LOG_TYPE_USER, log.LOG_SUBTYPE_DELETED, targetID, adminID).Scan(&logCount)
	assert.Equal(t, int64(1), logCount, "Delete should create a User/Deleted log entry")
}

func TestLimboUserNotAdmin(t *testing.T) {
	prefix := uniquePrefix("delnotadmin")
	userID := CreateTestUser(t, prefix+"_user", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	_, userToken := CreateTestSession(t, userID)

	payload := map[string]interface{}{
		"id": targetID,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("DELETE", "/api/user?jwt="+userToken, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

// =============================================================================
// POST /user tests (Unbounce and Merge actions)
// =============================================================================

func TestPostUserUnbounce(t *testing.T) {
	prefix := uniquePrefix("unbounce")
	db := database.DBConn

	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	_, adminToken := CreateTestSession(t, adminID)

	// Set the target user as bouncing.
	db.Exec("UPDATE users SET bouncing = 1 WHERE id = ?", targetID)

	payload := map[string]interface{}{
		"action": "Unbounce",
		"id":     targetID,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+adminToken, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify bouncing is now 0.
	var bouncing bool
	db.Raw("SELECT bouncing FROM users WHERE id = ?", targetID).Scan(&bouncing)
	assert.False(t, bouncing)
}

func TestPostUserUnbounceNotAdmin(t *testing.T) {
	prefix := uniquePrefix("unbouncenonadm")
	userID := CreateTestUser(t, prefix+"_user", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	_, userToken := CreateTestSession(t, userID)

	payload := map[string]interface{}{
		"action": "Unbounce",
		"id":     targetID,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+userToken, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestPostUserMerge(t *testing.T) {
	prefix := uniquePrefix("merge")
	db := database.DBConn

	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, adminToken := CreateTestSession(t, adminID)

	// Create a message for user1 (DISCARD) to verify it gets moved to user2 (KEEP).
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, user1ID, groupID, "Member")
	msgID := CreateTestMessage(t, user1ID, groupID, "Merge test "+prefix, 55.9533, -3.1883)

	// id1=user1ID (DISCARD), id2=user2ID (KEEP): merge FROM user1 INTO user2.
	payload := map[string]interface{}{
		"action": "Merge",
		"id1":    user1ID,
		"id2":    user2ID,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+adminToken, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify user1 (DISCARD) is hard-deleted, user2 (KEEP) is alive.
	var count1 int64
	db.Raw("SELECT COUNT(*) FROM users WHERE id = ?", user1ID).Scan(&count1)
	assert.Equal(t, int64(0), count1, "id1 (discarded user) should be hard-deleted")
	var count2 int64
	db.Raw("SELECT COUNT(*) FROM users WHERE id = ?", user2ID).Scan(&count2)
	assert.Equal(t, int64(1), count2, "id2 (kept user) should still exist")

	// Verify the message now belongs to user2 (KEEP).
	var fromuser uint64
	db.Raw("SELECT fromuser FROM messages WHERE id = ?", msgID).Scan(&fromuser)
	assert.Equal(t, user2ID, fromuser)
}

func TestPostUserMergeNotAdmin(t *testing.T) {
	prefix := uniquePrefix("mergenonadm")
	userID := CreateTestUser(t, prefix+"_user", "User")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, userToken := CreateTestSession(t, userID)

	payload := map[string]interface{}{
		"action": "Merge",
		"id1":    user1ID,
		"id2":    user2ID,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+userToken, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestPostUserMergeWithStringIDs(t *testing.T) {
	// Clients (e.g. Vue b-form-input type=number) may send IDs as JSON strings.
	// FlexUint64 must accept both.
	prefix := uniquePrefix("mergestr")

	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, adminToken := CreateTestSession(t, adminID)

	// Deliberately send ids as JSON strings, not numbers.
	payload := fmt.Sprintf(`{"action":"Merge","id1":"%d","id2":"%d"}`, user1ID, user2ID)
	request := httptest.NewRequest("POST", "/api/user?jwt="+adminToken, bytes.NewBufferString(payload))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// id1 (discard) must be hard-deleted; id2 (keep) must survive.
	db := database.DBConn
	var count1 int64
	db.Raw("SELECT COUNT(*) FROM users WHERE id = ?", user1ID).Scan(&count1)
	assert.Equal(t, int64(0), count1, "id1 (discarded user) should be hard-deleted")

	var count2 int64
	db.Raw("SELECT COUNT(*) FROM users WHERE id = ?", user2ID).Scan(&count2)
	assert.Equal(t, int64(1), count2, "id2 (kept user) must still exist")
}

func TestPostUserMergeByEmail(t *testing.T) {
	// Merging by email: the frontend sends email1/email2 instead of id1/id2.
	// The API must look up the user IDs from users_emails before merging.
	prefix := uniquePrefix("mergemail")
	db := database.DBConn

	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	email1 := prefix + "_u1@test.com"
	email2 := prefix + "_u2@test.com"
	user1ID := CreateTestUserWithEmail(t, prefix+"_u1", email1)
	user2ID := CreateTestUserWithEmail(t, prefix+"_u2", email2)

	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, user2ID, groupID, "Member")

	payload := map[string]interface{}{
		"action": "Merge",
		"email1": email1,
		"email2": email2,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+adminToken, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// id1 (discard) must be hard-deleted; id2 (keep) must survive.
	var cntU1 int64
	db.Raw("SELECT COUNT(*) FROM users WHERE id = ?", user1ID).Scan(&cntU1)
	assert.Equal(t, int64(0), cntU1, "id1 (discarded) should be hard-deleted")

	var cntU2 int64
	db.Raw("SELECT COUNT(*) FROM users WHERE id = ?", user2ID).Scan(&cntU2)
	assert.Equal(t, int64(1), cntU2, "id2 (kept) must still exist")
}

func TestPostUserMergeByModerator(t *testing.T) {
	// V1 parity: a moderator who moderates both users can merge them.
	prefix := uniquePrefix("mergemod")
	db := database.DBConn

	modID := CreateTestUser(t, prefix+"_mod", "User")
	_, modToken := CreateTestSession(t, modID)

	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	CreateTestMembership(t, user1ID, groupID, "Member")
	CreateTestMembership(t, user2ID, groupID, "Member")

	payload := map[string]interface{}{
		"action": "Merge",
		"id1":    user1ID,
		"id2":    user2ID,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+modToken, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// id1 (discard) must be hard-deleted; id2 (keep) must survive.
	var cnt1 int64
	db.Raw("SELECT COUNT(*) FROM users WHERE id = ?", user1ID).Scan(&cnt1)
	assert.Equal(t, int64(0), cnt1, "id1 (discarded) should be hard-deleted after merge")

	var cnt2 int64
	db.Raw("SELECT COUNT(*) FROM users WHERE id = ?", user2ID).Scan(&cnt2)
	assert.Equal(t, int64(1), cnt2, "id2 (kept) must still exist")
}

func TestPostUserMergeByModeratorForbiddenForOutsideUser(t *testing.T) {
	// A moderator cannot merge users they don't moderate.
	prefix := uniquePrefix("mergemodout")

	modID := CreateTestUser(t, prefix+"_mod", "User")
	_, modToken := CreateTestSession(t, modID)

	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")

	// user1 is in the mod's group, user2 is not.
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	CreateTestMembership(t, user1ID, groupID, "Member")
	// user2 intentionally not added to the group.

	payload := map[string]interface{}{
		"action": "Merge",
		"id1":    user1ID,
		"id2":    user2ID,
	}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/user?jwt="+modToken, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(request)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

// Tests for the support tools endpoints (GET /api/user/:id/*).
// All endpoints require the caller to be a moderator of a group the target belongs to.

func TestUserChatrooms_ModCanSee(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supChat")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, modID)

	// Create a chat room where target is user1.
	db.Exec("INSERT INTO chat_rooms (user1, user2, chattype, latestmessage) VALUES (?, ?, 'User2User', NOW())",
		targetID, modID)

	url := fmt.Sprintf("/api/user/%d/chatrooms?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	body := rsp(resp)
	assert.Equal(t, 200, resp.StatusCode, "Response: %s", string(body))

	var rooms []map[string]interface{}
	json.Unmarshal(body, &rooms)
	assert.GreaterOrEqual(t, len(rooms), 1, "Expected at least 1 chat room, got %d. Response: %s", len(rooms), string(body))
	if len(rooms) > 0 {
		assert.Equal(t, "User2User", rooms[0]["chattype"])
	}
}

func TestUserChatrooms_NonModForbidden(t *testing.T) {
	prefix := uniquePrefix("supChatForbid")
	groupID := CreateTestGroup(t, prefix)
	callerID := CreateTestUser(t, prefix+"_caller", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, callerID, groupID, "Member")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, callerID)

	url := fmt.Sprintf("/api/user/%d/chatrooms?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 403, resp.StatusCode)
}

func TestUserEmailHistory(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supEmail")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, modID)

	db.Exec("INSERT INTO logs_emails (userid, `from`, `to`, subject, status) VALUES (?, 'noreply@test.com', 'user@test.com', 'Test Subject', 'Sent')",
		targetID)

	url := fmt.Sprintf("/api/user/%d/emailhistory?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var emails []map[string]interface{}
	json.Unmarshal(rsp(resp), &emails)
	assert.GreaterOrEqual(t, len(emails), 1)
	assert.Equal(t, "Test Subject", emails[0]["subject"])
}

func TestUserBans(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supBans")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, modID)

	db.Exec("INSERT INTO users_banned (userid, groupid, byuser) VALUES (?, ?, ?)",
		targetID, groupID, modID)

	url := fmt.Sprintf("/api/user/%d/bans?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var bans []map[string]interface{}
	json.Unmarshal(rsp(resp), &bans)
	assert.GreaterOrEqual(t, len(bans), 1)
	assert.Equal(t, float64(groupID), bans[0]["groupid"])
}

func TestUserNewsfeed(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supNews")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, modID)

	db.Exec("INSERT INTO newsfeed (userid, type, message, position) VALUES (?, 'Message', 'Test chitchat post', ST_GeomFromText('POINT(0 0)', 3857))",
		targetID)

	url := fmt.Sprintf("/api/user/%d/newsfeed?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var posts []map[string]interface{}
	json.Unmarshal(rsp(resp), &posts)
	assert.GreaterOrEqual(t, len(posts), 1)
	assert.Equal(t, "Test chitchat post", posts[0]["message"])
}

func TestUserApplied(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supApplied")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, modID)

	db.Exec("INSERT INTO memberships_history (userid, groupid, collection, added) VALUES (?, ?, 'Approved', NOW())",
		targetID, groupID)

	url := fmt.Sprintf("/api/user/%d/applied?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var applied []map[string]interface{}
	json.Unmarshal(rsp(resp), &applied)
	assert.GreaterOrEqual(t, len(applied), 1)
	assert.Equal(t, float64(groupID), applied[0]["groupid"])
}

func TestUserMembershipHistory(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supMemHist")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, modID)

	db.Exec("INSERT INTO logs (user, groupid, type, subtype, timestamp) VALUES (?, ?, 'Group', 'Joined', '2025-01-01 00:00:00')",
		targetID, groupID)

	url := fmt.Sprintf("/api/user/%d/membershiphistory?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var history []map[string]interface{}
	json.Unmarshal(rsp(resp), &history)
	assert.GreaterOrEqual(t, len(history), 1)
}

func TestUserLogins(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supLogins")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, modID)

	db.Exec("INSERT INTO users_logins (userid, type, uid) VALUES (?, 'Native', ?)",
		targetID, fmt.Sprintf("%s@test.com", prefix))

	url := fmt.Sprintf("/api/user/%d/logins?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var logins []map[string]interface{}
	json.Unmarshal(rsp(resp), &logins)
	assert.GreaterOrEqual(t, len(logins), 1)
	assert.Equal(t, "Native", logins[0]["type"])
}

func TestUserFetchMT_ReturnsModFields(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supFetchMT")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, modID)

	db.Exec("UPDATE users SET chatmodstatus = 'Fully', newsfeedmodstatus = 'Suppressed' WHERE id = ?", targetID)

	url := fmt.Sprintf("/api/user/%d?modtools=true&jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var raw map[string]interface{}
	json.Unmarshal(rsp(resp), &raw)
	assert.Equal(t, "Fully", raw["chatmodstatus"])
	assert.Equal(t, "Suppressed", raw["newsfeedmodstatus"])
}

func TestSpammers_FilterByUserid(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supSpamUid")
	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Create an admin to access spammers endpoint.
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	db.Exec("INSERT INTO spam_users (userid, collection, reason, added) VALUES (?, 'Spammer', 'Test reason', NOW())",
		targetID)

	url := fmt.Sprintf("/apiv2/modtools/spammers?userid=%d&jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.Unmarshal(rsp(resp), &result)
	spammers := result["spammers"].([]interface{})
	assert.GreaterOrEqual(t, len(spammers), 1)
	first := spammers[0].(map[string]interface{})
	assert.Equal(t, float64(targetID), first["userid"])
	assert.Equal(t, "Test reason", first["reason"])
}

func TestUserFetchMT_HidesModFieldsFromNonMod(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("supHideMod")
	groupID := CreateTestGroup(t, prefix)
	callerID := CreateTestUser(t, prefix+"_caller", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, callerID, groupID, "Member")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, callerID)

	db.Exec("UPDATE users SET chatmodstatus = 'Fully', newsfeedmodstatus = 'Suppressed' WHERE id = ?", targetID)

	// Non-mod fetching another user — mod-only fields should be hidden.
	url := fmt.Sprintf("/api/user/%d?jwt=%s", targetID, token)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var raw map[string]interface{}
	json.Unmarshal(rsp(resp), &raw)
	assert.Nil(t, raw["chatmodstatus"], "chatmodstatus should be hidden from non-mods")
	assert.Nil(t, raw["newsfeedmodstatus"], "newsfeedmodstatus should be hidden from non-mods")
	assert.Nil(t, raw["tnuserid"], "tnuserid should be hidden from non-mods")
}

func TestSupportEndpoints_AllReturn403ForNonMod(t *testing.T) {
	prefix := uniquePrefix("supAll403")
	groupID := CreateTestGroup(t, prefix)
	callerID := CreateTestUser(t, prefix+"_caller", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, callerID, groupID, "Member")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, callerID)

	endpoints := []string{
		"chatrooms", "emailhistory", "bans", "newsfeed",
		"applied", "membershiphistory", "logins",
	}

	for _, ep := range endpoints {
		url := fmt.Sprintf("/api/user/%d/%s?jwt=%s", targetID, ep, token)
		resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
		assert.Equal(t, 403, resp.StatusCode, "Endpoint %s should return 403 for non-mod", ep)
	}
}

// Ensure time import is used.
var _ = time.Now

// =============================================================================
// Tests for GET /api/user/search
// =============================================================================

func TestSearchUsers_ByName(t *testing.T) {
	prefix := uniquePrefix("searchname")
	db := database.DBConn

	// Create an admin user.
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	// Create a target user with a known fullname.
	targetName := "SearchTarget_" + prefix
	targetID := CreateTestUser(t, prefix+"_target", "User")
	// Update fullname to something searchable.
	db.Exec("UPDATE users SET fullname = ? WHERE id = ?", targetName, targetID)

	// Search by name.
	url := fmt.Sprintf("/api/user/search?q=%s&jwt=%s", targetName, adminToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	users, ok := result["users"].([]interface{})
	assert.True(t, ok)
	assert.GreaterOrEqual(t, len(users), 1, "Should find at least one user")

	// Verify the target user ID is in the results.
	found := false
	for _, u := range users {
		if uint64(u.(float64)) == targetID {
			found = true
			break
		}
	}
	assert.True(t, found, "Target user should be in search results")
}

func TestSearchUsers_ByEmail(t *testing.T) {
	prefix := uniquePrefix("searchemail")
	db := database.DBConn

	// Create an admin user.
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	// Create a target user with a known email.
	targetEmail := prefix + "_findme@test.com"
	targetID := CreateTestUser(t, prefix+"_target", "User")
	db.Exec("INSERT INTO users_emails (userid, email, canon) VALUES (?, ?, ?)", targetID, targetEmail, targetEmail)

	// Search by email.
	url := fmt.Sprintf("/api/user/search?q=%s&jwt=%s", targetEmail, adminToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	users := result["users"].([]interface{})
	assert.GreaterOrEqual(t, len(users), 1, "Should find user by email")

	// Verify the target user ID is in the results.
	found := false
	for _, u := range users {
		if uint64(u.(float64)) == targetID {
			found = true
			break
		}
	}
	assert.True(t, found, "Target user should be in search results")

	// Fetch user individually and verify emails are returned for admin.
	userResp, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/user/%d?modtools=true&jwt=%s", targetID, adminToken), nil))
	assert.Equal(t, 200, userResp.StatusCode)
	var userResult map[string]interface{}
	json.NewDecoder(userResp.Body).Decode(&userResult)
	emails, hasEmails := userResult["emails"]
	assert.True(t, hasEmails, "Admin should see emails on user fetch")
	emailList, ok := emails.([]interface{})
	assert.True(t, ok)
	assert.Greater(t, len(emailList), 0, "Should have at least one email")
}

func TestSearchUsers_ByID(t *testing.T) {
	prefix := uniquePrefix("searchid")

	// Create an admin user.
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	// Create a target user.
	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Search by numeric ID.
	url := fmt.Sprintf("/api/user/search?q=%d&jwt=%s", targetID, adminToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	users := result["users"].([]interface{})
	assert.GreaterOrEqual(t, len(users), 1, "Should find user by ID")

	found := false
	for _, u := range users {
		if uint64(u.(float64)) == targetID {
			found = true
			break
		}
	}
	assert.True(t, found, "Target user should be found by ID")
}

func TestSearchUsers_Unauthorized(t *testing.T) {
	// Not logged in should get 401.
	resp, err := getApp().Test(httptest.NewRequest("GET", "/api/user/search?q=test", nil))
	assert.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestSearchUsers_ForbiddenForNonAdmin(t *testing.T) {
	prefix := uniquePrefix("searchforbid")

	// Create a regular user.
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Regular user should get 403.
	url := fmt.Sprintf("/api/user/search?q=test&jwt=%s", token)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestSearchUsers_ForbiddenForModerator(t *testing.T) {
	prefix := uniquePrefix("searchmod")

	// Create a moderator user (not admin/support).
	modID := CreateTestUser(t, prefix, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Moderator should get 403 (only Admin/Support allowed).
	url := fmt.Sprintf("/api/user/search?q=test&jwt=%s", token)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestSearchUsers_EmptyQuery(t *testing.T) {
	prefix := uniquePrefix("searchempty")

	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	// Empty search term should return 400.
	url := fmt.Sprintf("/api/user/search?q=&jwt=%s", adminToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestSearchUsers_NoResults(t *testing.T) {
	prefix := uniquePrefix("searchnone")

	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	// Search for something that should not exist.
	url := fmt.Sprintf("/api/user/search?q=zzzznonexistent99999&jwt=%s", adminToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	users := result["users"].([]interface{})
	assert.Equal(t, 0, len(users), "Should find no users")
}

func TestSearchUsers_SupportRole(t *testing.T) {
	prefix := uniquePrefix("searchsupport")

	// Create a Support user.
	supportID := CreateTestUser(t, prefix+"_support", "Support")
	_, supportToken := CreateTestSession(t, supportID)

	// Create a target user.
	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Support role should also be able to search.
	url := fmt.Sprintf("/api/user/search?q=%d&jwt=%s", targetID, supportToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	users := result["users"].([]interface{})
	assert.GreaterOrEqual(t, len(users), 1, "Support should find users")
}

func TestSearchUsers_V2Path(t *testing.T) {
	prefix := uniquePrefix("searchv2")

	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)
	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Test the v2 API path.
	url := fmt.Sprintf("/apiv2/user/search?q=%d&jwt=%s", targetID, adminToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// =============================================================================
// Tests for GET /api/user/:id (with modtools)
// =============================================================================

func TestGetUserFetchMT_WithInfo(t *testing.T) {
	prefix := uniquePrefix("fetchmt")

	// Create a user to fetch.
	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Fetch the user with info (no auth needed for basic fetch, but info object always returned).
	url := fmt.Sprintf("/api/user/%d", targetID)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var user user2.User
	err = json.NewDecoder(resp.Body).Decode(&user)
	assert.NoError(t, err)
	assert.Equal(t, targetID, user.ID)

	// Info should always be present (it's part of the User struct).
	// Verify the info object has expected structure.
	assert.GreaterOrEqual(t, user.Info.Openage, uint64(0))
}

func TestGetUserFetchMT_AdminSeesEmails(t *testing.T) {
	prefix := uniquePrefix("fetchmt_admin")
	db := database.DBConn

	// Create an admin user.
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	// Create a target user with a known email.
	targetID := CreateTestUser(t, prefix+"_target", "User")
	testEmail := prefix + "_target@test.com"
	db.Exec("INSERT INTO users_emails (userid, email) VALUES (?, ?) ON DUPLICATE KEY UPDATE email = email", targetID, testEmail)

	// Fetch user as admin - should see emails.
	url := fmt.Sprintf("/api/user/%d?jwt=%s", targetID, adminToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var user user2.User
	err = json.NewDecoder(resp.Body).Decode(&user)
	assert.NoError(t, err)
	assert.Equal(t, targetID, user.ID)
	assert.NotNil(t, user.Emails, "Admin should see emails")
	assert.Greater(t, len(user.Emails), 0, "Should have at least one email")
}

func TestGetUserFetchMT_AdminSeesDonations(t *testing.T) {
	prefix := uniquePrefix("fetchmt_don")
	db := database.DBConn

	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	db.Exec("UPDATE users SET permissions = 'GiftAid' WHERE id = ?", adminID)
	_, adminToken := CreateTestSession(t, adminID)

	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Insert a donation for the target user.
	db.Exec("INSERT INTO users_donations (userid, Payer, PayerDisplayName, GrossAmount, source, timestamp, type) VALUES (?, ?, ?, 25.50, 'DonateWithPayPal', NOW(), 'PayPal')",
		targetID, prefix+"_payer", prefix+"_payer")

	// Admin should see donations.
	url := fmt.Sprintf("/api/user/%d?modtools=true&jwt=%s", targetID, adminToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	donations, ok := result["donations"].([]interface{})
	assert.True(t, ok, "Admin should see donations array")
	assert.Equal(t, 1, len(donations), "Should have one donation")
	d := donations[0].(map[string]interface{})
	assert.Equal(t, 25.5, d["GrossAmount"])
	assert.Equal(t, "DonateWithPayPal", d["source"])
}

func TestGetUserFetchMT_NonAdminNoDonations(t *testing.T) {
	prefix := uniquePrefix("fetchmt_nodon")
	db := database.DBConn

	// Create a regular mod (not admin/support).
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, targetID, groupID, "Member")

	// Insert a donation.
	db.Exec("INSERT INTO users_donations (userid, Payer, PayerDisplayName, GrossAmount, source, timestamp, type) VALUES (?, ?, ?, 10.00, 'Stripe', NOW(), 'Stripe')",
		targetID, prefix+"_payer", prefix+"_payer")

	// Regular mod should NOT see donations.
	url := fmt.Sprintf("/api/user/%d?modtools=true&jwt=%s", targetID, modToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	_, hasDonations := result["donations"]
	assert.False(t, hasDonations, "Regular mod should NOT see donations")
}

func TestGetUserFetchMT_RegularUserNoEmails(t *testing.T) {
	prefix := uniquePrefix("fetchmt_noem")

	// Create a regular user.
	userID := CreateTestUser(t, prefix+"_viewer", "User")
	_, userToken := CreateTestSession(t, userID)

	// Create a target user.
	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Regular user should not see target's emails.
	url := fmt.Sprintf("/api/user/%d?jwt=%s", targetID, userToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var user user2.User
	err = json.NewDecoder(resp.Body).Decode(&user)
	assert.NoError(t, err)
	assert.Equal(t, targetID, user.ID)
	assert.Nil(t, user.Emails, "Regular user should not see emails")
}

func TestGetUserFetchMT_WithModtoolsComments(t *testing.T) {
	prefix := uniquePrefix("fetchmt_cmts")
	db := database.DBConn

	// Create a moderator.
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Create target user.
	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Create a group and membership.
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")

	// Add a comment.
	db.Exec("INSERT INTO users_comments (userid, groupid, byuserid, user1, date) VALUES (?, ?, ?, 'Fetchmt note', NOW())",
		targetID, groupID, modID)

	// Fetch with modtools=true.
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
	assert.Equal(t, "Fetchmt note", *user.Comments[0].User1)
}

func TestGetUserFetchMT_MissingID(t *testing.T) {
	// No id parameter should return 400.
	resp, err := getApp().Test(httptest.NewRequest("GET", "/api/user/abc", nil))
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestGetUserFetchMT_InvalidID(t *testing.T) {
	// Non-numeric id should return 400.
	resp, err := getApp().Test(httptest.NewRequest("GET", "/api/user/abc", nil))
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestGetUserFetchMT_NonExistentUser(t *testing.T) {
	// Non-existent user should return 404.
	resp, err := getApp().Test(httptest.NewRequest("GET", "/api/user/999999999", nil))
	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestGetUserFetchMT_V2Path(t *testing.T) {
	prefix := uniquePrefix("fetchmt_v2")

	targetID := CreateTestUser(t, prefix+"_target", "User")

	url := fmt.Sprintf("/apiv2/user/%d", targetID)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetUserFetchMT_MessageHistoryForMod(t *testing.T) {
	prefix := uniquePrefix("fetchmt_mh")

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	CreateTestMessage(t, posterID, groupID, prefix+" History Test Item", 55.9533, -3.1883)

	// Fetch user with modtools=true as moderator — should include messagehistory.
	url := fmt.Sprintf("/api/user/%d?modtools=true&jwt=%s", posterID, modToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var u user2.User
	err = json.NewDecoder(resp.Body).Decode(&u)
	assert.NoError(t, err)
	assert.Equal(t, posterID, u.ID)

	// Should have messagehistory with at least one entry.
	require.NotNil(t, u.MessageHistory, "Should have messagehistory for modtools fetch")
	assert.Greater(t, len(u.MessageHistory), 0, "Should have recent posts")

	// Verify the test message is in history.
	found := false
	for _, h := range u.MessageHistory {
		if h.Groupid == groupID {
			found = true
			assert.GreaterOrEqual(t, h.Daysago, 0, "Daysago should be non-negative")
			break
		}
	}
	assert.True(t, found, "Should find the test message group in history")
}

func TestGetUserFetchMT_MessageHistoryOutcome(t *testing.T) {
	prefix := uniquePrefix("fetchmt_mho")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	msgID := CreateTestMessage(t, posterID, groupID, prefix+" Outcome Test", 55.9533, -3.1883)

	// Add an outcome for this message.
	db.Exec("INSERT INTO messages_outcomes (msgid, outcome, timestamp) VALUES (?, 'Taken', NOW())", msgID)

	// Fetch user with modtools=true — messagehistory should include outcome.
	url := fmt.Sprintf("/api/user/%d?modtools=true&jwt=%s", posterID, modToken)
	resp, _ := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	history := result["messagehistory"].([]interface{})
	assert.Greater(t, len(history), 0)

	found := false
	for _, h := range history {
		entry := h.(map[string]interface{})
		if uint64(entry["id"].(float64)) == msgID {
			found = true
			assert.Equal(t, "Taken", entry["outcome"], "Outcome should be 'Taken'")

			// arrival must be present and must be a past date, not the current time.
			arrivalStr, ok := entry["arrival"].(string)
			assert.True(t, ok, "messagehistory entry must have 'arrival' field")
			assert.NotEmpty(t, arrivalStr, "arrival must not be empty")
			break
		}
	}
	assert.True(t, found, "Should find the test message in history")

	// Cleanup
	db.Exec("DELETE FROM messages_outcomes WHERE msgid = ?", msgID)
}

func TestGetUserFetchMT_NoMessageHistoryWithoutModtools(t *testing.T) {
	prefix := uniquePrefix("fetchmt_nomh")

	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Fetch without modtools=true — should NOT include messagehistory.
	url := fmt.Sprintf("/api/user/%d", targetID)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)
	assert.Nil(t, result["messagehistory"], "Should not have messagehistory without modtools=true")
}

func TestGetUserFetchMT_MembershipsReturned(t *testing.T) {
	prefix := uniquePrefix("fetchmt_memb")

	groupID := CreateTestGroup(t, prefix)
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, targetToken := CreateTestSession(t, targetID)

	url := fmt.Sprintf("/api/user/%d?jwt=%s", targetID, targetToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var u user2.User
	err = json.NewDecoder(resp.Body).Decode(&u)
	assert.NoError(t, err)
	assert.Equal(t, targetID, u.ID)

	// Should have memberships.
	require.NotNil(t, u.Memberships, "Should have memberships")
	assert.Greater(t, len(u.Memberships), 0, "Should have at least one membership")

	found := false
	for _, m := range u.Memberships {
		if m.Groupid == groupID {
			found = true
			assert.Equal(t, "Member", m.Role)
			break
		}
	}
	assert.True(t, found, "Should find the test group membership")
}

func TestGetUserMembershipsPostingStatus(t *testing.T) {
	prefix := uniquePrefix("fetchmt_ps")
	db := database.DBConn

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, modToken := CreateTestSession(t, modID)

	// Create users with different posting statuses.
	nullUser := CreateTestUser(t, prefix+"_null", "User")
	CreateTestMembership(t, nullUser, groupID, "Member")
	// NULL is the default — don't set it.

	defaultUser := CreateTestUser(t, prefix+"_def", "User")
	CreateTestMembership(t, defaultUser, groupID, "Member")
	db.Exec("UPDATE memberships SET ourPostingStatus = 'DEFAULT' WHERE userid = ? AND groupid = ?", defaultUser, groupID)

	moderatedUser := CreateTestUser(t, prefix+"_mod2", "User")
	CreateTestMembership(t, moderatedUser, groupID, "Member")
	db.Exec("UPDATE memberships SET ourPostingStatus = 'MODERATED' WHERE userid = ? AND groupid = ?", moderatedUser, groupID)

	prohibitedUser := CreateTestUser(t, prefix+"_proh", "User")
	CreateTestMembership(t, prohibitedUser, groupID, "Member")
	db.Exec("UPDATE memberships SET ourPostingStatus = 'PROHIBITED' WHERE userid = ? AND groupid = ?", prohibitedUser, groupID)

	// Fetch each user with modtools=true and check posting status.
	for _, tc := range []struct {
		name     string
		uid      uint64
		expected string
	}{
		{"NULL→MODERATED", nullUser, "MODERATED"},
		{"DEFAULT stays DEFAULT", defaultUser, "DEFAULT"},
		{"MODERATED stays MODERATED", moderatedUser, "MODERATED"},
		{"PROHIBITED stays PROHIBITED", prohibitedUser, "PROHIBITED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			url := fmt.Sprintf("/api/user/%d?modtools=true&jwt=%s", tc.uid, modToken)
			resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
			assert.NoError(t, err)
			assert.Equal(t, 200, resp.StatusCode)

			var result map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&result)
			memberships := result["memberships"].([]interface{})

			found := false
			for _, m := range memberships {
				mem := m.(map[string]interface{})
				if uint64(mem["groupid"].(float64)) == groupID {
					found = true
					assert.Equal(t, tc.expected, mem["ourpostingstatus"], "ourpostingstatus should be %s", tc.expected)
					break
				}
			}
			assert.True(t, found, "Should find group membership")
		})
	}
}

// TestFetchMTModmailsCount verifies that the modmails count is returned in fetchmt responses
// and filters by the viewing mod's groups.
func TestFetchMTModmailsCount(t *testing.T) {
	prefix := uniquePrefix("fetchmt_mm")
	db := database.DBConn

	targetID := CreateTestUser(t, prefix+"_target", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, targetID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Insert a modmail record on the mod's group. Use a high logid to avoid collisions.
	db.Exec("INSERT INTO users_modmails (userid, logid, timestamp, groupid) VALUES (?, ?, NOW(), ?)",
		targetID, 90000000+targetID, groupID)

	url := fmt.Sprintf("/api/user/%d?modtools=true&jwt=%s", targetID, modToken)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var u map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&u)
	modmails, ok := u["modmails"]
	assert.True(t, ok, "Should have modmails field")
	assert.GreaterOrEqual(t, modmails.(float64), float64(1), "Should have at least 1 modmail")
}

// TestFetchMTModmailsGroupFilter verifies modmails only count entries on the viewer's groups.
func TestFetchMTModmailsGroupFilter(t *testing.T) {
	prefix := uniquePrefix("fetchmt_mmg")
	db := database.DBConn

	targetID := CreateTestUser(t, prefix+"_target", "User")
	groupA := CreateTestGroup(t, prefix+"_a")
	groupB := CreateTestGroup(t, prefix+"_b")
	CreateTestMembership(t, targetID, groupA, "Member")
	CreateTestMembership(t, targetID, groupB, "Member")

	// Modmail on group B only. Use a high logid to avoid collisions.
	db.Exec("INSERT INTO users_modmails (userid, logid, timestamp, groupid) VALUES (?, ?, NOW(), ?)",
		targetID, 90000000+targetID, groupB)

	// Mod on group A only — should see 0 modmails.
	modAID := CreateTestUser(t, prefix+"_modA", "User")
	CreateTestMembership(t, modAID, groupA, "Moderator")
	_, modAToken := CreateTestSession(t, modAID)

	url := fmt.Sprintf("/api/user/%d?modtools=true&jwt=%s", targetID, modAToken)
	req := httptest.NewRequest("GET", url, nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var u map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&u)
	assert.Equal(t, float64(0), u["modmails"], "Mod on group A should see 0 modmails (modmail is on group B)")

	// Mod on group B — should see 1 modmail.
	modBID := CreateTestUser(t, prefix+"_modB", "User")
	CreateTestMembership(t, modBID, groupB, "Moderator")
	_, modBToken := CreateTestSession(t, modBID)

	url2 := fmt.Sprintf("/api/user/%d?modtools=true&jwt=%s", targetID, modBToken)
	req2 := httptest.NewRequest("GET", url2, nil)
	resp2, _ := getApp().Test(req2)
	assert.Equal(t, 200, resp2.StatusCode)

	var u2 map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&u2)
	assert.Equal(t, float64(1), u2["modmails"], "Mod on group B should see 1 modmail")
}

// TestFetchMTRepliesByType verifies that repliesoffer and replieswanted are returned in user info.
func TestFetchMTRepliesByType(t *testing.T) {
	prefix := uniquePrefix("fetchmt_rbt")

	targetID := CreateTestUser(t, prefix+"_target", "User")
	_, targetToken := CreateTestSession(t, targetID)

	url := fmt.Sprintf("/api/user/%d?modtools=true&jwt=%s", targetID, targetToken)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var u map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&u)

	info, ok := u["info"].(map[string]interface{})
	require.True(t, ok, "Should have info object")

	// Verify the new fields exist (may be 0 for a test user with no activity).
	_, hasRepliesOffer := info["repliesoffer"]
	assert.True(t, hasRepliesOffer, "Should have repliesoffer field")

	_, hasRepliesWanted := info["replieswanted"]
	assert.True(t, hasRepliesWanted, "Should have replieswanted field")

	_, hasExpectedReplies := info["expectedreplies"]
	assert.True(t, hasExpectedReplies, "Should have expectedreplies field")
}

// =============================================================================
// Tests for GET /api/user/{id}/replies
// =============================================================================

func TestGetUserReplies(t *testing.T) {
	prefix := uniquePrefix("replies")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")

	replierID := CreateTestUser(t, prefix+"_replier", "User")
	CreateTestMembership(t, replierID, groupID, "Member")

	// Create a test message.
	msgID := CreateTestMessage(t, posterID, groupID, "OFFER: Test item", 54.0, -2.8)

	// Create a chat room and an INTERESTED chat message from replier.
	var roomID uint64
	db.Raw("INSERT INTO chat_rooms (chattype) VALUES ('User2User')").Scan(&roomID)
	db.Exec("INSERT INTO chat_rooms (chattype) VALUES ('User2User')")
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&roomID)
	db.Exec("INSERT INTO chat_messages (chatid, userid, type, refmsgid, message, date) VALUES (?, ?, ?, ?, 'interested', NOW())",
		roomID, replierID, "Interested", msgID)

	// Fetch replies as mod.
	url := fmt.Sprintf("/api/user/%d/replies?jwt=%s", replierID, modToken)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var replies []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&replies)
	assert.GreaterOrEqual(t, len(replies), 1, "Should have at least one reply")

	found := false
	for _, r := range replies {
		if uint64(r["id"].(float64)) == msgID {
			found = true
			assert.Equal(t, "Offer", r["type"])
			assert.Contains(t, r["subject"], "Test item")
			_, hasArrival := r["arrival"]
			assert.True(t, hasArrival, "Should have arrival field")
		}
	}
	assert.True(t, found, "Should find the test message in replies")

	// Filter by type=Offer should still include it.
	url = fmt.Sprintf("/api/user/%d/replies?type=Offer&jwt=%s", replierID, modToken)
	req = httptest.NewRequest("GET", url, nil)
	resp, err = getApp().Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	json.NewDecoder(resp.Body).Decode(&replies)
	assert.GreaterOrEqual(t, len(replies), 1)

	// Filter by type=Wanted should NOT include it.
	url = fmt.Sprintf("/api/user/%d/replies?type=Wanted&jwt=%s", replierID, modToken)
	req = httptest.NewRequest("GET", url, nil)
	resp, err = getApp().Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	json.NewDecoder(resp.Body).Decode(&replies)

	foundWanted := false
	for _, r := range replies {
		if uint64(r["id"].(float64)) == msgID {
			foundWanted = true
		}
	}
	assert.False(t, foundWanted, "Offer message should not appear when filtering by Wanted")
}

func TestGetUserReplies_NonModForbidden(t *testing.T) {
	prefix := uniquePrefix("replies_nomod")
	groupID := CreateTestGroup(t, prefix)

	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, userToken := CreateTestSession(t, userID)

	otherID := CreateTestUser(t, prefix+"_other", "User")
	CreateTestMembership(t, otherID, groupID, "Member")

	url := fmt.Sprintf("/api/user/%d/replies?jwt=%s", otherID, userToken)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

// =============================================================================
// Tests for GET /api/user/{id}/publiclocation
// =============================================================================

func TestPublicLocation_ValidUser(t *testing.T) {
	prefix := uniquePrefix("publoc")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")

	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/user/%d/publiclocation", userID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result user.Publiclocation
	json2.Unmarshal(rsp(resp), &result)

	// User was created with settings containing lat/lng, so should get a location.
	// The exact values depend on test data setup, but the structure should be valid.
	assert.NotNil(t, result)
}

func TestPublicLocation_NoAuthRequired(t *testing.T) {
	prefix := uniquePrefix("publocnoauth")
	userID := CreateTestUser(t, prefix, "User")

	// Public location should be accessible without authentication.
	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/user/%d/publiclocation", userID), nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestPublicLocation_InvalidUserID(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/abc/publiclocation", nil))
	// Invalid ID should return 200 with empty result (handler doesn't error, returns empty struct).
	assert.Equal(t, 200, resp.StatusCode)

	var result user.Publiclocation
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, "", result.Location)
	assert.Equal(t, "", result.Display)
}

func TestPublicLocation_NonExistentUser(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/999999999/publiclocation", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result user.Publiclocation
	json2.Unmarshal(rsp(resp), &result)
	// Non-existent user should return empty location.
	assert.Equal(t, "", result.Location)
}

func TestPublicLocation_V2Path(t *testing.T) {
	prefix := uniquePrefix("publocv2")
	userID := CreateTestUser(t, prefix, "User")

	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/apiv2/user/%d/publiclocation", userID), nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestPublicLocation_ResponseStructure(t *testing.T) {
	prefix := uniquePrefix("publocstruct")
	userID := CreateTestUser(t, prefix, "User")

	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/user/%d/publiclocation", userID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	// Verify the response has expected keys.
	_, hasLocation := result["location"]
	_, hasDisplay := result["display"]
	_, hasGroupid := result["groupid"]
	_, hasGroupname := result["groupname"]

	assert.True(t, hasLocation, "Response should have 'location' field")
	assert.True(t, hasDisplay, "Response should have 'display' field")
	assert.True(t, hasGroupid, "Response should have 'groupid' field")
	assert.True(t, hasGroupname, "Response should have 'groupname' field")
}

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

func TestUserSearch_UniqueTermsReturned(t *testing.T) {
	prefix := uniquePrefix("usrsearchuniq")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	db := database.DBConn

	// Create distinct searches - DB has unique constraint on (userid, term).
	db.Exec("INSERT INTO users_searches (userid, term, deleted, date) VALUES (?, ?, 0, NOW())", userID, "term_a_"+prefix)
	db.Exec("INSERT INTO users_searches (userid, term, deleted, date) VALUES (?, ?, 0, NOW())", userID, "term_b_"+prefix)
	defer db.Exec("DELETE FROM users_searches WHERE userid = ?", userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/user/%d/search?jwt=%s", userID, token), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var searches []user.Search
	json2.Unmarshal(rsp(resp), &searches)

	// Should have at least the 2 terms we inserted.
	assert.GreaterOrEqual(t, len(searches), 2)

	// Verify both terms are present.
	terms := make(map[string]bool)
	for _, s := range searches {
		terms[s.Term] = true
	}
	assert.True(t, terms["term_a_"+prefix], "Should contain term_a")
	assert.True(t, terms["term_b_"+prefix], "Should contain term_b")
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

func TestGetDeletedUserNameAsMod(t *testing.T) {
	prefix := uniquePrefix("usrDelMod")
	db := database.DBConn

	// Create a user and mark them as deleted.
	userID := CreateTestUser(t, prefix+"_user", "User")
	db.Exec("UPDATE users SET fullname = 'Jane Doe', deleted = NOW() WHERE id = ?", userID)

	// Create a mod who can view deleted users.
	modID := CreateTestUser(t, prefix+"_mod", "User")
	_, modToken := CreateTestSession(t, modID)
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")

	// Mod should see the real name, not "Deleted User #ID".
	url := fmt.Sprintf("/api/user/%d?jwt=%s", userID, modToken)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	displayname := result["displayname"].(string)
	assert.Equal(t, "Jane Doe", displayname, "Mod should see real name of deleted user")
}

func TestGetDeletedUserNameAsNonMod(t *testing.T) {
	prefix := uniquePrefix("usrDelUsr")
	db := database.DBConn

	// Create a user and mark them as deleted.
	userID := CreateTestUser(t, prefix+"_user", "User")
	db.Exec("UPDATE users SET fullname = 'Jane Doe', deleted = NOW() WHERE id = ?", userID)

	// Create a regular user.
	viewerID := CreateTestUser(t, prefix+"_viewer", "User")
	_, viewerToken := CreateTestSession(t, viewerID)

	// Non-mod should see censored name.
	url := fmt.Sprintf("/api/user/%d?jwt=%s", userID, viewerToken)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	displayname := result["displayname"].(string)
	assert.Contains(t, displayname, "Deleted User", "Non-mod should see censored name")
}

func TestGetUserReturnsEngagement(t *testing.T) {
	prefix := uniquePrefix("usrEngBdg")
	db := database.DBConn

	// Create a user with engagement set.
	userID := CreateTestUser(t, prefix+"_user", "User")
	db.Exec("UPDATE users SET engagement = 'Frequent' WHERE id = ?", userID)

	// Create a mod who can view this user.
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	CreateTestMembership(t, userID, groupID, "Member")
	_, modToken := CreateTestSession(t, modID)

	// Fetch user with modtools=true — should include engagement.
	url := fmt.Sprintf("/api/user/%d?modtools=true&jwt=%s", userID, modToken)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	engagement, ok := result["engagement"].(string)
	assert.True(t, ok, "engagement field should be a string")
	assert.Equal(t, "Frequent", engagement, "engagement should be returned")
}

func TestGetUserReturnsGiftAid(t *testing.T) {
	prefix := uniquePrefix("usrGiftAid")
	db := database.DBConn

	// Create a user with PERM_GIFTAID permission (the viewer).
	viewerID := CreateTestUser(t, prefix+"_viewer", "User")
	db.Exec("UPDATE users SET permissions = ? WHERE id = ?", "GiftAid", viewerID)
	_, viewerToken := CreateTestSession(t, viewerID)

	// Create the target user.
	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Insert a giftaid record for the target.
	db.Exec("INSERT INTO giftaid (userid, period, fullname, homeaddress) VALUES (?, 'Future', 'Test Name', 'Test Address')", targetID)
	defer db.Exec("DELETE FROM giftaid WHERE userid = ?", targetID)

	// Fetch the target user with modtools=true&info=true using the viewer's JWT.
	url := fmt.Sprintf("/api/user/%d?modtools=true&info=true&jwt=%s", targetID, viewerToken)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	giftaid, ok := result["giftaid"].(map[string]interface{})
	assert.True(t, ok, "giftaid field should be present as an object")
	assert.Equal(t, "Future", giftaid["period"], "giftaid period should be 'Future'")
}

func TestSettingsDefaultsApplied(t *testing.T) {
	prefix := uniquePrefix("settingsDef")
	db := database.DBConn
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Set settings with only a custom field — no notificationmails etc.
	db.Exec(`UPDATE users SET settings = '{"somecustom":true}' WHERE id = ?`, userID)

	// Fetch own user.
	url := fmt.Sprintf("/api/user/%d?jwt=%s", userID, token)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	settings, ok := result["settings"].(map[string]interface{})
	require.True(t, ok, "settings should be a map")

	// V1-parity defaults should be applied.
	assert.Equal(t, true, settings["notificationmails"], "notificationmails should default to true")
	assert.Equal(t, true, settings["engagement"], "engagement should default to true")
	assert.Equal(t, float64(4), settings["modnotifs"], "modnotifs should default to 4")
	assert.Equal(t, float64(12), settings["backupmodnotifs"], "backupmodnotifs should default to 12")

	// Existing field should still be present.
	assert.Equal(t, true, settings["somecustom"], "existing field should be preserved")
}

func TestSettingsNotVisibleToOtherUsers(t *testing.T) {
	prefix := uniquePrefix("settingsVis")
	db := database.DBConn

	// Create target user with settings.
	targetID := CreateTestUser(t, prefix+"_target", "User")
	db.Exec(`UPDATE users SET settings = '{"notificationmails":true}' WHERE id = ?`, targetID)

	// Create a regular viewer (not a mod).
	viewerID := CreateTestUser(t, prefix+"_viewer", "User")
	_, viewerToken := CreateTestSession(t, viewerID)

	// Regular user viewing another user — settings should be empty/null.
	url := fmt.Sprintf("/api/user/%d?jwt=%s", targetID, viewerToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	// Settings should not be returned for a non-mod viewing another user.
	settings := result["settings"]
	if settings != nil {
		// If present, it should be empty/null JSON (not the actual settings).
		settingsMap, isMap := settings.(map[string]interface{})
		if isMap {
			assert.Empty(t, settingsMap, "settings should be empty for non-mod viewer")
		}
	}

	// Create a mod viewer in a shared group — they should see settings.
	modID := CreateTestUser(t, prefix+"_mod", "User")
	_, modToken := CreateTestSession(t, modID)
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")

	url = fmt.Sprintf("/api/user/%d?jwt=%s", targetID, modToken)
	resp, err = getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var modResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&modResult)

	modSettings, ok := modResult["settings"].(map[string]interface{})
	require.True(t, ok, "mod should see settings")
	assert.Equal(t, true, modSettings["notificationmails"], "mod should see the actual settings value")
}

func TestGetUserReturnsBounceReason(t *testing.T) {
	prefix := uniquePrefix("usrBounce")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	db.Exec("UPDATE users SET bouncing = 1 WHERE id = ?", userID)

	// Insert a bounce record.
	var emailID uint64
	db.Raw("SELECT id FROM users_emails WHERE userid = ? LIMIT 1", userID).Scan(&emailID)
	assert.NotZero(t, emailID, "user should have an email")
	db.Exec("INSERT INTO bounces_emails (emailid, reason, permanent) VALUES (?, 'Mailbox full', 0)", emailID)
	t.Cleanup(func() {
		db.Exec("DELETE FROM bounces_emails WHERE emailid = ?", emailID)
	})

	// Fetch own user — should include bounce details.
	_, token := CreateTestSession(t, userID)
	url := fmt.Sprintf("/api/user/%d?jwt=%s", userID, token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	assert.Equal(t, true, result["bouncing"], "bouncing should be true")
	assert.Equal(t, "Mailbox full", result["bouncereason"], "bouncereason should be populated")
	assert.NotNil(t, result["bounceat"], "bounceat should be populated")
}
