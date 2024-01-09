package test

import (
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestDeleted(t *testing.T) {
	db := database.DBConn

	// Find a user with deleted not null
	var uid uint64
	db.Raw("SELECT id FROM users WHERE deleted IS NOT NULL LIMIT 1").Scan(&uid)
	assert.Greater(t, uid, uint64(0))

	// Get of the user should work, even though they're deleted.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/user/"+fmt.Sprint(uid), nil))
	assert.Equal(t, 200, resp.StatusCode)
}
