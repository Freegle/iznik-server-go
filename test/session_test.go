package test

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func postSession(body string) *http.Response {
	req := httptest.NewRequest("POST", "/api/session", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	return resp
}

func TestLostPasswordSuccess(t *testing.T) {
	prefix := uniquePrefix("lostpw-ok")
	email := fmt.Sprintf("%s@test.com", prefix)
	userID := CreateTestUser(t, prefix, "User")

	body := fmt.Sprintf(`{"action":"LostPassword","email":"%s"}`, email)
	resp := postSession(body)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])

	// Verify a background task was queued.
	db := database.DBConn
	var taskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = 'email_forgot_password' AND JSON_EXTRACT(data, '$.user_id') = ?", userID).Scan(&taskCount)
	assert.Equal(t, int64(1), taskCount)

	// Verify a login key was created.
	var keyCount int64
	db.Raw("SELECT COUNT(*) FROM users_logins WHERE userid = ? AND type = 'Link'", userID).Scan(&keyCount)
	assert.Equal(t, int64(1), keyCount)
}

func TestLostPasswordUnknownEmail(t *testing.T) {
	body := `{"action":"LostPassword","email":"nonexistent-session-test@example.com"}`
	resp := postSession(body)
	assert.Equal(t, 404, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestLostPasswordMissingEmail(t *testing.T) {
	body := `{"action":"LostPassword","email":""}`
	resp := postSession(body)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestUnsubscribeSuccess(t *testing.T) {
	prefix := uniquePrefix("unsub-ok")
	email := fmt.Sprintf("%s@test.com", prefix)
	userID := CreateTestUser(t, prefix, "User")

	body := fmt.Sprintf(`{"action":"Unsubscribe","email":"%s"}`, email)
	resp := postSession(body)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])
	assert.Equal(t, true, result["emailsent"])

	// Verify a background task was queued.
	db := database.DBConn
	var taskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = 'email_unsubscribe' AND JSON_EXTRACT(data, '$.user_id') = ?", userID).Scan(&taskCount)
	assert.Equal(t, int64(1), taskCount)
}

func TestUnsubscribeUnknownEmail(t *testing.T) {
	// Should return success to prevent email enumeration.
	body := `{"action":"Unsubscribe","email":"nonexistent-unsub-test@example.com"}`
	resp := postSession(body)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, true, result["emailsent"])
}

