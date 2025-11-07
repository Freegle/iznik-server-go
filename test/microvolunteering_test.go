package test

import (
	json2 "encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/microvolunteering"
	"github.com/stretchr/testify/assert"
)

func TestGetMicrovolunteering_NotLoggedIn(t *testing.T) {
	// Test without authentication - should return 401
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/microvolunteering", nil))
	assert.Equal(t, 401, resp.StatusCode)

	var result map[string]string
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, "Not logged in", result["error"])
}

func TestGetMicrovolunteering_NoChallenge(t *testing.T) {
	db := database.DBConn

	// Create a simple test user
	var userID uint64
	db.Exec("INSERT INTO users (firstname, lastname, systemrole) VALUES ('MVTest', 'User1', 'User')")
	db.Raw("SELECT id FROM users WHERE firstname = 'MVTest' AND lastname = 'User1' ORDER BY id DESC LIMIT 1").Scan(&userID)
	defer db.Exec("DELETE FROM users WHERE id = ?", userID)

	// Get JWT token for this user
	token := getToken(t, userID)

	// Ensure user has no group memberships
	db.Exec("DELETE FROM memberships WHERE userid = ?", userID)

	// Block invite challenge by adding a recent invite microaction
	db.Exec("INSERT INTO microactions (actiontype, userid, version, comments, timestamp) VALUES (?, ?, 4, 'Test block', NOW())", microvolunteering.ChallengeInvite, userID)
	defer db.Exec("DELETE FROM microactions WHERE userid = ? AND actiontype = ?", userID, microvolunteering.ChallengeInvite)

	// Make authenticated request
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/microvolunteering?jwt="+token, nil))

	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	// Should return empty object when no challenge available
	assert.NotContains(t, result, "type")
}

func TestGetMicrovolunteering_DeclinedUser(t *testing.T) {
	db := database.DBConn

	// Create a test user with Declined trust level
	var userID uint64
	db.Exec("INSERT INTO users (firstname, lastname, systemrole, trustlevel) VALUES ('MVTest', 'User3', 'User', ?)", microvolunteering.TrustDeclined)
	db.Raw("SELECT id FROM users WHERE firstname = 'MVTest' AND lastname = 'User3' ORDER BY id DESC LIMIT 1").Scan(&userID)
	defer db.Exec("DELETE FROM users WHERE id = ?", userID)

	// Get JWT token for this user
	token := getToken(t, userID)

	// Make authenticated request
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/microvolunteering?jwt="+token, nil))

	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	// Declined users should not get challenges
	assert.NotContains(t, result, "type")
}

func TestGetMicrovolunteering_ExcludedUser(t *testing.T) {
	db := database.DBConn

	// Create a test user with Excluded trust level
	var userID uint64
	db.Exec("INSERT INTO users (firstname, lastname, systemrole, trustlevel) VALUES ('MVTest', 'User4', 'User', ?)", microvolunteering.TrustExcluded)
	db.Raw("SELECT id FROM users WHERE firstname = 'MVTest' AND lastname = 'User4' ORDER BY id DESC LIMIT 1").Scan(&userID)
	defer db.Exec("DELETE FROM users WHERE id = ?", userID)

	// Get JWT token for this user
	token := getToken(t, userID)

	// Make authenticated request
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/microvolunteering?jwt="+token, nil))

	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	// Excluded users should not get challenges
	assert.NotContains(t, result, "type")
}

