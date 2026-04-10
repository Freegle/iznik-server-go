package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

func TestHousekeeperNotifyUpsertsRegistry(t *testing.T) {
	prefix := uniquePrefix("hkreg")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, userID)

	body := []byte(`{
		"task": "facebook-deletion",
		"status": "success",
		"summary": "Test with registry",
		"timestamp": "2026-04-09T10:00:00Z",
		"email": "admin@test.com",
		"data": {},
		"registry": [
			{
				"task_key": "facebook-deletion",
				"name": "Facebook Data Deletion",
				"description": "Download pending deletion requests",
				"interval_hours": 168,
				"enabled": true,
				"placeholder": false
			},
			{
				"task_key": "paypal-giving-fund",
				"name": "PayPal Giving Fund",
				"description": "Process donation reports",
				"interval_hours": 720,
				"enabled": false,
				"placeholder": true
			}
		]
	}`)

	req := httptest.NewRequest("POST", "/api/housekeeper/notify?jwt="+token, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify registry rows were upserted.
	var count int64
	db.Raw("SELECT COUNT(*) FROM housekeeper_tasks WHERE task_key IN ('facebook-deletion', 'paypal-giving-fund')").Scan(&count)
	assert.Equal(t, int64(2), count, "Both registry tasks should be upserted")

	// Verify the PayPal stub details.
	var name string
	var placeholder bool
	db.Raw("SELECT name, placeholder FROM housekeeper_tasks WHERE task_key = 'paypal-giving-fund'").Row().Scan(&name, &placeholder)
	assert.Equal(t, "PayPal Giving Fund", name)
	assert.True(t, placeholder)

	// Clean up.
	db.Exec("DELETE FROM housekeeper_tasks WHERE task_key IN ('facebook-deletion', 'paypal-giving-fund')")
	db.Exec("DELETE FROM background_tasks WHERE task_type = 'housekeeper_notify' AND JSON_EXTRACT(data, '$.summary') = '\"Test with registry\"'")
}

func TestHousekeeperNotifyUpdatesLastRun(t *testing.T) {
	prefix := uniquePrefix("hklast")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, userID)

	body := []byte(`{
		"task": "facebook-deletion",
		"status": "success",
		"summary": "Found 5 IDs",
		"timestamp": "2026-04-09T10:00:00Z",
		"email": "admin@test.com",
		"data": {}
	}`)

	req := httptest.NewRequest("POST", "/api/housekeeper/notify?jwt="+token, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify last_run_at and last_status were set.
	var lastStatus string
	var lastSummary string
	db.Raw("SELECT last_status, last_summary FROM housekeeper_tasks WHERE task_key = 'facebook-deletion'").Row().Scan(&lastStatus, &lastSummary)
	assert.Equal(t, "success", lastStatus)
	assert.Equal(t, "Found 5 IDs", lastSummary)

	// Clean up.
	db.Exec("DELETE FROM housekeeper_tasks WHERE task_key = 'facebook-deletion'")
	db.Exec("DELETE FROM background_tasks WHERE task_type = 'housekeeper_notify' AND JSON_EXTRACT(data, '$.summary') = '\"Found 5 IDs\"'")
}

func TestListTasksReturnsAll(t *testing.T) {
	prefix := uniquePrefix("hklist")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, userID)

	// Seed two tasks.
	db.Exec(`INSERT INTO housekeeper_tasks (task_key, name, description, interval_hours, enabled, placeholder, last_run_at, last_status, updated_at)
		VALUES ('test-task-a', 'Task A', 'Desc A', 168, 1, 0, NOW(), 'success', NOW()),
		       ('test-task-b', 'Task B', 'Desc B', 24, 1, 0, DATE_SUB(NOW(), INTERVAL 48 HOUR), 'success', NOW())`)

	req := httptest.NewRequest("GET", "/api/housekeeper/tasks?jwt="+token, nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var tasks []map[string]interface{}
	err = json.Unmarshal(respBody, &tasks)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(tasks), 2, "Should return at least 2 tasks")

	// Find test-task-b and verify it's overdue (last run 48h ago, interval 24h).
	for _, task := range tasks {
		if task["task_key"] == "test-task-b" {
			assert.True(t, task["overdue"].(bool), "Task B should be overdue")
		}
		if task["task_key"] == "test-task-a" {
			assert.False(t, task["overdue"].(bool), "Task A should not be overdue")
		}
	}

	// Clean up.
	db.Exec("DELETE FROM housekeeper_tasks WHERE task_key IN ('test-task-a', 'test-task-b')")
}

