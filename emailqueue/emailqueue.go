package emailqueue

import (
	"encoding/json"
	"github.com/freegle/iznik-server-go/database"
	"time"
)

// EmailType constants for the email_queue table.
const (
	TypeForgotPassword = "forgot_password"
	TypeVerifyEmail    = "verify_email"
	TypeWelcome        = "welcome"
	TypeUnsubscribe    = "unsubscribe"
	TypeMergeOffer     = "merge_offer"
	TypeModMail        = "modmail"
)

// EmailQueueItem represents a row in the email_queue table.
type EmailQueueItem struct {
	ID           uint64     `json:"id" gorm:"primaryKey;column:id"`
	EmailType    string     `json:"email_type" gorm:"column:email_type"`
	UserID       *uint64    `json:"user_id" gorm:"column:user_id"`
	GroupID      *uint64    `json:"group_id" gorm:"column:group_id"`
	MessageID    *uint64    `json:"message_id" gorm:"column:message_id"`
	ChatID       *uint64    `json:"chat_id" gorm:"column:chat_id"`
	ExtraData    *string    `json:"extra_data" gorm:"column:extra_data;type:json"`
	CreatedAt    time.Time  `json:"created_at" gorm:"column:created_at"`
	ProcessedAt  *time.Time `json:"processed_at" gorm:"column:processed_at"`
	FailedAt     *time.Time `json:"failed_at" gorm:"column:failed_at"`
	ErrorMessage *string    `json:"error_message" gorm:"column:error_message"`
}

func (EmailQueueItem) TableName() string {
	return "email_queue"
}

// marshalExtraData converts a map to a JSON string pointer, or nil if the map is nil.
func marshalExtraData(extraData map[string]interface{}) (*string, error) {
	if extraData == nil {
		return nil, nil
	}
	jsonBytes, err := json.Marshal(extraData)
	if err != nil {
		return nil, err
	}
	jsonStr := string(jsonBytes)
	return &jsonStr, nil
}

// QueueEmail inserts an email request into the queue for Laravel to process.
func QueueEmail(emailType string, userID *uint64, groupID *uint64, extraData map[string]interface{}) error {
	extra, err := marshalExtraData(extraData)
	if err != nil {
		return err
	}

	item := EmailQueueItem{
		EmailType: emailType,
		UserID:    userID,
		GroupID:   groupID,
		ExtraData: extra,
	}

	return database.DBConn.Create(&item).Error
}

// QueueEmailWithMessage inserts an email request with a message ID.
func QueueEmailWithMessage(emailType string, userID *uint64, messageID *uint64, extraData map[string]interface{}) error {
	extra, err := marshalExtraData(extraData)
	if err != nil {
		return err
	}

	item := EmailQueueItem{
		EmailType: emailType,
		UserID:    userID,
		MessageID: messageID,
		ExtraData: extra,
	}

	return database.DBConn.Create(&item).Error
}

// QueueEmailWithChat inserts an email request with a chat ID.
func QueueEmailWithChat(emailType string, userID *uint64, chatID *uint64, extraData map[string]interface{}) error {
	extra, err := marshalExtraData(extraData)
	if err != nil {
		return err
	}

	item := EmailQueueItem{
		EmailType: emailType,
		UserID:    userID,
		ChatID:    chatID,
		ExtraData: extra,
	}

	return database.DBConn.Create(&item).Error
}
