package test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func mergeUsersB(t *testing.T, adminToken string, id1, id2 uint64) int {
	t.Helper()
	payload := map[string]interface{}{"action": "Merge", "id1": id1, "id2": id2}
	s, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/user?jwt="+adminToken, bytes.NewBuffer(s))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	return resp.StatusCode
}

func mergeAdminSetupB(t *testing.T, prefix string) (uint64, string) {
	t.Helper()
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)
	return adminID, token
}

func TestMergeBMessagesTransferred(t *testing.T) {
	prefix := uniquePrefix("mergeB_msg")
	db := database.DBConn
	_, adminToken := mergeAdminSetupB(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	groupID := CreateTestGroup(t, prefix)
	msgID := CreateTestMessage(t, id1, groupID, "Test offer", 55.9533, -3.1883)

	mergeUsersB(t, adminToken, id1, id2)

	var fromuser uint64
	db.Raw("SELECT fromuser FROM messages WHERE id = ?", msgID).Scan(&fromuser)
	assert.Equal(t, id2, fromuser, "message should now belong to id2")
}

func TestMergeBMessagesHistoryTransferred(t *testing.T) {
	prefix := uniquePrefix("mergeB_msghist")
	db := database.DBConn
	_, adminToken := mergeAdminSetupB(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")

	// Insert a messages_history row for id1 (msgid=NULL to avoid unique constraint issues).
	db.Exec("INSERT INTO messages_history (fromuser, msgid, arrival) VALUES (?, NULL, NOW())", id1)

	mergeUsersB(t, adminToken, id1, id2)

	var count int64
	db.Raw("SELECT COUNT(*) FROM messages_history WHERE fromuser = ?", id2).Scan(&count)
	assert.GreaterOrEqual(t, count, int64(1), "messages_history should transfer to id2")

	db.Raw("SELECT COUNT(*) FROM messages_history WHERE fromuser = ?", id1).Scan(&count)
	assert.Equal(t, int64(0), count, "id1 should have no messages_history rows")
}

func TestMergeBLogsTransferred(t *testing.T) {
	prefix := uniquePrefix("mergeB_logs")
	db := database.DBConn
	_, adminToken := mergeAdminSetupB(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")

	db.Exec("INSERT INTO logs (user, byuser, type, subtype, timestamp) VALUES (?, ?, 'User', 'Created', NOW())", id1, id1)

	mergeUsersB(t, adminToken, id1, id2)

	var count int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE user = ? AND subtype != 'Merged'", id2).Scan(&count)
	assert.GreaterOrEqual(t, count, int64(1), "logs about id1 should transfer to id2")

	db.Raw("SELECT COUNT(*) FROM logs WHERE byuser = ? AND subtype != 'Merged'", id2).Scan(&count)
	assert.GreaterOrEqual(t, count, int64(1), "logs by id1 should transfer to id2")
}

func TestMergeBChatRoomSimpleTransfer(t *testing.T) {
	// id1 has a room with user3; id2 has no room with user3 → room transfers to id2.
	prefix := uniquePrefix("mergeB_chat")
	db := database.DBConn
	_, adminToken := mergeAdminSetupB(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	user3 := CreateTestUser(t, prefix+"_u3", "User")

	roomID := CreateTestChatRoom(t, id1, &user3, nil, "User2User")

	mergeUsersB(t, adminToken, id1, id2)

	var user1 uint64
	db.Raw("SELECT user1 FROM chat_rooms WHERE id = ?", roomID).Scan(&user1)
	assert.Equal(t, id2, user1, "chat room should now belong to id2")
}

func TestMergeBChatRoomDeduplicated(t *testing.T) {
	// Both id1 and id2 have User2User rooms with user3 → merge into one room.
	prefix := uniquePrefix("mergeB_dedup")
	db := database.DBConn
	_, adminToken := mergeAdminSetupB(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	user3 := CreateTestUser(t, prefix+"_u3", "User")

	room1 := CreateTestChatRoom(t, id1, &user3, nil, "User2User")
	CreateTestChatMessage(t, room1, id1, "Hello from id1 a")
	CreateTestChatMessage(t, room1, id1, "Hello from id1 b")

	room2 := CreateTestChatRoom(t, id2, &user3, nil, "User2User")
	CreateTestChatMessage(t, room2, id2, "Hello from id2")

	mergeUsersB(t, adminToken, id1, id2)

	// Only 1 room between id2 and user3.
	var roomCount int64
	db.Raw("SELECT COUNT(*) FROM chat_rooms WHERE (user1 = ? AND user2 = ?) OR (user1 = ? AND user2 = ?)",
		id2, user3, user3, id2).Scan(&roomCount)
	assert.Equal(t, int64(1), roomCount, "should be exactly one room between id2 and user3")

	// All 3 messages in the surviving room (room2).
	var msgCount int64
	db.Raw("SELECT COUNT(*) FROM chat_messages WHERE chatid = ?", room2).Scan(&msgCount)
	assert.Equal(t, int64(3), msgCount, "all 3 messages should be in the surviving room")
}

func TestMergeBChatRosterTransferred(t *testing.T) {
	prefix := uniquePrefix("mergeB_roster")
	db := database.DBConn
	_, adminToken := mergeAdminSetupB(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	user3 := CreateTestUser(t, prefix+"_u3", "User")
	room := CreateTestChatRoom(t, id1, &user3, nil, "User2User")
	db.Exec("INSERT INTO chat_roster (chatid, userid, lastmsgseen) VALUES (?, ?, 0)", room, id1)

	mergeUsersB(t, adminToken, id1, id2)

	var count int64
	db.Raw("SELECT COUNT(*) FROM chat_roster WHERE userid = ?", id2).Scan(&count)
	assert.GreaterOrEqual(t, count, int64(1), "chat_roster should transfer to id2")
}

func TestMergeBSessionsTransferred(t *testing.T) {
	prefix := uniquePrefix("mergeB_sess")
	db := database.DBConn
	_, adminToken := mergeAdminSetupB(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	id1SessionID, _ := CreateTestSession(t, id1)

	mergeUsersB(t, adminToken, id1, id2)

	var userid uint64
	db.Raw("SELECT userid FROM sessions WHERE id = ?", id1SessionID).Scan(&userid)
	assert.Equal(t, id2, userid, "id1's session should be reassigned to id2")
}

func TestMergeBLoginsTransferred(t *testing.T) {
	prefix := uniquePrefix("mergeB_login")
	db := database.DBConn
	_, adminToken := mergeAdminSetupB(t, prefix)

	id1 := CreateTestUser(t, prefix+"_u1", "User")
	id2 := CreateTestUser(t, prefix+"_u2", "User")
	db.Exec("INSERT INTO users_logins (userid, type, uid) VALUES (?, 'Facebook', 'fb12345')", id1)

	mergeUsersB(t, adminToken, id1, id2)

	var count int64
	db.Raw("SELECT COUNT(*) FROM users_logins WHERE userid = ? AND type = 'Facebook'", id2).Scan(&count)
	assert.Equal(t, int64(1), count, "id1's Facebook login should transfer to id2")
}
