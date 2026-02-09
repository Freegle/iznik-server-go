package test

import (
	"encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/queue"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestQueueTask(t *testing.T) {
	prefix := uniquePrefix("queue_basic")
	db := database.DBConn

	// Ensure table exists (CI migration should handle this, but be safe).
	db.Exec(`CREATE TABLE IF NOT EXISTS background_tasks (
		id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
		task_type VARCHAR(50) NOT NULL,
		data JSON NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		processed_at TIMESTAMP NULL,
		failed_at TIMESTAMP NULL,
		error_message TEXT NULL,
		attempts INT UNSIGNED DEFAULT 0,
		INDEX idx_task_type (task_type),
		INDEX idx_pending (processed_at, created_at)
	)`)

	data := map[string]interface{}{
		"group_id": 42,
		"test_ref": prefix,
	}

	err := queue.QueueTask(queue.TaskPushNotifyGroupMods, data)
	assert.NoError(t, err)

	// Verify the task was inserted.
	type Task struct {
		ID          uint64  `json:"id"`
		TaskType    string  `json:"task_type"`
		Data        string  `json:"data"`
		ProcessedAt *string `json:"processed_at"`
		Attempts    int     `json:"attempts"`
	}
	var task Task
	db.Raw("SELECT id, task_type, data, processed_at, attempts FROM background_tasks WHERE data LIKE ? ORDER BY id DESC LIMIT 1",
		fmt.Sprintf("%%%s%%", prefix)).Scan(&task)

	assert.NotZero(t, task.ID)
	assert.Equal(t, queue.TaskPushNotifyGroupMods, task.TaskType)
	assert.Nil(t, task.ProcessedAt)
	assert.Equal(t, 0, task.Attempts)

	// Verify JSON data is correct.
	var parsedData map[string]interface{}
	err = json.Unmarshal([]byte(task.Data), &parsedData)
	assert.NoError(t, err)
	assert.Equal(t, float64(42), parsedData["group_id"])
	assert.Equal(t, prefix, parsedData["test_ref"])

	// Clean up.
	db.Exec("DELETE FROM background_tasks WHERE id = ?", task.ID)
}

func TestQueueTaskEmailReport(t *testing.T) {
	prefix := uniquePrefix("queue_email")
	db := database.DBConn

	// Ensure table exists.
	db.Exec(`CREATE TABLE IF NOT EXISTS background_tasks (
		id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
		task_type VARCHAR(50) NOT NULL,
		data JSON NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		processed_at TIMESTAMP NULL,
		failed_at TIMESTAMP NULL,
		error_message TEXT NULL,
		attempts INT UNSIGNED DEFAULT 0,
		INDEX idx_task_type (task_type),
		INDEX idx_pending (processed_at, created_at)
	)`)

	data := map[string]interface{}{
		"user_id":     123,
		"user_name":   "Test User",
		"user_email":  "test@example.com",
		"newsfeed_id": 456,
		"reason":      "Inappropriate content",
		"test_ref":    prefix,
	}

	err := queue.QueueTask(queue.TaskEmailChitchatReport, data)
	assert.NoError(t, err)

	// Verify the task was inserted with correct type.
	var taskType string
	db.Raw("SELECT task_type FROM background_tasks WHERE data LIKE ? ORDER BY id DESC LIMIT 1",
		fmt.Sprintf("%%%s%%", prefix)).Scan(&taskType)

	assert.Equal(t, queue.TaskEmailChitchatReport, taskType)

	// Clean up.
	db.Exec("DELETE FROM background_tasks WHERE data LIKE ?", fmt.Sprintf("%%%s%%", prefix))
}

func TestQueueMultipleTasks(t *testing.T) {
	prefix := uniquePrefix("queue_multi")
	db := database.DBConn

	// Ensure table exists.
	db.Exec(`CREATE TABLE IF NOT EXISTS background_tasks (
		id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
		task_type VARCHAR(50) NOT NULL,
		data JSON NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		processed_at TIMESTAMP NULL,
		failed_at TIMESTAMP NULL,
		error_message TEXT NULL,
		attempts INT UNSIGNED DEFAULT 0,
		INDEX idx_task_type (task_type),
		INDEX idx_pending (processed_at, created_at)
	)`)

	// Queue multiple tasks.
	for i := 0; i < 3; i++ {
		data := map[string]interface{}{
			"group_id": i + 1,
			"test_ref": prefix,
		}
		err := queue.QueueTask(queue.TaskPushNotifyGroupMods, data)
		assert.NoError(t, err)
	}

	// Verify all were inserted.
	var count int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE data LIKE ?",
		fmt.Sprintf("%%%s%%", prefix)).Scan(&count)
	assert.Equal(t, int64(3), count)

	// Clean up.
	db.Exec("DELETE FROM background_tasks WHERE data LIKE ?", fmt.Sprintf("%%%s%%", prefix))
}
