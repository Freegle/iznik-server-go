package queue

import (
	"encoding/json"
	"github.com/freegle/iznik-server-go/database"
	"log"
)

// Task types for the background_tasks queue.
// Go inserts rows with these types; iznik-batch processes them.
const (
	// TaskPushNotifyGroupMods sends FCM push notifications to all moderators of a group.
	TaskPushNotifyGroupMods = "push_notify_group_mods"

	// TaskEmailChitchatReport sends a report email to ChitChat support when a newsfeed post is reported.
	TaskEmailChitchatReport = "email_chitchat_report"

	// TaskEmailDonateExternal sends a notification email to info@ilovefreegle.org when an external donation is recorded.
	TaskEmailDonateExternal = "email_donate_external"

	// TaskEmailForgotPassword sends a password reset email with auto-login link.
	TaskEmailForgotPassword = "email_forgot_password"

	// TaskEmailUnsubscribe sends an unsubscribe confirmation email with auto-login link.
	TaskEmailUnsubscribe = "email_unsubscribe"

	// TaskEmailMerge sends merge offer emails to both users involved in a merge.
	TaskEmailMerge = "email_merge"

	// TaskEmailVerify sends a verification email when a user adds a new email address.
	TaskEmailVerify = "email_verify"
)

// QueueTask inserts a task into the background_tasks table for async processing by iznik-batch.
func QueueTask(taskType string, data map[string]interface{}) error {
	db := database.DBConn

	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("Failed to marshal task data for type %s: %v", taskType, err)
		return err
	}

	result := db.Exec(
		"INSERT INTO background_tasks (task_type, data) VALUES (?, ?)",
		taskType, string(jsonData),
	)

	if result.Error != nil {
		log.Printf("Failed to queue task type %s: %v", taskType, result.Error)
		return result.Error
	}

	return nil
}
