package test

import (
	json2 "encoding/json"
	"github.com/freegle/iznik-server-go/database"
	newsfeed2 "github.com/freegle/iznik-server-go/newsfeed"
	"github.com/freegle/iznik-server-go/router"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestFeed(t *testing.T) {
	app := fiber.New()
	database.InitDatabase()
	router.SetupRoutes(app)

	// Get logged out.
	resp, _ := app.Test(httptest.NewRequest("GET", "/api/newsfeed", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Should be able to get feed for a user.
	_, token := GetUserWithToken(t)

	resp, _ = app.Test(httptest.NewRequest("GET", "/api/newsfeed?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var newsfeed []newsfeed2.Newsfeed
	json2.Unmarshal(rsp(resp), &newsfeed)
	assert.Greater(t, len(newsfeed), 0)
}
