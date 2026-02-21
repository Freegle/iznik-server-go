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

func createTestSpammer(t *testing.T, userID uint64, collection string, reason string) uint64 {
	db := database.DBConn
	db.Exec("REPLACE INTO spam_users (userid, collection, reason, byuserid) VALUES (?, ?, ?, ?)",
		userID, collection, reason, userID)

	var id uint64
	db.Raw("SELECT id FROM spam_users WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&id)
	assert.Greater(t, id, uint64(0))
	return id
}

func TestGetSpammers(t *testing.T) {
	prefix := uniquePrefix("SpamGet")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	targetID := CreateTestUser(t, prefix+"_target", "User")
	createTestSpammer(t, targetID, "Spammer", "Test spam reason")

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/spammers?collection=Spammer&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "spammers")
	assert.Contains(t, result, "context")
}

func TestGetSpammersNotModerator(t *testing.T) {
	prefix := uniquePrefix("SpamNoMod")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/spammers?jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestPostSpammer(t *testing.T) {
	prefix := uniquePrefix("SpamPost")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Any user can report as PendingAdd.
	body := fmt.Sprintf(`{"userid":%d,"collection":"PendingAdd","reason":"Looks like spam"}`, targetID)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/spammers?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Greater(t, result["id"].(float64), float64(0))
}

func TestPostSpammerAdminOnly(t *testing.T) {
	prefix := uniquePrefix("SpamAdm")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	targetID := CreateTestUser(t, prefix+"_target", "User")

	// Regular user cannot add as Spammer (only PendingAdd).
	body := fmt.Sprintf(`{"userid":%d,"collection":"Spammer","reason":"Spam"}`, targetID)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/spammers?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestPatchSpammer(t *testing.T) {
	prefix := uniquePrefix("SpamPatch")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	targetID := CreateTestUser(t, prefix+"_target", "User")
	spamID := createTestSpammer(t, targetID, "PendingAdd", "Suspicious")

	body := fmt.Sprintf(`{"id":%d,"collection":"Spammer","reason":"Confirmed spam"}`, spamID)
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/spammers?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
}

func TestDeleteSpammer(t *testing.T) {
	prefix := uniquePrefix("SpamDel")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	targetID := CreateTestUser(t, prefix+"_target", "User")
	spamID := createTestSpammer(t, targetID, "Spammer", "Bad actor")

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/spammers?id=%d&jwt=%s", spamID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify deleted.
	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM spam_users WHERE id = ?", spamID).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestDeleteSpammerNotAdmin(t *testing.T) {
	prefix := uniquePrefix("SpamDelNA")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	targetID := CreateTestUser(t, prefix+"_target", "User")
	spamID := createTestSpammer(t, targetID, "Spammer", "Test")

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/spammers?id=%d&jwt=%s", spamID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestGetSpammersV2Path(t *testing.T) {
	req := httptest.NewRequest("GET", "/apiv2/spammers", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}
