package housekeeper

import (
	"log"
	"time"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/queue"
	"github.com/gofiber/fiber/v2"
)

// TaskHousekeeperNotify is the background_tasks type for housekeeping notifications.
const TaskHousekeeperNotify = "housekeeper_notify"

// TaskInfo describes a single task from the extension's registry.
type TaskInfo struct {
	TaskKey       string `json:"task_key"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	IntervalHours int    `json:"interval_hours"`
	Enabled       bool   `json:"enabled"`
	Placeholder   bool   `json:"placeholder"`
}

// NotifyRequest is the JSON body sent by the Chrome extension.
type NotifyRequest struct {
	Task      string      `json:"task"`
	Status    string      `json:"status"`
	Summary   string      `json:"summary"`
	Timestamp string      `json:"timestamp"`
	Email     string      `json:"email"`
	Data      interface{} `json:"data"`
	Registry  []TaskInfo  `json:"registry"`
}

// HousekeeperTask is the DB row returned by ListTasks.
type HousekeeperTask struct {
	ID            uint64     `json:"id"`
	TaskKey       string     `json:"task_key"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	IntervalHours int        `json:"interval_hours"`
	Enabled       bool       `json:"enabled"`
	Placeholder   bool       `json:"placeholder"`
	LastRunAt     *time.Time `json:"last_run_at"`
	LastStatus    *string    `json:"last_status"`
	LastSummary   *string    `json:"last_summary"`
	UpdatedAt     time.Time  `json:"updated_at"`
	Overdue       bool       `json:"overdue"`
}

// Notify receives housekeeping task results from the Chrome extension and
// queues a background task for Laravel to process.
//
// Requires system admin authentication via JWT.
func Notify(c *fiber.Ctx) error {
	myid := auth.WhoAmI(c)

	if myid == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Not logged in"})
	}

	if !auth.IsAdmin(myid) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "System admin required"})
	}

	var req NotifyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON"})
	}

	if req.Task == "" || req.Status == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "task and status are required"})
	}

	log.Printf("[Housekeeper] User %d submitted %s result: %s — %s", myid, req.Task, req.Status, req.Summary)

	// Queue for Laravel processing.
	err := queue.QueueTask(TaskHousekeeperNotify, map[string]interface{}{
		"task":      req.Task,
		"status":    req.Status,
		"summary":   req.Summary,
		"timestamp": req.Timestamp,
		"email":     req.Email,
		"data":      req.Data,
	})

	if err != nil {
		log.Printf("[Housekeeper] Failed to queue task: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to queue"})
	}

	// Upsert registry entries if provided.
	if len(req.Registry) > 0 {
		upsertRegistry(req.Registry)
	}

	// Update last_run for the current task.
	upsertLastRun(req.Task, req.Status, req.Summary)

	return c.JSON(fiber.Map{"queued": true})
}

// upsertRegistry inserts or updates housekeeper_tasks rows from the extension's registry.
func upsertRegistry(registry []TaskInfo) {
	db := database.DBConn

	for _, t := range registry {
		if t.TaskKey == "" {
			continue
		}

		db.Exec(`INSERT INTO housekeeper_tasks (task_key, name, description, interval_hours, enabled, placeholder, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, NOW())
			ON DUPLICATE KEY UPDATE
				name = VALUES(name),
				description = VALUES(description),
				interval_hours = VALUES(interval_hours),
				enabled = VALUES(enabled),
				placeholder = VALUES(placeholder),
				updated_at = NOW()`,
			t.TaskKey, t.Name, t.Description, t.IntervalHours, t.Enabled, t.Placeholder)
	}
}

// upsertLastRun updates the last_run_at, last_status, and last_summary for a task.
func upsertLastRun(taskKey, status, summary string) {
	db := database.DBConn

	db.Exec(`INSERT INTO housekeeper_tasks (task_key, name, last_run_at, last_status, last_summary, updated_at)
		VALUES (?, ?, NOW(), ?, ?, NOW())
		ON DUPLICATE KEY UPDATE
			last_run_at = NOW(),
			last_status = VALUES(last_status),
			last_summary = VALUES(last_summary),
			updated_at = NOW()`,
		taskKey, taskKey, status, summary)
}

// ListTasks returns all housekeeper tasks with an overdue flag.
//
// Requires system admin authentication via JWT.
func ListTasks(c *fiber.Ctx) error {
	myid := auth.WhoAmI(c)

	if myid == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Not logged in"})
	}

	if !auth.IsAdmin(myid) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "System admin required"})
	}

	db := database.DBConn

	var tasks []HousekeeperTask
	db.Raw(`SELECT id, task_key, name, description, interval_hours, enabled, placeholder,
			last_run_at, last_status, last_summary, updated_at
		FROM housekeeper_tasks
		ORDER BY task_key`).Scan(&tasks)

	// Compute overdue flag.
	now := time.Now()
	for i := range tasks {
		if tasks[i].LastRunAt == nil {
			tasks[i].Overdue = true
		} else {
			deadline := tasks[i].LastRunAt.Add(time.Duration(tasks[i].IntervalHours) * time.Hour)
			tasks[i].Overdue = now.After(deadline)
		}
	}

	return c.JSON(tasks)
}