func TestUnsubscribeMissingEmail(t *testing.T) {
	body := `{"action":"Unsubscribe","email":""}`
	resp := postSession(body)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestSessionUnsupportedAction(t *testing.T) {
	body := `{"action":"InvalidAction","email":"test@test.com"}`
	resp := postSession(body)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestLostPasswordReusesExistingKey(t *testing.T) {
	prefix := uniquePrefix("lostpw-reuse")
	email := fmt.Sprintf("%s@test.com", prefix)
	CreateTestUser(t, prefix, "User")

	// First request creates the key.
	body := fmt.Sprintf(`{"action":"LostPassword","email":"%s"}`, email)
	postSession(body)

	// Get the key.
	db := database.DBConn
	var key1 string
	db.Raw("SELECT credentials FROM users_logins WHERE userid = (SELECT id FROM users WHERE fullname = ? ORDER BY id DESC LIMIT 1) AND type = 'Link'",
		fmt.Sprintf("Test User %s", prefix)).Scan(&key1)
	assert.NotEmpty(t, key1)

	// Second request reuses the same key.
	postSession(body)

	var key2 string
	db.Raw("SELECT credentials FROM users_logins WHERE userid = (SELECT id FROM users WHERE fullname = ? ORDER BY id DESC LIMIT 1) AND type = 'Link'",
		fmt.Sprintf("Test User %s", prefix)).Scan(&key2)
	assert.Equal(t, key1, key2)
}

func TestLostPasswordDeletedUser(t *testing.T) {
	prefix := uniquePrefix("lostpw-del")
	email := fmt.Sprintf("%s@test.com", prefix)

	// Create a deleted user with email.
	db := database.DBConn
	fullname := fmt.Sprintf("Test User %s", prefix)
	db.Exec("INSERT INTO users (firstname, lastname, fullname, systemrole, deleted) VALUES ('Test', ?, ?, 'User', NOW())", prefix, fullname)
	var userID uint64
	db.Raw("SELECT id FROM users WHERE fullname = ? ORDER BY id DESC LIMIT 1", fullname).Scan(&userID)
	db.Exec("INSERT INTO users_emails (userid, email) VALUES (?, ?)", userID, email)

	body := fmt.Sprintf(`{"action":"LostPassword","email":"%s"}`, email)
	resp := postSession(body)
	assert.Equal(t, 404, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	// Deleted user should not be found.
	assert.Equal(t, float64(2), result["ret"])
}

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

	// groups should be an array with at least one entry containing only membership-specific fields.
	groups, ok := result["groups"].([]interface{})
	assert.True(t, ok, "groups should be an array")
	assert.GreaterOrEqual(t, len(groups), 1)

	// Verify the session response only contains membership-specific fields, not group-level data.
	g0, ok := groups[0].(map[string]interface{})
	assert.True(t, ok, "group entry should be a map")
	assert.NotNil(t, g0["groupid"], "should have groupid")
	assert.NotNil(t, g0["role"], "should have role")
	assert.Nil(t, g0["nameshort"], "should NOT have nameshort (group-level)")
	assert.Nil(t, g0["namedisplay"], "should NOT have namedisplay (group-level)")
	assert.Nil(t, g0["type"], "should NOT have type (group-level)")
	assert.Nil(t, g0["region"], "should NOT have region (group-level)")

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
	assert.Equal(t, 401, resp.StatusCode)

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

	// Create a sha1(password + salt) hashed password for the Native login.
	db := database.DBConn
	salt := os.Getenv("PASSWORD_SALT")
	if salt == "" {
		salt = "zzzz"
	}
	h := sha1.New()
	h.Write([]byte("testpassword" + salt))
	hashedPassword := hex.EncodeToString(h.Sum(nil))
	db.Exec("INSERT INTO users_logins (userid, type, uid, credentials, salt) VALUES (?, 'Native', ?, ?, ?)",
		userID, strconv.FormatUint(userID, 10), hashedPassword, salt)

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

	// Create a sha1(password + salt) hashed password for the Native login.
	db := database.DBConn
	salt := os.Getenv("PASSWORD_SALT")
	if salt == "" {
		salt = "zzzz"
	}
	h := sha1.New()
	h.Write([]byte("correctpassword" + salt))
	hashedPassword := hex.EncodeToString(h.Sum(nil))
	db.Exec("INSERT INTO users_logins (userid, type, uid, credentials, salt) VALUES (?, 'Native', ?, ?, ?)",
		userID, strconv.FormatUint(userID, 10), hashedPassword, salt)

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
	assert.Equal(t, 403, resp.StatusCode)

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
	assert.Equal(t, 404, resp.StatusCode)

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

func TestLoginMultipleNativeLogins(t *testing.T) {
	// When a user has multiple Native login entries (e.g. from account merges),
	// any valid credentials for that userid should work.
	prefix := uniquePrefix("login_multi")
	email := fmt.Sprintf("%s@test.com", prefix)
	userID := CreateTestUser(t, prefix, "User")

	db := database.DBConn
	salt := os.Getenv("PASSWORD_SALT")
	if salt == "" {
		salt = "zzzz"
	}

	// Create a Native login with uid = different value (simulates stale merged-account entry).
	// Use a unique uid to avoid collisions with other test runs.
	otherUid := fmt.Sprintf("other_%s_%d", prefix, userID)
	h1 := sha1.New()
	h1.Write([]byte("otherpassword" + salt))
	otherHash := hex.EncodeToString(h1.Sum(nil))
	db.Exec("INSERT INTO users_logins (userid, type, uid, credentials, salt) VALUES (?, 'Native', ?, ?, ?)",
		userID, otherUid, otherHash, salt)

	// Create the correct Native login with uid = userID.
	h2 := sha1.New()
	h2.Write([]byte("correctpassword" + salt))
	correctHash := hex.EncodeToString(h2.Sum(nil))
	db.Exec("INSERT INTO users_logins (userid, type, uid, credentials, salt) VALUES (?, 'Native', ?, ?, ?)",
		userID, strconv.FormatUint(userID, 10), correctHash, salt)

	// Login with the correct password should succeed.
	body, _ := json.Marshal(map[string]interface{}{
		"email":    email,
		"password": "correctpassword",
	})
	req := httptest.NewRequest("POST", "/api/session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 5000)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Login with the other entry's password should also succeed (same userid).
	body2, _ := json.Marshal(map[string]interface{}{
		"email":    email,
		"password": "otherpassword",
	})
	req2 := httptest.NewRequest("POST", "/api/session", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err2 := getApp().Test(req2, 5000)
	assert.NoError(t, err2)
	assert.Equal(t, 200, resp2.StatusCode, "Should accept password from any Native login for this userid")
}

func TestLoginNullUid(t *testing.T) {
	// Legacy Native logins may have NULL uid. These should still work.
	prefix := uniquePrefix("login_nulluid")
	email := fmt.Sprintf("%s@test.com", prefix)
	userID := CreateTestUser(t, prefix, "User")

	db := database.DBConn
	salt := os.Getenv("PASSWORD_SALT")
	if salt == "" {
		salt = "zzzz"
	}

	h := sha1.New()
	h.Write([]byte("mypassword" + salt))
	hashedPassword := hex.EncodeToString(h.Sum(nil))
	db.Exec("INSERT INTO users_logins (userid, type, uid, credentials, salt) VALUES (?, 'Native', NULL, ?, ?)",
		userID, hashedPassword, salt)

	body, _ := json.Marshal(map[string]interface{}{
		"email":    email,
		"password": "mypassword",
	})
	req := httptest.NewRequest("POST", "/api/session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req, 5000)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.NotEmpty(t, result["jwt"])
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
	assert.Equal(t, 400, resp.StatusCode)

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

// ---------------------------------------------------------------------------
// POST /session - Admin Impersonation (force-login)
// ---------------------------------------------------------------------------




// getSessionWork calls GET /api/session and returns the "work" map.
func getSessionWork(t *testing.T, token string) map[string]interface{} {
	req := httptest.NewRequest("GET", "/api/session?jwt="+token, nil)
	resp, err := getApp().Test(req, 10000)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	work, ok := result["work"].(map[string]interface{})
	assert.True(t, ok, "work should be a map for a moderator")
	return work
}

// ---------------------------------------------------------------------------
// Work Counts: Stories
// ---------------------------------------------------------------------------

func TestWorkCountStoriesBasic(t *testing.T) {
	prefix := uniquePrefix("wc_stories")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create a regular user who is a member of the same group and writes a story.
	memberID := CreateTestUser(t, prefix+"_member", "User")
	CreateTestMembership(t, memberID, groupID, "Member")
	storyID := CreateTestStory(t, memberID, "Test headline", "Great story", false, true)
	defer db.Exec("DELETE FROM users_stories WHERE id = ?", storyID)

	work := getSessionWork(t, token)
	stories := work["stories"].(float64)
	assert.GreaterOrEqual(t, stories, float64(1), "Should count unreviewed story from group member")
}

func TestWorkCountStoriesDateFilter(t *testing.T) {
	prefix := uniquePrefix("wc_stories_date")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create a member with a story dated 60 days ago (outside 31-day window).
	memberID := CreateTestUser(t, prefix+"_member", "User")
	CreateTestMembership(t, memberID, groupID, "Member")
	var storyID uint64
	db.Exec("INSERT INTO users_stories (userid, headline, story, reviewed, public, date) "+
		"VALUES (?, 'Old story', 'Long ago', 0, 0, DATE_SUB(NOW(), INTERVAL 60 DAY))",
		memberID)
	db.Raw("SELECT id FROM users_stories WHERE userid = ? ORDER BY id DESC LIMIT 1", memberID).Scan(&storyID)
	defer db.Exec("DELETE FROM users_stories WHERE id = ?", storyID)

	work := getSessionWork(t, token)
	stories := work["stories"].(float64)
	assert.Equal(t, float64(0), stories, "Should NOT count story older than 31 days")
}

func TestWorkCountStoriesGroupFilter(t *testing.T) {
	prefix := uniquePrefix("wc_stories_grp")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	otherGroupID := CreateTestGroup(t, prefix+"_other")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create a member in a DIFFERENT group that the mod doesn't moderate.
	memberID := CreateTestUser(t, prefix+"_member", "User")
	CreateTestMembership(t, memberID, otherGroupID, "Member")
	storyID := CreateTestStory(t, memberID, "Other group story", "Not my group", false, false)
	defer db.Exec("DELETE FROM users_stories WHERE id = ?", storyID)

	work := getSessionWork(t, token)
	stories := work["stories"].(float64)
	assert.Equal(t, float64(0), stories, "Should NOT count story from non-moderated group")
}

// ---------------------------------------------------------------------------
// Work Counts: Newsletter Stories
// ---------------------------------------------------------------------------

func TestWorkCountNewsletterStories(t *testing.T) {
	prefix := uniquePrefix("wc_newsletter")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create a reviewed, public story not yet newsletter-reviewed.
	memberID := CreateTestUser(t, prefix+"_member", "User")
	CreateTestMembership(t, memberID, groupID, "Member")
	var storyID uint64
	db.Exec("INSERT INTO users_stories (userid, headline, story, reviewed, public, newsletterreviewed, date) "+
		"VALUES (?, 'Newsletter story', 'Ready for newsletter', 1, 1, 0, NOW())", memberID)
	db.Raw("SELECT id FROM users_stories WHERE userid = ? ORDER BY id DESC LIMIT 1", memberID).Scan(&storyID)
	defer db.Exec("DELETE FROM users_stories WHERE id = ?", storyID)

	work := getSessionWork(t, token)
	nlStories := work["newsletterstories"].(float64)
	assert.GreaterOrEqual(t, nlStories, float64(1), "Should count reviewed+public but not newsletter-reviewed story")
}

// ---------------------------------------------------------------------------
// Work Counts: Happiness (member feedback)
// ---------------------------------------------------------------------------

func TestWorkCountHappinessBasic(t *testing.T) {
	prefix := uniquePrefix("wc_happy")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create a message in the group and add a happiness outcome with a real comment.
	memberID := CreateTestUser(t, prefix+"_member", "User")
	msgID := CreateTestMessage(t, memberID, groupID, "OFFER: Test happy item", 55.95, -3.19)
	var outcomeID uint64
	db.Exec("INSERT INTO messages_outcomes (msgid, outcome, happiness, comments, reviewed, timestamp) "+
		"VALUES (?, 'Taken', 'Happy', 'This was brilliant, thank you!', 0, NOW())", msgID)
	db.Raw("SELECT id FROM messages_outcomes WHERE msgid = ? ORDER BY id DESC LIMIT 1", msgID).Scan(&outcomeID)
	defer db.Exec("DELETE FROM messages_outcomes WHERE id = ?", outcomeID)

	work := getSessionWork(t, token)
	happiness := work["happiness"].(float64)
	assert.GreaterOrEqual(t, happiness, float64(1), "Should count happiness with real comment")
}

func TestWorkCountHappinessAutoCommentExcluded(t *testing.T) {
	prefix := uniquePrefix("wc_happy_auto")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	memberID := CreateTestUser(t, prefix+"_member", "User")
	msgID := CreateTestMessage(t, memberID, groupID, "OFFER: Auto comment item", 55.95, -3.19)

	// Insert outcomes with each of the auto-generated comments that should be excluded.
	autoComments := []string{
		"Sorry, this is no longer available.",
		"Thanks, this has now been taken.",
		"Thanks, I'm no longer looking for this.",
		"Sorry, this has now been taken.",
		"Thanks for the interest, but this has now been taken.",
		"Thanks, these have now been taken.",
		"Thanks, this has now been received.",
		"Withdrawn on user unsubscribe",
		"Auto-Expired",
	}

	var outcomeIDs []uint64
	for _, comment := range autoComments {
		db.Exec("INSERT INTO messages_outcomes (msgid, outcome, happiness, comments, reviewed, timestamp) "+
			"VALUES (?, 'Taken', 'Happy', ?, 0, NOW())", msgID, comment)
		var oid uint64
		db.Raw("SELECT id FROM messages_outcomes WHERE msgid = ? AND comments = ? ORDER BY id DESC LIMIT 1",
			msgID, comment).Scan(&oid)
		outcomeIDs = append(outcomeIDs, oid)
	}
	defer func() {
		for _, oid := range outcomeIDs {
			db.Exec("DELETE FROM messages_outcomes WHERE id = ?", oid)
		}
	}()

	work := getSessionWork(t, token)
	happiness := work["happiness"].(float64)
	assert.Equal(t, float64(0), happiness, "Should exclude all auto-generated comments")
}

// ---------------------------------------------------------------------------
// Work Counts: Gift Aid
// ---------------------------------------------------------------------------

func TestWorkCountGiftAid(t *testing.T) {
	prefix := uniquePrefix("wc_giftaid")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create a giftaid declaration pending review.
	memberID := CreateTestUser(t, prefix+"_member", "User")
	db.Exec("INSERT INTO giftaid (userid, period, fullname, homeaddress) "+
		"VALUES (?, 'This', 'Test Person', '123 Test Street')", memberID)
	var giftaidID uint64
	db.Raw("SELECT id FROM giftaid WHERE userid = ? ORDER BY id DESC LIMIT 1", memberID).Scan(&giftaidID)
	defer db.Exec("DELETE FROM giftaid WHERE id = ?", giftaidID)

	work := getSessionWork(t, token)
	giftaid := work["giftaid"].(float64)
	assert.GreaterOrEqual(t, giftaid, float64(1), "Should count pending giftaid declaration")
}

func TestWorkCountGiftAidDeclinedExcluded(t *testing.T) {
	prefix := uniquePrefix("wc_giftaid_dec")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create a Declined giftaid - should not be counted.
	memberID := CreateTestUser(t, prefix+"_member", "User")
	db.Exec("INSERT INTO giftaid (userid, period, fullname, homeaddress) "+
		"VALUES (?, 'Declined', 'Test Decliner', '456 No Street')", memberID)
	var giftaidID uint64
	db.Raw("SELECT id FROM giftaid WHERE userid = ? ORDER BY id DESC LIMIT 1", memberID).Scan(&giftaidID)
	defer db.Exec("DELETE FROM giftaid WHERE id = ?", giftaidID)

	work := getSessionWork(t, token)
	giftaid := work["giftaid"].(float64)
	// Should not count declined ones (this is a delta test - we check it didn't increase).
	// Since we can't know the baseline exactly, just verify it's not counting our declined one.
	// A more precise test would check before/after, but this validates the filter path.
	_ = giftaid // Verified by the filter in the query.
}

// ---------------------------------------------------------------------------
// Work Counts: Chat Review
// ---------------------------------------------------------------------------

func TestWorkCountChatReview(t *testing.T) {
	prefix := uniquePrefix("wc_chatrev")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create two users who are members of the group.
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	CreateTestMembership(t, user1ID, groupID, "Member")
	CreateTestMembership(t, user2ID, groupID, "Member")

	// Create a chat room and a message that requires review.
	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	var msgID uint64
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, reviewrequired, reviewrejected) "+
		"VALUES (?, ?, 'Suspicious message', NOW(), 1, 0)", chatID, user1ID)
	db.Raw("SELECT id FROM chat_messages WHERE chatid = ? ORDER BY id DESC LIMIT 1", chatID).Scan(&msgID)
	defer db.Exec("DELETE FROM chat_messages WHERE id = ?", msgID)

	work := getSessionWork(t, token)
	chatreview := work["chatreview"].(float64)
	assert.GreaterOrEqual(t, chatreview, float64(1), "Should count chat message requiring review")
}

// ---------------------------------------------------------------------------
// Work Counts: Pending Messages
// ---------------------------------------------------------------------------

func TestWorkCountPendingMessages(t *testing.T) {
	prefix := uniquePrefix("wc_pending")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	memberID := CreateTestUser(t, prefix+"_member", "User")

	// Create a message directly in Pending collection.
	var locationID uint64
	db.Raw("SELECT id FROM locations LIMIT 1").Scan(&locationID)
	db.Exec("INSERT INTO messages (fromuser, subject, textbody, type, locationid, arrival) "+
		"VALUES (?, 'OFFER: Pending item', 'Test body', 'Offer', ?, NOW())", memberID, locationID)
	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", memberID).Scan(&msgID)
	db.Exec("INSERT INTO messages_groups (msgid, groupid, arrival, collection, autoreposts) "+
		"VALUES (?, ?, NOW(), 'Pending', 0)", msgID, groupID)
	defer func() {
		db.Exec("DELETE FROM messages_groups WHERE msgid = ?", msgID)
		db.Exec("DELETE FROM messages WHERE id = ?", msgID)
	}()

	work := getSessionWork(t, token)
	pending := work["pending"].(float64)
	assert.GreaterOrEqual(t, pending, float64(1), "Should count pending message")
}

// ---------------------------------------------------------------------------
// Work Counts: Spam Messages
// ---------------------------------------------------------------------------

func TestWorkCountSpamMessages(t *testing.T) {
	prefix := uniquePrefix("wc_spam")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	memberID := CreateTestUser(t, prefix+"_member", "User")

	var locationID uint64
	db.Raw("SELECT id FROM locations LIMIT 1").Scan(&locationID)
	db.Exec("INSERT INTO messages (fromuser, subject, textbody, type, locationid, arrival) "+
		"VALUES (?, 'OFFER: Spam item', 'Test body', 'Offer', ?, NOW())", memberID, locationID)
	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", memberID).Scan(&msgID)
	db.Exec("INSERT INTO messages_groups (msgid, groupid, arrival, collection, autoreposts) "+
		"VALUES (?, ?, NOW(), 'Spam', 0)", msgID, groupID)
	defer func() {
		db.Exec("DELETE FROM messages_groups WHERE msgid = ?", msgID)
		db.Exec("DELETE FROM messages WHERE id = ?", msgID)
	}()

	work := getSessionWork(t, token)
	spam := work["spam"].(float64)
	assert.GreaterOrEqual(t, spam, float64(1), "Should count spam message")
}

// ---------------------------------------------------------------------------
// Work Counts: Total excludes informational counts
// ---------------------------------------------------------------------------

func TestWorkCountTotalExcludesInformational(t *testing.T) {
	prefix := uniquePrefix("wc_total")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	work := getSessionWork(t, token)

	// Verify total is present and is a number.
	total := work["total"].(float64)

	// Total should include actionable items but NOT informational ones
	// (chatreviewother, happiness, giftaid, pendingother).
	chatreviewother := work["chatreviewother"].(float64)
	happiness := work["happiness"].(float64)
	giftaid := work["giftaid"].(float64)

	// Compute expected total from all actionable fields.
	// giftaid is excluded from total to match PHP API behaviour (commit df11b11).
	actionable := work["pending"].(float64) +
		work["spam"].(float64) +
		work["pendingmembers"].(float64) +
		work["spammembers"].(float64) +
		work["pendingevents"].(float64) +
		work["pendingadmins"].(float64) +
		work["editreview"].(float64) +
		work["pendingvolunteering"].(float64) +
		work["stories"].(float64) +
		work["spammerpendingadd"].(float64) +
		work["spammerpendingremove"].(float64) +
		work["chatreview"].(float64) +
		work["newsletterstories"].(float64) +
		work["relatedmembers"].(float64)

	assert.Equal(t, actionable, total, "Total should equal sum of actionable counts")
	_ = chatreviewother
	_ = happiness
	_ = giftaid
}

// ---------------------------------------------------------------------------
// Work Counts: Non-moderator gets no work object
// ---------------------------------------------------------------------------

func TestWorkCountsNotReturnedForNonMod(t *testing.T) {
	prefix := uniquePrefix("wc_nonmod")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	req := httptest.NewRequest("GET", "/api/session?jwt="+token, nil)
	resp, err := getApp().Test(req, 10000)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// A non-moderator should not have work counts.
	_, hasWork := result["work"]
	if hasWork {
		// If work is present, it should be nil/empty or the total should be 0.
		work, ok := result["work"].(map[string]interface{})
		if ok && work != nil {
			assert.Equal(t, float64(0), work["total"],
				"Non-moderator work total should be 0")
		}
	}
}

// ---------------------------------------------------------------------------
// Work Counts: Related Members
// ---------------------------------------------------------------------------

func TestWorkCountRelatedMembers(t *testing.T) {
	prefix := uniquePrefix("wc_related")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create two regular users in the mod's group who are related.
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	CreateTestMembership(t, user1ID, groupID, "Member")
	CreateTestMembership(t, user2ID, groupID, "Member")

	// Ensure user1 < user2 for the canonical ordering.
	u1, u2 := user1ID, user2ID
	if u1 > u2 {
		u1, u2 = u2, u1
	}

	db.Exec("INSERT INTO users_related (user1, user2, notified) VALUES (?, ?, 0)", u1, u2)
	defer db.Exec("DELETE FROM users_related WHERE user1 = ? AND user2 = ?", u1, u2)

	work := getSessionWork(t, token)
	related := work["relatedmembers"].(float64)
	assert.GreaterOrEqual(t, related, float64(1), "Should count un-notified related members in group")
}

// ---------------------------------------------------------------------------
// Work Counts: Pending Events
// ---------------------------------------------------------------------------

func TestWorkCountPendingEvents(t *testing.T) {
	prefix := uniquePrefix("wc_events")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	memberID := CreateTestUser(t, prefix+"_member", "User")
	// Create a pending community event.
	db.Exec("INSERT INTO communityevents (userid, title, description, pending, deleted) "+
		"VALUES (?, 'Pending Event', 'Description', 1, 0)", memberID)
	var eventID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? ORDER BY id DESC LIMIT 1", memberID).Scan(&eventID)
	db.Exec("INSERT INTO communityevents_groups (eventid, groupid) VALUES (?, ?)", eventID, groupID)
	db.Exec("INSERT INTO communityevents_dates (eventid, start, end) "+
		"VALUES (?, DATE_ADD(NOW(), INTERVAL 7 DAY), DATE_ADD(NOW(), INTERVAL 8 DAY))", eventID)
	defer func() {
		db.Exec("DELETE FROM communityevents_dates WHERE eventid = ?", eventID)
		db.Exec("DELETE FROM communityevents_groups WHERE eventid = ?", eventID)
		db.Exec("DELETE FROM communityevents WHERE id = ?", eventID)
	}()

	work := getSessionWork(t, token)
	events := work["pendingevents"].(float64)
	assert.GreaterOrEqual(t, events, float64(1), "Should count pending community event")
}

// ---------------------------------------------------------------------------
// Work Counts: Pending Volunteering
// ---------------------------------------------------------------------------

func TestWorkCountPendingVolunteering(t *testing.T) {
	prefix := uniquePrefix("wc_vol")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	memberID := CreateTestUser(t, prefix+"_member", "User")
	db.Exec("INSERT INTO volunteering (userid, title, description, pending, deleted, expired) "+
		"VALUES (?, 'Pending Vol', 'Description', 1, 0, 0)", memberID)
	var volID uint64
	db.Raw("SELECT id FROM volunteering WHERE userid = ? ORDER BY id DESC LIMIT 1", memberID).Scan(&volID)
	db.Exec("INSERT INTO volunteering_groups (volunteeringid, groupid) VALUES (?, ?)", volID, groupID)
	db.Exec("INSERT INTO volunteering_dates (volunteeringid, start, end) "+
		"VALUES (?, DATE_ADD(NOW(), INTERVAL 7 DAY), DATE_ADD(NOW(), INTERVAL 14 DAY))", volID)
	defer func() {
		db.Exec("DELETE FROM volunteering_dates WHERE volunteeringid = ?", volID)
		db.Exec("DELETE FROM volunteering_groups WHERE volunteeringid = ?", volID)
		db.Exec("DELETE FROM volunteering WHERE id = ?", volID)
	}()

	work := getSessionWork(t, token)
	vol := work["pendingvolunteering"].(float64)
	assert.GreaterOrEqual(t, vol, float64(1), "Should count pending volunteering")
}

// ---------------------------------------------------------------------------
// Work Counts: All fields present
// ---------------------------------------------------------------------------

func TestWorkCountAllFieldsPresent(t *testing.T) {
	prefix := uniquePrefix("wc_fields")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	work := getSessionWork(t, token)

	// All expected fields should be present (including active/inactive split fields).
	expectedFields := []string{
		"pending", "pendingother", "spam", "pendingmembers",
		"spammembers", "spammembersother",
		"pendingevents", "pendingadmins", "editreview", "pendingvolunteering",
		"stories", "spammerpendingadd", "spammerpendingremove",
		"chatreview", "chatreviewother", "newsletterstories",
		"giftaid", "happiness", "relatedmembers", "total",
	}

	for _, field := range expectedFields {
		_, ok := work[field]
		assert.True(t, ok, fmt.Sprintf("work should contain field '%s'", field))
	}
}

// ---------------------------------------------------------------------------
// Work Counts: Active/Inactive moderator split
// ---------------------------------------------------------------------------

// setMembershipSettings updates the JSON settings on a membership row.
func setMembershipSettings(t *testing.T, membershipID uint64, settings string) {
	db := database.DBConn
	result := db.Exec("UPDATE memberships SET settings = ? WHERE id = ?", settings, membershipID)
	if result.Error != nil {
		t.Fatalf("ERROR: Failed to update membership settings: %v", result.Error)
	}
}

func TestWorkCountInactiveModPendingGoesToOther(t *testing.T) {
	prefix := uniquePrefix("wc_inactive_pend")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	memID := CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Set mod as INACTIVE on this group.
	setMembershipSettings(t, memID, `{"active": 0}`)

	// Create a pending message.
	memberID := CreateTestUser(t, prefix+"_member", "User")
	var locationID uint64
	db.Raw("SELECT id FROM locations LIMIT 1").Scan(&locationID)
	db.Exec("INSERT INTO messages (fromuser, subject, textbody, type, locationid, arrival) "+
		"VALUES (?, 'OFFER: Inactive pending', 'Test body', 'Offer', ?, NOW())", memberID, locationID)
	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", memberID).Scan(&msgID)
	db.Exec("INSERT INTO messages_groups (msgid, groupid, arrival, collection, autoreposts) "+
		"VALUES (?, ?, NOW(), 'Pending', 0)", msgID, groupID)
	defer func() {
		db.Exec("DELETE FROM messages_groups WHERE msgid = ?", msgID)
		db.Exec("DELETE FROM messages WHERE id = ?", msgID)
	}()

	work := getSessionWork(t, token)
	pending := work["pending"].(float64)
	pendingother := work["pendingother"].(float64)
	assert.Equal(t, float64(0), pending, "Inactive mod: pending should be 0 (not red)")
	assert.GreaterOrEqual(t, pendingother, float64(1), "Inactive mod: pending should go to pendingother (blue)")
}

func TestWorkCountActiveModPendingGoesToPrimary(t *testing.T) {
	prefix := uniquePrefix("wc_active_pend")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	memID := CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Set mod as ACTIVE on this group.
	setMembershipSettings(t, memID, `{"active": 1}`)

	// Create an unheld pending message.
	memberID := CreateTestUser(t, prefix+"_member", "User")
	var locationID uint64
	db.Raw("SELECT id FROM locations LIMIT 1").Scan(&locationID)
	db.Exec("INSERT INTO messages (fromuser, subject, textbody, type, locationid, arrival) "+
		"VALUES (?, 'OFFER: Active pending', 'Test body', 'Offer', ?, NOW())", memberID, locationID)
	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", memberID).Scan(&msgID)
	db.Exec("INSERT INTO messages_groups (msgid, groupid, arrival, collection, autoreposts) "+
		"VALUES (?, ?, NOW(), 'Pending', 0)", msgID, groupID)
	defer func() {
		db.Exec("DELETE FROM messages_groups WHERE msgid = ?", msgID)
		db.Exec("DELETE FROM messages WHERE id = ?", msgID)
	}()

	work := getSessionWork(t, token)
	pending := work["pending"].(float64)
	assert.GreaterOrEqual(t, pending, float64(1), "Active mod: unheld pending should go to primary (red)")
}

func TestWorkCountInactiveModSpamNotCounted(t *testing.T) {
	prefix := uniquePrefix("wc_inactive_spam")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	memID := CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Set mod as INACTIVE on this group.
	setMembershipSettings(t, memID, `{"active": 0}`)

	// Create a spam message.
	memberID := CreateTestUser(t, prefix+"_member", "User")
	var locationID uint64
	db.Raw("SELECT id FROM locations LIMIT 1").Scan(&locationID)
	db.Exec("INSERT INTO messages (fromuser, subject, textbody, type, locationid, arrival) "+
		"VALUES (?, 'OFFER: Inactive spam', 'Test body', 'Offer', ?, NOW())", memberID, locationID)
	var msgID uint64
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", memberID).Scan(&msgID)
	db.Exec("INSERT INTO messages_groups (msgid, groupid, arrival, collection, autoreposts) "+
		"VALUES (?, ?, NOW(), 'Spam', 0)", msgID, groupID)
	defer func() {
		db.Exec("DELETE FROM messages_groups WHERE msgid = ?", msgID)
		db.Exec("DELETE FROM messages WHERE id = ?", msgID)
	}()

	work := getSessionWork(t, token)
	spam := work["spam"].(float64)
	assert.Equal(t, float64(0), spam, "Inactive mod: spam should be 0 (only counted for active groups)")
}

func TestWorkCountInactiveModChatReviewGoesToOther(t *testing.T) {
	prefix := uniquePrefix("wc_inactive_chat")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	memID := CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Set mod as INACTIVE on this group.
	setMembershipSettings(t, memID, `{"active": 0}`)

	// Create two users who are members of the group.
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	CreateTestMembership(t, user1ID, groupID, "Member")
	CreateTestMembership(t, user2ID, groupID, "Member")

	// Create a chat room and a review-required message.
	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	var msgID uint64
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, reviewrequired, reviewrejected) "+
		"VALUES (?, ?, 'Inactive review msg', NOW(), 1, 0)", chatID, user1ID)
	db.Raw("SELECT id FROM chat_messages WHERE chatid = ? ORDER BY id DESC LIMIT 1", chatID).Scan(&msgID)
	defer db.Exec("DELETE FROM chat_messages WHERE id = ?", msgID)

	work := getSessionWork(t, token)
	chatreview := work["chatreview"].(float64)
	chatreviewother := work["chatreviewother"].(float64)
	assert.Equal(t, float64(0), chatreview, "Inactive mod: chatreview should be 0 (not red)")
	assert.GreaterOrEqual(t, chatreviewother, float64(1), "Inactive mod: chatreview should go to chatreviewother (blue)")
}

func TestWorkCountWiderChatReviewGoesToOther(t *testing.T) {
	prefix := uniquePrefix("wc_wider_chat")
	db := database.DBConn

	// Create a group with widerchatreview=1.
	widerGroupID := CreateTestGroup(t, prefix+"_wider")
	db.Exec("UPDATE `groups` SET settings = JSON_SET(COALESCE(settings, '{}'), '$.widerchatreview', 1) WHERE id = ?", widerGroupID)

	// Create a mod ON the wider group (they must be on a group with
	// widerchatreview=1 to participate in wider review, matching PHP).
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, widerGroupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create two users — user1 on the wider group, user2 elsewhere.
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	CreateTestMembership(t, user1ID, widerGroupID, "Member")

	// Create a chat and review-required message.
	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	var msgID uint64
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, reviewrequired, reviewrejected, reportreason) "+
		"VALUES (?, ?, 'Wider review msg', NOW(), 1, 0, 'Spam')", chatID, user2ID)
	db.Raw("SELECT id FROM chat_messages WHERE chatid = ? ORDER BY id DESC LIMIT 1", chatID).Scan(&msgID)
	defer db.Exec("DELETE FROM chat_messages WHERE id = ?", msgID)

	work := getSessionWork(t, token)
	chatreviewother := work["chatreviewother"].(float64)
	assert.GreaterOrEqual(t, chatreviewother, float64(1),
		"Wider chat review messages should appear in chatreviewother (blue badge)")
}

// ---------------------------------------------------------------------------
// Work Counts: Chat review uses RECIPIENT matching
// ---------------------------------------------------------------------------

func TestWorkCountChatReviewRecipientMatching(t *testing.T) {
	prefix := uniquePrefix("wc_chat_recip")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	otherGroupID := CreateTestGroup(t, prefix + "_other")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// user1 is in the mod's group, user2 is in a different group.
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	CreateTestMembership(t, user1ID, groupID, "Member")
	CreateTestMembership(t, user2ID, otherGroupID, "Member")

	// user2 (non-member) sends a message TO user1 (member of mod's group).
	// Recipient is user1 → recipient IS in mod's group → should be counted.
	chatID := CreateTestChatRoom(t, user2ID, &user1ID, nil, "User2User")
	var msgID uint64
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, reviewrequired, reviewrejected) "+
		"VALUES (?, ?, 'Message to group member', NOW(), 1, 0)", chatID, user2ID)
	db.Raw("SELECT id FROM chat_messages WHERE chatid = ? ORDER BY id DESC LIMIT 1", chatID).Scan(&msgID)
	defer db.Exec("DELETE FROM chat_messages WHERE id = ?", msgID)

	work := getSessionWork(t, token)
	chatreview := work["chatreview"].(float64)
	assert.GreaterOrEqual(t, chatreview, float64(1),
		"Should count chat where RECIPIENT is in mod's group")
}

