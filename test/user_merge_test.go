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

// mergeUsers calls POST /api/user action=Merge with an admin token.
func mergeUsers(t *testing.T, adminToken string, id1, id2 uint64) int {
	t.Helper()
	payload := map[string]interface{}{"action": "Merge", "id1": id1, "id2": id2}
	s, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/user?jwt="+adminToken, bytes.NewBuffer(s))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	return resp.StatusCode
}

// mergeAdminSetup creates an Admin user and returns (adminID, adminToken).
func mergeAdminSetup(t *testing.T, prefix string) (uint64, string) {
	t.Helper()
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)
	return adminID, token
}

// ── Section A: emails and memberships ────────────────────────────────────────

func TestMergeEmailsTransferred(t *testing.T) {
	prefix := uniquePrefix("merge_email")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")

	var count int64
	db.Raw("SELECT COUNT(*) FROM users_emails WHERE userid = ?", id1).Scan(&count)
	assert.Equal(t, int64(1), count)

	status := mergeUsers(t, adminToken, id1, id2)
	assert.Equal(t, 200, status, "merge request should succeed")

	db.Raw("SELECT COUNT(*) FROM users_emails WHERE userid = ?", id2).Scan(&count)
	assert.GreaterOrEqual(t, count, int64(2), "id2 should have its own + id1's email")

	db.Raw("SELECT COUNT(*) FROM users_emails WHERE userid = ?", id1).Scan(&count)
	assert.Equal(t, int64(0), count, "id1 should have no emails after merge")
}

func TestMergeEmailPreferredWinsForId2(t *testing.T) {
	prefix := uniquePrefix("merge_pref")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")

	db.Exec("UPDATE users_emails SET preferred = 1 WHERE userid = ?", id1)
	db.Exec("UPDATE users_emails SET preferred = 1 WHERE userid = ?", id2)

	var id2PreferredEmail string
	db.Raw("SELECT email FROM users_emails WHERE userid = ? AND preferred = 1", id2).Scan(&id2PreferredEmail)
	assert.NotEmpty(t, id2PreferredEmail, "id2 should have a preferred email before merge")

	status := mergeUsers(t, adminToken, id1, id2)
	assert.Equal(t, 200, status, "merge request should succeed")

	var preferredEmail string
	db.Raw("SELECT email FROM users_emails WHERE userid = ? AND preferred = 1", id2).Scan(&preferredEmail)
	assert.Equal(t, id2PreferredEmail, preferredEmail, "id2's preferred email must remain preferred after merge")
}

func TestMergeMembershipTransferred(t *testing.T) {
	prefix := uniquePrefix("merge_memb")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, id1, groupID, "Member")

	status := mergeUsers(t, adminToken, id1, id2)
	assert.Equal(t, 200, status, "merge request should succeed")

	var role string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ?", id2, groupID).Scan(&role)
	assert.Equal(t, "Member", role, "id2 should inherit id1's membership")

	var count int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ?", id1).Scan(&count)
	assert.Equal(t, int64(0), count, "id1 should have no memberships after merge")
}

func TestMergeMembershipRoleTakesMax(t *testing.T) {
	prefix := uniquePrefix("merge_role")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, id1, groupID, "Moderator")
	CreateTestMembership(t, id2, groupID, "Member")

	status := mergeUsers(t, adminToken, id1, id2)
	assert.Equal(t, 200, status, "merge request should succeed")

	var role string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ?", id2, groupID).Scan(&role)
	assert.Equal(t, "Moderator", role, "merged user should have the higher role")
}

func TestMergeMembershipConflictTakesOlderDate(t *testing.T) {
	prefix := uniquePrefix("merge_date")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	groupID := CreateTestGroup(t, prefix)

	db.Exec(fmt.Sprintf("INSERT INTO memberships (userid, groupid, role, added) VALUES (%d, %d, 'Member', '2013-01-01 00:00:00')", id1, groupID))
	db.Exec(fmt.Sprintf("INSERT INTO memberships (userid, groupid, role, added) VALUES (%d, %d, 'Member', NOW())", id2, groupID))

	status := mergeUsers(t, adminToken, id1, id2)
	assert.Equal(t, 200, status, "merge request should succeed")

	var added string
	db.Raw("SELECT DATE_FORMAT(added, '%Y-%m-%d') FROM memberships WHERE userid = ? AND groupid = ?", id2, groupID).Scan(&added)
	assert.Equal(t, "2013-01-01", added, "merged user should have the older joined date")
}

