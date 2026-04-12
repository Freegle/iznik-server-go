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

// TestAIImageReview_TypesParameter verifies that the types query parameter is respected.
// When types=AIImageReview is specified, only AI image challenges should be returned
// (not CheckMessage or PhotoRotate even if available).
func TestAIImageReview_TypesParameter(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("mv_aitype")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	blockInviteChallenge(t, userID)

	imgID := createTestAIImage(t, "types-test-"+prefix, 42)

	// Request only AIImageReview type — should get it even though other types might have work.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/microvolunteering?jwt="+token+"&types=AIImageReview", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result microvolunteering.Challenge
	json2.Unmarshal(rsp(resp), &result)

	assert.Equal(t, microvolunteering.ChallengeAIImageReview, result.Type)
	assert.NotNil(t, result.AIImage)
	assert.Equal(t, imgID, result.AIImage.ID)

	t.Cleanup(func() {
		db.Exec("DELETE FROM microactions WHERE userid = ? AND actiontype = ?", userID, microvolunteering.ChallengeAIImageReview)
	})
}

// createTestAIImage inserts a test AI image and returns its ID. Cleanup is registered via t.Cleanup.
func createTestAIImage(t *testing.T, name string, usageCount int) uint64 {
	db := database.DBConn
	uid := "freegletusd-test-" + name

	db.Exec("INSERT INTO ai_images (name, externaluid, usage_count) VALUES (?, ?, ?)", name, uid, usageCount)

	var id uint64
	db.Raw("SELECT id FROM ai_images WHERE name = ? ORDER BY id DESC LIMIT 1", name).Scan(&id)
	assert.NotZero(t, id, "Failed to create test AI image")

	t.Cleanup(func() {
		db.Exec("DELETE FROM ai_images WHERE id = ?", id)
	})

	return id
}

// blockInviteChallenge prevents the invite challenge from being served, so we get to the AI image challenge.
func blockInviteChallenge(t *testing.T, userID uint64) {
	db := database.DBConn
	db.Exec("INSERT INTO microactions (actiontype, userid, version, comments, timestamp, result) VALUES (?, ?, 4, 'Test block', NOW(), 'Approve')",
		microvolunteering.ChallengeInvite, userID)

	t.Cleanup(func() {
		db.Exec("DELETE FROM microactions WHERE userid = ? AND actiontype = ?", userID, microvolunteering.ChallengeInvite)
	})
}

func TestAIImageReview_GetChallenge(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("mv_aiimg")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Block invite + ensure no group (so no CheckMessage/PhotoRotate challenges).
	blockInviteChallenge(t, userID)

	// Create a test AI image with high usage.
	imgID := createTestAIImage(t, "test-sofa-"+prefix, 100)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/microvolunteering?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result microvolunteering.Challenge
	json2.Unmarshal(rsp(resp), &result)

	assert.Equal(t, microvolunteering.ChallengeAIImageReview, result.Type)
	assert.NotNil(t, result.AIImage)
	assert.Equal(t, imgID, result.AIImage.ID)
	assert.Equal(t, "test-sofa-"+prefix, result.AIImage.Name)
	assert.Contains(t, result.AIImage.URL, "freegletusd-test-test-sofa-"+prefix)
	assert.Equal(t, uint64(100), result.AIImage.UsageCount)

	// Cleanup microactions created by the challenge (invite placeholder won't be created since we blocked it).
	t.Cleanup(func() {
		db.Exec("DELETE FROM microactions WHERE userid = ? AND actiontype = ?", userID, microvolunteering.ChallengeAIImageReview)
	})
}

func TestAIImageReview_UsageCountOrder(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("mv_aiord")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	blockInviteChallenge(t, userID)

	// Create two AI images — higher usage should be served first.
	createTestAIImage(t, "low-use-"+prefix, 5)
	highID := createTestAIImage(t, "high-use-"+prefix, 500)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/microvolunteering?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result microvolunteering.Challenge
	json2.Unmarshal(rsp(resp), &result)

	assert.Equal(t, microvolunteering.ChallengeAIImageReview, result.Type)
	assert.Equal(t, highID, result.AIImage.ID, "Higher usage image should be served first")

	t.Cleanup(func() {
		db.Exec("DELETE FROM microactions WHERE userid = ? AND actiontype = ?", userID, microvolunteering.ChallengeAIImageReview)
	})
}

