package test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	user2 "github.com/freegle/iznik-server-go/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Tests for GET /api/user/search
// =============================================================================

func TestSearchUsers_ByName(t *testing.T) {
	prefix := uniquePrefix("searchname")
	db := database.DBConn

	// Create an admin user.
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	// Create a target user with a known fullname.
	targetName := "SearchTarget_" + prefix
	targetID := CreateTestUser(t, prefix+"_target", "User")
	// Update fullname to something searchable.
	db.Exec("UPDATE users SET fullname = ? WHERE id = ?", targetName, targetID)

	// Search by name.
	url := fmt.Sprintf("/api/user/search?q=%s&jwt=%s", targetName, adminToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	users, ok := result["users"].([]interface{})
	assert.True(t, ok)
	assert.GreaterOrEqual(t, len(users), 1, "Should find at least one user")

	// Verify the target user is in the results.
	found := false
	for _, u := range users {
		userMap := u.(map[string]interface{})
		if uint64(userMap["id"].(float64)) == targetID {
			found = true
			break
		}
	}
	assert.True(t, found, "Target user should be in search results")
}

func TestSearchUsers_ByEmail(t *testing.T) {
	prefix := uniquePrefix("searchemail")
	db := database.DBConn

	// Create an admin user.
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	// Create a target user with a known email.
	targetEmail := prefix + "_findme@test.com"
	targetID := CreateTestUser(t, prefix+"_target", "User")
	db.Exec("INSERT INTO users_emails (userid, email, canon) VALUES (?, ?, ?)", targetID, targetEmail, targetEmail)

	// Search by email.
	url := fmt.Sprintf("/api/user/search?q=%s&jwt=%s", targetEmail, adminToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	users := result["users"].([]interface{})
	assert.GreaterOrEqual(t, len(users), 1, "Should find user by email")

	// Verify found user has emails populated.
	found := false
	for _, u := range users {
		userMap := u.(map[string]interface{})
		if uint64(userMap["id"].(float64)) == targetID {
			found = true
			// Check that emails are included for admin.
			emails, hasEmails := userMap["emails"]
			assert.True(t, hasEmails, "Admin should see emails")
			emailList, ok := emails.([]interface{})
			assert.True(t, ok)
			assert.Greater(t, len(emailList), 0, "Should have at least one email")
			break
		}
	}
	assert.True(t, found, "Target user should be in search results")
}

func TestSearchUsers_ByID(t *testing.T) {
	prefix := uniquePrefix("searchid")

	// Create an admin user.
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	// Create a target user.
	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Search by numeric ID.
	url := fmt.Sprintf("/api/user/search?q=%d&jwt=%s", targetID, adminToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	users := result["users"].([]interface{})
	assert.GreaterOrEqual(t, len(users), 1, "Should find user by ID")

	found := false
	for _, u := range users {
		userMap := u.(map[string]interface{})
		if uint64(userMap["id"].(float64)) == targetID {
			found = true
			break
		}
	}
	assert.True(t, found, "Target user should be found by ID")
}

func TestSearchUsers_Unauthorized(t *testing.T) {
	// Not logged in should get 401.
	resp, err := getApp().Test(httptest.NewRequest("GET", "/api/user/search?q=test", nil))
	assert.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestSearchUsers_ForbiddenForNonAdmin(t *testing.T) {
	prefix := uniquePrefix("searchforbid")

	// Create a regular user.
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Regular user should get 403.
	url := fmt.Sprintf("/api/user/search?q=test&jwt=%s", token)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestSearchUsers_ForbiddenForModerator(t *testing.T) {
	prefix := uniquePrefix("searchmod")

	// Create a moderator user (not admin/support).
	modID := CreateTestUser(t, prefix, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Moderator should get 403 (only Admin/Support allowed).
	url := fmt.Sprintf("/api/user/search?q=test&jwt=%s", token)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestSearchUsers_EmptyQuery(t *testing.T) {
	prefix := uniquePrefix("searchempty")

	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	// Empty search term should return 400.
	url := fmt.Sprintf("/api/user/search?q=&jwt=%s", adminToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestSearchUsers_NoResults(t *testing.T) {
	prefix := uniquePrefix("searchnone")

	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	// Search for something that should not exist.
	url := fmt.Sprintf("/api/user/search?q=zzzznonexistent99999&jwt=%s", adminToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)

	users := result["users"].([]interface{})
	assert.Equal(t, 0, len(users), "Should find no users")
}

func TestSearchUsers_SupportRole(t *testing.T) {
	prefix := uniquePrefix("searchsupport")

	// Create a Support user.
	supportID := CreateTestUser(t, prefix+"_support", "Support")
	_, supportToken := CreateTestSession(t, supportID)

	// Create a target user.
	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Support role should also be able to search.
	url := fmt.Sprintf("/api/user/search?q=%d&jwt=%s", targetID, supportToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	users := result["users"].([]interface{})
	assert.GreaterOrEqual(t, len(users), 1, "Support should find users")
}

func TestSearchUsers_V2Path(t *testing.T) {
	prefix := uniquePrefix("searchv2")

	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)
	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Test the v2 API path.
	url := fmt.Sprintf("/apiv2/user/search?q=%d&jwt=%s", targetID, adminToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// =============================================================================
// Tests for GET /api/user/fetchmt
// =============================================================================

func TestGetUserFetchMT_WithInfo(t *testing.T) {
	prefix := uniquePrefix("fetchmt")

	// Create a user to fetch.
	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Fetch the user with info (no auth needed for basic fetch, but info object always returned).
	url := fmt.Sprintf("/api/user/fetchmt?id=%d", targetID)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var user user2.User
	err = json.NewDecoder(resp.Body).Decode(&user)
	assert.NoError(t, err)
	assert.Equal(t, targetID, user.ID)

	// Info should always be present (it's part of the User struct).
	// Verify the info object has expected structure.
	assert.GreaterOrEqual(t, user.Info.Openage, uint64(0))
}

func TestGetUserFetchMT_AdminSeesEmails(t *testing.T) {
	prefix := uniquePrefix("fetchmt_admin")
	db := database.DBConn

	// Create an admin user.
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	// Create a target user with a known email.
	targetID := CreateTestUser(t, prefix+"_target", "User")
	testEmail := prefix + "_target@test.com"
	db.Exec("INSERT INTO users_emails (userid, email) VALUES (?, ?) ON DUPLICATE KEY UPDATE email = email", targetID, testEmail)

	// Fetch user as admin - should see emails.
	url := fmt.Sprintf("/api/user/fetchmt?id=%d&jwt=%s", targetID, adminToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var user user2.User
	err = json.NewDecoder(resp.Body).Decode(&user)
	assert.NoError(t, err)
	assert.Equal(t, targetID, user.ID)
	assert.NotNil(t, user.Emails, "Admin should see emails")
	assert.Greater(t, len(user.Emails), 0, "Should have at least one email")
}

func TestGetUserFetchMT_RegularUserNoEmails(t *testing.T) {
	prefix := uniquePrefix("fetchmt_noem")

	// Create a regular user.
	userID := CreateTestUser(t, prefix+"_viewer", "User")
	_, userToken := CreateTestSession(t, userID)

	// Create a target user.
	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Regular user should not see target's emails.
	url := fmt.Sprintf("/api/user/fetchmt?id=%d&jwt=%s", targetID, userToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var user user2.User
	err = json.NewDecoder(resp.Body).Decode(&user)
	assert.NoError(t, err)
	assert.Equal(t, targetID, user.ID)
	assert.Nil(t, user.Emails, "Regular user should not see emails")
}

func TestGetUserFetchMT_WithModtoolsComments(t *testing.T) {
	prefix := uniquePrefix("fetchmt_cmts")
	db := database.DBConn

	// Create a moderator.
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Create target user.
	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Create a group and membership.
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")

	// Add a comment.
	db.Exec("INSERT INTO users_comments (userid, groupid, byuserid, user1, date) VALUES (?, ?, ?, 'Fetchmt note', NOW())",
		targetID, groupID, modID)

	// Fetch with modtools=true.
	url := fmt.Sprintf("/api/user/fetchmt?id=%d&modtools=true&jwt=%s", targetID, modToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var user user2.User
	err = json.NewDecoder(resp.Body).Decode(&user)
	assert.NoError(t, err)
	assert.Equal(t, targetID, user.ID)
	assert.NotNil(t, user.Comments)
	assert.Equal(t, 1, len(user.Comments))
	assert.Equal(t, "Fetchmt note", *user.Comments[0].User1)
}

func TestGetUserFetchMT_MissingID(t *testing.T) {
	// No id parameter should return 400.
	resp, err := getApp().Test(httptest.NewRequest("GET", "/api/user/fetchmt", nil))
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestGetUserFetchMT_InvalidID(t *testing.T) {
	// Non-numeric id should return 400.
	resp, err := getApp().Test(httptest.NewRequest("GET", "/api/user/fetchmt?id=abc", nil))
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestGetUserFetchMT_NonExistentUser(t *testing.T) {
	// Non-existent user should return 404.
	resp, err := getApp().Test(httptest.NewRequest("GET", "/api/user/fetchmt?id=999999999", nil))
	assert.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestGetUserFetchMT_V2Path(t *testing.T) {
	prefix := uniquePrefix("fetchmt_v2")

	targetID := CreateTestUser(t, prefix+"_target", "User")

	url := fmt.Sprintf("/apiv2/user/fetchmt?id=%d", targetID)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetUserFetchMT_MessageHistoryForMod(t *testing.T) {
	prefix := uniquePrefix("fetchmt_mh")

	groupID := CreateTestGroup(t, prefix)
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, posterID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	CreateTestMessage(t, posterID, groupID, prefix+" History Test Item", 55.9533, -3.1883)

	// Fetch user with modtools=true as moderator — should include messagehistory.
	url := fmt.Sprintf("/api/user/fetchmt?id=%d&modtools=true&jwt=%s", posterID, modToken)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var u user2.User
	err = json.NewDecoder(resp.Body).Decode(&u)
	assert.NoError(t, err)
	assert.Equal(t, posterID, u.ID)

	// Should have messagehistory with at least one entry.
	require.NotNil(t, u.MessageHistory, "Should have messagehistory for modtools fetch")
	assert.Greater(t, len(u.MessageHistory), 0, "Should have recent posts")

	// Verify the test message is in history.
	found := false
	for _, h := range u.MessageHistory {
		if h.Groupid == groupID {
			found = true
			assert.GreaterOrEqual(t, h.Daysago, 0, "Daysago should be non-negative")
			break
		}
	}
	assert.True(t, found, "Should find the test message group in history")
}

func TestGetUserFetchMT_NoMessageHistoryWithoutModtools(t *testing.T) {
	prefix := uniquePrefix("fetchmt_nomh")

	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Fetch without modtools=true — should NOT include messagehistory.
	url := fmt.Sprintf("/api/user/fetchmt?id=%d", targetID)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)
	assert.Nil(t, result["messagehistory"], "Should not have messagehistory without modtools=true")
}

func TestGetUserFetchMT_MembershipsReturned(t *testing.T) {
	prefix := uniquePrefix("fetchmt_memb")

	groupID := CreateTestGroup(t, prefix)
	targetID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, targetID, groupID, "Member")

	url := fmt.Sprintf("/api/user/fetchmt?id=%d", targetID)
	resp, err := getApp().Test(httptest.NewRequest("GET", url, nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var u user2.User
	err = json.NewDecoder(resp.Body).Decode(&u)
	assert.NoError(t, err)
	assert.Equal(t, targetID, u.ID)

	// Should have memberships.
	require.NotNil(t, u.Memberships, "Should have memberships")
	assert.Greater(t, len(u.Memberships), 0, "Should have at least one membership")

	found := false
	for _, m := range u.Memberships {
		if m.Groupid == groupID {
			found = true
			assert.Equal(t, "Member", m.Role)
			break
		}
	}
	assert.True(t, found, "Should find the test group membership")
}

// TestFetchMTModmailsCount verifies that the modmails count is returned in fetchmt responses.
func TestFetchMTModmailsCount(t *testing.T) {
	prefix := uniquePrefix("fetchmt_mm")
	db := database.DBConn

	targetID := CreateTestUser(t, prefix+"_target", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, targetID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Create a User2Mod chat room with the target as user1.
	db.Exec("INSERT INTO chat_rooms (user1, user2, groupid, chattype, latestmessage) VALUES (?, ?, ?, 'User2Mod', NOW())",
		targetID, modID, groupID)

	url := fmt.Sprintf("/api/user/fetchmt?id=%d&modtools=true&jwt=%s", targetID, modToken)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var u map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&u)
	modmails, ok := u["modmails"]
	assert.True(t, ok, "Should have modmails field")
	assert.GreaterOrEqual(t, modmails.(float64), float64(1), "Should have at least 1 modmail")
}

// TestFetchMTRepliesByType verifies that repliesoffer and replieswanted are returned in user info.
func TestFetchMTRepliesByType(t *testing.T) {
	prefix := uniquePrefix("fetchmt_rbt")

	targetID := CreateTestUser(t, prefix+"_target", "User")
	_, targetToken := CreateTestSession(t, targetID)

	url := fmt.Sprintf("/api/user/fetchmt?id=%d&modtools=true&jwt=%s", targetID, targetToken)
	req := httptest.NewRequest("GET", url, nil)
	resp, err := getApp().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var u map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&u)

	info, ok := u["info"].(map[string]interface{})
	require.True(t, ok, "Should have info object")

	// Verify the new fields exist (may be 0 for a test user with no activity).
	_, hasRepliesOffer := info["repliesoffer"]
	assert.True(t, hasRepliesOffer, "Should have repliesoffer field")

	_, hasRepliesWanted := info["replieswanted"]
	assert.True(t, hasRepliesWanted, "Should have replieswanted field")

	_, hasExpectedReplies := info["expectedreplies"]
	assert.True(t, hasExpectedReplies, "Should have expectedreplies field")
}
