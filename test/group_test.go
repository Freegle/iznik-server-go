package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/group"
	"github.com/freegle/iznik-server-go/router"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestListGroups(t *testing.T) {
	app := fiber.New()
	database.InitDatabase()
	router.SetupRoutes(app)

	// List groups
	resp, _ := app.Test(httptest.NewRequest("GET", "/api/group", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var groups []group.GroupEntry
	json2.Unmarshal(rsp(resp), &groups)

	assert.Greater(t, len(groups), 1)
	assert.Greater(t, groups[0].ID, uint64(0))
	assert.Greater(t, len(groups[0].Nameshort), 0)

	// Get the first group.
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groups[0].ID), nil))
	assert.Equal(t, 200, resp.StatusCode)
	var group group.Group
	json2.Unmarshal(rsp(resp), &group)

	assert.Equal(t, group.Nameshort, groups[0].Nameshort)

	// Check that it has volunteers.
	assert.Greater(t, len(group.GroupVolunteers), 0)

	// Get the second group.
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groups[1].ID), nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &group)

	assert.Equal(t, group.Nameshort, groups[1].Nameshort)

	// Get an invalid group.
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/group/1", nil))
	assert.Equal(t, 404, resp.StatusCode)
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/group/notanint", nil))
	assert.Equal(t, 404, resp.StatusCode)
}
