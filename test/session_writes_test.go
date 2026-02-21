package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
)

// ---------------------------------------------------------------------------
// GET /session
// ---------------------------------------------------------------------------

func TestGetSession(t *testing.T) {
	prefix := uniquePrefix("get_sess")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	req := httptest.NewRequest("GET", "/api/session?jwt="+token, nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])

	// me should contain user info.
	me, ok := result["me"].(map[string]interface{})
	assert.True(t, ok, "me should be a map")
	assert.Equal(t, float64(userID), me["id"])
	assert.NotEmpty(t, me["systemrole"])

	// groups should be an array with at least one entry.
	groups, ok := result["groups"].([]interface{})
	assert.True(t, ok, "groups should be an array")
	assert.GreaterOrEqual(t, len(groups), 1)

	// emails should be an array with at least one entry.
	emails, ok := result["emails"].([]interface{})
	assert.True(t, ok, "emails should be an array")
	assert.GreaterOrEqual(t, len(emails), 1)

	// jwt should be present.
	assert.NotEmpty(t, result["jwt"])

	// persistent should be present.
	assert.NotNil(t, result["persistent"])
}

func TestGetSessionNotLoggedIn(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/session", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(1), result["ret"])
	assert.Equal(t, "Not logged in", result["status"])
}

// ---------------------------------------------------------------------------
// POST /session - Email/Password Login
// ---------------------------------------------------------------------------

func TestLoginEmailPassword(t *testing.T) {
	prefix := uniquePrefix("login_ep")
	email := fmt.Sprintf("%s@test.com", prefix)
	userID := CreateTestUser(t, prefix, "User")

	// Create a password hash and store it.
	db := database.DBConn
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("testpassword"), bcrypt.MinCost)
	assert.NoError(t, err)
	db.Exec("INSERT INTO users_logins (userid, type, uid, credentials) VALUES (?, 'Native', ?, ?)",
		userID, strconv.FormatUint(userID, 10), string(hashedPassword))

	body, _ := json.Marshal(map[string]interface{}{
		"email":    email,
		"password": "testpassword",
	})

	req := httptest.NewRequest("POST", "/api/session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 5000)
	assert.NoError(t, err, "Request should not timeout")
	if resp == nil {
		t.Fatal("Response is nil")
	}
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])
	assert.NotEmpty(t, result["jwt"], "Should return a JWT")
	assert.NotNil(t, result["persistent"], "Should return persistent token")

	// Verify a session was created.
	var sessionCount int64
	db.Raw("SELECT COUNT(*) FROM sessions WHERE userid = ?", userID).Scan(&sessionCount)
	assert.GreaterOrEqual(t, sessionCount, int64(1))
}

func TestLoginWrongPassword(t *testing.T) {
	prefix := uniquePrefix("login_wrongpw")
	email := fmt.Sprintf("%s@test.com", prefix)
	userID := CreateTestUser(t, prefix, "User")

	// Create a password hash.
	db := database.DBConn
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("correctpassword"), bcrypt.MinCost)
	db.Exec("INSERT INTO users_logins (userid, type, uid, credentials) VALUES (?, 'Native', ?, ?)",
		userID, strconv.FormatUint(userID, 10), string(hashedPassword))

	body, _ := json.Marshal(map[string]interface{}{
		"email":    email,
		"password": "wrongpassword",
	})

	req := httptest.NewRequest("POST", "/api/session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 5000)
	assert.NoError(t, err, "Request should not timeout")
	if resp == nil {
		t.Fatal("Response is nil")
	}
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(3), result["ret"])
	assert.Contains(t, result["status"], "password")
}