func TestGetMicrovolunteering_InviteChallenge(t *testing.T) {
	db := database.DBConn

	// Create a test user
	var userID uint64
	db.Exec("INSERT INTO users (firstname, lastname, systemrole) VALUES ('MVTest', 'User5', 'User')")
	db.Raw("SELECT id FROM users WHERE firstname = 'MVTest' AND lastname = 'User5' ORDER BY id DESC LIMIT 1").Scan(&userID)
	defer db.Exec("DELETE FROM users WHERE id = ?", userID)

	// Clean up any existing invite microactions for this user
	db.Exec("DELETE FROM microactions WHERE userid = ? AND actiontype = ?", userID, microvolunteering.ChallengeInvite)
	defer db.Exec("DELETE FROM microactions WHERE userid = ? AND actiontype = ?", userID, microvolunteering.ChallengeInvite)

	// Get JWT token for this user
	token := getToken(t, userID)

	// Make authenticated request
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/microvolunteering?jwt="+token, nil))

	assert.Equal(t, 200, resp.StatusCode)

	var result microvolunteering.Challenge
	json2.Unmarshal(rsp(resp), &result)

	// Should return invite challenge
	assert.Equal(t, microvolunteering.ChallengeInvite, result.Type)
}

func TestGetMicrovolunteering_CheckMessagePending(t *testing.T) {
	db := database.DBConn

	// Create a test user with Moderate trust level
	var userID uint64
	db.Exec("INSERT INTO users (firstname, lastname, systemrole, trustlevel) VALUES ('MVTest', 'User6', 'User', ?)", microvolunteering.TrustModerate)
	db.Raw("SELECT id FROM users WHERE firstname = 'MVTest' AND lastname = 'User6' ORDER BY id DESC LIMIT 1").Scan(&userID)
	defer db.Exec("DELETE FROM users WHERE id = ?", userID)

	// Block invite challenge by adding a recent invite microaction
	db.Exec("INSERT INTO microactions (actiontype, userid, version, comments, timestamp) VALUES (?, ?, 4, 'Test block', NOW())", microvolunteering.ChallengeInvite, userID)
	defer db.Exec("DELETE FROM microactions WHERE userid = ? AND actiontype = ?", userID, microvolunteering.ChallengeInvite)

	// Create another user to be the message sender
	var senderID uint64
	db.Exec("INSERT INTO users (firstname, lastname, systemrole) VALUES ('MVTest', 'Sender1', 'User')")
	db.Raw("SELECT id FROM users WHERE firstname = 'MVTest' AND lastname = 'Sender1' ORDER BY id DESC LIMIT 1").Scan(&senderID)
	defer db.Exec("DELETE FROM users WHERE id = ?", senderID)

	// Create a test group with microvolunteering enabled
	var groupID uint64
	db.Exec("INSERT INTO `groups` (nameshort, namefull, type, microvolunteering, polyindex) VALUES ('testgroup2', 'Test Group 2', 'Freegle', 1, ST_GeomFromText('POINT(0 0)', 3857))")
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&groupID)
	defer db.Exec("DELETE FROM `groups` WHERE id = ?", groupID)

	// Add both users to group
	db.Exec("INSERT INTO memberships (userid, groupid) VALUES (?, ?)", userID, groupID)
	db.Exec("INSERT INTO memberships (userid, groupid) VALUES (?, ?)", senderID, groupID)
	defer db.Exec("DELETE FROM memberships WHERE userid IN (?, ?) AND groupid = ?", userID, senderID, groupID)

	// Create a pending message from sender
	var msgID uint64
	db.Exec("INSERT INTO messages (fromuser, subject, type, arrival) VALUES (?, 'Test Offer', 'Offer', NOW())", senderID)
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&msgID)
	defer db.Exec("DELETE FROM messages WHERE id = ?", msgID)

	// Add message to group as pending
	db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, arrival) VALUES (?, ?, 'Pending', NOW())", msgID, groupID)
	defer db.Exec("DELETE FROM messages_groups WHERE msgid = ?", msgID)

	// Get JWT token for this user
	token := getToken(t, userID)

	// Make authenticated request
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/microvolunteering?jwt="+token, nil))

	assert.Equal(t, 200, resp.StatusCode)

	var result microvolunteering.Challenge
	json2.Unmarshal(rsp(resp), &result)

	// Should return check message challenge with the pending message
	assert.Equal(t, microvolunteering.ChallengeCheckMessage, result.Type)
	assert.NotNil(t, result.Msgid)
	assert.Equal(t, msgID, *result.Msgid)
}

