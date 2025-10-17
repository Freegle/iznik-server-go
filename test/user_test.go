package test

import (
	"encoding/json"
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

func TestGetUserByEmail(t *testing.T) {
	db := database.DBConn

	t.Run("Valid email returns exists true", func(t *testing.T) {
		// Find a user with a valid email
		var result struct {
			ID    uint64
			Email string
		}
		db.Raw("SELECT users.id, users_emails.email FROM users "+
			"INNER JOIN users_emails ON users_emails.userid = users.id "+
			"WHERE users.deleted IS NULL "+
			"LIMIT 1").Scan(&result)

		assert.Greater(t, result.ID, uint64(0))
		assert.NotEmpty(t, result.Email)

		// Test the API endpoint
		resp, err := getApp().Test(httptest.NewRequest("GET", "/api/user/byemail/"+result.Email, nil))
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		// Parse response
		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		assert.NoError(t, err)
		assert.True(t, response["exists"].(bool))
	})

	t.Run("Non-existent email returns exists false", func(t *testing.T) {
		resp, err := getApp().Test(httptest.NewRequest("GET", "/api/user/byemail/nonexistent@example.com", nil))
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		// Parse response
		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		assert.NoError(t, err)
		assert.False(t, response["exists"].(bool))
	})

	t.Run("Empty email returns 400", func(t *testing.T) {
		resp, err := getApp().Test(httptest.NewRequest("GET", "/api/user/byemail/", nil))
		assert.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode) // Route not found when email is empty
	})
}
