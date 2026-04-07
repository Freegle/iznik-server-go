package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/group"
	"github.com/stretchr/testify/assert"
)

func TestGetGroupWork_Unauthenticated(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/work", nil))
	assert.Equal(t, 401, resp.StatusCode)
}

func TestGetGroupWork_NoGroups(t *testing.T) {
	prefix := uniquePrefix("gwnogrp")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/work?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result []group.GroupWork
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, 0, len(result))
}

func TestGetGroupWork_ActiveMod(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("gwactive")

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")

	// Create active mod membership (default settings = active).
	CreateTestMembership(t, userID, groupID, "Moderator")
	_, token := CreateTestSession(t, userID)

	// Insert a pending message.
	senderID := CreateTestUser(t, prefix+"_sender", "User")
	var msgID uint64
	db.Exec("INSERT INTO messages (fromuser, type, subject) VALUES (?, 'Offer', 'Test pending')", senderID)
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", senderID).Scan(&msgID)
	db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, deleted) VALUES (?, ?, 'Pending', 0)", msgID, groupID)

	// Insert a spam message.
	var spamMsgID uint64
	db.Exec("INSERT INTO messages (fromuser, type, subject) VALUES (?, 'Offer', 'Test spam')", senderID)
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", senderID).Scan(&spamMsgID)
	db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, deleted) VALUES (?, ?, 'Spam', 0)", spamMsgID, groupID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/work?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result []group.GroupWork
	json2.Unmarshal(rsp(resp), &result)
	assert.GreaterOrEqual(t, len(result), 1)

	// Find our group in the results.
	var found *group.GroupWork
	for i := range result {
		if result[i].Groupid == groupID {
			found = &result[i]
			break
		}
	}
	assert.NotNil(t, found, "Expected group %d in work results", groupID)
	assert.GreaterOrEqual(t, found.Pending, int64(1), "Expected pending >= 1")
	assert.GreaterOrEqual(t, found.Spam, int64(1), "Expected spam >= 1")
	// Since this is an active group, pendingother should be 0 for unheld messages.
	assert.Equal(t, int64(0), found.Pendingother, "Unheld pending on active group should be in 'pending', not 'pendingother'")

	// Clean up.
	db.Exec("DELETE FROM messages_groups WHERE msgid IN (?, ?)", msgID, spamMsgID)
	db.Exec("DELETE FROM messages WHERE id IN (?, ?)", msgID, spamMsgID)
}

func TestGetGroupWork_BackupMod(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("gwbackup")

	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")

	// Create backup mod membership (active=0 in settings).
	db.Exec("INSERT INTO memberships (userid, groupid, role, settings) VALUES (?, ?, 'Moderator', ?)",
		userID, groupID, `{"active":0}`)
	_, token := CreateTestSession(t, userID)

	// Insert a pending message.
	senderID := CreateTestUser(t, prefix+"_sender", "User")
	var msgID uint64
	db.Exec("INSERT INTO messages (fromuser, type, subject) VALUES (?, 'Offer', 'Test backup pending')", senderID)
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", senderID).Scan(&msgID)
	db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, deleted) VALUES (?, ?, 'Pending', 0)", msgID, groupID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/work?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result []group.GroupWork
	json2.Unmarshal(rsp(resp), &result)

	var found *group.GroupWork
	for i := range result {
		if result[i].Groupid == groupID {
			found = &result[i]
			break
		}
	}
	assert.NotNil(t, found, "Expected group %d in work results", groupID)
	// Backup group: all pending → pendingother.
	assert.Equal(t, int64(0), found.Pending, "Backup group pending should be in 'pendingother'")
	assert.GreaterOrEqual(t, found.Pendingother, int64(1), "Expected pendingother >= 1 for backup group")
	// Spam should be 0 for inactive groups.
	assert.Equal(t, int64(0), found.Spam, "Backup group should not have spam count")

	// Clean up.
	db.Exec("DELETE FROM messages_groups WHERE msgid = ?", msgID)
	db.Exec("DELETE FROM messages WHERE id = ?", msgID)
}