func TestGetMicrovolunteering_CheckMessageApproved(t *testing.T) {
	db := database.DBConn

	// Create a test user (any trust level for approved messages)
	var userID uint64
	db.Exec("INSERT INTO users (firstname, lastname, systemrole) VALUES ('MVTest', 'User7', 'User')")
	db.Raw("SELECT id FROM users WHERE firstname = 'MVTest' AND lastname = 'User7' ORDER BY id DESC LIMIT 1").Scan(&userID)
	defer db.Exec("DELETE FROM users WHERE id = ?", userID)

	// Block invite challenge by adding a recent invite microaction
	db.Exec("INSERT INTO microactions (actiontype, userid, version, comments, timestamp) VALUES (?, ?, 4, 'Test block', NOW())", microvolunteering.ChallengeInvite, userID)
	defer db.Exec("DELETE FROM microactions WHERE userid = ? AND actiontype = ?", userID, microvolunteering.ChallengeInvite)

	// Create another user to be the message sender
	var senderID uint64
	db.Exec("INSERT INTO users (firstname, lastname, systemrole) VALUES ('MVTest', 'Sender2', 'User')")
	db.Raw("SELECT id FROM users WHERE firstname = 'MVTest' AND lastname = 'Sender2' ORDER BY id DESC LIMIT 1").Scan(&senderID)
	defer db.Exec("DELETE FROM users WHERE id = ?", senderID)

	// Create a test group with microvolunteering enabled
	var groupID uint64
	db.Exec("INSERT INTO `groups` (nameshort, namefull, type, microvolunteering, polyindex) VALUES ('testgroup3', 'Test Group 3', 'Freegle', 1, ST_GeomFromText('POINT(0 0)', 3857))")
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&groupID)
	defer db.Exec("DELETE FROM `groups` WHERE id = ?", groupID)

	// Add both users to group
	db.Exec("INSERT INTO memberships (userid, groupid) VALUES (?, ?)", userID, groupID)
	db.Exec("INSERT INTO memberships (userid, groupid) VALUES (?, ?)", senderID, groupID)
	defer db.Exec("DELETE FROM memberships WHERE userid IN (?, ?) AND groupid = ?", userID, senderID, groupID)

	// Create an approved message from sender
	var msgID uint64
	db.Exec("INSERT INTO messages (fromuser, subject, type, arrival, lat, lng) VALUES (?, 'Test Offer', 'Offer', NOW(), 0, 0)", senderID)
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&msgID)
	defer db.Exec("DELETE FROM messages WHERE id = ?", msgID)

	// Add message to group as approved
	db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, arrival) VALUES (?, ?, 'Approved', NOW())", msgID, groupID)
	defer db.Exec("DELETE FROM messages_groups WHERE msgid = ?", msgID)

	// Add message to spatial index (required for approved messages)
	db.Exec("INSERT INTO messages_spatial (msgid, groupid, point, arrival, successful) VALUES (?, ?, ST_GeomFromText('POINT(0 0)', 3857), NOW(), 0)", msgID, groupID)
	defer db.Exec("DELETE FROM messages_spatial WHERE msgid = ?", msgID)

	// Get JWT token for this user
	token := getToken(t, userID)

	// Make authenticated request
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/microvolunteering?jwt="+token, nil))

	assert.Equal(t, 200, resp.StatusCode)

	var result microvolunteering.Challenge
	json2.Unmarshal(rsp(resp), &result)

	// Should return check message challenge with the approved message
	assert.Equal(t, microvolunteering.ChallengeCheckMessage, result.Type)
	assert.NotNil(t, result.Msgid)
	assert.Equal(t, msgID, *result.Msgid)
}

