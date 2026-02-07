package test

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Tests for GET /api/systemlogs - Auth & Validation
// =============================================================================

func TestSystemLogs_Unauthorized(t *testing.T) {
	// No authentication.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/systemlogs", nil))
	assert.Equal(t, 401, resp.StatusCode)
}

func TestSystemLogs_ForbiddenForRegularUser(t *testing.T) {
	prefix := uniquePrefix("syslogs_reg")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Regular user without moderator role should be forbidden.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/systemlogs?jwt="+token, nil))
	assert.Equal(t, 403, resp.StatusCode)
}

func TestSystemLogs_ModeratorAccess(t *testing.T) {
	prefix := uniquePrefix("syslogs_mod")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Moderator")
	_, token := CreateTestSession(t, userID)

	// Moderator should get through auth. The actual query will likely fail
	// because Loki isn't available in test, but we should get past auth (not 401/403).
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/systemlogs?jwt="+token, nil))
	// Accept 200 (if Loki is available) or 500 (if Loki is not available).
	assert.NotEqual(t, 401, resp.StatusCode, "Moderator should not get 401")
	assert.NotEqual(t, 403, resp.StatusCode, "Moderator should not get 403")
}

func TestSystemLogs_SupportAccess(t *testing.T) {
	prefix := uniquePrefix("syslogs_sup")
	userID := CreateTestUser(t, prefix, "Support")
	_, token := CreateTestSession(t, userID)

	// Support user should get through auth.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/systemlogs?jwt="+token, nil))
	assert.NotEqual(t, 401, resp.StatusCode, "Support should not get 401")
	assert.NotEqual(t, 403, resp.StatusCode, "Support should not get 403")
}

func TestSystemLogs_AdminAccess(t *testing.T) {
	prefix := uniquePrefix("syslogs_admin")
	userID := CreateTestUser(t, prefix, "Admin")
	_, token := CreateTestSession(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/systemlogs?jwt="+token, nil))
	assert.NotEqual(t, 401, resp.StatusCode, "Admin should not get 401")
	assert.NotEqual(t, 403, resp.StatusCode, "Admin should not get 403")
}

func TestSystemLogs_ModeratorCannotViewUnrelatedUser(t *testing.T) {
	prefix := uniquePrefix("syslogs_moduser")
	groupID := CreateTestGroup(t, prefix)
	modUserID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modUserID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modUserID)

	// Create a user who is NOT in the moderator's group.
	otherUserID := CreateTestUser(t, prefix+"_other", "User")

	// Moderator should not be able to view logs for a user not in their groups.
	resp, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/systemlogs?jwt=%s&userid=%d", modToken, otherUserID), nil))
	assert.Equal(t, 403, resp.StatusCode)
}

func TestSystemLogs_ModeratorCanViewGroupMember(t *testing.T) {
	prefix := uniquePrefix("syslogs_modmem")
	groupID := CreateTestGroup(t, prefix)
	modUserID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modUserID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modUserID)

	// Create a user who IS in the moderator's group.
	memberUserID := CreateTestUser(t, prefix+"_member", "User")
	CreateTestMembership(t, memberUserID, groupID, "Member")

	// Moderator should be able to view logs for users in their groups.
	resp, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/systemlogs?jwt=%s&userid=%d", modToken, memberUserID), nil))
	// Should not be 403 - auth passed. Could be 200 or 500 (Loki unavailable).
	assert.NotEqual(t, 403, resp.StatusCode, "Moderator should be able to view group member logs")
}

func TestSystemLogs_SupportBypassesGroupCheck(t *testing.T) {
	prefix := uniquePrefix("syslogs_supbypass")
	supportUserID := CreateTestUser(t, prefix+"_sup", "Support")
	_, supToken := CreateTestSession(t, supportUserID)

	// Create any user.
	otherUserID := CreateTestUser(t, prefix+"_other", "User")

	// Support should bypass group membership checks.
	resp, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/systemlogs?jwt=%s&userid=%d", supToken, otherUserID), nil))
	assert.NotEqual(t, 403, resp.StatusCode, "Support should bypass group check")
}

// =============================================================================
// Tests for GET /api/systemlogs/counts - Auth & Validation
// =============================================================================

func TestSystemLogsCounts_Unauthorized(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/systemlogs/counts", nil))
	assert.Equal(t, 401, resp.StatusCode)
}

func TestSystemLogsCounts_ForbiddenForRegularUser(t *testing.T) {
	prefix := uniquePrefix("syslogscnt_reg")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/systemlogs/counts?jwt="+token, nil))
	assert.Equal(t, 403, resp.StatusCode)
}

func TestSystemLogsCounts_MissingSources(t *testing.T) {
	prefix := uniquePrefix("syslogscnt_nosrc")
	userID := CreateTestUser(t, prefix, "Support")
	_, token := CreateTestSession(t, userID)

	// Missing required 'sources' parameter should return 400.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/systemlogs/counts?jwt="+token, nil))
	assert.Equal(t, 400, resp.StatusCode)
}

func TestSystemLogsCounts_WithSources(t *testing.T) {
	prefix := uniquePrefix("syslogscnt_src")
	userID := CreateTestUser(t, prefix, "Support")
	_, token := CreateTestSession(t, userID)

	// With sources parameter should get past validation.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/systemlogs/counts?jwt="+token+"&sources=api", nil))
	// Should not be 400 (validation passed) or 401/403 (auth passed).
	assert.NotEqual(t, 400, resp.StatusCode, "Should not get validation error with sources")
	assert.NotEqual(t, 401, resp.StatusCode)
	assert.NotEqual(t, 403, resp.StatusCode)
}

func TestSystemLogsCounts_ModeratorAccess(t *testing.T) {
	prefix := uniquePrefix("syslogscnt_mod")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Moderator")
	_, token := CreateTestSession(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/systemlogs/counts?jwt="+token+"&sources=api", nil))
	assert.NotEqual(t, 401, resp.StatusCode)
	assert.NotEqual(t, 403, resp.StatusCode)
}