// ── Section B: messages, history, chat rooms, sessions, logins ───────────────

func TestMergeMessagesTransferred(t *testing.T) {
	prefix := uniquePrefix("merge_msg")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, id1, groupID, "Test offer", 55.9533, -3.1883)

	mergeUsers(t, adminToken, id1, id2)

	var fromuser uint64
	db.Raw("SELECT fromuser FROM messages WHERE id = ?", msgID).Scan(&fromuser)
	assert.Equal(t, id2, fromuser, "message should now belong to id2")
}

func TestMergeMessagesHistoryTransferred(t *testing.T) {
	prefix := uniquePrefix("merge_msghist")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")

	db.Exec("INSERT INTO messages_history (fromuser, msgid, arrival) VALUES (?, NULL, NOW())", id1)

	mergeUsers(t, adminToken, id1, id2)

	var count int64
	db.Raw("SELECT COUNT(*) FROM messages_history WHERE fromuser = ?", id2).Scan(&count)
	assert.GreaterOrEqual(t, count, int64(1), "messages_history should transfer to id2")

	db.Raw("SELECT COUNT(*) FROM messages_history WHERE fromuser = ?", id1).Scan(&count)
	assert.Equal(t, int64(0), count, "id1 should have no messages_history rows")
}

func TestMergeLogsTransferred(t *testing.T) {
	prefix := uniquePrefix("merge_logs")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")

	db.Exec("INSERT INTO logs (user, byuser, type, subtype, timestamp) VALUES (?, ?, 'User', 'Created', NOW())", id1, id1)

	mergeUsers(t, adminToken, id1, id2)

	var count int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE user = ? AND subtype != 'Merged'", id2).Scan(&count)
	assert.GreaterOrEqual(t, count, int64(1), "logs about id1 should transfer to id2")

	db.Raw("SELECT COUNT(*) FROM logs WHERE byuser = ? AND subtype != 'Merged'", id2).Scan(&count)
	assert.GreaterOrEqual(t, count, int64(1), "logs by id1 should transfer to id2")
}

func TestMergeChatRoomSimpleTransfer(t *testing.T) {
	// id1 has a room with user3; id2 has no room with user3 → room transfers to id2.
	prefix := uniquePrefix("merge_chat")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	user3 := CreateTestUser(t, prefix+"_u3", "User")

	roomID := CreateTestChatRoom(t, id1, &user3, nil, "User2User")

	mergeUsers(t, adminToken, id1, id2)

	var user1 uint64
	db.Raw("SELECT user1 FROM chat_rooms WHERE id = ?", roomID).Scan(&user1)
	assert.Equal(t, id2, user1, "chat room should now belong to id2")
}

func TestMergeChatRoomDeduplicated(t *testing.T) {
	// Both id1 and id2 have User2User rooms with user3 → merge into one room.
	prefix := uniquePrefix("merge_dedup")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	user3 := CreateTestUser(t, prefix+"_u3", "User")

	room1 := CreateTestChatRoom(t, id1, &user3, nil, "User2User")
	CreateTestChatMessage(t, room1, id1, "Hello from id1 a")
	CreateTestChatMessage(t, room1, id1, "Hello from id1 b")

	room2 := CreateTestChatRoom(t, id2, &user3, nil, "User2User")
	CreateTestChatMessage(t, room2, id2, "Hello from id2")

	mergeUsers(t, adminToken, id1, id2)

	var roomCount int64
	db.Raw("SELECT COUNT(*) FROM chat_rooms WHERE (user1 = ? AND user2 = ?) OR (user1 = ? AND user2 = ?)",
		id2, user3, user3, id2).Scan(&roomCount)
	assert.Equal(t, int64(1), roomCount, "should be exactly one room between id2 and user3")

	var msgCount int64
	db.Raw("SELECT COUNT(*) FROM chat_messages WHERE chatid = ?", room2).Scan(&msgCount)
	assert.Equal(t, int64(3), msgCount, "all 3 messages should be in the surviving room")
}

