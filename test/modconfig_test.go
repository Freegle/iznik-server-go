package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/log"
	"github.com/stretchr/testify/assert"
)

func createTestModConfig(t *testing.T, name string, createdby uint64) uint64 {
	db := database.DBConn
	result := db.Exec("INSERT INTO mod_configs (name, createdby) VALUES (?, ?)", name, createdby)
	assert.NoError(t, result.Error)

	var id uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)
	assert.Greater(t, id, uint64(0))
	return id
}

func TestGetModConfigSingle(t *testing.T) {
	prefix := uniquePrefix("ModCfg")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	cfgID := createTestModConfig(t, prefix+"_cfg", modID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/modtools/modconfig?id=%d&jwt=%s", cfgID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "config")

	cfg := result["config"].(map[string]interface{})
	assert.Equal(t, float64(cfgID), cfg["id"])
	assert.Contains(t, cfg, "stdmsgs")
}

func TestPostModConfig(t *testing.T) {
	prefix := uniquePrefix("ModCfgPost")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	body := fmt.Sprintf(`{"name":"%s_newcfg"}`, prefix)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/modtools/modconfig?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Greater(t, result["id"].(float64), float64(0))
}

func TestPostModConfigNotMod(t *testing.T) {
	prefix := uniquePrefix("ModCfgNM")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{"name":"ShouldFail"}`
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/modtools/modconfig?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(4), result["ret"])
}

func TestPatchModConfig(t *testing.T) {
	prefix := uniquePrefix("ModCfgPatch")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	cfgID := createTestModConfig(t, prefix+"_cfg", modID)

	body := fmt.Sprintf(`{"id":%d,"name":"%s_updated","subjlen":80}`, cfgID, prefix)
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/modtools/modconfig?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
}

func TestDeleteModConfig(t *testing.T) {
	prefix := uniquePrefix("ModCfgDel")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	cfgID := createTestModConfig(t, prefix+"_cfg", adminID)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/modtools/modconfig?id=%d&jwt=%s", cfgID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
}

func TestDeleteModConfigInUse(t *testing.T) {
	prefix := uniquePrefix("ModCfgDelUse")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Admin")
	_, token := CreateTestSession(t, modID)

	cfgID := createTestModConfig(t, prefix+"_cfg", modID)

	// Assign config to membership so it's in use.
	db := database.DBConn
	CreateTestMembership(t, modID, groupID, "Owner")
	db.Exec("UPDATE memberships SET configid = ? WHERE userid = ? AND groupid = ?", cfgID, modID, groupID)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/modtools/modconfig?id=%d&jwt=%s", cfgID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 409, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(5), result["ret"])
}

func TestPostModConfigCreatesLog(t *testing.T) {
	prefix := uniquePrefix("ModCfgLogC")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	body := fmt.Sprintf(`{"name":"%s_newcfg"}`, prefix)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/modtools/modconfig?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	newID := uint64(result["id"].(float64))
	assert.Greater(t, newID, uint64(0))

	// Verify a log entry was created with type='Config', subtype='Created'.
	db := database.DBConn
	var logCount int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE type = ? AND subtype = ? AND byuser = ? AND configid = ?",
		log.LOG_TYPE_CONFIG, log.LOG_SUBTYPE_CREATED, modID, newID).Scan(&logCount)
	assert.Equal(t, int64(1), logCount, "Expected a Config/Created log entry after POST")
}

func TestPatchModConfigCreatesLog(t *testing.T) {
	prefix := uniquePrefix("ModCfgLogE")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	cfgID := createTestModConfig(t, prefix+"_cfg", modID)

	body := fmt.Sprintf(`{"id":%d,"name":"%s_updated"}`, cfgID, prefix)
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/modtools/modconfig?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify a log entry was created with type='Config', subtype='Edit'.
	db := database.DBConn
	var logCount int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE type = ? AND subtype = ? AND byuser = ? AND configid = ?",
		log.LOG_TYPE_CONFIG, log.LOG_SUBTYPE_EDIT, modID, cfgID).Scan(&logCount)
	assert.Equal(t, int64(1), logCount, "Expected a Config/Edit log entry after PATCH")
}

func TestPatchModConfigProtectedSetsCreatedby(t *testing.T) {
	prefix := uniquePrefix("ModCfgProt")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	// Create config owned by someone else (unprotected, so modID can modify).
	otherID := CreateTestUser(t, prefix+"_other", "User")
	cfgID := createTestModConfig(t, prefix+"_cfg", otherID)

	// Assign it to a group modID moderates so they can see it.
	db := database.DBConn
	CreateTestMembership(t, otherID, groupID, "Moderator")
	db.Exec("UPDATE memberships SET configid = ? WHERE userid = ? AND groupid = ?", cfgID, otherID, groupID)

	// Set protected=1 — should also set createdby to the caller.
	body := fmt.Sprintf(`{"id":%d,"protected":1}`, cfgID)
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/modtools/modconfig?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var createdby uint64
	db.Raw("SELECT createdby FROM mod_configs WHERE id = ?", cfgID).Scan(&createdby)
	assert.Equal(t, modID, createdby, "Setting protected should update createdby to the caller")
}

func TestDeleteModConfigCreatesLog(t *testing.T) {
	prefix := uniquePrefix("ModCfgLogD")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	cfgID := createTestModConfig(t, prefix+"_cfg", adminID)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/modtools/modconfig?id=%d&jwt=%s", cfgID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify a log entry was created with type='Config', subtype='Deleted'.
	db := database.DBConn
	var logCount int64
	db.Raw("SELECT COUNT(*) FROM logs WHERE type = ? AND subtype = ? AND byuser = ? AND configid = ?",
		log.LOG_TYPE_CONFIG, log.LOG_SUBTYPE_DELETED, adminID, cfgID).Scan(&logCount)
	assert.Equal(t, int64(1), logCount, "Expected a Config/Deleted log entry after DELETE")
}

func TestPostModConfigCopyBulkops(t *testing.T) {
	prefix := uniquePrefix("ModCfgBulk")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	// Create a source config with bulkops.
	srcCfgID := createTestModConfig(t, prefix+"_src", modID)
	db := database.DBConn
	db.Exec("INSERT INTO mod_bulkops (title, configid, `set`, criterion, runevery, action, bouncingfor) VALUES (?, ?, 'Members', 'Bouncing', 168, 'Unbounce', 90)",
		prefix+"_bulkop1", srcCfgID)
	db.Exec("INSERT INTO mod_bulkops (title, configid, `set`, criterion, runevery, action, bouncingfor) VALUES (?, ?, 'Members', 'All', 24, 'Remove', 30)",
		prefix+"_bulkop2", srcCfgID)

	var srcBulkCount int64
	db.Raw("SELECT COUNT(*) FROM mod_bulkops WHERE configid = ?", srcCfgID).Scan(&srcBulkCount)
	assert.Equal(t, int64(2), srcBulkCount, "Source config should have 2 bulkops")

	// Copy the config.
	body := fmt.Sprintf(`{"name":"%s_copy","id":%d}`, prefix, srcCfgID)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/modtools/modconfig?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	newID := uint64(result["id"].(float64))
	assert.Greater(t, newID, uint64(0))

	// Verify bulkops were copied to the new config.
	var newBulkCount int64
	db.Raw("SELECT COUNT(*) FROM mod_bulkops WHERE configid = ?", newID).Scan(&newBulkCount)
	assert.Equal(t, int64(2), newBulkCount, "Copied config should have 2 bulkops")

	// Verify the source bulkops are still there (not moved).
	db.Raw("SELECT COUNT(*) FROM mod_bulkops WHERE configid = ?", srcCfgID).Scan(&srcBulkCount)
	assert.Equal(t, int64(2), srcBulkCount, "Source config should still have 2 bulkops")
}

func TestGetModConfigV2Path(t *testing.T) {
	prefix := uniquePrefix("ModCfgV2P")
	modID := CreateTestUser(t, prefix+"_mod", "Admin")
	_, token := CreateTestSession(t, modID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/apiv2/modtools/modconfig?id=0&jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}
