package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

// createTestSocialAction creates a social action for testing and returns its ID
func createTestSocialAction(t *testing.T, userID uint64, groupID uint64, actionType string) uint64 {
	db := database.DBConn

	result := db.Exec("INSERT INTO socialactions (userid, groupid, action_type, created) VALUES (?, ?, ?, NOW())",
		userID, groupID, actionType)
	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create social action: %v", result.Error)
	}

	var id uint64
	db.Raw("SELECT id FROM socialactions WHERE userid = ? AND groupid = ? ORDER BY id DESC LIMIT 1",
		userID, groupID).Scan(&id)

	if id == 0 {
		t.Fatalf("ERROR: Social action was created but ID not found")
	}

	return id
}

// createTestSocialActionWithMsg creates a social action with a message reference
func createTestSocialActionWithMsg(t *testing.T, userID uint64, groupID uint64, msgID uint64, actionType string) uint64 {
	db := database.DBConn

	result := db.Exec("INSERT INTO socialactions (userid, groupid, msgid, action_type, created) VALUES (?, ?, ?, ?, NOW())",
		userID, groupID, msgID, actionType)
	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create social action with msg: %v", result.Error)
	}

	var id uint64
	db.Raw("SELECT id FROM socialactions WHERE userid = ? AND groupid = ? AND msgid = ? ORDER BY id DESC LIMIT 1",
		userID, groupID, msgID).Scan(&id)

	if id == 0 {
		t.Fatalf("ERROR: Social action was created but ID not found")
	}

	return id
}

func TestGetSocialActions(t *testing.T) {
	prefix := uniquePrefix("sa_get")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Create a user who posts the social action
	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")

	// Create a pending social action
	saID := createTestSocialAction(t, posterID, groupID, "share")

	// Mod should see pending social actions for their group
	resp, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/socialactions?jwt=%s", modToken), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result []map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	// Should find the social action we created
	found := false
	for _, sa := range result {
		if uint64(sa["id"].(float64)) == saID {
			found = true
			assert.Equal(t, float64(posterID), sa["userid"])
			assert.Equal(t, float64(groupID), sa["groupid"])
			assert.Equal(t, "share", sa["action_type"])
		}
	}
	assert.True(t, found, "Should find the created social action")
}

