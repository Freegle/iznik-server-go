package test

import (
	json2 "encoding/json"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/router"
	user2 "github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestAuth(t *testing.T) {
	app := fiber.New()
	database.InitDatabase()
	router.SetupRoutes(app)

	user, token := GetUserWithToken(t)

	// Get the logged in user.
	resp, _ := app.Test(httptest.NewRequest("GET", "/api/user?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var user2 user2.User
	json2.Unmarshal(rsp(resp), &user2)

	// Should match the user we tried to log in as.
	assert.Equal(t, user2.ID, user.ID)

	// Should see memberships.
	assert.Greater(t, len(user2.Memberships), 0)
}
