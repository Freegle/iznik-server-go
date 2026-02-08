package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestCommentCreate(t *testing.T) {
	prefix := uniquePrefix("cmwr_create")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, modToken := CreateTestSession(t, modID)

	body := fmt.Sprintf(`{"userid":%d,"groupid":%d,"user1":"Test comment","user2":"More info","flag":false}`, targetID, groupID)
	req := httptest.NewRequest("POST", "/api/comment?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Greater(t, result["id"], float64(0))

	// Cleanup
	db := database.DBConn
	db.Exec("DELETE FROM users_comments WHERE id = ?", int(result["id"].(float64)))
}

func TestCommentCreateUnauthorized(t *testing.T) {
	body := `{"userid":1,"groupid":1,"user1":"Test"}`
	req := httptest.NewRequest("POST", "/api/comment", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestCommentCreateNotModerator(t *testing.T) {
	prefix := uniquePrefix("cmwr_notmod")
	userID := CreateTestUser(t, prefix+"_user", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	body := fmt.Sprintf(`{"userid":%d,"groupid":%d,"user1":"Test"}`, targetID, groupID)
	req := httptest.NewRequest("POST", "/api/comment?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestCommentCreateBySupport(t *testing.T) {
	prefix := uniquePrefix("cmwr_support")
	supportID := CreateTestUser(t, prefix+"_support", "Support")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	_, supportToken := CreateTestSession(t, supportID)

	// Support can add comment without a group
	body := fmt.Sprintf(`{"userid":%d,"user1":"Support comment"}`, targetID)
	req := httptest.NewRequest("POST", "/api/comment?jwt="+supportToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Greater(t, result["id"], float64(0))

	// Cleanup
	db := database.DBConn
	db.Exec("DELETE FROM users_comments WHERE id = ?", int(result["id"].(float64)))
}

func TestCommentCreateWithFlag(t *testing.T) {
	prefix := uniquePrefix("cmwr_flag")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, modToken := CreateTestSession(t, modID)

	body := fmt.Sprintf(`{"userid":%d,"groupid":%d,"user1":"Flagged comment","flag":true}`, targetID, groupID)
	req := httptest.NewRequest("POST", "/api/comment?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	commentID := int(result["id"].(float64))
	assert.Greater(t, commentID, 0)

	// Verify flag was set in DB
	db := database.DBConn
	var flag int
	db.Raw("SELECT flag FROM users_comments WHERE id = ?", commentID).Scan(&flag)
	assert.Equal(t, 1, flag)

	// Cleanup
	db.Exec("DELETE FROM users_comments WHERE id = ?", commentID)
}

func TestCommentEdit(t *testing.T) {
	prefix := uniquePrefix("cmwr_edit")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, modToken := CreateTestSession(t, modID)

	// Create a comment first
	db := database.DBConn
	db.Exec("INSERT INTO users_comments (userid, groupid, byuserid, user1, flag) VALUES (?, ?, ?, 'Original', 0)", targetID, groupID, modID)
	var commentID uint64
	db.Raw("SELECT id FROM users_comments WHERE userid = ? AND groupid = ? ORDER BY id DESC LIMIT 1", targetID, groupID).Scan(&commentID)

	body := fmt.Sprintf(`{"id":%d,"user1":"Updated comment","user2":"New field","flag":false}`, commentID)
	req := httptest.NewRequest("PATCH", "/api/comment?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify updated in DB
	var user1 string
	db.Raw("SELECT user1 FROM users_comments WHERE id = ?", commentID).Scan(&user1)
	assert.Equal(t, "Updated comment", user1)

	// Cleanup
	db.Exec("DELETE FROM users_comments WHERE id = ?", commentID)
}

func TestCommentEditUnauthorized(t *testing.T) {
	body := `{"id":1,"user1":"Hacked"}`
	req := httptest.NewRequest("PATCH", "/api/comment", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestCommentEditNotModerator(t *testing.T) {
	prefix := uniquePrefix("cmwr_editnm")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	userID := CreateTestUser(t, prefix+"_user", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, userID, groupID, "Member")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, userToken := CreateTestSession(t, userID)

	// Create a comment as mod
	db := database.DBConn
	db.Exec("INSERT INTO users_comments (userid, groupid, byuserid, user1, flag) VALUES (?, ?, ?, 'Mod comment', 0)", targetID, groupID, modID)
	var commentID uint64
	db.Raw("SELECT id FROM users_comments WHERE userid = ? AND groupid = ? ORDER BY id DESC LIMIT 1", targetID, groupID).Scan(&commentID)

	body := fmt.Sprintf(`{"id":%d,"user1":"Hacked"}`, commentID)
	req := httptest.NewRequest("PATCH", "/api/comment?jwt="+userToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)

	// Cleanup
	db.Exec("DELETE FROM users_comments WHERE id = ?", commentID)
}

func TestCommentDelete(t *testing.T) {
	prefix := uniquePrefix("cmwr_del")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, modToken := CreateTestSession(t, modID)

	// Create a comment first
	db := database.DBConn
	db.Exec("INSERT INTO users_comments (userid, groupid, byuserid, user1, flag) VALUES (?, ?, ?, 'To delete', 0)", targetID, groupID, modID)
	var commentID uint64
	db.Raw("SELECT id FROM users_comments WHERE userid = ? AND groupid = ? ORDER BY id DESC LIMIT 1", targetID, groupID).Scan(&commentID)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/comment/%d?jwt=%s", commentID, modToken), nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Verify deleted from DB (real delete, not soft delete)
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_comments WHERE id = ?", commentID).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestCommentDeleteUnauthorized(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("DELETE", "/api/comment/1", nil))
	assert.Equal(t, 401, resp.StatusCode)
}

func TestCommentDeleteNotModerator(t *testing.T) {
	prefix := uniquePrefix("cmwr_delnm")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	userID := CreateTestUser(t, prefix+"_user", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, userID, groupID, "Member")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, userToken := CreateTestSession(t, userID)

	// Create a comment as mod
	db := database.DBConn
	db.Exec("INSERT INTO users_comments (userid, groupid, byuserid, user1, flag) VALUES (?, ?, ?, 'Mod only', 0)", targetID, groupID, modID)
	var commentID uint64
	db.Raw("SELECT id FROM users_comments WHERE userid = ? AND groupid = ? ORDER BY id DESC LIMIT 1", targetID, groupID).Scan(&commentID)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/comment/%d?jwt=%s", commentID, userToken), nil))
	assert.Equal(t, 403, resp.StatusCode)

	// Cleanup
	db.Exec("DELETE FROM users_comments WHERE id = ?", commentID)
}

func TestCommentCreateWithFlagOthers(t *testing.T) {
	prefix := uniquePrefix("cmwr_flago")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	group1ID := CreateTestGroup(t, prefix+"_g1")
	group2ID := CreateTestGroup(t, prefix+"_g2")
	CreateTestMembership(t, modID, group1ID, "Moderator")
	CreateTestMembership(t, targetID, group1ID, "Member")
	CreateTestMembership(t, targetID, group2ID, "Member")
	_, modToken := CreateTestSession(t, modID)

	// Clear any existing review state
	db := database.DBConn
	db.Exec("UPDATE memberships SET reviewreason = NULL, reviewrequestedat = NULL WHERE userid = ? AND groupid = ?", targetID, group2ID)

	// Create a flagged comment on group1
	body := fmt.Sprintf(`{"userid":%d,"groupid":%d,"user1":"Flagged user","flag":true}`, targetID, group1ID)
	req := httptest.NewRequest("POST", "/api/comment?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	commentID := int(result["id"].(float64))

	// Verify flagOthers: group2 membership should have reviewreason set
	var reviewreason *string
	db.Raw("SELECT reviewreason FROM memberships WHERE userid = ? AND groupid = ?", targetID, group2ID).Scan(&reviewreason)
	assert.NotNil(t, reviewreason)
	assert.Equal(t, "Note flagged to other groups", *reviewreason)

	// group1 should NOT have reviewreason set (it's the source group)
	var reviewreason1 *string
	db.Raw("SELECT reviewreason FROM memberships WHERE userid = ? AND groupid = ?", targetID, group1ID).Scan(&reviewreason1)
	assert.Nil(t, reviewreason1)

	// Cleanup
	db.Exec("DELETE FROM users_comments WHERE id = ?", commentID)
	db.Exec("UPDATE memberships SET reviewreason = NULL, reviewrequestedat = NULL WHERE userid = ?", targetID)
}

func TestCommentEditWithFlagOthers(t *testing.T) {
	prefix := uniquePrefix("cmwr_eflg")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	group1ID := CreateTestGroup(t, prefix+"_g1")
	group2ID := CreateTestGroup(t, prefix+"_g2")
	CreateTestMembership(t, modID, group1ID, "Moderator")
	CreateTestMembership(t, targetID, group1ID, "Member")
	CreateTestMembership(t, targetID, group2ID, "Member")
	_, modToken := CreateTestSession(t, modID)

	// Create an unflagged comment
	db := database.DBConn
	db.Exec("INSERT INTO users_comments (userid, groupid, byuserid, user1, flag) VALUES (?, ?, ?, 'Original', 0)", targetID, group1ID, modID)
	var commentID uint64
	db.Raw("SELECT id FROM users_comments WHERE userid = ? AND groupid = ? ORDER BY id DESC LIMIT 1", targetID, group1ID).Scan(&commentID)

	// Clear any existing review state
	db.Exec("UPDATE memberships SET reviewreason = NULL, reviewrequestedat = NULL WHERE userid = ? AND groupid = ?", targetID, group2ID)

	// Edit with flag=true
	body := fmt.Sprintf(`{"id":%d,"user1":"Now flagged","flag":true}`, commentID)
	req := httptest.NewRequest("PATCH", "/api/comment?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify flagOthers: group2 membership should have reviewreason set
	var reviewreason *string
	db.Raw("SELECT reviewreason FROM memberships WHERE userid = ? AND groupid = ?", targetID, group2ID).Scan(&reviewreason)
	assert.NotNil(t, reviewreason)
	assert.Equal(t, "Note flagged to other groups", *reviewreason)

	// Cleanup
	db.Exec("DELETE FROM users_comments WHERE id = ?", commentID)
	db.Exec("UPDATE memberships SET reviewreason = NULL, reviewrequestedat = NULL WHERE userid = ?", targetID)
}

func TestCommentDeleteByAdmin(t *testing.T) {
	prefix := uniquePrefix("cmwr_deladm")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	targetID := CreateTestUser(t, prefix+"_target", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	CreateTestMembership(t, targetID, groupID, "Member")
	_, adminToken := CreateTestSession(t, adminID)

	// Create a comment as mod
	db := database.DBConn
	db.Exec("INSERT INTO users_comments (userid, groupid, byuserid, user1, flag) VALUES (?, ?, ?, 'Admin can delete', 0)", targetID, groupID, modID)
	var commentID uint64
	db.Raw("SELECT id FROM users_comments WHERE userid = ? AND groupid = ? ORDER BY id DESC LIMIT 1", targetID, groupID).Scan(&commentID)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/comment/%d?jwt=%s", commentID, adminToken), nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Verify deleted
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_comments WHERE id = ?", commentID).Scan(&count)
	assert.Equal(t, int64(0), count)
}
