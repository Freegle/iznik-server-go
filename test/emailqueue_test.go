package test

import (
	json2 "encoding/json"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/emailqueue"
	"github.com/stretchr/testify/assert"
)

func TestQueueEmail_Basic(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("eq")

	userID := uint64(99999)
	groupID := uint64(88888)

	err := emailqueue.QueueEmail(
		emailqueue.TypeForgotPassword,
		&userID,
		&groupID,
		map[string]interface{}{"email": prefix + "@test.com"},
	)
	assert.NoError(t, err)

	// Verify the row was created.
	var item emailqueue.EmailQueueItem
	result := db.Where("email_type = ? AND extra_data LIKE ?", emailqueue.TypeForgotPassword, "%"+prefix+"%").
		Order("id DESC").First(&item)
	assert.NoError(t, result.Error)
	assert.Equal(t, emailqueue.TypeForgotPassword, item.EmailType)
	assert.Equal(t, userID, *item.UserID)
	assert.Equal(t, groupID, *item.GroupID)
	assert.Nil(t, item.ProcessedAt)
	assert.Nil(t, item.FailedAt)

	// Verify extra_data JSON.
	var extra map[string]interface{}
	err = json2.Unmarshal([]byte(*item.ExtraData), &extra)
	assert.NoError(t, err)
	assert.Equal(t, prefix+"@test.com", extra["email"])

	// Cleanup.
	db.Delete(&item)
}

func TestQueueEmail_NilExtraData(t *testing.T) {
	db := database.DBConn

	userID := uint64(99998)

	err := emailqueue.QueueEmail(
		emailqueue.TypeUnsubscribe,
		&userID,
		nil,
		nil,
	)
	assert.NoError(t, err)

	var item emailqueue.EmailQueueItem
	result := db.Where("email_type = ? AND user_id = ?", emailqueue.TypeUnsubscribe, 99998).
		Order("id DESC").First(&item)
	assert.NoError(t, result.Error)
	assert.Nil(t, item.ExtraData)
	assert.Nil(t, item.GroupID)

	db.Delete(&item)
}

func TestQueueEmailWithMessage(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("eqm")

	userID := uint64(99997)
	messageID := uint64(77777)

	err := emailqueue.QueueEmailWithMessage(
		emailqueue.TypeModMail,
		&userID,
		&messageID,
		map[string]interface{}{"subject": prefix + " test subject"},
	)
	assert.NoError(t, err)

	var item emailqueue.EmailQueueItem
	result := db.Where("email_type = ? AND extra_data LIKE ?", emailqueue.TypeModMail, "%"+prefix+"%").
		Order("id DESC").First(&item)
	assert.NoError(t, result.Error)
	assert.Equal(t, messageID, *item.MessageID)
	assert.Nil(t, item.ChatID)

	db.Delete(&item)
}

func TestQueueEmailWithChat(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("eqc")

	userID := uint64(99996)
	chatID := uint64(66666)

	err := emailqueue.QueueEmailWithChat(
		"chat_notification",
		&userID,
		&chatID,
		map[string]interface{}{"sender": prefix},
	)
	assert.NoError(t, err)

	var item emailqueue.EmailQueueItem
	result := db.Where("email_type = ? AND extra_data LIKE ?", "chat_notification", "%"+prefix+"%").
		Order("id DESC").First(&item)
	assert.NoError(t, result.Error)
	assert.Equal(t, chatID, *item.ChatID)
	assert.Nil(t, item.MessageID)
	assert.Nil(t, item.GroupID)

	db.Delete(&item)
}

func TestQueueEmail_AllTypes(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("eqt")

	types := []string{
		emailqueue.TypeForgotPassword,
		emailqueue.TypeVerifyEmail,
		emailqueue.TypeWelcome,
		emailqueue.TypeUnsubscribe,
		emailqueue.TypeMergeOffer,
		emailqueue.TypeModMail,
	}

	userID := uint64(99995)

	for _, emailType := range types {
		err := emailqueue.QueueEmail(
			emailType,
			&userID,
			nil,
			map[string]interface{}{"prefix": prefix, "type": emailType},
		)
		assert.NoError(t, err, "Should queue %s without error", emailType)
	}

	// Verify all were created.
	var count int64
	db.Model(&emailqueue.EmailQueueItem{}).
		Where("user_id = ? AND extra_data LIKE ?", 99995, "%"+prefix+"%").
		Count(&count)
	assert.Equal(t, int64(len(types)), count)

	// Cleanup.
	db.Where("user_id = ? AND extra_data LIKE ?", 99995, "%"+prefix+"%").
		Delete(&emailqueue.EmailQueueItem{})
}