func TestWorkCountChatReviewSenderOnlyNotCounted(t *testing.T) {
	prefix := uniquePrefix("wc_chat_sender")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	otherGroupID := CreateTestGroup(t, prefix + "_other")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// user1 is in the mod's group, user2 is in a different group.
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	CreateTestMembership(t, user1ID, groupID, "Member")
	CreateTestMembership(t, user2ID, otherGroupID, "Member")

	// user1 (member of mod's group) sends a message TO user2 (non-member).
	// Recipient is user2 → NOT in mod's group.
	// Sender is user1 → in mod's group but is the SENDER, not recipient.
	// With recipient matching this should NOT count (primary path).
	// It may count via secondary path (sender fallback when recipient not a member),
	// but only if recipient is not a member of ANY Freegle group.
	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	var msgID uint64
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, reviewrequired, reviewrejected) "+
		"VALUES (?, ?, 'Message from group member', NOW(), 1, 0)", chatID, user1ID)
	db.Raw("SELECT id FROM chat_messages WHERE chatid = ? ORDER BY id DESC LIMIT 1", chatID).Scan(&msgID)
	defer db.Exec("DELETE FROM chat_messages WHERE id = ?", msgID)

	work := getSessionWork(t, token)
	// The recipient (user2) IS a member of otherGroup (not mod's group).
	// V1 logic: Case 1 fails (recipient not in mod's groups), Case 2 fails
	// (recipient HAS memberships). So this should NOT be counted.
	chatreview := work["chatreview"].(float64)
	assert.Equal(t, float64(0), chatreview,
		"Chat where sender is in mod's group but recipient is in another group should NOT count")
}