func TestListTasksRequiresAdminOrSupport(t *testing.T) {
	prefix := uniquePrefix("hklistauth")

	// No auth → 401.
	req := httptest.NewRequest("GET", "/api/housekeeper/tasks", nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	// Regular user → 403.
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	req = httptest.NewRequest("GET", "/api/housekeeper/tasks?jwt="+token, nil)
	resp, err = getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)

	// Support user → 200.
	supportID := CreateTestUser(t, prefix+"_support", "Support")
	_, supportToken := CreateTestSession(t, supportID)

	req = httptest.NewRequest("GET", "/api/housekeeper/tasks?jwt="+supportToken, nil)
	resp, err = getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestListCronJobsReturnsData(t *testing.T) {
	prefix := uniquePrefix("hkcron")

	userID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, userID)

	req := httptest.NewRequest("GET", "/api/housekeeper/cronjobs?jwt="+token, nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var jobs []map[string]interface{}
	err = json.Unmarshal(respBody, &jobs)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(jobs), 10, "Should return at least 10 cron jobs")

	// Verify structure of first job.
	firstJob := jobs[0]
	assert.NotEmpty(t, firstJob["command"])
	assert.NotEmpty(t, firstJob["name"])
	assert.NotEmpty(t, firstJob["description"])
	assert.NotEmpty(t, firstJob["schedule"])
	assert.NotEmpty(t, firstJob["category"])
}

func TestListCronJobsRequiresAdminOrSupport(t *testing.T) {
	prefix := uniquePrefix("hkcronauth")

	// No auth → 401.
	req := httptest.NewRequest("GET", "/api/housekeeper/cronjobs", nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	// Regular user → 403.
	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	req = httptest.NewRequest("GET", "/api/housekeeper/cronjobs?jwt="+token, nil)
	resp, err = getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestListCronJobsIncludesLastRunData(t *testing.T) {
	prefix := uniquePrefix("hkcronrun")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, userID)

	// Seed a cron_job_status row matching a known cron job command.
	db.Exec(`INSERT INTO cron_job_status (command, last_run_at, last_finished_at, last_exit_code, last_output, updated_at)
		VALUES ('deploy:watch', NOW(), NOW(), 0, 'Version unchanged', NOW())`)

	// Also seed one with flags to test prefix matching.
	db.Exec(`INSERT INTO cron_job_status (command, last_run_at, last_finished_at, last_exit_code, last_output, updated_at)
		VALUES ('mail:chat:user2user --max-iterations=60 --spool', NOW(), NOW(), 0, 'Processed 5 chats', NOW())`)

	req := httptest.NewRequest("GET", "/api/housekeeper/cronjobs?jwt="+token, nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var jobs []map[string]interface{}
	err = json.Unmarshal(respBody, &jobs)
	assert.NoError(t, err)

	// Find deploy:watch and verify it has last_run_at.
	for _, job := range jobs {
		if job["command"] == "deploy:watch" {
			assert.NotNil(t, job["last_run_at"], "deploy:watch should have last_run_at from DB")
			assert.Equal(t, float64(0), job["last_exit_code"], "deploy:watch should have exit code 0")
			assert.Equal(t, "Version unchanged", job["last_output"])
		}
		if job["command"] == "mail:chat:user2user" {
			assert.NotNil(t, job["last_run_at"], "mail:chat:user2user should match via prefix")
			assert.Equal(t, "Processed 5 chats", job["last_output"])
		}
	}

	// Clean up.
	db.Exec("DELETE FROM cron_job_status WHERE command IN ('deploy:watch', 'mail:chat:user2user --max-iterations=60 --spool')")
}

func TestSessionWorkIncludesHousekeeping(t *testing.T) {
	prefix := uniquePrefix("hkwork")
	db := database.DBConn

	// Create an admin user with a group membership (needed for work counts).
	userID := CreateTestUser(t, prefix+"_admin", "Admin")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Moderator")
	_, token := CreateTestSession(t, userID)

	// Insert a failed, enabled housekeeper task.
	db.Exec(`INSERT INTO housekeeper_tasks (task_key, name, interval_hours, enabled, placeholder, last_run_at, last_status, updated_at)
		VALUES (?, 'Test Failed Task', 168, 1, 0, NOW(), 'failure', NOW())`, prefix+"_failed")

	// Insert an overdue, enabled housekeeper task.
	db.Exec(`INSERT INTO housekeeper_tasks (task_key, name, interval_hours, enabled, placeholder, last_run_at, last_status, updated_at)
		VALUES (?, 'Test Overdue Task', 1, 1, 0, DATE_SUB(NOW(), INTERVAL 48 HOUR), 'success', NOW())`, prefix+"_overdue")

	req := httptest.NewRequest("GET", "/api/session?jwt="+token, nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var session map[string]interface{}
	err = json.Unmarshal(respBody, &session)
	assert.NoError(t, err)

	work, ok := session["work"].(map[string]interface{})
	assert.True(t, ok, "Session should contain work object")

	housekeeping, ok := work["housekeeping"]
	assert.True(t, ok, "Work should contain housekeeping field")
	assert.GreaterOrEqual(t, housekeeping.(float64), float64(2), "Should have at least 2 housekeeping issues (failed + overdue)")

	// Clean up.
	db.Exec("DELETE FROM housekeeper_tasks WHERE task_key IN (?, ?)", prefix+"_failed", prefix+"_overdue")
}
