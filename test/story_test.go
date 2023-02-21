package test

import (
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestStory(t *testing.T) {
	// Get logged out.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story/1", nil))
	assert.Equal(t, 404, resp.StatusCode)
}
