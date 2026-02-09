package test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestListInvitationsUnauthorized(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/invitation", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(1), result["ret"])
}

func TestListInvitationsEmpty(t *testing.T) {
	prefix := uniquePrefix("ListInvEmpty")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)

	req := httptest.NewRequest("GET", "/api/invitation?jwt="+token, nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	invitations := result["invitations"].([]interface{})
	assert.Len(t, invitations, 0)
}

func TestCreateInvitation(t *testing.T) {
	prefix := uniquePrefix("CreateInv")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)

	db := database.DBConn

	// Give the user some invites.
	db.Exec("UPDATE users SET invitesleft = 5 WHERE id = ?", userID)

	body := fmt.Sprintf(`{"email":"invited-%s@test.com"}`, prefix)
	req := httptest.NewRequest("PUT", "/api/invitation?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify invitation was created.
	var invCount int64
	db.Raw("SELECT COUNT(*) FROM users_invitations WHERE userid = ? AND email = ?",
		userID, fmt.Sprintf("invited-%s@test.com", prefix)).Scan(&invCount)
	assert.Equal(t, int64(1), invCount)

	// Verify invitesleft was decremented.
	var invitesLeft int
	db.Raw("SELECT invitesleft FROM users WHERE id = ?", userID).Scan(&invitesLeft)
	assert.Equal(t, 4, invitesLeft)

	// Verify email task was queued.
	var taskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = 'email_invitation' AND processed_at IS NULL").Scan(&taskCount)
	assert.Greater(t, taskCount, int64(0))
}

func TestCreateInvitationNoQuota(t *testing.T) {
	prefix := uniquePrefix("InvNoQuota")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)

	db := database.DBConn
	db.Exec("UPDATE users SET invitesleft = 0 WHERE id = ?", userID)

	body := fmt.Sprintf(`{"email":"noquota-%s@test.com"}`, prefix)
	req := httptest.NewRequest("PUT", "/api/invitation?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Always returns success even with no quota.
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// But no invitation should have been created.
	var invCount int64
	db.Raw("SELECT COUNT(*) FROM users_invitations WHERE userid = ? AND email = ?",
		userID, fmt.Sprintf("noquota-%s@test.com", prefix)).Scan(&invCount)
	assert.Equal(t, int64(0), invCount)
}

