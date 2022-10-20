package test

import (
	json2 "encoding/json"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/router"
	user2 "github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"strconv"
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

func TestPersistent(t *testing.T) {
	app := fiber.New()
	database.InitDatabase()
	router.SetupRoutes(app)

	// This is the old-style persistent token used by the PHP API.
	token := GetPersistentToken()

	// Get the logged in user.
	req := httptest.NewRequest("GET", "/api/user", nil)
	req.Header.Set("Authorization2", token)
	resp, _ := app.Test(req)
	assert.Equal(t, 200, resp.StatusCode)
	var user2 user2.User
	json2.Unmarshal(rsp(resp), &user2)
	assert.Greater(t, user2.ID, uint64(0))
}

func TestSearches(t *testing.T) {
	app := fiber.New()
	database.InitDatabase()
	router.SetupRoutes(app)

	user, token := GetUserWithToken(t)

	// Get the logged in user.
	id := strconv.FormatUint(user.ID, 10)
	resp, _ := app.Test(httptest.NewRequest("GET", "/api/user/"+id+"/search?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	id = strconv.FormatUint(0, 10)
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/user/"+id+"/search?jwt="+token, nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestPublicLocation(t *testing.T) {
	app := fiber.New()
	database.InitDatabase()
	router.SetupRoutes(app)

	user, token := GetUserWithToken(t)

	// Get the logged in user.
	id := strconv.FormatUint(user.ID, 10)
	resp, _ := app.Test(httptest.NewRequest("GET", "/api/user/"+id+"/publiclocation?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var location user2.Publiclocation
	json2.Unmarshal(rsp(resp), &location)
	assert.Greater(t, len(location.Location), 0)
}