func TestLoginUnknownEmail(t *testing.T) {
	body, _ := json.Marshal(map[string]interface{}{
		"email":    "nonexistent-login-test-9999@example.com",
		"password": "whatever",
	})

	req := httptest.NewRequest("POST", "/api/session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(2), result["ret"])
	assert.Contains(t, result["status"], "email")
}

// ---------------------------------------------------------------------------
// POST /session - Link Login
// ---------------------------------------------------------------------------

func TestLoginLinkKey(t *testing.T) {
	prefix := uniquePrefix("login_link")
	userID := CreateTestUser(t, prefix, "User")

	// Use the LostPassword flow to create a Link login key.
	email := fmt.Sprintf("%s@test.com", prefix)
	lpBody, _ := json.Marshal(map[string]interface{}{
		"action": "LostPassword",
		"email":  email,
	})
	lpReq := httptest.NewRequest("POST", "/api/session", bytes.NewReader(lpBody))
	lpReq.Header.Set("Content-Type", "application/json")
	getApp().Test(lpReq)

	// Get the link key from DB.
	db := database.DBConn
	var linkKey string
	db.Raw("SELECT credentials FROM users_logins WHERE userid = ? AND type = 'Link' LIMIT 1", userID).Scan(&linkKey)
	assert.NotEmpty(t, linkKey, "Link key should have been created")

	// Login using u + k.
	body, _ := json.Marshal(map[string]interface{}{
		"u": userID,
		"k": linkKey,
	})

	req := httptest.NewRequest("POST", "/api/session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])
	assert.NotEmpty(t, result["jwt"])
	assert.NotNil(t, result["persistent"])
}

// ---------------------------------------------------------------------------
// PATCH /session
// ---------------------------------------------------------------------------

func TestPatchSessionDisplayname(t *testing.T) {
	prefix := uniquePrefix("patch_dname")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"displayname": "New Display Name",
	})

	req := httptest.NewRequest("PATCH", "/api/session?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify the update in DB.
	db := database.DBConn
	var fullname string
	db.Raw("SELECT fullname FROM users WHERE id = ?", userID).Scan(&fullname)
	assert.Equal(t, "New Display Name", fullname)

	// firstname and lastname should be NULL.
	var firstname, lastname *string
	db.Raw("SELECT firstname FROM users WHERE id = ?", userID).Scan(&firstname)
	db.Raw("SELECT lastname FROM users WHERE id = ?", userID).Scan(&lastname)
	assert.Nil(t, firstname)
	assert.Nil(t, lastname)
}

func TestPatchSessionSettings(t *testing.T) {
	prefix := uniquePrefix("patch_settings")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	newSettings := `{"notifications":{"push":true},"email":"daily"}`
	body, _ := json.Marshal(map[string]interface{}{
		"settings": json.RawMessage(newSettings),
	})

	req := httptest.NewRequest("PATCH", "/api/session?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify settings were updated.
	db := database.DBConn
	var settings string
	db.Raw("SELECT settings FROM users WHERE id = ?", userID).Scan(&settings)
	assert.Contains(t, settings, `"email":"daily"`)
}

func TestPatchSessionOnHoliday(t *testing.T) {
	prefix := uniquePrefix("patch_holiday")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"onholidaytill": "2026-03-01",
	})

	req := httptest.NewRequest("PATCH", "/api/session?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify the update in DB.
	db := database.DBConn
	var onholidaytill string
	db.Raw("SELECT onholidaytill FROM users WHERE id = ?", userID).Scan(&onholidaytill)
	assert.Contains(t, onholidaytill, "2026-03-01")
}

func TestPatchSessionNotLoggedIn(t *testing.T) {
	body, _ := json.Marshal(map[string]interface{}{
		"displayname": "Should Fail",
	})

	req := httptest.NewRequest("PATCH", "/api/session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// DELETE /session
// ---------------------------------------------------------------------------

func TestDeleteSession(t *testing.T) {
	prefix := uniquePrefix("del_sess")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Verify session exists.
	db := database.DBConn
	var countBefore int64
	db.Raw("SELECT COUNT(*) FROM sessions WHERE userid = ?", userID).Scan(&countBefore)
	assert.GreaterOrEqual(t, countBefore, int64(1))

	req := httptest.NewRequest("DELETE", "/api/session?jwt="+token, nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify session was destroyed.
	var countAfter int64
	db.Raw("SELECT COUNT(*) FROM sessions WHERE userid = ?", userID).Scan(&countAfter)
	assert.Equal(t, int64(0), countAfter)
}

// ---------------------------------------------------------------------------
// POST /session - Forget
// ---------------------------------------------------------------------------

func TestPostSessionForget(t *testing.T) {
	prefix := uniquePrefix("forget")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"action": "Forget",
	})

	req := httptest.NewRequest("POST", "/api/session?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])

	// Verify user is marked as deleted.
	db := database.DBConn
	var deleted *string
	db.Raw("SELECT deleted FROM users WHERE id = ?", userID).Scan(&deleted)
	assert.NotNil(t, deleted, "User should be marked as deleted")

	// Verify session was destroyed.
	var sessionCount int64
	db.Raw("SELECT COUNT(*) FROM sessions WHERE userid = ?", userID).Scan(&sessionCount)
	assert.Equal(t, int64(0), sessionCount)
}

func TestPostSessionForgetMod(t *testing.T) {
	prefix := uniquePrefix("forget_mod")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Moderator")
	_, token := CreateTestSession(t, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"action": "Forget",
	})

	req := httptest.NewRequest("POST", "/api/session?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(2), result["ret"])
	assert.Contains(t, result["status"], "demote")

	// Verify user is NOT deleted.
	db := database.DBConn
	var deleted *string
	db.Raw("SELECT deleted FROM users WHERE id = ?", userID).Scan(&deleted)
	assert.Nil(t, deleted, "Moderator should not be deleted")
}

// ---------------------------------------------------------------------------
// POST /session - Related
// ---------------------------------------------------------------------------

func TestPostSessionRelated(t *testing.T) {
	prefix := uniquePrefix("related")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	otherID := CreateTestUser(t, prefix+"_other", "User")

	body, _ := json.Marshal(map[string]interface{}{
		"action":   "Related",
		"userlist": []uint64{otherID},
	})

	req := httptest.NewRequest("POST", "/api/session?jwt="+token, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify the related record was created.
	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_related WHERE user1 = ? AND user2 = ?", userID, otherID).Scan(&count)
	assert.Equal(t, int64(1), count)
}
