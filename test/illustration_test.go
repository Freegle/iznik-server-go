package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestIllustrationNoItem(t *testing.T) {
	// Test with no item parameter - should return error.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/illustration", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.IllustrationResult
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, 2, result.Ret)
	assert.Equal(t, "Item name required", result.Status)
}

func TestIllustrationEmptyItem(t *testing.T) {
	// Test with empty item parameter - should return error.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/illustration?item=", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.IllustrationResult
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, 2, result.Ret)
}

func TestIllustrationWhitespaceItem(t *testing.T) {
	// Test with whitespace only item parameter - should return error.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/illustration?item=%20%20%20", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.IllustrationResult
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, 2, result.Ret)
}

func TestIllustrationNotCached(t *testing.T) {
	// Test with an item that doesn't exist in cache - should return ret=3.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/illustration?item=NonexistentItem12345", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.IllustrationResult
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, 3, result.Ret)
	assert.Equal(t, "Not cached - use PHP API for generation", result.Status)
}

func TestIllustrationCached(t *testing.T) {
	// Insert a cached illustration directly into the database.
	testUid := fmt.Sprintf("test-uid-%d", time.Now().UnixNano())
	testItem := fmt.Sprintf("UTTest Sofa %d", time.Now().UnixNano())

	db := database.DBConn
	db.Exec("INSERT INTO ai_images (name, externaluid) VALUES (?, ?)", testItem, testUid)

	// Now request it - should return cached version.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/illustration?item="+url.QueryEscape(testItem), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.IllustrationResult
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, 0, result.Ret)
	assert.Equal(t, "Success", result.Status)
	assert.NotNil(t, result.Illustration)
	assert.Equal(t, testUid, result.Illustration.ExternalUID)
	assert.True(t, result.Illustration.Cached)

	// Clean up.
	db.Exec("DELETE FROM ai_images WHERE name = ?", testItem)
}

func TestIllustrationPrefixStripping(t *testing.T) {
	// Insert a cached illustration.
	testUid := fmt.Sprintf("test-uid-prefix-%d", time.Now().UnixNano())
	testItem := "Red Chair"

	db := database.DBConn
	db.Exec("INSERT INTO ai_images (name, externaluid) VALUES (?, ?)", testItem, testUid)

	// Request with OFFER: prefix - should strip it and find the cached item.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/illustration?item=OFFER:%20Red%20Chair", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.IllustrationResult
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, 0, result.Ret)
	assert.Equal(t, testUid, result.Illustration.ExternalUID)

	// Request with WANTED: prefix.
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/illustration?item=WANTED:%20Red%20Chair", nil))
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, 0, result.Ret)
	assert.Equal(t, testUid, result.Illustration.ExternalUID)

	// Clean up.
	db.Exec("DELETE FROM ai_images WHERE name = ?", testItem)
}

func TestIllustrationLocationSuffixStripping(t *testing.T) {
	// Insert a cached illustration.
	testUid := fmt.Sprintf("test-uid-location-%d", time.Now().UnixNano())
	testItem := "Blue Bike"

	db := database.DBConn
	db.Exec("INSERT INTO ai_images (name, externaluid) VALUES (?, ?)", testItem, testUid)

	// Request with location suffix in parentheses - should strip it.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/illustration?item=Blue%20Bike%20(London)", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.IllustrationResult
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, 0, result.Ret)
	assert.Equal(t, testUid, result.Illustration.ExternalUID)

	// Clean up.
	db.Exec("DELETE FROM ai_images WHERE name = ?", testItem)
}
