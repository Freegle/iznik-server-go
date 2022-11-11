package test

import (
	json2 "encoding/json"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/notification"
	"github.com/freegle/iznik-server-go/router"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestNotifications(t *testing.T) {
	app := fiber.New()
	database.InitDatabase()
	router.SetupRoutes(app)

	_, token := GetUserWithToken(t)

	resp, _ := app.Test(httptest.NewRequest("GET", "/api/notification/count?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	type Count struct {
		Count uint64
	}

	var count Count

	json2.Unmarshal(rsp(resp), &count)

	resp, _ = app.Test(httptest.NewRequest("GET", "/api/notification?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var notifications []notification.Notification

	json2.Unmarshal(rsp(resp), &notifications)
	assert.GreaterOrEqual(t, uint64(len(notifications)), count.Count)
}
