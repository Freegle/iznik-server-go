package test

import (
	json2 "encoding/json"
	"github.com/freegle/iznik-server-go/database"
	newsfeed2 "github.com/freegle/iznik-server-go/newsfeed"
	"github.com/freegle/iznik-server-go/router"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"strconv"
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

	// Get with distance
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/newsfeed?distance=100&jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &newsfeed)
	assert.Greater(t, len(newsfeed), 0)

	resp, _ = app.Test(httptest.NewRequest("GET", "/api/newsfeed?distance=0&jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &newsfeed)
	assert.Greater(t, len(newsfeed), 0)

	// Get individual
	id := strconv.FormatUint(newsfeed[0].ID, 10)
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/newsfeed/"+id, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var single newsfeed2.Newsfeed
	json2.Unmarshal(rsp(resp), &single)
	assert.Greater(t, single.ID, uint64(0))

	resp, _ = app.Test(httptest.NewRequest("GET", "/api/newsfeed/-1", nil))
	assert.Equal(t, 404, resp.StatusCode)

	// Get count
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/newsfeedcount", nil))
	assert.Equal(t, 401, resp.StatusCode)

	resp, _ = app.Test(httptest.NewRequest("GET", "/api/newsfeedcount?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
}
