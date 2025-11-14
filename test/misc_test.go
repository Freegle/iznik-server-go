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
