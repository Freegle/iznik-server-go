package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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
	resp, _ := getApp().Test(req, -1)
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
	resp, err := getApp().Test(req, -1)
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
	resp, err := getApp().Test(req, -1)
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
	resp, err := getApp().Test(req, -1)
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
	resp, err := getApp().Test(req, -1)
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

	// Verify background task was queued (JSON_OBJECT produces "key": value with space after colon).
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
	resp, err := getApp().Test(req, -1)
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

	// Verify background task was queued (JSON_OBJECT produces "key": value with space after colon).
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
	resp, err := getApp().Test(req, -1)
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
	resp, err := getApp().Test(req, -1)
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
	resp, err := getApp().Test(req, -1)
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
	resp, err := getApp().Test(req, -1)
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
	resp, err := getApp().Test(req, -1)
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
	resp, err := getApp().Test(req, -1)
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
	resp, err := getApp().Test(req, -1)
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
	resp, err := getApp().Test(req, -1)
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
	resp, err := getApp().Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestGetMembershipsNotLoggedIn(t *testing.T) {
	url := fmt.Sprintf("/api/memberships?groupid=%d", 1)
	req := httptest.NewRequest("GET", url, nil)
	resp, _ := getApp().Test(req, -1)
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
	resp, err := getApp().Test(req, -1)
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
	resp, err := getApp().Test(req, -1)
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
	resp, err := getApp().Test(req, -1)
	assert.NoError(t, err)
	// Without groupid, GET returns empty list (graceful degradation).
	assert.Equal(t, 200, resp.StatusCode)
}

// --- GET /memberships?collection=Happiness ---

// parseHappinessResponse decodes the happiness response wrapper and returns the members and ratings arrays.
func parseHappinessResponse(t *testing.T, resp *http.Response) ([]map[string]interface{}, []map[string]interface{}) {
	var wrapper map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&wrapper)

	var members []map[string]interface{}
	if membersRaw, ok := wrapper["members"].([]interface{}); ok {
		members = make([]map[string]interface{}, len(membersRaw))
		for i, m := range membersRaw {
			members[i] = m.(map[string]interface{})
		}
	}

	var ratings []map[string]interface{}
	if ratingsRaw, ok := wrapper["ratings"].([]interface{}); ok {
		ratings = make([]map[string]interface{}, len(ratingsRaw))
		for i, r := range ratingsRaw {
			ratings[i] = r.(map[string]interface{})
		}
	}

	return members, ratings
}

// createHappinessOutcome inserts a messages_outcomes row and returns its ID.
func createHappinessOutcome(t *testing.T, msgID uint64, happiness string, comments string) uint64 {
	db := database.DBConn
	result := db.Exec("INSERT INTO messages_outcomes (msgid, outcome, happiness, comments, reviewed) VALUES (?, 'Taken', ?, ?, 0)",
		msgID, happiness, comments)
	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create happiness outcome: %v", result.Error)
	}
	var id uint64
	db.Raw("SELECT id FROM messages_outcomes WHERE msgid = ? ORDER BY id DESC LIMIT 1", msgID).Scan(&id)
	return id
}

