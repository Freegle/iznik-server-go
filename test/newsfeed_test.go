package test

import (
	"fmt"
	json2 "encoding/json"
	newsfeed2 "github.com/freegle/iznik-server-go/newsfeed"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestFeed(t *testing.T) {
	// Get logged out - should return 401
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/newsfeed", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Create a full test user with newsfeed entry
	prefix := uniquePrefix("feed")
	userID, token := CreateFullTestUser(t, prefix)

	// Create a newsfeed entry for this user
	lat := 55.9533
	lng := -3.1883
	message := fmt.Sprintf("Test newsfeed message %s", prefix)
	newsfeedID := CreateTestNewsfeed(t, userID, lat, lng, message)

	// Should be able to get feed for a user
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/newsfeed?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var newsfeed []newsfeed2.Newsfeed
	json2.Unmarshal(rsp(resp), &newsfeed)
	assert.Greater(t, len(newsfeed), 0)

	// Get with distance
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/newsfeed?distance=10000&jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &newsfeed)
	assert.Greater(t, len(newsfeed), 0)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/newsfeed?distance=0&jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &newsfeed)
	assert.Greater(t, len(newsfeed), 0)

	// Get the specific newsfeed entry we created
	id := strconv.FormatUint(newsfeedID, 10)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/newsfeed/"+id, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var single newsfeed2.Newsfeed
	json2.Unmarshal(rsp(resp), &single)
	assert.Greater(t, single.ID, uint64(0))

	// Non-existent newsfeed should return 404
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/newsfeed/-1", nil))
	assert.Equal(t, 404, resp.StatusCode)

	// Get count - requires auth
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/newsfeedcount", nil))
	assert.Equal(t, 401, resp.StatusCode)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/newsfeedcount?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestNewsfeed_InvalidID(t *testing.T) {
	// Non-integer ID
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/newsfeed/notanint", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestNewsfeed_SingleWithAuth(t *testing.T) {
	// Single newsfeed with auth should also work
	prefix := uniquePrefix("feedsingleauth")
	userID, token := CreateFullTestUser(t, prefix)
	lat := 55.9533
	lng := -3.1883
	newsfeedID := CreateTestNewsfeed(t, userID, lat, lng, "Test single auth "+prefix)

	id := strconv.FormatUint(newsfeedID, 10)
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/newsfeed/"+id+"?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var single newsfeed2.Newsfeed
	json2.Unmarshal(rsp(resp), &single)
	assert.Equal(t, newsfeedID, single.ID)
}

func TestNewsfeed_V2Path(t *testing.T) {
	prefix := uniquePrefix("feedv2")
	_, token := CreateFullTestUser(t, prefix)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/newsfeed?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
}
