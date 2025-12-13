package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/notification"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestNotifications(t *testing.T) {
	prefix := uniquePrefix("notif")
	_, token := CreateFullTestUser(t, prefix)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/notification/count?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	type Count struct {
		Count uint64
	}

	var count Count

	json2.Unmarshal(rsp(resp), &count)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/notification?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var notifications []notification.Notification

	json2.Unmarshal(rsp(resp), &notifications)
	assert.GreaterOrEqual(t, uint64(len(notifications)), count.Count)
}

func TestNotificationSeen(t *testing.T) {
	prefix := uniquePrefix("notif_seen")

	// Create user and get token
	userID := CreateTestUser(t, prefix, "User")
	fromUserID := CreateTestUser(t, prefix+"_from", "User")
	_, token := CreateTestSession(t, userID)

	// Create a notification
	notifID := CreateTestNotification(t, userID, fromUserID, "Comment")

	// Mark it as seen
	body := fmt.Sprintf(`{"id": %d}`, notifID)
	req := httptest.NewRequest("POST", "/api/notification/seen?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, true, result["success"])
}

func TestNotificationSeenUnauthorized(t *testing.T) {
	// Test without token - should fail
	body := `{"id": 1}`
	req := httptest.NewRequest("POST", "/api/notification/seen", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestNotificationSeenInvalidBody(t *testing.T) {
	prefix := uniquePrefix("notif_invalid")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Test with missing ID
	body := `{}`
	req := httptest.NewRequest("POST", "/api/notification/seen?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestNotificationAllSeen(t *testing.T) {
	prefix := uniquePrefix("notif_allseen")

	// Create user and get token
	userID := CreateTestUser(t, prefix, "User")
	fromUserID := CreateTestUser(t, prefix+"_from", "User")
	_, token := CreateTestSession(t, userID)

	// Create multiple notifications
	CreateTestNotification(t, userID, fromUserID, "Comment")
	CreateTestNotification(t, userID, fromUserID, "Loved")

	// Mark all as seen
	req := httptest.NewRequest("POST", "/api/notification/allseen?jwt="+token, nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, true, result["success"])

	// Verify count is now 0
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/notification/count?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	type Count struct {
		Count uint64
	}
	var count Count
	json2.Unmarshal(rsp(resp), &count)
	assert.Equal(t, uint64(0), count.Count)
}

func TestNotificationAllSeenUnauthorized(t *testing.T) {
	// Test without token - should fail
	req := httptest.NewRequest("POST", "/api/notification/allseen", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}
