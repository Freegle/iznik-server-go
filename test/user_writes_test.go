package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

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
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

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

// =============================================================================
// DELETE /user tests
// =============================================================================

func TestDeleteUserSelf(t *testing.T) {
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

func TestDeleteUserAdmin(t *testing.T) {
	prefix := uniquePrefix("deladmin")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	_, adminToken := CreateTestSession(t, adminID)

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
	db := database.DBConn
	var deleted *string
	db.Raw("SELECT deleted FROM users WHERE id = ?", targetID).Scan(&deleted)
	assert.NotNil(t, deleted)
}

func TestDeleteUserNotAdmin(t *testing.T) {
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

	// Create a message for user2 to verify it gets moved to user1.
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, user2ID, groupID, "Member")
	msgID := CreateTestMessage(t, user2ID, groupID, "Merge test "+prefix, 55.9533, -3.1883)

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

	// Verify user2 is marked as deleted.
	var deleted *string
	db.Raw("SELECT deleted FROM users WHERE id = ?", user2ID).Scan(&deleted)
	assert.NotNil(t, deleted)

	// Verify the message now belongs to user1.
	var fromuser uint64
	db.Raw("SELECT fromuser FROM messages WHERE id = ?", msgID).Scan(&fromuser)
	assert.Equal(t, user1ID, fromuser)
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
