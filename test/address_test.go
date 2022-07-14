package test

import (
	json2 "encoding/json"
	"fmt"
	address2 "github.com/freegle/iznik-server-go/address"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/router"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestAddress(t *testing.T) {
	app := fiber.New()
	database.InitDatabase()
	router.SetupRoutes(app)

	// Get logged out.
	resp, _ := app.Test(httptest.NewRequest("GET", "/api/address", nil))
	assert.Equal(t, 401, resp.StatusCode)

	user, token := GetUserWithToken(t)

	resp, _ = app.Test(httptest.NewRequest("GET", "/api/address?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var addresses []address2.Address
	json2.Unmarshal(rsp(resp), &addresses)
	assert.Greater(t, len(addresses), 0)
	assert.Equal(t, addresses[0].Userid, user.ID)
	fmt.Printf("%#v\n", addresses)
}
