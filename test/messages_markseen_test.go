package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestMessagesMarkSeen(t *testing.T) {
	prefix := uniquePrefix("markseen")
	groupID := CreateTestGroup(t, prefix)

	// Create message owner and viewer
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	CreateTestMembership(t, ownerID, groupID, "Member")

	viewerID := CreateTestUser(t, prefix+"_viewer", "User")
	CreateTestMembership(t, viewerID, groupID, "Member")
	_, viewerToken := CreateTestSession(t, viewerID)

	// Create messages
	msgID1 := CreateTestMessage(t, ownerID, groupID, "Test Item 1", 55.9533, -3.1883)
	msgID2 := CreateTestMessage(t, ownerID, groupID, "Test Item 2", 55.9533, -3.1883)

	// Mark both messages as seen via POST
	body := fmt.Sprintf(`{"ids": [%d, %d]}`, msgID1, msgID2)
	req := httptest.NewRequest("POST", "/api/messages/markseen?jwt="+viewerToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, true, result["success"])

	// Verify messages are now marked as seen by checking the user message endpoint
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(ownerID)+"/message?jwt="+viewerToken, nil))
	assert.Equal(t, 200, resp.StatusCode)

	type MessageWithUnseen struct {
		ID     uint64 `json:"id"`
		Unseen bool   `json:"unseen"`
	}

	var msgs []MessageWithUnseen
	json2.Unmarshal(rsp(resp), &msgs)

	for _, m := range msgs {
		if m.ID == msgID1 || m.ID == msgID2 {
			assert.False(t, m.Unseen, "Message %d should be seen after MarkSeen", m.ID)
		}
	}
}

func TestMessagesMarkSeenUnauthorized(t *testing.T) {
	// Test without token - should fail
	body := `{"ids": [1]}`
	req := httptest.NewRequest("POST", "/api/messages/markseen", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestMessagesMarkSeenEmptyIds(t *testing.T) {
	prefix := uniquePrefix("markseen_empty")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Test with empty IDs array
	body := `{"ids": []}`
	req := httptest.NewRequest("POST", "/api/messages/markseen?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestMessagesMarkSeenInvalidBody(t *testing.T) {
	prefix := uniquePrefix("markseen_invalid")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Test with missing IDs field
	body := `{}`
	req := httptest.NewRequest("POST", "/api/messages/markseen?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestMessagesMarkSeenIdempotent(t *testing.T) {
	// Marking the same message as seen twice should succeed (ON DUPLICATE KEY UPDATE)
	prefix := uniquePrefix("markseen_idem")
	groupID := CreateTestGroup(t, prefix)

	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	CreateTestMembership(t, ownerID, groupID, "Member")

	viewerID := CreateTestUser(t, prefix+"_viewer", "User")
	CreateTestMembership(t, viewerID, groupID, "Member")
	_, viewerToken := CreateTestSession(t, viewerID)

	msgID := CreateTestMessage(t, ownerID, groupID, "Test Idempotent", 55.9533, -3.1883)

	body := fmt.Sprintf(`{"ids": [%d]}`, msgID)

	// First mark
	req := httptest.NewRequest("POST", "/api/messages/markseen?jwt="+viewerToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Second mark (should also succeed)
	req = httptest.NewRequest("POST", "/api/messages/markseen?jwt="+viewerToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}