func TestGetSocialActionsWithGroupFilter(t *testing.T) {
	prefix := uniquePrefix("sa_getgf")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	group1ID := CreateTestGroup(t, prefix+"_g1")
	group2ID := CreateTestGroup(t, prefix+"_g2")
	CreateTestMembership(t, modID, group1ID, "Moderator")
	CreateTestMembership(t, modID, group2ID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, group1ID, "Member")
	CreateTestMembership(t, posterID, group2ID, "Member")

	// Create social actions in both groups
	createTestSocialAction(t, posterID, group1ID, "share")
	sa2ID := createTestSocialAction(t, posterID, group2ID, "share")

	// Filter by group2 only
	resp, _ := getApp().Test(httptest.NewRequest("GET",
		fmt.Sprintf("/api/socialactions?groupid=%d&jwt=%s", group2ID, modToken), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result []map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	// Should only find the social action in group2
	for _, sa := range result {
		assert.Equal(t, float64(group2ID), sa["groupid"])
	}

	found := false
	for _, sa := range result {
		if uint64(sa["id"].(float64)) == sa2ID {
			found = true
		}
	}
	assert.True(t, found, "Should find social action in filtered group")
}

func TestGetSocialActionsUnauthorized(t *testing.T) {
	// No auth token
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/socialactions", nil))
	assert.Equal(t, 401, resp.StatusCode)
}

func TestPostSocialActionDo(t *testing.T) {
	prefix := uniquePrefix("sa_do")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")

	saID := createTestSocialAction(t, posterID, groupID, "share")

	body := fmt.Sprintf(`{"id":%d,"uid":"12345","action":"Do"}`, saID)
	req := httptest.NewRequest("POST", "/api/socialactions?jwt="+modToken,
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify the action was marked as performed with uid
	db := database.DBConn
	var performed *string
	var uid *string
	db.Raw("SELECT performed, uid FROM socialactions WHERE id = ?", saID).Row().Scan(&performed, &uid)
	assert.NotNil(t, performed, "performed should be set")
	assert.NotNil(t, uid, "uid should be set")
	assert.Equal(t, "12345", *uid)
}

func TestPostSocialActionHide(t *testing.T) {
	prefix := uniquePrefix("sa_hide")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")

	saID := createTestSocialAction(t, posterID, groupID, "share")

	body := fmt.Sprintf(`{"id":%d,"uid":"99999","action":"Hide"}`, saID)
	req := httptest.NewRequest("POST", "/api/socialactions?jwt="+modToken,
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify the action was marked as performed (without uid update)
	db := database.DBConn
	var performed *string
	db.Raw("SELECT performed FROM socialactions WHERE id = ?", saID).Row().Scan(&performed)
	assert.NotNil(t, performed, "performed should be set")
}

func TestPostSocialActionNotMod(t *testing.T) {
	prefix := uniquePrefix("sa_notmod")
	userID := CreateTestUser(t, prefix+"_user", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")

	saID := createTestSocialAction(t, posterID, groupID, "share")

	body := fmt.Sprintf(`{"id":%d,"uid":"12345","action":"Do"}`, saID)
	req := httptest.NewRequest("POST", "/api/socialactions?jwt="+token,
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestPostSocialActionDoPopular(t *testing.T) {
	prefix := uniquePrefix("sa_dopop")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")

	msgID := CreateTestMessage(t, posterID, groupID, "Test popular post "+prefix, 55.9533, -3.1883)
	createTestSocialActionWithMsg(t, posterID, groupID, msgID, "popular")

	body := fmt.Sprintf(`{"groupid":%d,"msgid":%d,"uid":"67890","action":"DoPopular"}`, groupID, msgID)
	req := httptest.NewRequest("POST", "/api/socialactions?jwt="+modToken,
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify the popular action was marked as performed
	db := database.DBConn
	var performed *string
	var uid *string
	db.Raw("SELECT performed, uid FROM socialactions WHERE groupid = ? AND msgid = ? AND action_type = 'popular'",
		groupID, msgID).Row().Scan(&performed, &uid)
	assert.NotNil(t, performed, "performed should be set")
	assert.NotNil(t, uid, "uid should be set")
	assert.Equal(t, "67890", *uid)
}

func TestPostSocialActionHidePopular(t *testing.T) {
	prefix := uniquePrefix("sa_hidpop")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	posterID := CreateTestUser(t, prefix+"_poster", "User")
	CreateTestMembership(t, posterID, groupID, "Member")

	msgID := CreateTestMessage(t, posterID, groupID, "Test popular hide "+prefix, 55.9533, -3.1883)
	createTestSocialActionWithMsg(t, posterID, groupID, msgID, "popular")

	body := fmt.Sprintf(`{"groupid":%d,"msgid":%d,"action":"HidePopular"}`, groupID, msgID)
	req := httptest.NewRequest("POST", "/api/socialactions?jwt="+modToken,
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify the popular action was marked as performed
	db := database.DBConn
	var performed *string
	db.Raw("SELECT performed FROM socialactions WHERE groupid = ? AND msgid = ? AND action_type = 'popular'",
		groupID, msgID).Row().Scan(&performed)
	assert.NotNil(t, performed, "performed should be set")
}

func TestPostSocialActionUnauthorized(t *testing.T) {
	body := `{"id":1,"uid":"12345","action":"Do"}`
	req := httptest.NewRequest("POST", "/api/socialactions",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestPostSocialActionInvalidAction(t *testing.T) {
	prefix := uniquePrefix("sa_inval")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	body := `{"id":1,"action":"InvalidAction"}`
	req := httptest.NewRequest("POST", "/api/socialactions?jwt="+modToken,
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}