func TestGetMicrovolunteering_PhotoRotateChallenge(t *testing.T) {
	db := database.DBConn

	// Create a test user
	var userID uint64
	db.Exec("INSERT INTO users (firstname, lastname, systemrole) VALUES ('MVTest', 'User8', 'User')")
	db.Raw("SELECT id FROM users WHERE firstname = 'MVTest' AND lastname = 'User8' ORDER BY id DESC LIMIT 1").Scan(&userID)
	defer db.Exec("DELETE FROM users WHERE id = ?", userID)

	// Block invite challenge by adding a recent invite microaction
	db.Exec("INSERT INTO microactions (actiontype, userid, version, comments, timestamp) VALUES (?, ?, 4, 'Test block', NOW())", microvolunteering.ChallengeInvite, userID)
	defer db.Exec("DELETE FROM microactions WHERE userid = ? AND actiontype = ?", userID, microvolunteering.ChallengeInvite)

	// Create another user to be the message sender
	var senderID uint64
	db.Exec("INSERT INTO users (firstname, lastname, systemrole) VALUES ('MVTest', 'Sender3', 'User')")
	db.Raw("SELECT id FROM users WHERE firstname = 'MVTest' AND lastname = 'Sender3' ORDER BY id DESC LIMIT 1").Scan(&senderID)
	defer db.Exec("DELETE FROM users WHERE id = ?", senderID)

	// Create a test group with microvolunteering and photo rotate enabled
	var groupID uint64
	db.Exec("INSERT INTO `groups` (nameshort, namefull, type, microvolunteering, polyindex) VALUES ('testgroup4', 'Test Group 4', 'Freegle', 1, ST_GeomFromText('POINT(0 0)', 3857))")
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&groupID)
	defer db.Exec("DELETE FROM `groups` WHERE id = ?", groupID)

	// Add both users to group
	db.Exec("INSERT INTO memberships (userid, groupid) VALUES (?, ?)", userID, groupID)
	db.Exec("INSERT INTO memberships (userid, groupid) VALUES (?, ?)", senderID, groupID)
	defer db.Exec("DELETE FROM memberships WHERE userid IN (?, ?) AND groupid = ?", userID, senderID, groupID)

	// Create a message with attachments
	var msgID uint64
	db.Exec("INSERT INTO messages (fromuser, subject, type, arrival) VALUES (?, 'Test Offer with Photos', 'Offer', NOW())", senderID)
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&msgID)
	defer db.Exec("DELETE FROM messages WHERE id = ?", msgID)

	// Add message to group
	db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, arrival) VALUES (?, ?, 'Approved', NOW())", msgID, groupID)
	defer db.Exec("DELETE FROM messages_groups WHERE msgid = ?", msgID)

	// Add photo attachments
	var photoID1, photoID2 uint64
	db.Exec("INSERT INTO messages_attachments (msgid, archived) VALUES (?, 0)", msgID)
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&photoID1)
	db.Exec("INSERT INTO messages_attachments (msgid, archived) VALUES (?, 0)", msgID)
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&photoID2)
	defer db.Exec("DELETE FROM messages_attachments WHERE msgid = ?", msgID)

	// Get JWT token for this user
	token := getToken(t, userID)

	// Make authenticated request
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/microvolunteering?jwt="+token, nil))

	assert.Equal(t, 200, resp.StatusCode)

	var result microvolunteering.Challenge
	json2.Unmarshal(rsp(resp), &result)

	// Should return photo rotate challenge
	assert.Equal(t, microvolunteering.ChallengePhotoRotate, result.Type)
	assert.NotEmpty(t, result.Photos)
	assert.LessOrEqual(t, len(result.Photos), 9)

	// Each photo should have an ID and path
	for _, photo := range result.Photos {
		assert.Greater(t, photo.ID, uint64(0))
		assert.NotEmpty(t, photo.Path)
	}
}
