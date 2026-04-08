package test

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func TestHousekeeperNotifySuccess(t *testing.T) {
	prefix := uniquePrefix("hknotify")
	db := database.DBConn

	// Create an admin user and session.
	userID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, userID)

	body := []byte(`{
		"task": "facebook-deletion",
		"status": "success",
		"summary": "Downloaded 1 file(s), found 3 user ID(s)",
		"timestamp": "2026-04-08T22:10:36Z",
		"email": "admin@test.com",
		"data": {"ids": ["123456789", "987654321"], "files": 1}
	}`)

	req := httptest.NewRequest("POST", "/api/housekeeper/notify?jwt="+token, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify the background_tasks row was queued.
	var taskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = 'housekeeper_notify' AND JSON_EXTRACT(data, '$.task') = 'facebook-deletion' AND processed_at IS NULL").Scan(&taskCount)
	assert.Equal(t, int64(1), taskCount, "Housekeeper notify task should be queued")

	// Verify the queued data contains the expected fields.
	var summary string
	db.Raw("SELECT JSON_EXTRACT(data, '$.summary') FROM background_tasks WHERE task_type = 'housekeeper_notify' AND JSON_EXTRACT(data, '$.task') = 'facebook-deletion' AND processed_at IS NULL ORDER BY id DESC LIMIT 1").Scan(&summary)
	assert.Contains(t, summary, "Downloaded 1 file(s), found 3 user ID(s)")

	// Clean up.
	db.Exec("DELETE FROM background_tasks WHERE task_type = 'housekeeper_notify' AND JSON_EXTRACT(data, '$.task') = 'facebook-deletion'")
}

func TestHousekeeperNotifyUnauthorized(t *testing.T) {
	body := []byte(`{
		"task": "facebook-deletion",
		"status": "success",
		"summary": "test",
		"timestamp": "2026-04-08T22:10:36Z",
		"email": "admin@test.com",
		"data": {}
	}`)

	// No JWT token — should get 401.
	req := httptest.NewRequest("POST", "/api/housekeeper/notify", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestHousekeeperNotifyForbiddenNonAdmin(t *testing.T) {
	prefix := uniquePrefix("hkforbid")
	db := database.DBConn

	// Create a regular (non-admin) user.
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	body := []byte(`{
		"task": "facebook-deletion",
		"status": "success",
		"summary": "test",
		"timestamp": "2026-04-08T22:10:36Z",
		"email": "user@test.com",
		"data": {}
	}`)

	req := httptest.NewRequest("POST", "/api/housekeeper/notify?jwt="+token, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)

	// Verify no task was queued.
	var taskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = 'housekeeper_notify' AND processed_at IS NULL AND JSON_EXTRACT(data, '$.email') = ?",
		fmt.Sprintf("%s_user@test.com", prefix)).Scan(&taskCount)
	assert.Equal(t, int64(0), taskCount, "No task should be queued for non-admin user")
}

func TestHousekeeperNotifyFailureQueued(t *testing.T) {
	prefix := uniquePrefix("hkfail")
	db := database.DBConn

	// Create an admin user and session.
	userID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, userID)

	// Extension reports a failure — should still be queued for notification.
	body := []byte(`{
		"task": "facebook-deletion",
		"status": "failure",
		"summary": "Could not find download button for user identifiers",
		"timestamp": "2026-04-08T22:10:36Z",
		"email": "admin@test.com",
		"data": {"pagePreview": "Some page content"}
	}`)

	req := httptest.NewRequest("POST", "/api/housekeeper/notify?jwt="+token, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify failure result was queued with correct status.
	var status string
	db.Raw("SELECT JSON_UNQUOTE(JSON_EXTRACT(data, '$.status')) FROM background_tasks WHERE task_type = 'housekeeper_notify' AND JSON_EXTRACT(data, '$.email') = 'admin@test.com' AND processed_at IS NULL ORDER BY id DESC LIMIT 1").Scan(&status)
	assert.Equal(t, "failure", status, "Failure status should be preserved in queued task")

	// Clean up.
	db.Exec("DELETE FROM background_tasks WHERE task_type = 'housekeeper_notify' AND JSON_EXTRACT(data, '$.email') = 'admin@test.com'")
}

func TestHousekeeperNotifyBadRequest(t *testing.T) {
	prefix := uniquePrefix("hkbadreq")

	// Create an admin user.
	userID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, userID)

	// Missing required "task" and "status" fields.
	body := []byte(`{
		"summary": "test",
		"timestamp": "2026-04-08T22:10:36Z",
		"email": "admin@test.com",
		"data": {}
	}`)

	req := httptest.NewRequest("POST", "/api/housekeeper/notify?jwt="+token, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}
