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

func TestPutMembershipsNotLoggedIn(t *testing.T) {
	body := map[string]interface{}{"groupid": 1}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/memberships", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestPutMembershipsJoinGroup(t *testing.T) {
	prefix := uniquePrefix("mem_join")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)

	// Join the group.
	body := map[string]interface{}{
		"userid":  userID,
		"groupid": groupID,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("PUT", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Approved", result["addedto"])

	// Verify membership exists in DB.
	var count int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Approved'",
		userID, groupID).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestPutMembershipsAlreadyMember(t *testing.T) {
	prefix := uniquePrefix("mem_already")

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")

	// Join again - should succeed (idempotent).
	body := map[string]interface{}{
		"userid":  userID,
		"groupid": groupID,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("PUT", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestPutMembershipsGroupNotFound(t *testing.T) {
	prefix := uniquePrefix("mem_nogrp")

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)

	body := map[string]interface{}{
		"userid":  userID,
		"groupid": 999999999,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("PUT", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestPutMembershipsCannotAddOther(t *testing.T) {
	prefix := uniquePrefix("mem_other")

	userID := CreateTestUser(t, prefix+"_user", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)

	body := map[string]interface{}{
		"userid":  otherID,
		"groupid": groupID,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("PUT", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestDeleteMembershipsNotLoggedIn(t *testing.T) {
	body := map[string]interface{}{"groupid": 1}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("DELETE", "/api/memberships", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestDeleteMembershipsLeaveGroup(t *testing.T) {
	prefix := uniquePrefix("mem_leave")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")

	// Leave the group.
	body := map[string]interface{}{
		"userid":  userID,
		"groupid": groupID,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("DELETE", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify membership is gone.
	var count int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Approved'",
		userID, groupID).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestDeleteMembershipsNotMember(t *testing.T) {
	prefix := uniquePrefix("mem_notmem")

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)

	// Leave a group we're not in - should succeed (idempotent).
	body := map[string]interface{}{
		"userid":  userID,
		"groupid": groupID,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("DELETE", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestDeleteMembershipsCannotRemoveOther(t *testing.T) {
	prefix := uniquePrefix("mem_rmoth")

	userID := CreateTestUser(t, prefix+"_user", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, otherID, groupID, "Member")

	body := map[string]interface{}{
		"userid":  otherID,
		"groupid": groupID,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("DELETE", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestPatchMembershipsNotLoggedIn(t *testing.T) {
	body := map[string]interface{}{"groupid": 1}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PATCH", "/api/memberships", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestPatchMembershipsEmailFrequency(t *testing.T) {
	prefix := uniquePrefix("mem_ef")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")

	// Update email frequency to 0 (never).
	body := map[string]interface{}{
		"userid":         userID,
		"groupid":        groupID,
		"emailfrequency": 0,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("PATCH", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify in DB.
	var ef int
	db.Raw("SELECT emailfrequency FROM memberships WHERE userid = ? AND groupid = ?",
		userID, groupID).Scan(&ef)
	assert.Equal(t, 0, ef)

	// Update back to 24.
	body["emailfrequency"] = 24
	bodyBytes, _ = json.Marshal(body)
	req = httptest.NewRequest("PATCH", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err = getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	db.Raw("SELECT emailfrequency FROM memberships WHERE userid = ? AND groupid = ?",
		userID, groupID).Scan(&ef)
	assert.Equal(t, 24, ef)
}

func TestPatchMembershipsEventsAllowed(t *testing.T) {
	prefix := uniquePrefix("mem_ev")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")

	// Disable events.
	body := map[string]interface{}{
		"userid":        userID,
		"groupid":       groupID,
		"eventsallowed": 0,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("PATCH", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var ea int
	db.Raw("SELECT eventsallowed FROM memberships WHERE userid = ? AND groupid = ?",
		userID, groupID).Scan(&ea)
	assert.Equal(t, 0, ea)
}

func TestPatchMembershipsVolunteeringAllowed(t *testing.T) {
	prefix := uniquePrefix("mem_vol")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")

	// Disable volunteering.
	body := map[string]interface{}{
		"userid":              userID,
		"groupid":             groupID,
		"volunteeringallowed": 0,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("PATCH", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var va int
	db.Raw("SELECT volunteeringallowed FROM memberships WHERE userid = ? AND groupid = ?",
		userID, groupID).Scan(&va)
	assert.Equal(t, 0, va)
}

func TestPatchMembershipsNotMember(t *testing.T) {
	prefix := uniquePrefix("mem_pnm")

	userID := CreateTestUser(t, prefix+"_user", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)

	body := map[string]interface{}{
		"userid":         userID,
		"groupid":        groupID,
		"emailfrequency": 0,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("PATCH", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestPatchMembershipsCannotModifyOther(t *testing.T) {
	prefix := uniquePrefix("mem_poth")

	userID := CreateTestUser(t, prefix+"_user", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, otherID, groupID, "Member")

	body := map[string]interface{}{
		"userid":         otherID,
		"groupid":        groupID,
		"emailfrequency": 0,
	}
	bodyBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/memberships?jwt=%s", token)
	req := httptest.NewRequest("PATCH", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}
