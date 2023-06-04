package test

import (
	json2 "encoding/json"
	"github.com/freegle/iznik-server-go/config"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestConfig(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/config/wibble", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var results []config.ConfigItem
	json2.Unmarshal(rsp(resp), &results)
	assert.Equal(t, len(results), 0)
}
