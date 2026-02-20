package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/microvolunteering"
	"github.com/stretchr/testify/assert"
)

func TestMicroVolunteeringResponseCheckMessage(t *testing.T) {
	db := database.DBConn

	prefix := uniquePrefix("mv_checkmsg")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")

	// Create a message from a different user
	senderID := CreateTestUser(t, prefix+"_sender", "User")
	CreateTestMembership(t, senderID, groupID, "Member")
	msgID := CreateTestMessage(t, senderID, groupID, "Test MV Check "+prefix, 55.9533, -3.1883)

	body := fmt.Sprintf(`{"msgid":%d,"response":"Approve","comments":"Looks good"}`, msgID)
	req := httptest.NewRequest("POST", "/api/microvolunteering?jwt="+token,
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify the microaction was recorded
	var actionType string
	var actionResult string
	db.Raw("SELECT actiontype, result FROM microactions WHERE userid = ? AND msgid = ? ORDER BY id DESC LIMIT 1",
		userID, msgID).Row().Scan(&actionType, &actionResult)
	assert.Equal(t, microvolunteering.ChallengeCheckMessage, actionType)
	assert.Equal(t, "Approve", actionResult)
}

func TestMicroVolunteeringResponseSearchTerm(t *testing.T) {
	db := database.DBConn

	prefix := uniquePrefix("mv_search")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Create test items
	item1ID := CreateTestItem(t, "testitem1_"+prefix)
	item2ID := CreateTestItem(t, "testitem2_"+prefix)

	body := fmt.Sprintf(`{"searchterm1":%d,"searchterm2":%d}`, item1ID, item2ID)
	req := httptest.NewRequest("POST", "/api/microvolunteering?jwt="+token,
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify the microaction was recorded
	var actionType string
	db.Raw("SELECT actiontype FROM microactions WHERE userid = ? AND item1 = ? AND item2 = ? ORDER BY id DESC LIMIT 1",
		userID, item1ID, item2ID).Row().Scan(&actionType)
	assert.Equal(t, microvolunteering.ChallengeSearchTerm, actionType)
}

func TestMicroVolunteeringResponsePhotoRotate(t *testing.T) {
	db := database.DBConn

	prefix := uniquePrefix("mv_photo")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")

	// Create a message with an attachment
	senderID := CreateTestUser(t, prefix+"_sender", "User")
	CreateTestMembership(t, senderID, groupID, "Member")
	msgID := CreateTestMessage(t, senderID, groupID, "Test Photo "+prefix, 55.9533, -3.1883)
	photoID := CreateTestAttachment(t, msgID)

	body := fmt.Sprintf(`{"photoid":%d,"response":"Approve","deg":90}`, photoID)
	req := httptest.NewRequest("POST", "/api/microvolunteering?jwt="+token,
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify the microaction was recorded
	var actionType string
	db.Raw("SELECT actiontype FROM microactions WHERE userid = ? AND rotatedimage = ? ORDER BY id DESC LIMIT 1",
		userID, photoID).Row().Scan(&actionType)
	assert.Equal(t, microvolunteering.ChallengePhotoRotate, actionType)
}

func TestMicroVolunteeringResponseInvite(t *testing.T) {
	db := database.DBConn

	prefix := uniquePrefix("mv_invite")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{"invite":true,"response":"Yes"}`
	req := httptest.NewRequest("POST", "/api/microvolunteering?jwt="+token,
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify the microaction was recorded
	var actionType string
	var actionResult string
	db.Raw("SELECT actiontype, result FROM microactions WHERE userid = ? AND actiontype = ? ORDER BY id DESC LIMIT 1",
		userID, microvolunteering.ChallengeInvite).Row().Scan(&actionType, &actionResult)
	assert.Equal(t, microvolunteering.ChallengeInvite, actionType)
	assert.Equal(t, "Yes", actionResult)
}

func TestMicroVolunteeringResponseUnauthorized(t *testing.T) {
	body := `{"msgid":1,"response":"Approve"}`
	req := httptest.NewRequest("POST", "/api/microvolunteering",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestMicroVolunteeringResponseInvalidParams(t *testing.T) {
	prefix := uniquePrefix("mv_invalid")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Send empty body - no valid parameters
	body := `{}`
	req := httptest.NewRequest("POST", "/api/microvolunteering?jwt="+token,
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestMicroVolunteeringResponseFacebook(t *testing.T) {
	db := database.DBConn

	prefix := uniquePrefix("mv_fb")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{"facebook":12345,"response":"Shared"}`
	req := httptest.NewRequest("POST", "/api/microvolunteering?jwt="+token,
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify the microaction was recorded
	var actionType string
	db.Raw("SELECT actiontype FROM microactions WHERE userid = ? AND facebook_post = ? ORDER BY id DESC LIMIT 1",
		userID, 12345).Row().Scan(&actionType)
	assert.Equal(t, microvolunteering.ChallengeFacebookShare, actionType)
}
