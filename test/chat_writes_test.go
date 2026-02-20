package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// PutChatRoom tests
// =============================================================================

func TestPutChatRoom(t *testing.T) {
	prefix := uniquePrefix("putchat")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token := CreateTestSession(t, user1ID)

	// Create a new chat room.
	payload := map[string]interface{}{"userid": user2ID}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])
	chatID := uint64(result["id"].(float64))
	assert.Greater(t, chatID, uint64(0))

	// Verify chat room was created in DB.
	db := database.DBConn
	var chattype string
	var user1, user2 uint64
	db.Raw("SELECT chattype, user1, user2 FROM chat_rooms WHERE id = ?", chatID).Row().Scan(&chattype, &user1, &user2)
	assert.Equal(t, utils.CHAT_TYPE_USER2USER, chattype)
	assert.Equal(t, user1ID, user1)
	assert.Equal(t, user2ID, user2)

	// Verify roster entries were created for both users.
	var rosterCount int64
	db.Raw("SELECT COUNT(*) FROM chat_roster WHERE chatid = ?", chatID).Scan(&rosterCount)
	assert.Equal(t, int64(2), rosterCount)
}

func TestPutChatRoomAlreadyExists(t *testing.T) {
	prefix := uniquePrefix("putchat_exists")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token := CreateTestSession(t, user1ID)

	// Create a chat room between them first.
	existingChatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")

	// Try to create another chat - should return the existing one.
	payload := map[string]interface{}{"userid": user2ID}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, float64(existingChatID), result["id"])
}

func TestPutChatRoomAlreadyExistsReversed(t *testing.T) {
	// Test that it finds an existing chat even if user1/user2 are reversed.
	prefix := uniquePrefix("putchat_rev")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token2 := CreateTestSession(t, user2ID)

	// Create chat with user1 as user1 and user2 as user2.
	existingChatID := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")

	// Log in as user2 and try to create a chat with user1 - should find existing.
	payload := map[string]interface{}{"userid": user1ID}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token2, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, float64(existingChatID), result["id"])
}

func TestPutChatRoomSelf(t *testing.T) {
	prefix := uniquePrefix("putchat_self")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	// Try to create a chat with yourself.
	payload := map[string]interface{}{"userid": user1ID}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestPutChatRoomNotLoggedIn(t *testing.T) {
	payload := map[string]interface{}{"userid": 12345}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/chat/rooms", bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestPutChatRoomMissingUserid(t *testing.T) {
	prefix := uniquePrefix("putchat_noid")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	payload := map[string]interface{}{}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestPutChatRoomInvalidBody(t *testing.T) {
	prefix := uniquePrefix("putchat_bad")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	request := httptest.NewRequest("PUT", "/api/chat/rooms?jwt="+token, bytes.NewBuffer([]byte("not json")))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

// =============================================================================
// ReferToSupport tests
// =============================================================================

func TestReferToSupport(t *testing.T) {
	prefix := uniquePrefix("refersupport")
	db := database.DBConn

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	CreateTestChatMessage(t, chatid, user1ID, "Hello")
	_, token := CreateTestSession(t, user1ID)

	// Refer to support.
	payload := map[string]interface{}{"id": chatid, "action": "ReferToSupport"}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])

	// Verify a ReferToSupport message was created in the chat.
	var msgType string
	var processingrequired int
	db.Raw("SELECT type, processingrequired FROM chat_messages WHERE chatid = ? AND type = ? ORDER BY id DESC LIMIT 1",
		chatid, utils.CHAT_MESSAGE_REFER_TO_SUPPORT).Row().Scan(&msgType, &processingrequired)
	assert.Equal(t, utils.CHAT_MESSAGE_REFER_TO_SUPPORT, msgType)
	assert.Equal(t, 1, processingrequired)
}

func TestReferToSupportNotMember(t *testing.T) {
	prefix := uniquePrefix("refersupport_nm")

	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	user3ID := CreateTestUser(t, prefix+"_u3", "User")
	chatid := CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")
	_, token3 := CreateTestSession(t, user3ID)

	// User3 tries to refer a chat they're not in.
	payload := map[string]interface{}{"id": chatid, "action": "ReferToSupport"}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token3, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestReferToSupportNotLoggedIn(t *testing.T) {
	payload := map[string]interface{}{"id": 1, "action": "ReferToSupport"}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms", bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestReferToSupportNonExistentChat(t *testing.T) {
	prefix := uniquePrefix("refersupport_ne")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	payload := map[string]interface{}{"id": 999999999, "action": "ReferToSupport"}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestReferToSupportMissingChatID(t *testing.T) {
	prefix := uniquePrefix("refersupport_noid")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	_, token := CreateTestSession(t, user1ID)

	payload := map[string]interface{}{"action": "ReferToSupport"}
	s, _ := json.Marshal(payload)
	request := httptest.NewRequest("POST", "/api/chatrooms?jwt="+token, bytes.NewBuffer(s))
	request.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(request)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}
