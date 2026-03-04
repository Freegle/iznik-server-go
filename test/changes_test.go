package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
	"time"
)

func TestChangesNoPartnerKey(t *testing.T) {
	// Should reject requests without a partner key.
	req := httptest.NewRequest("GET", "/api/changes", nil)
	resp, err := getApp().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestChangesInvalidPartnerKey(t *testing.T) {
	// Should reject requests with an invalid partner key.
	req := httptest.NewRequest("GET", "/api/changes?partner=invalid_key_xyz", nil)
	resp, err := getApp().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestChangesValidPartner(t *testing.T) {
	prefix := uniquePrefix("changes")
	db := database.DBConn

	// Create a test partner key.
	partnerKey := prefix + "_key"
	db.Exec("INSERT INTO partners_keys (partner, `key`) VALUES (?, ?)", prefix+"_partner", partnerKey)
	defer db.Exec("DELETE FROM partners_keys WHERE partner = ?", prefix+"_partner")

	// Request changes with valid partner key.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/changes?partner=%s", partnerKey), nil)
	resp, err := getApp().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])

	changes, ok := result["changes"].(map[string]interface{})
	require.True(t, ok)

	// All three arrays should be present.
	assert.NotNil(t, changes["messages"])
	assert.NotNil(t, changes["users"])
	assert.NotNil(t, changes["ratings"])
}

func TestChangesWithSince(t *testing.T) {
	prefix := uniquePrefix("changes_since")
	db := database.DBConn

	// Create a test partner key.
	partnerKey := prefix + "_key"
	db.Exec("INSERT INTO partners_keys (partner, `key`) VALUES (?, ?)", prefix+"_partner", partnerKey)
	defer db.Exec("DELETE FROM partners_keys WHERE partner = ?", prefix+"_partner")

	// Request with a since parameter far in the future — should return empty results.
	futureTime := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/changes?partner=%s&since=%s", partnerKey, futureTime), nil)
	resp, err := getApp().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	changes := result["changes"].(map[string]interface{})
	messages := changes["messages"].([]interface{})
	users := changes["users"].([]interface{})
	ratings := changes["ratings"].([]interface{})
	assert.Equal(t, 0, len(messages))
	assert.Equal(t, 0, len(users))
	assert.Equal(t, 0, len(ratings))
}

func TestChangesMessageOutcome(t *testing.T) {
	prefix := uniquePrefix("changes_msg")
	db := database.DBConn

	// Create partner key.
	partnerKey := prefix + "_key"
	db.Exec("INSERT INTO partners_keys (partner, `key`) VALUES (?, ?)", prefix+"_partner", partnerKey)
	defer db.Exec("DELETE FROM partners_keys WHERE partner = ?", prefix+"_partner")

	// Create a test user, group, and message.
	groupID := CreateTestGroup(t, prefix)
	defer db.Exec("DELETE FROM `groups` WHERE id = ?", groupID)

	userID := CreateTestUser(t, prefix, "User")
	defer db.Exec("DELETE FROM users WHERE id = ?", userID)

	msgID := CreateTestMessage(t, userID, groupID, "OFFER: "+prefix+" test item", 55.95, -3.19)
	defer db.Exec("DELETE FROM messages WHERE id = ?", msgID)

	// Add a message outcome.
	db.Exec("INSERT INTO messages_outcomes (msgid, outcome, timestamp) VALUES (?, 'Taken', NOW())", msgID)
	defer db.Exec("DELETE FROM messages_outcomes WHERE msgid = ?", msgID)

	// Request changes — should include the outcome.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/changes?partner=%s", partnerKey), nil)
	resp, err := getApp().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	changes := result["changes"].(map[string]interface{})
	messages := changes["messages"].([]interface{})

	// Find our message in the results.
	found := false
	for _, m := range messages {
		msg := m.(map[string]interface{})
		if uint64(msg["id"].(float64)) == msgID {
			assert.Equal(t, "Taken", msg["type"])
			found = true
			break
		}
	}
	assert.True(t, found, "Expected message outcome in changes")
}

func TestChangesRatings(t *testing.T) {
	prefix := uniquePrefix("changes_rate")
	db := database.DBConn

	// Create partner key.
	partnerKey := prefix + "_key"
	db.Exec("INSERT INTO partners_keys (partner, `key`) VALUES (?, ?)", prefix+"_partner", partnerKey)
	defer db.Exec("DELETE FROM partners_keys WHERE partner = ?", prefix+"_partner")

	// Create two test users.
	raterID := CreateTestUser(t, prefix+"_rater", "User")
	defer db.Exec("DELETE FROM users WHERE id = ?", raterID)

	rateeID := CreateTestUser(t, prefix+"_ratee", "User")
	defer db.Exec("DELETE FROM users WHERE id = ?", rateeID)

	// Create a rating.
	db.Exec("INSERT INTO ratings (rater, ratee, rating, timestamp, visible) VALUES (?, ?, 'Up', NOW(), 1)", raterID, rateeID)
	defer db.Exec("DELETE FROM ratings WHERE rater = ? AND ratee = ?", raterID, rateeID)

	// Request changes — should include the rating.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/changes?partner=%s", partnerKey), nil)
	resp, err := getApp().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	changes := result["changes"].(map[string]interface{})
	ratings := changes["ratings"].([]interface{})

	found := false
	for _, r := range ratings {
		rating := r.(map[string]interface{})
		if uint64(rating["rater"].(float64)) == raterID && uint64(rating["ratee"].(float64)) == rateeID {
			assert.Equal(t, "Up", rating["rating"])
			found = true
			break
		}
	}
	assert.True(t, found, "Expected rating in changes")
}

func TestChangesInvalidSince(t *testing.T) {
	prefix := uniquePrefix("changes_bad")
	db := database.DBConn

	// Create partner key.
	partnerKey := prefix + "_key"
	db.Exec("INSERT INTO partners_keys (partner, `key`) VALUES (?, ?)", prefix+"_partner", partnerKey)
	defer db.Exec("DELETE FROM partners_keys WHERE partner = ?", prefix+"_partner")

	// Request with invalid since parameter.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/changes?partner=%s&since=not-a-date", partnerKey), nil)
	resp, err := getApp().Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}