func TestGetHappinessBasic(t *testing.T) {
	prefix := uniquePrefix("happy_basic")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	msgID := CreateTestMessage(t, posterID, groupID, prefix+" offer item", 55.95, -3.19)

	outcomeID := createHappinessOutcome(t, msgID, "Happy", "Great experience!")

	url := fmt.Sprintf("/api/memberships?groupid=%d&collection=Happiness&jwt=%s", groupID, token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	results, _ := parseHappinessResponse(t, resp)
	assert.GreaterOrEqual(t, len(results), 1)

	// Find our outcome.
	found := false
	for _, r := range results {
		if uint64(r["id"].(float64)) == outcomeID {
			found = true
			assert.Equal(t, "Happy", r["happiness"])
			assert.Equal(t, "Great experience!", r["comments"])
			assert.Equal(t, float64(0), r["reviewed"])
			assert.Equal(t, float64(groupID), r["groupid"])

			// Check nested user object.
			u, ok := r["user"].(map[string]interface{})
			assert.True(t, ok)
			assert.Equal(t, float64(posterID), u["id"])

			// Check nested message object.
			m, ok := r["message"].(map[string]interface{})
			assert.True(t, ok)
			assert.Equal(t, float64(msgID), m["id"])
			break
		}
	}
	assert.True(t, found, "happiness outcome should be in results")

	// Cleanup.
	db.Exec("DELETE FROM messages_outcomes WHERE id = ?", outcomeID)
}

func TestGetHappinessFilterHappy(t *testing.T) {
	prefix := uniquePrefix("happy_filt")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	msgID1 := CreateTestMessage(t, posterID, groupID, prefix+" happy item", 55.95, -3.19)
	msgID2 := CreateTestMessage(t, posterID, groupID, prefix+" unhappy item", 55.95, -3.19)

	happyID := createHappinessOutcome(t, msgID1, "Happy", "Loved it!")
	unhappyID := createHappinessOutcome(t, msgID2, "Unhappy", "Bad experience")

	// Filter=Happy should only return happy outcomes.
	url := fmt.Sprintf("/api/memberships?groupid=%d&collection=Happiness&filter=Happy&jwt=%s", groupID, token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	results, _ := parseHappinessResponse(t, resp)

	foundHappy := false
	foundUnhappy := false
	for _, r := range results {
		id := uint64(r["id"].(float64))
		if id == happyID {
			foundHappy = true
		}
		if id == unhappyID {
			foundUnhappy = true
		}
	}
	assert.True(t, foundHappy, "happy outcome should be present with Happy filter")
	assert.False(t, foundUnhappy, "unhappy outcome should NOT be present with Happy filter")

	db.Exec("DELETE FROM messages_outcomes WHERE id IN (?, ?)", happyID, unhappyID)
}

func TestGetHappinessFilterUnhappy(t *testing.T) {
	prefix := uniquePrefix("happy_unhy")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	msgID1 := CreateTestMessage(t, posterID, groupID, prefix+" happy item", 55.95, -3.19)
	msgID2 := CreateTestMessage(t, posterID, groupID, prefix+" unhappy item", 55.95, -3.19)

	happyID := createHappinessOutcome(t, msgID1, "Happy", "Good stuff")
	unhappyID := createHappinessOutcome(t, msgID2, "Unhappy", "Terrible")

	url := fmt.Sprintf("/api/memberships?groupid=%d&collection=Happiness&filter=Unhappy&jwt=%s", groupID, token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	results, _ := parseHappinessResponse(t, resp)

	foundHappy := false
	foundUnhappy := false
	for _, r := range results {
		id := uint64(r["id"].(float64))
		if id == happyID {
			foundHappy = true
		}
		if id == unhappyID {
			foundUnhappy = true
		}
	}
	assert.False(t, foundHappy, "happy outcome should NOT be present with Unhappy filter")
	assert.True(t, foundUnhappy, "unhappy outcome should be present with Unhappy filter")

	db.Exec("DELETE FROM messages_outcomes WHERE id IN (?, ?)", happyID, unhappyID)
}

func TestGetHappinessAutoCommentFiltered(t *testing.T) {
	prefix := uniquePrefix("happy_auto")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	msgID := CreateTestMessage(t, posterID, groupID, prefix+" auto item", 55.95, -3.19)

	// Auto-generated comment should be filtered out.
	autoID := createHappinessOutcome(t, msgID, "Happy", "Thanks, this has now been taken.")

	url := fmt.Sprintf("/api/memberships?groupid=%d&collection=Happiness&jwt=%s", groupID, token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	results, _ := parseHappinessResponse(t, resp)

	for _, r := range results {
		id := uint64(r["id"].(float64))
		assert.NotEqual(t, autoID, id, "auto-generated comment should be filtered out")
	}

	db.Exec("DELETE FROM messages_outcomes WHERE id = ?", autoID)
}

func TestGetHappinessAllGroups(t *testing.T) {
	prefix := uniquePrefix("happy_all")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	msgID := CreateTestMessage(t, posterID, groupID, prefix+" all groups item", 55.95, -3.19)

	outcomeID := createHappinessOutcome(t, msgID, "Happy", "All groups test!")

	// groupid=0 should return results across all mod groups.
	url := fmt.Sprintf("/api/memberships?groupid=0&collection=Happiness&jwt=%s", token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	results, _ := parseHappinessResponse(t, resp)

	found := false
	for _, r := range results {
		if uint64(r["id"].(float64)) == outcomeID {
			found = true
			break
		}
	}
	assert.True(t, found, "outcome should be found when querying all groups")

	db.Exec("DELETE FROM messages_outcomes WHERE id = ?", outcomeID)
}

func TestGetHappinessNotMod(t *testing.T) {
	prefix := uniquePrefix("happy_nmod")
	groupID := CreateTestGroup(t, prefix)

	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	url := fmt.Sprintf("/api/memberships?groupid=%d&collection=Happiness&jwt=%s", groupID, token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestGetHappinessNotLoggedIn(t *testing.T) {
	url := "/api/memberships?groupid=1&collection=Happiness"
	req := httptest.NewRequest("GET", url, nil)
	resp, _ := getApp().Test(req, -1)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestGetHappinessOrdering(t *testing.T) {
	prefix := uniquePrefix("happy_ord")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	msgID1 := CreateTestMessage(t, posterID, groupID, prefix+" reviewed item", 55.95, -3.19)
	msgID2 := CreateTestMessage(t, posterID, groupID, prefix+" unreviewed item", 55.95, -3.19)

	reviewedID := createHappinessOutcome(t, msgID1, "Happy", "Reviewed feedback")
	unrevID := createHappinessOutcome(t, msgID2, "Happy", "Unreviewed feedback")

	// Mark one as reviewed.
	db.Exec("UPDATE messages_outcomes SET reviewed = 1 WHERE id = ?", reviewedID)

	url := fmt.Sprintf("/api/memberships?groupid=%d&collection=Happiness&jwt=%s", groupID, token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	results, _ := parseHappinessResponse(t, resp)

	// Find positions of our two outcomes.
	unrevPos := -1
	revPos := -1
	for i, r := range results {
		id := uint64(r["id"].(float64))
		if id == unrevID {
			unrevPos = i
		}
		if id == reviewedID {
			revPos = i
		}
	}

	// Unreviewed should come before reviewed (reviewed ASC).
	if unrevPos >= 0 && revPos >= 0 {
		assert.Less(t, unrevPos, revPos, "unreviewed items should come before reviewed items")
	}

	db.Exec("DELETE FROM messages_outcomes WHERE id IN (?, ?)", reviewedID, unrevID)
}

// --- Ratings in Happiness response ---

func TestGetHappinessRatingsIncluded(t *testing.T) {
	prefix := uniquePrefix("happy_rate")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	raterID := CreateTestUser(t, prefix+"_rater", "User")
	CreateTestMembership(t, raterID, groupID, "Member")

	rateeID := CreateTestUser(t, prefix+"_ratee", "User")
	CreateTestMembership(t, rateeID, groupID, "Member")

	// Insert a rating.
	db.Exec("INSERT INTO ratings (rater, ratee, rating, reason, text, timestamp, reviewrequired) VALUES (?, ?, 'Down', 'NoShow', 'Did not show up', NOW(), 1)",
		raterID, rateeID)

	var ratingID uint64
	db.Raw("SELECT id FROM ratings WHERE rater = ? AND ratee = ? ORDER BY id DESC LIMIT 1", raterID, rateeID).Scan(&ratingID)
	assert.NotZero(t, ratingID)

	url := fmt.Sprintf("/api/memberships?groupid=%d&collection=Happiness&jwt=%s", groupID, token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	_, ratings := parseHappinessResponse(t, resp)

	// Find our rating.
	found := false
	for _, r := range ratings {
		if uint64(r["id"].(float64)) == ratingID {
			found = true
			assert.Equal(t, float64(raterID), r["rater"])
			assert.Equal(t, float64(rateeID), r["ratee"])
			assert.Equal(t, "Down", r["rating"])
			assert.Equal(t, "NoShow", r["reason"])
			assert.Equal(t, "Did not show up", r["text"])
			assert.Equal(t, float64(1), r["reviewrequired"])
			assert.Equal(t, float64(groupID), r["groupid"])
			assert.NotEmpty(t, r["raterdisplayname"])
			assert.NotEmpty(t, r["rateedisplayname"])
			break
		}
	}
	assert.True(t, found, "rating should be in response")

	// Cleanup.
	db.Exec("DELETE FROM ratings WHERE id = ?", ratingID)
}

func TestGetHappinessRatingsRequireSameGroup(t *testing.T) {
	prefix := uniquePrefix("happy_rgrp")
	db := database.DBConn
	groupID1 := CreateTestGroup(t, prefix+"1")
	groupID2 := CreateTestGroup(t, prefix+"2")

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID1, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Rater in group1, ratee in group2 only - should not be visible.
	raterID := CreateTestUser(t, prefix+"_rater", "User")
	CreateTestMembership(t, raterID, groupID1, "Member")

	rateeID := CreateTestUser(t, prefix+"_ratee", "User")
	CreateTestMembership(t, rateeID, groupID2, "Member")

	db.Exec("INSERT INTO ratings (rater, ratee, rating, timestamp) VALUES (?, ?, 'Up', NOW())",
		raterID, rateeID)

	var ratingID uint64
	db.Raw("SELECT id FROM ratings WHERE rater = ? AND ratee = ? ORDER BY id DESC LIMIT 1", raterID, rateeID).Scan(&ratingID)

	url := fmt.Sprintf("/api/memberships?groupid=%d&collection=Happiness&jwt=%s", groupID1, token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	_, ratings := parseHappinessResponse(t, resp)

	// Should not find the rating since rater/ratee are in different groups.
	for _, r := range ratings {
		if uint64(r["id"].(float64)) == ratingID {
			t.Error("rating should not be visible when rater and ratee are in different groups")
		}
	}

	// Cleanup.
	db.Exec("DELETE FROM ratings WHERE id = ?", ratingID)
}

// --- Member filter tests ---

func TestGetMembershipsFilterModerators(t *testing.T) {
	prefix := uniquePrefix("mf_mod")
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	memberID := CreateTestUser(t, prefix+"_member", "User")
	CreateTestMembership(t, memberID, groupID, "Member")

	// Filter=2 (Moderators) should only return the mod, not the regular member.
	url := fmt.Sprintf("/api/memberships?groupid=%d&collection=Approved&filter=2&jwt=%s", groupID, token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var members []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&members)

	// Should have at least the mod but not the regular member.
	found := false
	for _, m := range members {
		uid := uint64(m["userid"].(float64))
		assert.NotEqual(t, memberID, uid, "regular member should not appear with filter=2")
		if uid == modID {
			found = true
		}
	}
	assert.True(t, found, "moderator should appear with filter=2")
}

func TestGetMembershipsFilterBouncing(t *testing.T) {
	prefix := uniquePrefix("mf_bnc")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	bouncingID := CreateTestUser(t, prefix+"_bounce", "User")
	CreateTestMembership(t, bouncingID, groupID, "Member")
	db.Exec("UPDATE users SET bouncing = 1 WHERE id = ?", bouncingID)

	normalID := CreateTestUser(t, prefix+"_normal", "User")
	CreateTestMembership(t, normalID, groupID, "Member")

	// Filter=3 (Bouncing) should only return the bouncing user.
	url := fmt.Sprintf("/api/memberships?groupid=%d&collection=Approved&filter=3&jwt=%s", groupID, token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var members []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&members)

	for _, m := range members {
		uid := uint64(m["userid"].(float64))
		assert.Equal(t, bouncingID, uid, "only bouncing member should appear with filter=3")
	}
}

func TestGetMembershipsFilterBanned(t *testing.T) {
	prefix := uniquePrefix("mf_ban")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	bannedID := CreateTestUser(t, prefix+"_banned", "User")
	// Create a banned membership.
	db.Exec("INSERT INTO memberships (userid, groupid, role, collection) VALUES (?, ?, 'Member', 'Banned')",
		bannedID, groupID)

	// Filter=5 (Banned) should return the banned member.
	url := fmt.Sprintf("/api/memberships?groupid=%d&collection=Approved&filter=5&jwt=%s", groupID, token)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var members []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&members)

	found := false
	for _, m := range members {
		uid := uint64(m["userid"].(float64))
		if uid == bannedID {
			found = true
		}
	}
	assert.True(t, found, "banned member should appear with filter=5")
}