func TestGetGroupWork_HeldPending(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("gwheld")

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	holderID := CreateTestUser(t, prefix+"_holder", "User")

	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Insert a held pending message.
	senderID := CreateTestUser(t, prefix+"_sender", "User")
	var msgID uint64
	db.Exec("INSERT INTO messages (fromuser, type, subject, heldby) VALUES (?, 'Offer', 'Test held', ?)", senderID, holderID)
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", senderID).Scan(&msgID)
	db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, deleted) VALUES (?, ?, 'Pending', 0)", msgID, groupID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/work?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result []group.GroupWork
	json2.Unmarshal(rsp(resp), &result)

	var found *group.GroupWork
	for i := range result {
		if result[i].Groupid == groupID {
			found = &result[i]
			break
		}
	}
	assert.NotNil(t, found, "Expected group %d in work results", groupID)
	// Held message on active group → pendingother.
	assert.Equal(t, int64(0), found.Pending, "Held pending should not be in 'pending'")
	assert.GreaterOrEqual(t, found.Pendingother, int64(1), "Held pending should be in 'pendingother'")

	// Clean up.
	db.Exec("DELETE FROM messages_groups WHERE msgid = ?", msgID)
	db.Exec("DELETE FROM messages WHERE id = ?", msgID)
}

func TestGetGroupWork_SpamMembers(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("gwspam")

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")

	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Insert a spam member (reviewrequestedat set, reviewedat NULL).
	spamUserID := CreateTestUser(t, prefix+"_spam", "User")
	db.Exec("INSERT INTO memberships (userid, groupid, role, collection, reviewrequestedat) VALUES (?, ?, 'Member', 'Approved', NOW())",
		spamUserID, groupID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/work?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result []group.GroupWork
	json2.Unmarshal(rsp(resp), &result)

	var found *group.GroupWork
	for i := range result {
		if result[i].Groupid == groupID {
			found = &result[i]
			break
		}
	}
	assert.NotNil(t, found)
	assert.GreaterOrEqual(t, found.Spammembers, int64(1), "Expected spammembers >= 1")
}

func TestGetGroupWork_MultipleGroups(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("gwmulti")

	groupID1 := CreateTestGroup(t, prefix + "_g1")
	groupID2 := CreateTestGroup(t, prefix + "_g2")
	modID := CreateTestUser(t, prefix+"_mod", "User")

	// Active on group 1, backup on group 2.
	CreateTestMembership(t, modID, groupID1, "Moderator")
	db.Exec("INSERT INTO memberships (userid, groupid, role, settings) VALUES (?, ?, 'Moderator', ?)",
		modID, groupID2, `{"active":0}`)
	_, token := CreateTestSession(t, modID)

	// Pending message in each group.
	senderID := CreateTestUser(t, prefix+"_sender", "User")
	var msgID1, msgID2 uint64
	db.Exec("INSERT INTO messages (fromuser, type, subject) VALUES (?, 'Offer', 'Test multi 1')", senderID)
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", senderID).Scan(&msgID1)
	db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, deleted) VALUES (?, ?, 'Pending', 0)", msgID1, groupID1)

	db.Exec("INSERT INTO messages (fromuser, type, subject) VALUES (?, 'Offer', 'Test multi 2')", senderID)
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", senderID).Scan(&msgID2)
	db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, deleted) VALUES (?, ?, 'Pending', 0)", msgID2, groupID2)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/work?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result []group.GroupWork
	json2.Unmarshal(rsp(resp), &result)

	var g1, g2 *group.GroupWork
	for i := range result {
		if result[i].Groupid == groupID1 {
			g1 = &result[i]
		}
		if result[i].Groupid == groupID2 {
			g2 = &result[i]
		}
	}

	assert.NotNil(t, g1, "Expected group %d (active) in results", groupID1)
	assert.NotNil(t, g2, "Expected group %d (backup) in results", groupID2)

	// Active group: pending in primary field.
	assert.GreaterOrEqual(t, g1.Pending, int64(1))
	assert.Equal(t, int64(0), g1.Pendingother)

	// Backup group: pending in other field.
	assert.Equal(t, int64(0), g2.Pending)
	assert.GreaterOrEqual(t, g2.Pendingother, int64(1))

	// Results should be sorted by groupid.
	for i := 1; i < len(result); i++ {
		assert.Less(t, result[i-1].Groupid, result[i].Groupid, "Results should be sorted by groupid")
	}

	// Clean up.
	db.Exec("DELETE FROM messages_groups WHERE msgid IN (?, ?)", msgID1, msgID2)
	db.Exec("DELETE FROM messages WHERE id IN (?, ?)", msgID1, msgID2)
}

