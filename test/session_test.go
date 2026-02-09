package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	assert.Equal(t, 200, resp.StatusCode)

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
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	// Deleted user should not be found.
	assert.Equal(t, float64(2), result["ret"])
}
