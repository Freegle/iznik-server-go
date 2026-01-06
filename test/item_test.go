package test

import (
	"testing"

	"github.com/freegle/iznik-server-go/item"
	"github.com/stretchr/testify/assert"
)

func TestFetchForMessageWithValidMessage(t *testing.T) {
	prefix := uniquePrefix("ItemMsg")

	// Create test data
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")

	// Create a message with specific coordinates
	messageID := CreateTestMessage(t, userID, groupID, "Test Chair for Item Test", 55.9533, -3.1883)

	// Create an item and link it to the message
	itemID := CreateTestItem(t, "chair")
	CreateTestMessageItem(t, messageID, itemID)

	// Test FetchForMessage
	result := item.FetchForMessage(messageID)

	assert.NotNil(t, result, "Result should not be nil")
	assert.Equal(t, itemID, result.ID, "Item ID should match")
	assert.Equal(t, "chair", result.Name, "Item name should be 'chair'")
}

func TestFetchForMessageWithNoItem(t *testing.T) {
	prefix := uniquePrefix("ItemNoLink")

	// Create test data
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")

	// Create a message without linking an item
	messageID := CreateTestMessage(t, userID, groupID, "Test Item No Link", 55.9533, -3.1883)

	// Test FetchForMessage with a message that has no item
	result := item.FetchForMessage(messageID)

	// Result should be an empty struct (ID = 0, Name = "")
	assert.NotNil(t, result, "Result should not be nil")
	assert.Equal(t, uint64(0), result.ID, "Item ID should be 0 for message with no item")
	assert.Equal(t, "", result.Name, "Item name should be empty for message with no item")
}

func TestFetchForMessageWithInvalidMessage(t *testing.T) {
	// Test with a message ID that doesn't exist
	result := item.FetchForMessage(999999999)

	// Result should be an empty struct
	assert.NotNil(t, result, "Result should not be nil")
	assert.Equal(t, uint64(0), result.ID, "Item ID should be 0 for non-existent message")
	assert.Equal(t, "", result.Name, "Item name should be empty for non-existent message")
}