func TestMergeChatRosterTransferred(t *testing.T) {
	prefix := uniquePrefix("merge_roster")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	user3 := CreateTestUser(t, prefix+"_u3", "User")
	room := CreateTestChatRoom(t, id1, &user3, nil, "User2User")
	db.Exec("INSERT INTO chat_roster (chatid, userid, lastmsgseen) VALUES (?, ?, 0)", room, id1)

	mergeUsers(t, adminToken, id1, id2)

	var count int64
	db.Raw("SELECT COUNT(*) FROM chat_roster WHERE userid = ?", id2).Scan(&count)
	assert.GreaterOrEqual(t, count, int64(1), "chat_roster should transfer to id2")
}

func TestMergeSessionsTransferred(t *testing.T) {
	prefix := uniquePrefix("merge_sess")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	id1SessionID, _ := CreateTestSession(t, id1)

	mergeUsers(t, adminToken, id1, id2)

	var userid uint64
	db.Raw("SELECT userid FROM sessions WHERE id = ?", id1SessionID).Scan(&userid)
	assert.Equal(t, id2, userid, "id1's session should be reassigned to id2")
}

func TestMergeLoginsTransferred(t *testing.T) {
	prefix := uniquePrefix("merge_login")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	db.Exec("INSERT INTO users_logins (userid, type, uid) VALUES (?, 'Facebook', ?)", id1, prefix+"_fb")

	mergeUsers(t, adminToken, id1, id2)

	var count int64
	db.Raw("SELECT COUNT(*) FROM users_logins WHERE userid = ? AND type = 'Facebook'", id2).Scan(&count)
	assert.Equal(t, int64(1), count, "id1's Facebook login should transfer to id2")
}

// ── Section C: user attributes, simple tables, bans, giftaid, log entries ────

func TestMergeUserAttributeNameFromId1(t *testing.T) {
	// id2 has no fullname; id1's fullname should be taken.
	prefix := uniquePrefix("merge_name")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	db.Exec("UPDATE users SET fullname = 'Alice Smith', firstname = 'Alice', lastname = 'Smith' WHERE id = ?", id1)
	db.Exec("UPDATE users SET fullname = NULL, firstname = NULL, lastname = NULL WHERE id = ?", id2)

	mergeUsers(t, adminToken, id1, id2)

	var fullname string
	db.Raw("SELECT fullname FROM users WHERE id = ?", id2).Scan(&fullname)
	assert.Equal(t, "Alice Smith", fullname, "id2 should inherit id1's fullname when id2 has none")

	var firstname string
	db.Raw("SELECT firstname FROM users WHERE id = ?", id2).Scan(&firstname)
	assert.Equal(t, "Alice", firstname, "id2 should inherit id1's firstname when id2 has none")
}

func TestMergeUserAttributeId2NamePreserved(t *testing.T) {
	// id2 already has a fullname; it should NOT be overwritten.
	prefix := uniquePrefix("merge_keep")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	db.Exec("UPDATE users SET fullname = 'Old Name' WHERE id = ?", id1)
	db.Exec("UPDATE users SET fullname = 'Alice Smith' WHERE id = ?", id2)

	mergeUsers(t, adminToken, id1, id2)

	var fullname string
	db.Raw("SELECT fullname FROM users WHERE id = ?", id2).Scan(&fullname)
	assert.Equal(t, "Alice Smith", fullname, "id2's existing fullname should not be overwritten")
}

func TestMergeSystemroleTakesMax(t *testing.T) {
	prefix := uniquePrefix("merge_sysrole")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "Support")
	id2 := CreateTestUser(t, prefix+"_u2", "User")

	mergeUsers(t, adminToken, id1, id2)

	var role string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", id2).Scan(&role)
	assert.Equal(t, "Support", role, "id2 should have the higher systemrole after merge")
}