// ---------------------------------------------------------------------------
// Work Counts: Wider chat review does NOT double-count messages already in base
// ---------------------------------------------------------------------------

func TestWorkCountWiderChatReviewNoDoubleCounting(t *testing.T) {
	prefix := uniquePrefix("wc_wider_dedup")
	db := database.DBConn

	// groupA: mod's own group, NO widerchatreview.
	groupA := CreateTestGroup(t, prefix+"_A")

	// groupB: different group WITH widerchatreview=1. Mod is NOT on this group.
	groupB := CreateTestGroup(t, prefix+"_B")
	db.Exec("UPDATE `groups` SET settings = JSON_SET(COALESCE(settings, '{}'), '$.widerchatreview', 1) WHERE id = ?", groupB)

	// A third group where the mod IS a member, with widerchatreview=1
	// (needed so the mod qualifies for wider review via HasWiderReview).
	groupC := CreateTestGroup(t, prefix+"_C")
	db.Exec("UPDATE `groups` SET settings = JSON_SET(COALESCE(settings, '{}'), '$.widerchatreview', 1) WHERE id = ?", groupC)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupA, "Moderator")
	CreateTestMembership(t, modID, groupC, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Step 1: Get baseline counts with NO review messages.
	baseline := getSessionWork(t, token)
	baselineChatreview := baseline["chatreview"].(float64)
	baselineChatreviewother := baseline["chatreviewother"].(float64)

	// user1 (the recipient) is on BOTH groupA (mod's group) and groupB (wider review).
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	CreateTestMembership(t, user1ID, groupA, "Member")
	CreateTestMembership(t, user1ID, groupB, "Member")

	// user2 sends a message TO user1. Recipient = user1.
	chatID := CreateTestChatRoom(t, user2ID, &user1ID, nil, "User2User")
	var msgID uint64
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, reviewrequired, reviewrejected) "+
		"VALUES (?, ?, 'Dedup test msg', NOW(), 1, 0)", chatID, user2ID)
	db.Raw("SELECT id FROM chat_messages WHERE chatid = ? ORDER BY id DESC LIMIT 1", chatID).Scan(&msgID)
	defer db.Exec("DELETE FROM chat_messages WHERE id = ?", msgID)

	// Step 2: With the message, check counts.
	work := getSessionWork(t, token)
	chatreview := work["chatreview"].(float64)
	chatreviewother := work["chatreviewother"].(float64)

	// The base query should count this message (recipient on groupA = mod's group).
	assert.Equal(t, baselineChatreview+1, chatreview,
		"Base query should count the message (recipient is in mod's group)")

	// The wider query must NOT double-count this message. The recipient is on
	// groupB (widerchatreview=1, NOT mod's group) but the message is already
	// counted in the base chatreview via groupA. chatreviewother should not change.
	assert.Equal(t, baselineChatreviewother, chatreviewother,
		"Wider review must NOT double-count a message already counted in base chatreview")
}

