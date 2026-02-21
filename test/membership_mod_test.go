package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

// createPendingMember inserts a membership with collection='Pending' for testing.
func createPendingMember(t *testing.T, userID uint64, groupID uint64) {
	db := database.DBConn
	result := db.Exec("INSERT INTO memberships (userid, groupid, role, collection) VALUES (?, ?, 'Member', 'Pending')",
		userID, groupID)
	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create pending membership: %v", result.Error)
	}
}

// createBannedMember inserts a membership with collection='Banned' for testing.
func createBannedMember(t *testing.T, userID uint64, groupID uint64) {
	db := database.DBConn
	result := db.Exec("INSERT INTO memberships (userid, groupid, role, collection) VALUES (?, ?, 'Member', 'Banned')",
		userID, groupID)
	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create banned membership: %v", result.Error)
	}
}

// --- POST /memberships (mod actions) ---

func TestPostMembershipsNotLoggedIn(t *testing.T) {
	body := map[string]interface{}{
		"userid":  1,
		"groupid": 1,
		"action":  "Hold",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/memberships", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestPostMembershipsNotMod(t *testing.T) {
	prefix := uniquePrefix("mod_notmod")
	groupID := CreateTestGroup(t, prefix)
	// Regular member, not a mod.
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	targetID := CreateTestUser(t, prefix+"_target", "User")
	createPendingMember(t, targetID, groupID)

	body := map[string]interface{}{
		"userid":  targetID,
		"groupid": groupID,
		"action":  "Hold",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestPostMembershipsHold(t *testing.T) {
	prefix := uniquePrefix("mod_hold")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	// Create mod user.
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create target pending member.
	targetID := CreateTestUser(t, prefix+"_target", "User")
	createPendingMember(t, targetID, groupID)

	body := map[string]interface{}{
		"userid":  targetID,
		"groupid": groupID,
		"action":  "Hold",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify heldby is set in DB.
	var heldby uint64
	db.Raw("SELECT COALESCE(heldby, 0) FROM memberships WHERE userid = ? AND groupid = ?",
		targetID, groupID).Scan(&heldby)
	assert.Equal(t, modID, heldby)
}

func TestPostMembershipsRelease(t *testing.T) {
	prefix := uniquePrefix("mod_release")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	targetID := CreateTestUser(t, prefix+"_target", "User")
	createPendingMember(t, targetID, groupID)

	// Hold first.
	db.Exec("UPDATE memberships SET heldby = ? WHERE userid = ? AND groupid = ?",
		modID, targetID, groupID)

	body := map[string]interface{}{
		"userid":  targetID,
		"groupid": groupID,
		"action":  "Release",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify heldby is NULL.
	var heldby uint64
	db.Raw("SELECT COALESCE(heldby, 0) FROM memberships WHERE userid = ? AND groupid = ?",
		targetID, groupID).Scan(&heldby)
	assert.Equal(t, uint64(0), heldby)
}

func TestPostMembershipsApprove(t *testing.T) {
	prefix := uniquePrefix("mod_approve")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	targetID := CreateTestUser(t, prefix+"_target", "User")
	createPendingMember(t, targetID, groupID)

	body := map[string]interface{}{
		"userid":  targetID,
		"groupid": groupID,
		"action":  "Approve",
		"subject": "Welcome!",
		"body":    "Welcome to the group.",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify collection changed to Approved.
	var collection string
	db.Raw("SELECT collection FROM memberships WHERE userid = ? AND groupid = ?",
		targetID, groupID).Scan(&collection)
	assert.Equal(t, "Approved", collection)

	// Verify heldby is NULL.
	var heldby uint64
	db.Raw("SELECT COALESCE(heldby, 0) FROM memberships WHERE userid = ? AND groupid = ?",
		targetID, groupID).Scan(&heldby)
	assert.Equal(t, uint64(0), heldby)

	// Verify background task was queued.
	var taskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = 'email_membership_approved' AND data LIKE ?",
		fmt.Sprintf("%%\"userid\": %d%%", targetID)).Scan(&taskCount)
	assert.Greater(t, taskCount, int64(0))
}

func TestPostMembershipsReject(t *testing.T) {
	prefix := uniquePrefix("mod_reject")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	targetID := CreateTestUser(t, prefix+"_target", "User")
	createPendingMember(t, targetID, groupID)

	body := map[string]interface{}{
		"userid":  targetID,
		"groupid": groupID,
		"action":  "Reject",
		"subject": "Sorry",
		"body":    "Your request was rejected.",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify membership deleted.
	var count int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ? AND collection IN ('Pending', 'Approved')",
		targetID, groupID).Scan(&count)
	assert.Equal(t, int64(0), count)

	// Verify background task was queued.
	var taskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = 'email_membership_rejected' AND data LIKE ?",
		fmt.Sprintf("%%\"userid\": %d%%", targetID)).Scan(&taskCount)
	assert.Greater(t, taskCount, int64(0))
}

func TestPostMembershipsBan(t *testing.T) {
	prefix := uniquePrefix("mod_ban")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, targetID, groupID, "Member")

	body := map[string]interface{}{
		"userid":  targetID,
		"groupid": groupID,
		"action":  "Ban",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify no Approved membership.
	var approvedCount int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Approved'",
		targetID, groupID).Scan(&approvedCount)
	assert.Equal(t, int64(0), approvedCount)

	// Verify Banned record exists.
	var bannedCount int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Banned'",
		targetID, groupID).Scan(&bannedCount)
	assert.Equal(t, int64(1), bannedCount)
}

func TestPostMembershipsUnban(t *testing.T) {
	prefix := uniquePrefix("mod_unban")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	targetID := CreateTestUser(t, prefix+"_target", "User")
	createBannedMember(t, targetID, groupID)

	body := map[string]interface{}{
		"userid":  targetID,
		"groupid": groupID,
		"action":  "Unban",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify banned record removed.
	var bannedCount int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Banned'",
		targetID, groupID).Scan(&bannedCount)
	assert.Equal(t, int64(0), bannedCount)
}

func TestPostMembershipsReviewHold(t *testing.T) {
	prefix := uniquePrefix("mod_rvhold")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, targetID, groupID, "Member")

	body := map[string]interface{}{
		"userid":  targetID,
		"groupid": groupID,
		"action":  "ReviewHold",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// ReviewHold sets heldby on the membership (same column as Hold, different context).
	var heldby uint64
	db.Raw("SELECT COALESCE(heldby, 0) FROM memberships WHERE userid = ? AND groupid = ?",
		targetID, groupID).Scan(&heldby)
	assert.Equal(t, modID, heldby)
}

func TestPostMembershipsReviewRelease(t *testing.T) {
	prefix := uniquePrefix("mod_rvrel")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, targetID, groupID, "Member")

	// Set up as held first (simulating a ReviewHold).
	db.Exec("UPDATE memberships SET heldby = ? WHERE userid = ? AND groupid = ?",
		modID, targetID, groupID)

	body := map[string]interface{}{
		"userid":  targetID,
		"groupid": groupID,
		"action":  "ReviewRelease",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// ReviewRelease clears heldby on the membership.
	var heldby uint64
	db.Raw("SELECT COALESCE(heldby, 0) FROM memberships WHERE userid = ? AND groupid = ?",
		targetID, groupID).Scan(&heldby)
	assert.Equal(t, uint64(0), heldby)
}

func TestPostMembershipsHappinessReviewed(t *testing.T) {
	prefix := uniquePrefix("mod_happy")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, targetID, groupID, "Member")

	// Create a message and outcome to review.
	msgID := CreateTestMessage(t, targetID, groupID, prefix+" item", 55.9533, -3.1883)
	db.Exec("INSERT INTO messages_outcomes (msgid, outcome, happiness) VALUES (?, 'Taken', 'Happy')",
		msgID)

	var outcomeID uint64
	db.Raw("SELECT id FROM messages_outcomes WHERE msgid = ? ORDER BY id DESC LIMIT 1", msgID).Scan(&outcomeID)
	assert.NotZero(t, outcomeID)

	happinessStr := fmt.Sprintf("%d", outcomeID)
	body := map[string]interface{}{
		"userid":    targetID,
		"groupid":   groupID,
		"action":    "HappinessReviewed",
		"happiness": happinessStr,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify outcome was marked as reviewed.
	var reviewed int
	db.Raw("SELECT reviewed FROM messages_outcomes WHERE id = ?", outcomeID).Scan(&reviewed)
	assert.Equal(t, 1, reviewed)
}

func TestPostMembershipsUnknownAction(t *testing.T) {
	prefix := uniquePrefix("mod_unkact")
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, targetID, groupID, "Member")

	body := map[string]interface{}{
		"userid":  targetID,
		"groupid": groupID,
		"action":  "BogusAction",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostMembershipsAdminBypass(t *testing.T) {
	// Admin users should be able to perform mod actions on any group.
	prefix := uniquePrefix("mod_admin")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	// Create admin user (not a member of the group).
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	targetID := CreateTestUser(t, prefix+"_target", "User")
	createPendingMember(t, targetID, groupID)

	body := map[string]interface{}{
		"userid":  targetID,
		"groupid": groupID,
		"action":  "Hold",
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify heldby set.
	var heldby uint64
	db.Raw("SELECT COALESCE(heldby, 0) FROM memberships WHERE userid = ? AND groupid = ?",
		targetID, groupID).Scan(&heldby)
	assert.Equal(t, adminID, heldby)
}

// --- GET /memberships (mod list) ---

func TestGetMemberships(t *testing.T) {
	prefix := uniquePrefix("mod_getmem")
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create a couple of regular members.
	member1ID := CreateTestUser(t, prefix+"_m1", "User")
	CreateTestMembership(t, member1ID, groupID, "Member")
	member2ID := CreateTestUser(t, prefix+"_m2", "User")
	CreateTestMembership(t, member2ID, groupID, "Member")

	url := fmt.Sprintf("/api/memberships?groupid=%d&jwt=%s", groupID, token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var members []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&members)
	// Should have at least 3 members (mod + 2 regular members).
	assert.GreaterOrEqual(t, len(members), 3)

	// Check that member IDs are present.
	foundMember1 := false
	foundMember2 := false
	for _, m := range members {
		uid := uint64(m["userid"].(float64))
		if uid == member1ID {
			foundMember1 = true
		}
		if uid == member2ID {
			foundMember2 = true
		}
	}
	assert.True(t, foundMember1, "member1 should be in the list")
	assert.True(t, foundMember2, "member2 should be in the list")
}

func TestGetMembershipsNotMod(t *testing.T) {
	prefix := uniquePrefix("mod_getnmod")
	groupID := CreateTestGroup(t, prefix)

	// Regular member, not mod.
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	url := fmt.Sprintf("/api/memberships?groupid=%d&jwt=%s", groupID, token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestGetMembershipsNotLoggedIn(t *testing.T) {
	url := fmt.Sprintf("/api/memberships?groupid=%d", 1)
	req := httptest.NewRequest("GET", url, nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestGetMembershipsSearch(t *testing.T) {
	prefix := uniquePrefix("mod_search")
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create a member with a distinct name.
	targetID := CreateTestUser(t, prefix+"_findme", "User")
	CreateTestMembership(t, targetID, groupID, "Member")

	// Search by the unique part of the name.
	url := fmt.Sprintf("/api/memberships?groupid=%d&search=%s&jwt=%s", groupID, prefix+"_findme", token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var members []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&members)
	assert.GreaterOrEqual(t, len(members), 1)

	// The target should be in the results.
	found := false
	for _, m := range members {
		uid := uint64(m["userid"].(float64))
		if uid == targetID {
			found = true
			break
		}
	}
	assert.True(t, found, "searched member should be in results")
}

func TestGetMembershipsPendingCollection(t *testing.T) {
	prefix := uniquePrefix("mod_getpend")
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create a pending member.
	targetID := CreateTestUser(t, prefix+"_pend", "User")
	createPendingMember(t, targetID, groupID)

	url := fmt.Sprintf("/api/memberships?groupid=%d&collection=Pending&jwt=%s", groupID, token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var members []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&members)
	assert.GreaterOrEqual(t, len(members), 1)

	found := false
	for _, m := range members {
		uid := uint64(m["userid"].(float64))
		if uid == targetID {
			found = true
			assert.Equal(t, "Pending", m["collection"])
			break
		}
	}
	assert.True(t, found, "pending member should be in results")
}

func TestGetMembershipsMissingGroupid(t *testing.T) {
	prefix := uniquePrefix("mod_nogrp")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	_, token := CreateTestSession(t, modID)

	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	// Without groupid, GET should return 400.
	assert.Equal(t, 400, resp.StatusCode)
}
