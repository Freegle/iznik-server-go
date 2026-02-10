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

func createTestStdMsg(t *testing.T, configID uint64, title string) uint64 {
	db := database.DBConn
	result := db.Exec("INSERT INTO mod_stdmsgs (configid, title) VALUES (?, ?)", configID, title)
	assert.NoError(t, result.Error)

	var id uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)
	assert.Greater(t, id, uint64(0))
	return id
}

func TestGetStdMsg(t *testing.T) {
	prefix := uniquePrefix("StdMsg")
	modID := CreateTestUser(t, prefix+"_mod", "User")

	cfgID := createTestModConfig(t, prefix+"_cfg", modID)
	msgID := createTestStdMsg(t, cfgID, prefix+"_msg")

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/stdmsg?id=%d", msgID), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "stdmsg")

	msg := result["stdmsg"].(map[string]interface{})
	assert.Equal(t, float64(msgID), msg["id"])
}

func TestPostStdMsg(t *testing.T) {
	prefix := uniquePrefix("StdMsgPost")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	cfgID := createTestModConfig(t, prefix+"_cfg", modID)

	body := fmt.Sprintf(`{"configid":%d,"title":"%s_newmsg"}`, cfgID, prefix)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/stdmsg?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Greater(t, result["id"].(float64), float64(0))
}

func TestPatchStdMsg(t *testing.T) {
	prefix := uniquePrefix("StdMsgPatch")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	cfgID := createTestModConfig(t, prefix+"_cfg", modID)
	msgID := createTestStdMsg(t, cfgID, prefix+"_msg")

	body := fmt.Sprintf(`{"id":%d,"title":"%s_updated","body":"New body text"}`, msgID, prefix)
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/stdmsg?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
}

func TestDeleteStdMsg(t *testing.T) {
	prefix := uniquePrefix("StdMsgDel")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	cfgID := createTestModConfig(t, prefix+"_cfg", modID)
	msgID := createTestStdMsg(t, cfgID, prefix+"_msg")

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/stdmsg?id=%d&jwt=%s", msgID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM mod_stdmsgs WHERE id = ?", msgID).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestPostStdMsgMissingTitle(t *testing.T) {
	prefix := uniquePrefix("StdMsgNoTitle")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	_, token := CreateTestSession(t, modID)

	cfgID := createTestModConfig(t, prefix+"_cfg", modID)

	body := fmt.Sprintf(`{"configid":%d}`, cfgID)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/stdmsg?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(3), result["ret"])
}

func TestGetStdMsgV2Path(t *testing.T) {
	req := httptest.NewRequest("GET", "/apiv2/stdmsg?id=0", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}
