package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestGetDonations(t *testing.T) {
	// Test without groupid - should return default target and current month's raised amount
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/donations", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	// Should have target and raised fields
	assert.Contains(t, result, "target")
	assert.Contains(t, result, "raised")

	// Target should be the default (2000 unless DONATION_TARGET env var is set)
	target, ok := result["target"].(float64)
	assert.True(t, ok, "target should be a number")
	assert.Greater(t, target, float64(0), "target should be positive")

	// Raised should be a non-negative number
	raised, ok := result["raised"].(float64)
	assert.True(t, ok, "raised should be a number")
	assert.GreaterOrEqual(t, raised, float64(0), "raised should be >= 0")
}

func TestGetDonationsWithGroupID(t *testing.T) {
	// Test with a valid group ID
	// Note: This will return 0 for raised if no donations exist for this group
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/donations?groupid=1", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	assert.Contains(t, result, "target")
	assert.Contains(t, result, "raised")

	// Raised should be a valid number
	raised, ok := result["raised"].(float64)
	assert.True(t, ok, "raised should be a number")
	assert.GreaterOrEqual(t, raised, float64(0), "raised should be >= 0")
}

func TestGetDonationsInvalidGroupID(t *testing.T) {
	// Test with non-existent group ID - should fall back to default target
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/donations?groupid=999999", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	assert.Contains(t, result, "target")
	assert.Contains(t, result, "raised")

	// Should fall back to default target when group not found
	target, ok := result["target"].(float64)
	assert.True(t, ok, "target should be a number")
	assert.Greater(t, target, float64(0), "target should be positive (falls back to default)")

	// Raised should be 0 for non-existent group
	assert.Equal(t, float64(0), result["raised"])
}

func TestAddDonationExternal(t *testing.T) {
	prefix := uniquePrefix("AddDonation")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)
	db := database.DBConn

	// Give the calling user GiftAid permission.
	db.Exec("UPDATE users SET permissions = 'GiftAid' WHERE id = ?", userID)

	// Create the target user who receives the donation.
	targetUserID := CreateTestUser(t, prefix+"Target", "Member")

	body := fmt.Sprintf(`{"userid":%d,"amount":10.00,"date":"2026-01-15 12:00:00"}`, targetUserID)
	req := httptest.NewRequest("PUT", "/api/donations?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	respBody := rsp(resp)
	var result map[string]interface{}
	json2.Unmarshal(respBody, &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.NotZero(t, result["id"])

	// Verify donation was inserted.
	var donationType string
	db.Raw("SELECT type FROM users_donations WHERE id = ?", uint64(result["id"].(float64))).Scan(&donationType)
	assert.Equal(t, "External", donationType)

	// Verify GiftAid notification was created for the target user.
	var notifCount int64
	db.Raw("SELECT COUNT(*) FROM users_notifications WHERE touser = ? AND type = 'GiftAid'", targetUserID).Scan(&notifCount)
	assert.Greater(t, notifCount, int64(0))

	// Verify email task was queued.
	var taskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = 'email_donate_external' AND processed_at IS NULL").Scan(&taskCount)
	assert.Greater(t, taskCount, int64(0))
}

func TestAddDonationZeroAmount(t *testing.T) {
	prefix := uniquePrefix("AddDonZero")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)

	// No GiftAid permission needed for zero amount.
	targetUserID := CreateTestUser(t, prefix+"Target", "Member")

	body := fmt.Sprintf(`{"userid":%d,"amount":0,"date":"2026-01-15 12:00:00"}`, targetUserID)
	req := httptest.NewRequest("PUT", "/api/donations?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	respBody := rsp(resp)
	var result map[string]interface{}
	json2.Unmarshal(respBody, &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.NotZero(t, result["id"])

	// Verify no GiftAid notification for zero amount.
	db := database.DBConn
	var notifCount int64
	db.Raw("SELECT COUNT(*) FROM users_notifications WHERE touser = ? AND type = 'GiftAid'", targetUserID).Scan(&notifCount)
	assert.Equal(t, int64(0), notifCount)
}

func TestAddDonationNoPermission(t *testing.T) {
	prefix := uniquePrefix("AddDonNoPerm")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)

	targetUserID := CreateTestUser(t, prefix+"Target", "Member")

	// Non-zero amount without GiftAid permission should be denied.
	body := fmt.Sprintf(`{"userid":%d,"amount":25.00,"date":"2026-01-15 12:00:00"}`, targetUserID)
	req := httptest.NewRequest("PUT", "/api/donations?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestAddDonationUnauthorized(t *testing.T) {
	body := `{"userid":1,"amount":10.00,"date":"2026-01-15 12:00:00"}`
	req := httptest.NewRequest("PUT", "/api/donations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestAddDonationMissingUserID(t *testing.T) {
	prefix := uniquePrefix("AddDonNoUID")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)

	body := `{"amount":10.00,"date":"2026-01-15 12:00:00"}`
	req := httptest.NewRequest("PUT", "/api/donations?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestAddDonationInvalidUserID(t *testing.T) {
	prefix := uniquePrefix("AddDonBadUID")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)
	db := database.DBConn

	db.Exec("UPDATE users SET permissions = 'GiftAid' WHERE id = ?", userID)

	body := `{"userid":999999999,"amount":10.00,"date":"2026-01-15 12:00:00"}`
	req := httptest.NewRequest("PUT", "/api/donations?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestAddDonationSkipsGiftAidNotifWhenExisting(t *testing.T) {
	prefix := uniquePrefix("AddDonGAExist")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)
	db := database.DBConn

	db.Exec("UPDATE users SET permissions = 'GiftAid' WHERE id = ?", userID)

	targetUserID := CreateTestUser(t, prefix+"Target", "Member")

	// Create a pre-existing giftaid record with period != 'This'.
	db.Exec("INSERT INTO giftaid (userid, period, fullname, homeaddress) VALUES (?, 'Declined', 'Test', 'Test')", targetUserID)

	body := fmt.Sprintf(`{"userid":%d,"amount":15.00,"date":"2026-01-15 12:00:00"}`, targetUserID)
	req := httptest.NewRequest("PUT", "/api/donations?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Should NOT create a GiftAid notification since giftaid record exists with period != 'This'.
	var notifCount int64
	db.Raw("SELECT COUNT(*) FROM users_notifications WHERE touser = ? AND type = 'GiftAid'", targetUserID).Scan(&notifCount)
	assert.Equal(t, int64(0), notifCount)
}
