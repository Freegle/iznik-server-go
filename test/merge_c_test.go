package test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func mergeUsersC(t *testing.T, adminToken string, id1, id2 uint64) int {
	t.Helper()
	payload := map[string]interface{}{"action": "Merge", "id1": id1, "id2": id2}
	s, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/user?jwt="+adminToken, bytes.NewBuffer(s))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	return resp.StatusCode
}

func mergeAdminSetupC(t *testing.T, prefix string) (uint64, string) {
	t.Helper()
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)
	return adminID, token
}

func TestMergeCUserAttributeNameFromId1(t *testing.T) {
	// id2 has no fullname; id1's fullname should be taken.
	prefix := uniquePrefix("mergeC_name")
	db := database.DBConn
	_, adminToken := mergeAdminSetupC(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	db.Exec("UPDATE users SET fullname = 'Alice Smith', firstname = 'Alice', lastname = 'Smith' WHERE id = ?", id1)
	db.Exec("UPDATE users SET fullname = NULL, firstname = NULL, lastname = NULL WHERE id = ?", id2)

	mergeUsersC(t, adminToken, id1, id2)

	var fullname string
	db.Raw("SELECT fullname FROM users WHERE id = ?", id2).Scan(&fullname)
	assert.Equal(t, "Alice Smith", fullname, "id2 should inherit id1's fullname when id2 has none")

	var firstname string
	db.Raw("SELECT firstname FROM users WHERE id = ?", id2).Scan(&firstname)
	assert.Equal(t, "Alice", firstname, "id2 should inherit id1's firstname when id2 has none")
}

func TestMergeCUserAttributeId2NamePreserved(t *testing.T) {
	// id2 already has a fullname; it should NOT be overwritten.
	prefix := uniquePrefix("mergeC_keep")
	db := database.DBConn
	_, adminToken := mergeAdminSetupC(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	db.Exec("UPDATE users SET fullname = 'Old Name' WHERE id = ?", id1)
	db.Exec("UPDATE users SET fullname = 'Alice Smith' WHERE id = ?", id2)

	mergeUsersC(t, adminToken, id1, id2)

	var fullname string
	db.Raw("SELECT fullname FROM users WHERE id = ?", id2).Scan(&fullname)
	assert.Equal(t, "Alice Smith", fullname, "id2's existing fullname should not be overwritten")
}

func TestMergeCSystemroleTakesMax(t *testing.T) {
	prefix := uniquePrefix("mergeC_role")
	db := database.DBConn
	_, adminToken := mergeAdminSetupC(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "Support")
	id2 := CreateTestUser(t, prefix+"_u2", "User")

	mergeUsersC(t, adminToken, id1, id2)

	var role string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", id2).Scan(&role)
	assert.Equal(t, "Support", role, "id2 should have the higher systemrole after merge")
}

func TestMergeCAddedDateTakesOldest(t *testing.T) {
	prefix := uniquePrefix("mergeC_added")
	db := database.DBConn
	_, adminToken := mergeAdminSetupC(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	db.Exec("UPDATE users SET added = '2010-01-01 00:00:00' WHERE id = ?", id1)
	db.Exec("UPDATE users SET added = '2020-01-01 00:00:00' WHERE id = ?", id2)

	mergeUsersC(t, adminToken, id1, id2)

	var added string
	db.Raw("SELECT DATE_FORMAT(added, '%Y-%m-%d') FROM users WHERE id = ?", id2).Scan(&added)
	assert.Equal(t, "2010-01-01", added, "id2 should have the oldest added date")
}

func TestMergeCDonationsTransferred(t *testing.T) {
	// users_donations is the table that caused the real-world merge incident.
	prefix := uniquePrefix("mergeC_donate")
	db := database.DBConn
	_, adminToken := mergeAdminSetupC(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	db.Exec("INSERT INTO users_donations (userid, Payer, PayerDisplayName, GrossAmount, source, timestamp, type) VALUES (?, 'test@example.com', 'Test Donor', 500, 'Stripe', NOW(), 'Stripe')", id1)

	mergeUsersC(t, adminToken, id1, id2)

	var count int64
	db.Raw("SELECT COUNT(*) FROM users_donations WHERE userid = ?", id2).Scan(&count)
	assert.GreaterOrEqual(t, count, int64(1), "id1's donation should transfer to id2")

	db.Raw("SELECT COUNT(*) FROM users_donations WHERE userid = ?", id1).Scan(&count)
	assert.Equal(t, int64(0), count, "no donations should remain on id1")
}

func TestMergeCBansHandled(t *testing.T) {
	prefix := uniquePrefix("mergeC_bans")
	db := database.DBConn
	_, adminToken := mergeAdminSetupC(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	groupID := CreateTestGroup(t, prefix)
	bannerID := CreateTestUser(t, prefix+"_banner", "Admin")

	db.Exec("INSERT INTO users_banned (userid, groupid, byuser) VALUES (?, ?, ?)", id1, groupID, bannerID)
	CreateTestMembership(t, id2, groupID, "Member")

	mergeUsersC(t, adminToken, id1, id2)

	var banCount int64
	db.Raw("SELECT COUNT(*) FROM users_banned WHERE userid = ? AND groupid = ?", id2, groupID).Scan(&banCount)
	assert.GreaterOrEqual(t, banCount, int64(1), "id2 should be banned after inheriting id1's ban")

	var membCount int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ?", id2, groupID).Scan(&membCount)
	assert.Equal(t, int64(0), membCount, "id2's membership should be removed for banned group")
}

func TestMergeCGiftaidBestPeriodKept(t *testing.T) {
	prefix := uniquePrefix("mergeC_giftaid")
	db := database.DBConn
	_, adminToken := mergeAdminSetupC(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")

	db.Exec("INSERT INTO giftaid (userid, period, fullname, homeaddress) VALUES (?, 'Past4YearsAndFuture', 'Alice', '1 Main St')", id1)
	db.Exec("INSERT INTO giftaid (userid, period, fullname, homeaddress) VALUES (?, 'Declined', 'Alice', '1 Main St')", id2)

	mergeUsersC(t, adminToken, id1, id2)

	var count int64
	db.Raw("SELECT COUNT(*) FROM giftaid WHERE userid = ?", id2).Scan(&count)
	assert.Equal(t, int64(1), count, "only one giftaid row should remain")

	var period string
	db.Raw("SELECT period FROM giftaid WHERE userid = ?", id2).Scan(&period)
	assert.Equal(t, "Past4YearsAndFuture", period, "the best period should be kept")
}

func TestMergeCMergeLogEntries(t *testing.T) {
	prefix := uniquePrefix("mergeC_logentry")
	db := database.DBConn
	_, adminToken := mergeAdminSetupC(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")

	mergeUsersC(t, adminToken, id1, id2)

	// Two log entries with subtype=Merged must exist.
	entry1 := findLog(db, "User", "Merged", id1)
	assert.NotNil(t, entry1, "should have a Merged log for the discarded user (id1)")

	entry2 := findLog(db, "User", "Merged", id2)
	assert.NotNil(t, entry2, "should have a Merged log for the kept user (id2)")
}
