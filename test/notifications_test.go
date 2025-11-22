package test

import (
	json2 "encoding/json"
	"github.com/freegle/iznik-server-go/notification"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestNotifications(t *testing.T) {
	prefix := uniquePrefix("notif")
	_, token := CreateFullTestUser(t, prefix)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/notification/count?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	type Count struct {
		Count uint64
	}

	var count Count

	json2.Unmarshal(rsp(resp), &count)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/notification?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var notifications []notification.Notification

	json2.Unmarshal(rsp(resp), &notifications)
	assert.GreaterOrEqual(t, uint64(len(notifications)), count.Count)
}