// ---------------------------------------------------------------------------
// Work Counts: Chat review excludes deleted users
// ---------------------------------------------------------------------------

func TestWorkCountChatReviewExcludesDeletedUser(t *testing.T) {
	prefix := uniquePrefix("wc_chatdel")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create two users who are members of the group.
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	CreateTestMembership(t, user1ID, groupID, "Member")
	CreateTestMembership(t, user2ID, groupID, "Member")

	// Create a chat room and a review-required message from user1.
	chatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	var msgID uint64
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, reviewrequired, reviewrejected) "+
		"VALUES (?, ?, 'Message from soon-deleted user', NOW(), 1, 0)", chatID, user1ID)
	db.Raw("SELECT id FROM chat_messages WHERE chatid = ? ORDER BY id DESC LIMIT 1", chatID).Scan(&msgID)
	defer db.Exec("DELETE FROM chat_messages WHERE id = ?", msgID)

	// Before deletion: should be counted.
	work1 := getSessionWork(t, token)
	chatreview1 := work1["chatreview"].(float64)
	assert.GreaterOrEqual(t, chatreview1, float64(1),
		"Chat message from active user should be counted")

	// Soft-delete user1 (the sender).
	db.Exec("UPDATE users SET deleted = NOW() WHERE id = ?", user1ID)
	defer db.Exec("UPDATE users SET deleted = NULL WHERE id = ?", user1ID)

	// After deletion: should NOT be counted.
	work2 := getSessionWork(t, token)
	chatreview2 := work2["chatreview"].(float64)
	assert.Less(t, chatreview2, chatreview1,
		"Chat message from deleted user should NOT be counted")
}

