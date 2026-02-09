package test

import (
	json2 "encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/config"
	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

// Tests for GET /api/config/{key} endpoint

func TestConfigGet_ExistingKey(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("cfgget")

	// Create a config entry.
	key := "test_" + prefix
	db.Exec("INSERT INTO config (`key`, value) VALUES (?, ?)", key, "test_value_123")
	defer db.Exec("DELETE FROM config WHERE `key` = ?", key)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/config/"+key, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var results []config.ConfigItem
	json2.Unmarshal(rsp(resp), &results)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, key, results[0].Key)
	assert.Equal(t, "test_value_123", results[0].Value)
}

func TestConfigGet_NonExistentKey(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/config/nonexistent_key_xyz_999", nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Should return empty array, not null.
	body := rsp(resp)
	assert.Equal(t, "[]", string(body))
}

func TestConfigGet_V2Path(t *testing.T) {
	db := database.DBConn
	prefix := uniquePrefix("cfggetv2")

	key := "test_" + prefix
	db.Exec("INSERT INTO config (`key`, value) VALUES (?, ?)", key, "v2_value")
	defer db.Exec("DELETE FROM config WHERE `key` = ?", key)

	// Test that the /apiv2/ path also works.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/config/"+key, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var results []config.ConfigItem
	json2.Unmarshal(rsp(resp), &results)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, "v2_value", results[0].Value)
}

func TestConfigGet_NoAuthRequired(t *testing.T) {
	// Config get should work without authentication.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/config/some_key", nil))
	assert.Equal(t, 200, resp.StatusCode)
}
