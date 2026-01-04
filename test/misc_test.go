package test

import (
	json2 "encoding/json"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestMisc(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/online", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.OnlineResult

	json2.Unmarshal(rsp(resp), &result)
	assert.True(t, result.Online)
}

func TestLatestMessage(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/latestmessage", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.LatestMessageResult

	json2.Unmarshal(rsp(resp), &result)

	// In test environment, messages table may be empty, so we accept either success or "No messages found"
	if result.Ret == 0 {
		assert.Equal(t, "Success", result.Status)
		assert.NotEmpty(t, result.LatestMessage)
	} else {
		assert.Equal(t, 1, result.Ret)
		assert.Equal(t, "No messages found", result.Status)
	}
}

func TestIllustrationNoItem(t *testing.T) {
	// Test without item parameter - should return error.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/illustration", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.IllustrationResult
	json2.Unmarshal(rsp(resp), &result)

	assert.Equal(t, 2, result.Ret, "Should return ret=2 for missing item")
	assert.Equal(t, "Item name required", result.Status)
}

func TestIllustrationEmptyItem(t *testing.T) {
	// Test with empty item parameter.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/illustration?item=", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.IllustrationResult
	json2.Unmarshal(rsp(resp), &result)

	assert.Equal(t, 2, result.Ret, "Should return ret=2 for empty item")
}

func TestIllustrationWhitespaceItem(t *testing.T) {
	// Test with whitespace-only item parameter.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/illustration?item=%20%20", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.IllustrationResult
	json2.Unmarshal(rsp(resp), &result)

	assert.Equal(t, 2, result.Ret, "Should return ret=2 for whitespace item")
}

func TestIllustrationNotCached(t *testing.T) {
	// Test with item that's not in cache - should return ret=3.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/illustration?item=uniqueItemNotInCache12345", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.IllustrationResult
	json2.Unmarshal(rsp(resp), &result)

	assert.Equal(t, 3, result.Ret, "Should return ret=3 for uncached item")
	assert.Contains(t, result.Status, "Not cached")
}

func TestIllustrationWithOfferPrefix(t *testing.T) {
	// Test that OFFER: prefix is stripped.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/illustration?item=OFFER:%20chair", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.IllustrationResult
	json2.Unmarshal(rsp(resp), &result)

	// Should process without error (will likely return ret=3 for uncached).
	assert.NotEqual(t, 2, result.Ret, "Should not return error for valid item with prefix")
}

func TestIllustrationWithWantedPrefix(t *testing.T) {
	// Test that WANTED: prefix is stripped.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/illustration?item=WANTED:%20desk", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.IllustrationResult
	json2.Unmarshal(rsp(resp), &result)

	assert.NotEqual(t, 2, result.Ret, "Should not return error for valid item with prefix")
}

func TestIllustrationWithLocationSuffix(t *testing.T) {
	// Test that location suffix is stripped.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/illustration?item=chair%20(Edinburgh)", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.IllustrationResult
	json2.Unmarshal(rsp(resp), &result)

	assert.NotEqual(t, 2, result.Ret, "Should not return error for valid item with suffix")
}

func TestIllustrationOnlyPrefixRemaining(t *testing.T) {
	// Test with just a prefix that gets stripped to empty.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/illustration?item=OFFER:", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.IllustrationResult
	json2.Unmarshal(rsp(resp), &result)

	assert.Equal(t, 2, result.Ret, "Should return ret=2 when only prefix with no item")
}

func TestIllustrationV2Endpoint(t *testing.T) {
	// Test the v2 endpoint.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/illustration?item=table", nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestOnlineV2Endpoint(t *testing.T) {
	// Test the v2 online endpoint.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/online", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result misc.OnlineResult
	json2.Unmarshal(rsp(resp), &result)
	assert.True(t, result.Online)
}

func TestLatestMessageV2Endpoint(t *testing.T) {
	// Test the v2 latestmessage endpoint.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/latestmessage", nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetImageDeliveryUrlBasic(t *testing.T) {
	// Test the image delivery URL generation with a basic UID.
	uid := "freegletusd-abc123def456"
	url := misc.GetImageDeliveryUrl(uid, "")

	// URL should contain the UID without the freegletusd- prefix.
	assert.Contains(t, url, "abc123def456")
	assert.NotContains(t, url, "freegletusd-")
}

func TestGetImageDeliveryUrlWithRotation(t *testing.T) {
	// Test the image delivery URL generation with rotation modifier.
	uid := "freegletusd-xyz789test000"
	mods := `{"rotate": 90}`
	url := misc.GetImageDeliveryUrl(uid, mods)

	// URL should contain the UID and the rotation parameter.
	assert.Contains(t, url, "xyz789test000")
	assert.Contains(t, url, "&ro=90")
}

func TestGetImageDeliveryUrlWithZeroRotation(t *testing.T) {
	// Test with zero rotation.
	uid := "freegletusd-test00000000"
	mods := `{"rotate": 0}`
	url := misc.GetImageDeliveryUrl(uid, mods)

	assert.Contains(t, url, "test00000000")
	assert.Contains(t, url, "&ro=0")
}

func TestGetImageDeliveryUrlEmptyMods(t *testing.T) {
	// Test with empty mods string.
	uid := "freegletusd-nomodifiers00"
	url := misc.GetImageDeliveryUrl(uid, "")

	assert.Contains(t, url, "nomodifiers00")
	assert.NotContains(t, url, "&ro=")
}

func TestGetImageDeliveryUrlInvalidJson(t *testing.T) {
	// Test with invalid JSON mods - should not add rotation.
	uid := "freegletusd-invalidjson00"
	mods := "not valid json"
	url := misc.GetImageDeliveryUrl(uid, mods)

	assert.Contains(t, url, "invalidjson00")
	// Should not crash, and should not add rotation parameter.
	assert.NotContains(t, url, "&ro=")
}
