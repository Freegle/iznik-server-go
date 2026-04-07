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

// mergeUsersA is a helper: POST /api/user action=Merge with admin token.
func mergeUsersA(t *testing.T, adminToken string, id1, id2 uint64) int {
	t.Helper()
	payload := map[string]interface{}{"action": "Merge", "id1": id1, "id2": id2}
	s, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/user?jwt="+adminToken, bytes.NewBuffer(s))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	return resp.StatusCode
}

// mergeAdminSetupA creates an Admin user and returns (adminID, adminToken).
func mergeAdminSetupA(t *testing.T, prefix string) (uint64, string) {
	t.Helper()
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)
	return adminID, token
}

func TestMergeAEmailsTransferred(t *testing.T) {
	prefix := uniquePrefix("mergeA_email")
	db := database.DBConn
	_, adminToken := mergeAdminSetupA(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")

	// Confirm id1 has its default email
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_emails WHERE userid = ?", id1).Scan(&count)
	assert.Equal(t, int64(1), count)

	status := mergeUsersA(t, adminToken, id1, id2)
	assert.Equal(t, 200, status, "merge request should succeed")

	// id1's email must now belong to id2
	db.Raw("SELECT COUNT(*) FROM users_emails WHERE userid = ?", id2).Scan(&count)
	assert.GreaterOrEqual(t, count, int64(2), "id2 should have its own + id1's email")

	db.Raw("SELECT COUNT(*) FROM users_emails WHERE userid = ?", id1).Scan(&count)
	assert.Equal(t, int64(0), count, "id1 should have no emails after merge")
}

func TestMergeAEmailPreferredWinsForId2(t *testing.T) {
	prefix := uniquePrefix("mergeA_pref")
	db := database.DBConn
	_, adminToken := mergeAdminSetupA(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")

	db.Exec("UPDATE users_emails SET preferred = 1 WHERE userid = ?", id1)
	db.Exec("UPDATE users_emails SET preferred = 1 WHERE userid = ?", id2)

	var id2PreferredEmail string
	db.Raw("SELECT email FROM users_emails WHERE userid = ? AND preferred = 1", id2).Scan(&id2PreferredEmail)
	assert.NotEmpty(t, id2PreferredEmail, "id2 should have a preferred email before merge")

	status := mergeUsersA(t, adminToken, id1, id2)
	assert.Equal(t, 200, status, "merge request should succeed")

	var preferredEmail string
	db.Raw("SELECT email FROM users_emails WHERE userid = ? AND preferred = 1", id2).Scan(&preferredEmail)
	assert.Equal(t, id2PreferredEmail, preferredEmail, "id2's preferred email must remain preferred after merge")
}

func TestMergeAMembershipTransferred(t *testing.T) {
	prefix := uniquePrefix("mergeA_memb")
	db := database.DBConn
	_, adminToken := mergeAdminSetupA(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, id1, groupID, "Member")

	status := mergeUsersA(t, adminToken, id1, id2)
	assert.Equal(t, 200, status, "merge request should succeed")

	var role string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ?", id2, groupID).Scan(&role)
	assert.Equal(t, "Member", role, "id2 should inherit id1's membership")

	var count int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ?", id1).Scan(&count)
	assert.Equal(t, int64(0), count, "id1 should have no memberships after merge")
}

func TestMergeAMembershipRoleTakesMax(t *testing.T) {
	prefix := uniquePrefix("mergeA_role")
	db := database.DBConn
	_, adminToken := mergeAdminSetupA(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, id1, groupID, "Moderator")
	CreateTestMembership(t, id2, groupID, "Member")

	status := mergeUsersA(t, adminToken, id1, id2)
	assert.Equal(t, 200, status, "merge request should succeed")

	var role string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ?", id2, groupID).Scan(&role)
	assert.Equal(t, "Moderator", role, "merged user should have the higher role")
}

func TestMergeAMembershipConflictTakesOlderDate(t *testing.T) {
	prefix := uniquePrefix("mergeA_date")
	db := database.DBConn
	_, adminToken := mergeAdminSetupA(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	groupID := CreateTestGroup(t, prefix)

	db.Exec(fmt.Sprintf("INSERT INTO memberships (userid, groupid, role, added) VALUES (%d, %d, 'Member', '2013-01-01 00:00:00')", id1, groupID))
	db.Exec(fmt.Sprintf("INSERT INTO memberships (userid, groupid, role, added) VALUES (%d, %d, 'Member', NOW())", id2, groupID))

	status := mergeUsersA(t, adminToken, id1, id2)
	assert.Equal(t, 200, status, "merge request should succeed")

	var added string
	db.Raw("SELECT DATE_FORMAT(added, '%Y-%m-%d') FROM memberships WHERE userid = ? AND groupid = ?", id2, groupID).Scan(&added)
	assert.Equal(t, "2013-01-01", added, "merged user should have the older joined date")
}