func TestGetGroupWork_AllFieldsPresent(t *testing.T) {
	// Verify the JSON response includes all expected fields.
	prefix := uniquePrefix("gwfields")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/work?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	body := rsp(resp)
	var rawResult []map[string]interface{}
	json2.Unmarshal(body, &rawResult)
	assert.GreaterOrEqual(t, len(rawResult), 1)

	// Find our group.
	var found map[string]interface{}
	for _, r := range rawResult {
		if uint64(r["groupid"].(float64)) == groupID {
			found = r
			break
		}
	}
	assert.NotNil(t, found, "Expected group %d in results", groupID)

	// Verify all 16 fields are present in JSON.
	expectedFields := []string{
		"groupid", "pending", "pendingother", "spam",
		"pendingmembers", "pendingmembersother", "spammembers", "spammembersother",
		"pendingevents", "pendingvolunteering", "editreview", "pendingadmins",
		"happiness", "relatedmembers", "chatreview", "chatreviewother",
	}
	for _, field := range expectedFields {
		_, ok := found[field]
		assert.True(t, ok, "Expected field %q in response", field)
	}
}

func TestGetGroupWork_PendingMembers(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("gwpendmem")

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create a pending member.
	pendingUserID := CreateTestUser(t, prefix+"_pending", "User")
	db.Exec("INSERT INTO memberships (userid, groupid, role, collection) VALUES (?, ?, 'Member', 'Pending')",
		pendingUserID, groupID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/work?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result []group.GroupWork
	json2.Unmarshal(rsp(resp), &result)

	var found *group.GroupWork
	for i := range result {
		if result[i].Groupid == groupID {
			found = &result[i]
			break
		}
	}
	assert.NotNil(t, found)
	assert.GreaterOrEqual(t, found.Pendingmembers, int64(1), "Expected pendingmembers >= 1")
}

func TestGetGroupWork_EditReview(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("gwedit")

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create a message with an edit needing review.
	senderID := CreateTestUser(t, prefix+"_sender", "User")
	var msgID uint64
	db.Exec("INSERT INTO messages (fromuser, type, subject) VALUES (?, 'Offer', 'Test edit review')", senderID)
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", senderID).Scan(&msgID)
	db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, deleted) VALUES (?, ?, 'Approved', 0)", msgID, groupID)
	db.Exec("INSERT INTO messages_edits (msgid, timestamp, reviewrequired, oldtext, newtext) VALUES (?, NOW(), 1, 'old', 'new')", msgID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/work?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result []group.GroupWork
	json2.Unmarshal(rsp(resp), &result)

	var found *group.GroupWork
	for i := range result {
		if result[i].Groupid == groupID {
			found = &result[i]
			break
		}
	}
	assert.NotNil(t, found)
	assert.GreaterOrEqual(t, found.Editreview, int64(1), "Expected editreview >= 1")

	// Clean up.
	db.Exec("DELETE FROM messages_edits WHERE msgid = ?", msgID)
	db.Exec("DELETE FROM messages_groups WHERE msgid = ?", msgID)
	db.Exec("DELETE FROM messages WHERE id = ?", msgID)
}

func TestGetGroupWork_OwnerRole(t *testing.T) {
	// Owners should also see work counts.
	prefix := uniquePrefix("gwowner")
	groupID := CreateTestGroup(t, prefix)
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	CreateTestMembership(t, ownerID, groupID, "Owner")
	_, token := CreateTestSession(t, ownerID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/work?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result []group.GroupWork
	json2.Unmarshal(rsp(resp), &result)

	var found *group.GroupWork
	for i := range result {
		if result[i].Groupid == groupID {
			found = &result[i]
			break
		}
	}
	assert.NotNil(t, found, "Owner should see group %d in work results", groupID)
	assert.Equal(t, groupID, found.Groupid)
}

func TestGetGroupWork_RegularMemberNoResults(t *testing.T) {
	// Regular members (not mod/owner) should not see work counts for that group.
	prefix := uniquePrefix("gwmember")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix+"_user", "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/work?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result []group.GroupWork
	json2.Unmarshal(rsp(resp), &result)

	// Regular member should not have this group in results.
	for _, r := range result {
		assert.NotEqual(t, groupID, r.Groupid, "Regular member should not see group work for %d", groupID)
	}
}

func TestGetGroupWork_PendingAdmins(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("gwadmin")

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	// Create a pending admin.
	db.Exec("INSERT INTO admins (groupid, pending, created) VALUES (?, 1, NOW())", groupID)
	var adminID uint64
	db.Raw("SELECT id FROM admins WHERE groupid = ? ORDER BY id DESC LIMIT 1", groupID).Scan(&adminID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/work?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result []group.GroupWork
	json2.Unmarshal(rsp(resp), &result)

	var found *group.GroupWork
	for i := range result {
		if result[i].Groupid == groupID {
			found = &result[i]
			break
		}
	}
	assert.NotNil(t, found)
	assert.GreaterOrEqual(t, found.Pendingadmins, int64(1), "Expected pendingadmins >= 1")

	// Clean up.
	if adminID > 0 {
		db.Exec("DELETE FROM admins WHERE id = ?", adminID)
	}
}

func TestGetGroupWork_PendingAdmins_BackupGroupIgnored(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("gwadmbk")

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")

	// Backup mod.
	db.Exec("INSERT INTO memberships (userid, groupid, role, settings) VALUES (?, ?, 'Moderator', ?)",
		modID, groupID, `{"active":0}`)
	_, token := CreateTestSession(t, modID)

	// Create a pending admin.
	db.Exec("INSERT INTO admins (groupid, pending, created) VALUES (?, 1, NOW())", groupID)
	var adminID uint64
	db.Raw("SELECT id FROM admins WHERE groupid = ? ORDER BY id DESC LIMIT 1", groupID).Scan(&adminID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/work?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result []group.GroupWork
	json2.Unmarshal(rsp(resp), &result)

	var found *group.GroupWork
	for i := range result {
		if result[i].Groupid == groupID {
			found = &result[i]
			break
		}
	}
	assert.NotNil(t, found)
	// Pending admins only counted for active groups.
	assert.Equal(t, int64(0), found.Pendingadmins, "Backup group should not count pending admins")

	// Clean up.
	if adminID > 0 {
		db.Exec("DELETE FROM admins WHERE id = ?", adminID)
	}
}

func TestGetGroupWork_DeletedMessageNotCounted(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("gwdelmsg")

	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, token := CreateTestSession(t, modID)

	senderID := CreateTestUser(t, prefix+"_sender", "User")
	var msgID uint64
	db.Exec("INSERT INTO messages (fromuser, type, subject, deleted) VALUES (?, 'Offer', 'Test deleted pending', NOW())", senderID)
	db.Raw("SELECT id FROM messages WHERE fromuser = ? ORDER BY id DESC LIMIT 1", senderID).Scan(&msgID)
	db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, deleted) VALUES (?, ?, 'Pending', 0)", msgID, groupID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/work?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result []group.GroupWork
	json2.Unmarshal(rsp(resp), &result)

	var found *group.GroupWork
	for i := range result {
		if result[i].Groupid == groupID {
			found = &result[i]
			break
		}
	}
	assert.NotNil(t, found, "Expected group %d in work results", groupID)
	assert.Equal(t, int64(0), found.Pending, "Deleted message should not be counted in pending")
	assert.Equal(t, int64(0), found.Pendingother, "Deleted message should not be counted in pendingother")

	// Clean up.
	db.Exec("DELETE FROM messages_groups WHERE msgid = ?", msgID)
	db.Exec("DELETE FROM messages WHERE id = ?", msgID)
}

func TestGetGroupWork_SortedByGroupid(t *testing.T) {
	prefix := uniquePrefix("gwsort")

	// Create 3 groups.
	gids := make([]uint64, 3)
	for i := 0; i < 3; i++ {
		gids[i] = CreateTestGroup(t, fmt.Sprintf("%s_%d", prefix, i))
	}

	modID := CreateTestUser(t, prefix+"_mod", "User")
	for _, gid := range gids {
		CreateTestMembership(t, modID, gid, "Moderator")
	}
	_, token := CreateTestSession(t, modID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group/work?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result []group.GroupWork
	json2.Unmarshal(rsp(resp), &result)

	// Verify sorted.
	for i := 1; i < len(result); i++ {
		assert.Less(t, result[i-1].Groupid, result[i].Groupid)
	}
}