func TestAIImageReview_PostResponse(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("mv_aipost")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	imgID := createTestAIImage(t, "test-chair-"+prefix, 50)

	body := fmt.Sprintf(`{"aiimageid":%d,"response":"Approve","containspeople":false}`, imgID)
	req := httptest.NewRequest("POST", "/api/microvolunteering?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify the microaction was recorded correctly.
	var actionType, actionResult string
	var containsPeople int
	db.Raw("SELECT actiontype, result, COALESCE(containspeople, -1) FROM microactions WHERE userid = ? AND aiimageid = ? ORDER BY id DESC LIMIT 1",
		userID, imgID).Row().Scan(&actionType, &actionResult, &containsPeople)
	assert.Equal(t, microvolunteering.ChallengeAIImageReview, actionType)
	assert.Equal(t, "Approve", actionResult)
	assert.Equal(t, 0, containsPeople, "containspeople should be false (0)")

	t.Cleanup(func() {
		db.Exec("DELETE FROM microactions WHERE userid = ? AND actiontype = ?", userID, microvolunteering.ChallengeAIImageReview)
	})
}

func TestAIImageReview_PostResponseWithPeople(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("mv_aipeople")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	imgID := createTestAIImage(t, "test-desk-"+prefix, 30)

	body := fmt.Sprintf(`{"aiimageid":%d,"response":"Reject","containspeople":true}`, imgID)
	req := httptest.NewRequest("POST", "/api/microvolunteering?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify.
	var containsPeople int
	var actionResult string
	db.Raw("SELECT result, COALESCE(containspeople, -1) FROM microactions WHERE userid = ? AND aiimageid = ? ORDER BY id DESC LIMIT 1",
		userID, imgID).Row().Scan(&actionResult, &containsPeople)
	assert.Equal(t, "Reject", actionResult)
	assert.Equal(t, 1, containsPeople, "containspeople should be true (1)")

	t.Cleanup(func() {
		db.Exec("DELETE FROM microactions WHERE userid = ? AND actiontype = ?", userID, microvolunteering.ChallengeAIImageReview)
	})
}

func TestAIImageReview_NoDuplicateVote(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("mv_aidedup")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	blockInviteChallenge(t, userID)

	imgID := createTestAIImage(t, "test-lamp-"+prefix, 20)

	// First vote.
	body := fmt.Sprintf(`{"aiimageid":%d,"response":"Approve","containspeople":false}`, imgID)
	req := httptest.NewRequest("POST", "/api/microvolunteering?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Second vote — should update, not create duplicate.
	body2 := fmt.Sprintf(`{"aiimageid":%d,"response":"Reject","containspeople":true}`, imgID)
	req2 := httptest.NewRequest("POST", "/api/microvolunteering?jwt="+token, strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := getApp().Test(req2)
	assert.Equal(t, 200, resp2.StatusCode)

	// Should have exactly one row.
	var count int64
	db.Raw("SELECT COUNT(*) FROM microactions WHERE userid = ? AND aiimageid = ?", userID, imgID).Scan(&count)
	assert.Equal(t, int64(1), count, "Should have exactly one vote per user per image")

	// Should be updated to Reject.
	var actionResult string
	db.Raw("SELECT result FROM microactions WHERE userid = ? AND aiimageid = ?", userID, imgID).Scan(&actionResult)
	assert.Equal(t, "Reject", actionResult)

	t.Cleanup(func() {
		db.Exec("DELETE FROM microactions WHERE userid = ? AND actiontype = ?", userID, microvolunteering.ChallengeAIImageReview)
	})
}

func TestAIImageReview_QuorumReached(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("mv_aiquor")

	// Create 6 users — 5 will vote, 1 will check that the image is no longer served.
	var voters []uint64
	var voterTokens []string
	for i := 0; i < 5; i++ {
		uid := CreateTestUser(t, fmt.Sprintf("%s_v%d", prefix, i), "User")
		_, tok := CreateTestSession(t, uid)
		voters = append(voters, uid)
		voterTokens = append(voterTokens, tok)
	}

	checkerID := CreateTestUser(t, prefix+"_checker", "User")
	_, checkerToken := CreateTestSession(t, checkerID)
	blockInviteChallenge(t, checkerID)

	imgID := createTestAIImage(t, "test-table-"+prefix, 200)

	// Have 5 users vote.
	for i := 0; i < 5; i++ {
		body := fmt.Sprintf(`{"aiimageid":%d,"response":"Approve","containspeople":false}`, imgID)
		req := httptest.NewRequest("POST", "/api/microvolunteering?jwt="+voterTokens[i], strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := getApp().Test(req)
		assert.Equal(t, 200, resp.StatusCode)
	}

	// Verify 5 votes recorded.
	var voteCount int64
	db.Raw("SELECT COUNT(*) FROM microactions WHERE aiimageid = ? AND actiontype = ?",
		imgID, microvolunteering.ChallengeAIImageReview).Scan(&voteCount)
	assert.Equal(t, int64(5), voteCount)

	// The checker should NOT get this image since quorum is reached.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/microvolunteering?jwt="+checkerToken, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	// Either no challenge or a different type — not this image.
	if typ, ok := result["type"]; ok {
		if typ == microvolunteering.ChallengeAIImageReview {
			aiimage := result["aiimage"].(map[string]interface{})
			assert.NotEqual(t, float64(imgID), aiimage["id"], "Image at quorum should not be served")
		}
	}

	t.Cleanup(func() {
		db.Exec("DELETE FROM microactions WHERE aiimageid = ? AND actiontype = ?", imgID, microvolunteering.ChallengeAIImageReview)
	})
}

func TestAIImageReview_SkipAlreadyReviewed(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("mv_aiskip")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	blockInviteChallenge(t, userID)

	// Create two images.
	reviewedID := createTestAIImage(t, "reviewed-"+prefix, 100)
	unreviewed := createTestAIImage(t, "unreviewed-"+prefix, 50)

	// User has already reviewed the first image.
	db.Exec("INSERT INTO microactions (actiontype, userid, aiimageid, result, version) VALUES (?, ?, ?, 'Approve', 4)",
		microvolunteering.ChallengeAIImageReview, userID, reviewedID)

	// Should get the unreviewed image.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/microvolunteering?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result microvolunteering.Challenge
	json2.Unmarshal(rsp(resp), &result)

	assert.Equal(t, microvolunteering.ChallengeAIImageReview, result.Type)
	assert.Equal(t, unreviewed, result.AIImage.ID, "Should skip already-reviewed image")

	t.Cleanup(func() {
		db.Exec("DELETE FROM microactions WHERE userid = ? AND actiontype = ?", userID, microvolunteering.ChallengeAIImageReview)
	})
}

// TestAIImageReview_RandomizationWithCheckMessage verifies that when both CheckMessage
// and AIImageReview have work, both types are served roughly 50% of the time.
func TestAIImageReview_RandomizationWithCheckMessage(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("mv_airand")

	// Create the reviewing user.
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)
	blockInviteChallenge(t, userID)

	// Create a sender (different user) and a group with microvolunteering.
	senderID := CreateTestUser(t, prefix+"_sender", "User")
	var groupID uint64
	db.Exec("INSERT INTO `groups` (nameshort, namefull, type, microvolunteering, polyindex) VALUES (?, ?, 'Freegle', 1, ST_GeomFromText('POINT(0 0)', 3857))",
		"testgroup-"+prefix, "Test Group "+prefix)
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&groupID)
	t.Cleanup(func() { db.Exec("DELETE FROM `groups` WHERE id = ?", groupID) })

	// Add reviewing user to the group.
	db.Exec("INSERT INTO memberships (userid, groupid) VALUES (?, ?)", userID, groupID)
	db.Exec("INSERT INTO memberships (userid, groupid) VALUES (?, ?)", senderID, groupID)
	t.Cleanup(func() {
		db.Exec("DELETE FROM memberships WHERE userid IN (?, ?) AND groupid = ?", userID, senderID, groupID)
	})

	// Create an approved message from sender (in spatial index, today).
	var msgID uint64
	db.Exec("INSERT INTO messages (fromuser, subject, type, arrival, lat, lng) VALUES (?, 'Test Offer', 'Offer', NOW(), 0, 0)", senderID)
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&msgID)
	t.Cleanup(func() { db.Exec("DELETE FROM messages WHERE id = ?", msgID) })

	db.Exec("INSERT INTO messages_groups (msgid, groupid, collection, arrival) VALUES (?, ?, 'Approved', NOW())", msgID, groupID)
	t.Cleanup(func() { db.Exec("DELETE FROM messages_groups WHERE msgid = ?", msgID) })

	db.Exec("INSERT INTO messages_spatial (msgid, groupid, point, arrival, successful) VALUES (?, ?, ST_GeomFromText('POINT(0 0)', 3857), NOW(), 0)", msgID, groupID)
	t.Cleanup(func() { db.Exec("DELETE FROM messages_spatial WHERE msgid = ?", msgID) })

	// Create an AI image.
	createTestAIImage(t, "rand-test-"+prefix, 100)

	// Call the endpoint 40 times and count how many of each type we get.
	// After each call, delete the microaction so the same challenge can be served again.
	checkCount := 0
	aiCount := 0

	for i := 0; i < 40; i++ {
		resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/microvolunteering?jwt="+token, nil))
		assert.Equal(t, 200, resp.StatusCode)

		var result microvolunteering.Challenge
		json2.Unmarshal(rsp(resp), &result)

		switch result.Type {
		case microvolunteering.ChallengeCheckMessage:
			checkCount++
		case microvolunteering.ChallengeAIImageReview:
			aiCount++
		}

		// Clean up so the same challenge is available next iteration.
		db.Exec("DELETE FROM microactions WHERE userid = ? AND (actiontype = ? OR actiontype = ?)",
			userID, microvolunteering.ChallengeCheckMessage, microvolunteering.ChallengeAIImageReview)
	}

	// With 40 trials at 50/50, expect each type at least 5 times (p < 0.0001 of failure).
	assert.GreaterOrEqual(t, aiCount, 5, "AIImageReview should be served at least 5/40 times, got %d", aiCount)
	assert.GreaterOrEqual(t, checkCount, 5, "CheckMessage should be served at least 5/40 times, got %d", checkCount)
	t.Logf("Distribution over 40 calls: CheckMessage=%d, AIImageReview=%d", checkCount, aiCount)
}
