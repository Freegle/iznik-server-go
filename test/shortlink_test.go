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

func createTestShortlink(t *testing.T, name string, groupID uint64) uint64 {
	db := database.DBConn
	result := db.Exec("INSERT INTO shortlinks (name, type, groupid) VALUES (?, 'Group', ?)", name, groupID)
	assert.NoError(t, result.Error)

	var id uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)
	return id
}

func TestGetShortlinkByID(t *testing.T) {
	prefix := uniquePrefix("Shortlink")
	groupID := CreateTestGroup(t, prefix)
	slID := createTestShortlink(t, prefix+"_link", groupID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/shortlink?id=%d", slID), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	sl := result["shortlink"].(map[string]interface{})
	assert.Equal(t, float64(slID), sl["id"])
	assert.Equal(t, prefix+"_link", sl["name"])
	assert.Equal(t, "Group", sl["type"])
	assert.Contains(t, sl, "clickhistory")
}

func TestGetShortlinkList(t *testing.T) {
	prefix := uniquePrefix("ShortlinkList")
	groupID := CreateTestGroup(t, prefix)
	createTestShortlink(t, prefix+"_a", groupID)
	createTestShortlink(t, prefix+"_b", groupID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/shortlink?groupid=%d", groupID), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	links := result["shortlinks"].([]interface{})
	assert.GreaterOrEqual(t, len(links), 2)
}

func TestPostShortlink(t *testing.T) {
	prefix := uniquePrefix("ShortlinkPost")
	groupID := CreateTestGroup(t, prefix)

	body := fmt.Sprintf(`{"name":"%s_newlink","groupid":%d}`, prefix, groupID)
	req := httptest.NewRequest("POST", "/api/shortlink", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Greater(t, result["id"].(float64), float64(0))
}

func TestPostShortlinkDuplicate(t *testing.T) {
	prefix := uniquePrefix("ShortlinkDup")
	groupID := CreateTestGroup(t, prefix)
	createTestShortlink(t, prefix+"_dup", groupID)

	body := fmt.Sprintf(`{"name":"%s_dup","groupid":%d}`, prefix, groupID)
	req := httptest.NewRequest("POST", "/api/shortlink", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(3), result["ret"])
	assert.Equal(t, "Name already in use", result["status"])
}

func TestPostShortlinkMissingParams(t *testing.T) {
	body := `{"name":"","groupid":0}`
	req := httptest.NewRequest("POST", "/api/shortlink", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestGetShortlinkV2Path(t *testing.T) {
	req := httptest.NewRequest("GET", "/apiv2/shortlink", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}