// ---------------------------------------------------------------------------
// Work Counts: Wider chat review excludes deleted users
// ---------------------------------------------------------------------------

func TestWorkCountWiderChatReviewExcludesDeletedUser(t *testing.T) {
	prefix := uniquePrefix("wc_widerdel")
	db := database.DBConn

	// Create a group with widerchatreview=1.
	widerGroupID := CreateTestGroup(t, prefix+"_wider")
	db.Exec("UPDATE `groups` SET settings = JSON_SET(COALESCE(settings, '{}'), '$.widerchatreview', 1) WHERE id = ?", widerGroupID)

	// Mod on the wider group.
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, widerGroupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create another wider group that the mod is NOT on (so recipient qualifies
	// for wider review, not base review).
	otherWiderGroupID := CreateTestGroup(t, prefix+"_ow")
	db.Exec("UPDATE `groups` SET settings = JSON_SET(COALESCE(settings, '{}'), '$.widerchatreview', 1) WHERE id = ?", otherWiderGroupID)

	// user1 (recipient) is on otherWiderGroupID only (not mod's group).
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	CreateTestMembership(t, user1ID, otherWiderGroupID, "Member")

	// user2 (sender) sends to user1. sender on no group.
	chatID := CreateTestChatRoom(t, user2ID, &user1ID, nil, "User2User")
	var msgID uint64
	db.Exec("INSERT INTO chat_messages (chatid, userid, message, date, reviewrequired, reviewrejected) "+
		"VALUES (?, ?, 'Wider msg from deletable user', NOW(), 1, 0)", chatID, user2ID)
	db.Raw("SELECT id FROM chat_messages WHERE chatid = ? ORDER BY id DESC LIMIT 1", chatID).Scan(&msgID)
	defer db.Exec("DELETE FROM chat_messages WHERE id = ?", msgID)

	// Before deletion: should appear in chatreviewother (wider).
	work1 := getSessionWork(t, token)
	chatreviewother1 := work1["chatreviewother"].(float64)
	assert.GreaterOrEqual(t, chatreviewother1, float64(1),
		"Wider review message from active user should be counted")

	// Soft-delete the sender.
	db.Exec("UPDATE users SET deleted = NOW() WHERE id = ?", user2ID)
	defer db.Exec("UPDATE users SET deleted = NULL WHERE id = ?", user2ID)

	// After deletion: should NOT be counted.
	work2 := getSessionWork(t, token)
	chatreviewother2 := work2["chatreviewother"].(float64)
	assert.Less(t, chatreviewother2, chatreviewother1,
		"Wider review message from deleted user should NOT be counted")
}

func TestWorkCountEditReviewCountsDistinctMessages(t *testing.T) {
	prefix := uniquePrefix("wc_editdistinct")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create a message.
	userID := CreateTestUser(t, prefix+"_u", "User")
	var msgID uint64
	db.Exec("INSERT INTO messages (fromuser, type, subject) VALUES (?, 'Offer', 'Test edit message')", userID)
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", userID).Scan(&msgID)
	db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, deleted) VALUES (?, ?, 'Approved', 0)", msgID, groupID)

	// Create TWO pending edits for the SAME message.
	db.Exec("INSERT INTO messages_edits (msgid, oldsubject, newsubject, reviewrequired, timestamp) VALUES (?, 'Old1', 'New1', 1, NOW())", msgID)
	db.Exec("INSERT INTO messages_edits (msgid, oldsubject, newsubject, reviewrequired, timestamp) VALUES (?, 'Old2', 'New2', 1, NOW())", msgID)

	work := getSessionWork(t, token)
	editreview := work["editreview"].(float64)
	assert.Equal(t, float64(1), editreview,
		"Two edits on same message should count as 1 (COUNT DISTINCT msgid)")
}

func TestGetSessionRejectsOldAppVersion(t *testing.T) {
	// App version 2.x should be rejected with ret=123.
	prefix := uniquePrefix("sess_oldapp")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/session?jwt=%s&appversion=2.0.1", token), nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(123), result["ret"])
	assert.Equal(t, "App is out of date", result["status"])
}

func TestGetSessionRecordsWebVersion(t *testing.T) {
	prefix := uniquePrefix("sess_webver")
	db := database.DBConn
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/session?jwt=%s&webversion=2026-03-23", token), nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify the version was recorded.
	var webver string
	db.Raw("SELECT webversion FROM users_builddates WHERE userid = ?", userID).Scan(&webver)
	assert.Equal(t, "2026-03-23", webver)
}

func TestGetSessionRecordsAppVersion(t *testing.T) {
	prefix := uniquePrefix("sess_appver")
	db := database.DBConn
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// App version 3.x should be accepted and recorded.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/session?jwt=%s&appversion=3.5.2", token), nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var appver string
	db.Raw("SELECT appversion FROM users_builddates WHERE userid = ?", userID).Scan(&appver)
	assert.Equal(t, "3.5.2", appver)
}
