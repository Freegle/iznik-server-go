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

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/modconfig?id=%d&jwt=%s", cfgID, token), nil)
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
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	body := fmt.Sprintf(`{"name":"%s_newcfg"}`, prefix)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/modconfig?jwt=%s", token), strings.NewReader(body))
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
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/modconfig?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

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
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/modconfig?jwt=%s", token), strings.NewReader(body))
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

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/modconfig?id=%d&jwt=%s", cfgID, token), nil)
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

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/modconfig?id=%d&jwt=%s", cfgID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(5), result["ret"])
}

func TestGetModConfigV2Path(t *testing.T) {
	req := httptest.NewRequest("GET", "/apiv2/modconfig?id=0", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}
