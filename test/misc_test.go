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
