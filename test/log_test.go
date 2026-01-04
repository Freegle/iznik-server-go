package test

import (
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/log"
	"github.com/stretchr/testify/assert"
)

func TestLogConstants(t *testing.T) {
	// Verify log type constants are defined correctly.
	assert.Equal(t, "Group", log.LOG_TYPE_GROUP)
	assert.Equal(t, "User", log.LOG_TYPE_USER)
	assert.Equal(t, "Message", log.LOG_TYPE_MESSAGE)
	assert.Equal(t, "Config", log.LOG_TYPE_CONFIG)
	assert.Equal(t, "StdMsg", log.LOG_TYPE_STDMSG)
	assert.Equal(t, "BulkOp", log.LOG_TYPE_BULKOP)
	assert.Equal(t, "Location", log.LOG_TYPE_LOCATION)
	assert.Equal(t, "Chat", log.LOG_TYPE_CHAT)
}

func TestLogSubtypeConstants(t *testing.T) {
	// Verify log subtype constants are defined correctly.
	assert.Equal(t, "Created", log.LOG_SUBTYPE_CREATED)
	assert.Equal(t, "Deleted", log.LOG_SUBTYPE_DELETED)
	assert.Equal(t, "Edit", log.LOG_SUBTYPE_EDIT)
	assert.Equal(t, "Approved", log.LOG_SUBTYPE_APPROVED)
	assert.Equal(t, "Rejected", log.LOG_SUBTYPE_REJECTED)
	assert.Equal(t, "Login", log.LOG_SUBTYPE_LOGIN)
	assert.Equal(t, "Logout", log.LOG_SUBTYPE_LOGOUT)
}

func TestLogEntryTableName(t *testing.T) {
	entry := log.LogEntry{}
	assert.Equal(t, "logs", entry.TableName())
}

func TestLogCreatesEntry(t *testing.T) {
	prefix := uniquePrefix("log")
	db := database.DBConn

	// Create a test user to associate with the log.
	userID := CreateTestUser(t, prefix, "User")

	// Create a log entry.
	text := "Test log entry from Go tests"
	entry := log.LogEntry{
		Type:    log.LOG_TYPE_USER,
		Subtype: log.LOG_SUBTYPE_LOGIN,
		User:    &userID,
		Text:    &text,
	}

	log.Log(entry)

	// Verify the log entry was created.
	var count int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE user = ? AND type = ? AND subtype = ?",
		userID, log.LOG_TYPE_USER, log.LOG_SUBTYPE_LOGIN).Scan(&count)

	assert.Greater(t, count, int64(0), "Log entry should have been created")

	// Clean up.
	db.Exec("DELETE FROM logs WHERE user = ? AND text = ?", userID, text)
}

func TestLogWithGroupID(t *testing.T) {
	prefix := uniquePrefix("loggrp")
	db := database.DBConn

	// Create test data.
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")

	// Create a log entry with group ID.
	text := "Test group log entry"
	entry := log.LogEntry{
		Type:    log.LOG_TYPE_GROUP,
		Subtype: log.LOG_SUBTYPE_JOINED,
		Groupid: &groupID,
		User:    &userID,
		Text:    &text,
	}

	log.Log(entry)

	// Verify the log entry was created with the group ID.
	var foundGroupID uint64
	db.Raw("SELECT groupid FROM logs WHERE user = ? AND type = ? ORDER BY id DESC LIMIT 1",
		userID, log.LOG_TYPE_GROUP).Scan(&foundGroupID)

	assert.Equal(t, groupID, foundGroupID, "Log entry should have correct group ID")

	// Clean up.
	db.Exec("DELETE FROM logs WHERE user = ? AND text = ?", userID, text)
}

func TestLogWithMessageID(t *testing.T) {
	prefix := uniquePrefix("logmsg")
	db := database.DBConn

	// Create test data.
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	messageID := CreateTestMessage(t, userID, groupID, "Test message for log", 55.9533, -3.1883)

	// Create a log entry with message ID.
	text := "Test message log entry"
	entry := log.LogEntry{
		Type:    log.LOG_TYPE_MESSAGE,
		Subtype: log.LOG_SUBTYPE_APPROVED,
		Msgid:   &messageID,
		User:    &userID,
		Text:    &text,
	}

	log.Log(entry)

	// Verify the log entry was created with the message ID.
	var foundMsgID uint64
	db.Raw("SELECT msgid FROM logs WHERE user = ? AND type = ? ORDER BY id DESC LIMIT 1",
		userID, log.LOG_TYPE_MESSAGE).Scan(&foundMsgID)

	assert.Equal(t, messageID, foundMsgID, "Log entry should have correct message ID")

	// Clean up.
	db.Exec("DELETE FROM logs WHERE user = ? AND text = ?", userID, text)
}

func TestLogWithByUser(t *testing.T) {
	prefix := uniquePrefix("logby")
	db := database.DBConn

	// Create test users.
	targetUserID := CreateTestUser(t, prefix+"_target", "User")
	modUserID := CreateTestUser(t, prefix+"_mod", "User")

	// Create a log entry where one user performs action on another.
	text := "Test byuser log entry"
	entry := log.LogEntry{
		Type:    log.LOG_TYPE_USER,
		Subtype: log.LOG_SUBTYPE_EDIT,
		User:    &targetUserID,
		Byuser:  &modUserID,
		Text:    &text,
	}

	log.Log(entry)

	// Verify the log entry was created with byuser.
	var foundByUser uint64
	db.Raw("SELECT byuser FROM logs WHERE user = ? AND type = ? AND subtype = ? ORDER BY id DESC LIMIT 1",
		targetUserID, log.LOG_TYPE_USER, log.LOG_SUBTYPE_EDIT).Scan(&foundByUser)

	assert.Equal(t, modUserID, foundByUser, "Log entry should have correct byuser")

	// Clean up.
	db.Exec("DELETE FROM logs WHERE user = ? AND text = ?", targetUserID, text)
}

func TestLogTimestampIsSet(t *testing.T) {
	prefix := uniquePrefix("logts")
	db := database.DBConn

	userID := CreateTestUser(t, prefix, "User")

	// Create a log entry.
	text := "Test timestamp log entry"
	entry := log.LogEntry{
		Type:    log.LOG_TYPE_USER,
		Subtype: log.LOG_SUBTYPE_LOGIN,
		User:    &userID,
		Text:    &text,
	}

	log.Log(entry)

	// Verify the timestamp was set.
	var timestamp string
	db.Raw("SELECT timestamp FROM logs WHERE user = ? AND text = ? ORDER BY id DESC LIMIT 1",
		userID, text).Scan(&timestamp)

	assert.NotEmpty(t, timestamp, "Timestamp should be set")

	// Clean up.
	db.Exec("DELETE FROM logs WHERE user = ? AND text = ?", userID, text)
}
