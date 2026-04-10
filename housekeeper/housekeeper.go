package housekeeper

import (
	"log"
	"strings"
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
	LastLog       *string    `json:"last_log"`
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
// Requires support or admin authentication via JWT.
func ListTasks(c *fiber.Ctx) error {
	myid := auth.WhoAmI(c)

	if myid == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Not logged in"})
	}

	if !auth.IsAdminOrSupport(myid) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Support or admin required"})
	}

	db := database.DBConn

	var tasks []HousekeeperTask
	result := db.Raw(`SELECT * FROM housekeeper_tasks ORDER BY task_key`).Scan(&tasks)

	if result.Error != nil {
		log.Printf("[Housekeeper] ListTasks query error: %v", result.Error)
		return c.JSON([]HousekeeperTask{})
	}

	if tasks == nil {
		tasks = []HousekeeperTask{}
	}

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

// CronJob describes a scheduled Laravel command for display in the SysAdmin dashboard.
type CronJob struct {
	Command        string     `json:"command"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Schedule       string     `json:"schedule"`
	Category       string     `json:"category"`
	Active         bool       `json:"active"`
	LastRunAt      *time.Time `json:"last_run_at"`
	LastFinishedAt *time.Time `json:"last_finished_at"`
	LastExitCode   *int       `json:"last_exit_code"`
	LastOutput     *string    `json:"last_output"`
}

// cronJobStatus is the DB row from cron_job_status.
type cronJobStatus struct {
	Command        string     `json:"command"`
	LastRunAt      *time.Time `json:"last_run_at"`
	LastFinishedAt *time.Time `json:"last_finished_at"`
	LastExitCode   *int       `json:"last_exit_code"`
	LastOutput     *string    `json:"last_output"`
}

// cronJobs is the static registry of all Laravel scheduled commands.
var cronJobs = []CronJob{
	// System
	{Command: "deploy:watch", Name: "Deployment Watcher", Description: "Detects code updates and auto-refreshes application", Schedule: "Every minute", Category: "System", Active: true},
	{Command: "queue:background-tasks", Name: "Background Task Queue", Description: "Processes tasks queued by Go API (push notifications, emails)", Schedule: "Every minute", Category: "System", Active: true},

	// Email — Chat Notifications
	{Command: "mail:chat:user2user", Name: "Chat: User to User", Description: "Sends email notifications for user-to-user chat messages", Schedule: "Every minute", Category: "Email — Chat", Active: true},
	{Command: "mail:chat:mod2mod", Name: "Chat: Mod to Mod", Description: "Sends email notifications for moderator-to-moderator chat messages", Schedule: "Every minute", Category: "Email — Chat", Active: true},
	{Command: "mail:chat:user2mod", Name: "Chat: User to Mod", Description: "Sends email notifications for user-to-moderator chat messages", Schedule: "Every minute", Category: "Email — Chat", Active: true},

	// Email — Member Engagement
	{Command: "mail:welcome:send", Name: "Welcome Emails", Description: "Sends welcome emails to new members", Schedule: "Every minute", Category: "Email — Engagement", Active: true},

	// Email — Admin
	{Command: "mail:admin:copy", Name: "Admin Copy", Description: "Creates per-group copies of suggested admin emails for moderator approval", Schedule: "Every minute", Category: "Email — Admin", Active: true},
	{Command: "mail:admin:send", Name: "Admin Send", Description: "Sends approved admin emails to group members", Schedule: "Every minute", Category: "Email — Admin", Active: true},
	{Command: "mail:admin:chase", Name: "Admin Chase", Description: "Reminds moderators about pending suggested admin emails (after 48h)", Schedule: "Hourly", Category: "Email — Admin", Active: true},

	// Data & Cleanup
	{Command: "mail:spool:process --cleanup", Name: "Spool Cleanup", Description: "Cleans up sent emails older than 7 days from spool directory", Schedule: "Daily at 4am", Category: "Cleanup", Active: true},
	{Command: "mail:cleanup-archive", Name: "Email Archive Cleanup", Description: "Removes incoming email archives older than 48 hours", Schedule: "Hourly", Category: "Cleanup", Active: true},
	{Command: "data:update-cpi", Name: "CPI Data Update", Description: "Fetches UK CPI inflation data from ONS for reuse benefit calculations", Schedule: "Monthly", Category: "Data", Active: true},

	// AI & Analytics
	{Command: "ai:usage-counts:update", Name: "AI Usage Counts", Description: "Updates usage counts for AI-generated images across posts", Schedule: "Hourly", Category: "AI & Analytics", Active: true},
	{Command: "mail:ai-image-review:digest", Name: "AI Image Review Digest", Description: "Sends daily digest of AI image review verdicts to geeks", Schedule: "Daily at 12pm", Category: "AI & Analytics", Active: true},
	{Command: "data:git-summary", Name: "Git Summary", Description: "Sends AI-powered summary of weekly code changes to Discourse", Schedule: "Weekly (Wed 6pm)", Category: "AI & Analytics", Active: true},
}

// ListCronJobs returns the static list of Laravel scheduled commands enriched
// with last-run data from the cron_job_status table.
//
// Requires support or admin authentication.
func ListCronJobs(c *fiber.Ctx) error {
	myid := auth.WhoAmI(c)

	if myid == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Not logged in"})
	}

	if !auth.IsAdminOrSupport(myid) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Support or admin required"})
	}

	db := database.DBConn

	var statuses []cronJobStatus
	statusResult := db.Raw(`SELECT * FROM cron_job_status`).Scan(&statuses)

	if statusResult.Error != nil {
		log.Printf("[Housekeeper] ListCronJobs cron_job_status query error: %v", statusResult.Error)
	}

	// Build a map keyed by command for fast lookup.
	statusMap := make(map[string]*cronJobStatus, len(statuses))
	for i := range statuses {
		statusMap[statuses[i].Command] = &statuses[i]
	}

	// Build result by merging static metadata with DB status.
	// Match DB rows where the stored command starts with the static command name.
	result := make([]CronJob, len(cronJobs))
	for i, job := range cronJobs {
		result[i] = job

		// Exact match first.
		if s, ok := statusMap[job.Command]; ok {
			result[i].LastRunAt = s.LastRunAt
			result[i].LastFinishedAt = s.LastFinishedAt
			result[i].LastExitCode = s.LastExitCode
			result[i].LastOutput = s.LastOutput
			continue
		}

		// Prefix match: DB command may include flags (e.g. "mail:chat:user2user --max-iterations=60 --spool").
		for cmd, s := range statusMap {
			if strings.HasPrefix(cmd, job.Command+" ") || strings.HasPrefix(cmd, job.Command+"\t") {
				result[i].LastRunAt = s.LastRunAt
				result[i].LastFinishedAt = s.LastFinishedAt
				result[i].LastExitCode = s.LastExitCode
				result[i].LastOutput = s.LastOutput
				break
			}
		}
	}

	return c.JSON(result)
}
