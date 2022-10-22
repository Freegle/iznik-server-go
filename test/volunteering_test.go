package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/router"
	volunteering2 "github.com/freegle/iznik-server-go/volunteering"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestVolunteering(t *testing.T) {
	app := fiber.New()
	database.InitDatabase()
	router.SetupRoutes(app)

	// Get logged out.
	resp, _ := app.Test(httptest.NewRequest("GET", "/api/volunteering/1", nil))
	assert.Equal(t, 404, resp.StatusCode)

	var id []uint64

	db := database.DBConn

	db.Raw("SELECT volunteering.id FROM volunteering INNER JOIN volunteering_dates ON volunteering_dates.volunteeringid = volunteering.id WHERE pending = 0 AND deleted = 0 AND heldby IS NULL ORDER BY id DESC LIMIT 1").Pluck("id", &id)
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/volunteering/"+fmt.Sprint(id[0]), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var volunteering volunteering2.Volunteering
	json2.Unmarshal(rsp(resp), &volunteering)
	assert.Greater(t, volunteering.ID, uint64(0))
	assert.Greater(t, len(volunteering.Title), 0)
	assert.Greater(t, len(volunteering.Dates), 0)

	resp, _ = app.Test(httptest.NewRequest("GET", "/api/volunteering", nil))
	assert.Equal(t, 401, resp.StatusCode)

	_, token := GetUserWithToken(t)
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/volunteering?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.Greater(t, len(ids), 0)
}