func TestCreateInvitationDuplicate(t *testing.T) {
	prefix := uniquePrefix("InvDup")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)

	db := database.DBConn
	db.Exec("UPDATE users SET invitesleft = 5 WHERE id = ?", userID)

	email := fmt.Sprintf("dup-%s@test.com", prefix)

	// First invitation should succeed.
	body := fmt.Sprintf(`{"email":"%s"}`, email)
	req := httptest.NewRequest("PUT", "/api/invitation?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Second invitation to same email should silently succeed.
	req2 := httptest.NewRequest("PUT", "/api/invitation?jwt="+token, strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := getApp().Test(req2)
	assert.Equal(t, fiber.StatusOK, resp2.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Only one invitation should exist.
	var invCount int64
	db.Raw("SELECT COUNT(*) FROM users_invitations WHERE userid = ? AND email = ?", userID, email).Scan(&invCount)
	assert.Equal(t, int64(1), invCount)
}

func TestCreateInvitationDeclined(t *testing.T) {
	prefix := uniquePrefix("InvDecl")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)

	db := database.DBConn
	db.Exec("UPDATE users SET invitesleft = 5 WHERE id = ?", userID)

	email := fmt.Sprintf("declined-%s@test.com", prefix)

	// Insert a previously declined invitation (from any user).
	db.Exec("INSERT INTO users_invitations (userid, email, outcome) VALUES (?, ?, 'Declined')", userID, email)

	// Trying to invite the same email should silently succeed but not create a new record.
	body := fmt.Sprintf(`{"email":"%s"}`, email)
	req := httptest.NewRequest("PUT", "/api/invitation?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
}

func TestUpdateOutcomeAccepted(t *testing.T) {
	prefix := uniquePrefix("OutcomeAcc")
	userID := CreateTestUser(t, prefix, "Member")

	db := database.DBConn
	db.Exec("UPDATE users SET invitesleft = 3 WHERE id = ?", userID)

	// Create a pending invitation.
	db.Exec("INSERT INTO users_invitations (userid, email) VALUES (?, ?)",
		userID, fmt.Sprintf("outcome-%s@test.com", prefix))
	var inviteID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&inviteID)

	// Accept the invitation (no auth required per PHP).
	body := fmt.Sprintf(`{"id":%d,"outcome":"Accepted"}`, inviteID)
	req := httptest.NewRequest("PATCH", "/api/invitation", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Verify outcome was updated.
	var outcome string
	db.Raw("SELECT outcome FROM users_invitations WHERE id = ?", inviteID).Scan(&outcome)
	assert.Equal(t, "Accepted", outcome)

	// Verify sender got 2 extra invites.
	var invitesLeft int
	db.Raw("SELECT invitesleft FROM users WHERE id = ?", userID).Scan(&invitesLeft)
	assert.Equal(t, 5, invitesLeft) // 3 + 2
}

func TestUpdateOutcomeDeclined(t *testing.T) {
	prefix := uniquePrefix("OutcomeDec")
	userID := CreateTestUser(t, prefix, "Member")

	db := database.DBConn
	db.Exec("UPDATE users SET invitesleft = 3 WHERE id = ?", userID)

	// Create a pending invitation.
	db.Exec("INSERT INTO users_invitations (userid, email) VALUES (?, ?)",
		userID, fmt.Sprintf("declined-outcome-%s@test.com", prefix))
	var inviteID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&inviteID)

	body := fmt.Sprintf(`{"id":%d,"outcome":"Declined"}`, inviteID)
	req := httptest.NewRequest("PATCH", "/api/invitation", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Verify outcome was updated.
	var outcome string
	db.Raw("SELECT outcome FROM users_invitations WHERE id = ?", inviteID).Scan(&outcome)
	assert.Equal(t, "Declined", outcome)

	// Sender should NOT get extra invites for declined.
	var invitesLeft int
	db.Raw("SELECT invitesleft FROM users WHERE id = ?", userID).Scan(&invitesLeft)
	assert.Equal(t, 3, invitesLeft) // unchanged
}

func TestUpdateOutcomeAlreadyAccepted(t *testing.T) {
	prefix := uniquePrefix("OutcomeAlready")
	userID := CreateTestUser(t, prefix, "Member")

	db := database.DBConn
	db.Exec("UPDATE users SET invitesleft = 3 WHERE id = ?", userID)

	// Create an already accepted invitation.
	db.Exec("INSERT INTO users_invitations (userid, email, outcome, outcometimestamp) VALUES (?, ?, 'Accepted', NOW())",
		userID, fmt.Sprintf("already-%s@test.com", prefix))
	var inviteID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&inviteID)

	// Try to decline it - should do nothing since it's not Pending.
	body := fmt.Sprintf(`{"id":%d,"outcome":"Declined"}`, inviteID)
	req := httptest.NewRequest("PATCH", "/api/invitation", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Outcome should still be Accepted.
	var outcome string
	db.Raw("SELECT outcome FROM users_invitations WHERE id = ?", inviteID).Scan(&outcome)
	assert.Equal(t, "Accepted", outcome)
}

func TestListInvitationsWithData(t *testing.T) {
	prefix := uniquePrefix("ListInvData")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)

	db := database.DBConn
	db.Exec("INSERT INTO users_invitations (userid, email) VALUES (?, ?)",
		userID, fmt.Sprintf("list-%s@test.com", prefix))

	req := httptest.NewRequest("GET", "/api/invitation?jwt="+token, nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	invitations := result["invitations"].([]interface{})
	assert.Len(t, invitations, 1)

	inv := invitations[0].(map[string]interface{})
	assert.Equal(t, fmt.Sprintf("list-%s@test.com", prefix), inv["email"])
	assert.Equal(t, "Pending", inv["outcome"])
}

func TestCreateInvitationUnauthorized(t *testing.T) {
	body := `{"email":"test@test.com"}`
	req := httptest.NewRequest("PUT", "/api/invitation", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(1), result["ret"])
}
