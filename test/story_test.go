package test

import (
	json2 "encoding/json"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestStory(t *testing.T) {
	// Get logged out.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story/1", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestListStory(t *testing.T) {
	// Get logged out.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
}

func TestGroupStory(t *testing.T) {
	// Get logged out.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story/group/0", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
}
