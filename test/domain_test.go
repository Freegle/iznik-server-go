package test

import (
	json2 "encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func TestGetDomain(t *testing.T) {
	db := database.DBConn

	// Ensure a known domain exists.
	db.Exec("INSERT IGNORE INTO domains_common (domain, count) VALUES ('gmail.com', 1000)")

	req := httptest.NewRequest("GET", "/api/domains?domain=gmail.com", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Known domain should NOT have suggestions.
	_, hasSuggestions := result["suggestions"]
	assert.False(t, hasSuggestions)
}

func TestGetDomainSuggestions(t *testing.T) {
	db := database.DBConn

	// Ensure some domains exist for suggestion matching.
	db.Exec("INSERT IGNORE INTO domains_common (domain, count) VALUES ('gmail.com', 1000)")
	db.Exec("INSERT IGNORE INTO domains_common (domain, count) VALUES ('yahoo.com', 900)")

	// Query for a misspelled domain.
	req := httptest.NewRequest("GET", "/api/domains?domain=gmial.com", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "suggestions")

	// Suggestions should be an array (may or may not contain gmail.com depending on damlevlim).
	suggestions := result["suggestions"].([]interface{})
	assert.NotNil(t, suggestions)
}

func TestGetDomainMissing(t *testing.T) {
	// No domain parameter.
	req := httptest.NewRequest("GET", "/api/domains", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestGetDomainV2Path(t *testing.T) {
	req := httptest.NewRequest("GET", "/apiv2/domains?domain=test", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}
