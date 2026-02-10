package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func TestGetABTestNoUID(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/abtest", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestGetABTestEmpty(t *testing.T) {
	prefix := uniquePrefix("ab_empty")
	uid := prefix + "_nonexistent"

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/abtest?uid=%s", uid), nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Nil(t, result["variant"])
}

func TestPostABTestShown(t *testing.T) {
	prefix := uniquePrefix("ab_shown")
	db := database.DBConn
	uid := prefix + "_test"
	variant := "variantA"

	body, _ := json.Marshal(map[string]interface{}{
		"uid":     uid,
		"variant": variant,
		"shown":   true,
	})
	req := httptest.NewRequest("POST", "/api/abtest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify in DB
	var shown uint64
	db.Raw("SELECT shown FROM abtest WHERE uid = ? AND variant = ?", uid, variant).Scan(&shown)
	assert.Equal(t, uint64(1), shown)

	// Post again - shown should increment
	req = httptest.NewRequest("POST", "/api/abtest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err = getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	db.Raw("SELECT shown FROM abtest WHERE uid = ? AND variant = ?", uid, variant).Scan(&shown)
	assert.Equal(t, uint64(2), shown)
}

func TestPostABTestAction(t *testing.T) {
	prefix := uniquePrefix("ab_action")
	db := database.DBConn
	uid := prefix + "_test"
	variant := "variantB"

	// First show it
	body, _ := json.Marshal(map[string]interface{}{
		"uid":     uid,
		"variant": variant,
		"shown":   true,
	})
	req := httptest.NewRequest("POST", "/api/abtest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	getApp().Test(req)

	// Then record action
	body, _ = json.Marshal(map[string]interface{}{
		"uid":     uid,
		"variant": variant,
		"action":  true,
	})
	req = httptest.NewRequest("POST", "/api/abtest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var action uint64
	var rate float64
	db.Raw("SELECT action, rate FROM abtest WHERE uid = ? AND variant = ?", uid, variant).Row().Scan(&action, &rate)
	assert.Equal(t, uint64(1), action)
	assert.InDelta(t, 100.0, rate, 0.01) // 1 action / 1 shown = 100%
}

func TestPostABTestActionWithScore(t *testing.T) {
	prefix := uniquePrefix("ab_score")
	db := database.DBConn
	uid := prefix + "_test"
	variant := "variantC"

	// Show first
	body, _ := json.Marshal(map[string]interface{}{
		"uid":     uid,
		"variant": variant,
		"shown":   true,
	})
	req := httptest.NewRequest("POST", "/api/abtest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	getApp().Test(req)

	// Action with score=5
	body, _ = json.Marshal(map[string]interface{}{
		"uid":     uid,
		"variant": variant,
		"action":  true,
		"score":   5,
	})
	req = httptest.NewRequest("POST", "/api/abtest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var action uint64
	db.Raw("SELECT action FROM abtest WHERE uid = ? AND variant = ?", uid, variant).Scan(&action)
	assert.Equal(t, uint64(5), action)
}

func TestPostABTestIgnoreApp(t *testing.T) {
	prefix := uniquePrefix("ab_app")
	db := database.DBConn
	uid := prefix + "_test"
	variant := "variantD"

	body, _ := json.Marshal(map[string]interface{}{
		"uid":     uid,
		"variant": variant,
		"shown":   true,
		"app":     true,
	})
	req := httptest.NewRequest("POST", "/api/abtest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Should NOT be recorded in DB
	var count int64
	db.Raw("SELECT COUNT(*) FROM abtest WHERE uid = ? AND variant = ?", uid, variant).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestPostABTestMissingFields(t *testing.T) {
	// Missing uid/variant should succeed but do nothing
	body, _ := json.Marshal(map[string]interface{}{
		"shown": true,
	})
	req := httptest.NewRequest("POST", "/api/abtest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestPostABTestInvalidJSON(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/abtest", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPostABTestActionWithoutShown(t *testing.T) {
	// Action on a uid/variant that was never shown - should still succeed (INSERT)
	prefix := uniquePrefix("ab_noshown")
	uid := prefix + "_test"
	variant := "variantX"

	body, _ := json.Marshal(map[string]interface{}{
		"uid":     uid,
		"variant": variant,
		"action":  true,
	})
	req := httptest.NewRequest("POST", "/api/abtest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify record created with shown=0, action=1
	db := database.DBConn
	var shown, action uint64
	db.Raw("SELECT shown, action FROM abtest WHERE uid = ? AND variant = ?", uid, variant).Row().Scan(&shown, &action)
	assert.Equal(t, uint64(0), shown)
	assert.Equal(t, uint64(1), action)
}

func TestPostABTestEmptyBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/abtest", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	// Empty body returns success (handler treats missing uid/variant as no-op)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetABTestReturnsVariant(t *testing.T) {
	prefix := uniquePrefix("ab_get")
	db := database.DBConn
	uid := prefix + "_test"

	// Insert test data directly
	db.Exec("INSERT INTO abtest (uid, variant, shown, action, rate, suggest) VALUES (?, 'best', 100, 90, 90.0, 1)", uid)
	db.Exec("INSERT INTO abtest (uid, variant, shown, action, rate, suggest) VALUES (?, 'worst', 100, 10, 10.0, 1)", uid)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/abtest?uid=%s", uid), nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(0), result["ret"])
	assert.NotNil(t, result["variant"])

	// Variant should be a map with uid field
	v, ok := result["variant"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, uid, v["uid"])
}