func TestMergeAddedDateTakesOldest(t *testing.T) {
	prefix := uniquePrefix("merge_added")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	db.Exec("UPDATE users SET added = '2010-01-01 00:00:00' WHERE id = ?", id1)
	db.Exec("UPDATE users SET added = '2020-01-01 00:00:00' WHERE id = ?", id2)

	mergeUsers(t, adminToken, id1, id2)

	var added string
	db.Raw("SELECT DATE_FORMAT(added, '%Y-%m-%d') FROM users WHERE id = ?", id2).Scan(&added)
	assert.Equal(t, "2010-01-01", added, "id2 should have the oldest added date")
}

func TestMergeDonationsTransferred(t *testing.T) {
	prefix := uniquePrefix("merge_donate")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	db.Exec("INSERT INTO users_donations (userid, type, Payer, PayerDisplayName, GrossAmount, source) VALUES (?, 'Stripe', 'Test', 'Test', 10.00, 'Stripe')", id1)

	mergeUsers(t, adminToken, id1, id2)

	var count int64
	db.Raw("SELECT COUNT(*) FROM users_donations WHERE userid = ?", id2).Scan(&count)
	assert.GreaterOrEqual(t, count, int64(1), "id1's donation should transfer to id2")

	db.Raw("SELECT COUNT(*) FROM users_donations WHERE userid = ?", id1).Scan(&count)
	assert.Equal(t, int64(0), count, "no donations should remain on id1")
}

func TestMergeBansHandled(t *testing.T) {
	prefix := uniquePrefix("merge_bans")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	groupID := CreateTestGroup(t, prefix)
	bannerID := CreateTestUser(t, prefix+"_banner", "Admin")

	db.Exec("INSERT INTO users_banned (userid, groupid, byuser) VALUES (?, ?, ?)", id1, groupID, bannerID)
	CreateTestMembership(t, id2, groupID, "Member")

	mergeUsers(t, adminToken, id1, id2)

	var banCount int64
	db.Raw("SELECT COUNT(*) FROM users_banned WHERE userid = ? AND groupid = ?", id2, groupID).Scan(&banCount)
	assert.GreaterOrEqual(t, banCount, int64(1), "id2 should be banned after inheriting id1's ban")

	var membCount int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ?", id2, groupID).Scan(&membCount)
	assert.Equal(t, int64(0), membCount, "id2's membership should be removed for banned group")
}

func TestMergeGiftaidBestPeriodKept(t *testing.T) {
	prefix := uniquePrefix("merge_giftaid")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")

	db.Exec("INSERT INTO giftaid (userid, period, fullname, homeaddress) VALUES (?, 'Past4YearsAndFuture', 'Alice', '1 Main St')", id1)
	db.Exec("INSERT INTO giftaid (userid, period, fullname, homeaddress) VALUES (?, 'Declined', 'Alice', '1 Main St')", id2)

	mergeUsers(t, adminToken, id1, id2)

	var count int64
	db.Raw("SELECT COUNT(*) FROM giftaid WHERE userid = ?", id2).Scan(&count)
	assert.Equal(t, int64(1), count, "only one giftaid row should remain")

	var period string
	db.Raw("SELECT period FROM giftaid WHERE userid = ?", id2).Scan(&period)
	assert.Equal(t, "Past4YearsAndFuture", period, "the best period should be kept")
}

func TestMergeMergeLogEntries(t *testing.T) {
	prefix := uniquePrefix("merge_logentry")
	db := database.DBConn
	_, adminToken := mergeAdminSetup(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")

	mergeUsers(t, adminToken, id1, id2)

	entry1 := findLog(db, "User", "Merged", id1)
	assert.NotNil(t, entry1, "should have a Merged log for the discarded user (id1)")

	entry2 := findLog(db, "User", "Merged", id2)
	assert.NotNil(t, entry2, "should have a Merged log for the kept user (id2)")
}
