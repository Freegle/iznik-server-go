package housekeeper

import (
	"log"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/queue"
	"github.com/gofiber/fiber/v2"
)

// TaskHousekeeperNotify is the background_tasks type for housekeeping notifications.
const TaskHousekeeperNotify = "housekeeper_notify"

// NotifyRequest is the JSON body sent by the Chrome extension.
type NotifyRequest struct {
	Task      string      `json:"task"`
	Status    string      `json:"status"`
	Summary   string      `json:"summary"`
	Timestamp string      `json:"timestamp"`
	Email     string      `json:"email"`
	Data      interface{} `json:"data"`
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

	return c.JSON(fiber.Map{"queued": true})
}
